---
name: zettel-index
description: "Build or rebuild a Map of Content (MOC) for a topic tag. Trigger: 'build zettel index', 'create MOC', 'map of content for', 'index zettels by tag', 'zettel-index'."
user-invocable: true
---

# zettel-index

Build an annotated Map of Content (MOC) for a given tag. A MOC is an *authored* zettel — not a generated table of contents. Each link carries a brief annotation explaining its role in the cluster. This is what makes it useful for navigation vs. being a flat dump.

## Input

A tag name (e.g., `memory`, `act-r`, `spaced-repetition`).

## Steps

1. **Find all matching zettels.** Grep `state/zettel/*.md` for `tags:` lines containing the given tag. Collect matching file paths. If zero match, report and stop.

2. **Read each zettel.** Extract:
   - `id`, `title`, `tags` (full list), `links`
   - First sentence of the body (for context)

3. **Group by sub-tag.** If zettels share a secondary tag (beyond the query tag), group them under that sub-tag as a section. Ungrouped zettels go under "General".

4. **Write annotated MOC** to `state/zettel/moc-<tag>.md`:

```markdown
---
id: moc-<tag>
title: MOC: <tag>
tags: [moc, <tag>]
links: []
created: YYYY-MM-DD
---

A Map of Content for zettels tagged `<tag>`. Updated: YYYY-MM-DD. <N> zettels.

## <Sub-tag or "General">

- [[<id>]] **<title>** — <one-line annotation: why this matters here, what role it plays, or contrast with another zettel>
- [[<id>]] **<title>** — <annotation>

## <Sub-tag>

- [[<id>]] **<title>** — <annotation>
```

   The annotation is not the first sentence of the body verbatim — it's a brief editorial note (10–20 words) placing the zettel in context within the cluster. Write it as a human curator would, not as a summarizer.

5. **Link back.** Add `moc-<tag>` to the `links:` frontmatter of each zettel included in the MOC (if not already present).

6. **Update INDEX.md.** If `state/zettel/INDEX.md` exists, check whether this MOC is already listed. If not, and if the topic is substantive (5+ zettels), suggest adding it.

7. Confirm: "MOC written to `state/zettel/moc-<tag>.md` — <N> zettels."

## Notes

- MOC files are zettels themselves — they participate in the link graph. Give them a proper `id: moc-<tag>` and frontmatter.
- Do not regenerate automatically. MOCs are authored documents; overwriting one discards editorial work. Warn before overwriting an existing MOC.
- If a MOC already exists for this tag, read it first. Prefer adding new entries to the existing MOC rather than regenerating from scratch.
- `INDEX.md` is the start-here zettel listing major MOCs. It lives at `state/zettel/INDEX.md`.
