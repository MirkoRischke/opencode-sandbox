# Features

- **Per-directory environments:** each directory, including Git worktrees, gets
  its own VM so parallel tasks can use separate dependencies and the same guest
  ports without conflicts.
- **Live host editing:** workspace changes are immediately visible on the host,
  so you can use your usual editor without syncing files.
- **Persistent environments:** VMs stop when OpenCode exits, but retain installed
  tools and guest files for the next launch.
- **Persistent OpenCode history (experimental):** a checkout and its linked Git
  worktrees share a host-backed OpenCode database, independent of VM disks and
  snapshots. See [storage and concurrency risks](opencode-history.md).
- **Project tools:** automatically install tools declared in the directory's
  `mise.toml` during workspace snapshot preparation.
- **Custom VM images:** use a custom Docker/OCI image with your required tools
  preinstalled. See [custom VM images](../config/README.md#custom-vm-images).
- **Network allowlist:** allow common development services over HTTPS by default;
  customize allowed hosts or deny all application internet access.
- **Custom credentials:** configure file- or OS-keyring-backed secrets, injected into
  HTTPS headers by the host proxy without storing their values in the guest.
  [Snapshot restore limitations](../config/README.md#workspace-snapshots) apply to header selection.
- **Workspace configuration:** override global settings with a directory-local
  `.opencode-sandbox.toml`; new or changed configs require approval.
- **Host file mappings:** explicitly mount selected files or directories read-only,
  or copy them into writable guest storage on creation and snapshot restore,
  including OpenCode configuration, instructions, and skills. No extra host files
  are shared by default beyond the workspace, required Git metadata, and managed
  OpenCode database. See [`[[files]]` configuration](../config/README.md#read-only-host-mounts).
- **Sandbox-aware instructions:** OpenCode gets information about the sandbox
  it is running in.
- **AWS roles:** assume a role on the host and provide automatically refreshed,
  temporary credentials to the guest. See [Accepted risks](../risks.md).
- **Guest Docker:** build and run images with Docker Engine, Buildx, and Compose.
  Each VM has its own persistent Docker data disk.
  See [Docker configuration](../config/README.md#docker-inside-the-vm).
- **Port mappings:** access servers running inside the VM from your host using
  explicit loopback-only [TCP port mappings](../config/README.md#directional-port-access).
- **Host service access:** reach explicitly allowed host TCP ports from inside the
  VM at `host.microsandbox.internal:<port>` (for example,
  `http://host.microsandbox.internal:3000`). No host service ports are reachable by
  default. See [directional port access](../config/README.md#directional-port-access).
- **Prepared workspace snapshots:** warm tools, dependency caches, and Docker images
  once; recreated VMs restore the directory's baseline, with or without Git.
