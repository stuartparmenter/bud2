---
name: zettel-convert
description: "Convert a notes/ file into one or more zettels. Trigger: 'convert note to zettel', 'atomize this note', 'extract zettels from', 'zettelify'."
user-invocable: true
---

# zettel-convert

Extract atomic knowledge from a `notes/` file into the zettelkasten. This skill guides judgment — it does not auto-generate zettels.

## Steps

### 1. Read the source file

Read the full contents of the target `notes/` file.

### 2. Identify candidate concepts

List every distinct idea, claim, or finding in the file. Number them. For each, assess:
- **Atomic?** Can it stand alone without the source document?
- **Evergreen?** Will it still be true or relevant in a year?
- **Novel?** Does it add something not already obvious from context?

Mark each: `zettel` / `combine` / `skip`.
- `zettel`: warrants its own zettel
- `combine`: related to another candidate, should be merged into one zettel
- `skip`: procedural, ephemeral, or too narrow to stand alone

**Show this list to the user** (or reason through it explicitly) before writing any zettels.

### 3. For each `zettel` or `combine` group

- Run `zettel-search` to check for existing coverage
- If not covered: run `zettel-new` to create the zettel
- If covered: note the existing zettel slug (for the comment in step 4)

### 4. Mark the source file

Add a comment at the very top of the source note (after any YAML frontmatter if present):

```
<!-- converted: YYYY-MM-DD, zettels: slug1, slug2, slug3 -->
```

### 5. Do NOT move or delete the source

The `notes/` file stays as a reference. Only move it if separately running `zettel-archive`.

## Judgment guidance

Err toward fewer, better zettels. A 10-page design doc might yield 3–5 zettels, not 30. If a concept only makes sense in the context of the doc, it's a `skip`.
