# Persistent workspace state

Use `[[state]]` to keep selected guest directories across VM removal. No
directories are persisted this way by default. Without this option, guest files
survive stop/start on the VM disk but are lost when the VM is removed.

Each entry names a directory the launcher creates on the host and mounts
writable at the specified guest path:

```text
${XDG_STATE_HOME:-$HOME/.local/state}/opencode-sandbox/state/<repo-id>/<workspace-id>/<name>/
```

Linked worktrees share a repository directory but never share writable state.
Outside Git, state belongs to the exact workspace directory. The guest can
change or delete anything in its mounted state, so persist only the directories
you need. The launcher does not mount your existing host agent data.

State is not mounted in snapshot preparation VMs or captured in baselines.
`remove` and snapshot removal leave it on the host; `prune` deletes it for the
selected scope. Removing a config entry does not delete its host directory.
Recreate the VM to apply mapping changes. A workspace `state` list replaces the
global list; use `state = []` to clear inherited entries.

## Example: keep OpenCode sessions

Add the state mapping and set the startup command in `.opencode-sandbox.toml`.
If you already have a `[startup]` section, replace its `command`:

```toml
[startup]
command = ["env", "OPENCODE_DB=/var/lib/opencode-sandbox/opencode/opencode.db", "opencode"]

[[state]]
name = "opencode-sessions"
target = "/var/lib/opencode-sandbox/opencode"
```

The startup command sets `OPENCODE_DB` in the guest without a custom image.
If the workspace already has a VM, recreate it to apply the state mapping:

```sh
opencode-sandbox remove
opencode-sandbox
```

The directory keeps the SQLite database and its sidecar files together.
Use OpenCode's session picker to resume. Existing sessions are not migrated.
