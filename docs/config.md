# Launcher configuration

Copy `docs/config.example.toml` to
`~/.config/opencode-sandbox/config.toml` (or
`$XDG_CONFIG_HOME/opencode-sandbox/config.toml`). This is the general launcher
configuration; OpenCode's `opencode.json` and `cli.json` remain separate.

AWS role access is enabled per launch with `--aws-role` and optional
`--aws-profile`, not with global configuration. See [AWS credentials](aws.md).

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
[vm]
cpus = 2
memory_mib = 2048
root_disk_mib = 4096

[network]
allowed_hosts = ["github.com", "api.github.com"]
```

VM settings default to the values above; each can be overridden independently.
Values must be positive integers (CPUs: 1–255; memory/disk: 1–4294967295 MiB).
Microsandbox and host resource limits still apply. Root-disk size controls
guest-local storage, not mounted host worktrees. Existing VMs retain their specs;
recreate them to apply changes.

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

## Secrets

Only explicitly configured secrets are loaded. There is no implicit GitHub token
loading and no fallback to host environment variables or host `gh` credentials.

```toml
[secrets.github]
file = "creds/github-token"
env = "GH_TOKEN"
allowed_hosts = ["github.com", "api.github.com"]
headers = ["authorization"]
```

Add other named `[secrets.<name>]` sections in the same form. Each must have a
unique environment variable name, a file, non-empty exact hostnames, and non-empty
HTTP header names. Relative paths resolve against the launcher config directory;
absolute paths are also accepted. Inline values and environment sources are not
supported. Use mode `700` on the credential directory and `600` on the file.
Symlinked paths, oversized files (over 16 KiB), empty values, multiple lines, and
NUL bytes are rejected. A trailing newline is stripped. Configured files are
validated on every launch; missing/invalid files fail launch, even for existing VMs.

Guests receive placeholders such as `$MSB_GH_TOKEN`, never the source value.
Microsandbox substitutes values only in the selected headers for the selected
hosts over verified TLS. Secret scopes never grant network access: the hosts
must independently be allowed by `[network]`. Configuring `GH_TOKEN` also enables
GitHub's guest HTTPS Git credential helper. OpenAI subscription authentication
remains separate and unchanged.

Real values persist in Microsandbox's **host-side VM configuration**. Never log
resolved secrets or commit credential files. Allowed upstreams receive real
values and could reflect them back; this is not an absolute secrecy guarantee.
Only new VMs receive changes: recreate a VM to rotate or remove its stored
credentials. Removing an entry stops reading its file, but does not revoke
credentials in existing VMs. Revoke credentials upstream for immediate revocation.
