# Architectural Analysis: bud2

Date: 2026-04-10

This report covers four dimensions of architectural health: dead/unused components, overlapping/duplicate components, poor architectural fit, and design improvement opportunities.

---

## 1. Dead / Unused Components

### 1.1 `internal/memory/` Pool Types — ~825 lines of dead code

**Files:** `internal/memory/percepts.go`, `traces.go`, `threads.go`, `impulses.go`

`PerceptPool`, `TracePool`, `ThreadPool`, and `ImpulsePool` are fully implemented with CRUD, persistence (`Load`/`Save`), and domain-specific methods (activation decay, spread, reinforcement, pruning). **None of their constructors are called anywhere outside their own test files.** The system migrated from local file-based memory pools to the external Engram service (`internal/engram/`), but the old implementations were never removed.

Only `InboxMessage` from `memory/inbox.go` sees active use — the rest of the package is inert.

**Killed methods include:** `Add`, `Get`, `GetMany`, `ExpireOlderThan`, `All`, `Load`, `Save`, `Clear`, `Count`, `DecayActivation`, `SpreadActivation`, `GetActivated`, `FindSimilar`, `Reinforce`, `PruneWeak`, `HasSource`, `GetCore`, `HasCore`, `SetCore`, `Delete`, `ClearNonCore`, `ClearCore`, `ByStatus`, `Active`, `BySessionState`, `Focused`, `AddPerceptRef`, `ClearByStatus`, `Remove`, `GetAll`, `GetPending`, `Expire`.

**Direction:** Delete the four pool types entirely. Keep `InboxMessage` (move to a `messaging` or `pipeline` package if desired).

---

### 1.2 `ExecutiveV2.ProcessNext()` — Never called in production

**File:** `internal/executive/executive_v2.go:612`

`ProcessNext()` uses `Attention.SelectNext()` with salience-gating, but production exclusively calls `ProcessNextP1()` and `ProcessNextBackground()`, which bypass the attention threshold. This is an evolutionary remnant.

**Direction:** Delete `ProcessNext()`.

---

### 1.3 `focus.Attention` — Mostly dead code

**File:** `internal/focus/attention.go`

The entire `Attention` struct's sophisticated features are unused in production:
- `SetCallback()` / `FocusCallback` — never called outside tests
- `DecayArousal()` — never called outside tests
- `SelectNext()` with salience threshold — never called (see 1.2)
- `IsAttending()` / `SetMode()` / `ClearMode()` / `GetActiveMode()` — only the reflex engine's `Process()` calls `IsAttending()`, but `SetAttention()` is never called from main
- `computeSalience()` / `adjustArousal()` / `getSelectionThreshold()` — only invoked by unused `SelectNext()`

In production, `Attention` is only used as a **suspended-item stack** (`Focus()` and `Complete()`).

**Direction:** Simplify `Attention` to just a suspended-item stack, removing attention/arousal/threshold code.

---

### 1.4 `focus.Queue` — Multiple dead methods

**File:** `internal/focus/queue.go`

- `Peek()` (line 79) — never called
- `FilterByPriority()` (line 220) — never called outside the package
- `FilterByType()` (line 234) — never called anywhere
- `Count()` (line 248) — never called
- `Clear()` (line 255) — never called
- `Save()` (line 330) — never called (only `Load()` is called at startup)

**Direction:** Delete dead methods.

---

### 1.5 `SimpleSession` — Multiple unused tracking methods

**File:** `internal/executive/simple_session.go`

- `HasSeenItem()` / `MarkItemsSeen()` — item-level dedup never used; only memory-level dedup via `HasSeenMemory`/`MarkMemoriesSeen` is used
- `UpdateBufferSync()` / `LastBufferSync()` — written/queried but values never read by callers
- `IncrementUserMessages()` / `UserMessageCount()` — never called
- `IsFirstPrompt()` — never called

**Direction:** Delete dead methods.

---

### 1.6 `RunCustomSession` and supporting types — Dead code path

**File:** `internal/executive/custom_session.go`

Implements a one-shot `claude --print` CLI invocation pattern. Only tested, never called from production code. The `StreamEvent` and `ToolUse` types in `types.go:78-95` are only consumed here.

**Direction:** Delete `custom_session.go` and related types in `types.go`.

---

### 1.7 `budget.SignalProcessor` — Entirely dead

**File:** `internal/budget/signals.go` (120 lines)

`SignalProcessor`, `Signal`, `SetOnComplete()`, `ProcessSignals()`, `Start()`, and `handleSignal()` are never instantiated or called outside tests. The main binary processes signals via `handleSignal()` in `cmd/bud/main.go:718` (a local closure using `InboxMessage` — not `signals.jsonl`).

**Direction:** Delete `internal/budget/signals.go` entirely.

---

### 1.8 `budget.ThinkingBudget` — Multiple dead methods and fields

**File:** `internal/budget/budget.go`

- `MaxSessionDuration` (line 15) — set to 10m but never enforced; real capping uses `autonomousSessionCap` in executive config
- `GetStatus()` (line 41) — returns `BudgetStatus` but never called in production
- `LogStatus()` (line 90) — never called
- `BudgetStatus` type (lines 75-87) — populated but never consumed

**Direction:** Remove dead fields/methods. Consider splitting `SessionTracker` out of the `budget` package.

---

### 1.9 `internal/tmux/` — Completely dead package (108 lines)

**File:** `internal/tmux/windows.go`

`main.go` imports only `internal/zellij` (aliased misleadingly as `tmuxwindow`). The tmux package is **never imported** anywhere in production.

**Direction:** Delete `internal/tmux/`. If tmux support is needed later, create a proper `TerminalWindowManager` interface.

---

### 1.10 Embedding client dead code — ~100 lines

**File:** `internal/embedding/ollama.go`

- `Generate()` method — never called
- `Summarize()` method — never called
- `SetGenerationModel()` — never called
- `generateRequest` / `generateResponse` types — only used by dead `Generate()`
- `AverageEmbeddings()` — never called
- `UpdateCentroid()` — never called

Only `Embed()` and `CosineSimilarity()` see use (in `memory/traces.go`, itself dead pool code).

**Direction:** Delete dead generation methods and types.

---

### 1.11 Calendar client dead code

**File:** `internal/integrations/calendar/client.go`

- `CalendarInfo` struct and `ListCalendars()` — never called
- `PrimaryCalendarID()` — never called (only `CalendarIDs()` is used)
- `Event.Duration()`, `Event.IsHappeningSoon()`, `Event.IsHappeningNow()`, `Event.ToJSON()` — never called

**Direction:** Delete dead methods/types.

---

### 1.12 GitHub client dead code

**File:** `internal/integrations/github/client.go`

- `FormatItemsCompact()` and `ProjectItem.CompactItem()` — never called (MCP tools use `json.MarshalIndent`)

**Direction:** Delete dead methods.

---

### 1.13 MCP Tools: Dead and no-op tools

**File:** `internal/mcp/tools/register.go`

- `registerMotivationTools` (line 640) — **empty function body** with comment "migrated to Things MCP." Still called at line 42.
- `GKToolNames()` (`gk.go:395-407`) — exported, never called
- `memory_flush` tool (line 762) — no-op handler that returns "Engram handles consolidation automatically." Misleading to callers.
- `ctx any` parameter on all tool handlers — always `nil`, never read

**Direction:** Delete empty `registerMotivationTools` and its call. Remove `GKToolNames()`. Either make `memory_flush` useful or remove it. Remove `ctx any` from `ToolHandler` signature.

---

### 1.14 `types.go` — Massively over-declared struct fields

**File:** `internal/types/types.go`

These fields are declared but **never populated or read** in production:

- `Percept.RawInput`, `ProcessedBy`, `DialogueAct`, `EntropyScore`, `Features`, `Embedding`, `ReplyTo`
- `Thread.Features` (struct `ThreadFeatures`), `Thread.Embeddings` (struct `ThreadEmbeddings`), `Thread.WindowName`, `Thread.ErrorCount`, `Thread.LastError`
- `Trace.Inhibits`, `Trace.LabileUntil`, `Trace.IsLabile()`, `Trace.MakeLabile()`, `Trace.Recency()`
- `Percept.Recency()`, `Impulse.Recency()`
- `Arousal` type / `ArousalFactors` type — never referenced
- `Reflex` struct — superseded by YAML-based reflex engine

**Direction:** Prune unused fields and methods. If any are planned features, add `// TODO` comments; otherwise delete.

---

### 1.15 Dead code in `state.Inspector`

**File:** `internal/state/inspect.go`

- `StateSummary.Traces.Total` is never populated (always 0), making the health check `> 1000` useless
- Inspector reads/writes JSON directly (see 3.1), bypassing pool encapsulation

---

### 1.16 GTD validation never called in production

**File:** `internal/gtd/validation.go`

`ValidateTask()` and `ValidateProject()` are only called in tests. MCP tool handlers create/update tasks without calling validation, allowing invalid state.

**Direction:** Wire validation into MCP tool handlers.

---

### 1.17 Test-only code not flagged as such

- `internal/effectors/test.go:TestEffector.ClearOutput()` — exported, never called outside its own file
- `TestEffector` lacks `StartTyping`/`StopTyping`/`StopAllTyping` methods, forcing type assertions in callers

---

### 1.18 `cmd/test-synthetic/` — References obsolete SQLite memory

**File:** `cmd/test-synthetic/main.go`

Queries a `memory.db` SQLite database for traces/episodes/entities, but the architecture moved to Engram (HTTP service). This harness is outdated.

---

### 1.19 Root-level binaries committed

The `bud` and `bud2` binaries in the repo root are compiled artifacts that should be in `.gitignore`.

---

### 1.20 `gkRequired()` — Trivial identity wrapper

**File:** `internal/mcp/tools/gk.go:16`

```go
func gkRequired(fields ...string) []string { return fields }
```

Adds no value over `[]string{"entities"}`. Misleading — reader expects validation.

**Direction:** Replace calls with direct slice literals.

---

## 2. Overlapping / Duplicate Components

### 2.1 Two parallel memory systems: `memory.*Pool` vs. `engram.Client`

The `TracePool`, `PerceptPool`, `ThreadPool` implement local in-memory trace management with activation decay, spread, reinforcement, persistence. `engram.Client` provides the same capabilities (and more) via HTTP. They share the concept of "traces" but use different types (`types.Trace` vs. `engram.Trace`). **Only Engram is used in production.** The local pools are fully orphaned but still present.

**Direction:** Delete local pools (see 1.1).

---

### 2.2 Two parallel thread/percept systems: `memory.*Pool` vs. `state.Inspector`

`state.Inspector` has its own `loadThreads()`/`saveThreads()` and `loadPercepts()`/`savePercepts()` that read/write `system/threads.json` and `system/queues/percepts.json` directly. `memory.ThreadPool` and `memory.PerceptPool` also Load/Save these same files. If both were active, they'd cause data races.

**Direction:** Since the pools are dead, update Inspector to be the sole owner. Remove pool persistence code.

---

### 2.3 Duplicate JSONL handling: `activity.Log` vs. `state.Inspector`

`activity.Log` reads JSONL via `readAll()`. `state.Inspector` reimplements JSONL reading/writing with `countJSONL`, `tailJSONL`, `truncateJSONL`. Both read `activity.jsonl` via different code paths, and Inspector does so *without* the mutex that `activity.Log` uses, creating a potential data race.

**Direction:** Extract shared JSONL utilities into a `jsonl` package. Make `state.Inspector` call `activity.Log` methods instead of reimplementing them.

---

### 2.4 Three `truncate` implementations

| Location | Function |
|---|---|
| `internal/executive/types.go:97` | `truncate(s string, maxLen int)` |
| `internal/executive/simple_session.go:1255` | `truncatePrompt(s string, maxLen int)` |
| `internal/senses/discord.go:266` | `truncate(s string, maxLen int)` |

All identical logic (or near-identical). `truncatePrompt` adds newline→space replacement.

**Direction:** Extract to `internal/textutil/truncate.go`.

---

### 2.5 Dual `processItem` paths: Provider vs. SDK

**File:** `internal/executive/executive_v2.go:774-1120`

The `processItem` method has a massive `if e.providerSession != nil` branch that duplicates: user response tracking, typing callbacks, session start/end notifications, debug event emission, MCP tool call tracking, and fallback message sending. Two nearly identical code paths.

**Direction:** Unify under `provider.Session` interface.

---

### 2.6 `activity_recent` vs. `journal_recent` / `activity_today` vs. `journal_today` MCP tools

**File:** `internal/mcp/tools/register.go`

Both pairs call the same `deps.ActivityLog.Recent(count)` / `deps.ActivityLog.Today()`. The only difference is `journal_log` allows filtering by semantic type (decision/impulse/etc.) vs `activity_by_type` filtering by internal event type. Confusing for users.

**Direction:** Consolidate into a single set of tools with clear parameter-based filtering.

---

### 2.7 `list_traces` vs. `state_traces list` MCP tools

**File:** `internal/mcp/tools/register.go`

Both call `deps.EngramClient.ListTraces()`. `state_traces` with `action="list"` returns more data. Redundant from a user perspective.

**Direction:** Remove `list_traces` or make `state_traces` the sole interface.

---

### 2.8 `formatEventsCompact()` vs. `Event.FormatEventSummary()` in Calendar

**File:** `internal/mcp/tools/register.go:2469-2478` vs. `internal/integrations/calendar/client.go`

Two formatting paths for calendar events that produce similar but inconsistent output.

**Direction:** Consolidate on one format function.

---

### 2.9 Two `ToPercept()` methods

| Location | Method | Status |
|---|---|---|
| `memory/inbox.go:55` | `InboxMessage.ToPercept()` | **Active** — called in `main.go` |
| `types/types.go:185` | `Impulse.ToPercept()` | **Dead** — never called |

These produce structurally similar but not identical `Percept` objects (different ID format, extra fields).

**Direction:** Delete `Impulse.ToPercept()`.

---

### 2.10 Two session ID generation mechanisms

- `generateSessionUUID()` in `simple_session.go:629` — UUID v4
- `generateSessionID()` in `opencode_serve.go:807` — `bud-oc-{timestamp}`
- Temp UUID + Claude SDK ID in `subagent_session.go:305`

**Direction:** Unify on one strategy with naming conventions for log identification.

---

### 2.11 tmux vs. zellij implementations

`internal/tmux/` and `internal/zellij/` provide nearly identical functionality (`OpenExecWindow`, `OpenSubagentWindow`, `StartCleanupLoop`). Only zellij is used. The import `tmuxwindow "github.com/vthunder/bud2/internal/zellij"` in main.go makes the alias misleading.

Zellij's `CloseOldPanes()` and `StartCleanupLoop()` are **no-ops** (return 0 / empty), meaning old pane cleanup is broken.

**Direction:** Delete `internal/tmux/`. Fix or document no-op zellij methods.

---

### 2.12 Effector interface duplication

`DiscordEffector` and `TestEffector` share the same method signatures (`Submit`, `Start`, `Stop`, `SetOnSend`, `SetOnAction`, `SetOnError`, `SetOnRetry`, `SetPendingInteractionCallback`, `SetMaxRetryDuration`) but no common interface. `main.go` uses concrete types, forcing type-specific branching. `TestEffector` also lacks `StartTyping`/`StopTyping`/`StopAllTyping`.

**Direction:** Define an `Effector` interface.

---

## 3. Poor Fit / Architectural Mismatches

### 3.1 `state.Inspector` undermines encapsulation

**File:** `internal/state/inspect.go`

Inspector directly reads/writes JSON files that logically belong to other packages (`percepts.json` → `PerceptPool`, `threads.json` → `ThreadPool`, `activity.jsonl` → `activity.Log`). It has its own `loadThreads`/`saveThreads`/`loadPercepts`/`savePercepts`/`tailJSONL`/`truncateJSONL` that duplicate the persistence logic of the original packages. Additionally:
- Inspector's `Health()` reports `Traces.Total` as always 0 (never populated)
- Inspector reads `activity.jsonl` without `activity.Log`'s mutex, creating a data race risk

**Direction:** Each data format should have exactly one owner package. Inspector should delegate to those packages' APIs.

---

### 3.2 `ExecutiveV2` is a 1986-line god object

**File:** `internal/executive/executive_v2.go`

This single struct handles: session management, context building, subagent lifecycle, MCP tool callbacks, debug event broadcasting, memory evaluation extraction, prompt construction, user response validation + fallback, background session management, and plugin loading. The `cmd/bud/main.go` is also 1753 lines.

**Direction:** Decompose into `ContextBuilder`, `PromptBuilder`, `SubagentManager`, etc. Extract `main.go` into `app.Setup()`, `app.CreateDeps()`, `app.Run()`.

---

### 3.3 `register.go` is a 2,479-line god file

**File:** `internal/mcp/tools/register.go`

Registers ~50 tools across 8+ domains (communication, memory, activity, GTD, reflex, calendar, GitHub, subagents, eval, projects, VM/browser) in a single file. Some domains are already extracted (`gk.go`, `resource.go`, `vm_browser.go`, `image_gen.go`) but most remain crammed in.

**Direction:** Split into domain-specific files: `register_comm.go`, `register_memory.go`, `register_activity.go`, etc.

---

### 3.4 `Dependencies` struct is a 99-field god object

**File:** `internal/mcp/tools/deps.go`

~20 callback function fields, 6 service pointers, and numerous path/URL strings. Tool handlers directly type-assert `map[string]any` args and reach deeply into `deps`. Makes testing individual tools very hard.

**Direction:** Group related deps into sub-structures (`CommunicationDeps`, `MemoryDeps`, etc.) for clearer ownership and simpler construction.

---

### 3.5 `SubagentCallbacks()` returns a 12-tuple of function values

**File:** `internal/executive/executive_v2.go:226-236`

Any change to the subagent API requires updating all callers to match positional arguments. Extremely fragile.

**Direction:** Replace with a `SubagentOps` struct.

---

### 3.6 Attention system is architecturally mismatched

`Attention` was designed as a cognitive model with arousal, threshold gating, and mode-based focus. In production, the executive only uses it as a **suspended-item stack** (`Focus()`/`Complete()`). All salience computation, arousal adjustment, threshold selection, and mode management are dead code paths. The `Queue` does all the real dispatch work via priority-based pop.

**Direction:** Simplify `Attention` to just a suspended-item stack, or replace both `Attention` and `Queue` with a unified `FocusManager`.

---

### 3.7 Reflex engine couples action definitions with engine construction

**File:** `internal/reflex/engine.go`

`NewEngine()` immediately calls `createGTDActions()`, `createCallToolAction()`, `createGTDThingsActions()`, `createInvokeReflexAction()`, and `createJSONQueryAction()` — ~400 lines of inline closures. Action registration should be pluggable rather than hardcoded in the constructor.

**Direction:** Use a registry pattern where actions are registered externally or loaded from config.

---

### 3.8 `senses.discord.classifyDialogueAct` is misplaced

**File:** `internal/senses/discord.go:202`

Dialogue act classification belongs in the reflex engine or an NLP utility, not embedded in the Discord sense layer.

**Direction:** Move to `internal/reflex/` or `internal/nlp/`.

---

### 3.9 `memory.InboxMessage` type lives in the wrong package

**File:** `internal/memory/inbox.go`

`InboxMessage` is a messaging/notification type used by `senses/discord.go` and `senses/calendar.go` — a conduit between the senses layer and processing pipeline. It's not a memory management type.

**Direction:** Move to a `messaging` or `pipeline` package.

---

### 3.10 `engram.Trace` vs. `types.Trace` — Name collision, different schemas

- `types.Trace`: `Content`, `Embedding`, `Activation`, `Strength`, `Sources`, `IsCore`, `CreatedAt`, `LastAccess`, `Inhibits`, `LabileUntil`
- `engram.Trace`: `ID`, `Summary`, `Level`, `EventTime`, `SchemaIDs`

No adapter or mapping between them. They represent different stages of the memory pipeline.

**Direction:** Define a single authoritative Trace type. If `types.Trace` is needed for local processing, have it populated from `engram.Trace` responses. Remove dead fields like `Inhibits`, `LabileUntil`.

---

### 3.11 `image_gen.go` embeds raw API logic in tool handler

**File:** `internal/mcp/tools/register.go:33-37`

Reads `REPLICATE_API_TOKEN` from env inside the handler and makes raw HTTP calls. Should follow the pattern of `calendar/client.go` and `github/client.go` (proper integration client).

**Direction:** Extract to `internal/integrations/replicate/client.go`.

---

### 3.12 `vm_browser.go` has hardcoded paths

**File:** `internal/mcp/tools/vm_browser.go:21-22`

```go
const vmControlScript = "/Users/thunder/src/bud2/state/projects/sandmill/vm-control-server.js"
```

Absolute, user-specific path. Non-portable.

**Direction:** Derive from config/env var.

---

### 3.13 `memory_reset` tool blocks for 3 seconds

**File:** `internal/mcp/tools/register.go:791`

```go
time.Sleep(3 * time.Second)
```

Blocks the MCP handler goroutine for 3 seconds. On stdio transport, this blocks the entire server from processing any other requests.

**Direction:** Use a goroutine or callback-based approach.

---

### 3.14 `trigger_bud_redeploy` tool is fragile

**File:** `internal/mcp/tools/register.go:834-842`

Spawns `nohup bash deploy.sh` with no cancellation, no output capture, and no health check. If the deploy script fails, the tool still returns success.

**Direction:** Capture output, check exit status, integrate with a health check.

---

### 3.15 `Percept` struct overloading

`Percept` serves as both an external input container (from Discord/calendar) and an internal impulse converter, with many fields (`RawInput`, `ProcessedBy`, `DialogueAct`, `EntropyScore`, `Features`, `Embedding`) that are declared but never populated for either use case.

**Direction:** Simplify `Percept` to the fields actually used, or split into `ExternalPercept` and `InternalPercept`.

---

### 3.16 Sidecar Python process with zero Go integration

**Directory:** `sidecar/`

The sidecar is a Python FastAPI/spaCy service with deployment config (`deploy/ner-sidecar.service.example`), but no Go code in the main binary calls it. NER is handled by Engram. The sidecar appears vestigial.

**Direction:** Remove if truly unused, or document its intended purpose.

---

### 3.17 `gtd.Store` interface with single implementation

**File:** `internal/gtd/types.go:52-70`

The `Store` interface exists but only `*GTDStore` implements it. No mock exists, no second backend is provided. The interface adds indirection without benefit.

**Direction:** Either remove the interface and use `*GTDStore` directly, or create a mock for testing MCP tools.

---

### 3.18 Plugin loading performance issue

**File:** `internal/executive/simple_session.go`

`generateZettelLibraries()` (line 1028) runs on every `SendPromptWithCfg()` call — reads every plugin's `plugin.json`, opens files, and rewrites a YAML file. Should be called once at startup or on plugin change only.

Similarly, `cachedPlugins()` (line 618) caches but the cache is never invalidated.

---

## 4. Design Improvement Opportunities

### 4.1 Extract `internal/textutil` package

Consolidate the three `truncate` implementations and `estimateTokens` into a shared utility package.

---

### 4.2 Replace `SubagentCallbacks()` 12-tuple with a struct

```go
type SubagentOps struct {
    Spawn       func(...) (id, logPath string, err error)
    List        func() []map[string]any
    Answer      func(sessionID, answer string) error
    // ... etc
}
```

---

### 4.3 Unify `processItem`'s dual path

The provider-vs-SDK branch at `executive_v2.go:965` should be a single code path through `provider.Session`. `SimpleSession` already implements `provider.Session` — always use `SendPrompt` and move callback wiring into `StreamCallbacks`.

---

### 4.4 Define an `Effector` interface

```go
type Effector interface {
    Submit(action *types.Action)
    Start()
    Stop()
    SetOnSend(func(channelID, content string))
    SetOnAction(func(actionType, channelID, content, source string))
    SetOnError(func(actionID, actionType, errMsg string))
    SetOnRetry(func(actionID, actionType, errMsg string, attempt int, nextRetry time.Duration))
    SetPendingInteractionCallback(func(channelID string) *PendingInteraction)
    SetMaxRetryDuration(d time.Duration)
    StartTyping(channelID string)
    StopTyping(channelID string)
    StopAllTyping()
}
```

This eliminates type-specific branching in `main.go`.

---

### 4.5 Split `register.go` into domain files

```
register.go          — RegisterAll() only
register_comm.go     — communication tools
register_memory.go   — memory tools
register_activity.go — activity/journal tools
register_state.go    — state management tools
register_gtd.go      — GTD tools
register_calendar.go — calendar tools
register_github.go   — GitHub tools
register_subagent.go — subagent tools
```

Each file ~100-200 lines instead of 2,479.

---

### 4.6 Extract context building from `ExecutiveV2`

`buildContext()` (lines 1123-1299), `buildRecentConversation()` (lines 1310-1488), and `buildRecentConversationForWake()` are prime candidates for a `ContextBuilder` struct.

---

### 4.7 Fix `Inspector.Health()` completeness

- Populate `Traces.Total` from actual data
- Make thresholds configurable
- Remove hardcoded `Inbox: 0` with "Inbox is now in-memory only" — reflect reality in the struct

---

### 4.8 Make reflex action registration pluggable

Instead of hardcoding GTD/calendar/shell actions in `NewEngine()`, use a registry pattern where actions are registered externally or loaded from config.

---

### 4.9 The `reflex.Log` aging mechanism is brittle

**File:** `internal/reflex/log.go`

Uses `lastSent` index tracking that becomes invalid when entries are trimmed. Consider using a monotonic sequence counter instead.

---

### 4.10 Extract JSONL utilities

Both `activity.Log` and `state.Inspector` implement JSONL reading, counting, tailing, and truncating. A `jsonl` package with `Read`, `Count`, `Tail`, `Truncate` would eliminate ~100 lines of duplicate code.

---

### 4.11 `activity.Log.readAll()` reads entire file on every query

`Recent()`, `Today()`, `Search()`, `ByType()`, `Range()`, and `LastUserInputTime()` all call `readAll()` which reads the entire JSONL file. For a long-running system, this could be very slow.

**Direction:** Add an indexed approach or size-bounded sliding window.

---

### 4.12 Consolidate duplicate activity/journal MCP tools

Either remove `journal_recent`/`journal_today` and keep only `activity_recent`/`activity_today`, or differentiate them clearly (e.g., `journal_log` adds type-tagged entries while `activity_*` reads raw entries — the read-path tools should not be duplicated).

---

### 4.13 Wire GTD validation

Call `ValidateTask()` / `ValidateProject()` in MCP tool handlers before `AddTask()` / `UpdateTask()`. Currently at `register.go:1067` tasks are added without validation.

---

### 4.14 Clean up zellij stub implementations

`CloseOldPanes()` and `StartCleanupLoop()` are no-ops. Either implement zellij pane cleanup or document that auto-cleanup is not supported.

---

### 4.15 Consolidate logging

`log.Printf` (stdlib), `logging.Info()`/`logging.Debug()` (custom), and `profiling.Profiler` (disabled) create inconsistent formatting. Standardize on `logging.*` calls.

---

### 4.16 `Deps` struct coupling

Group related dependencies into sub-structures:

```go
type CommunicationDeps struct {
    SendMessage    func(channelID, message string) error
    AddReaction    func(channelID, messageID, emoji string) error
    SendFile       func(channelID, filePath, message string) error
    DefaultChannel string
}
```

Makes it clearer which tools need which deps and simplifies testing.

---

### 4.17 Remove the unused `ctx any` parameter from `ToolHandler`

Since `ctx` is never used by any handler, the signature should be `func(args map[string]any) (string, error)`. The `Server.context` field and `SetContext` method can also be removed.

---

## Summary Statistics

| Category | Count |
|---|---|
| Dead/unused components | 20 |
| Overlapping/duplicate components | 12 |
| Architectural mismatches | 18 |
| Design improvement opportunities | 17 |

### Largest dead code removal opportunities (by line count):

| Component | Estimated Lines |
|---|---|
| `internal/memory/` pool types (4 files) | ~825 |
| `internal/executive/custom_session.go` | ~300 |
| `internal/budget/signals.go` | ~120 |
| `internal/tmux/windows.go` | ~108 |
| `internal/embedding/` generation code | ~100 |
| `internal/focus/attention.go` salience/arousal code | ~80 |
| `types.go` unused types and methods | ~60 |
| Calendar/GitHub dead methods | ~80 |
| **Total removable** | **~1,673** |

### Highest-impact restructuring opportunities:

1. **Split `register.go`** (2,479 lines) into domain files
2. **Decompose `ExecutiveV2`** (1,986 lines) into focused components
3. **Refactor `main.go`** (1,753 lines) into `app.*` packages
4. **Unify `processItem` dual path** to eliminate ~350 lines of duplicated logic
5. **Define `Effector` interface** to eliminate type-specific branching