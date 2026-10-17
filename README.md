# opencode-sandbox

Run OpenCode in an isolated VM for your Git worktree, powered by
[Microsandbox](https://microsandbox.dev) under the hood. VMs stop on exit and
retain installed tools and guest files for the next launch.

## Features

- **Network blocking:** deny application internet access by default; allow only
  explicitly configured HTTPS hosts.
- **Custom credentials:** configure file-backed secrets, injected into selected
  HTTPS headers by the host proxy without exposing their values to the guest.
- **AWS roles:** assume a role on the host and provide automatically refreshed,
  temporary credentials to the guest. Unlike proxy secrets, these are readable
  by guest code.

## How it works

The custom Debian image in [Dockerfile](Dockerfile) includes OpenCode, mise,
Node.js, Git, GitHub CLI, and AWS CLI. Docker builds the image; Microsandbox runs
it as a VM.

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
```

Configure network access and secrets in [launcher configuration](docs/config.md).
New VMs deny application internet access by default. See also
[GitHub access](docs/github.md) and [AWS credentials](docs/aws.md).

## Development

```sh
mise run fmt  # Format Go code
mise run lint # Lint the Dockerfile
mise run test # Build, import, and smoke-test in a fresh VM
```
