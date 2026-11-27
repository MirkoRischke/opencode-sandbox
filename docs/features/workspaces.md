# Workspace VMs

Each directory gets its own VM, so you can work on several projects or Git
worktrees in parallel without mixing dependencies or fighting over guest
ports. Run `opencode-sandbox` from the directory you want to work in.
[Ports exposed on the host](networking.md#host-to-vm) must still be unique.

1. [Choose a workspace](#choose-a-workspace)
2. [Launch, stop, and resume](#launch-stop-and-resume)
3. [Set VM resources](#set-vm-resources)

## Choose a workspace

- Launch from the exact directory you want the guest to edit. It is mounted
  at `/workspace`, where guest commands and shells start. Changes are visible
  immediately on the host, so you can use your usual editor.
- Launch from a Git checkout or worktree root if you want Git metadata
  available. From a subdirectory, the parent repository and its configuration
  are not shared. Use Git's default absolute paths for linked worktrees
  that reference metadata outside the checkout. Git is optional.
- Use a project directory, not a drive root, home directory, or system
  directory. Symlinks to the same directory use the same VM.
- Use [host file mappings](host-files.md) to share other files. Review the
  [accepted risks](../risks.md) before running files the guest can modify.

## Launch, stop, and resume

1. The launcher resumes the workspace's VM if one exists. Otherwise it
   [restores or prepares a baseline](snapshots.md).
2. Use your [coding agent](coding-agents.md) in the guest. When you quit the
   agent, you land in Bash.
3. Exit Bash to stop the VM. Run `opencode-sandbox` again to resume it.

Guest disks survive stop/start but are deleted by `opencode-sandbox remove`.
Use [persistent workspace state](persistent-state.md) for selected directories
that should survive removal. See [resource cleanup](cleanup.md) for the
difference between removing a VM, deleting a baseline, and pruning a workspace.

## Set VM resources

These are the default resource settings. Override individual fields in
[launcher configuration](../config/README.md).

```toml
[vm]
cpus = 2
memory_mib = 2048
root_disk_mib = 4096
docker_disk_mib = 10240
```

Disk sizes are in MiB. The root disk setting does not limit the size of the
mounted workspace. Recreate the VM to apply CPU or memory changes. Also
[rebuild the snapshot](snapshots.md#rebuild-a-baseline) if you change disk
sizes or the [VM image](vm-images.md).
