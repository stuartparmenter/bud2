---
topic: State-Defaults Overlay & Configuration Loading
repo: bud2
generated_at: 2026-04-11T00:00:00Z
commit: d5c10d8
key_modules: [internal/paths, internal/config, cmd/bud]
score: 0.82
---

# State-Defaults Overlay & Configuration Loading

> Repo: `bud2` | Generated: 2026-04-11 | Commit: d5c10d8

## Summary

This subsystem controls how Bud discovers and loads all of its runtime configuration: the LLM provider config (`bud.yaml`), and the filesystem-level system files (instructions, reflexes, plugins, guides) that live in either `state-defaults/system/` (shipping defaults) or `state/system/` (user overrides). It solves two problems: giving Bud a safe set of shipping defaults without requiring users to manually copy or maintain them, and allowing local customization that persists across Bud upgrades. The central mechanism is a read-time overlay — `state/system/<path>` wins over `state-defaults/system/<path>` if both exist; no files are ever copied on startup.

## Key Data Structures

### `BudConfig` (`internal/config/config.go`)
Top-level configuration loaded from `bud.yaml` (or the `--config`/`BUD_CONFIG` path). Fields:
- `Providers map[string]ProviderConfig` — keyed by provider name (`claude-code`, `opencode-serve`, etc.)
- `Models map[string]string` — role-to-model-ref mapping (e.g. `executive: "claude-code/claude-sonnet-4-6"`)
- `TerminalManager string` — `"zellij"` (default) or `"tmux"`

If `bud.yaml` is absent, `DefaultConfig()` produces a built-in fallback that covers the `claude-code` provider.

### `ProviderConfig` (`internal/config/config.go`)
Configuration for one LLM backend:
- `Type string` — `"claude-code"`, `"opencode-serve"`, or `"openai-compatible"`
- `APIKeyEnv string` — name of the env var holding the API key (e.g. `"ANTHROPIC_API_KEY"`)
- `BaseURL string` — optional override for the API endpoint
- `Models map[string]ModelConfig` — per-model capacity overrides (context window, max output tokens)
- `Properties map[string]any` — provider-specific extension data

### `ModelConfig` (`internal/config/config.go`)
Capacity declarations for a specific model within a provider:
- `ContextWindow int` — maximum context length in tokens
- `MaxOutputTokens int` — output cap per completion

### Overlay resolution (no struct — implicit contract in `paths.go`)
The overlay has no explicit struct. It is a naming convention:
- `<statePath>/system/<relPath>` — user override (wins)
- `state-defaults/system/<relPath>` — shipping default (fallback)
- `relPath` is always relative to `system/`, e.g. `"core.md"`, `"reflexes/gtd-handler.yaml"`

## Lifecycle

### Config loading

1. **Load env file**: `main()` calls `godotenv.Load()` (optional — no error if absent). Environment variables for API keys and feature flags are set here.

2. **Resolve config path**: `main()` checks the `--config` CLI flag, then the `BUD_CONFIG` env var. If either is set, it calls `config.Load(path)` which reads the file, unmarshals YAML into `BudConfig`, and returns it. If neither is set, it calls `config.DefaultConfig()` which returns a hard-coded `BudConfig` with the `claude-code` provider wired to `ANTHROPIC_API_KEY`.

3. **Validate and resolve model roles**: The config is used to call `budCfg.ResolveModel(role)` for `"executive"` and `"agent"` roles, returning `(providerName, modelID, err)`. If a role is missing, `ResolveModel` falls back to the `"executive"` role. `budCfg.APIKey(providerName)` reads the actual key from the environment using `APIKeyEnv`.

4. **Initialize provider**: Based on the resolved provider name, `main()` constructs the appropriate `provider.Provider` implementation. The `BudConfig` is consumed at this step; the rest of startup uses the constructed provider, not the config struct.

### State-defaults overlay (read-time)

5. **Create state dirs**: `paths.EnsureStateSystemDirs(statePath)` creates `<statePath>/system/` and required subdirectories. It does **not** copy any files — this is the key behavioral difference from the old seed-copy model.

6. **Load system files on demand**: Whenever the executive, reflex engine, or main needs a file from the system directory (e.g. `wakeup.md` for autonomous wakes, `startup-instructions.md` for daemon restarts), it calls `paths.ResolveFile(statePath, relPath)`. This function:
   - Constructs `statePath + "/system/" + relPath`
   - If that file exists, reads and returns it (user override)
   - Otherwise constructs `DefaultsDir + "/system/" + relPath` (`state-defaults/system/<relPath>`)
   - Returns content and a boolean indicating whether it came from state (true) or defaults (false)
   - Returns `("", false)` if neither exists

7. **Merge directories**: For directories containing multiple files (reflexes, plugins, guides), callers use `paths.MergeDir(statePath, subDir, extensions)` or `paths.MergeDirRecursive(...)`:
   - Builds a name→path map from `state-defaults/system/<subDir>/`
   - Overlays with name→path entries from `state/system/<subDir>/` (state entries win by filename)
   - Returns sorted slice of absolute file paths
   - Files with the same name in both dirs: only the state version appears in the result

8. **Reflex hot-reload**: `paths.MergeDir` is called on each incoming percept to check for reflex config updates (comment in `processPercept`), so new or modified reflex YAML in `state/system/reflexes/` takes effect without restart.

## Design Decisions

- **Read-time overlay instead of copy-on-startup**: The old model copied files from `state-defaults/` to `state/` once at startup. When defaults changed (e.g. new wakeup instructions, updated reflexes), users would keep stale copies unless they manually deleted them. The new model never copies — it reads from defaults at runtime and lets `state/system/` shadow specific files. This means upgrades to `state-defaults/` take effect immediately for any file the user hasn't customized. (Adopted in commits 49aa775 and 6efc99f per the overview.)

- **`EnsureStateSystemDirs` creates structure but not content**: The function creates the directory tree so that `state/system/` paths are valid places to write customizations, but populating them is the user's choice. This avoids the "surprise copy" problem where startup silently installed files the user didn't ask for.

- **Name-based shadowing in `MergeDir`, not path-based**: Two files shadow each other only if they have the same filename within the subdirectory. A file at `state/system/reflexes/my-custom.yaml` does not affect `state-defaults/system/reflexes/gtd-handler.yaml`. This allows additive customization (add a new reflex) without requiring a full copy of all defaults.

- **`MergeDirRecursive` uses relative path as the key**: For nested structures, the full relative path (not just filename) determines shadowing — `state/system/plugins/bud/SKILL.md` shadows `state-defaults/system/plugins/bud/SKILL.md` specifically.

- **`config.DefaultConfig()` as a fallback, not a file**: The default LLM config is embedded in code rather than stored in `state-defaults/`. This keeps the two systems separate: LLM provider config is user-supplied (sensitive API keys, model preferences), while system files are operator-supplied (instructions, reflexes). A missing `bud.yaml` is a valid startup state; missing `state-defaults/` is not.

## Integration Points

| From | To | What crosses the boundary |
|------|----|--------------------------|
| `cmd/bud/main.go` | `internal/config` | `config.Load(path)` or `config.DefaultConfig()` at startup; returns `*BudConfig` |
| `cmd/bud/main.go` | `internal/paths` | `EnsureStateSystemDirs`, `ResolveFile`, `MergeDir` — called throughout startup and at runtime for system files |
| `internal/executive/executive_v2.go` | `internal/paths` | `ResolveFile` for wakeup.md and startup-instructions.md at session start (likely — inferred from overview description) |
| `internal/reflex/engine.go` | `internal/paths` | `MergeDir("reflexes", [".yaml", ".yml"])` to discover and hot-reload reflex configs |
| `cmd/bud/main.go` | `internal/executive/provider` | Constructed from `BudConfig.ResolveModel` output; provider type drives which implementation is instantiated |
| `internal/config` | OS environment | `APIKey(providerName)` reads `os.Getenv(APIKeyEnv)` — the only runtime env access in the config package |

## Non-Obvious Behaviors

- **`ResolveFile` returns the defaults content, not a path**: Callers get the file content directly, not a pointer to where it came from. The boolean return value (`fromState`) lets callers distinguish override from default, but most callers ignore it. If a system file is missing from both locations, callers receive `("", false)` — they must handle empty content gracefully.

- **Reflex configs reload on every percept, not periodically**: `main.go` calls `paths.MergeDir` inside `processPercept` to check for reflex config updates. This means a new `.yaml` dropped into `state/system/reflexes/` takes effect on the very next incoming message — no restart or explicit reload command needed.

- **`state-defaults/` is relative to the binary's working directory**: `DefaultsDir = "state-defaults"` is a bare directory name, not an absolute path. When Bud runs with `WorkDir: statePath` (the executive's working directory), but `state-defaults/` must be accessible from wherever the binary is launched. In practice, Bud is started from the source checkout directory (via launchd/systemd with `WorkingDirectory` set), so `state-defaults/` resolves to the repo directory.

- **`MergeDir` returns deterministic ordering (sorted by name), not by modification time**: This matters for reflex evaluation order — if multiple reflex files match a percept, they are evaluated in alphabetical filename order, not in the order they were created or last modified.

- **`DefaultConfig()` does not set `TerminalManager`**: Calling `budCfg.GetTerminalManager()` on the default config returns `"zellij"` (the method supplies a hardcoded default when the field is empty), but the YAML field is not populated. Code that inspects the struct field directly rather than calling `GetTerminalManager()` would see an empty string.

- **Config is consumed at startup and not re-read**: `BudConfig` is loaded once at daemon startup. Changes to `bud.yaml` require a daemon restart. This is unlike system files (reflexes, instructions) which are read on demand and reflect live changes.

## Start Here

- `internal/paths/paths.go` — the entire overlay logic is here (≈120 lines); read `ResolveFile` and `MergeDir` in full to understand the shadowing semantics before touching any system file loading
- `internal/config/config.go` — `BudConfig`, `ProviderConfig`, `Load`, `ResolveModel`, and `DefaultConfig`; understand this before adding a new LLM provider
- `cmd/bud/main.go` (startup section, before exec initialization) — see exactly which files are resolved at startup (`EnsureStateSystemDirs`, `writeMCPConfig`, wakeup/startup instruction loads) vs. on demand
- `state-defaults/system/` — browse this directory to understand what shipping defaults exist; any file here can be overridden by a same-path file in `state/system/`
- `bud.yaml.example` — reference config; the only documentation for `bud.yaml` fields beyond the struct definition in `config.go`
