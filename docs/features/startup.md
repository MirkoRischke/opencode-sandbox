# Works with any coding agent

OpenCode starts by default. To use another terminal-based coding agent, make
sure it’s installed in your [VM image](../config/README.md#custom-vm-images).
Set the command in `.opencode-sandbox.toml` (or in your global launcher config):

```toml
[vm]
image = "my-sandbox-image:latest"

[startup]
command = ["copilot"]
# Optional: use the agent name recognized by your Herdr installation.
# herdr_agent = "copilot"
```

Copilot is just an example; the stock image still ships OpenCode. The first
element of `command` is the executable; subsequent elements are passed as
literal arguments. The launcher runs it in the guest workspace when you open
the terminal. After it exits, you land in interactive Bash in the same
workspace. Exit Bash to stop the VM.

Startup settings take effect on the next launch, even with an existing VM.
If you switch images to install the agent, rebuild the snapshot and recreate
the VM so it gets the new image.

In a Herdr pane, an explicit `HERDR_AGENT` environment variable on the host
takes precedence over `startup.herdr_agent`. With neither set, the default
command `["opencode"]` gets the `opencode` hint; other commands get no hint.
The hint only controls Herdr’s pane detection, not what runs in the VM.

The [LiteLLM gateway](../../llm-gateway/README.md) is still available regardless
of the startup command. Configure the agent to use the gateway’s guest URL
(`http://host.microsandbox.internal:4000/v1` by default) and `LLM_GATEWAY_KEY`
placeholder if it supports an OpenAI-compatible provider.
