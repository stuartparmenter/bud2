---
generated_at: 2026-04-07T13:11:02Z
commit: 12cd10a5
repomix: available
---

# bud2 — Overview

> Generated: 2026-04-07 | Commit: 12cd10a5

## Purpose

Bud is a personal AI agent that runs as a macOS launchd daemon, providing autonomous assistance, long-term memory, and task management through a Discord interface. Built in Go, it wraps the Claude Agent SDK to give Claude an event loop, attention system, and a rich set of MCP tools for interacting with external services and its own state.

## Data Flow

External signals arrive through senses (`internal/senses/discord.go` for Discord messages, `internal/senses/calendar.go` for calendar events). Each sense creates a **percept** in the `PerceptPool` (`internal/memory/percepts.go`) and enqueues a **focus item** into `focus.Queue` (`internal/focus/queue.go`). The `focus.Attention` system (`internal/focus/attention.go`) computes salience (priority × source boost × recency) and selects the highest-priority item. Before reaching the executive, the **reflex engine** (`internal/reflex/engine.go`) evaluates YAML rules from `seed/reflexes/` — simple queries are answered directly via Discord without waking Claude.

For items that pass through reflexes, `ExecutiveV2` (`internal/executive/executive_v2.go`) opens a Claude session via the Claude Agent SDK, injects context assembled from focus state, recent memories (fetched from Engram via `internal/engram/client.go`), and seed instructions. Agent definitions and skills are reloaded from plugins on every prompt — changes to plugin configs take effect without a daemon restart. Claude calls MCP tools registered in `internal/mcp/tools/register.go` (served by `internal/mcp/server.go` on port 8066, bound synchronously at startup). Responses flow back through `internal/effectors/discord.go`. Autonomous wakes fire on a configurable timer (default 2h) and follow the same path, capped by `MaxAutonomousSessionDuration`; when no queued work exists, an idle fallback (doc-maintain) runs via a background subagent.

## Module Map

| Path | Responsibility |
|------|---------------|
| `cmd/bud/main.go` | Daemon entry point — wires all subsystems, handles config, launchd lifecycle, plugin manifest loading |
| `internal/executive/executive_v2.go` | Core orchestrator — runs Claude sessions, manages signal_done, subagents, per-prompt agent/skill reload |
| `internal/executive/agent_defs.go` | Agent definition loading from plugins; applies tool grants from plugins.yaml |
| `internal/focus/` | Attention system — salience computation, priority queue, focus/suspend/resume |
| `internal/senses/` | Input adapters — Discord and calendar event ingestion → percepts |
| `internal/effectors/` | Output adapters — Discord message sending and reactions |
| `internal/reflex/` | YAML-defined reflexes — fast-path responses without invoking Claude |
| `internal/mcp/` | MCP HTTP server and all tool registrations (GK, calendar, GitHub, VM, etc.) |
| `internal/memory/` | Short-term working memory — percept pool, threads, traces, inbox |
| `internal/engram/` | HTTP client to the Engram memory service (long-term graph memory) |
| `internal/types/` | Shared type definitions — most-imported package (centrality 33) |
| `internal/integrations/` | External integration helpers (calendar, GitHub; centrality 16) |
| `internal/budget/` | Token/thinking-time budget tracking across sessions |
| `internal/gtd/` | Local GTD task store (JSON-backed; Things 3 integration via things-mcp MCP server) |
| `seed/` | Core bundled plugins (`bud`, `bud-ops`), guides, reflexes, wakeup instructions |
| `state/system/plugins.yaml` | External plugin manifest — lists GitHub repos to clone and load at startup |
| `state/system/skill-grants.yaml` | Centralized agent→skill grants — controls which skills each agent type can invoke |

## Key Files

- `cmd/bud/main.go` — wires all subsystems together; start here to understand initialization order
- `internal/executive/executive_v2.go` — `ExecutiveV2` struct: session lifecycle, context assembly, subagent management
- `internal/executive/agent_defs.go` — plugin-aware agent definition loading; applies `tool_grants` from plugins.yaml
- `internal/focus/attention.go` — salience computation and focus selection logic
- `internal/mcp/tools/register.go` — all MCP tool definitions; largest file, entry point for any tool work
- `internal/reflex/engine.go` — YAML reflex loading, evaluation, and action dispatch
- `state/system/plugins.yaml` — declares external plugins (GitHub repos) loaded at startup with optional tool_grants
- `state/system/skill-grants.yaml` — centralized skill grant rules for all agent profiles (replaces per-agent `skills:` fields)

## Conventions

- **Testing**: Co-located `*_test.go` files. Go standard testing; run with `go test ./...`. Integration tests in `tests/scenarios/` (YAML-defined). See `docs/testing-playbook.md`.
- **Naming**: Go idioms throughout. Internal packages under `internal/`. Tool names in MCP use snake_case (e.g. `talk_to_user`, `signal_done`).
- **Entry points**: Single binary `bin/bud` built by `scripts/build.sh`. State server `bin/bud-state` is a separate MCP-only binary. `things-mcp` is a TypeScript server built separately.
- **Patterns to know**: Core plugins (`bud`, `bud-ops`) live in `seed/plugins/` and are always bundled. External plugins (useful-plugins, autopilot, etc.) are declared in `state/system/plugins.yaml` — Bud clones/updates them at startup from GitHub into a local cache. `seed/` files are copied to `state/system/` on first run; edits to `seed/` don't take effect until redeployment or manual copy. Agent definitions and skills hot-reload from plugins on every prompt without restart. The `state/` directory is Bud's working directory and is a separate git repo (`bud2/state`).

## Start Here

For a given task type, start at:
- **Adding a new MCP tool**: `internal/mcp/tools/register.go` — all tools are registered here; follow the pattern of an existing tool
- **Modifying reflex behavior**: `seed/reflexes/*.yaml` — YAML rules evaluated by `internal/reflex/engine.go`; hot-reload on change
- **Changing how Claude is prompted**: `internal/executive/executive_v2.go` around `buildPrompt` — context assembly is here; also check `seed/core.md` and `seed/wakeup.md`
- **Adding/configuring a plugin**: `state/system/plugins.yaml` — add a GitHub repo entry; use `tool_grants` to control which MCP tools the plugin's agents can call
- **Changing agent skill access**: `state/system/skill-grants.yaml` — centralized grants file; agent profiles matched by `"namespace:agent"` glob patterns
- **Adding a new sense/integration**: `internal/senses/` — create a new file following the Discord pattern, wire in `cmd/bud/main.go`
- **Understanding memory retrieval**: `internal/engram/client.go` (HTTP client) + Engram service repo for the storage side
- **Running locally**: `./scripts/build.sh` then `launchctl kickstart -k gui/501/com.bud.daemon`; logs at `~/Library/Logs/bud.log`
