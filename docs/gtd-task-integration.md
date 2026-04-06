---
topic: GTD & Task Integration
repo: bud2
generated_at: 2026-04-06T09:36:42Z
commit: cf0924af
key_modules: [internal/gtd, things-mcp]
score: 0.31
---

# GTD & Task Integration

> Repo: `bud2` | Generated: 2026-04-06 | Commit: cf0924af

## Summary

Bud maintains two parallel task systems: a local JSON-backed GTD store (`internal/gtd`) for tracking Bud's own commitments, and an embedded TypeScript MCP server (`things-mcp`) that bridges directly to the user's Things 3 app via AppleScript and URL scheme. The two systems are deliberately decoupled — `gtd_*` MCP tools operate on the local store, while `things_*` tools (served by the proxy) provide full access to the user's Things 3 database.

## Key Data Structures

### `Task` (`internal/gtd/types.go`)
The core task entity in the local GTD store. Key fields:
- `When string` — scheduling slot: `"inbox"`, `"today"`, `"anytime"`, `"someday"`, or `"YYYY-MM-DD"` (date string)
- `Status string` — `"open"`, `"completed"`, `"canceled"`
- `Repeat string` — recurrence pattern: `"daily"`, `"weekly"`, `"monthly"`, etc.
- `Project string` — foreign key to a `Project.ID`
- `Heading string` — optional heading within the project (must exist in `Project.Headings`)
- `Area string` — foreign key to an `Area.ID` (only if not in a project)
- `Order float64` — sort position; auto-assigned on `AddTask` via `generateID()` timestamp seed

Inbox tasks may not have a project, area, or heading — this is enforced by `ValidateTask`.

### `GTDStore` (`internal/gtd/store.go`)
The only live implementation of `gtd.Store`. Wraps a `StoreData` value (three slices: areas, projects, tasks) with a `sync.RWMutex`. Persists to `<statePath>/user_tasks.json`.

### `StoreData` (`internal/gtd/types.go`)
The full JSON-serializable snapshot: `Areas []Area`, `Projects []Project`, `Tasks []Task`. All reads and writes go through `GTDStore` which holds this in memory and flushes on each mutation.

### `Store` interface (`internal/gtd/types.go`)
Defines the full storage contract: load/save, CRUD for areas/projects/tasks, `CompleteTask`, and `FindTaskByTitle`. Only `GTDStore` implements it; the `ThingsStore` backend referenced in the older `things-integration.md` doc has been removed.

### `ThingsTodo` / `ThingsProject` (`things-mcp/src/types/things.ts`)
TypeScript types for Things 3 data returned by the `things_*` MCP tools. `ThingsTodoDetails` extends `ThingsTodo` with `deadline`, `scheduledDate`, `notes`, `status`, `project`, and date fields.

## Lifecycle

1. **Daemon startup** (`cmd/bud/main.go:299`): `gtd.NewGTDStore(statePath)` constructs the store with path `<statePath>/user_tasks.json`. `gtdStore.Load()` reads the file; if missing, starts empty.

2. **GTD store injected into reflex engine** (`cmd/bud/main.go:322`): `reflexEngine.SetGTDStore(gtdStore)` makes the store available to reflex pipeline actions (`gtd_list`, `gtd_add`, `gtd_complete`, `gtd_dispatch`).

3. **GTD MCP tools registered** (`internal/mcp/tools/register.go:45`): If `deps.GTDStore != nil`, `registerGTDTools` installs `gtd_add`, `gtd_list`, `gtd_complete`, `gtd_update`, `gtd_areas`, and `gtd_projects` on the MCP server at port 8066. These tools read/write the local JSON store directly.

4. **things-mcp started as stdio proxy** (`cmd/bud/main.go:533`): At startup, `mcp.StartProxiesFromConfig` reads `state/.mcp.json`, which declares a `"things"` server entry pointing to `things-mcp/dist/index.js` with `THINGS_AUTH_TOKEN` in the environment. The proxy launches the Node.js process and re-registers all its tools on the main MCP server, making `things_*` tools available to Claude sessions alongside the `gtd_*` tools.

5. **things-mcp tool call** (`things-mcp/src/index.ts`): Claude calls a `things_*` tool → the MCP server proxy forwards it to the Node.js process → `ToolRegistry.executeHandler` routes to the appropriate `AbstractToolHandler` (add, get, show, or update-json) → handlers either run AppleScript files (reads) via `executeAppleScriptFile` or write via the `things:///` URL scheme.

6. **Reflex fast-path** (`internal/reflex/engine.go:1237`): The `gtd_dispatch_things` action handles recognized intents (`gtd_show_today`, `gtd_show_inbox`, `gtd_add_inbox`) by calling `things_*` tools via the `toolCaller` bridge, bypassing Claude entirely for simple queries.

7. **Completing a repeating task** (`internal/gtd/store.go:380`): `CompleteTask` sets the task's `Status` to `"completed"` and, if `Repeat` is set, calls `createNextOccurrence` to generate a new task. If `When` is a `YYYY-MM-DD` date, `calculateNextDate` advances it by the repeat period; otherwise the same `When` slot is reused.

## Design Decisions

- **Two-store separation**: `gtd_*` tools track Bud's commitments in a local JSON file (`user_tasks.json`), while `things_*` tools operate directly on the user's Things 3 database. This keeps Bud's internal task state independent of the user's personal task manager, avoiding pollution of Things with agent internals.

- **things-mcp as git submodule**: `things-mcp/` is a forked submodule (originally `hildersantos/things-mcp`, published as `vthunder/things-mcp`). Upstream changes can be pulled in; local patches (e.g., the 5-minute tool call timeout, non-blocking startup) live on the fork.

- **Reads via AppleScript, writes via URL scheme**: The things-mcp server reads Things data via AppleScript (full query power, returns structured data) but writes via the `things:///` URL scheme (official write API, respects CloudKit sync, prevents corruption). Writes are fire-and-forget — success cannot be verified synchronously.

- **AppleScript argument sanitization** (`things-mcp/src/lib/validation.ts`): Arguments passed to AppleScript files are validated and sanitized (`validateAppleScriptArg` allows only `[a-zA-Z0-9 \-_.@]`) to prevent AppleScript injection. No string interpolation is used in scripts — all data passes as positional `argv`.

- **Things headings are flat siblings**: The `things-mcp` JSON API represents headings and todos as a flat array — headings are visual dividers, not containers. Todos that follow a heading appear under it in the UI. `buildProjectItems` in `json-builder.ts` enforces this flat structure.

- **`gtd.Store` interface preserved but only one implementation**: The interface suggests the JSON store was intended to be swappable (for a Things backend). The Things store was removed; only `GTDStore` (JSON) remains. The interface is still useful as a seam for testing.

## Integration Points

| From | To | What crosses the boundary |
|------|----|--------------------------|
| `cmd/bud` | `internal/gtd` | `gtdStore` injected into MCP deps and reflex engine on startup |
| `internal/mcp/tools` | `internal/gtd` | Tool handlers call `GTDStore` methods directly to read/write tasks |
| `cmd/bud` | `things-mcp` | `mcp.StartProxiesFromConfig` launches the Node.js process and proxies its tools |
| `internal/reflex` | `things-mcp` | `toolCaller.Call("things_*", ...)` in `gtd_dispatch_things` action |
| `things-mcp` | Things 3 app | AppleScript (reads via `osascript`) + `things:///` URL scheme (writes via `open`) |
| `internal/executive` | `internal/gtd` + `things-mcp` | Claude calls `gtd_*` and `things_*` tools via MCP HTTP at port 8066 |

## Non-Obvious Behaviors

- **`gtd_*` tools are for Bud's tasks, not the user's Things**: The local JSON store is used by Bud to manage its own commitments (things Bud is working on). The user's personal tasks live in Things 3 and are accessed via `things_*` tools. A new engineer might expect a single unified task system.

- **things-mcp is not auto-started if the proxy config is missing**: If `state/.mcp.json` doesn't exist or the `"things"` entry is absent, the `things_*` tools are simply not registered. No error is surfaced at startup. Reflexes using `gtd_dispatch_things` will fail at runtime with `"tool caller not configured"`.

- **`generateID()` is timestamp-seeded but monotonic**: `generateID` uses `time.Now().UnixNano()` as the base and an `int64` atomic counter (`idCounter`) as a suffix to ensure uniqueness within the same nanosecond. IDs look like timestamps but are not sortable by creation order alone across restarts.

- **`Order` field on tasks and projects is not recalculated on delete**: When a task is removed, the `Order` values of remaining tasks are not compacted. Gaps accumulate over time. Sorting by `Order` still works correctly; it just doesn't produce a dense sequence.

- **The `things_add_todo` reflex action targets the "Bud" list**: In `createGTDThingsActions`, `gtd_add_inbox` calls `things_add_todo` with `"list": "Bud"` — not the Things Inbox. Tasks added via reflex fast-path land in a dedicated "Bud" project/area, keeping them separate from personal inbox items.

- **Headings cannot be added to existing Things projects**: `ThingsJSONBuilder.addItemsToProject` explicitly skips heading items with a warning: _"Headings cannot be added to existing projects via JSON API — they can only be added during project creation."_ This is a Things API limitation.

## Start Here

- `internal/gtd/types.go` — `Task`, `Project`, `Area`, `StoreData`, and the `Store` interface; read this first to understand the data model
- `internal/gtd/store.go` — `GTDStore` implementation: `Load`/`Save`, `AddTask`, `CompleteTask`, `createNextOccurrence`
- `internal/mcp/tools/register.go:1021` — `registerGTDTools`: all six `gtd_*` MCP tool definitions with their parameter parsing and store calls
- `things-mcp/src/index.ts` — MCP server entry point: tool registration, stdio transport setup, startup sequence
- `internal/reflex/engine.go:1234` — `createGTDThingsActions`: how reflexes interact with Things 3 via the tool caller bridge
