# LLM gateway development

For model setup, authentication, and operation, see the
[host LLM gateway guide](../docs/features/llm-gateway.md).

The standalone `mise run gateway:*` tasks use this directory's `.env` and
Compose files rather than the installed copies. [`compose.sh`](compose.sh)
selects the optional user model configuration just as the launcher does.
The tasks accept `OPENCODE_SANDBOX_GATEWAY_PORT` and Compose's
`COMPOSE_PROJECT_NAME` override. Use one consistent set of files and secrets
for a shared Compose project; don't alternate independently generated `.env`
files against the same database.

See the [gateway smoke test](../docs/development/README.md#gateway-smoke-test)
to verify the installed launcher and gateway together.
