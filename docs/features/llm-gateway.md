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

See the [gateway setup guide](../../llm-gateway/README.md) for prerequisites,
authentication, model configuration, and an OpenCode example.
