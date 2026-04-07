# Autopilot Planning Cascade

## Pre-flight questions (before dispatching autopilot-vision:planner)

When a user explicitly asks to run the planning cascade on a project, do NOT immediately spawn the vision planner. First collect orienting context:

1. Call `talk_to_user` with 2–3 questions:
   - "Who is the primary audience for [project]?"
   - "What's the primary goal — [options based on context]?"
   - "Any hard constraints I should factor in? (timeline, scope, out-of-scope areas)"
2. Save a thought tagged `["autopilot", "preflight", "pending"]` with the project context (path, name, what the user said).
3. Call `signal_done`.

On the next message, check for a recent `["autopilot", "preflight", "pending"]` thought. Combine the user's answers as a `Seed direction from owner:` block in the vision planner context, then spawn `autopilot-vision:planner`.

Skip pre-flight if the user explicitly says "skip questions", "just run it", or provides a seed direction inline.

## Gate response handling

When woken by a user message, check for recent `save_thought` entries tagged `["autopilot", "gate", "pending"]` from within the last hour. If one exists, treat the user's message as a gate response:

- **"yes" / "proceed" / "ok" / "looks good"** → extract saved `next_agent` and context, spawn via `Agent_spawn_async`, do NOT call `signal_done`
- **"no" / "stop" / "halt"** → acknowledge, call `signal_done`, cascade ended
- **"adjust: [feedback]"** → append `Owner feedback: [feedback]` to saved context, spawn next agent with it, do NOT call `signal_done`
- **Ambiguous** → ask for clarification before acting

If no pending gate exists (or it's older than 1 hour), treat the message normally.
