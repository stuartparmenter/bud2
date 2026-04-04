---
name: zettel-lint
description: "Periodic health check of the zettel corpus. Trigger: 'lint zettels', 'check zettel health', 'find orphaned zettels', 'zettel maintenance'."
user-invocable: true
---

# zettel-lint

Audit the zettel corpus for structural problems. Run periodically (every few weeks) to prevent rot.

## Checks

Run all five checks against `state/zettel/*.md`:

### 1. Orphaned zettels
A zettel with no incoming links — nothing in the corpus points to it. Likely too isolated to contribute to the knowledge web.

```
grep -rL "<id>" state/zettel/  # for each zettel, check if its id appears in any other zettel's links: or body
```

Report: list of orphaned zettel ids + titles.
Action: for each orphan, suggest either linking it into an existing cluster or reviewing whether it should exist.

### 2. Missing cross-references
Pairs of zettels that share 2+ tags but are not linked to each other. Likely related but overlooked.

For each pair with tag overlap ≥ 2: check if either links to the other.

Report: list of unlinked pairs with shared tags.
Action: prompt to run `zettel-link` on flagged pairs.

### 3. Stale sources
Zettels with a `source:` field pointing to a file in `state/notes/` that no longer exists (moved, renamed, deleted).

```
for each zettel with source: field, check if state/<source> exists
```

Report: list of zettels with broken source paths.
Action: update source path or clear it if provenance is no longer recoverable.

### 4. Tag drift
Tags that appear in only one zettel. Likely a one-off that should be normalized to an existing tag or dropped.

```
extract all tags, count frequency, flag count == 1
```

Report: singleton tags + the zettel that uses them.
Action: suggest merging with a similar existing tag or removing if the tag adds no value.

### 5. Tag clusters with no MOC

Tags that appear in 5+ zettels but have no corresponding `moc-<tag>.md` file. These are dense clusters that have outgrown ad-hoc linking and warrant a Map of Content.

```
for each tag with count >= 5: check if state/zettel/moc-<tag>.md exists
```

Report: list of tag clusters missing a MOC, with zettel count.
Action: suggest running `zettel-index <tag>` for each flagged cluster.

## Output format

```
Zettel Lint Report — YYYY-MM-DD
================================
Corpus: N zettels

ORPHANS (no incoming links): N
- 20240312-slug — Title of orphaned zettel
  → Suggestion: link from <related-zettel-id>

MISSING CROSS-REFS: N pairs
- 20240312-slug-a ↔ 20240315-slug-b (shared tags: tag1, tag2)
  → Run: zettel-link

STALE SOURCES: N
- 20240318-slug — source: notes/missing-file.md (file not found)

TAG DRIFT (singleton tags): N
- "sparse-tag" used only in 20240320-slug — consider merging with "similar-tag"

MOC GAPS (dense tag clusters with no MOC): N
- "memory-retrieval" — 8 zettels, no moc-memory-retrieval.md → run: zettel-index memory-retrieval

All clear: No issues found.
```

## Cadence

Run after bulk conversions and then every 2–4 weeks. Not required before every `zettel-new` — that's `zettel-search`'s job.
