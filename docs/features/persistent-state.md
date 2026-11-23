# Persistent workspace state

Keep work you want to resume, such as agent sessions, even when you delete
and recreate the VM. Only the directories you choose are kept.

1. [Keep a guest directory](#keep-a-guest-directory)
2. [Keep OpenCode sessions](#keep-opencode-sessions)

## Keep a guest directory

Add `[[state]]` to your config. Give it a name and the guest path to keep:

```toml
[[state]]
name = "agent-data"
target = "/var/lib/opencode-sandbox/agent-data"
```

- Recreate the VM to apply a new or changed entry. Each workspace, including
  each linked worktree, has its own state.
- The guest can change or delete its saved files. Persist only what you need.
  Snapshots do not include this state.
- `state` entries accumulate across config files. Remove an entry from the
  config that defines it to stop mounting it without deleting saved files.
  Use [prune](cleanup.md#prune-workspace-resources) to delete saved state.

## Keep OpenCode sessions

Add the state mapping and set the startup command in `.opencode-sandbox.toml`.
If you already have a `[startup]` section, replace its `command`:

```toml
[startup]
command = ["env", "OPENCODE_DB=/var/lib/opencode-sandbox/opencode/opencode.db", "opencode"]

[[state]]
name = "opencode-sessions"
target = "/var/lib/opencode-sandbox/opencode"
```

If the workspace already has a VM, recreate it to apply the mapping:

```sh
opencode-sandbox remove
opencode-sandbox
```

Use OpenCode's session picker to resume sessions saved after this setup.
It does not move sessions from the old VM.
