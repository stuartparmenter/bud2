# Bud GK-Conventions Additions

## Guide Access

Use `mcp__bud2__read_resource` with `gk://guides/{name}` URIs:

```
mcp__bud2__read_resource(uri="gk://guides/query")
mcp__bud2__read_resource(uri="gk://guides/extraction")
mcp__bud2__read_resource(uri="gk://guides/pyramid")
mcp__bud2__read_resource(uri="gk://guides/review")
```

## Storing and Reading Cycle Data

Use the `mcp__bud2__gk_*` tools directly. GK is a live knowledge graph — use it as described in the guides.

- **Read prior data:** `mcp__bud2__gk_search`, `mcp__bud2__gk_search_keyword`, `mcp__bud2__gk_get_entity`
- **Store results:** `mcp__bud2__gk_add_entities`, `mcp__bud2__gk_add_observations`, `mcp__bud2__gk_add_relationships`
- **Validate:** `mcp__bud2__gk_validate_graph`

Do NOT use `search_memory` or `save_thought` — those are Engram tools for personal episodic memory, not the project knowledge graph.
