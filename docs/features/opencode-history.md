# OpenCode history (experimental)

Workspace VMs for a checkout and its linked Git worktrees share one OpenCode
SQLite database on the host:

```text
${XDG_STATE_HOME:-$HOME/.local/state}/opencode-sandbox/opencode/<repo-id>/opencode.db
```

The repository ID hashes the canonical Git common-directory path. Separate clones
have separate databases. A non-Git directory (including a repository subdirectory
without its own `.git` marker) uses its canonical directory identity instead.
Symlink aliases share identity.

Only that repository's database directory is mounted writable into its workspace
VMs, at `/var/lib/opencode-sandbox/opencode`. `OPENCODE_DB` selects `opencode.db`
there; SQLite's WAL/SHM sidecars live alongside it. The host's own OpenCode
database is never mounted. This shares database state, not just conversation text;
it does not share the entire OpenCode configuration or data directory.

VM and snapshot removal never delete this storage. Preparation VMs do not mount
it, and restoring a baseline does not roll history back. Existing VMs need
recreation to adopt the mount and environment; existing VM-local sessions are not
migrated. Existing disk baselines can still be restored.

Use OpenCode's native session picker, toggling its scope with Ctrl+A when needed.
Sessions from removed worktrees retain their old location; use OpenCode's native
session move action to continue in an existing worktree. Worktree files, branches,
and running tools are not restored by retaining session history.

## Accepted experimental risk

**Concurrent access can corrupt or lose the shared database.** Microsandbox
v0.7.11 does not forward POSIX locks between VMs, while SQLite relies on those
locks. OpenCode v2.2.5 also has process-local event and execution coordination;
sharing rows does not make independent servers one coordinated service. Changes
may not appear live in another instance, and the same session must not be assumed
safe to execute in both VMs.

Use disposable history while evaluating this behavior. Passing persistence tests
does not establish concurrency safety. See [MSB-10](../limitations/microsandbox.md#msb-10--bind-mount-file-locks-do-not-coordinate-independent-vms).
