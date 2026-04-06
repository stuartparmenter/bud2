---
topic: Seed Configuration & Plugin System
repo: bud2
generated_at: 2026-04-06T08:20:40Z
commit: d84689a1
key_modules: [cmd/bud, seed/]
score: 0.66
---

# Seed Configuration & Plugin System

> Repo: `bud2` | Generated: 2026-04-06 | Commit: d84689a1

## Summary

The seed system bootstraps Bud's live configuration from a set of versioned template files in `seed/`. On first run, seeds are copied once into `state/system/`; after that, the state directory is the live config and seeds are ignored. The plugin system extends this by providing namespaced agent definitions and Claude Code skills loaded from `state/system/plugins/`, with two distinct loading paths: one for interactive Claude Code CLI sessions and one for SDK-spawned background subagents.

## Key Data Structures

### `Agent` (`internal/executive/profiles.go`)
Parsed from YAML frontmatter + markdown body in agent files. Fields: `Name string`, `Description string`, `Model string`, `Skills []string`, `Tools []string`, `Body string`. The `Body` becomes the system prompt; `Skills` names are resolved to skill content and concatenated into the prompt.

### `AgentDefinition` (`github.com/severity1/claude-agent-sdk-go`)
The SDK type used to register agents. Fields: `Description string`, `Prompt string` (assembled from agent body + skill content), `Tools []string`, `Model AgentModel`. Returned by `LoadAllAgents()` and passed to Claude sessions via `WithAgents`.

### `AgentAliases` (`internal/executive/profiles.go`)
Loaded from `state/system/agent-aliases.yaml`. Has `Agents map[string]string` (legacy name → `namespace/agent`) and `Skills map[string]string` (alias → canonical skill name). Only skill aliases remain active; agent aliases are legacy.

### Plugin manifest (`seed/plugins/<ns>/.claude-plugin/plugin.json`)
Minimal JSON identifying a directory as a Claude Code plugin:
```json
{"name": "dev", "description": "...", "skills": "./skills/"}
```
Presence of this file is the gate for `scanLocalPlugins()`. Directories without it can still host agent definitions but are not surfaced to Claude Code as skill providers.

### `seedSystemDir(statePath, dirName string)` (`cmd/bud/main.go:1499`)
A one-shot copy function: if `state/system/<dirName>` does not exist, copies `seed/<dirName>/` to `state/system/<dirName>/` using `os.CopyFS`. Returns silently if the destination already exists or the source is missing.

## Lifecycle

1. **Bootstrap (first startup only)**: `main()` calls `seedSystemDir` for `guides`, `workflows`, and `plugins` in sequence. Each copies the corresponding `seed/` subtree to `state/system/` if the destination doesn't yet exist. `core.md` is handled separately: copied from `seed/core.md` to `state/system/core.md` only if the destination is missing.

2. **Reflex seeding (first reflex load only)**: The reflex engine's `Load()` method (`internal/reflex/engine.go:136`) creates `state/system/reflexes/` if absent, then calls `seedFromDefaults()` to copy `seed/reflexes/*.yaml` into it. This is the only subsystem that self-seeds rather than relying on `main.go`.

3. **Every startup**: `writeMCPConfig()` regenerates `state/.mcp.json` with the current HTTP MCP port (from `MCP_HTTP_PORT` env or default 8066). `wakeup.md` is re-read fresh from `seed/wakeup.md` into memory.

4. **Plugin discovery — CLI path**: When launching a Claude Code session, `scanLocalPlugins(statePath)` (`internal/executive/simple_session.go:25`) scans `state/system/plugins/*/` and returns the path of any directory containing `.claude-plugin/plugin.json`. These paths are passed as `--plugin-dir` flags, giving the Claude Code CLI session access to all skills via the `Skill` tool.

5. **Plugin discovery — SDK path**: On each session start, `loadAgentDefs()` calls `LoadAllAgents(statePath)` (`internal/executive/agent_defs.go:19`). This scans **all** subdirs of `state/system/plugins/` for `agents/*.yaml|.md` files, parses frontmatter, resolves skill aliases, and embeds skill content from `state/system/skills/<name>/SKILL.md` into each agent's system prompt. The resulting `map[string]AgentDefinition` (keyed by `namespace:agent`) is passed via `WithAgents` to the SDK session.

6. **Live reload**: Agent definitions are re-loaded on every session start (not cached), so edits to `state/system/plugins/*/agents/` take effect without restarting Bud. Reflexes are hot-reloaded when their file mod time changes (checked on each percept).

## Design Decisions

- **One-time seed, mutable state**: `seedSystemDir` is explicitly idempotent — it skips if the destination exists. This means `state/system/` is the authoritative live config. Shipping a new seed file doesn't affect running instances; it only takes effect on fresh deployments or manual copy. This design allows operators to customize live config without it being overwritten on restart.

- **`wakeup.md` reads from seed on every startup**: Unlike guides and plugins, the autonomous wake prompt is read fresh from `seed/wakeup.md` each time Bud starts. This lets the wake prompt be updated by changing seed and restarting (no manual state copy needed), since wakeup instructions are considered more volatile than identity/guides.

- **`.mcp.json` regenerated on every startup**: The MCP config must always point to the correct port. Rather than seeding it once and risking stale config, `writeMCPConfig` re-writes it on startup, merging with any existing entries so user-added servers are preserved.

- **Agent defs loaded per-session**: `loadAgentDefs()` is called at the start of every Claude session rather than caching at startup. This live-loads any agent edits without a Bud restart — a deliberate tradeoff of slight latency per session for operational flexibility.

- **Two plugin loading paths for two execution contexts**: The CLI path (`--plugin-dir`) integrates with Claude Code's native skill mechanism; the SDK path (`WithAgents`) embeds everything into the system prompt. These exist because the executive runs Claude Code as a subprocess (CLI) while subagents use the SDK directly — different execution contexts with different capability mechanisms.

## Integration Points

| From | To | What crosses the boundary |
|------|----|--------------------------|
| `cmd/bud/main.go` | `seed/` | One-time bootstrap copy of guides, workflows, plugins via `seedSystemDir` |
| `cmd/bud/main.go` | `seed/wakeup.md` | Read on each startup; string passed to `ExecutiveV2Config.WakeupInstructions` |
| `internal/executive/simple_session.go` | `state/system/plugins/` | `scanLocalPlugins` returns plugin paths for `--plugin-dir` CLI flags |
| `internal/executive/agent_defs.go` | `state/system/plugins/*/agents/` | `LoadAllAgents` builds `AgentDefinition` map for SDK sessions |
| `internal/executive/profiles.go` | `state/system/skills/` | `LoadSkillContent` reads skill body for embedding into agent prompts |
| `internal/reflex/engine.go` | `seed/reflexes/` | Self-seeding into `state/system/reflexes/` on first load |
| `cmd/bud/main.go` | `state/.mcp.json` | `writeMCPConfig` writes HTTP MCP server config on every startup |

## Non-Obvious Behaviors

- **Skills silently absent in SDK-spawned subagents**: `LoadSkillContent` looks at `state/system/skills/` but this directory does not exist — skills live in `state/system/plugins/<ns>/skills/`. So agent `Skills` lists (e.g. `bud:researcher` has `skills: [web-research]`) silently produce empty content for SDK-spawned subagents. The Skill tool works for the executive session (via `--plugin-dir`) but skill content is not embedded in subagent system prompts.

- **Seed updates are invisible to running instances**: If a new `seed/guides/foo.md` is added, the running Bud won't see it because `seedSystemDir` is a no-op when the destination exists. New guide files only land in `state/system/guides/` on a clean deployment or manual copy.

- **Reflexes seed themselves, not via main bootstrap**: The reflex seeding path (`internal/reflex/engine.go:152`) is independent of `seedSystemDir` in `main.go`. Reflexes in `seed/reflexes/` land in `state/system/reflexes/` via the engine's own `Load()` call, not via the three-directory loop in main.

- **Agent directories don't require `plugin.json`**: `LoadAllAgents` scans all subdirectories of `plugins/` for agent files regardless of whether `.claude-plugin/plugin.json` is present. The `autopilot-*` namespaces have no `plugin.json` but their agents are still loaded. Only skill surfacing to the Claude Code `Skill` tool requires `plugin.json`.

- **`core.md` becomes a mutable live file**: After the first copy from `seed/core.md`, `state/system/core.md` is Bud's live identity prompt. Edits there take effect on the next Claude session. The seed version is only consulted if `state/system/core.md` is deleted.

## Start Here

- `cmd/bud/main.go:1499` — `seedSystemDir` and `writeMCPConfig`: the full bootstrap sequence in ~60 lines
- `internal/executive/simple_session.go:25` — `scanLocalPlugins`: the CLI plugin path (how skills reach Claude Code)
- `internal/executive/agent_defs.go:12` — `LoadAllAgents`: the SDK agent path (how agents reach subagents)
- `internal/executive/profiles.go:162` — `LoadSkillContent` and `ResolveSubagentConfig`: skill embedding logic
- `seed/guides/skills.md` — conceptual overview of the plugin structure and when to use each mechanism
