# Doc Plan: bud2 — 2026-04-14

Scoring: centrality (0.30) + coverage gap (0.30) + complexity (0.20) + churn (0.10) + bug density (0.10)
Topics span modules — signals are the max across constituent modules.

| Rank | Topic | Score | Key Modules | Signals | Status |
|------|-------|-------|-------------|---------|--------|
| 1 | State-Defaults Overlay & Configuration Loading | 0.82 | `internal/paths`, `internal/config`, `cmd/bud` | no doc, centrality 8+ (paths imported by executive/reflex/main), 107 commits/90d (cmd/bud), replaces seed-copy model. (foundational) | generated |
| 2 | Session Lifecycle & Context Assembly | 0.80 | `internal/executive`, `internal/types` | centrality 19 (types), 124 commits/90d, 53 fix-commits; memory limit 6, startup path. (foundational). Source: `session-lifecycle-context-assembly.md` | generated |
| 3 | Subagent Orchestration | 0.80 | `internal/executive`, `internal/types`, `internal/effectors` | centrality 19, 124 commits/90d, 53 fix-commits; startup subagent restart-notes pattern. (foundational). Source: `subagent-orchestration.md` | generated |
| 4 | Startup Lifecycle & Context Injection | 0.78 | `cmd/bud`, `internal/executive`, `state-defaults/system/startup-instructions.md` | no stale doc, 107 commits/90d, 45 fix-commits; now reads via paths overlay. (foundational). Source: `startup-lifecycle-context-injection.md` | generated |
| 5 | Memory Quality Feedback Loop | 0.76 | `internal/executive`, `internal/engram` | centrality 8 (engram), 124 commits/90d, 53 fix-commits; RateEngrams() after signal_done. (foundational). Source: `memory-quality-feedback-loop.md` | generated |
| 6 | MCP Tool Dispatch & Registration | 0.58 | `internal/mcp`, `internal/types` | centrality 19 (via types), complexity max (5498 LoC, 9 files), 62 commits/90d. Source: `mcp-tool-dispatch-registration.md` | generated |
| 7 | Plugin Manifest Runtime & Tool Grants | 0.55 | `cmd/bud`, `internal/executive`, `state/system/extensions.yaml` | 105 commits/90d, 43 fix-commits; now covers `plugins:` + `skills:` sections, ClaWHub registry, git/local standalone skill loading via `extensions.go`. (foundational). Source: `plugin-manifest-runtime-tool-grants.md` | generated |
| 8 | Wake Scheduling & Autonomous Sessions | 0.55 | `internal/executive`, `internal/focus`, `internal/budget` | 124 commits/90d, 53 fix-commits; idle-fallback and startup handling. (foundational). Source: `wake-scheduling-autonomous-sessions.md` | generated |
| 9 | Zettel Library Discovery & Generation | 0.45 | `internal/executive/simple_session.go`, `cmd/bud` | 107 commits/90d; merge semantics (manual entries preserved). Source: `zettel-library-discovery-generation.md` | generated |
| 10 | Token Budget & Session Caps | 0.45 | `internal/budget`, `internal/executive` | centrality 6, complexity max (7143 LoC executive), 124 commits/90d. Source: `token-budget-session-caps.md` | generated |
| 11 | Skill Grants & Agent Composition | 0.45 | `internal/executive/agent_defs.go`, `internal/executive/profiles.go`, `state/system/skill-grants.yaml` | 124 commits/90d; exclude-list applied during manifest load. Source: `skill-grants-agent-composition.md` | generated |
| 12 | Reflex Evaluation Pipeline | 0.44 | `internal/reflex`, `internal/senses`, `internal/types` | centrality 19, 2971 LoC, 11 commits/90d. Source: `reflex-evaluation-pipeline.md` | generated |
| 13 | Attention & Salience Computation | 0.37 | `internal/focus`, `internal/types` | centrality 19 (via types), 18 commits/90d. Source: `attention-salience-computation.md` | generated |

## Recommended next

All topics generated. Run `dev:repo-doc bud2` to refresh the overview and doc-plan when significant new features land. Note: `workflow.go` and `reflex/log.go` dead subsystems removed since last cycle; `extensions.go` now handles standalone skill loading (ClaWHub/git/local) — covered in `plugin-manifest-runtime-tool-grants.md`.

---
_Generated: 2026-04-14T06:25:00Z | Commit: 31ceb932_
