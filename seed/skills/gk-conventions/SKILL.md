---
name: gk-conventions
description: >-
  Graph knowledge store conventions for autopilot planning agents. Teaches
  agents to read GK guides, read prior cycle data from the knowledge graph,
  and store directions, observations, and predictions after planning.
---

# GK Conventions

The knowledge graph (GK) is a live graph database. Use it to read guides, retrieve prior cycle data, and store planning outputs.

## Guide Access

Always read the relevant guides before acting. Use `mcp__bud2__read_resource`:

```
mcp__bud2__read_resource(uri="gk://guides/query")       # search patterns
mcp__bud2__read_resource(uri="gk://guides/extraction")  # entity/observation conventions
```

## Reading Prior Cycle Data

Before dispatching sub-agents or running /planning, search GK for prior cycle data. This prevents re-exploring known ground and surfaces prior decisions.

```
mcp__bud2__gk_search(query="vision direction")       # prior vision cycles
mcp__bud2__gk_search(query="strategy direction")     # prior strategy cycles
mcp__bud2__gk_search(query="epic planning")          # prior epic cycles
mcp__bud2__gk_search(query="task decomposition")     # prior task cycles
mcp__bud2__gk_search(query="autopilot observations") # cross-cycle observations
```

Read the results before acting. Prior directions inform what to explore and what diversity axes to use.

### What to look for

- **Directions**: Previously selected vision/strategy/epic/task directions
- **Rationale**: Why a prior direction was selected
- **Observations**: Findings from prior explorer/researcher sub-agents
- **Predictions**: Prior predictions that can now be verified
- **Blockers**: Prior cycles that stalled or signaled DOWN/STAY

## Storing Results

After /planning completes, store results in GK so the next cycle can build on them.

### Save directions

Use `mcp__bud2__gk_add_entities` to create a direction entity, then `mcp__bud2__gk_add_observations` to attach content:

```
mcp__bud2__gk_add_entities(entities=[{
  "name": "Task Direction: <title>",
  "type": "PlanningDirection",
  "confidence": 0.9,
  "metadata": {"level": "task", "cycle": "<date>"}
}])

mcp__bud2__gk_add_observations(observations=[{
  "entity_name": "Task Direction: <title>",
  "content": "Selected approach: <description>\nRationale: <rationale>",
  "metadata": {"level": "overview"}
}])
```

Use the appropriate type prefix: `Vision Direction`, `Strategy Direction`, `Epic Direction`, `Task Direction`.

### Save observations

```
mcp__bud2__gk_add_entities(entities=[{"name": "Explorer Finding: <topic>", "type": "Observation"}])
mcp__bud2__gk_add_observations(observations=[{
  "entity_name": "Explorer Finding: <topic>",
  "content": "<finding>\nSource: <file>\nConfidence: high/medium/low",
  "metadata": {"level": "detail"}
}])
```

### Link hierarchy

Use `mcp__bud2__gk_add_relationships` to link strategy to vision, epic to strategy, task to epic:

```
mcp__bud2__gk_add_relationships(relationships=[{
  "from_name": "Task Direction: <title>",
  "to_name": "Epic Direction: <parent title>",
  "type": "child_of"
}])
```

## Validate Before Completing

After storing results, run `mcp__bud2__gk_validate_graph` and fix any issues before signaling done.
