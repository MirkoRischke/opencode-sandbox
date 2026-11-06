# Resource cleanup

```sh
opencode-sandbox prune           # Current directory/worktree
opencode-sandbox prune --repo    # Repo and its other worktrees
opencode-sandbox prune --all     # All managed workspaces, plus ALL MSB images/caches
```

Deletes workspace VMs, snapshots, OpenCode databases (including conversation
history), host logs, diagnostics, and workspace ownership records. Running target
VMs are forcibly stopped and removed; no activity or age filters apply.

Targets are shown before confirmation. `--yes` (`-y`) skips confirmation;
`--dry-run` previews without deleting. `--repo` and `--all` are mutually exclusive.
`--repo` requires a checkout/worktree root. Ownership is recorded on launch and
snapshot creation, allowing repo cleanup after a tracked worktree is deleted.

`--all` force-removes **every cached MSB image**, including those referenced by
unrelated MSB sandboxes, and requests machine-wide runtime cache pruning. MSB may
retain owned cache files; these are reported as incomplete cleanup. Stable lock
files are retained. Forced image removal untags images, but MSB preserves backing
still owned by unrelated sandboxes/snapshots. See
[MSB-11](../limitations/microsandbox.md#msb-11--cache-cleanup-retains-owned-backing).

Workspace source files, configuration, credentials, the gateway, and Docker
resources are preserved. If VM/snapshot deletion fails, host data and ownership
records are retained for retry; other runtime targets are still attempted.
