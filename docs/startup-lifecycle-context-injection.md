---
topic: Startup Lifecycle & Context Injection
repo: bud2
generated_at: 2026-04-12T00:00:00Z
commit: 82261e9
key_modules: [cmd/bud, internal/executive, seed/startup-instructions.md]
score: 0.85
---

# Startup Lifecycle & Context Injection

> Repo: `bud2` | Generated: 2026-04-12 | Commit: 82261e9

## Summary

Bud's startup lifecycle covers everything from process start through the first Claude session
that runs post-deploy housekeeping. The system distinguishes startup from ordinary wakes by
injecting a different instruction file (`seed/startup-instructions.md` instead of
`seed/wakeup.md`), disabling memory retrieval, and surfacing handoff notes written by the
previous session. This design keeps cold-start prompts lean and ensures Claude's first action
is to re-spawn any subagents that were interrupted by the redeploy.

## Key Data Structures

### `ExecutiveV2Config` (`internal/executive/executive_v2.go`)

The configuration record passed to `NewExecutiveV2`. Two fields are startup-critical:

- `WakeupInstructions` — content of `seed/wakeup.md`, injected into every autonomous wake prompt.
- `StartupInstructions` — content of `seed/startup-instructions.md`, injected only into the startup impulse prompt.

Both are loaded by `main.go` at boot and passed verbatim into the executive. A blank `StartupInstructions` suppresses startup context entirely (`buildPrompt` checks for non-empty before injecting).

### `execSessionDiskFormat` (`internal/executive/simple_session.go`)

```go
type execSessionDiskFormat struct {
    ClaudeSessionID string    `json:"claude_session_id"`
    SavedAt         time.Time `json:"saved_at"`
    CacheReadTokens int       `json:"cache_read_tokens"`
}
```

Written to `state/system/exec_session.json` after every successful prompt via `SaveSessionToDisk`. Read back by `LoadSessionFromDisk` at startup. If the saved session is still valid, the first user interaction after restart resumes the Claude session (same conversation) instead of starting fresh — but the startup impulse itself always runs in a new session.

### `SimpleSession` (`internal/executive/simple_session.go`)

The per-session state manager. Startup-relevant fields:

- `claudeSessionID` — Claude-assigned session ID, persisted across Bud restarts.
- `isResuming` — set by `PrepareForResume()`; tells `buildPrompt` to skip static context (core identity, conversation buffer) already in Claude's session history.
- `seenMemoryIDs` — memories already injected; persists across resume turns but cleared on full `Reset()`.
- `lastBufferSync` — timestamp after which new conversation episodes are included; cleared on reset.

### `focus.PendingItem` (startup impulse)

The startup impulse is a focus item with `Type = "impulse:startup"` injected by `main.go` after the executive is initialized. Its `Data` map may contain `"handoff"` — the content of any handoff notes left by previous autonomous sessions (from `seed/autonomous-handoff.md`).

## Lifecycle

1. **Process start & environment load** (`cmd/bud/main.go:main`): `.env` is loaded via `godotenv`; config comes from environment variables (`DISCORD_TOKEN`, `MCP_HTTP_PORT` defaulting to 8066, `AUTONOMOUS_INTERVAL`, `AUTONOMOUS_SESSION_CAP` defaulting to 8 minutes, `DAILY_TOKEN_BUDGET` defaulting to 1M tokens, `USER_TIMEZONE`).

2. **PID guard** (`checkPidFile`): Bud writes its PID to `state/system/bud.pid`. If an existing PID file names a running process with a matching name, Bud prompts the user (interactive) or auto-kills (service mode, `BUD_SERVICE=1`). This prevents duplicate daemons from sharing state.

3. **Directory seeding** (`seedSystemDir`): Each subdirectory under `seed/` (guides, jobs, plugins, reflexes, workflows) is copied to `state/system/` if the destination does not exist. This is a one-way, first-boot-only operation — existing state is never overwritten.

4. **MCP config generation** (`writeMCPConfig`): Writes `state/.mcp.json` with the HTTP MCP server URL (`http://127.0.0.1:<port>/mcp`). Claude sessions launched from `state/` pick this up automatically so MCP tools are available without manual configuration. Critically, the MCP HTTP server is **bound synchronously** before the P1 handler goroutine starts — this ensures the port is open before the first focus item could trigger a Claude session.

5. **Session ID recovery** (`LoadSessionFromDisk`): `SimpleSession.LoadSessionFromDisk()` reads `state/system/exec_session.json`. If the file exists and the stored `ClaudeSessionID` is non-empty, it sets `claudeSessionID` so the next user-initiated session can resume the previous conversation thread. The startup impulse itself does NOT resume — it always starts a new session.

6. **Plugin manifest loading** (`loadManifestPlugins` + `loadManifestSkills`): `state/system/extensions.yaml` is parsed (falls back to `plugins.yaml` with a deprecation warning if the new file is absent). The manifest has two top-level sections: `plugins:` for full plugin repos (agents + skills + tool grants) and `skills:` for standalone skill-only packages from ClaWHub (`clawhub:slug[@version]`), GitHub (`git:owner/repo[:dir][@ref]`), or local paths (`path:/local/path`). Remote plugin repos are cloned into `~/Library/Caches/bud/plugins/` (`os.UserCacheDir()/bud/plugins/`) on first run and fast-forward-pulled on subsequent starts; ClaWHub skills land under `~/Library/Caches/bud/skills-clawhub/`. Plugin entries support explicit `git:` / `path:` prefixes; bare `owner/repo` still works with a deprecation warning. Plugin directories discovered this way are passed to Claude sessions as `--plugin-dir` so agents and skills defined in external repos are available without symlinks. The `exclude:` list on each manifest entry skips named sub-plugin subdirectories during both agent-def loading and MCP tool registration.

7. **MCP tool registration** (`tools.RegisterAll`): All MCP tools are registered with the HTTP server. After registration, `exec.SetKnownMCPTools(knownTools)` is called — this lets wildcard patterns in `tool_grants` (e.g. `mcp__bud2__gk_*`) expand correctly against the live tool list.

8. **Startup impulse injection** (`main.go`, after `exec.Start()`): A 3-second sleep lets the executive initialize; then `main.go` calls `exec.AddPending(&focus.PendingItem{Type: "impulse:startup", Priority: 3})` with any autonomous handoff notes read from `readAutonomousHandoff(statePath)` attached to the item's `Data["handoff"]`.

9. **Startup prompt assembly** (`ExecutiveV2.buildPrompt`): The executive dequeues the startup impulse. Since `claudeSessionID` is empty at this point (or the startup item bypasses resume), `PrepareNewSession()` is called — setting a fresh session UUID, clearing `seenItemIDs`, `lastUsage`, `lastBufferSync`. `buildContext` is called:
   - Memory retrieval is **skipped** for startup impulses (no Engram queries). This avoids loading stale or irrelevant memories when Claude's first action should be deterministic housekeeping.
   - Conversation buffer retrieval (`buildRecentConversation`) is also skipped for startup.
   - `buildPrompt` detects the startup type and injects `StartupInstructions` (from `seed/startup-instructions.md`) in place of wakeup checklist content.
   - If `Data["handoff"]` is non-empty, the handoff note appears as a `## Previous Session Note` subsection.

10. **Startup housekeeping session**: Claude receives the startup prompt with instructions to (a) check `system/subagent-restart-notes.md` for interrupted subagents and re-spawn them, (b) review the handoff note for urgent follow-ups, and (c) call `signal_done`. The session is capped by `MaxAutonomousSessionDuration` (default 8 minutes) like any other wake.

11. **`signal_done` and memory eval**: When Claude calls `signal_done`, `ExecutiveV2.SignalDone()` cancels the session context. The executive extracts any `<memory_eval>` block from the output and calls `RateEngrams()` to feed quality ratings back to Engram — even for startup sessions.

## Design Decisions

- **Startup uses a separate instruction file, not wakeup.md**: The wakeup checklist instructs Claude to review subagent status, check Things tasks, and potentially spawn new work. Startup's job is narrower: re-spawn interrupted subagents and signal done. A separate file prevents startup from accidentally triggering autonomous work sessions immediately after deploy.

- **Memory retrieval disabled at startup**: The overview notes that 48% of wake-context memories were rated 1/5 in quality analysis. Startup prompts are even more generic, making memory retrieval even less useful. The decision was to skip it entirely — Claude doesn't need recalled memories to check `subagent-restart-notes.md` and call `signal_done`.

- **Startup impulse is P3 (background priority)**: Priority 3 is `P3ActiveWork` — the same as autonomous wakes. This means a user message arriving during the 3-second startup delay will be queued as P1 and pre-empt the startup impulse. The startup runs after the first user interaction, not before it.

- **Session ID persisted across restarts, but startup always starts fresh**: The `claudeSessionID` is saved to disk so user-initiated sessions can resume the previous conversation. However, the startup impulse itself uses a new session (no `--resume`). This avoids the startup prompt landing inside an ongoing conversation thread from the previous daemon instance.

- **Plugin manifest loading is blocking at startup**: `loadManifestPlugins` performs `git clone` / `git pull` synchronously before the executive starts. This ensures the plugin tool set is complete before the first Claude session runs. Failures are non-fatal: the entry is logged and skipped.

- **MCP server binds before P1 handler starts**: If the port were bound asynchronously, a rapidly-delivered user message could cause Claude to connect before the server is listening. The synchronous bind eliminates this race at the cost of a brief startup delay.

## Integration Points

| From | To | What crosses the boundary |
|------|----|--------------------------|
| `cmd/bud/main.go` | `internal/executive` | `NewExecutiveV2(cfg)` — passes `StartupInstructions`, `WakeupInstructions`, all callbacks; calls `exec.AddPending()` to inject startup impulse |
| `cmd/bud/main.go` | `internal/mcp/server` | Registers all MCP tools via `tools.RegisterAll`; binds HTTP server; wires Discord send/react callbacks |
| `internal/executive/executive_v2.go` | `internal/executive/simple_session.go` | `LoadSessionFromDisk()` at startup; `PrepareNewSession()` / `PrepareForResume()` per wake; `SendPrompt()` to run Claude |
| `internal/executive/simple_session.go` | `state/system/exec_session.json` | Reads/writes persisted `claudeSessionID` across Bud restarts |
| `cmd/bud/main.go` | `seed/startup-instructions.md` | File is read at boot and stored in `ExecutiveV2Config.StartupInstructions` |
| `cmd/bud/main.go` | `state/system/autonomous-handoff.md` | `readAutonomousHandoff()` reads and truncates this file; contents become `Data["handoff"]` on the startup impulse |
| `internal/executive/executive_v2.go` | `internal/engram/client.go` | Startup skips memory retrieval; post-session, `RateEngrams()` sends memory quality ratings regardless of session type |

## Non-Obvious Behaviors

- **The 3-second startup sleep is load-bearing**: `main.go` sleeps 3 seconds after initializing the executive before injecting the startup impulse. This allows the executive's goroutines (P1 handler, background ticker, subagent watchers) to fully start. Removing it can cause the startup impulse to be queued before the executive is ready to process it.

- **`LoadSessionFromDisk` does not immediately resume**: It sets `claudeSessionID` so the *next user message* can resume. The startup impulse runs in a new session (the code calls `PrepareNewSession`, not `PrepareForResume`). An engineer expecting the startup session to continue the previous conversation will be surprised.

- **Handoff notes are truncated on read**: `readAutonomousHandoff` opens `autonomous-handoff.md` and calls `f.Truncate(0)` after reading. The file is consumed exactly once — the next startup will not see notes from the previous-previous session. This is intentional: stale notes from two restarts ago would be confusing.

- **Plugin agent definitions hot-reload on every prompt**: Even though plugins are cloned at startup, `loadAgentDefs()` re-reads the plugin directories on every `processItem` call. Changes to agent YAML files take effect without restarting Bud — only plugin manifest changes (adding/removing repos) require a restart.

- **Startup skips conversation buffer injection**: `buildRecentConversation` is not called for startup impulses. Claude starts with no conversation history beyond the startup instruction. This is why the instructions say "Previous Session Note" (from handoff) rather than showing the full conversation log.

- **`signal_done` in startup triggers memory eval even with no memories shown**: The `<memory_eval>` extraction happens unconditionally in `processItem`. For startup, where `seenMemoryIDs` is empty, this produces an empty eval map, and `RateEngrams()` is called with zero ratings — a no-op but not an error.

## Start Here

- `cmd/bud/main.go` — entry point; read the `main()` function top-to-bottom to see the exact initialization order, especially the `seedSystemDir` calls, `writeMCPConfig`, `loadManifestPlugins`, `loadManifestSkills`, and the startup impulse injection at the bottom.
- `internal/executive/extensions.go` — standalone skill loading: `loadManifestSkills`, `downloadClawhubSkill`, `cloneOrUpdateGitSkillEntry`, `resolvedManifestSkillDirs`; read alongside `simple_session.go` for full plugin loading picture.
- `seed/startup-instructions.md` — the instruction file injected into startup prompts; defines what Claude does on cold start (subagent re-spawn + signal_done).
- `internal/executive/executive_v2.go:buildPrompt` — where startup vs. wake branching happens; look for the `impulse:startup` type check and how `StartupInstructions` is injected.
- `internal/executive/simple_session.go:LoadSessionFromDisk` / `SaveSessionToDisk` — the session persistence mechanism; shows what survives a Bud restart and what doesn't.
- `internal/executive/simple_session.go:PrepareNewSession` / `PrepareForResume` — the two session preparation paths; startup always calls `PrepareNewSession`, resume turns call `PrepareForResume`.
