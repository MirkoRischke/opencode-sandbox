# Features

- **Network allowlist:** allow common development services over HTTPS by default;
  customize allowed hosts or deny all application internet access.
- **Custom credentials:** configure file- or OS-keyring-backed secrets, injected into
  HTTPS headers by the host proxy without storing their values in the guest.
  [Snapshot restore limitations](config.md#workspace-snapshots) apply to header selection.
- **Workspace configuration:** override global settings with a directory-local
  `.opencode-sandbox.toml`; new or changed configs require approval.
- **Host file mappings:** explicitly mount selected files or directories read-only,
  or copy them into writable guest storage on creation and snapshot restore,
  including OpenCode configuration, instructions, and skills. No extra host files
  are shared by default. See [`[[files]]` configuration](config.md#read-only-host-mounts).
- **Sandbox-aware instructions:** OpenCode gets information about the sandbox
  it is running in.
- **AWS roles:** assume a role on the host and provide automatically refreshed,
  temporary credentials to the guest. See [Accepted risks](risks.md).
- **Guest Docker:** build and run images with Docker Engine, Buildx, and Compose.
  Each VM has its own persistent Docker data disk. Expose development servers to
  your host browser using explicit loopback-only [TCP port mappings](config.md#port-mappings).
  See [Docker configuration](config.md#docker-inside-the-vm).
- **Prepared workspace snapshots:** warm tools, dependency caches, and Docker images
  once; recreated VMs restore the directory's baseline, with or without Git.
