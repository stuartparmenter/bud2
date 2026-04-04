# Memory Guide

How to decide where to save information so it's actually findable later.

## Decision Tree

**Saving a passing observation or reasoning trace?**
→ `save_thought` tool — gets ingested into Engram and consolidated over time.

**Learning something specific about a project (gotcha, decision, design context)?**
→ Write it to `state/projects/<project>/notes.md`. This is the canonical scratchpad for per-project context.

**Discovering a general process, workflow, or convention that applies across projects?**
→ Find the relevant guide in `state/system/guides/` (or `seed/guides/` for seeded content) and update it. If no guide fits, ask the owner whether a new guide is warranted.

**Discovering a surprising gotcha or system quirk (not a full guide-worthy pattern yet)?**
→ Append to `state/notes/learnings.md` with date and context. Review periodically to promote into guides or code fixes.

**Something that must be recalled at the start of every session without any retrieval?**
→ **Ask the owner first.** If approved, add to `seed/core.md` AND `state/system/core.md`. Deploy after. This is a high bar — core.md is the always-loaded system prompt; cluttering it degrades focus.

## What NOT to Do

- Don't add facts to `MEMORY.md`. That file is a Claude Code artifact — it doesn't integrate with Engram or Bud's state system. Use the paths above instead.
- Don't write ephemeral task details to any persistent file. Use the task queue (Things Bud area) for in-progress work.
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
