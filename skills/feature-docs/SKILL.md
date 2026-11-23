---
name: feature-docs
description: Load only when writing, consolidating, or reviewing guides for user-facing capabilities that are a meaningful reason to use opencode-sandbox. Skip minor options and conveniences, routine documentation edits, internal changes, and developer-only docs.
---

# Feature docs

Keep one authoritative guide per major user-facing feature in `docs/features/`.
For review-only requests, report which checks fail without editing.

## Decide whether it deserves a guide

Ask: **Would this capability be a meaningful reason to choose opencode-sandbox?**
Being visible to users isn't enough. A small flag, alias, or convenience
doesn't earn its own guide or feature overview entry. If it changes how users
complete a task in an existing guide, update that guide briefly. Otherwise,
leave it to command help or the configuration reference.

If the capability warrants a new guide, follow the approval rule in
[`AGENTS.md`](../../AGENTS.md) before creating it.

## Workflow

1. **Find the existing guidance.** Search the repo for the feature name,
   command names, config keys, and distinctive examples. Read the matching
   docs, including other feature guides, the root README, config docs, and
   component READMEs. Prefer updating a guide that already owns the topic.
2. **Write for users.** Lead with what the feature lets them do and why it
   helps. Organize around tasks, not internals. Use short sentences, bullets,
   and copyable commands or config examples. Put prerequisites, destructive
   effects, and steps such as recreating a VM next to the action they affect.
   Keep implementation details in developer docs.
3. **Consolidate and index.** Move unique user instructions into the canonical
   guide. Replace repeated procedures elsewhere with links to the relevant
   section. Keep brief summaries, first-run steps that link to details,
   standalone risk warnings, and historical evidence where they serve a
   distinct purpose. Add or update one entry per feature in the
   [feature overview](../../docs/features/README.md), under the best-fit
   category. Link to the guide and give a one-line user benefit. List
   subfeatures in the guide, not as separate overview entries.

## Template

For a single-purpose guide, skip the numbered list and extra headings. For a
worked example, see [Prepared workspace snapshots](../../docs/features/snapshots.md):
the opening explains why new VMs start faster before explaining how to use them.

```markdown
# Feature name

Say what users can do and why it helps. Lead with the benefit.

1. [First task](#first-task)
2. [Second task](#second-task)

## First task

Briefly explain what this does. Show the shortest working command or
config example.

## Second task

Explain what users need to do. Put any warning or prerequisite next to
the step it affects.
```

## Definition of done

- [ ] **Benefit:** The feature earns its own guide, and the opening says what
  users can do and why it helps.
- [ ] **Usability:** Each section explains what it does and how to use it.
  Verify commands, config examples, and user-visible behavior against the
  current tool. Check that prerequisites and warnings appear before the
  affected action.
- [ ] **Navigation:** For multiple subfeatures, the numbered list at the top
  links to each section in order. The feature overview has one entry linking
  to the canonical guide with a short benefit. New or changed links and
  anchors resolve.
- [ ] **No duplicate guidance:** Recheck the repo-wide search after editing.
  No other doc repeats the procedure or configuration explanation for this
  feature. Check that links replacing moved instructions point to the right
  section and that no unique guidance was lost.
- [ ] **Final read:** A reader can find and follow the right steps without
  reading the source. Remove implementation details that don't affect user
  choices or actions.

Only report the feature as documented once every applicable check passes.
Say what duplicate guidance was moved or replaced, or that the search found
none. If a check is blocked, report what remains unverified.
