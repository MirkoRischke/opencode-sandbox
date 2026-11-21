# Features

- **[Works with any coding agent](coding-agents.md):** launch an agent already
  installed in the VM image. OpenCode remains the default.
- **[Host LLM gateway](llm-gateway.md):** sign in once on the host, keep LLM
  credentials out of VMs, and track usage by repository and VM.
- **Per-directory environments:** each directory, including Git worktrees, gets
  its own VM so parallel tasks can use separate dependencies and the same guest
  ports without conflicts.
- **Live host editing:** workspace changes are immediately visible on the host,
  so you can use your usual editor without syncing files.
- **Persistent environments:** VMs stop when you exit the guest shell, but
  retain installed tools and guest files for the next launch.
- **Resource cleanup:** prune the current worktree, a repo, or all managed
  workspaces, including history, snapshots, and logs. Global cleanup also removes
  all MSB images and prunes runtime caches. See [cleanup](cleanup.md).
- **[Persistent workspace state](persistent-state.md):** opt in to keeping selected
  guest directories, such as agent sessions, across VM removal.
- **Project tools:** automatically install tools declared in the directory's
  `mise.toml` during workspace snapshot preparation.
- **Custom VM images:** use a custom Docker/OCI image with your required tools
  preinstalled. See [custom VM images](../config/README.md#custom-vm-images).
- **Network allowlist:** allow common development services over HTTPS by default;
  customize allowed hosts or deny all application internet access.
- **Custom credentials:** configure file- or OS-keyring-backed secrets, injected into
  HTTP headers by the host proxy without storing their values in the guest.
  Verified TLS is required unless a secret explicitly opts into HTTP, as for the
  [host LLM gateway](llm-gateway.md).
- **Workspace configuration:** override global settings with a directory-local
  `.opencode-sandbox.toml`; new or changed configs require approval.
- **Host file mappings:** explicitly mount selected files or directories read-only,
  or copy them into writable guest storage on creation and snapshot restore,
  including OpenCode configuration, instructions, and skills. No extra host files
  are shared by default beyond the workspace and required Git metadata.
  See [`[[files]]` configuration](../config/README.md#read-only-host-mounts).
- **[Sandbox instructions](sandbox-instructions.md):** opt in to loading a
  launcher-generated description of the guest environment in your agent.
- **AWS roles:** assume a role on the host and provide automatically refreshed,
  temporary credentials to the guest. See [Accepted risks](../risks.md).
- **Guest Docker:** build and run images with Docker Engine, Buildx, and Compose.
  Each VM has its own persistent Docker data disk.
  See [Docker configuration](../config/README.md#docker-inside-the-vm).
- **Port mappings:** access servers running inside the VM from your host using
  explicit loopback-only [TCP port mappings](../config/README.md#directional-port-access).
- **Host service access:** reach explicitly allowed host TCP ports from inside the
  VM at `host.microsandbox.internal:<port>` (for example,
  `http://host.microsandbox.internal:3000`). Only TCP port 4000 (the shared LLM
  gateway) is reachable by default. See
  [directional port access](../config/README.md#directional-port-access).
- **Prepared workspace snapshots:** warm tools, dependency caches, and Docker images
  once; linked Git worktrees restore one shared repository baseline into private
  VMs. Outside Git, baselines belong to individual directories.
