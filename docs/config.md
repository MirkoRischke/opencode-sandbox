# Launcher configuration

Copy `docs/config.example.toml` to
`~/.config/opencode-sandbox/config.toml` (or
`$XDG_CONFIG_HOME/opencode-sandbox/config.toml`). This is the general launcher
configuration; OpenCode's `opencode.json` and `cli.json` remain separate.

The launcher always stops its VM on exit, including errors and handled termination
signals. It never removes the VM: installed tools and guest files persist for the
next launch. Forced termination (`SIGKILL`) or a host crash cannot run cleanup.

New VMs carry the label `app=opencode-sandbox` for bulk management:

```sh
msb ps --label app=opencode-sandbox
msb stop --label app=opencode-sandbox
msb rm --force --label app=opencode-sandbox
```

Existing VMs are not automatically relabeled. Removal deletes guest-local state,
but not mounted host worktrees.

```toml
[network]
allowed_hosts = ["github.com", "api.github.com"]
```

Only exact DNS hostnames are accepted. Connections are limited to TCP port 443
with strict hostname checks and verified TLS inspection. Other outbound traffic
is denied, apart from DNS through Microsandbox's gateway. Host/private networks,
loopback, link-local and metadata destinations remain blocked. An empty list
denies application internet access. Missing files or omitted network settings use
that empty allowlist by default. Malformed TOML, unknown keys, and invalid values
fail launch before VM startup.

Settings apply **only when creating a VM**. Existing VMs keep their old policy,
including VMs created before this feature with public internet access. No policy
comparison or live updates are performed. Recreate VMs to apply restrictions or
changes; recreation deletes guest-local state, not the mounted host worktree.

The config is read on the host, not mounted into the VM. Allowing network access
does not expand GitHub credential substitution permissions. Allowed services can
still receive data: this is a destination restriction, not an exfiltration or
read-only guarantee.

For new launcher settings, extend the `Settings` struct in
`internal/sandbox/settings.go`, add validation, and document the new keys here.
