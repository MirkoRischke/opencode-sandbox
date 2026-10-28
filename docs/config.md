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
MCPs, and GitHub Git/API/raw-file access. Tool downloads, package registries,
source archives, release assets, and container registries are not included;
snapshot preparation has public internet access. Add project-specific hosts for
downloads needed while the agent runs.
Diagnostics and unrestricted access are disabled; no credentials are enabled.
For workspace overrides, keep only settings you want to override: saving all defaults
also overrides your global values with those defaults.

## Workspace overrides and trust

Optionally add `.opencode-sandbox.toml` in the current directory. The exact current
directory is always the workspace, with or without Git. Parent directories and
their repositories/configs are never discovered or merged. Each canonical
directory has its own config, approval, VM, and snapshot baseline.

Settings load in this order: built-in defaults, global `config.toml`, then workspace
config. Tables merge recursively, scalars and arrays replace, and omitted keys
inherit, except that each named secret is replaced completely. For example:

```toml
# .opencode-sandbox.toml
[vm]
memory_mib = 8192

[network]
allowed_hosts = []
additional_allowed_hosts = []

[secrets.github]
keyring = {}
env = "GH_TOKEN"
allowed_hosts = ["api.github.com"]
headers = ["authorization"]
```

This preserves global CPU/disk settings, disables application internet access,
and replaces the entire GitHub secret definition. Other global secrets remain
available. A workspace secret must supply its own source, environment name,
hosts, and headers; no fields inherit from the same-named global secret.
An empty named secret table is invalid. Other empty tables do not clear inherited
entries; empty arrays replace inherited arrays. Relative secret file paths resolve
against the directory of the config defining that secret.
Do not commit credential files.

To add workspace-specific destinations without replacing global `allowed_hosts`, use:

```toml
# .opencode-sandbox.toml
[network]
additional_allowed_hosts = ["registry.npmjs.org", "nodejs.org"]
```

The effective network allowlist is the union of `allowed_hosts` and
`additional_allowed_hosts`, deduplicated case-insensitively after merging.
Both fields follow the same inheritance/replacement rules: workspace additions replace
global additions, not append to them. Keep your shared baseline in global
`allowed_hosts`. To deny all application internet access, set **both** lists to
`[]` and keep `unrestricted = false`. Neither list expands a secret's credential
substitution scope.

Before a new or changed workspace config is used, an interactive launch shows its
contents and the effective settings, and asks for approval (default: no).
Credential values are not read during this review. Approval authorizes all
settings, including host credential files and substitution destinations.
Unapproved configs fail in non-interactive launches. Invalid configs fail before
approval. An absent workspace config needs no approval.

Approvals are stored outside the workspace at
`$XDG_STATE_HOME/opencode-sandbox/trust.json`, defaulting to
`~/.local/state/opencode-sandbox/trust.json`. Each record binds the canonical
absolute workspace path to the SHA-256 hash of the exact config bytes. Any change,
including comments, or a moved/copied checkout requires approval again. Symlink
aliases of the same directory share approval. The directory uses mode `700`, the
file mode `600`, and writes replace the file atomically. Malformed trust storage
fails launch. Delete a workspace's entry (or the trust file) to require approval again.

The launcher uses the same bytes it reviewed, even if the file changes while the
prompt is open. Approval is checked before loading credentials or starting the VM,
including when resuming an existing VM. Existing VM policies and credentials still
require recreation to change, as described below.

## Port mappings

Expose a development server to your host browser with explicit TCP mappings:

```toml
[[ports]]
host = 3000
guest = 8080
```

Run the application on `0.0.0.0:8080` inside the VM, then open
`http://127.0.0.1:3000` on the host. HTTP, HTTPS, WebSockets, and hot reload
work over the TCP forwarding; HTTPS still requires trusting your application's
certificate. Host listeners bind only to `127.0.0.1`, never your LAN.

No ports are exposed by default. Both fields are required, in the range 1–65535;
host ports must be unique and available when starting the VM. Multiple mappings
are supported. Workspace `ports` replaces the entire global list; top-level
`ports = []` disables inherited mappings. Workspace changes require normal config
approval. Outbound network restrictions are unchanged.

Mappings last while the VM runs and release their host ports when it stops.
Recreate the VM after changing mappings (`opencode-sandbox remove`, then launch).
Snapshot restores apply the current configuration; snapshot preparation does not
bind host ports. For Docker applications, also publish the container port to the
VM (for example, `docker run -p 8080:80 ...`). UDP, LAN binding, automatic port
selection, and coordination between worktrees are not supported.

## Launch behaviour

The writable workspace is the exact current directory after resolving symlinks.
Git checkouts and linked worktrees retain required Git metadata mounts when
launched at their root; launching in a repository subdirectory mounts only that
subdirectory, not the ancestor repository or its Git metadata.

For safety, filesystem/volume mount roots, user homes, ancestors of your home,
and common broad directories (`/home`, `/Users`, `/Volumes`, `/mnt`, `/media`,
temporary-directory roots, `/opt`, `/srv`, `/var`, `/private`, `/Library`, and
`/Applications`) cannot be workspaces. Sensitive system trees such as `/etc`,
`/usr`, `/System`, `/proc`, `/sys`, and `/dev` are also blocked, including their
descendants and canonical macOS aliases. Choose a project subdirectory instead;
there is no bypass flag. This also rejects a dedicated project mounted directly
at a filesystem root. Explicit read-only mounts and required Git metadata retain
their separate validation rules. These checks prevent common mistakes, not
arbitrarily large projects or sensitive files within an otherwise allowed project.

AWS role access is enabled per launch with `--aws-role` and optional
`--aws-profile`, not with global configuration. See [AWS credentials](aws.md).

The launcher always stops its VM on exit, including errors and handled termination
signals. Installed tools and guest files persist for the next launch. A newly
created/restored VM is removed if initial file copying or environment setup fails.
Forced termination (`SIGKILL`) or a host crash cannot run cleanup.

### Read-only host mounts

Explicitly expose host files or directories with `[[files]]` entries in global
`config.toml` or an approved workspace `.opencode-sandbox.toml`. No extra host files,
OpenCode configuration, instructions, or skills are shared by default.

```toml
[[files]]
source = "~/.config/opencode/AGENTS.md"
target = "/root/.config/opencode/AGENTS.md"

[[files]]
source = "~/.config/opencode/skills"
target = "/root/.config/opencode/skills"

[[files]]
source = "~/.config/opencode-sandbox/opencode.json"
target = "/etc/opencode/opencode.json"

[[files]]
source = "~/.config/opencode/cli.json"
target = "/root/.config/opencode/cli.json"

[[files]]
source = "~/.config/opencode-sandbox/gitconfig"
target = "/root/.gitconfig"
mode = "copy"
```

Sources must exist as regular files or directories. `~/` expands to the host
home directory; relative sources resolve against the configuration file supplying
the list. Targets must be clean absolute guest paths. Missing sources fail launch
before VM startup, with the entry index and host path. There are no globs or
optional mappings. The default `mode = "mount"` makes files and directories read-only, even for guest
root; mounting one file does not expose its host siblings. Mounting a directory
exposes its contents, including skill scripts and references. Read-only prevents
guest modification, **not reading or exfiltration**. Host edits remain visible;
OpenCode's own reload behavior still applies. Its settings UI cannot save to a
read-only `cli.json`.

File arrays replace, rather than append, across configuration layers. Use
`files = []` at the top level of a workspace config to disable inherited mappings.
New/changed workspace configuration requires the normal approval, which authorizes
the displayed host paths. Duplicate or nested targets and overlaps with the
workspace, Git metadata, Docker storage, or runtime mounts are rejected.

Mappings apply at VM creation; recreate the VM after adding, removing, or changing
them. Configured mounts are also available during snapshot preparation and are
explicitly rebound on restore; their host contents are not copied into the snapshot.
Recreate a prepared baseline when removing/changing its required mount targets.
Each mapping consumes a runtime device, so mounts are not unlimited; a handful
of configuration/skill mounts is well below the runtime device budget.

With `mode = "copy"`, the source is copied into the guest's writable filesystem,
not mounted. Guest edits never change the host source, and host edits are not
live. Copying runs before GitHub/project initialization for fresh VMs, snapshot
preparation, and new VMs restored from snapshots, using current host contents.
Ordinary stop/start resumes do not recopy, preserving guest edits.

The target is always the exact destination: a directory's contents merge directly
into it, even if it already exists. Matching files are overwritten; destination-only
files (including snapshot files deleted from the host) remain. File/directory type
conflicts fail rather than implicitly deleting a directory. Copying is not atomic;
on failure the newly created VM is removed. Copied contents are captured in
snapshots, so removing a copy entry does not erase its previously captured files.
Unlike mounts, copies do not consume permanent mount devices.

OpenCode interprets the mounted paths normally. `/etc/opencode/opencode.json` is
system configuration and overrides project settings; mount at
`/root/.config/opencode/opencode.json` instead for user-level configuration.
For additional sandbox-specific instructions, mount a separate Markdown file
and reference its **guest path** from your mounted OpenCode configuration:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "instructions": ["/etc/opencode/sandbox-instructions.md"]
}
```

The launcher does not load or copy `opencode.json` / `cli.json` implicitly and has
no `--config` / `--cli-config` flags. Authentication remains separate.

### VM management

New VMs carry the label `opencode-sandbox` for bulk management. Run from any
directory, without loading launch configuration or secrets:

```sh
opencode-sandbox list
opencode-sandbox remove
```

`ls` is an alias for `list`; `rm` is an alias for `remove`.

`list` shows names and states of all managed VMs, including stopped VMs. `remove`
stops and deletes only the current directory's VM without confirmation. Use
`remove --all` to delete every managed VM, attempting every target and exiting
nonzero if any removal fails. No matches is a successful no-op. Unlabeled VMs
are excluded. Removal deletes guest-local state, not mounted host directories.

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

Set `vm.image` in global `config.toml` or workspace `.opencode-sandbox.toml` to select
your own image. It defaults to `opencode-sandbox:poc` and must not be empty.
Docker must be available on the host. For fresh VMs and snapshot preparation, the
launcher uses the image in your local Docker store whenever present; otherwise it
runs `docker pull`, using your Docker context and registry credentials. Registry
updates are never checked when the image exists locally; run `docker pull` yourself
to update it.

For a local image, building it is enough:

```sh
docker build -t my-custom-image:tag .
```

```toml
[vm]
image = "my-custom-image:tag"
```

Registry references work the same way, for example `ghcr.io/you/dev-sandbox:v1`.
For private registries, authenticate on the host with `docker login` first.

The launcher automatically imports Docker images into Microsandbox and caches
them under internal references derived from their full Docker image IDs. Unchanged
images reuse their import; rebuilt tags or explicitly pulled updates get a new
import. Missing cache entries are imported again. Imports use a temporary Docker
save archive, deleted afterward, and require temporary disk space for that archive.
No separate cache index is maintained.

Existing VMs resume from their persistent disks without resolving the image.
Snapshot restores likewise use the captured image. Recreate the VM and, when used,
the snapshot baseline to adopt a changed image.

The simplest compatible image extends the supplied image with
`FROM opencode-sandbox:poc`. Custom images need Bash, Git, mise, and OpenCode on
`PATH`, plus a working system CA trust store for Microsandbox's TLS inspection.
GitHub credential integration additionally requires `gh`; AWS integration requires
the AWS CLI. Docker is optional: images without `dockerd` skip daemon startup.

### Workspace snapshots

Snapshots are prepared disk baselines, automatically created on first launch for a canonical
directory, including non-Git directories. Linked worktrees have separate baselines.
Optionally configure an additional shell command globally or in the approved
directory-local `.opencode-sandbox.toml`:

```toml
[snapshot]
command = "./scripts/sandbox-prepare.sh"
```

For example, the script can run `npm ci`, `go mod download`,
`./gradlew dependencies`, and `docker compose pull`. Its command runs with
`bash -eu -c` from `/workspace`, a writable mount of the **current host directory**.
It may change host files; failure does not roll those changes back. Cache paths
must be on guest disks, outside that mount, to enter the snapshot. Scripts should
stop any background project services and remove containers/volumes they do not
want cloned, including containers with automatic restart policies.

```sh
opencode-sandbox snapshot create
opencode-sandbox snapshot list
opencode-sandbox snapshot remove
```

`create` always creates a **fresh** temporary VM from `vm.image`, starts Docker,
configures GitHub integration, trusts/installs project mise tools, and runs the
custom command if provided. Both manual and automatic snapshot preparation allow
public internet access regardless of the workspace hostname allowlist. Host,
private, link-local, and metadata destinations remain blocked; configured proxy
secrets retain their destination scope. Preparation does not import OpenAI
subscription credentials or assume a session AWS role. Mise configuration and
setup scripts are trusted: they can access mounted workspace files and send data
to public destinations during preparation. The preparation VM is stopped before
capture; restored workspace VMs enforce the configured network policy from boot.
Empty/omitted or whitespace-only `snapshot.command` skips only the custom command;
normal project initialization is still captured. There is no
incremental mode or automatic expiry/rebuild when scripts, settings, branches,
or lockfiles change.

After preparation succeeds, creation stops running containers and Docker cleanly,
flushes disks, stops the VM, and takes a native Microsandbox **disk-only** snapshot.
It explicitly selects the successful generation as the native group head before
deleting previous generations. A failed preparation leaves the old head intact.
The temporary VM is deleted on success or failure. Forced termination can leave
a temporary VM or an unpublished generation; snapshot removal also cleans up
managed leftover generations in that workspace's group.

Normal launch first resumes an existing workspace VM. Otherwise it restores the
directory's baseline if present, or uses the same fresh preparation/publication flow
as `snapshot create` before restoring when none exists. Mise initialization and the
custom command run only during snapshot preparation, never on normal launches,
restores, or restarts; Docker startup and GitHub configuration still run each launch.
Provisioning changes require an explicit `snapshot create` and workspace VM removal.
A broken baseline
is reported, not silently ignored. Workspace mounts, network policy, CPU/memory,
and current host secrets are configured for the restored VM. Root/Docker capacity,
base image and warmed disk contents come from the snapshot: recreate the baseline
to change those. Existing VMs are neither updated nor migrated.

**What is stored:** all guest root-disk changes (tools, caches, configuration,
logs, temporary files), the owned Docker disk (images/layers, containers, named
volumes, build cache), and native snapshot metadata such as base-image identity
and mount descriptions. RAM/processes and host bind-mounted contents are not
copied. In particular, `node_modules` in the workspace is not captured; missing
dependencies still need installation, ideally from warmed package-manager caches.
Any sensitive file deliberately written to guest disks is captured too; snapshots
are sensitive local state. Host-proxy credential values are not guest files and
are not copied into the snapshot by substitution alone. Captured system CA trust
can retain the preparation VM's CA in addition to the destination runtime CA.

The canonical absolute workspace path identifies a baseline. Symlink aliases
share a group; different directories, including linked worktrees, do not.
Moving a workspace changes its identity. Native group names are
`oc-workspace-<readable-name>-<path-hash>`, with uniquely named `prepared-*` members.
Labels include `opencode-sandbox`, `kind=workspace-baseline`, `workspace-name`,
`workspace-id`, and `workspace-path`. Microsandbox stores disks/metadata in its host-local
snapshot store. Group/head lookup needs no launcher JSON index. Launcher cache
contains only per-workspace lock files to prevent concurrent create/remove/restore
operations; a conflicting operation reports an error rather than racing cleanup.

`snapshot list` shows all managed workspace snapshots, with a dedicated current
workspace section and other snapshots grouped by workspace. Non-Git directories
also get a current-workspace section. Each row includes the snapshot reference,
creation time (UTC), human-readable runtime-reported size (`—` if unavailable),
and whether it is the current baseline used for new VMs. Reported size is not a
guarantee of disk space reclaimed on deletion. Listing needs no configuration
approval or credential reads and does not start VMs.

`snapshot remove` needs no configuration approval or credential reads. It deletes
only that directory's managed snapshot members, old members before the head;
repeat removal is a no-op. Existing restored VMs retain independent writable
disks and survive snapshot removal/replacement and subsequent cold starts.
The existing top-level `remove` remains VM-only and does not delete baselines.

**Microsandbox 0.7.11 restore limitations:** restored VMs require an extra
stop/configure/start cycle for labels, working directory and credentials, before
any project initialization. Restored VMs preserve the configured selected-header
substitution policy and allowed hosts through the Go secret-modification API
(available since v0.7.10), just like preparation VMs.
Restores without secrets still enable verified TLS
inspection via a temporary non-credential secret, removed before project commands.
Network diagnostics retain debug logs and cumulative collection. In v0.7.11,
restored VMs have the HTTP denial responder enabled when diagnostics are configured,
including hostname evidence for blocked HTTPS without a DNS query. Denied-host
evidence remains best effort. Ordinary destination restrictions remain
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
container ports work inside the VM. To reach them from the host, configure
an explicit [port mapping](#port-mappings) in addition to Docker's port publishing.

The built-in defaults and `docs/config.example.toml` allow public Docker Hub and
Amazon ECR Public pulls, including Docker Hub's authentication and image-download
hosts. Guest builds and containers still obey the VM network
policy: package repositories and other download hosts need separate approval.
Containers do not automatically inherit the guest's injected TLS CA; images that
make HTTPS requests through the inspected network must trust that CA themselves.

### Network policy

Both network host lists accept Microsandbox's native syntax: `example.com` matches
only that host; `.example.com` matches the base domain and all nested subdomains.
Values are passed unchanged; `*.example.com`, URLs, IPs, and ports are rejected.
Connections are limited to TCP port 443
with strict hostname checks and verified TLS inspection. Other outbound traffic
is denied, apart from DNS for allowed names through Microsandbox's gateway.
Disallowed DNS queries are rejected locally without forwarding upstream; an
empty effective allowlist also blocks external DNS lookups. The trusted resolver
can follow CNAMEs while resolving an allowed name, without granting guest access
to query the alias targets directly. Suffix entries also permit DNS queries for
all matching subdomains, so avoid suffixes containing attacker-controlled names.
Host/private networks, loopback, link-local and metadata destinations remain
blocked. Both host lists
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

Opt in globally or in an approved workspace config:

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

Print the saved JSON unchanged from the same workspace directory:

```sh
opencode-sandbox diagnostics network
opencode-sandbox diagnostics network | jq '.denied_targets'
```

This read-only command does not start a VM or load credentials. Only logging
policy and existing workspace approval are read. It works after VM deletion and
shows the latest saved snapshot, not live logs. Missing history is an error
reported on stderr with a nonzero exit.
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
credential loading and Microsandbox policy conversion remain in
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
unique environment variable name, exactly one source (`file` or a `keyring` table),
non-empty host patterns, and non-empty HTTP header names. Relative paths resolve
against the config directory supplying the `file` field (global or repo);
absolute paths are also accepted. Inline values and environment sources are not
supported. Use mode `700` on the credential directory and `600` on the file.
Symlinked paths, oversized files (over 16 KiB), empty values, multiple lines, and
NUL bytes are rejected. A trailing newline is stripped. Configured files are
validated on every launch; missing/invalid files fail launch, even for existing VMs.

### OS keyring

Use an empty table to select the host OS keyring with default identifiers:

```toml
[secrets.github]
keyring = {}
env = "GH_TOKEN"
allowed_hosts = ["github.com", "api.github.com"]
headers = ["authorization"]
```

The default service is `opencode-sandbox`; the account is the secret name (`github`
above). Override either identifier when needed:

```toml
keyring = { service = "my-company", account = "work-github" }
```

Empty or omitted identifier fields use defaults. Same service/account pairs share
one entry across projects. Workspace definitions completely replace same-named
global secrets, including keyring identifiers; defaults apply to the replacement.
`keyring = true` and configuring both `file` and `keyring` are invalid.

The launcher uses [zalando/go-keyring](https://github.com/zalando/go-keyring):
macOS Keychain on macOS, and Secret Service (normally GNOME Keyring) on Ubuntu.
Ubuntu requires a running user D-Bus session and an available keyring; headless/SSH
sessions may need additional setup. The OS may prompt to unlock or authorize access.
There is no automatic fallback to a file. Missing entries and access failures stop
launch, even for existing VMs. Values must be non-empty single-line strings without
NUL bytes, at most 16 KiB; providers may impose smaller storage limits.

Manage keyring entries independently of launcher configuration, from any directory:

```sh
opencode-sandbox secret set github
opencode-sandbox secret remove github # alias: rm
opencode-sandbox secret list # alias: ls
```

These commands read only logging policy and existing workspace approval; they do
not change configuration, request workspace approval, or launch a VM. The default
service namespace is `opencode-sandbox`, and the account is the supplied name, whether or not that name
is configured (including names currently configured as file-backed).
Use `--service` to select another namespace, and `--account` on `set`/`remove` to
override the account when using custom identifiers:

```sh
opencode-sandbox secret set github --service my-company --account work-github
opencode-sandbox secret list --service my-company
```

`list` enumerates stored account names in the selected namespace, sorted and
quoted for safe display; it does not retrieve or print credential values and
does not list configured file sources. Listing uses an upstream go-keyring commit
pinned in `go.mod`, because the latest tagged release lacks enumeration.
`set` requires a terminal and prompts for hidden input; it stores or replaces the
entry without accepting the value as a command argument or printing it.
`remove` asks for confirmation and warns that the entry may be shared across projects.
A missing entry is harmless after confirmation. No command prints resolved secrets.
To use a stored entry on launch, separately configure `keyring = {}` (or the matching
custom service/account) instead of `file` in the secret definition.

### Proxy scope and persistence

Secret host patterns use Microsandbox's native secret syntax, different from the
network lists: `example.com` matches exactly; `*.example.com` matches the base
domain and all nested subdomains. Values are passed unchanged. A leading dot
(`.example.com`) and unrestricted `*` are rejected for secrets. Wildcards authorize
every matching subdomain to receive the credential; scope them carefully.

Guests receive placeholders such as `$MSB_GH_TOKEN`, never the source value.
Microsandbox substitutes values only in the selected headers for the selected
hosts over verified TLS. Secret scopes never grant network access: the hosts
must independently be allowed by `[network]`. Configuring `GH_TOKEN` also enables
GitHub's guest HTTPS Git credential helper. OpenAI subscription authentication
remains separate and unchanged.

Real values persist in Microsandbox's **host-side VM configuration**. Never log
resolved secrets or commit credential files. Keyring storage protects the source
credential but does not eliminate the copy in host-side VM configuration.
Allowed upstreams receive real values and could reflect them back; this is not
an absolute secrecy guarantee.
Only new VMs receive changes: recreate a VM to rotate or remove its stored
credentials. Updating/deleting a keyring entry does not update existing VMs.
Removing a secret definition stops loading its source, but does not revoke
credentials in existing VMs. Revoke credentials upstream for immediate revocation.

## Launcher logging

Host launcher logs are written as JSON at Info and above to rotating private
logfiles. Console logs are human-readable and go to stderr, never stdout.

```toml
[logging]
console_level = "warn" # debug, info, warn, or error
```

`console_level` is a minimum severity and defaults to `warn`. Override it for one
invocation with `opencode-sandbox --console-log-level debug`; this shared flag
also works with subcommands. Precedence: CLI flag, approved workspace config,
global config, built-in default. Unapproved workspace settings do not affect
management commands. Startup uses the default until settings are loaded, unless
the flag is supplied. File logging is unaffected by the console threshold.
