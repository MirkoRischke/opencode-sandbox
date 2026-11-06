# Development

Run these commands from the repository root:

```sh
mise run fmt  # Format Go code
mise run lint
mise run verify      # Lint + prepare changed images + full standard test suite
mise run quality baseline # Save local complexity metrics before a refactor
mise run quality compare  # Show current hotspots and changes from that baseline
mise run test:unit    # Fast source-adjacent tests; no VMs or external services
mise run test:non-vm  # Unit/script + CLI + image tests; Docker only, no Microsandbox
mise run test:image   # Build + offline image smoke tests; Docker only, no VMs
mise run test:prepare # Prepare changed images only when image inputs change
mise run test        # Prepare changed images, then run the full test suite
mise run test:live   # Real VM prompt via gateway; requires persisted provider auth
```

Image preparation tracks both Dockerfiles and the base `.dockerignore`.
Unchanged builds and imports are skipped; tests always run. After deleting
Docker or Microsandbox images, run `mise run --force test:prepare` to prepare them again.

Source-focused tests live beside their implementation. Higher-level tests are
split into `tests/cli` (no Microsandbox), `tests/image` (Docker), and `tests/vm`
(Microsandbox). PR checks run linting, build the launcher, check module consistency,
compile all tests including the `live` tag, and run `test:non-vm` with race detection.
Renovate automerges dependency updates, including majors, after checks pass;
the Microsandbox SDK is excluded because it must be upgraded with its runtime.

The opt-in live test uses the real launcher and OpenCode inside a disposable VM.
Authenticate the installed gateway once as shown in the
[gateway guide](../../llm-gateway/README.md). The gateway may be stopped before
the test: the launcher starts it automatically. Tests never stop the shared
gateway or remove its authentication volumes. `OPENCODE_TEST_MODEL` selects the
gateway model ID (default `chatgpt/gpt-6-luna`). Missing setup fails the test rather
than skipping it. It creates isolated provider configuration and exercises
automatic virtual-key provisioning; the real key is substituted outside the VM.
Cleanup removes the test VM and its key. Ordinary tests use no real provider
credentials.

Quality reports are advisory and saved under `.quality/` (ignored by Git).
They require Python 3 and the pinned golangci-lint; compare with the same tool
version and configuration. See [the readability review rubric](quality-review.md)
for reviewing implementation and tests alongside the metrics.

## Upgrading Microsandbox

Upgrade the SDK and runtime together from this repository:

```sh
mise run upgrade-microsandbox       # Resolve the latest stable runtime version
mise run upgrade-microsandbox 0.7.9 # Or select an exact version
# Dotfiles default to ../dotfiles; override with --dotfiles /path/to/dotfiles
```

The task updates the SDK dependency and local runtime pin, runs `mise run verify`
with that runtime, installs the launcher, then updates the Microsandbox pin in
`home/private_dot_config/mise/config.toml.tmpl` in your dotfiles repository.
Review both repositories and apply your Mise configuration through Chezmoi to
select the matching runtime globally. The global pin must be an exact version,
not `latest`, to prevent unattended upgrades from breaking SDK compatibility.
Failures leave changes for inspection; the task does not commit or roll back.
