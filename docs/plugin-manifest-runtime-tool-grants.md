---
topic: Plugin Manifest Runtime & Tool Grants
repo: bud2
generated_at: 2026-04-07T13:44:02Z
commit: 33d63834
key_modules: [cmd/bud, internal/executive/agent_defs.go, internal/executive/profiles.go, internal/executive/simple_session.go, state/system/plugins.yaml, state/system/skill-grants.yaml]
score: 0.66
---

# Plugin Manifest Runtime & Tool Grants

> Repo: `bud2` | Generated: 2026-04-07 | Commit: 33d63834

## Summary

The plugin manifest system allows external GitHub repos (and local paths) to register agent definitions, skills, and per-agent tool grants into bud2 at runtime without modifying core configuration. At startup, bud2 clones or updates each listed repo to `~/.cache/bud/plugins/`, then at agent-spawn time loads agent definitions from those directories with the tool grants they declared. The `skill-grants.yaml` file in state complements this with a centralized pattern-based mapping from agent identity to skill names.

## Key Data Structures

### `pluginManifest` / `pluginManifestEntry` (`internal/executive/simple_session.go`)

`pluginManifest` is the in-memory representation of `state/system/plugins.yaml`. Each `pluginManifestEntry` captures one plugin source:

```go
type pluginManifestEntry struct {
    owner, repo  string              // remote GitHub repo (e.g. "vthunder", "useful-plugins")
    dir          string              // subdirectory within repo (empty = root)
    ref          string              // branch/tag/commit (empty = default branch)
    localPath    string              // alternative: local filesystem path
    ToolGrants   map[string][]string // agent pattern → list of tool names (may include wildcards)
}
```

`ToolGrants` is the security boundary: it declares which MCP tools agents from this plugin are allowed to invoke. Patterns are `"namespace:agent"` globs (e.g. `"autopilot-*:*"`) or `"*"` for all agents.

### `pluginDir` (`internal/executive/simple_session.go`)

Associates a resolved local filesystem path with the tool grants from its manifest entry:

```go
type pluginDir struct {
    Path       string
    ToolGrants map[string][]string // nil for local (non-manifest) plugins
}
```

Local plugins under `state/system/plugins/` always have `nil` ToolGrants — they can only use tools they declare in their agent YAML `tools:` field.

### `SkillGrants` (`internal/executive/agent_defs.go`)

Loaded from `state/system/skill-grants.yaml`. Maps agent identity patterns to skill names:

```go
type SkillGrants struct {
    Grants map[string][]string `yaml:"grants"` // pattern → skill names
}
```

This is the centralized alternative to the per-agent `skills:` field in agent YAML. When a skill-grants.yaml entry matches an agent, it wins over the agent's own `Skills` field.

### `Agent` (`internal/executive/profiles.go`)

Parsed in-memory representation of an agent definition file (`agents/*.md` or `agents/*.yaml`):

```go
type Agent struct {
    Name, Description string
    Level  string
    Model  string   // "sonnet" / "opus" / "haiku"
    Skills []string // skill names — fallback if no skill-grants.yaml entry matches
    Tools  []string // additional tools beyond the base set
    Body   string   // markdown body after YAML frontmatter
}
```

The `Body` field is the agent's system prompt content. Skills listed here are resolved by `LoadSkillContent()` and concatenated to `Body` to form the final system prompt.

### `AgentAliases` (`internal/executive/profiles.go`)

Loaded from `state/system/agent-aliases.yaml`. Allows remapping agent names and skill names:

```go
type AgentAliases struct {
    Agents map[string]string `yaml:"agents"` // alias → resolved file path
    Skills map[string]string `yaml:"skills"` // alias → resolved skill name
}
```

Used for backward compatibility when plugin namespaces or agent file names change.

## Lifecycle

### Phase 1 — Startup: Plugin Cloning (`loadManifestPlugins`)

Called once during `main()` before the SDK session is created. Returns a list of local paths to pass to `WithLocalPlugin`.

1. **Read manifest**: `loadManifestPlugins(statePath)` reads `state/system/plugins.yaml`. Parse errors are logged and the function returns nil (non-fatal).

2. **Resolve cache dir**: `pluginCacheDir = ~/.cache/bud/plugins/` (via `os.UserCacheDir()`).

3. **For each entry** in `manifest.Plugins`:
   - **Local path**: if `e.localPath != ""` and no `e.owner`, skip git and call `resolvePluginPathsFromLocalPath(e.localPath)` directly.
   - **Remote repo (first run)**: if `~/.cache/bud/plugins/<owner>/<repo>/` does not exist, run `git clone --depth=1 [--branch <ref>] <url> <repoDir>`.
   - **Remote repo (subsequent runs)**:
     - If `e.ref` is set (pinned): `git fetch --depth=1 origin <ref>` then `git checkout <ref>`.
     - If `e.ref` is empty (floating): `git pull --ff-only`. Failures are logged but non-fatal — existing checkout is kept.

4. **Resolve plugin dirs**: Apply `e.dir` to get `localPath = repoDir[/dir]`. Then call `resolvePluginPathsFromLocalPath(localPath)`:
   - If `localPath` itself has `.claude-plugin/plugin.json` → return as single dir.
   - Otherwise, scan immediate subdirectories and return those that `looksLikePluginDir()` (has `.claude-plugin/plugin.json`, an `agents/` dir, or `.md` files). This handles monorepo-style repos with multiple plugins.

5. **Return paths** — these are passed to the Claude Agent SDK as local plugin directories.

### Phase 2 — Startup: MCP Tool Registration & `SetKnownMCPTools`

After all MCP tools are registered (including stdio proxy servers from `.mcp.json`), `main.go` calls:

```go
exec.SetKnownMCPTools(tools)
```

This stores the full list of live MCP tool names on the executive. These names are needed for wildcard expansion in tool grants (e.g., `mcp__bud2__gk_*` → all registered gk tools). **This call must happen before any agent is spawned.**

### Phase 3 — Agent Spawn: `LoadAllAgents` / `ResolveSubagentConfig`

When the executive spawns a subagent, it calls either `LoadAllAgents` (for batch SDK initialization) or `ResolveSubagentConfig` (for single on-demand spawn).

**`LoadAllAgents(statePath, knownTools)`** (`agent_defs.go`):

1. **Load aliases**: `LoadAgentAliases(statePath)` reads `agent-aliases.yaml`.
2. **Load skill grants**: `LoadSkillGrants(statePath)` reads `skill-grants.yaml`. Returns empty grants if file missing.
3. **Collect plugin dirs**: `allPluginDirsForAgents(statePath)` merges:
   - Local plugins: `scanLocalPlugins()` scans `state/system/plugins/` — each gets `pluginDir{ToolGrants: nil}`.
   - Manifest plugins: `resolvedManifestPluginDirs()` reads plugins.yaml **without git ops**, returns already-cloned dirs with their `ToolGrants` intact.
4. **For each plugin dir**, scan `agents/` for `*.md` and `*.yaml` files. For each agent file:
   a. Parse agent definition via `parseAgentData()` (YAML frontmatter + markdown body).
   b. Build agent key: `"<pluginNamespace>:<agentName>"` (e.g. `"autopilot-vision:planner"`).
   c. **Resolve skills**: call `resolveGrantedSkills(grants, key)` — if a match found in skill-grants.yaml, use those skills; otherwise fall back to `agent.Skills`.
   d. **Assemble prompt**: load skill content for each skill via `LoadSkillContent(pluginDirs, skillName)` and concatenate to agent body.
   e. **Expand tool grants**: for each manifest entry's `ToolGrants`, find patterns that match the agent key via `matchesAgentPattern()`, then call `expandToolGrants(grantedTools, knownTools)` to expand wildcards against the live tool list.
   f. Build `claudecode.AgentDefinition` with the assembled prompt and merged tools list.
5. Return the map of all agent definitions.

### Phase 4 — Skill Resolution (`resolveGrantedSkills`)

Priority order (first match wins):
1. **Exact match**: `grants["autopilot-vision:planner"]`
2. **Namespace wildcard**: `grants["autopilot-vision:*"]`
3. **Glob patterns**: `filepath.Match("autopilot-*:planner", key)` — checked for all non-exact, non-`*` patterns
4. **Global wildcard**: `grants["*"]`

If none match, returns `(nil, false)` — the agent's own `Skills` field is used as fallback.

## Design Decisions

- **Startup cloning, on-demand agent loading**: `loadManifestPlugins` (with git ops) runs once at startup. `resolvedManifestPluginDirs` (no git, read-only) is called whenever agent definitions are needed. This avoids git operations on the hot path.

- **Tool grants live in plugins.yaml, not skill-grants.yaml**: Plugin authors control which MCP tools their agents can use by declaring `tool_grants` in the manifest. Core configuration (`skill-grants.yaml`) controls which skills agents get. This separates external trust grants from internal skill assignment.

- **Wildcard expansion against live tool list**: `expandToolGrants` resolves patterns like `mcp__bud2__gk_*` against the registered MCP tool names at agent-load time, not at startup. When a new gk tool is added to the MCP server, any agent with a matching wildcard grant automatically gets access — no manifest change required.

- **`"bud:*": []` override in skill-grants.yaml**: The `"*"` entry grants `gk-conventions` to all agents. The `"bud:*": []` entry explicitly overrides this for bud-namespace agents (coder, researcher, writer, reviewer), giving them no skills by default. Without this, every bud agent would get gk-conventions injected unnecessarily.

- **Monorepo expansion is one level deep**: `resolvePluginPathsFromLocalPath` only scans immediate children of the root path. This handles repos like `autopilot` where `plugins/` contains multiple plugin dirs, but does not recurse deeper.

## Integration Points

| From | To | What crosses the boundary |
|------|----|--------------------------|
| `cmd/bud/main.go` | `executive.loadManifestPlugins` | Called at startup; returns resolved plugin paths passed to SDK's `WithLocalPlugin` |
| `cmd/bud/main.go` | `executive.SetKnownMCPTools` | Called after all MCP tools registered; provides the name list for wildcard expansion |
| `internal/executive/executive_v2.go` | `agent_defs.LoadAllAgents` | Called when rebuilding the SDK session's agent pool (triggered by `SetKnownMCPTools`) |
| `internal/executive/agent_defs.go` | `simple_session.allPluginDirsForAgents` | Provides the `[]pluginDir` list (local + manifest with grants) for agent scanning |
| `internal/executive/agent_defs.go` | `profiles.LoadSkillGrants` + `LoadSkillContent` | Loads skill grant table and reads SKILL.md files from plugin dirs |
| `internal/executive/profiles.go:ResolveSubagentConfig` | `profiles.LoadAgent` + `LoadSkillContent` | Single-agent path used by the executive for on-demand spawning with explicit agent name |

## Non-Obvious Behaviors

- **Tool grants are not applied during `loadManifestPlugins`**: The cloning phase returns only plain paths. Tool grants are read separately by `resolvedManifestPluginDirs()` when building agent definitions. If you add a manifest entry with tool grants but don't restart bud, the grants won't take effect until the next `LoadAllAgents` call.

- **Local plugins (`state/system/plugins/`) cannot have tool grants**: Only manifest entries (in `plugins.yaml`) carry `ToolGrants`. Agents in local plugin dirs can only access tools they declare in their `tools:` YAML field. This is an implicit privilege difference between managed (manifest) and local plugins.

- **`skill-grants.yaml` absence is non-fatal**: If the file doesn't exist, `LoadSkillGrants` returns empty grants and every agent falls back to its own `skills:` field. The system works the same as before the file was introduced — backward compatible.

- **A single manifest entry can expand to many plugin dirs**: If `autopilot/plugins/` is listed with no `dir:`, and it contains `autopilot-core/`, `autopilot-vision/`, etc., each becomes a separate `pluginDir` — all inheriting the same `ToolGrants` from that manifest entry.

- **Skill alias resolution strips namespaces**: `LoadSkillContent` normalizes `"bud-ops:gk-conventions"` to `"gk-conventions"` before searching plugin dirs, so skills work regardless of whether they're referenced with or without a namespace prefix.

- **`SetKnownMCPTools` triggers a full agent reload**: After storing the tool list, `executive_v2.go` immediately calls `LoadAllAgents` to rebuild the agent definition map. Any tool grants with wildcards that couldn't be expanded before (because the list was empty) are now fully expanded.

## Start Here

- `internal/executive/simple_session.go` — read `loadManifestPlugins`, `resolvedManifestPluginDirs`, `expandToolGrants`, and `matchesAgentPattern` to understand the full plugin discovery and grant expansion pipeline
- `internal/executive/agent_defs.go` — `LoadAllAgents` is the integration point that combines plugin dirs, skill grants, and tool grants into `claudecode.AgentDefinition` objects
- `state/system/plugins.yaml` — the manifest that drives cloning, dir resolution, and tool grant declaration for external plugins
- `state/system/skill-grants.yaml` — centralized skill assignment; the `"bud:*": []` and `"*":` entries reveal the override precedence
- `internal/executive/profiles.go` — `resolveGrantedSkills`, `LoadSkillContent`, `Agent` type, and `AgentAliases` for understanding how skill names resolve to file content
