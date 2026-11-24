# Coding agents and startup command

Use the coding agent you prefer without changing how the sandbox works.
OpenCode is the default.

1. [Choose an agent](#choose-an-agent)
2. [Customize what starts](#startup-command)
3. [Help Herdr detect your agent](#optional-agent-detection-in-herdr)

## Choose an agent

Build the image for the agent you want:

| Agent | Build task | Image | Startup command |
| --- | --- | --- | --- |
| OpenCode | `mise run build` (or `mise run build:opencode`) | `opencode-sandbox:opencode` | `opencode` |
| OpenAI Codex CLI | `mise run build:codex` | `opencode-sandbox:codex` | `codex` |
| GitHub Copilot CLI | `mise run build:copilot` | `opencode-sandbox:copilot` | `copilot` |

For a nondefault agent, set both the image and the command in
`.opencode-sandbox.toml` or your global launcher config:

```toml
[vm]
image = "opencode-sandbox:copilot"

[startup]
command = ["copilot"]
```

For another terminal-based agent, install it in a
[custom VM image](vm-images.md) and configure it the same way.

## Startup command

Set `startup.command` to the executable followed by any arguments. For
example, use `["opencode", "--help"]` to show OpenCode's help on launch.
The command runs in the guest workspace. When it exits, you land in Bash.

Changing just the command takes effect on the next launch. If you switch
images, [rebuild the snapshot and recreate the VM](snapshots.md#rebuild-a-baseline).

The [host LLM gateway](llm-gateway.md#connect-another-client) is available
regardless of startup command. Use it with agents that support an
OpenAI-compatible provider.

## Optional: agent detection in Herdr

If Herdr can't detect OpenCode behind the sandbox, set the hint on the host
when launching it. For interactive use, add this to `~/.zshrc` or `~/.bashrc`:

```sh
alias opencode-sandbox='HERDR_AGENT=opencode opencode-sandbox'
```

When launching directly without an interactive shell, use
`env HERDR_AGENT=opencode opencode-sandbox`. For a different startup agent, use
the name Herdr recognizes. The hint doesn't change what runs in the VM.
