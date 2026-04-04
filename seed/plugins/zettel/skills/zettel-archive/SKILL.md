---
name: zettel-archive
description: "Move an ephemeral notes/ file to notes/archive/. Trigger: 'archive this note', 'move to archive', 'this note has no zettel value', 'archive sprint brief', 'archive PR snapshot'."
user-invocable: true
---

# zettel-archive

Move an ephemeral `notes/` file to `notes/archive/`. Use for notes with no evergreen knowledge value: sprint briefs, PR status snapshots, benchmark runs, wellness checks, meeting agendas.

**Do NOT create a zettel** as part of this skill. If there is knowledge worth keeping, run `zettel-convert` first, then archive.

## Steps

1. **Check for zettel citations.** Grep `state/zettel/*.md` for the source file's path in any `source:` frontmatter field.
   - If found: update the `source:` path in those zettels to `notes/archive/<filename>`.
   - If not found: proceed.

2. **Move the file.**
   ```bash
   mv state/notes/<filename> state/notes/archive/<filename>
   ```
   If a file with that name already exists in archive, append the current date to the filename before moving: `<stem>-YYYYMMDD.<ext>`.

3. **Confirm.** Report: "Archived `notes/<filename>` → `notes/archive/<filename>`." and list any zettels whose `source:` path was updated.

## What belongs in archive

- Sprint briefs, weekly plans, status updates
- PR or deploy snapshots
- One-off benchmark or profiling runs
- Wellness checks, incident timelines (after postmortem zettel is written)
- Meeting notes with no lasting insight

## What does NOT belong in archive

If a note contains a novel claim, design decision rationale, or lasting finding — run `zettel-convert` first to extract the value, then archive.
