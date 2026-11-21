# Host LLM gateway

The launcher runs a shared LiteLLM gateway on the host. Coding agents in the VMs
send model requests to the gateway instead of talking directly to the provider.

- **Keep LLM secrets out of the VM.** Provider credentials and real gateway API
  keys stay on the host. The VM gets a placeholder that Microsandbox replaces
  with the real key only in the authorization header on requests to the host
  gateway. The VM can make model requests without reading the key.
- **Sign in once on the host.** VMs share the gateway's provider login, so
  neither new VMs nor restarts need a separate login. Sign in again if the
  provider login expires or is revoked.
- **Switch providers in one place.** Configure your coding agent to use the
  gateway once. As long as the gateway model ID and API stay the same, you can
  switch upstream providers without changing the agent or the launcher.
- **Track usage by repo and VM.** Each VM gets its own key, and linked Git
  worktrees share a repository team. Use the gateway to see usage per VM or
  across the team. You can also configure spend limits in LiteLLM; the launcher
  doesn't set them automatically.

The gateway starts automatically on sandbox launch and keeps running after VMs
exit. Stopping it interrupts model access for all VMs using it.

## Use the key from inside the VM

The launcher automatically sets the `LLM_GATEWAY_KEY` environment variable
inside the VM. It contains a placeholder, not the real API key. No
`[secrets.llm_gateway]` entry or manual export is needed.

For other tools, set the base URL to `http://host.microsandbox.internal:4000/v1`
and use the value of `LLM_GATEWAY_KEY` as the bearer token. Use HTTP, not
WebSocket: Microsandbox substitutes the real key in the HTTP `Authorization`
header on the host gateway route.

## Configure OpenCode

First [configure a Responses model in LiteLLM and authenticate on the
host](../../llm-gateway/README.md). Then create
`~/.config/opencode-sandbox/opencode.json` on the host (or merge these settings
into your existing file):

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

The default network settings allow host port `4000`. If you override
`[network.vm_to_host]`, include `4000` in `ports`. Recreate an existing VM after
changing its file or network mappings.
