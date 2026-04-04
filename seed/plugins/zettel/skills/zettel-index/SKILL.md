---
name: zettel-index
description: "Build or rebuild a Map of Content (MOC) for a topic tag. Trigger: 'build zettel index', 'create MOC', 'map of content for', 'index zettels by tag', 'zettel-index'."
user-invocable: true
---

# zettel-index

Build or rebuild a Map of Content (MOC) for a given tag. Overwrites any existing MOC for that tag.

## Input

A tag name (e.g., `memory`, `act-r`, `spaced-repetition`).

## Steps

1. **Find all matching zettels.** Grep `state/zettel/*.md` for `tags:` lines containing the given tag. Collect matching file paths.

2. **Read each zettel.** Extract:
   - `id`
   - `title`
   - `tags` (full list, to find sub-tags)
   - First sentence of the body

3. **Group by sub-tag.** If zettels share a secondary tag (beyond the query tag), group them under that sub-tag as a section. Ungrouped zettels go under "General".

4. **Write MOC** to `state/zettel/moc-<tag>.md`:

```markdown
# MOC: <tag>

*Generated: YYYY-MM-DD. <N> zettels.*

## <Sub-tag or "General">

- [[<id>]] **<title>** — <one-line summary>
- [[<id>]] **<title>** — <one-line summary>

## <Sub-tag>

- [[<id>]] **<title>** — <one-line summary>
```

5. Confirm: "MOC written to `state/zettel/moc-<tag>.md` — <N> zettels indexed."

## Notes

- If zero zettels match the tag, report that and do not write a file.
- MOC files are regenerated on demand — they are not manually edited.
- The one-line summary is the first sentence of the body, truncated to ~100 chars.
