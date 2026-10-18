# opencode-sandbox

Run OpenCode in an isolated VM for your Git worktree, powered by
[Microsandbox](https://microsandbox.dev) under the hood. VMs stop on exit and
retain installed tools and guest files for the next launch.

## Features

- **Network blocking:** deny application internet access by default; allow only
  explicitly configured HTTPS hosts.
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
opencode-sandbox remove # Delete ALL managed VMs without confirmation (alias: rm)
opencode-sandbox --help # Show launch options
opencode-sandbox config default # Print documented launcher defaults
```

Configure network access and secrets in [launcher configuration](docs/config.md).
New VMs deny application internet access by default. See also
[GitHub access](docs/github.md) and [AWS credentials](docs/aws.md).

## Limitations

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
mise run test # Build, import, and smoke-test in a fresh VM
```
