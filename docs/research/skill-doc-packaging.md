# Packaging documentation with agent skills

Research date: **2026-11-24**. Recommendation only; no implementation change.

## Vercel `skills`: intentional per-skill boundary

The reported local tests match upstream: `skills@1.7.4` copied a valid
`../../docs` symlink, whereas `skills@1.9.0` skips that entry with
`SKILLS_SYMLINK_OUTSIDE_SKILL`. The [1.7.4 copier][old] dereferenced links
without a containment check. [PR #2589][confinement] deliberately changed the
boundary to the **real path of the individual skill directory**, not the Git
checkout. The check is already in [1.8.0][first]. [PR #2652][nested] extended it
to links nested under linked directories and to `skills use`.

This is a known consequence of intentional security hardening, not an
unexplained 1.9.0 regression. The [1.9.0 copier][current] materializes safe
intra-skill links but omits escaping links even if their targets are tracked in
the same repo. The [API docs][warnings] describe this as a warning about an
omitted entry, not a failure of the entire skill. The CLI's standard
**Symlink** install mode still [copies to a canonical skill directory][mode]
before linking that directory into agent locations; it does not change source
file confinement. The [Agent Skills specification][spec] recommends paths
relative to the skill root for bundled references.

## What other projects ship

- **Svelte: docs to installable skills.** The [Svelte sync workflow][svelte-ci]
  selects changed `svelte.dev` docs marked `skill: true`, runs a
  [resolver][svelte-resolver] that writes docs into local `references/` files and
  repoints the skill at them, regenerates plugin/package copies, and opens a PR
  when outputs differ. It also [generates a human-facing skill catalog][svelte-catalog]
  from the resulting `SKILL.md` files. This is a sync PR workflow, not a
  required freshness check on every PR.
- **Next.js: one docs tree, separate workflow skills.** The [package build][next-build]
  copies repo `docs/` into `next/dist/docs/`, retaining the tree and renaming
  `.mdx` to `.md`. The [agent guide][next-guide] explains that managed
  `AGENTS.md` instructions point at those version-matched docs, while installable
  skills sequence workflows and point at the relevant docs rather than
  duplicating framework knowledge in each skill.
- **TanStack Intent: CI for separately authored guidance.** A skill can
  [declare the upstream files it depends on][intent-review]. The reusable
  [PR workflow][intent-ci] checks for unreviewed changes against the PR base;
  maintainers can record an update or an evidenced no-change decision. This is
  drift detection, not text generation or proof that the guidance is correct.

## Recommendation for opencode-sandbox

Keep `docs/` canonical and in place: [feature guides already link to config][config],
[limitations][limits], and [risks][risks]. Keep the [installable skill][skill]
focused on the workflow and CLI-provided defaults and validation. If an
installed skill needs offline prose, generate only the needed pages as regular
files under `references/`. Preserve the relative layout for bundled pages and
rewrite links to unbundled pages to revision-pinned canonical URLs. If a Git
install is advertised, commit the generated files at the install ref and have
CI regenerate, fail on drift, and check links from a fresh install outside the
checkout. Do not move the human docs or depend on the old dereferencing behavior.

[old]: https://github.com/vercel-labs/skills/blob/v1.7.4/src/installer.ts#L517-L554
[confinement]: https://github.com/vercel-labs/skills/pull/2589
[first]: https://github.com/vercel-labs/skills/blob/v1.8.0/src/installer.ts#L580-L594
[nested]: https://github.com/vercel-labs/skills/pull/2652
[current]: https://github.com/vercel-labs/skills/blob/v1.9.0/src/installer.ts#L552-L620
[warnings]: https://github.com/vercel-labs/skills/blob/v1.9.0/docs/api.md#L233-L240
[mode]: https://github.com/vercel-labs/skills/blob/v1.9.0/src/installer.ts#L409-L421
[spec]: https://agentskills.io/specification#file-references
[svelte-ci]: https://github.com/sveltejs/ai-tools/blob/main/.github/workflows/sync-docs-skills.yml#L45-L108
[svelte-resolver]: https://github.com/sveltejs/ai-tools/blob/main/scripts/resolve-references.ts#L194-L245
[svelte-catalog]: https://github.com/sveltejs/ai-tools/blob/main/scripts/update-docs-skills.ts#L41-L89
[next-build]: https://github.com/vercel/next.js/blob/canary/packages/next/taskfile.js#L35-L47
[next-guide]: https://nextjs.org/docs/app/guides/ai-agents
[intent-review]: https://github.com/TanStack/intent/blob/main/packages/intent/meta/generate-skill/references/source-review.md
[intent-ci]: https://github.com/TanStack/intent/blob/main/.github/workflows/check-skills.yml#L154-L170
[config]: ../features/workspaces.md
[limits]: ../features/networking.md
[risks]: ../features/workspaces.md
[skill]: ../../skills/opencode-sandbox/SKILL.md
