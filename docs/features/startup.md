# Works with any coding agent

OpenCode starts by default. Build the image for the agent you want:

| Agent | Build task | Image | Startup command |
| --- | --- | --- | --- |
| OpenCode | `mise run build` (or `mise run build:opencode`) | `opencode-sandbox:opencode` | `opencode` |
| OpenAI Codex CLI | `mise run build:codex` | `opencode-sandbox:codex` | `codex` |
| GitHub Copilot CLI | `mise run build:copilot` | `opencode-sandbox:copilot` | `copilot` |

The launcher imports the selected image from Docker automatically. To use a
nondefault agent, set both the image and the command in `.opencode-sandbox.toml`
or your global launcher config:

```toml
[vm]
image = "opencode-sandbox:copilot"

[startup]
command = ["copilot"]
```

For another terminal-based agent, install it in a
[custom VM image](../config/README.md#custom-vm-images) and configure it the same
way. The first element of `command` is the executable; subsequent elements are
passed as literal arguments. The launcher runs it in the guest workspace when you open
the terminal. After it exits, you land in interactive Bash in the same
workspace. Exit Bash to stop the VM.

Startup settings take effect on the next launch, even with an existing VM.
If you switch images to install the agent, rebuild the snapshot and recreate
the VM so it gets the new image.

The [LiteLLM gateway](../../llm-gateway/README.md) is still available regardless
of the startup command. Configure the agent to use the gateway’s guest URL
(`http://host.microsandbox.internal:4000/v1` by default) and `LLM_GATEWAY_KEY`
placeholder if it supports an OpenAI-compatible provider.

## Optional: agent detection in Herdr

If Herdr can't detect OpenCode behind the sandbox, set the hint on the host
when launching it. For interactive use, add this to `~/.zshrc` or `~/.bashrc`:

```sh
alias opencode-sandbox='HERDR_AGENT=opencode opencode-sandbox'
```

When launching directly without an interactive shell, use
`env HERDR_AGENT=opencode opencode-sandbox`. For a different startup agent, use
the name Herdr recognizes. The hint doesn't change what runs in the VM.
