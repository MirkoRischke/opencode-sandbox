# Features

## Workspaces and agents

- [Workspace VMs](workspaces.md): run each directory or Git worktree in its own
  VM, with live host editing and guest disks that survive restarts.
- [Coding agents](coding-agents.md): use OpenCode, Codex CLI, Copilot CLI, or
  another agent installed in your image.
- [Launcher configuration](../config/README.md): set defaults globally and
  approve directory-local overrides.
- [Host file mappings](host-files.md): mount selected host files read-only or
  copy them into writable guest storage.
- [Sandbox instructions](sandbox-instructions.md): tell your agent about its
  workspace and network restrictions.

## Images and storage

- [Prepared workspace snapshots](snapshots.md): warm mise tools, dependency
  caches, and Docker images once across linked worktrees.
- [Custom VM images](vm-images.md): bring a local Docker or registry image.
- [Guest Docker](guest-docker.md): build and run containers on a private VM disk.
- [Persistent workspace state](persistent-state.md): keep selected guest
  directories, such as agent sessions, across VM removal.

## Connectivity and credentials

- [Sandbox networking](networking.md): configure internet access and TCP
  connections between host and guest.
- [Host LLM gateway](llm-gateway.md): sign in on the host and track usage by
  repository and VM without putting real gateway keys in the guest.
- [Proxied secrets](secrets.md): use file- or keyring-backed credentials without
  sharing their values with the guest.
- [GitHub access](github.md): authenticate `gh` and HTTPS Git with a scoped token.
- [AWS roles](aws.md): provide temporary role credentials that refresh from the host.

## Troubleshooting and cleanup

- [Diagnostics and logs](diagnostics.md): find launcher logs and inspect
  blocked network destinations.
- [Resource cleanup](cleanup.md): list and remove VMs or prune workspace and
  repository resources.
