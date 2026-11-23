# Documentation

When a change appears to introduce a capability that gives users a meaningful
reason to choose opencode-sandbox, ask whether to add a feature guide. Add one
to `docs/features/` only after the user confirms.

For significant user-facing features, follow the
[feature-docs skill](skills/feature-docs/SKILL.md) when writing, consolidating,
or reviewing their guides.

Update `README.md` only when explicitly asked or to correct information made
outdated by your changes. Exceptions:

- a new command was added

When encountering a Microsandbox limitation or issue while working on this tool,
update `docs/limitations/microsandbox.md` with the affected SDK/runtime version,
impact on this tool, evidence, and a check for future upgrades. Distinguish
confirmed limitations from suspected bugs; update existing entries when rechecked.

# Implementation

- Handle each error once: either return it (optionally wrapped with context), or log it where handled. Never log and propagate the same error.
- Preserve underlying errors when adding context: wrap with `%w` so callers retain `errors.Is`/`errors.As` and diagnostics retain the cause.

- Do not consider migration scenarios; recreation of sandboxes/VMs is acceptable.

# CLI conventions

- Use `remove` (alias `rm`) for removal commands and `list` (alias `ls`) for listing commands, including subcommands.
