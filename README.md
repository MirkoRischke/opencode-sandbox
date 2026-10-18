# opencode-sandbox

Run OpenCode in an isolated VM for your Git worktree, powered by
[Microsandbox](https://microsandbox.dev) under the hood. VMs stop on exit and
retain installed tools and guest files for the next launch.

## Features

- **Network allowlist:** allow common development services over HTTPS by default;
  customize allowed hosts or deny all application internet access.
- **Custom credentials:** configure file-backed secrets, injected into selected
  HTTPS headers by the host proxy without exposing their values to the guest.
- **Repository configuration:** override global settings with a repo-root
  `.opencode-sandbox.toml`; new or changed configs require approval.
- **AWS roles:** assume a role on the host and provide automatically refreshed,
  temporary credentials to the guest. Unlike proxy secrets, these are readable
  by guest code.
- **Guest Docker:** build and run images with Docker Engine, Buildx, and Compose.
  Each VM has its own persistent Docker data disk; container ports stay inside
  the VM. See [Docker configuration](docs/config.md#docker-inside-the-vm).
- **Prepared repo snapshots:** warm tools, dependency caches, and Docker images
  once; new worktree VMs restore independent copies of the repo baseline.

## How it works

The custom Debian image comes with pre-installed tools (see
[Dockerfile](Dockerfile)). Docker builds the image; Microsandbox runs it as a VM.

On launch, the host launcher:

1. Finds the Git worktree and creates or resumes its dedicated VM, mounting the
   worktree and required Git metadata. Changes to the Git repository inside the
   sandbox are immediately visible on the host—no syncing or copying needed.
2. Configures credentials and trusts mise configuration, automatically running
   `mise install` when the project has mise configuration. Tool downloads require
   the relevant hosts to be allowed by the network policy.
3. Configures guest OpenCode, prepares authentication (including optional AWS
   role credentials), and attaches your terminal to OpenCode.

**The OpenCode TUI runs inside the guest**, alongside its tools and commands—not
on the host. Exiting stops the VM without deleting its persistent disk.

## Setup

With mise, Docker, and Microsandbox available, run from this repository:

```sh
mise install
mise run install # Install the launcher to ~/.local/bin
mise run load    # Build and import the sandbox image
```

## Usage

```sh
opencode-sandbox        # Launch from inside a Git worktree
opencode-sandbox list   # List managed VMs (alias: ls)
opencode-sandbox remove # Delete this worktree's VM without confirmation (alias: rm)
opencode-sandbox remove --all # Delete ALL managed VMs without confirmation
opencode-sandbox --help # Show launch options
opencode-sandbox config default # Print documented launcher defaults
opencode-sandbox diagnostics network # Print saved network diagnostics JSON for this worktree
opencode-sandbox snapshot create # Prepare a fresh VM and replace this repo's baseline
opencode-sandbox snapshot list   # List all repo snapshots; current repo gets its own section
opencode-sandbox snapshot remove # Delete this repo's baseline, leaving VMs untouched
opencode-sandbox snapshot remove --all # Delete ALL managed repo snapshots without confirmation
```

Configure network access and secrets in [launcher configuration](docs/config.md).
New VMs use a built-in development-service allowlist by default. See also
[GitHub access](docs/github.md) and [AWS credentials](docs/aws.md).

### Prepared repository snapshots

Define a preparation command in the repo's `.opencode-sandbox.toml`:

```toml
[snapshot]
command = "./scripts/sandbox-prepare.sh"
```

Run `opencode-sandbox snapshot create` to prepare a fresh VM and capture its
guest disks, including Docker images.

See [snapshot configuration and limitations](docs/config.md#repository-snapshots).

### Shell completion

Cobra provides completion scripts for Bash, Zsh, Fish, and PowerShell. Run
`opencode-sandbox completion <shell> --help` for installation instructions.

For Zsh, add this after completion initialization (`compinit`) in `~/.zshrc`:

```zsh
source <(opencode-sandbox completion zsh)
```

Zsh normally also completes a simple alias such as `alias ocs=opencode-sandbox`.
For Bash, add this to `~/.bashrc` after loading bash-completion and defining the alias:

```bash
source <(opencode-sandbox completion bash)
complete -o default -F __start_opencode-sandbox ocs
```

## Limitations

- **Incomplete network diagnostics after snapshot restore:** with Microsandbox
  v0.7.8, restored VMs lose the HTTP denial responder required to log blocked
  HTTPS destinations. Network blocking still works, but those requests are
  missing from `diagnostics network`. Denials collected during snapshot
  preparation remain in the report. Use a fresh VM without snapshot restore
  when complete blocked-HTTPS diagnostics are needed.
- **OpenAI-only automatic authentication:** the launcher imports an active host
  OpenAI subscription OAuth credential. Other providers aren't explicitly
  blocked, but their authentication isn't set up automatically; a fresh VM
  currently requires the OpenAI import to succeed.
- **Model credentials are exposed to the guest:** imported OpenAI credentials
  are stored inside the VM and can be read by sandbox code. They don't use the
  host-proxy protection provided for custom secrets.

## Development

```sh
mise run fmt  # Format Go code
mise run lint
mise run test:unit    # Fast internal tests; no VMs or external services
mise run test:prepare # Build/import images once; repeat after image changes
mise run test        # VM/integration tests using the prepared images
mise run test:live   # Opt-in OpenAI smoke test; uses host credentials
```
