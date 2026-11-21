# Works with any coding agent

OpenCode starts by default. To use another terminal-based coding agent, make
sure it’s installed in your [VM image](../config/README.md#custom-vm-images).
Set the command in `.opencode-sandbox.toml` (or in your global launcher config):

```toml
[vm]
image = "my-sandbox-image:latest"

[startup]
command = ["copilot"]
```

Copilot is just an example; the stock image still ships OpenCode. The first
element of `command` is the executable; subsequent elements are passed as
literal arguments. The launcher runs it in the guest workspace when you open
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
