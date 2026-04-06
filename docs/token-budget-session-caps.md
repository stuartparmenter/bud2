---
topic: Token Budget & Session Caps
repo: bud2
generated_at: 2026-04-06T08:30:00Z
commit: 07547c79
key_modules: [internal/budget, internal/executive]
score: 0.74
---

# Token Budget & Session Caps

> Repo: `bud2` | Generated: 2026-04-06 | Commit: 07547c79

## Summary

This subsystem enforces two distinct resource limits on Claude session activity: a daily output-token budget that gates whether autonomous work may begin, and per-session context and duration caps that determine when a running session must reset or terminate. Together they prevent runaway token spend while ensuring user-initiated sessions are never blocked by budget state.

## Key Data Structures

### `ThinkingBudget` (`internal/budget/budget.go`)

Wraps a `SessionTracker` and exposes the single gate `CanDoAutonomousWork() (bool, string)`. Key fields:

- `DailyOutputTokens int` — cumulative output-token limit per 24 h (default **1,000,000**)
- `MaxSessionDuration time.Duration` — max length per session (default **10 min**); stored here but enforcement is done via context timeout in the executive

### `BudgetStatus` (`internal/budget/budget.go`)

Read-only snapshot returned by `GetStatus()`. Includes `TodayOutputTokens`, `DailyOutputTokenLimit`, `RemainingOutputTokens`, `TodaySessions`, `TodayTurns`, `ActiveSessions`, `LongestActiveDur`, and `CanDoAutonomous`. Used for observability (status MCP tool).

### `Session` (`internal/budget/sessions.go`)

Represents one Claude session's lifecycle and usage:

```go
type Session struct {
    ID          string
    ThreadID    string
    StartedAt   time.Time
    CompletedAt *time.Time
    DurationSec float64
    InputTokens              int
    OutputTokens             int
    CacheCreationInputTokens int
    CacheReadInputTokens     int
    NumTurns                 int
}
```

Token fields are populated after the Claude CLI emits a `result` event via `SetSessionUsage()`.

### `TokenUsage` (`internal/budget/sessions.go`)

Daily aggregation of all completed sessions:

```go
type TokenUsage struct {
    InputTokens, OutputTokens int
    CacheCreationInputTokens, CacheReadInputTokens int
    SessionCount, TotalTurns int
}
```

Returned by `TodayTokenUsage()`. Only completed sessions contribute; active sessions are not counted.

### `SessionTracker` (`internal/budget/sessions.go`)

Manages in-memory session state with disk persistence:

- `active map[string]*Session` — sessions currently running (keyed by session ID)
- `completed []*Session` — sessions finished today
- `today string` — date string; `checkDayRollover()` resets state on new day
- Persisted to `state/sessions.json` via `save()` / `load()`

### `SignalProcessor` (`internal/budget/signals.go`)

Reads `state/signals.jsonl` incrementally (via a byte-offset cursor) and calls `tracker.CompleteSession()` when a `signal_done` signal arrives:

```go
type Signal struct {
    Type      string    // e.g. "session_complete"
    SessionID string
    Summary   string
    Timestamp time.Time
}
```

The `onComplete func(session *Session, summary string)` callback is fired after session state is updated.

### `SimpleSession` (`internal/executive/simple_session.go`)

Manages a single persistent Claude session. Relevant to budget:

- `MaxContextTokens = 150_000` — threshold at which `ShouldReset()` returns true; uses `cache_read_input_tokens + input_tokens` from the previous prompt's `ResultMessage`
- `lastUsage *SessionUsage` — token counts from the last completed prompt
- `claudeSessionID string` — the `--resume` value; cleared by `Reset()` and `PrepareNewSession()`, preserved by `PrepareForResume()`

### `ExecutiveV2Config` (`internal/executive/executive_v2.go`)

The `MaxAutonomousSessionDuration time.Duration` field (recommended: 8–10 min) is the live enforcement mechanism for per-session duration, applied as a `context.WithTimeout` in `processItem()`.

## Lifecycle

### Daily token budget check

1. **Gate check**: before starting any autonomous wake, the executive (likely in `cmd/bud/main.go` or the focus processing loop) calls `ThinkingBudget.CanDoAutonomousWork()`.
2. **Aggregation**: `CanDoAutonomousWork()` calls `canDoAutonomousInternal()`, which calls `tracker.TodayTokenUsage()` and compares `OutputTokens` to `DailyOutputTokens`.
3. **Active sessions do not block**: the test comment "Active sessions should NOT block autonomous work (attention handles priority)" confirms that in-flight sessions are excluded from the gate check. Only completed sessions' output tokens are summed.
4. **Block result**: if `todayOutputTokens >= DailyOutputTokens`, returns `(false, reason)` and the wake is skipped.

### Session tracking lifecycle

1. **Start**: `tracker.StartSession(sessionID, threadID)` inserts a new `Session` into `active`. Called before `SendPrompt`.
2. **Signal delivery**: the Claude subprocess calls the `signal_done` MCP tool, which writes a JSON line to `state/signals.jsonl`.
3. **Signal consumption**: `SignalProcessor.ProcessSignals()` polls the file (default interval from `Start(interval)`), seeks to the last offset, reads new lines, and calls `handleSignal()`.
4. **Completion**: `handleSignal()` calls `tracker.CompleteSession(sessionID)` which moves the session from `active` to `completed`, sets `CompletedAt` and `DurationSec`, then calls the `onComplete` callback.
5. **Usage recording**: the executive calls `tracker.SetSessionUsage(sessionID, input, output, cacheCreate, cacheRead, turns)` after extracting token counts from the `ResultMessage`.
6. **Persistence**: `save()` writes `state/sessions.json` after each mutation.
7. **Day rollover**: `checkDayRollover()` (called on every read) compares today's date string; on mismatch, `completed` is cleared and `today` is updated.

### Context window cap

1. **Check**: at the start of `processItem()`, `session.ShouldReset()` is evaluated. It returns `true` when `lastUsage.CacheReadInputTokens + lastUsage.InputTokens > 150_000`.
2. **New vs. resume**: if `ShouldReset()` is false and `ClaudeSessionID()` is non-empty, `PrepareForResume()` is called — preserving `claudeSessionID`, `seenMemoryIDs`, and `lastBufferSync`. If `ShouldReset()` is true, `PrepareNewSession()` is called instead, clearing `claudeSessionID` to force a fresh Claude session.
3. **First prompt**: `lastUsage` is nil on first prompt (or after `Reset()`), so `ShouldReset()` returns false — no spurious resets.
4. **Restart recovery**: on Bud restart, `LoadSessionFromDisk()` restores `claudeSessionID` from `state/exec-session.json`. `lastUsage` is not restored, so `ShouldReset()` returns false for the first post-restart prompt even if the old session was near the limit.

### Autonomous session duration cap

1. **Timeout setup**: in `processItem()`, for wake-type focus items, `context.WithTimeout(ctx, MaxAutonomousSessionDuration)` wraps the session context. Zero means no cap.
2. **Subprocess termination**: when the timeout fires, the context is cancelled, which terminates the Claude subprocess in `SendPrompt` via the SDK's cancellation path.
3. **signal_done interaction**: `SignalDone()` also cancels the session via `signalDoneCancel`. Whichever fires first (signal_done or timeout) terminates the subprocess; the other becomes a no-op.
4. **User sessions**: `MaxAutonomousSessionDuration` is only applied to wake (autonomous) items. User-initiated P1 items likely run with a longer `sessionTimeout = 30 * time.Minute` hardcoded in `simple_session.go`.

## Design Decisions

- **Only output tokens count toward the daily budget**: input tokens (even at 100K+ per session) are excluded. Output tokens represent actual generation cost and are the meaningful spend metric. This means a day of heavy reading (long prompts, large context) doesn't block further work.
- **Active sessions excluded from budget check**: including in-flight sessions in the gate would create a deadlock where a running session prevents the next one from starting. The budget gates *starting* new autonomous sessions, not concurrent execution.
- **Context reset uses cache_read + input, not output**: `ShouldReset()` measures how full the context window is, not how much was generated. `cache_read_input_tokens` reflects the cached session history being loaded; combined with the new input, this tells how close to the 200K window limit the session is. The 150K threshold leaves ~50K headroom for the current prompt and response.
- **`MaxSessionDuration` on `ThinkingBudget` vs. `MaxAutonomousSessionDuration` on `ExecutiveV2Config`**: the budget struct field exists but the actual enforcement is done in the executive via context timeout, not inside the budget package. The budget field is available for status reporting but is not wired to terminate anything directly.
- **SignalProcessor offset is in-memory only**: on Bud restart, the processor re-reads `signals.jsonl` from offset 0, but duplicate `signal_done` signals are harmless — `CompleteSession()` silently ignores IDs not in `active`.

## Integration Points

| From | To | What crosses the boundary |
|------|----|--------------------------|
| `internal/budget` | file system | Reads `state/signals.jsonl` (append-only), writes `state/sessions.json` (full JSON) |
| `internal/executive` | `internal/budget` | `ExecutiveV2Config.SessionTracker` field; executive calls `StartSession`, `SetSessionUsage`, and `CanDoAutonomousWork` |
| `internal/executive/simple_session.go` | `internal/executive/executive_v2.go` | `ShouldReset()` checked in `processItem()` before each prompt; result determines `PrepareForResume` vs. `PrepareNewSession` |
| `internal/mcp` | `internal/budget` | `signal_done` MCP tool writes to `signals.jsonl`; `SignalProcessor` reads it asynchronously |
| `cmd/bud` | `internal/budget` | Wires `SessionTracker` and `ThinkingBudget` at startup; `ThinkingBudget` consulted before wake dispatch |

## Non-Obvious Behaviors

- **Token budget resets at midnight, not on a rolling 24-hour window**: `checkDayRollover()` compares a date string (e.g. `"2026-04-06"`), so 1M tokens used at 11:59 PM clears at 12:00 AM — not 24 hours later. A large burst just before midnight effectively gets a free reset.
- **`PrepareNewSession()` doesn't clear `seenMemoryIDs`**: only `Reset()` clears them. After a context-triggered reset, the new session still skips memories already sent in previous turns of the same logical "day." This is intentional to avoid re-injecting memories the user just saw.
- **Session ID is a two-layer concept**: `session.sessionID` is an internal tracking UUID generated fresh each turn by `PrepareForResume()` / `PrepareNewSession()`. `session.claudeSessionID` is the Claude-assigned ID used for `--resume`. They are completely separate; the internal ID is for `SessionTracker`, the Claude ID is for SDK session resumption.
- **`ShouldReset()` reads stale data**: it uses `lastUsage` from the *previous* prompt's result, not the current one. The context decision is always one turn behind — the reset triggers after the turn that crossed the threshold, not before.
- **`SaveSessionToDisk()` must be called without holding `s.mu`**: the comment in `simple_session.go` notes callers must not hold the lock. This is easy to miss when adding resume paths.
- **Wake sessions skip memory retrieval entirely**: `buildContext()` in `executive_v2.go` contains a comment that autonomous wake sessions skip Engram memory retrieval because analysis showed 48% of wake memories rated 1/5, dragging precision to 29.6%. Budget-gated wakes thus run with less context than user sessions.

## Start Here

- `internal/budget/budget.go` — `ThinkingBudget` and `CanDoAutonomousWork`; the primary gate for autonomous sessions
- `internal/budget/sessions.go` — `SessionTracker` and `Session`; understand how token counts are stored and aggregated daily
- `internal/budget/signals.go` — `SignalProcessor`; the bridge between the MCP `signal_done` call and session completion bookkeeping
- `internal/executive/simple_session.go` — `ShouldReset()` and `MaxContextTokens`; context-window cap enforcement
- `internal/executive/executive_v2.go` around `processItem()` — where `ShouldReset()`, `MaxAutonomousSessionDuration`, and `SessionTracker.StartSession` are all wired together
