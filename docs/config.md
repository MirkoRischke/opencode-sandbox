# Launcher configuration

Copy `docs/config.example.toml` to
`~/.config/opencode-sandbox/config.toml` (or
`$XDG_CONFIG_HOME/opencode-sandbox/config.toml`). This is the general launcher
configuration; OpenCode's `opencode.json` and `cli.json` remain separate.

Print all built-in defaults with descriptions and a commented secret example:

```sh
opencode-sandbox config default
opencode-sandbox config default > .opencode-sandbox.toml
```

This works outside Git and does not load user configuration, trust state, or
credential files, or start a VM. Values come from the same `DefaultSettings()`
used at runtime; descriptions come from configuration struct tags. The host lists
are empty because built-in defaults deny internet access. Use
`docs/config.example.toml` for a suggested network-enabled configuration instead.
For repo overrides, keep only settings you want to override: saving all defaults
also overrides your global values with those defaults.

## Repository overrides and trust

Optionally add `.opencode-sandbox.toml` at the Git worktree root. Launching from
a subdirectory uses that same root file; parent directories and nested configs
are not merged. Each worktree has its own config and approval.

Settings load in this order: built-in defaults, global `config.toml`, then repo
config. Tables merge recursively, scalars and arrays replace, and omitted keys
inherit. These rules apply to every setting, including named secrets. For example:

```toml
# .opencode-sandbox.toml
[vm]
memory_mib = 8192

[network]
allowed_hosts = []
additional_allowed_hosts = []

[secrets.github]
allowed_hosts = ["api.github.com"]
```

This preserves global CPU/disk settings, disables application internet access,
and overrides only the GitHub secret's destination list. Empty tables do not
clear inherited entries; empty arrays replace inherited arrays. Relative secret
file paths resolve against the directory of the config supplying the `file`
field. Overriding another secret field does not change the inherited file's origin.
Do not commit credential files.

To add repo-specific destinations without replacing global `allowed_hosts`, use:

```toml
# .opencode-sandbox.toml
[network]
additional_allowed_hosts = ["registry.npmjs.org", "nodejs.org"]
```

The effective network allowlist is the union of `allowed_hosts` and
`additional_allowed_hosts`, deduplicated case-insensitively after merging.
Both fields follow the same inheritance/replacement rules: repo additions replace
global additions, not append to them. Keep your shared baseline in global
`allowed_hosts`. To deny all application internet access, set **both** lists to
`[]`. Neither list expands a secret's credential substitution scope.

Before a new or changed repo config is used, an interactive launch shows its
contents and the effective settings, and asks for approval (default: no).
Credential values are not read during this review. Approval authorizes all
settings, including host credential files and substitution destinations.
Unapproved configs fail in non-interactive launches. Invalid configs fail before
approval. An absent repo config needs no approval.

Approvals are stored outside the repo at
`$XDG_STATE_HOME/opencode-sandbox/trust.json`, defaulting to
`~/.local/state/opencode-sandbox/trust.json`. Each record binds the canonical
absolute worktree path to the SHA-256 hash of the exact config bytes. Any change,
including comments, or a moved/copied checkout requires approval again. Symlink
aliases of the same worktree share approval. The directory uses mode `700`, the
file mode `600`, and writes replace the file atomically. Malformed trust storage
fails launch. Delete a repo's entry (or the trust file) to require approval again.

The launcher uses the same bytes it reviewed, even if the file changes while the
prompt is open. Approval is checked before loading credentials or starting the VM,
including when resuming an existing VM. Existing VM policies and credentials still
require recreation to change, as described below.

## Launch behaviour

AWS role access is enabled per launch with `--aws-role` and optional
`--aws-profile`, not with global configuration. See [AWS credentials](aws.md).

The launcher always stops its VM on exit, including errors and handled termination
signals. It never removes the VM: installed tools and guest files persist for the
next launch. Forced termination (`SIGKILL`) or a host crash cannot run cleanup.

New VMs carry the label `app=opencode-sandbox` for bulk management. Run from any
directory, without loading launch configuration or secrets:

```sh
opencode-sandbox list
opencode-sandbox remove
```

`ls` is an alias for `list`; `rm` is an alias for `remove`.

`list` shows names and states of all managed VMs, including stopped VMs. `remove`
stops and deletes all managed VMs without confirmation, including stopped VMs.
It attempts every target and exits nonzero if any removal fails. No matches is a
successful no-op. Existing VMs are not automatically relabeled and unlabeled VMs
are excluded. Removal deletes guest-local state, but not mounted host worktrees.

Inside a Herdr pane (`HERDR_ENV=1`), the launcher automatically identifies itself
as OpenCode for agent detection. No command prefix or Herdr configuration is
needed. An explicitly set `HERDR_AGENT` takes precedence. The TUI still runs in
the guest; the guest receives no Herdr socket access.

```toml
[vm]
cpus = 2
memory_mib = 2048
root_disk_mib = 4096
docker_disk_mib = 10240

[network]
allowed_hosts = ["github.com", "api.github.com"]
```

VM settings default to the values above; each can be overridden independently.
Values must be positive integers (CPUs: 1–255; memory/disk: 1–4294967295 MiB).
Microsandbox and host resource limits still apply. Root-disk size controls
guest-local storage, not mounted host worktrees. Existing VMs retain their specs;
recreate them to apply changes.

### Docker inside the VM

The image includes Docker Engine, Buildx, and Compose (`docker compose`). On each
launch the host launcher starts the guest daemon if `dockerd` is installed, waits
for its local Unix socket to become ready, and fails with recent daemon logs if
startup fails. Images without `dockerd` skip this step. No image-specific startup
script or systemd service is required. An already healthy daemon is left running.
On a cold VM restart, the launcher clears its previous boot's default Docker and
managed-containerd PID files, which otherwise persist on the layered root.

VMs retain their shared, layered root filesystem. A separate private, VM-owned
ext4 disk at `/var/lib/docker` stores Docker images, containers, named volumes,
and build cache. `vm.docker_disk_mib` sets its capacity (default 10240 MiB),
independently of `root_disk_mib`. Both disks survive VM stop/start and are deleted
when the VM is removed. Bind-mounted host files are not deleted. Docker disks
are not shared between VMs. Recreate existing VMs to receive the new image and
disk; rebuilding the image alone does not upgrade them.

The supplied image selects Docker's classic `overlay2` backend, keeping image
data on that ext4 disk rather than Engine 29+'s default containerd image store
under `/var/lib/containerd`. The launcher does not overwrite an image's Docker
configuration. Custom configurations must keep their storage paths on suitable
filesystems.

Daemon output appends to `/var/log/opencode-sandbox/dockerd.log` in the guest,
without truncation or rotation. There is no automatic daemon restart-on-crash or
explicit graceful Docker shutdown. Exiting the launcher stops the entire VM.
The Docker socket is guest-local, never the host's socket; there is no TCP daemon
listener configured by the supplied image. Compose networks and published
container ports work inside the VM, with no port forwarding to the host.

`docs/config.example.toml` allows public Docker Hub and Amazon ECR Public pulls,
including Docker Hub's authentication and image-download hosts. Built-in network
defaults remain deny-all. Guest builds and containers still obey the VM network
policy: package repositories and other download hosts need separate approval.
Containers do not automatically inherit the guest's injected TLS CA; images that
make HTTPS requests through the inspected network must trust that CA themselves.

### Network policy

Only exact DNS hostnames are accepted. Connections are limited to TCP port 443
with strict hostname checks and verified TLS inspection. Other outbound traffic
is denied, apart from DNS through Microsandbox's gateway. Host/private networks,
loopback, link-local and metadata destinations remain blocked. Both host lists
must be empty to deny application internet access. Missing files or omitted
network settings use empty allowlists by default. Malformed TOML, unknown keys,
and invalid values fail launch before VM startup.

Settings apply **only when creating a VM**. Existing VMs keep their old policy,
including VMs created before this feature with public internet access. No policy
comparison or live updates are performed. Recreate VMs to apply restrictions or
changes; recreation deletes guest-local state, not the mounted host worktree.

The config is read on the host, not mounted into the VM. Allowing network access
does not expand GitHub credential substitution permissions. Allowed services can
still receive data: this is a destination restriction, not an exfiltration or
read-only guarantee.

For new launcher settings, extend the `Settings` struct in
`internal/config/settings.go`, add a `description` tag and validation, and document
the new keys here. The default-config printer discovers fields automatically;
tests enforce descriptions and verify that printed values match runtime defaults.

The `internal/config` package owns launcher settings, merging, trust storage,
validation, and default-config generation. Interactive approval remains in the CLI;
credential file loading and Microsandbox policy conversion remain in
`internal/sandbox`. Guest OpenCode JSON and TUI configuration are separate.

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
HTTP header names. Relative paths resolve against the config directory supplying
the `file` field (global or repo);
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
