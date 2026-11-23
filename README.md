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

Run OpenCode (or [another coding agent](docs/features/coding-agents.md)) in an isolated
VM for your current directory. [Microsandbox](https://microsandbox.dev) powers
the VM to reduce the risk of damaging files or accessing sensitive information
on your host.

## Why this tool exists

I wanted a safer way to let my agents run without reviewing every tool call they
want to make. See [the full motivation](docs/motivation.md) for my workflow and
the problems that led to this tool.

## Accepted risks

Read the [accepted risks](docs/risks.md) before using this tool.

## Quickstart

1. Install [mise](https://mise.jdx.dev) and [Docker with Compose](https://docs.docker.com/compose/install/). Start Docker.
2. Clone this repo and run `mise install`, `mise run install`, and `mise run build` from the checkout.
3. Make sure `~/.local/bin` and `msb` are on `PATH` outside the checkout. With [mise activated](https://mise.jdx.dev/shell-setup.html), run `mise use --global microsandbox` (see [mise.toml](mise.toml) for the pinned version).
4. [Add a model](docs/features/llm-gateway.md#add-a-model) to the host LLM gateway.
5. [Authenticate with your provider](docs/features/llm-gateway.md#authenticate-on-the-host).
6. [Configure OpenCode and map its config into the VM](docs/features/llm-gateway.md#configure-opencode).
7. Run `opencode-sandbox` from your project directory.

## Features

See the [feature overview](docs/features/README.md).

## How it works

Docker builds the image and Microsandbox runs it. The launcher resumes a
[workspace VM](docs/features/workspaces.md) or restores one from a
[prepared snapshot](docs/features/snapshots.md). **The agent and its tools run
inside the guest**, while edits to the mounted workspace appear on the host.

## Usage

```sh
opencode-sandbox                   # Launch for the current directory (Git optional)
opencode-sandbox --help            # Show launch options and commands
opencode-sandbox config default    # Print documented launcher defaults
opencode-sandbox config validate   # Validate without launching or approving
opencode-sandbox config show       # Print merged launcher config and its sources
opencode-sandbox diagnostics logs  # Find this workspace's launcher logs
opencode-sandbox snapshot create   # Rebuild the prepared baseline
opencode-sandbox list              # List managed VMs
opencode-sandbox remove            # Delete this workspace's VM without confirmation
opencode-sandbox prune --dry-run   # Preview broader workspace cleanup
```

For more commands and examples, see [configuration](docs/config/README.md),
[proxied secrets](docs/features/secrets.md), [diagnostics](docs/features/diagnostics.md),
[snapshots](docs/features/snapshots.md), and [resource cleanup](docs/features/cleanup.md).

## Limitations

See the [known limitations](docs/limitations/README.md).

## Development

See the [development guide](docs/development/README.md) for testing, quality checks,
and upgrading Microsandbox.
