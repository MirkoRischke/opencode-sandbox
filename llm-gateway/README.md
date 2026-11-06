# LLM gateway

The launcher runs a shared LiteLLM gateway on the host using Docker Compose.
Gateway startup overlaps snapshot/VM preparation; OpenCode starts only after the
gateway is healthy. Later launches reuse the healthy project. It stays running
after sandbox exit. Requires Docker with the Compose CLI plugin.

## Initial authentication

```sh
opencode-sandbox gateway authenticate          # ChatGPT (default)
opencode-sandbox gateway authenticate copilot  # Optional GitHub Copilot
```

These commands start the gateway if necessary, then print the provider's device
login URL/code. Complete login in your host browser. Enable device-code login in
ChatGPT security settings if requested. Credentials persist in Docker volumes;
no subscription credentials enter the VM. Expired/revoked login requires running
authentication again. Readiness checks do not verify provider authentication.

The binary embeds `compose.yaml` and `config.yaml`; no source checkout is needed.
On first use it installs these and generates an owner-only `.env` under
`$XDG_DATA_HOME/opencode-sandbox/gateway` (default
`~/.local/share/opencode-sandbox/gateway`). Existing installation files are
preserved. The Compose project is named `opencode-sandbox-gateway`.

Start explicitly with `opencode-sandbox gateway start`. To stop without deleting
credentials or usage data:

```sh
opencode-sandbox gateway stop
```

This stops the shared containers without removing them or their volumes. It
interrupts gateway access for any running sandboxes. The next sandbox launch
starts the gateway again automatically.

The repository's standalone `mise run gateway:*` tasks remain available for
development. They use this directory's `.env` and Compose files rather than the
installed copies. Use one consistent set of files/secrets for the shared project;
do not alternate independently generated `.env` files against the same database.

## Gateway API key and sandbox configuration

Provider login and gateway API authentication are separate. At
<http://localhost:4000/ui>, sign in as `admin` using `LITELLM_MASTER_KEY` from the
installed `.env`. Create a **virtual API key**, restricted to the desired models
(for example `chatgpt/gpt-6-luna`). Do not use the administrative master key in a VM.

Store the virtual key in an owner-only file (directory mode `700`, file mode
`600`) or the OS keyring. The launcher never provisions API keys or edits your
configuration automatically. For example, in
`~/.config/opencode-sandbox/config.toml`:

```toml
[[files]]
source = "~/.config/opencode-sandbox/opencode.json"
target = "/root/.config/opencode/opencode.json"

[secrets.llm_gateway]
file = "creds/llm-gateway-key"
env = "LLM_GATEWAY_KEY"
allowed_hosts = ["host.microsandbox.internal"]
headers = ["authorization"]
allow_http = true
```

Alternatively replace `file` with `keyring = {}` and store the key using
`opencode-sandbox secret set llm_gateway`.

`allow_http` explicitly disables the verified-TLS requirement for this secret.
This is appropriate only for the trusted host-local route, not public HTTP.
The runtime substitutes the key **only in the `Authorization` header**:
`Authorization: Bearer $MSB_LLM_GATEWAY_KEY` becomes
`Authorization: Bearer <real-gateway-key>` outside the VM.
Do not store the real key in the OpenCode JSON or pass it into the VM directly.

Create `~/.config/opencode-sandbox/opencode.json`:

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
        "coder": { "modelID": "chatgpt/gpt-6-luna" }
      }
    }
  }
}
```

Models use gateway IDs such as `chatgpt/<model>` or `copilot/<model>`. OpenCode's
custom provider uses the Responses runtime and sends the gateway key as a bearer
token. Port 4000 is allowed by default. If overriding `[network.vm_to_host]`, keep
`ports = [4000]` (plus any other needed host ports); `ports = []` blocks it.
Recreate VMs after changing secret/network/file mappings.

## Live smoke test

After authenticating the installed gateway once:

```sh
mise run test:live
```

The gateway need not already be running. The test invokes the real launcher,
starts OpenCode inside a disposable VM and checks a real prompt/reply through
the gateway, including header substitution. It reuses authentication volumes and
leaves the shared gateway running. It does not authenticate interactively or
silently skip missing setup. `OPENCODE_TEST_MODEL` overrides the gateway model ID
(default `chatgpt/gpt-6-luna`). Live requests consume provider usage.

The test supplies its own isolated launcher/provider configuration and uses the
installed `.env` administrative key as a test-only secret (never copied into the
VM). It doesn't require or change your normal secret configuration or provision
new API keys. Normal sandbox use should still use a restricted virtual key.
