# LLM gateway

The launcher runs a shared LiteLLM gateway on the host using Docker Compose.
The gateway starts before VM preparation so the launcher can provision its key.
Every launch reconciles the project with the installed CLI's bundled Compose file.
It stays running
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

The binary embeds `compose.yaml` and `compose.config.yaml`; no source checkout is
needed. On first use it installs these and generates an owner-only `.env` under
`$XDG_DATA_HOME/opencode-sandbox/llm-gateway` (default
`~/.local/share/opencode-sandbox/llm-gateway`). Every startup refreshes both Compose
files from the installed CLI and applies the selected configuration, even when
the containers are healthy. Existing `.env` and Docker volumes are preserved.
The Compose project is named `opencode-sandbox-gateway`.

## Optional model configuration

No model catalog is bundled. To configure models declaratively, provide:

```text
$XDG_CONFIG_HOME/opencode-sandbox/llm-gateway/config.yaml
```

If `XDG_CONFIG_HOME` is unset, the path is
`~/.config/opencode-sandbox/llm-gateway/config.yaml` on both Linux and macOS.
For example:

```yaml
model_list:
  - model_name: gpt-6-luna
    litellm_params:
      model: chatgpt/gpt-6-luna
    model_info:
      mode: responses
```

`model_name` is the public gateway ID clients send; `litellm_params.model`
selects the upstream provider/model. For Copilot, use
`github_copilot/gpt-6-luna` as the upstream and authenticate with `copilot`.
Only configure models available through your subscription.

The launcher never creates or overwrites this file. When present, it is mounted
read-only using `compose.config.yaml` and passed to LiteLLM with `--config`.
When absent, LiteLLM starts without `--config`; models can instead be added
through the admin UI/API and persisted in PostgreSQL. A fresh gateway without
either source of models can start, but isn't ready for inference.
File-defined and database-defined models can coexist.

Unreadable/non-file paths fail startup; invalid configuration is reported by
LiteLLM rather than silently ignored. Adding, editing, or removing the file
takes effect on the next gateway start or sandbox launch, recreating the LiteLLM
container as needed. There is no hot reload. Stopping does not read the optional
file, so a broken config cannot prevent `gateway stop`.

## Gateway lifecycle

For an isolated gateway, set `OPENCODE_SANDBOX_GATEWAY_PROJECT` and
`OPENCODE_SANDBOX_GATEWAY_PORT` (defaults: `opencode-sandbox-gateway` and `4000`),
plus a separate `XDG_DATA_HOME` for its runtime files and credentials. Use a
separate `XDG_CONFIG_HOME` as well to isolate its optional model config. The port
also controls the CLI's gateway API client; the listener remains loopback-only.
Standalone Compose tasks accept the same port variable and Compose's
`COMPOSE_PROJECT_NAME` override. Custom ports do not change VM networking or
OpenCode provider settings automatically.

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
installed copies, with the same optional user config path and startup selection
via `compose.sh`. Use one consistent set of files/secrets for the shared project;
do not alternate independently generated `.env` files against the same database.

## Gateway API key and sandbox configuration

Provider login and gateway API authentication are separate. The launcher
automatically creates one LiteLLM **team per repository** and one **virtual API
key per worktree/VM**. Linked Git worktrees share a team; independent clones do
not. Non-Git directories each get their own team. Keys can call every configured
gateway model, but cannot call management routes.

Keys are reused across VM stops/restarts and deleted when
`opencode-sandbox remove` (or `remove --all`) removes the VM. Deletion is attempted
once. If it fails, the command reports the failure; delete that key manually in
LiteLLM. The local credential is removed either way. Teams are retained for
historical reporting. Recreating a VM creates a new key.

The gateway manages credentials under
`$XDG_DATA_HOME/opencode-sandbox/llm-gateway/credentials` (default
`~/.local/share/opencode-sandbox/llm-gateway/credentials`), using directory mode `700`
and file mode `600`. Neither the real virtual key nor the master key is copied
into the guest. No `[secrets.llm_gateway]` entry is needed; the launcher replaces
that entry, or any secret using `LLM_GATEWAY_KEY`, with its managed credential.

At <http://localhost:4000/ui>, sign in as `admin` using `LITELLM_MASTER_KEY` from
the installed `.env`. In **Usage → Team Usage**, select the repository team for
overall usage and drill into its VM keys for individual usage. These team/key
views are available without Enterprise in the pinned LiteLLM `v1.108.1`.
Deleted-key history may be less convenient to navigate; deletion does not
intentionally purge usage logs. With subscription providers, calculated dollar
costs do not necessarily represent your actual bill or remaining quota.

You still configure your custom OpenCode provider. For example, copy its config
using `~/.config/opencode-sandbox/config.toml`:

```toml
[[files]]
source = "~/.config/opencode-sandbox/opencode.json"
target = "/root/.config/opencode/opencode.json"
```

The launcher supplies `LLM_GATEWAY_KEY` inside the VM as a **placeholder**, not
the real key. Reference it as `{env:LLM_GATEWAY_KEY}` in your OpenCode provider.
Microsandbox substitutes the real key **only in the `Authorization` header** on
requests to `host.microsandbox.internal`:
`Authorization: Bearer $MSB_LLM_GATEWAY_KEY` becomes
`Authorization: Bearer <real-gateway-key>` outside the VM.
HTTP substitution is enabled only for this trusted host-local route. Keep the
provider's HTTP transport; do not put a literal API key in the OpenCode config.

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
        "coder": { "modelID": "gpt-6-luna" }
      }
    }
  }
}
```

Models use the public gateway IDs configured in LiteLLM, such as `gpt-6-luna`
in the example above. OpenCode's
custom provider uses the Responses runtime and sends the gateway key as a bearer
token. Port 4000 is allowed by default. If overriding `[network.vm_to_host]`, keep
`ports = [4000]` (plus any other needed host ports); `ports = []` blocks it.
Recreate VMs after changing secret/network/file mappings.

## Live smoke test

After configuring a model and authenticating the installed gateway once:

```sh
mise run test:live
```

The gateway need not already be running. The test invokes the real launcher,
starts OpenCode inside a disposable VM and checks a real prompt/reply through
the gateway, including header substitution. It reuses authentication volumes and
leaves the shared gateway running. It does not authenticate interactively or
silently skip missing setup. `OPENCODE_TEST_MODEL` overrides the gateway model ID
(default `gpt-6-luna`). Live requests consume provider usage.

The test supplies its own isolated launcher/provider configuration and exercises
automatic team/key provisioning and header substitution. It does not change your
normal launcher configuration. Cleanup removes its VM and virtual key; the
repository team and usage history remain.
