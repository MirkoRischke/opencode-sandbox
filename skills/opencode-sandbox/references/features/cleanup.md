# Resource cleanup

Free disk space or reset an environment without deleting your project files.
Choose how much to remove:

1. [List and remove VMs](#list-and-remove-vms)
2. [Remove VMs left behind by deleted worktrees](#remove-vms-left-behind-by-deleted-worktrees)
3. [Remove just the prepared baseline](#remove-a-baseline)
4. [Prune workspace resources](#prune-workspace-resources)

## List and remove VMs

- Run `opencode-sandbox list` to see managed VMs, including stopped ones.
- Run `opencode-sandbox remove` from the workspace to delete its VM
  **without confirmation**. Use this when you need a fresh VM but want to
  keep the snapshot and host-backed state.
- `opencode-sandbox remove --all` deletes **all** managed VMs without
  confirmation.

## Remove VMs left behind by deleted worktrees

`git worktree remove` leaves the VM behind. Preview which VMs you can clean up:

```sh
opencode-sandbox remove --orphaned --dry-run        # Current Git repo
opencode-sandbox remove --orphaned --all --dry-run  # All repos, from anywhere
```

Drop `--dry-run` to delete the matching VMs **without confirmation**. Keep
`--orphaned` when using `--all` to limit deletion to orphaned VMs.

Only stopped VMs for linked worktrees whose directories are gone and whose
paths are no longer listed by Git qualify. If Git can't verify the worktree,
the VM is skipped. Snapshots, logs, diagnostics, and host-backed state are
preserved. Older VMs missing worktree metadata need manual cleanup.

## Remove a baseline

Use `opencode-sandbox snapshot remove` to delete just the prepared baseline.
It doesn't change existing VMs. Next time a new VM needs a baseline, it will
prepare one again. See [snapshot commands](snapshots.md#list-and-remove-snapshots)
for listing or removing baselines across workspaces.

## Prune workspace resources

Prune removes more than the VM, including saved state and logs. Start with
a preview:

```sh
opencode-sandbox prune --dry-run   # Preview (works with any scope)
opencode-sandbox prune            # Current directory or worktree
opencode-sandbox prune --repo     # Whole repo and its tracked worktrees
opencode-sandbox prune --all      # All workspaces AND Microsandbox images/caches
```

Prune stops and deletes target VMs. It keeps your workspace files, config,
credentials, gateway, and host Docker resources. It keeps the baseline unless
you use `--repo` or `--all`.

- `--repo` needs a checkout or worktree root. It also removes the shared
  baseline and resources from tracked worktrees you've deleted.
- `--all` also removes all managed baselines, **all cached Microsandbox
  images**, and runtime caches. This affects images and caches unrelated to
  this tool. [Some files may be retained](../limitations/microsandbox.md#msb-11--cache-cleanup-retains-owned-backing).
- Review the targets before confirming. Use `--yes` only to skip confirmation.
