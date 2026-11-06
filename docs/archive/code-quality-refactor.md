# Codebase quality refactor

> Historical task report, not current guidance. See the [development guide](../development/README.md) for current instructions.

Baseline: `46c5597` (the completed log-retention pilot). Branch: `codex/readability-quality`.
Local tooling and thresholds are unchanged. All initial measured hotspots were reviewed; 37 were resolved and two explicitly retained below. No CI checks were added.

## Measurements

Totals include extracted helpers and new tests. Thresholds remain cognitive 15 for production, cognitive 10 for tests, cyclomatic 10, and advisory test-length 60.

| Metric | Before | After |
|---|---:|---:|
| Production: function count | 147 | 184 |
| Production: cognitive total | 967 | 849 |
| Production: cognitive max | 52 | 14 |
| Production: cyclomatic max | 24 | 12 |
| Production: hotspots | 23 | 2 |
| Tests and helpers: function count | 264 | 312 |
| Tests and helpers: cognitive total | 478 | 303 |
| Tests and helpers: cognitive max | 35 | 10 |
| Tests and helpers: cyclomatic max | 15 | 10 |
| Tests and helpers: hotspots | 16 | 0 |

## Initial hotspot audit

Each entry is a review disposition, not a numerical readability score. The full per-function changes are available with `mise run quality compare`.

| Original function | File | Disposition |
|---|---|---|
| `launch` | `internal/cli/launch.go` | Retained after extracting host AWS discovery: cognitive 11, cyclomatic 12. Remaining branches are linear error returns; lifecycle/defer ordering stays visible. |
| `reportRemovals` | `internal/cli/removal.go` | Give observation and summary formatting explicit state; preserve deletion and output errors. |
| `TestManagementCommandsRejectExtraArguments` | `internal/cli/root_test.go` | Use direct error assertions instead of assertion control flow. |
| `TestRemoveAllFlag` | `internal/cli/root_test.go` | Separate successful parsing, flag value, and help-content assertions. |
| `TestSnapshotRemoveAllFlag` | `internal/cli/root_test.go` | Use direct assertions for discovery, parsing, value, and help. |
| `TestCommandHelp` | `internal/cli/root_test.go` | Separate root AWS help from subcommand help and non-inheritance assertions. |
| `TestCompletionScriptsWithoutLoadingHostState` | `internal/cli/root_test.go` | Keep corrupt-config fixture explicit; share isolated process execution mechanics. |
| `TestCompletionSuggestions` | `internal/cli/root_test.go` | Use direct per-suggestion assertions with returned output as evidence. |
| `TestIdentifyHerdrAgent` | `internal/cli/root_test.go` | Keep scenario inputs and process-environment assertion visible; isolate subprocess mechanics. |
| `secretCommand` | `internal/cli/secrets.go` | Separate construction, metadata validation, store, confirmed removal, and listing. |
| `newSnapshotCommand` | `internal/cli/snapshot.go` | Separate create and remove command construction. |
| `TestConfirmWorkspaceConfig` | `internal/cli/trust_test.go` | Separate acceptance, refusal, and prompt safety into independent tests. |
| `writeConfigTable` | `internal/config/default_config.go` | Separate field discovery, value rendering, and optional examples; defaults output is byte-identical. |
| `TestEveryConfigFieldHasDocumentation` | `internal/config/default_config_test.go` | Name the recursive documentation assertion helper and keep rendering setup in the test. |
| `TestDefaultConfigFileExampleIsValid` | `internal/config/default_config_test.go` | Locate the example explicitly and use a single flat scan to enable its fields. |
| `validateFiles` | `internal/config/files.go` | Separate each mapping's validity from cross-mapping overlap checks. |
| `mergeSettings` | `internal/config/layers.go` | Separate layer path resolution and whole-secret-entry replacement from TOML merging. |
| `ValidateSecrets` | `internal/config/secrets.go` | Separate per-source validation, cross-entry environment uniqueness, and destination scope validation. |
| `trustWorkspaceSettings` | `internal/config/workspace_settings.go` | Separate strict trust-store reading from approval and atomic publication. |
| `(*Sandbox).ImportOpenAI` | `internal/sandbox/auth.go` | Extract pure credential selection; add focused rejection and payload-preservation tests. |
| `(*Sandbox).StartAWS` | `internal/sandbox/aws.go` | Separate initial acquisition, refresh, retry scheduling, and cancellation. |
| `(*networkDiagnostics).run` | `internal/sandbox/diagnostics.go` | Separate polling failure state from the scheduling/drain loop. |
| `(*networkDiagnostics).observe` | `internal/sandbox/diagnostics.go` | Retained: cognitive 11, cyclomatic 11. Five destination fields define identity; time bounds, count, and overflow checks are short and explicit. |
| `parseNetworkDenial` | `internal/sandbox/diagnostics.go` | Separate event recognition from field interpretation; keep hostname/IP history semantics. |
| `hostnameForDiagnostics` | `internal/sandbox/diagnostics.go` | Express the accepted character grammar with one regexp and preserve the none sentinel exclusion. |
| `TestNetworkDiagnosticsDefersWarningsUntilShutdown` | `internal/sandbox/diagnostics_test.go` | Declare collection/save failures as scenario inputs; remove branching from the collector fixture. |
| `PrepareWorkspace` | `internal/sandbox/sandbox.go` | Share pre-allocation validation and new-guest configuration with snapshot preparation. |
| `(*Sandbox).Exec` | `internal/sandbox/sandbox.go` | Keep handle ownership in Exec; separate event draining, output writing, and input completion. |
| `loadSecrets` | `internal/sandbox/secrets.go` | Separate source resolution and suppressed source errors from ordered proxy-secret construction. |
| `readSecretFile` | `internal/sandbox/secrets.go` | Separate ancestor-directory safety checks from credential-file validation and reading. |
| `CreateWorkspaceSnapshot` | `internal/sandbox/snapshot.go` | Share input validation; separate guest quiescence from capture/publication/pruning. |
| `RemoveAllSnapshots` | `internal/sandbox/snapshot.go` | Share ownership discovery with listing; preserve sorted removals and error aggregation. |
| `ListSnapshots` | `internal/sandbox/snapshot_list.go` | Separate native metadata discovery, grouping, and deterministic sorting. |
| `Resolve` | `internal/workspace/workspace.go` | Separate canonical/safe root resolution from Git metadata discovery. |
| `TestDNSQueriesAreFilteredBeforeForwarding` | `tests/dns_test.go` | Replace nested mode branches with focused restricted, empty, unrestricted, CNAME, and protected-destination tests. |
| `startDNSUpstream` | `tests/dns_test.go` | Separate DNS question/response encoding, UDP serving, TCP accepting, and connection handling. |
| `TestDockerBuildComposeAndPersistence` | `tests/docker_test.go` | Separate storage/port contract, registry access, Compose, network enforcement, daemon health, and restart persistence. |
| `TestNetworkDiagnosticsPreservesWorktreeHistoryAcrossVMRecreation` | `tests/network_diagnostics_test.go` | Separate history merging from no-replay/removal persistence; read through the public diagnostics interface. |
| `TestRepositorySnapshotLifecycle` | `tests/snapshot_test.go` | Replace the long scenario with independent metadata, failed preparation, restore/TLS/Docker, replacement, removal, and workspace-isolation tests. |

## Readability review

Reviewed the old and new functions and their helpers using [the readability review rubric](../development/quality-review.md):

- Resolved test-purpose problems: CLI approval and secret actions, DNS policy modes, snapshot lifecycle behaviors, Docker contracts, and diagnostics persistence now have focused scenarios.
- Resolved visibility problems: test inputs and expected results remain in test bodies; helpers own VM/process setup, protocol mechanics, fixture creation, and command execution.
- Resolved ownership problems: command streams, AWS refresh cancellation, snapshot publication, and trust-store decoding have named responsibilities without new public interfaces.
- Preserved behavior during splitting: snapshot replacement proves it inherits neither an existing VM's files nor the previous baseline's files; DNS still verifies both transports and the upstream forwarding counter.
- Improved failure evidence: errors and output expectations use separate assertions; cases and subprocess failures identify their scenario.
- The remaining low-complexity implementation and tests were inspected by module. Straightforward data declarations, shell scripts, existing focused scenarios, and platform adapters were retained rather than changed for a score.

## Verification

- Each implementation commit passed affected internal race tests and repository lint. Runtime changes additionally passed relevant real-VM execution/AWS/snapshot/diagnostics checks.
- Rewritten DNS, snapshot, diagnostics, and Docker tests passed against real VMs before their commits.
- `go test -race -count=1 -json -parallel 4 -timeout 10m ./...`: all 521 test/subtest results pass; no test failures or skipped tests. The command package has no tests. Includes the VM and Docker image suites.
- `python3 -m unittest discover -s scripts -v`: all five tooling tests pass.
- `go test -race -count=1 -json -tags live -run '^TestOpenCodeCanReachOpenAI$' -timeout 1m ./tests`: passes, including live subscription authentication and inference.
- `golangci-lint run --build-tags live ./...`: zero issues.
- `git diff --check`: passes.
- Generated default TOML was compared before and after the renderer refactor: byte-identical.

Latest metrics, the full-suite event log, and the live-test event log are saved locally under ignored `.quality/`. Capture a new baseline only when intentionally starting another comparison.
