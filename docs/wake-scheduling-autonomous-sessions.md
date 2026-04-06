---
topic: Wake Scheduling & Autonomous Sessions
repo: bud2
generated_at: 2026-04-06T07:34:52Z
commit: 82b256a6
key_modules: [internal/executive, internal/focus, internal/budget]
score: 0.74
---

# Wake Scheduling & Autonomous Sessions

> Repo: `bud2` | Generated: 2026-04-06 | Commit: 82b256a6

## Summary

Bud runs periodic autonomous sessions ("wakes") in which Claude is given a checklist of background work to do without any user message triggering it. The system uses a configurable timer, an adaptive quiet-mode interval, a token-budget gate, and a hard session-duration cap to ensure autonomous work happens regularly but stays bounded. Wakes follow the same executive pipeline as user-triggered sessions, with specific adaptations: memory retrieval is skipped, a wakeup checklist is injected, and sessions are capped to stay coordinator-style rather than doing deep work directly.

## Key Data Structures

### `ThinkingBudget` (`internal/budget/budget.go`)
Controls whether autonomous work is allowed. Holds a reference to `SessionTracker` and enforces two limits: `DailyOutputTokens` (default 1M tokens/day) and `MaxSessionDuration` (default 10 minutes, unused in practice — the executive-level `MaxAutonomousSessionDuration` takes precedence). `CanDoAutonomousWork()` returns false when today's output token count exceeds the daily limit.

### `SessionTracker` (`internal/budget/sessions.go`)
Tracks active and completed sessions in memory (plus JSON on disk). On each daily rollover it resets the `completed` slice. `TodayTokenUsage()` aggregates token counts across all completed sessions for the current day — this is what `ThinkingBudget` reads. Persists to `state/system/sessions.json`; loaded on startup so daily totals survive restarts.

### `SignalProcessor` (`internal/budget/signals.go`)
Reads `state/system/signals.jsonl` in tail-follow mode. The MCP server writes `session_complete` signals containing token usage, which `SignalProcessor` parses and forwards to `SessionTracker.SetSessionUsage()`. This is the only path by which token counts make it into the daily budget. Runs in a background goroutine polling at a configurable interval.

### `PendingItem` (`internal/focus/types.go`)
The unit of work that flows through the attention system. For wakes, `Type = "wake"`, `Priority = P3ActiveWork (3)`, `Source = "impulse"`. The `Data` map carries `"trigger": "periodic"`, `"last_user_session_ts"`, and `"autonomous_handoff"` (the contents of `state/system/autonomous-handoff.md` — used by Claude to pick up where the previous wake left off).

### `ExecutiveV2Config` (`internal/executive/executive_v2.go`)
Configuration struct for the executive. Wake-relevant fields:
- `MaxAutonomousSessionDuration` — hard cap on wake session wall time (default 8m, env `AUTONOMOUS_SESSION_CAP`)
- `WakeupInstructions` — content of `seed/wakeup.md`, injected into wake prompts
- `DefaultChannelID` — used to fetch recent conversation for wake context

### `ContextBundle` (`internal/focus/types.go`)
Assembled context passed from `buildContext` to `buildPrompt`. For wakes, `Memories` is always nil/empty (retrieval is skipped), and `WakeSessionContext` holds the last 15 episodes at C16 compression.

## Lifecycle

1. **Timer goroutine starts** (`cmd/bud/main.go:1367`): A goroutine sleeps 10 seconds on startup, then loops with an adaptive interval. Base interval: `AUTONOMOUS_INTERVAL` env (default 2h). If the last user input was more than 4 hours ago, the interval doubles ("quiet mode").

2. **Idle gate check**: Before creating an impulse, the goroutine checks two conditions that cause it to `continue` (skip this wake): (a) `time.Since(lastInput) < autonomousIdleRequired` (env `AUTONOMOUS_IDLE_REQUIRED`, default 0 = disabled), and (b) `exec.IsP1Active()` — skip if a user session is currently running.

3. **Impulse creation**: A `types.Impulse{Type: "wake"}` is constructed with trigger metadata and the current autonomous handoff note. It is wrapped into an `InboxMessage` via `memory.NewInboxMessageFromImpulse` and pushed through `processInboxMessage`.

4. **Budget gate** (`cmd/bud/main.go:852`): In `processInboxMessage`, after the reflex engine (which likely passes wakes through), the percept source is checked. `isAutonomous = (percept.Source == "impulse" || percept.Source == "system")`. For autonomous items, `thinkingBudget.CanDoAutonomousWork()` is called. If it returns false, the percept is dropped with a debug log. High-priority urgent task impulses (P1, type "due"/"upcoming") bypass this gate.

5. **Queue enqueue**: The wake percept is enqueued as `P3ActiveWork` in `focus.Queue`. The queue signals its `notifyCh` channel (buffered 1) to wake listeners.

6. **Background dispatch** (`cmd/bud/main.go:1348`): A separate goroutine polls `exec.ProcessNextBackground(ctx)` every 500ms. `ProcessNextBackground` calls `queue.PopHighestMinPriority(P2DueTask)` — items at P2 or lower urgency (P2, P3, P4). Wake items at P3 qualify.

7. **Session resume decision** (`executive_v2.go:processItem`): The executive checks `session.ShouldReset()` (context tokens > 150K) and whether a `claudeSessionID` exists on disk. If a valid session ID exists and context isn't full, `PrepareForResume()` is called, setting `isResuming = true` so `buildPrompt` skips static context already in the Claude session history.

8. **Context assembly** (`executive_v2.go:buildContext`): For wake items, `item.Type == "wake"` triggers two divergences from the normal path: (a) memory retrieval is skipped entirely — a data analysis showed 48% of wake memories rated 1/5, dragging precision to 29.6%; (b) `buildRecentConversationForWake` is called instead of `buildRecentConversation`, fetching the last 15 episodes at C16 compression with a 1500-token budget.

9. **Session cap applied** (`executive_v2.go:894`): When `item.Type == "wake"` and `MaxAutonomousSessionDuration > 0`, a `context.WithTimeout` is used instead of `context.WithCancel`. The timeout fires at 8 minutes (default), cancelling the Claude subprocess regardless of whether `signal_done` was called.

10. **Prompt construction** (`executive_v2.go:buildPrompt`): For wake items, `buildPrompt` injects two additional sections: the `WakeupInstructions` block (content of `seed/wakeup.md`) and the `WakeSessionContext` (recent conversation at C16). The last user session timestamp and previous autonomous session handoff note are also surface from `item.Data`.

11. **Claude execution**: `SimpleSession.SendPrompt` is called with a 30-minute SDK-level timeout (always). The wake-specific `sessionCtx` timeout of 8 minutes takes effect first. Claude calls `signal_done` when done → `SignalDone()` cancels `sessionCtx` → subprocess exits.

12. **Post-completion bookkeeping**: The `claudeSessionID` from the `ResultMessage` is saved to `state/system/executive-session.json`. Token usage is recorded via `SessionTracker`. The `OnExecDone` callback fires, logging the session summary. The executive returns; the background goroutine resumes polling.

## Design Decisions

- **Memory retrieval skipped for wakes**: A data analysis of wake sessions found 48% of retrieved memories rated 1/5 by Claude and overall retrieval precision at 29.6% for wake prompts. Wake prompts are generic ("do background work") and don't anchor embedding search the way user messages do. The decision is documented inline at `executive_v2.go:1095`.

- **Adaptive quiet-mode interval**: Rather than stopping wakes entirely when the user is inactive, the interval doubles. This preserves background maintenance during off-hours while halving API cost. The 4-hour threshold is hardcoded in `main.go:1380`.

- **Session ID persistence across restarts**: `SaveSessionToDisk()` writes the `claudeSessionID` to disk after every wake. On restart, `LoadSessionFromDisk()` restores it. This lets sequential wakes continue a single Claude session, preserving reasoning context across Bud restarts without user awareness.

- **Concurrent P1/background processing**: `ProcessNextP1` and `ProcessNextBackground` operate on separate queue slices (via `PopHighestMaxPriority` vs `PopHighestMinPriority`) and run in parallel goroutines. When a P1 item arrives during a wake, `RequestBackgroundInterrupt()` cancels the background context so the user gets a prompt response.

- **8-minute cap for coordinator style**: Wake sessions are intentionally kept short so Claude delegates deep work to subagents rather than doing it inline. `MaxAutonomousSessionDuration` defaults to 8 minutes; the recommendation in the config comment is 8–10 minutes.

## Integration Points

| From | To | What crosses the boundary |
|------|----|--------------------------|
| `cmd/bud/main.go` | `internal/budget` | `CanDoAutonomousWork()` gate; `NewSessionTracker` + `NewThinkingBudget` initialization |
| `cmd/bud/main.go` | `internal/focus` | Wake impulse enqueued to `focus.Queue`; background goroutine drains via `ProcessNextBackground` |
| `internal/executive` | `internal/budget` | `SessionTracker` callbacks in `ExecutiveV2Config`; token usage recorded via `SetSessionUsage` |
| `internal/executive` | `internal/focus` | `buildContext` reads `PendingItem.Type` and `PendingItem.Data` to customize wake prompt |
| `internal/executive` | `internal/engram` | `buildRecentConversationForWake` fetches the last 15 episodes for wake context |
| `internal/budget.SignalProcessor` | `internal/budget.SessionTracker` | Parses `signals.jsonl` to update token usage from MCP server events |
| MCP server (signals.jsonl) | `internal/budget` | `session_complete` signal written by MCP server; picked up by `SignalProcessor` |

## Non-Obvious Behaviors

- **Wakes can resume a Claude session that survived a Bud restart**: `claudeSessionID` is persisted to disk. If Bud crashes mid-wake and restarts, the next wake will call `--resume` on the old session ID. If that session no longer exists on Anthropic's side, `isSessionNotFoundError` catches the error and retries without `--resume`.

- **The startup impulse fires regardless of the timer**: On launch, a hardcoded impulse is enqueued 3 seconds after startup (`main.go:1250`) to check for interrupted subagents. It uses the same "wake" type as periodic wakes but is tagged `"impulse:startup"`.

- **Quiet mode doubles the interval but uses a fresh timer each iteration**: The goroutine is not sleeping for the doubled interval — it reads `lastInput` at the top of each iteration and creates a new `time.NewTimer(interval)`. This means a user message arriving mid-sleep doesn't shorten the quiet-mode wait; it only affects the *next* iteration's interval calculation.

- **Token usage does not flow from the Claude subprocess directly into `ThinkingBudget`**: The path is indirect: Claude CLI writes a `session_complete` signal to `signals.jsonl` → `SignalProcessor` reads it → `SessionTracker.SetSessionUsage()` → `ThinkingBudget.CanDoAutonomousWork()` reads the aggregated daily total. There is a brief window between session completion and signal processing where the budget appears under-counted.

- **High-priority urgent task impulses bypass the budget gate**: Items at P1 with `impulse_type == "due"` or `"upcoming"` skip the `CanDoAutonomousWork()` check (`main.go:864`). This ensures calendar reminders and due task alerts always fire, even when the daily token budget is exhausted.

- **The background goroutine polls at 500ms even when no items are queued**: There is no blocking wait on `queue.NotifyChannel()` in the background goroutine — it simply calls `ProcessNextBackground` on a fixed 500ms ticker. `ProcessNextBackground` returns `(false, nil)` immediately when the queue is empty.

## Start Here

- `cmd/bud/main.go:1367` — the autonomous wake goroutine: adaptive interval calculation, idle gate, and impulse creation
- `cmd/bud/main.go:852` — the budget gate inside `processInboxMessage`: where `CanDoAutonomousWork()` is checked and autonomous percepts are dropped
- `internal/executive/executive_v2.go:890` — session cap application: where `context.WithTimeout` vs `context.WithCancel` is decided based on `item.Type == "wake"`
- `internal/budget/budget.go` — `ThinkingBudget.CanDoAutonomousWork()`: the gate logic and daily token limit check
- `internal/executive/executive_v2.go:1052` — `buildContext` for wake items: the skip of memory retrieval and construction of `WakeSessionContext`
