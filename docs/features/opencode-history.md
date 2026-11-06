# OpenCode history (experimental)

Each workspace VM has its own persistent OpenCode SQLite database on the host:

```text
${XDG_STATE_HOME:-$HOME/.local/state}/opencode-sandbox/opencode/<workspace-id>/opencode.db
```

The workspace ID hashes the canonical workspace directory path. A checkout and
each of its linked Git worktrees have separate databases, as do non-Git directories.
Symlink aliases share identity. Recreating a VM for the same workspace path reuses
that workspace's existing database.

Only that workspace's database directory is mounted writable into its VM,
at `/var/lib/opencode-sandbox/opencode`. `OPENCODE_DB` selects `opencode.db`
there; SQLite's WAL/SHM sidecars live alongside it. The host's own OpenCode
database is never mounted. Neither session databases nor the entire OpenCode
configuration or data directory are shared between workspace VMs.

VM and snapshot removal never delete this storage. Preparation VMs do not mount
it, and restoring a baseline does not roll history back. Existing VMs need
recreation to adopt the mount and environment; existing VM-local sessions are not
migrated. Existing disk baselines can still be restored.

Use OpenCode's native session picker to resume that workspace's sessions.
Worktree files, branches, and running tools are not restored by retaining session
history. Removing a worktree leaves its history on the host; a different workspace
path does not inherit it.
