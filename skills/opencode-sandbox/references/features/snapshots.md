# Prepared workspace snapshots

Snapshots make new VMs start faster: they reuse tools, dependency caches,
and Docker images instead of downloading and installing them again. The
first launch prepares a reusable snapshot, called the baseline. Each new
VM starts with its own copy, so worktrees don't share changes.

A Git checkout and its linked worktrees share a baseline. Independent clones,
non-Git directories, and repository subdirectories get their own. The first
workspace to create the baseline supplies its configuration.

1. [Prepare tools and dependencies](#prepare-tools-and-dependencies)
2. [Choose what goes into a snapshot](#choose-what-goes-into-a-snapshot)
3. [Rebuild a baseline](#rebuild-a-baseline)
4. [List and remove snapshots](#list-and-remove-snapshots)

## Prepare tools and dependencies

Preparation installs tools declared for mise. To also warm package caches
or pull Docker images, set a script in global config or your approved
`.opencode-sandbox.toml`:

```toml
[snapshot]
command = "./scripts/sandbox-prepare.sh"
```

The script runs from `/workspace`. Setup runs only when building a baseline,
not when resuming or restoring a VM.

**Only prepare snapshots from projects you trust.** Mise and setup scripts can
send data over the public internet even if the workspace VM has an allowlist.
Writes to `/workspace` change files on the host and aren't undone if setup
fails. Host services still require [explicit permission](networking.md#vm-to-host).
The gateway key and session AWS role aren't available during preparation.

## Choose what goes into a snapshot

- **Saved:** guest files, tools and caches, plus Docker images, containers,
  named volumes, and build cache. Remove containers or volumes you don't want
  in new VMs.
- **Not saved:** running processes, host mounts, and
  [persistent workspace state](persistent-state.md). For example, `node_modules`
  under `/workspace` isn't captured. Warm the package manager cache on the guest
  disk instead.
- **Sensitive files:** anything you write to a guest disk can end up in the
  snapshot. Keep credentials out of setup scripts and guest files.

## Rebuild a baseline

Baselines don't rebuild automatically when scripts, branches, or lockfiles
change. To refresh the setup, run these commands from the workspace whose
configuration you want to use, then launch again:

```sh
opencode-sandbox snapshot create
opencode-sandbox remove
```

If preparation fails, the previous baseline stays available. Rebuild to change
the image, disk sizes, or installed tools. New VMs already pick up current CPU,
memory, mount, credential, and network settings from your configuration.

## List and remove snapshots

Check which baseline new VMs will use, or free space by deleting snapshots
you no longer need.

- `opencode-sandbox snapshot list` shows snapshots and marks the active
  baseline. The displayed size is not the space you'll necessarily recover.
- `opencode-sandbox snapshot remove` deletes the baseline for this workspace
  or its linked worktree group **without confirmation**.
- `opencode-sandbox snapshot remove --all` deletes every managed snapshot
  **without confirmation**.

Removing or replacing a baseline does not change or delete existing VMs.
See [cleanup](cleanup.md) for deleting both across a repo.
