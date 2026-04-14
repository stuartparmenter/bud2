---
generated_at: 2026-04-14T06:25:00Z
commit: 31ceb932
repomix: available
---

# bud2 — Overview

> Generated: 2026-04-14 | Commit: 31ceb932

## Purpose

Bud is a personal AI agent that runs as a macOS launchd daemon (or Linux systemd service), providing autonomous assistance, long-term memory, and task management through a Discord interface. Built in Go, it uses a pluggable LLM provider architecture (`internal/executive/provider/`) to support multiple backends — Claude Code (`claude-code`), OpenCode Serve (`opencode-serve`), and OpenAI-compatible APIs — configured via `bud.yaml`. The default provider is `claude-code`. Bud gives each LLM provider an event loop, attention system, and a rich set of MCP tools for interacting with external services and its own state.

## Data Flow

External signals arrive through senses (`internal/senses/discord.go` for Discord messages, `internal/senses/calendar.go` for calendar events). Each sense creates a **percept** in the `PerceptPool` (`internal/memory/`) and enqueues a **focus item** into `focus.Queue` (`internal/focus/queue.go`). The `focus.Attention` system (`internal/focus/attention.go`) computes salience (priority × source boost × recency) and selects the highest-priority item. Before reaching the executive, the **reflex engine** (`internal/reflex/engine.go`) evaluates YAML rules resolved via `paths.MergeDir` from `state-defaults/system/reflexes/` (merged with `state/system/reflexes/` overrides) — simple queries are answered directly via Discord without waking Claude.

For items that pass through reflexes, `ExecutiveV2` (`internal/executive/executive_v2.go`) opens an LLM session via the configured provider (`internal/executive/provider/`), injects context assembled from focus state, recent memories (fetched from Engram via `internal/engram/client.go`), session-appropriate instructions (startup, autonomous, or interactive), and a handoff note (read from `state/handoff.md` if present — injected into all wake types via `buildPrompt`). Plugin manifests and skills are reloaded from extensions.yaml on every prompt without restart. Claude calls MCP tools registered in `internal/mcp/tools/register.go` (served by `internal/mcp/server.go` on port 8066). Responses flow back through `internal/effectors/discord.go`. After `signal_done`, the executive extracts `<memory_eval>` ratings and calls `RateEngrams()` to feed quality signals back to Engram. Autonomous wakes fire on a configurable timer (default 2h) and follow the same path, capped by `MaxAutonomousSessionDuration`.

## Module Map

| Path | Responsibility |
|------|---------------|
| `cmd/bud/main.go` | Daemon entry point — wires all subsystems, handles config, launchd/systemd lifecycle, plugin manifest loading, zettel-libraries generation |
| `internal/config/config.go` | Multi-provider LLM configuration — loads providers, models, terminal manager from `bud.yaml`; resolves model roles to provider+model pairs |
| `internal/paths/paths.go` | Read-time overlay resolution — `ResolveFile` and `MergeDir` serve files from `state/system/` with fallback to `state-defaults/system/` (replaces old seed-copy-on-startup model) |
| `internal/executive/provider/` | LLM provider abstraction — `Provider` interface with `claude-code` and `opencode-serve` implementations; pluggable session management |
| `internal/executive/executive_v2.go` | Core orchestrator — runs LLM sessions, manages signal_done, subagents, per-prompt agent/skill reload, memory quality feedback, handoff note injection for all wake types |
| `internal/executive/extensions.go` | Standalone skill loading — parses `skills:` section of extensions.yaml; downloads ClaWHub skills (HTTP zip, cached at `~/Library/Caches/bud/skills-clawhub`), clones git skills, loads local skills; `LoadSkillContent` searches all plugin dirs |
| `internal/executive/simple_session.go` | Plugin manifest parsing — handles `plugins:` entries (local-path, git), exclude list filtering, zettel-libraries merge (manual entries preserved) |
| `internal/executive/agent_defs.go` | Agent definition loading from plugins; applies tool grants and exclude lists from extensions.yaml |
| `internal/executive/profiles.go` | Agent profile definitions and skill grant application; wires standalone skills into agent contexts |
| `internal/focus/` | Attention system — salience computation, priority queue, focus/suspend/resume |
| `internal/senses/` | Input adapters — Discord and calendar event ingestion → percepts |
| `internal/effectors/` | Output adapters — Discord message sending and reactions |
| `internal/reflex/` | YAML-defined reflexes — fast-path responses without invoking Claude |
| `internal/mcp/` | MCP HTTP server and all tool registrations (GK, calendar, GitHub, VM, etc.) |
| `internal/memory/` | Short-term working memory — percept pool, threads, traces, inbox |
| `internal/engram/` | HTTP client to the Engram memory service (long-term graph memory); includes `RateEngrams()` for quality feedback |
| `internal/types/` | Shared type definitions — most-imported package (centrality 17) |
| `internal/integrations/` | External integration helpers (calendar, GitHub; centrality 16) |
| `internal/budget/` | Token/thinking-time budget tracking across sessions |
| `internal/gtd/` | Local GTD task store (JSON-backed; Things 3 integration via things-mcp MCP server) |
| `state-defaults/system/` | Bundled default plugins (`bud`, `bud-ops`), guides, reflexes, wakeup/startup instructions — read at runtime via `paths` overlay |
| `state/system/extensions.yaml` | Extensions manifest — `plugins:` section lists full plugin repos (tool_grants, exclude); `skills:` section lists standalone skills (ClaWHub, git, local) |
| `state/system/skill-grants.yaml` | Centralized agent→skill grants — controls which skills each agent type can invoke |

## Key Files

- `cmd/bud/main.go` — wires all subsystems together; start here to understand initialization order, config loading (`bud.yaml` or `BUD_CONFIG`), and terminal manager selection
- `internal/config/config.go` — multi-provider LLM config: declares providers, model roles, terminal manager; `bud.yaml.example` is the reference
- `internal/paths/paths.go` — `ResolveFile`/`MergeDir` implement the state-defaults overlay; understand this before touching config or prompt loading
- `internal/executive/executive_v2.go` — `ExecutiveV2` struct: session lifecycle, context assembly, subagent management, handoff injection, memory quality feedback after signal_done
- `internal/executive/extensions.go` — skill loading: ClaWHub (HTTP zip download), git (sparse checkout), local path; floating ClaWHub skills re-fetch based on `extensions.update_interval` in `bud.yaml`
- `internal/executive/simple_session.go` — plugin manifest parsing for the `plugins:` section; also handles zettel-libraries generation/merge
- `internal/mcp/tools/register.go` — all MCP tool definitions; largest file, entry point for any tool work
- `internal/reflex/engine.go` — YAML reflex loading, evaluation, and action dispatch
- `bud.yaml.example` — reference config for LLM providers, model assignments, terminal manager, and `extensions.update_interval`
- `state/system/extensions.yaml` — extensions manifest: `plugins:` for full plugin repos, `skills:` for standalone skills from ClaWHub, GitHub, or local paths

## Conventions

- **Testing**: Co-located `*_test.go` files. Go standard testing; run with `go test ./...`. Integration tests in `tests/scenarios/` (YAML-defined). See `docs/testing-playbook.md`.
- **Naming**: Go idioms throughout. Internal packages under `internal/`. Tool names in MCP use snake_case (e.g. `talk_to_user`, `signal_done`).
- **Entry points**: Single binary `bin/bud` built by `scripts/build.sh`. `things-mcp` is a TypeScript server built separately. Runs as launchd (macOS) or systemd (Linux).
- **Patterns to know**: Core defaults (`bud`, `bud-ops` plugins, guides, reflexes, wakeup/startup instructions) live in `state-defaults/system/` — these are the shipping defaults. The `paths` package implements a read-time overlay: `state/system/<path>` overrides `state-defaults/system/<path>` when present; no files are copied on startup. Config is loaded from `bud.yaml` (gitignored); `bud.yaml.example` is the reference. Terminal manager (zellij or tmux) set via `terminal_manager:` in `bud.yaml`, defaulting to zellij. Extensions are declared in `state/system/extensions.yaml` (replaces `plugins.yaml`; old name retained with deprecation warning). The `plugins:` section clones/updates GitHub repos at startup or loads local paths; use `git:owner/repo` or `path:/local/path` prefixes. The `skills:` section loads standalone skills from ClaWHub (`clawhub:slug[@ver]`), GitHub repos (`git:owner/repo[:dir][@ref]`), or local paths — ClaWHub skills are downloaded as a zip and cached locally; floating versions re-fetch based on `extensions.update_interval` in `bud.yaml` (default: 1h, 0 = disabled). `LoadSkillContent` warns when the same skill name is found in multiple dirs. Agent definitions and skills hot-reload on every prompt without restart. The `state/` directory is Bud's working directory (`~/Documents/bud-state`) and is a separate git repo. Handoff notes (`state/handoff.md`, if present) are injected into every session's context via `buildPrompt` — not just autonomous wakes. Memory retrieval is disabled for startup impulses; the limit is 6 engrams for regular sessions. After `signal_done`, recalled memory quality ratings are sent back to Engram via `RateEngrams()`. Profiling via `BUD_PROFILE=minimal|detailed|trace` (enable by uncommenting block in `main.go`). OS-aware `STATE_PATH` defaults: `~/Documents/bud-state` (macOS), `$XDG_DATA_HOME/bud/state` or `~/.local/share/bud/state` (Linux).

## Start Here

For a given task type, start at:
- **Adding a new MCP tool**: `internal/mcp/tools/register.go` — all tools are registered here; follow the pattern of an existing tool
- **Modifying reflex behavior**: `state-defaults/system/reflexes/*.yaml` — YAML rules evaluated by `internal/reflex/engine.go`; override in `state/system/reflexes/` for local customization
- **Changing how the LLM is prompted**: `internal/executive/executive_v2.go` around `buildPrompt` — context assembly, handoff injection; also check `state-defaults/system/core.md`, `state-defaults/system/wakeup.md` (autonomous wakes), and `state-defaults/system/startup-instructions.md` (daemon startup)
- **Configuring a different LLM provider**: `bud.yaml.example` → copy to `bud.yaml` and edit; `internal/config/config.go` defines the schema; supported providers: `claude-code`, `opencode-serve`, `openai-compatible`
- **Adding/configuring a plugin**: `state/system/extensions.yaml` `plugins:` section — add `git:owner/repo` or `path:/local/path`; use `tool_grants` and `exclude:` to control tool access
- **Adding a standalone skill**: `state/system/extensions.yaml` `skills:` section — `clawhub:slug[@version]`, `git:owner/repo[:dir][@ref]`, or `path:/local/path`; set re-fetch interval via `extensions.update_interval` in `bud.yaml`
- **Changing agent skill access**: `state/system/skill-grants.yaml` — centralized grants; agent profiles matched by `"namespace:agent"` glob patterns
- **Understanding ClaWHub skill loading**: `internal/executive/extensions.go` — `skillManifestEntry` parsing, `clawhubDownloadBase`, cache at `~/Library/Caches/bud/skills-clawhub/`
- **Adding a new sense/integration**: `internal/senses/` — create a new file following the Discord pattern, wire in `cmd/bud/main.go`
- **Understanding memory retrieval**: `internal/engram/client.go` (HTTP client) + Engram service repo for the storage side; quality feedback via `RateEngrams()`
- **Running locally**: `./scripts/build.sh` then `launchctl kickstart -k gui/$(id -u)/com.bud.daemon` (macOS) or `systemctl --user restart bud.service` (Linux); logs at `~/Library/Logs/bud.log` (macOS) or journalctl (Linux); state at `~/Documents/bud-state`
- **Understanding the overlay/defaults system**: `internal/paths/paths.go` — `ResolveFile` and `MergeDir` are the two entry points; `state-defaults/` provides shipping defaults without file copying
