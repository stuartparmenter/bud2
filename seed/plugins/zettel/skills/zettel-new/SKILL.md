---
name: zettel-new
description: "Create a new atomic zettel note. Trigger: 'new zettel', 'create zettel', 'add to zettelkasten', 'atomize this idea'."
user-invocable: true
---

# zettel-new

Create a single atomic zettel. One idea. One file. Densely linked.

## Title rule (enforce strictly)

**Title must be a claim or named concept — not a category.**

| Bad (category) | Good (claim or concept) |
|---|---|
| "ACT-R research" | "ACT-R base-level activation decays logarithmically" |
| "Memory notes" | "Working memory capacity is 4 ± 1 chunks" |
| "Spacing effect" | "Spaced repetition exploits the spacing effect to reduce forgetting" |

If the title reads like a folder name, reject it and ask for a specific claim.

## Steps

1. **Search first.** Run `zettel-search` with the core concept. If strong overlap exists, link to existing instead of creating.
2. **Gather info:**
   - Title (claim/concept — enforce the rule above)
   - Tags (2–5 lowercase kebab-case tags)
   - Links (slugs of related existing zettels)
   - Source file path (optional, relative to `state/`) if derived from a notes/ file
3. **Generate ID:** `YYYYMMDD-slug` where slug is a short (2–5 word) kebab-case version of the title. Use today's date.
4. **Write** to `state/zettel/<id>.md`:

```markdown
---
id: YYYYMMDD-slug
title: The claim or concept stated precisely
tags: [tag1, tag2]
links: [related-slug, another-slug]
source: notes/original-file.md
created: YYYY-MM-DD
---

Body: 50–200 words. Atomic — one idea per zettel. Self-contained: a reader should understand the idea without consulting the source. End with the implication or why this idea matters.
```

5. **Bidirectional links.** For each zettel listed in `links:`, open that file and add the new zettel's slug to its `links:` frontmatter array (if not already present).

## Quality checks

- Body is 50–200 words
- Body does not merely restate the title — it explains, justifies, or extends
- No unresolved pronouns ("it", "this") without clear antecedent
- `source:` is set if the idea came from a notes/ file
