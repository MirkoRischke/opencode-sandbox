# LLM gateway

Runs LiteLLM on the host, abstracting authentication to different LLM providers
behind one API. Requires Docker Compose and OpenSSL. Manage models and API keys at
<http://localhost:4000/ui> (user `admin`, password `LITELLM_MASTER_KEY` in `.env`).

Start or stop (stopping preserves credentials):

```sh
mise run gateway:start
mise run gateway:stop
```

For initial ChatGPT authentication, follow the device login instructions (enable
device-code login in ChatGPT security settings if needed):

```sh
mise run gateway:authenticate
```

For GitHub Copilot, run `mise run gateway:authenticate:copilot`. Models are exposed
as `chatgpt/<model>` or `copilot/<model>`; both providers' logins persist locally.
