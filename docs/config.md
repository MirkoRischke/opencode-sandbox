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
used at runtime; descriptions come from configuration struct tags. The built-in
allowlist includes OpenAI, the OpenCode model catalog, Context7 and AWS Knowledge
MCPs, GitHub, mise metadata, Go downloads/modules, npm, Docker Hub, and ECR Public.
Diagnostics and unrestricted access are disabled; no credentials are enabled.
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
`[]` and keep `unrestricted = false`. Neither list expands a secret's credential
substitution scope.

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
image = "opencode-sandbox:poc"
cpus = 2
memory_mib = 2048
root_disk_mib = 4096
docker_disk_mib = 10240

[network]
allowed_hosts = ["github.com", "api.github.com"]
```

VM settings default to the values above; each can be overridden independently.
Resource values must be positive integers (CPUs: 1–255; memory/disk: 1–4294967295 MiB).
Microsandbox and host resource limits still apply. Root-disk size controls
guest-local storage, not mounted host worktrees. Existing VMs retain their specs;
recreate them to apply changes.

### Custom VM images

Set `vm.image` in global `config.toml` or repo `.opencode-sandbox.toml` to select
your own image. It defaults to `opencode-sandbox:poc` and must not be empty.
The image must already exist in Microsandbox's local image store. Images are never
pulled automatically; a missing image fails launch. Docker's image store is separate,
so building an image with Docker alone is not enough:

```sh
docker build -t my-custom-image:tag .
docker save my-custom-image:tag | msb load
```

```toml
[vm]
image = "my-custom-image:tag"
```

The simplest compatible image extends the supplied image with
`FROM opencode-sandbox:poc`. Custom images need Bash, Git, mise, and OpenCode on
`PATH`, plus a working system CA trust store for Microsandbox's TLS inspection.
GitHub credential integration additionally requires `gh`; AWS integration requires
the AWS CLI. Docker is optional: images without `dockerd` skip daemon startup.

### Repository snapshots

Snapshots are optional, explicitly prepared disk baselines shared by all linked
worktrees of a repository. Configure a shell command globally or in the approved
repo-root `.opencode-sandbox.toml`:

```toml
[snapshot]
command = "./scripts/sandbox-prepare.sh"
```

For example, the script can run `npm ci`, `go mod download`,
`./gradlew dependencies`, and `docker compose pull`. Its command runs with
`bash -eu -c` from `/workspace`, a writable mount of the **current host worktree**.
It may change host files; failure does not roll those changes back. Cache paths
must be on guest disks, outside that mount, to enter the snapshot. Scripts should
stop any background project services and remove containers/volumes they do not
want cloned, including containers with automatic restart policies.

```sh
opencode-sandbox snapshot create
opencode-sandbox snapshot remove
```

`create` always creates a **fresh** temporary VM from `vm.image`, starts Docker,
configures GitHub integration, trusts/installs project mise tools, and runs the
command. It uses current approved networking and configured proxy secrets, but
does not import OpenAI subscription credentials or assume a session AWS role.
Empty/omitted `snapshot.command` rejects creation before VM startup. There is no
incremental mode or automatic expiry/rebuild when scripts, settings, branches,
or lockfiles change.

After the script succeeds, creation stops running containers and Docker cleanly,
flushes disks, stops the VM, and takes a native Microsandbox **disk-only** snapshot.
It explicitly selects the successful generation as the native group head before
deleting previous generations. A failed preparation leaves the old head intact.
The temporary VM is deleted on success or failure. Forced termination can leave
a temporary VM or an unpublished generation; snapshot removal also cleans up
managed leftover generations in that repository's group.

Normal launch first resumes an existing worktree VM. Otherwise it restores the
repo baseline if present, or creates a normal VM if none exists. A broken baseline
is reported, not silently ignored. New worktree mounts, network policy, CPU/memory,
and current host secrets are configured for the restored VM. Root/Docker capacity,
base image and warmed disk contents come from the snapshot: recreate the baseline
to change those. Existing VMs are neither updated nor migrated.

**What is stored:** all guest root-disk changes (tools, caches, configuration,
logs, temporary files), the owned Docker disk (images/layers, containers, named
volumes, build cache), and native snapshot metadata such as base-image identity
and mount descriptions. RAM/processes and host bind-mounted contents are not
copied. In particular, `node_modules` in the worktree is not captured; a new
worktree still needs installation, ideally from warmed package-manager caches.
Any sensitive file deliberately written to guest disks is captured too; snapshots
are sensitive local state. Host-proxy credential values are not guest files and
are not copied into the snapshot by substitution alone. Captured system CA trust
can retain the preparation VM's CA in addition to the destination runtime CA.

The canonical absolute Git common-directory path identifies a repository.
Linked worktrees and symlink aliases share a group; separate clones do not.
Moving a repository changes its identity. Native group names are
`oc-repo-<readable-repo-name>-<path-hash>`, with uniquely named `prepared-*` members.
Labels include `opencode-sandbox`, `kind=repo-baseline`, `repo-name`, `repo-id`, and
`repo-path`. Microsandbox stores the disks/metadata in its managed host-local
snapshot store. Group/head lookup needs no launcher JSON index. Launcher cache
contains only per-repository lock files to prevent concurrent create/remove/restore
operations; a conflicting operation reports an error rather than racing cleanup.

`snapshot remove` needs no configuration approval or credential reads. It deletes
only that repository's managed snapshot members, old members before the head;
repeat removal is a no-op. Existing restored VMs retain independent writable
disks and survive snapshot removal/replacement and subsequent cold starts.
The existing top-level `remove` remains VM-only and does not delete baselines.

**Microsandbox 0.7.8 restore limitations:** restored VMs require an extra
stop/configure/start cycle for labels, working directory and credentials, before
any project initialization. The Go secret-modification interface cannot express
the configured `headers` restriction: **on snapshot-derived VMs substitution can
occur in any header, but only on the configured allowed hosts**. Such an endpoint
could log/reflect the credential. Freshly created VMs and the preparation VM retain
selected-header restrictions. Restores without secrets still enable verified TLS
inspection via a temporary non-credential secret, removed before project commands.
Network diagnostics retain debug logs and cumulative collection, but restored
VMs cannot enable the optional HTTP denial responder through the restore interface;
denied-host evidence remains best effort. Ordinary destination restrictions remain
in effect. No custom SDK bindings or host cache proxy are used.

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

The built-in defaults and `docs/config.example.toml` allow public Docker Hub and
Amazon ECR Public pulls, including Docker Hub's authentication and image-download
hosts. Guest builds and containers still obey the VM network
policy: package repositories and other download hosts need separate approval.
Containers do not automatically inherit the guest's injected TLS CA; images that
make HTTPS requests through the inspected network must trust that CA themselves.

### Network policy

By default, only exact DNS hostnames are accepted. Connections are limited to TCP port 443
with strict hostname checks and verified TLS inspection. Other outbound traffic
is denied, apart from DNS through Microsandbox's gateway. Host/private networks,
loopback, link-local and metadata destinations remain blocked. Both host lists
must be empty and `unrestricted` must be false to deny application internet access. Missing files or omitted
network settings use the built-in development-service allowlist. Malformed TOML, unknown keys,
and invalid values fail launch before VM startup.

Opt in to unrestricted **public internet** globally or in an approved repo config:

```toml
[network]
unrestricted = true
```

Default: `false`. This allows public destinations on any port/protocol supported
by Microsandbox, including plain HTTP and direct IP connections. Both
`allowed_hosts` and `additional_allowed_hosts` are ignored, not rejected; their
values take effect again when `unrestricted` is disabled. Configured hostnames
must still be valid. Host/private networks, loopback, link-local, metadata,
multicast, and unspecified addresses remain blocked, apart from gateway DNS.
Guest-local loopback is internal to the VM and is unaffected by egress policy.
Inbound access remains denied. TLS inspection and verified upstream TLS remain
enabled, and secret substitution stays limited to its configured hosts/headers.
Unrestricted access does not expand any credential's substitution scope.
Existing TLS inspection limitations still apply: direct-IP HTTPS can fail even
though the public IP is allowed; use hostname-based HTTPS instead.

Settings apply **only when creating a VM**. Existing VMs keep their old policy,
including VMs created before this feature with public internet access. No policy
comparison or live updates are performed. Recreate VMs to apply restrictions or
changes; recreation deletes guest-local state, not the mounted host worktree.

The config is read on the host, not mounted into the VM. Allowing network access
does not expand GitHub credential substitution permissions. Allowed services can
still receive data: this is a destination restriction, not an exfiltration or
read-only guarantee.

### Network diagnostics

Opt in globally or in an approved worktree config:

```toml
[network]
diagnostics = true
```

Default: `false`. Recreate an existing VM after changing this setting: runtime
verbosity is stored with the VM, just like its network policy. Disabling collection
alone does not lower the verbosity of a VM previously created with diagnostics.

Microsandbox writes **all runtime debug logs**, including unrelated diagnostics,
to its host-side `<sandbox-dir>/logs/runtime.log`. These rotate at 10 MiB with three
rotated files (roughly 40 MiB). Normal info/warning/error logs remain included;
guest command output and kernel logs use their existing separate logging paths.
There is no networking-only verbosity filter or pre-disk Go log interception.

Diagnostics also enable Microsandbox's HTTP-denial responder: eligible HTTP/HTTPS
requests receive HTTP 403 instead of a connection reset. This lets its existing
TLS diagnostics record the denied hostname; early default-deny TCP resets are
otherwise silent. No upstream connection is allowed by the responder, and
protected/private destinations remain blocked. Clients must reach the TLS/SNI
handling path for hostname evidence to be available.

The launcher periodically reads new system-log records, recognizes Microsandbox
0.7.8 network-policy denial messages, and merges them into one cumulative overview:

```text
${XDG_STATE_HOME:-~/.local/state}/opencode-sandbox/diagnostics/<worktree-id>/network.json
```

The exact path is printed on startup. Worktree identity hashes its canonical path,
not the VM instance or branch. The JSON survives process exit and VM deletion or
recreation. Each launch collects only that launch's new records, not old raw logs.
It is saved atomically every five seconds when changed, and drained/saved during
normal shutdown. Background errors are reported after the interactive session ends,
not while OpenCode owns the terminal. Warnings distinguish collection from save
failures and include a safe error category. They do not prevent VM use; malformed history is not
overwritten. Disabled diagnostics leave existing summaries untouched.

Print the saved JSON unchanged from anywhere inside the worktree:

```sh
opencode-sandbox diagnostics network
opencode-sandbox diagnostics network | jq '.denied_targets'
```

This read-only command does not start a VM or load configuration, trust state, or
credentials. It works after VM deletion and shows the latest saved snapshot, not
live logs. Missing history is an error reported on stderr with a nonzero exit.
Running `opencode-sandbox diagnostics` shows usage and the available category.

`denied_targets` contains optional `hostname`, `ip`, `port`, `protocol`, and `reason`,
plus `observations`, `first_seen`, and `last_seen`. Hostnames group independently of
changing destination IPs; unknown hostnames use the IP instead. Different ports,
protocols, and reasons remain separate. No raw messages, URLs, headers, or payloads
are copied into the overview. `schema_version`, `worktree`, and `updated_at` describe
the overview; `overflow: true` indicates its 4096-distinct-target limit was reached.

This is best-effort historical evidence, **not a complete audit**: Microsandbox
does not log every denial or always expose a hostname. Counts are observed log
records, not unique requests. Allowed hosts are never added automatically; old
denials remain after policy changes, with `last_seen` showing their age. Summaries
are not automatically expired; remove the worktree's `network.json` to reset it
while its launcher is stopped.

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
