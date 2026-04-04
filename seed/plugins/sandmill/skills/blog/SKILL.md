---
name: blog
description: "Manage the sandmill.org blog pipeline: list post ideas, research, draft, polish, and publish. Trigger phrases: /blog, blog status, research post, draft post, publish post."
user-invocable: true
---

# Blog Pipeline Manager

Manage the sandmill.org blog end-to-end: ideation → research → draft → polish → publish.

**Things 3** is the hub. Each post = a Things project tagged `blog-post`, in the **Bud** area.
**Research/drafts** live in `state/notes/blog/{slug}.md`.
**Published posts** go to `/Users/thunder/src/sandmill/resources/content/blog/{date}.{slug}.md`.

---

## Sub-commands

Parse the user's invocation and dispatch to the appropriate section:

- `/blog status` — show the pipeline
- `/blog new <title>` — create a new post idea
- `/blog research <slug>` — spawn researcher for a post
- `/blog draft <slug>` — collaborative drafting session
- `/blog publish <slug>` — finalize and publish a post
- `/blog help` — print usage

If invoked as just `/blog` with no sub-command, run **status**.

---

## `/blog status`

Show all posts in the pipeline, grouped by status.

1. Call `things_get_projects` to find all projects tagged `blog-post` in the Bud area.
2. Group by their second tag (status): `idea`, `researching`, `drafting`, `polishing`, `ready`, `published`.
3. Print each group as a section, listing post title and a one-liner from the notes field.

**Output format:**
```
## Blog Pipeline

### idea
- Mac OS 8 in the Browser (macos8-emulator)
- Bud & Engram: Building a Second Brain (bud-engram)

### drafting
- Persona/BrowserID: The Federated Identity Standard That Almost Was (persona-browserid)

### published
(none)
```

---

## `/blog new <title>`

Create a new post idea in the pipeline.

1. Derive a slug from the title (lowercase, hyphenated). If ambiguous, ask the user with `talk_to_user`.
2. **Dedup check**: Call `things_get_projects` and scan for an existing project with the same title (or very similar). If one already exists, do NOT create a new one — report its ID and status instead.
3. Call `things_add_project` with:
   - `title`: the post title
   - `tags`: `["blog-post", "idea"]`
   - `notes`: a stub noting the slug and any initial angle provided
   - `area`: **Bud** (always — never Work or Life)
   - `checklist_items` (if supported): `["Research", "Outline approved", "Draft written", "Polish pass", "Markdown committed", "Deployed"]`
3. Create stub notes file at `state/notes/blog/{slug}.md`:
   ```markdown
   # {Title}

   slug: {slug}
   status: idea

   ## Angle / Initial Notes

   (Add notes here)
   ```
4. Report back: "Created blog post idea: '{title}' (slug: `{slug}`). Notes stub at `state/notes/blog/{slug}.md`."

---

## `/blog research <slug>`

Kick off research for a post.

1. Find the Things project where the notes contain the slug.
2. Update the Things project tag from `idea` → `researching` using `things_update_project`.
3. Spawn a researcher subagent with `Agent_spawn_async`:
   - **Role**: web researcher
   - **Task**: Research the topic thoroughly. Read the existing notes at `state/notes/blog/{slug}.md` for context. Gather facts, references, interesting angles, technical depth. Save all findings back to `state/notes/blog/{slug}.md` under a `## Research` section. Include source URLs for everything.
   - Pass the current notes content and slug as context.
4. While the subagent runs, report: "Spawned researcher for `{slug}`. Will update notes when complete."
5. When subagent completes (on wake), update the Things status tag to `researching` (already done) and notify the user the research is ready.

---

## `/blog draft <slug>`

Collaborative drafting session.

1. Read `state/notes/blog/{slug}.md` to load research.
2. Update Things tag to `drafting`.
3. Ask the user 2-3 focused questions using `talk_to_user`:
   - What's the primary angle or thesis?
   - Who is the target audience? (technical, general, niche)
   - Any specific tone? (technical deep-dive, narrative, tutorial, opinion)
4. Write a complete draft blog post based on the research and answers.
5. Save the draft to `state/notes/blog/{slug}.md` under a `## Draft` section (keep the research section intact above it).
6. Report: "Draft saved to `state/notes/blog/{slug}.md`. Review and run `/blog publish {slug}` when ready."

**Draft format:**
```markdown
## Draft

---
title: {Title}
date: {YYYY-MM-DD}
slug: {slug}
tags: []
excerpt: {one or two sentence summary}
draft: true
---

{Full post content in markdown}
```

---

## `/blog publish <slug>`

Finalize and publish the post.

1. Read `state/notes/blog/{slug}.md` and extract the `## Draft` section.
2. Determine the publication date (today's date: `{YYYY-MM-DD}`).
3. Write the final file to:
   `/Users/thunder/src/sandmill/resources/content/blog/{date}.{slug}.md`
   with `draft: false` in the YAML frontmatter.
4. Commit to the sandmill repo:
   ```bash
   cd /Users/thunder/src/sandmill
   git add resources/content/blog/{date}.{slug}.md
   git commit -m "Add blog post: {title}"
   ```
5. **Before deploying**, call `talk_to_user` to confirm:
   "Post `{title}` is committed. Deploy to Dokku now? (This will make it live on sandmill.org)"
6. If confirmed, trigger the deploy (e.g., `git push dokku main` or the appropriate deploy command).
7. Update Things project tag to `published`.
8. Report: "Published! `{title}` is live."

---

## `/blog help`

Print this usage guide:

```
/blog              — show pipeline status
/blog status       — show pipeline status
/blog new <title>  — create a new post idea
/blog research <slug>  — spawn researcher for a post
/blog draft <slug>     — start a drafting session
/blog publish <slug>   — finalize and publish
/blog help         — this message
```

---

## Things project conventions

Each blog post is a Things project with:
- **Tags**: `blog-post` + one status tag (`idea`, `researching`, `drafting`, `polishing`, `ready`, `published`)
- **Notes**: includes the slug and link to `state/notes/blog/{slug}.md`
- **Checklist**: Research → Outline approved → Draft written → Polish pass → Markdown committed → Deployed

When updating status, remove the old status tag and add the new one using `things_update_project`.

---

## File conventions

- Notes/research: `state/notes/blog/{slug}.md` (in bud2 repo)
- Published posts: `/Users/thunder/src/sandmill/resources/content/blog/{YYYY-MM-DD}.{slug}.md`
- Post frontmatter fields: `title`, `date`, `slug`, `tags`, `excerpt`, `draft`
