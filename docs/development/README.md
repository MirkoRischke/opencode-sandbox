# Development

Run these commands from the repository root:

```sh
mise run fmt  # Format Go code
mise run lint
mise run verify      # Lint + prepare changed images + full standard test suite
mise run quality baseline # Save local complexity metrics before a refactor
mise run quality compare  # Show current hotspots and changes from that baseline
mise run test:unit    # Fast source-adjacent tests; no VMs or external services
mise run test:non-vm  # Unit/script + CLI + image + gateway tests; Docker only
mise run test:image   # Build and smoke-test the base and all agent images; Docker only
mise run test:gateway # Real PostgreSQL/LiteLLM stack; no VMs or provider credentials
mise run test:prepare # Prepare changed images only when image inputs change
mise run test        # Prepare changed images, then run the full suite including image tests
mise run test:live   # Real VM prompt via gateway; requires persisted provider auth
```

After editing feature guides, configuration docs, limitations, or `docs/risks.md`,
run `mise run skill:sync` and include the generated references in the same change.
Edit the originals in `docs/`, not the copies under the skill. To check for drift
locally without writing files, run `mise run skill:check`.

Image preparation tracks the Dockerfiles and the root `.dockerignore`.
Unchanged builds and imports are skipped; tests always run. After deleting
Docker or Microsandbox images, run `mise run --force test:prepare` to prepare them again.

Source-focused tests live beside their implementation. Higher-level tests are
split into `tests/cli` (no Microsandbox), `tests/image` (Docker),
`tests/gateway` (real Docker Compose gateway), and `tests/vm`
(Microsandbox). PR checks run linting, build the launcher, check module consistency,
compile all tests including the `live` tag, and run `test:non-vm` with race detection.
Renovate tracks the stable version pins for all three coding agents and
automerges dependency updates, including majors, after checks pass;
the Microsandbox SDK is excluded because it must be upgraded with its runtime.

Gateway integration tests start the built CLI with the bundled Compose files,
temporary config/data directories, a unique project name, and an unused loopback
port. They verify startup without a config, external config reload/removal,
database-defined models, invalid-config failure, readiness, team/key provisioning,
authentication and permissions, key persistence across container recreation,
and deletion. One stack is shared
across focused, sequential scenarios to avoid repeated initialization; each
scenario provisions its own credentials. Cleanup removes only that project's
containers and volumes.
Failures include redacted container logs. Image downloads on the first run add
network-dependent time; later runs reuse cached images.

Quality reports are advisory and saved under `.quality/` (ignored by Git).
They require Python 3 and the pinned golangci-lint; compare with the same tool
version and configuration. See [the readability review rubric](quality-review.md)
for reviewing implementation and tests alongside the metrics.

## Gateway smoke test

Follow the [host LLM gateway guide](../features/llm-gateway.md) to configure
a model and authenticate once, then run:

```sh
mise run test:live
```

The test prompts the configured model through the real launcher and OpenCode
in a disposable VM. This consumes provider usage. `OPENCODE_TEST_MODEL` selects
the public gateway model ID (default `gpt-6-luna`). Missing setup fails the
test rather than skipping it.

The launcher starts the gateway if needed. The test isolates launcher and
provider settings, reuses the gateway's model configuration and authentication
volumes, and checks key provisioning and substitution. Cleanup removes the test
VM and its key and restores the gateway's normal config mount. The gateway
stays running; authentication, the repository team, and usage history remain.
Ordinary tests use no real provider credentials.

## Adding launcher settings

Add each setting with validation and a `description` tag in
[`internal/config`](../../internal/config). The default-config printer discovers
settings automatically. Tests check that descriptions exist and printed values
match runtime defaults. Put usage and examples in the relevant
[feature guide](../features/README.md).

## Upgrading Microsandbox

Upgrade the SDK and runtime together from this repository:

```sh
mise run upgrade-microsandbox       # Resolve the latest stable runtime version
# To select an exact version: mise run upgrade-microsandbox <version>
# Dotfiles default to ../dotfiles; override with --dotfiles /path/to/dotfiles
```

The task updates the SDK dependency and local runtime pin, runs `mise run verify`
with that runtime, installs the launcher, then updates the Microsandbox pin in
`home/private_dot_config/mise/config.toml.tmpl` in your dotfiles repository.
Review both repositories and apply your Mise configuration through Chezmoi to
select the matching runtime globally. The global pin must be an exact version,
not `latest`, to prevent unattended upgrades from breaking SDK compatibility.
Failures leave changes for inspection; the task does not commit or roll back.
