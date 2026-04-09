---
topic: Memory Quality Feedback Loop
repo: bud2
generated_at: 2026-04-09T03:00:00Z
commit: eb51a863
key_modules: [internal/executive, internal/engram]
score: 0.87
---

# Memory Quality Feedback Loop

> Repo: `bud2` | Generated: 2026-04-09 | Commit: eb51a863

## Summary

After each Claude session ends, bud2 extracts per-memory quality ratings from Claude's output and sends them back to the Engram memory service via `RateEngrams()`. These ratings close the feedback loop: Engram uses them to adjust memory quality scores that influence future semantic retrieval ranking, so frequently-shown but low-quality memories gradually surface less often while high-signal memories are reinforced. The mechanism also handles a separate path for subagent structured observations, which are auto-ingested to Engram when a subagent completes.

## Key Data Structures

### `SimpleSession.memoryIDMap` (`internal/executive/simple_session.go`)
Maps `trace_id → display_id` for memories shown in the current turn. Display IDs are the first 5 chars of the real Engram trace ID (e.g., `a3f9c`). Cleared by `PrepareForResume()` at the start of each turn so display IDs are fresh and the self-eval block only references memories injected in this specific turn. Used to build the reverse lookup in `ResolveMemoryEval`.

### `SimpleSession.seenMemoryIDs` (`internal/executive/simple_session.go`)
Set of full trace IDs that have been sent to Claude in this session. Prevents the same memory from being re-injected on subsequent turns of a resumed session. Preserved by `PrepareForResume()`; only cleared by `Reset()` (full memory reset). **Not** cleared by `PrepareNewSession()` (context-limit flush).

### `Trace` (`internal/engram/client.go`)
A consolidated memory (Tier 3). Key fields:
- `ID string` — full trace ID hash; first 5 chars used as display ID
- `Summary string` — the recalled memory text injected into the prompt
- `Level int` — compression level applied (0 = stored summary)
- `SchemaIDs []string` — schema annotations; used to surface `ActiveSchemas` in context

### `RetrievalResult` (`internal/engram/client.go`)
Output of `Search()`. Fields: `Traces []*Trace`, `Episodes []*Episode`, `Entities []*Entity`. For semantic memory search, only `Traces` is populated; `Episodes` and `Entities` are empty.

### `AgentOutput` (`internal/executive/executive_v2.go`)
Structured JSON schema optionally emitted by subagents at the end of their response. Fields:
- `Observations []AgentObservation` — auto-posted to Engram as thoughts when `watchSubagentDone()` processes the result
- `Next *AgentNext` — signals the recommended follow-up action to the executive
- `Principles []PrincipleEntry` — auto-stored to Engram with tag `"principle"`

### `AgentObservation` (`internal/executive/executive_v2.go`)
Single observation from a subagent. Fields: `Content`, `Source`, `Confidence`, `Strategic bool`. The `Strategic` flag is informational; all observations are ingested to Engram regardless.

## Lifecycle

### Path A — Session memory rating

1. **Memory retrieval** (`buildContext`, `executive_v2.go`): For user messages, `e.memory.Search(focusText, limit, level)` retrieves relevant traces from Engram. Autonomous wakes **skip this step entirely** — analysis showed 48% of wake memories rated 1/5, pulling retrieval precision to 29.6%.

2. **Display ID assignment** (`buildContext` → `session.GetOrAssignMemoryID`, `simple_session.go`): Each retrieved `Trace` gets a display ID assigned via the first 5 chars of its real trace ID. The mapping is stored in `session.memoryIDMap`. `seenMemoryIDs` is checked — already-seen memories are filtered out before injection.

3. **Activation boost** (`buildContext`, `executive_v2.go`): After retrieval, `e.memory.BoostTraces(traceIDs, boost, threshold)` is called for the shown memories. This happens at retrieval time, regardless of how Claude rates them later.

4. **Schema surfacing** (`buildContext`, `executive_v2.go`): Schema IDs from retrieved traces are counted by frequency. Top-5 by frequency are fetched via `memory.SearchSchemas()` and included in the prompt as `ActiveSchemas`.

5. **Prompt injection** (`buildPrompt`, `executive_v2.go`): Recalled memories are injected into the prompt with the format `[a3f9c] [timestamp] summary`. When memories are present, a memory self-eval instruction is appended: Claude is asked to output a `<memory_eval>{"a3f9c": 5, "b2e1d": 1}</memory_eval>` block rating each shown memory 1–5.

6. **Claude responds with ratings** (Claude subprocess): Claude emits memory ratings embedded in its text output inside `<memory_eval>...</memory_eval>` XML tags. This is a soft instruction — Claude may omit it (e.g., if signal_done fires mid-output).

7. **Rating extraction** (`extractMemoryEval`, `executive_v2.go`): After the session completes, `extractMemoryEval(output string) string` scans the full text output for the last `<memory_eval>...</memory_eval>` block. Returns the inner JSON string, or empty if absent.

8. **Rating resolution** (`session.ResolveMemoryEval`, `simple_session.go`): Takes the extracted JSON map (display_id → rating). Builds a reverse map from `memoryIDMap` (display_id → trace_id). Returns `map[string]int` (trace_id → rating). Legacy `M1`/`M2` format display IDs are silently skipped — they cannot be resolved with the new ID scheme.

9. **Ratings sent to Engram** (`e.memory.RateEngrams`, `engram/client.go`): `RateEngrams(ratings map[string]int)` posts the resolved ratings to Engram. Engram updates internal quality scores for each trace, which feed into future retrieval ranking. Zero-length ratings map is a no-op.

### Path B — Subagent observation auto-ingestion

1. **Subagent completes** (`watchSubagentDone`, `executive_v2.go`): The goroutine watching `SubagentManager.DoneNotify` receives a completed session result string.

2. **Parse agent output** (`parseAgentOutput`, `executive_v2.go`): Attempts to extract a `AgentOutput` JSON block from the result — first looking for a `\`\`\`json` fence, then falling back to the last bare `{...}` block.

3. **Auto-ingest observations**: For each `AgentObservation`, `e.memory.IngestThought(obs.Content)` is called directly — bypassing the rating mechanism entirely. `PrincipleEntry` items are similarly ingested with a `"principle"` tag (likely via the same path; exact implementation inferred from field presence).

4. **P3 focus item injected**: A P3 priority item is added to the queue so the executive wakes to review subagent results and approve staged memories from `save_thought` calls.

## Design Decisions

- **Display IDs are trace ID prefixes, not sequential counters**: The first 5 chars of the Engram trace ID serve as display IDs. This lets Claude query `GET /v1/engrams/<prefix>` directly when it needs full trace context. The prior sequential M1/M2 scheme was replaced; legacy IDs are gracefully skipped in `ResolveMemoryEval`.

- **Wake sessions skip memory retrieval entirely**: Hardcoded in `buildContext`. The data-driven comment: "analysis shows 48% of wake memories rated 1/5, dragging precision down to 29.6%. Wakes use generic prompts that pull irrelevant memories." This is an explicit quality-over-completeness tradeoff.

- **seenMemoryIDs outlives context-limit resets**: `PrepareNewSession` (context flush) does not clear `seenMemoryIDs`. Only `Reset()` (explicit `memory_reset` tool call) does. Rationale: if a memory was low-quality enough to appear once without being rated, it shouldn't be re-shown just because the context was flushed.

- **memoryIDMap cleared per turn**: `PrepareForResume()` clears `memoryIDMap` even though it preserves `seenMemoryIDs`. A fresh display-ID namespace per turn ensures the memory_eval block only references memories from the current turn — not from a prior turn in the same session whose IDs are no longer in Claude's active context.

- **Activation boost precedes rating**: `BoostTraces()` is called at retrieval time, before Claude rates the memories. Low-rated memories still receive the retrieval-time boost; only future retrieval probability is affected by the rating. This is intentional: the boost keeps traces alive long enough to be evaluated, rather than letting them decay before Claude can assess them.

## Integration Points

| From | To | What crosses the boundary |
|------|----|--------------------------|
| `internal/executive` | `internal/engram` | `Search()` for retrieval; `BoostTraces()` after retrieval; `RateEngrams()` after session; `IngestThought()` for subagent observations |
| `internal/executive` | Engram HTTP service | All calls are HTTP via `internal/engram/client.go` (baseURL from config) |
| `internal/executive/simple_session.go` | `internal/executive/executive_v2.go` | `ResolveMemoryEval()` resolves display IDs to trace IDs; `GetOrAssignMemoryID()` assigns display IDs during context assembly |
| `internal/executive` | `internal/focus` | Focus item text drives the semantic query sent to `Search()`; `ContextBundle.Memories` carries retrieved traces to `buildPrompt` |

## Non-Obvious Behaviors

- **Wake sessions have no memory context at all**: The `buildContext` skip is unconditional for wake-type focus items — not just reduced retrieval, but zero retrieval. Engineers adding wake-specific context assembly should note this branch.

- **Display IDs are directly queryable**: `a3f9c` in a memory_eval is not an opaque label — it's a valid prefix for `GET /v1/engrams/a3f9c` on the Engram service. Claude can and does use this to fetch full trace context mid-session.

- **No-op on missing memory_eval**: If Claude doesn't emit a `<memory_eval>` block (e.g., signal_done fires early, or Claude simply skips it), `extractMemoryEval` returns `""`, `ResolveMemoryEval` returns an empty map, and `RateEngrams` is not called. No error, no log warning — the omission is silently accepted.

- **BoostTraces fires even for memories Claude will rate 1/5**: The boost happens at context assembly time, not after ratings are received. A memory shown this turn gets a survival boost regardless of its eventual rating. The rating affects the *next* retrieval, not this one.

- **seenMemoryIDs never resets across context flushes**: `PrepareNewSession` (triggered when `ShouldReset()` is true — context > 150K tokens) preserves `seenMemoryIDs`. After 50+ turns in a long session, the seen set can grow large and filter out many potentially relevant memories. Only `Reset()` via `memory_reset` clears it.

- **Subagent observations bypass the rating system**: `AgentObservation` items from `watchSubagentDone` are ingested as thoughts directly — they don't go through the 1–5 rating loop. Their quality is implied by the `Confidence` field but that field is metadata only, not used to gate ingestion.

## Start Here

- `internal/executive/executive_v2.go` — `processItem()` (post-session block, lines after `sendErr`): where `extractMemoryEval` and `RateEngrams` are called; `buildContext()`: where retrieval happens and wakes skip it; `watchSubagentDone()`: subagent observation auto-ingestion path
- `internal/executive/simple_session.go` — `GetOrAssignMemoryID()`, `ResolveMemoryEval()`, `PrepareForResume()`: the three functions that implement session-level memory tracking and ID resolution
- `internal/engram/client.go` — `RateEngrams()`, `Search()`, `BoostTraces()`: the three Engram calls in the feedback loop; also `IngestThought()` for the subagent path
- `internal/executive/executive_v2.go` — `extractMemoryEval()` and `buildPrompt()` (memory eval instruction section): how ratings are extracted and how Claude is instructed to produce them
