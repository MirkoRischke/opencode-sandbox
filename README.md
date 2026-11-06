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
[Microsandbox](https://microsandbox.dev) under the hood. VMs stop on exit and
retain installed tools and guest files for the next launch.

## Features

- **Network allowlist:** allow common development services over HTTPS by default;
  customize allowed hosts or deny all application internet access.
- **Custom credentials:** configure file- or OS-keyring-backed secrets, injected into
  HTTPS headers by the host proxy without storing their values in the guest.
  [Snapshot restore limitations](docs/config.md#workspace-snapshots) apply to header selection.
- **Workspace configuration:** override global settings with a directory-local
  `.opencode-sandbox.toml`; new or changed configs require approval.
- **Host file mappings:** explicitly mount selected files or directories read-only,
  or copy them into writable guest storage on creation and snapshot restore,
  including OpenCode configuration, instructions, and skills. No extra host files
  are shared by default. See [`[[files]]` configuration](docs/config.md#read-only-host-mounts).
- **Sandbox-aware instructions:** OpenCode gets information about the sandbox
  it is running in.
- **AWS roles:** assume a role on the host and provide automatically refreshed,
  temporary credentials to the guest. Unlike proxy secrets, these are readable
  by guest code.
- **Guest Docker:** build and run images with Docker Engine, Buildx, and Compose.
  Each VM has its own persistent Docker data disk. Expose development servers to
  your host browser using explicit loopback-only [TCP port mappings](docs/config.md#port-mappings).
  See [Docker configuration](docs/config.md#docker-inside-the-vm).
- **Prepared workspace snapshots:** warm tools, dependency caches, and Docker images
  once; recreated VMs restore the directory's baseline, with or without Git.

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

## Setup

With mise, Docker, and Microsandbox available, run from this repository:

```sh
mise install
mise run install # Install the launcher to ~/.local/bin
mise run build   # Build the sandbox image; imported automatically on first use
```

### Upgrading Microsandbox

Upgrade the SDK and runtime together from this repository:

```sh
mise run upgrade-microsandbox       # Resolve the latest stable runtime version
mise run upgrade-microsandbox 0.7.9 # Or select an exact version
# Dotfiles default to ../dotfiles; override with --dotfiles /path/to/dotfiles
```

The task updates the SDK dependency and local runtime pin, runs `mise run verify`
with that runtime, installs the launcher, then updates the Microsandbox pin in
`home/private_dot_config/mise/config.toml.tmpl` in your dotfiles repository.
Review both repositories and apply your Mise configuration through Chezmoi to
select the matching runtime globally. The global pin must be an exact version,
not `latest`, to prevent unattended upgrades from breaking SDK compatibility.
Failures leave changes for inspection; the task does not commit or roll back.

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

Configure network access and secrets in [launcher configuration](docs/config.md).
New VMs use a built-in development-service allowlist by default. See also
[GitHub access](docs/github.md) and [AWS credentials](docs/aws.md).
Custom local Docker images and registry images are supported through
[`vm.image`](docs/config.md#custom-vm-images); Docker is required on the host.

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

See [snapshot configuration and limitations](docs/config.md#workspace-snapshots).

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

- **Host execution through shared files:** the workspace and required Git metadata
  are writable from the guest. This preserves live host editing and review, but
  guest code can modify files that host tools later execute—for example, mise or
  direnv configuration, Git hooks/configuration, and editor or language-server
  configuration. Linked worktrees may share affected Git metadata with other
  checkouts. The VM does not protect against execution triggered by host tools
  consuming these changes. Review changes before trusting or running them;
  automatic host tooling may execute them before review.
- **Incomplete network diagnostics after snapshot restore:** with Microsandbox
  v0.7.8, restored VMs lose the HTTP denial responder required to log blocked
  HTTPS destinations when requests bypass DNS. Network blocking and DNS-denial
  diagnostics still work, but those HTTPS-only denials are missing from
  `diagnostics network`. Denials collected during snapshot
   preparation remain in the report. Normal workspace VMs always restore a
   baseline, so this limitation applies to all newly created workspace VMs.
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
mise run verify      # Lint + prepare changed images + full standard test suite
mise run quality baseline # Save local complexity metrics before a refactor
mise run quality compare  # Show current hotspots and changes from that baseline
mise run test:unit    # Fast source-adjacent tests; no VMs or external services
mise run test:non-vm  # Unit/script + CLI + image tests; Docker only, no Microsandbox
mise run test:image   # Build + offline image smoke tests; Docker only, no VMs
mise run test:prepare # Prepare images only when image inputs change
mise run test        # Prepare changed images, then run the full test suite
mise run test:live   # Opt-in OpenAI smoke test; uses host credentials
```

Image preparation tracks both Dockerfiles and the base `.dockerignore`.
Unchanged builds and imports are skipped; tests always run. After deleting
Docker or Microsandbox images, run `mise run --force test:prepare` to prepare them again.

Source-focused tests live beside their implementation. Higher-level tests are
split into `tests/cli` (no Microsandbox), `tests/image` (Docker), and `tests/vm`
(Microsandbox). PR checks run linting, build the launcher, check module consistency,
compile all tests including the `live` tag, and run `test:non-vm` with race detection.
Renovate automerges dependency updates, including majors, after checks pass;
the Microsandbox SDK is excluded because it must be upgraded with its runtime.

Quality reports are advisory and saved under `.quality/` (ignored by Git).
They require Python 3 and the pinned golangci-lint; compare with the same tool
version and configuration. See [the readability review rubric](docs/quality-review.md)
for reviewing implementation and tests alongside the metrics.
