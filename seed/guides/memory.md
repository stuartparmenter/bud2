# Memory Guide

How to decide where to save information so it's actually findable later.

## Storage Systems

| System | Role | Access |
|--------|------|--------|
| **Engram** | Passive ambient recall — surfaces relevant context without explicit query | Automatic injection; write via `save_thought` |
| **Zettels** | Primary knowledge store — atomic, linked, human-browsable ideas and findings | Explicit: `zettel-search`, `zettel-new`, `zettel-convert` |
| **GK** | Autopilot planning only — structured entity/relationship graph for planning cycles | Explicit: `gk_*` tools; default domain `/` |
| **Notes/Guides** | Documents that must be read as a whole — multi-step guides, blog drafts, active plans, sprint workflows | Read/Edit directly |
| **Things** | Task queue only — not for knowledge | `gtd_*` / Things MCP |

**Engram is for passive influence, not reliable recall.** Don't use it to store facts you'll need to look up — retrieval is probabilistic and rated ~2/5 quality. Use zettels for that.

**GK is scoped to autopilot.** Don't use it for general knowledge storage. The relationship graph model fits planning data (epics → tasks, strategies → bets) but adds unnecessary overhead for everything else. Future: vector search on zettels will close the remaining gap.

## Decision Tree

**Saving a passing observation, reasoning trace, or behavioral note?**
→ `save_thought` → Engram. Good for things you want to *influence future context* without explicit retrieval.

**Discovered a concept, insight, pattern, or research finding worth preserving?**
→ `zettel-new` — creates an atomic note in `state/zettels/`. Run `zettel-search` first to avoid duplicates. This is the **default for new knowledge**.

**Learning something specific about a project (gotcha, decision, design context)?**
→ Write to `state/projects/<project>/notes.md`. Canonical scratchpad for per-project context.

**Discovering a general process, workflow, or convention that applies across projects?**
→ Update the relevant guide in `state/system/guides/`. If none fits, ask the owner whether a new guide is warranted.

**Discovering a surprising gotcha or system quirk (not guide-worthy yet)?**
→ Append to `state/notes/learnings.md` with date and context. Review periodically to promote into guides or code fixes.

**Writing something that must stay coherent as a whole (multi-step guide, blog draft, active plan)?**
→ `state/notes/` or `state/projects/<project>/`. Notes are for documents, not atoms.

**Writing longer research or design context to atomize later?**
→ Write to `state/notes/` first, then run `zettel-convert` before the session ends. Don't let source notes age unconverted.

**Deciding between zettel and note for an operational reference?**
→ If it's one focused concept and gains value from being linked to other ideas → zettel. If it only makes sense read start-to-finish → note. Zettels can describe current implementation; bitrot is a maintenance concern, not a categorization reason to avoid them.

**Something that must be recalled at the start of every session without retrieval?**
→ **Ask the owner first.** If approved, add to `seed/core.md` AND `state/system/core.md`. Deploy after. High bar — core.md is the always-loaded system prompt.

**Working in an autopilot planning cycle?**
→ Use `gk_*` tools with the appropriate domain. See `autopilot.md` for the full planning flow.

## What NOT to Do

- Don't use Engram to store facts you'll need to look up — retrieval is probabilistic, not guaranteed.
- Don't use GK for general knowledge. It's for autopilot planning data only.
- Don't add facts to `MEMORY.md`. That file is a Claude Code artifact — it doesn't integrate with Bud's state system.
- Don't write ephemeral task details to any persistent file. Use Things for in-progress work.
- Don't duplicate content across multiple locations. Pick one canonical home.

## Memory Self-Eval (signal_done)

When calling `signal_done`, include `memory_eval` ratings for memories recalled during the session. The goal is to improve retrieval, not to be modest or generous.

| Rating | Meaning |
|--------|---------|
| 5 | Directly used — changed my approach or decision |
| 4 | Confirmed context I was already working from |
| 3 | Provided relevant background |
| 2 | Retrieved but didn't influence the work |
| 1 | Actively misleading or total noise |

**Calibration note (2026-02-22):** I was rating almost everything 1. External judge averaged 2.80. Greeting/social/operational traces that provide interaction pattern context → 3, not 1.
