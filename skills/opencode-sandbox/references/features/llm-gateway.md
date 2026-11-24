# Host LLM gateway

Sign in to your model provider once on the host instead of logging in again
inside every VM. The shared LiteLLM gateway keeps real keys out of the guest
and lets you track usage by repository and VM. It requires Docker with Compose.

1. [Add a model](#add-a-model)
2. [Authenticate on the host](#authenticate-on-the-host)
3. [Configure OpenCode](#configure-opencode)
4. [Connect another client](#connect-another-client)
5. [Start and stop the gateway](#start-and-stop-the-gateway)
6. [Track usage](#track-usage)

## Add a model

No model catalog is bundled. For file-based setup, create
`${XDG_CONFIG_HOME:-~/.config}/opencode-sandbox/llm-gateway/config.yaml` on the
host. For example, for a ChatGPT Responses model:

```yaml
model_list:
  - model_name: gpt-6-luna
    litellm_params:
      model: chatgpt/gpt-6-luna
    model_info:
      mode: responses
```

- `model_name` is the ID your agent uses. `litellm_params.model` is the
  provider's model. Keep the ID and API the same to switch providers without
  reconfiguring your agents.
- For Copilot, use `github_copilot/` instead of `chatgpt/`. Pick a model your
  account can access.
- File changes take effect on the next gateway start or sandbox launch.
  You can also add models in the LiteLLM admin UI.

## Authenticate on the host

```sh
opencode-sandbox gateway authenticate          # ChatGPT (default)
opencode-sandbox gateway authenticate copilot  # GitHub Copilot
```

Open the displayed URL in your host browser and enter the code. Enable
device-code login in ChatGPT settings if prompted. All VMs share the login;
repeat it if it expires or is revoked. For other providers, see
[LiteLLM's provider documentation](https://docs.litellm.ai/docs/providers).

## Configure OpenCode

After configuring a Responses model and authenticating, create
`~/.config/opencode-sandbox/opencode.json` on the host or merge these settings
into your existing file:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "model": "gateway/coder",
  "providers": {
    "gateway": {
      "name": "Host LLM gateway",
      "env": ["LLM_GATEWAY_KEY"],
      "package": "@opencode/ai/providers/openai-compatible/responses",
      "settings": {
        "baseURL": "http://host.microsandbox.internal:4000/v1",
        "apiKey": "{env:LLM_GATEWAY_KEY}",
        "transport": "http"
      },
      "models": {
        "coder": { "modelID": "gpt-6-luna" }
      }
    }
  }
}
```

Replace `gpt-6-luna` with the public `model_name` configured in LiteLLM, not the
upstream provider's model ID. `gateway/coder` is the name you select in OpenCode.
OpenCode reads the placeholder from its VM environment at runtime.

Add this mapping to `~/.config/opencode-sandbox/config.toml` on the host so
OpenCode can read the file inside the VM:

```toml
[[files]]
source = "~/.config/opencode-sandbox/opencode.json"
target = "/root/.config/opencode/opencode.json"
```

The default [VM-to-host policy](networking.md#vm-to-host) allows port `4000`.
If you override it, include the gateway port. Recreate an existing VM after
changing its file or network mappings.

## Connect another client

For other OpenAI-compatible clients, set the base URL to
`http://host.microsandbox.internal:4000/v1` and use `LLM_GATEWAY_KEY` from the
guest environment as the bearer token. Use HTTP, not WebSocket.
The variable is set automatically and contains a placeholder, not the real key.

## Start and stop the gateway

The gateway starts when you launch a sandbox and keeps running after you
exit. To start it before a launch or shut it down, run:

```sh
opencode-sandbox gateway start
opencode-sandbox gateway stop
```

Stopping interrupts model access for running VMs but keeps logins and usage
data. The next sandbox launch starts the gateway again.

To run a separate instance, set `OPENCODE_SANDBOX_GATEWAY_PROJECT` and
`OPENCODE_SANDBOX_GATEWAY_PORT`. Use separate `XDG_DATA_HOME` and
`XDG_CONFIG_HOME` directories. If you change the port, update your agent URL
and [VM-to-host ports](networking.md#vm-to-host) too.

## Track usage

See how much a repo or individual VM uses across model providers:

1. Open <http://localhost:4000/ui> from the host.
2. Sign in as `admin`. The password is `LITELLM_MASTER_KEY` in
   `${XDG_DATA_HOME:-~/.local/share}/opencode-sandbox/llm-gateway/.env`.
3. Open **Usage → Team Usage**. Choose the repository team and drill down by
   VM key. Linked worktrees share a team; independent clones do not.

Subscription costs shown in LiteLLM may not match your bill or remaining
quota. Set spend limits in LiteLLM if you need them.

Removing a VM with `opencode-sandbox remove` deletes its gateway key, not its
team or usage history. If the command reports that key deletion failed,
delete the key in LiteLLM.
