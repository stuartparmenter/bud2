---
name: zettel-link
description: "Add a bidirectional link between two existing zettels. Trigger: 'link these zettels', 'connect zettel', 'add zettel link', 'relate two zettels'."
user-invocable: true
---

# zettel-link

Add a bidirectional link between two existing zettels.

## Input

Two zettel identifiers — either full IDs (`20240312-act-r-activation`) or slugs (`act-r-activation`). If slugs are ambiguous, glob `state/zettel/*<slug>*` to resolve.

## Steps

1. **Resolve both files.** Find `state/zettel/<id-a>.md` and `state/zettel/<id-b>.md`. If either doesn't exist, stop and report which file is missing.

2. **Check for existing link.** Read both files. If A already lists B in its `links:` and B already lists A in its `links:`, report "already linked" and stop.

3. **Add A → B.** In file A, add B's slug to the `links:` frontmatter array if not present.

4. **Add B → A.** In file B, add A's slug to the `links:` frontmatter array if not present.

5. Confirm: "Linked `<slug-a>` ↔ `<slug-b>`."

## Frontmatter editing rule

The `links:` field is a YAML inline array: `links: [slug1, slug2]`. Preserve existing entries. If `links:` is missing from the frontmatter, add it after the `tags:` line.
