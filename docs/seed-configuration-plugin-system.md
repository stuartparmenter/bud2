---
topic: Seed Configuration & Plugin System
repo: bud2
generated_at: 2026-04-08T08:00:00Z
commit: 8fe8c0b
key_modules: [cmd/bud, seed/]
score: 0.32
---

# Seed Configuration & Plugin System

> Repo: `bud2` | Generated: 2026-04-08 | Commit: 8fe8c0b

## Summary

The seed system bootstraps Bud's live configuration from a set of versioned template files in `seed/`. On first run, seeds are copied once into `state/system/`; after that, the state directory is the live config and seeds are ignored. The plugin system extends this through two complementary loading paths: **local plugins** (always-bundled core plugins in `state/system/plugins/`) and **manifest plugins** (external GitHub repos or local paths declared in `state/system/plugins.yaml`, cloned/updated at startup into an OS cache dir). Agent definitions, tool grants, and skill access are controlled through a layered set of YAML files that hot-reload without a daemon restart.

## Key Data Structures

### `Agent` (`internal/executive/profiles.go`)
Parsed from YAML frontmatter + markdown body in agent files. Fields: `Name string`, `Description string`, `Model string`, `Skills []string`, `Tools []string`, `Body string`. The `Body` becomes the system prompt; `Skills` names are resolved to skill content and concatenated into the prompt — but only if no `skill-grants.yaml` entry matches this agent (grants file wins over agent.Skills).

### `AgentDefinition` (`github.com/severity1/claude-agent-sdk-go`)
The SDK type used to register agents. Fields: `Description string`, `Prompt string` (assembled from agent body + skill content), `Tools []string`, `Model AgentModel`. Returned by `LoadAllAgents()` and passed to Claude sessions via `WithAgents`.

### `pluginManifestEntry` (`internal/executive/simple_session.go:58`)
A single entry in `plugins.yaml`. Supports string form (`"owner/repo[:dir][@ref]"`) or object form with fields: `repo`, `dir`, `ref`, `path` (local path override), `tool_grants` (pattern → tool list), `exclude` (sub-plugin names to skip). Remote entries are cloned under `~/Library/Caches/bud/plugins/<owner>/<repo>/`; `path:` entries use the local filesystem directly.

### `pluginDir` (`internal/executive/simple_session.go:130`)
Associates a local filesystem path with any tool grants from its manifest entry (`ToolGrants map[string][]string`). Used when loading agent definitions so grants can be applied per-plugin rather than globally.

### `SkillGrants` (`internal/executive/agent_defs.go`)
Loaded from `state/system/skill-grants.yaml`. Maps agent-key glob patterns to skill name lists (`Grants map[string][]string`). Examples: `"autopilot-*:*": [gk-conventions]`, `"*": [save_thought]`. Match priority: exact key > `"namespace:*"` > filepath.Match glob > `"*"`.

### `AgentAliases` (`internal/executive/profiles.go`)
Loaded from `state/system/agent-aliases.yaml`. Has `Skills map[string]string` (alias → canonical skill name). Only skill aliases remain active; agent aliases are legacy.

### `seedSystemDir(statePath, dirName string)` (`cmd/bud/main.go`)
A one-shot copy function: if `state/system/<dirName>` does not exist, copies `seed/<dirName>/` to `state/system/<dirName>/` using `os.CopyFS`. Returns silently if the destination already exists or the source is missing.

## Lifecycle

1. **Bootstrap (first startup only)**: `main()` calls `seedSystemDir` for `guides`, `workflows`, and `plugins` in sequence. Each copies the corresponding `seed/` subtree to `state/system/` if the destination doesn't yet exist. `core.md` is handled separately: copied from `seed/core.md` to `state/system/core.md` only if the destination is missing.

2. **Reflex seeding (first reflex load only)**: The reflex engine's `Load()` method (`internal/reflex/engine.go`) creates `state/system/reflexes/` if absent, then calls `seedFromDefaults()` to copy `seed/reflexes/*.yaml` into it. This is the only subsystem that self-seeds rather than relying on `main.go`.

3. **Every startup — MCP config and wakeup**: `writeMCPConfig()` regenerates `state/.mcp.json` with the current HTTP MCP port (default 8066, bound synchronously before the P1 goroutine starts). `wakeup.md` is re-read fresh from `seed/wakeup.md` into memory on each startup.

4. **Manifest plugin loading (`loadManifestPlugins`)**: `simple_session.go:loadManifestPlugins` reads `state/system/plugins.yaml`. For each remote entry (`owner/repo`): if the repo hasn't been cloned, `git clone --depth=1` into `~/Library/Caches/bud/plugins/<owner>/<repo>/`; if it exists, `git fetch && git checkout` to stay current. For `path:` entries the local path is used directly. Each resolved path is then passed to `resolvePluginPathsFromLocalPath`, which either returns the path itself (if it looks like a plugin dir) or expands one level deep for monorepo-style repos. The `exclude:` list is applied during `resolvedManifestPluginDirs()` — named sub-plugin directories are skipped before the path list is returned.

5. **Zettel library generation (`generateZettelLibraries`)**: Called inside `SendPrompt` after manifest plugins are loaded (once per session). Scans all plugin dirs for `plugin.json` entries that declare a `"zettels"` path and writes `state/system/zettel-libraries.yaml`. Plugins from the OS cache directory are always marked `readonly: true` to prevent accidental commits into cached clones.

6. **Plugin discovery — CLI path**: `scanLocalPlugins(statePath)` scans `state/system/plugins/*/` for directories containing `.claude-plugin/plugin.json`. These paths are passed as `--plugin-dir` flags to the Claude Code CLI session. `loadManifestPlugins` paths are also passed as `--plugin-dir`, making all manifest plugins available to the CLI's `Skill` tool.

7. **Plugin discovery — SDK path (`LoadAllAgents`)**: On each session start, `loadAgentDefs()` calls `LoadAllAgents(statePath, knownTools)`. This calls `allPluginDirsForAgents` (local + manifest, with tool grants attached) and scans each dir's `agents/*.yaml|.md` for agent definitions. For each agent, skill resolution uses `resolveGrantedSkills(grants, key)` first (skill-grants.yaml wins); if no match, falls back to `agent.Skills`. Skill content is loaded via `LoadSkillContent(allPluginDirs(statePath), skillName)`, which searches both local and manifest plugin dirs. Tool grants from the plugin's manifest entry are applied per-agent via `expandToolGrants`, which expands glob wildcards (e.g. `"mcp__bud2__gk_*"`) against `knownTools` (set by `exec.SetKnownMCPTools` after MCP server init).

8. **Live reload**: Agent definitions are re-loaded on every session start (not cached), so edits to `state/system/plugins/*/agents/` or changes to `plugins.yaml` take effect without restarting Bud. Reflexes are hot-reloaded when their file mod time changes (checked on each percept).

## Design Decisions

- **One-time seed, mutable state**: `seedSystemDir` is explicitly idempotent — it skips if the destination exists. `state/system/` is the authoritative live config. Shipping a new seed file doesn't affect running instances; it only takes effect on fresh deployments or manual copy. This design allows operators to customize live config without it being overwritten on restart.

- **`wakeup.md` reads from seed on every startup**: Unlike guides and plugins, the autonomous wake prompt is read fresh from `seed/wakeup.md` each time Bud starts. This lets the wake prompt be updated by changing seed and restarting (no manual state copy needed), since wakeup instructions are considered more volatile than identity/guides.

- **`.mcp.json` regenerated on every startup**: The MCP config must always point to the correct port. Rather than seeding it once and risking stale config, `writeMCPConfig` re-writes it on startup, merging with any existing entries so user-added servers are preserved.

- **Agent defs loaded per-session, not cached**: `loadAgentDefs()` is called at the start of every Claude session. This live-loads any agent edits without a Bud restart — a deliberate tradeoff of slight latency per session for operational flexibility.

- **Two plugin loading paths for two execution contexts**: The CLI path (`--plugin-dir`) integrates with Claude Code's native skill mechanism; the SDK path (`WithAgents`) embeds everything into the system prompt. These exist because the executive runs Claude Code as a subprocess (CLI) while subagents use the SDK directly — different execution contexts with different capability mechanisms. Both paths now include manifest plugins.

- **Cache clones are always readonly**: `generateZettelLibraries` marks any zettel library sourced from `~/Library/Caches/bud/plugins/` as `readonly: true`. This prevents agents from committing into a cached clone (which would be lost on the next `git fetch`).

- **Exclude lists for sub-plugin suppression**: The `exclude:` list on a manifest entry is applied in `resolvedManifestPluginDirs()` before the path is added to the dir list. This lets a single repo entry (e.g. `stuartparmenter/autopilot:plugins`) expose most sub-plugins while suppressing specific ones (e.g. `issues-linear`) that are not relevant to this deployment.

- **Centralized skill grants override per-agent Skills**: `skill-grants.yaml` is checked first in `LoadAllAgents`; the per-agent `skills:` field is only consulted if no grant pattern matches. This enables cluster-level grant changes without editing every agent file.

- **Periodic state-sync removed**: The hourly `git add -A && git push` goroutine was disabled because tracking large binary files (`memory.db`, WASM blobs) caused pathological git behavior. State is no longer auto-synced — the git repo is for manual checkpoints.

- **statePath defaults to `~/Documents/bud-state`**: Changed from a relative `"state"` path to an absolute home-relative default, enabling Bud to run from any working directory without breaking state discovery.

## Integration Points

| From | To | What crosses the boundary |
|------|----|--------------------------|
| `cmd/bud/main.go` | `seed/` | One-time bootstrap copy of guides, workflows, plugins via `seedSystemDir` |
| `cmd/bud/main.go` | `seed/wakeup.md` | Read on each startup; string passed to `ExecutiveV2Config.WakeupInstructions` |
| `internal/executive/simple_session.go` | `state/system/plugins.yaml` | `loadManifestPlugins` reads manifest, clones/updates repos into OS cache |
| `internal/executive/simple_session.go` | `~/Library/Caches/bud/plugins/` | Cache dir for cloned GitHub plugin repos; auto-readonly in zettel-libraries |
| `internal/executive/simple_session.go` | `state/system/plugins/` | `scanLocalPlugins` returns plugin paths for `--plugin-dir` CLI flags |
| `internal/executive/agent_defs.go` | `state/system/skill-grants.yaml` | `LoadSkillGrants` reads centralized agent→skill map |
| `internal/executive/agent_defs.go` | all plugin dirs (local + manifest) | `LoadAllAgents` builds `AgentDefinition` map; `expandToolGrants` expands tool wildcards |
| `internal/executive/profiles.go` | all plugin dirs (local + manifest) | `LoadSkillContent` searches all dirs for skill SKILL.md files |
| `internal/reflex/engine.go` | `seed/reflexes/` | Self-seeding into `state/system/reflexes/` on first load |
| `cmd/bud/main.go` | `state/.mcp.json` | `writeMCPConfig` writes HTTP MCP server config on every startup |
| `cmd/bud/main.go` | `mcpServer.ToolNames()` | `SetKnownMCPTools` feeds prefixed tool names for wildcard grant expansion |

## Non-Obvious Behaviors

- **Exclude list applied in `resolvedManifestPluginDirs`, not `loadManifestPlugins`**: `loadManifestPlugins` (used for CLI `--plugin-dir`) does *not* apply the `exclude:` list — it returns all sub-plugin paths. Only `resolvedManifestPluginDirs` (used for SDK agent loading and MCP server suppression) applies the exclude filter. A plugin in the exclude list may still appear in CLI sessions.

- **Skill content now found for SDK subagents**: Before the `allPluginDirs` refactor, `LoadSkillContent` only searched `state/system/skills/` (which doesn't exist — skills live in plugin dirs). Now it searches the full list of all plugin dirs (local + manifest), so SDK-spawned subagents get correct skill content embedded in their system prompts.

- **zettel-libraries.yaml is regenerated every session, not every startup**: `generateZettelLibraries` is called inside `SendPrompt`, not in `main()`. If the user edits `plugin.json` declarations, the change takes effect on the next Claude session — no daemon restart needed.

- **Tool grants wildcards require `SetKnownMCPTools` to be called first**: `expandToolGrants` only expands patterns like `"mcp__bud2__gk_*"` if `knownTools` is non-nil. If `exec.SetKnownMCPTools` is not called (or called before `mcpServer.ToolNames()` is populated), wildcard grants are silently dropped. The call happens after MCP tool registration in `main()`.

- **Seed updates are invisible to running instances**: If a new `seed/guides/foo.md` is added, the running Bud won't see it because `seedSystemDir` is a no-op when the destination exists. New guide files only land in `state/system/guides/` on a clean deployment or manual copy.

- **Reflexes seed themselves, not via main bootstrap**: The reflex seeding path (`internal/reflex/engine.go`) is independent of `seedSystemDir` in `main.go`. Reflexes in `seed/reflexes/` land in `state/system/reflexes/` via the engine's own `Load()` call, not via the three-directory loop in main.

- **Agent directories don't require `plugin.json`**: `LoadAllAgents` scans all subdirectories of each plugin dir for agent files regardless of whether `.claude-plugin/plugin.json` is present. The `autopilot-*` namespaces have no `plugin.json` but their agents are still loaded. Only skill surfacing to the Claude Code `Skill` tool requires `plugin.json`.

## Start Here

- `cmd/bud/main.go` (search `seedSystemDir`, `writeMCPConfig`) — the full bootstrap sequence and statePath default
- `internal/executive/simple_session.go:181` — `loadManifestPlugins`: GitHub clone/update, local path, exclude list logic
- `internal/executive/simple_session.go:286` — `resolvedManifestPluginDirs`: exclude filtering and tool grants attachment
- `internal/executive/agent_defs.go` — `LoadAllAgents`, `LoadSkillGrants`, `resolveGrantedSkills`, `expandToolGrants`
- `internal/executive/profiles.go` — `LoadSkillContent` (multi-dir search) and `ResolveSubagentConfig`
- `state/system/plugins.yaml` — the live manifest; edit here to add/exclude plugins or configure tool grants
- `state/system/skill-grants.yaml` — centralized agent→skill grants; edit here to change which skills an agent type can invoke
