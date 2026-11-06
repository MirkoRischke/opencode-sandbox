# opencode-sandbox

This is an independent project, not built by or affiliated with the OpenCode team.

> [!WARNING]
> This is a personal tool built for my workflow. I published it primarily as a reference and starting point. **Do not rely on this tool for anything important.**
>
> - I might abandon this project at any time.
> - This tool might contain bugs and security issues.
> - It is heavily vibe-coded. I have not reviewed all the generated code. Do not assume it is correct or secure.
> - I might introduce breaking changes at any time without advance notice. I do not follow semantic versioning.
> - I do not provide support or accept feature requests, bug reports, or pull requests. If something is missing or does not work as you need, please fork the project and adapt it yourself.

Run OpenCode in an isolated VM for your current directory, powered by
[Microsandbox](https://microsandbox.dev) under the hood to reduce the risk of damaging files or accessing sensitive information on your host machine.

## Why this tool exists

I wanted a safer way to let my agents run without reviewing every tool call they
want to make. See [the full motivation](docs/motivation.md) for my workflow and
the problems that led to this tool.

## Accepted risks

Read the [accepted risks](docs/risks.md) before using this tool.

## Features

See the [feature overview](docs/features/README.md).

## How it works

The custom Debian image comes with pre-installed tools (see
[Dockerfile](Dockerfile)). Docker builds the image; Microsandbox runs it as a VM.

On launch, the host launcher:

1. Creates or resumes a dedicated VM for the exact current directory, mounting it
   and any required Git metadata when it is a checkout or linked worktree root.
   Parent repositories/configuration are never discovered. Changes are immediately
   visible on the host—no syncing or copying needed. Drive roots, home directories,
   and common broad/system directories are rejected; use a project subdirectory.
2. If neither VM nor baseline exists, prepares a fresh snapshot with `mise install`
   (when configured) and the optional snapshot command, then restores the VM from it.
   Existing VMs are reused; otherwise the saved baseline is restored. Snapshot
   preparation has public internet access; restored VMs enforce your network policy.
3. Configures guest OpenCode, prepares authentication (including optional AWS
   role credentials), and attaches your terminal to OpenCode.

**The OpenCode TUI runs inside the guest**, alongside its tools and commands—not
on the host. Exiting stops the VM without deleting its persistent disk.

## Prerequisites

- [mise](https://mise.jdx.dev) — manages the pinned development tools and setup tasks.
- [Docker](https://www.docker.com) — with its daemon running, to build and import sandbox images.
- [Microsandbox](https://microsandbox.dev) (`msb`) — required on `PATH` when running `opencode-sandbox`,
  including outside this repository. Use the version pinned in [mise.toml](mise.toml).

## Setup

Run from this repository:

```sh
mise install
mise run install # Install the launcher to ~/.local/bin
mise run build   # Build the sandbox image; imported automatically on first use
```

## Usage

```sh
opencode-sandbox        # Launch for the exact current directory (Git optional)
opencode-sandbox list   # List managed VMs (alias: ls)
opencode-sandbox remove # Delete this directory's VM without confirmation (alias: rm)
opencode-sandbox remove --all # Delete ALL managed VMs without confirmation
opencode-sandbox --help # Show launch options
opencode-sandbox config default # Print documented launcher defaults
opencode-sandbox secret set github # Store/replace a keyring secret (hidden input; config unchanged)
opencode-sandbox secret remove github # Remove its keyring entry after confirmation (alias: rm)
opencode-sandbox secret list # List account names in the opencode-sandbox keyring namespace (alias: ls)
opencode-sandbox diagnostics network # Print saved network diagnostics JSON for this directory
opencode-sandbox diagnostics logs # Print this workspace's host log directory
opencode-sandbox snapshot create # Prepare a fresh VM and replace this directory's baseline
opencode-sandbox snapshot list   # List all snapshots; current directory gets its own section
opencode-sandbox snapshot remove # Delete this directory's baseline, leaving VMs untouched
opencode-sandbox snapshot remove --all # Delete ALL managed snapshots without confirmation
```

Configure network access and secrets in [launcher configuration](docs/config/README.md).
New VMs use a built-in development-service allowlist by default. See also
[GitHub access](docs/features/github.md) and [AWS credentials](docs/features/aws.md).
Custom local Docker images and registry images are supported through
[`vm.image`](docs/config/README.md#custom-vm-images); Docker is required on the host.

### Prepared workspace snapshots

Optionally define an additional preparation command in the directory's `.opencode-sandbox.toml`:

```toml
[snapshot]
command = "./scripts/sandbox-prepare.sh"
```

The first launch automatically prepares a baseline, including installed mise tools
and Docker images. No custom command is required. Provisioning never runs on normal
launches or restores. After provisioning configuration changes, run
`opencode-sandbox snapshot create` to rebuild, then `opencode-sandbox remove` to
recreate the workspace VM on the next launch. Baselines are not auto-invalidated.

Each canonical directory has its own VM and baseline, including separate linked
worktrees and repository subdirectories. Symlink aliases share identity.

See [snapshot configuration and limitations](docs/config/README.md#workspace-snapshots).

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

See the [known limitations](docs/limitations/README.md).

## Development

See the [development guide](docs/development/README.md) for testing, quality checks,
and upgrading Microsandbox.
