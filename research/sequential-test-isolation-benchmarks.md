# Sequential test isolation benchmarks

Baseline: `be39c55`, 2026-10-18, same Linux host and prepared images. VM test
runs execute one at a time, with `-parallel 4`; no image imports/builds overlap.
Compilation is excluded. The initial integration-package run passed 108
top-level tests in **95.360s**.

Each group below ran three repetitions in one process (`-test.count=3`). Reported
means are package elapsed time divided by three, including `TestMain` and any
one-time launcher build. These are group throughput measurements, not claims
that individual VM operations became faster.

## Configuration isolation

Pass an explicit configuration directory into the OpenCode and CLI JSON loaders.
CLI setup resolves XDG/HOME; tests use private paths and run in parallel. Move
the two HOME-fallback checks to VM-free CLI tests, retaining their loading and
JSON assertions. The guest-copy/refresh/removal checks remain real VM tests.

| Group | Before total / mean | After total / mean | Reduction |
| --- | --- | --- | --- |
| `config_test.go`, `tui_test.go`, `cli_config_test.go` | 62.302s / 20.767s | 20.056s / 6.685s | 67.8% |

All scenarios passed: 19 top-level tests per original repetition; 17 in the
integration package afterward, with the two fallback cases moved to `internal/cli`.
Internal packages passed separately (CLI package 0.032s during validation).

## Credential and settings isolation

Resolve config/trust directories once in CLI setup. Global settings resolve
relative credential paths against their configuration directory; repo overrides
retain their repo origin. Secret loading accepts resolved absolute paths and
does not discover host environment or configuration directories.

GitHub, secret, VM/network, settings, and repo-trust tests use private explicit
inputs and run in parallel. HOME-based settings/trust checks move to VM-free CLI
tests. Keep environment-credential rejection coverage through real launcher
subprocesses with explicit environments, sharing the existing guest-probe fixture
with AWS tests instead of duplicating it.

| Group | Before total / mean | After total / mean | Reduction |
| --- | --- | --- | --- |
| `github_test.go`, `secrets_test.go`, `settings_test.go`, `repo_settings_test.go`, `network_test.go`, `vm_settings_test.go` | 86.939s / 28.980s | 40.914s / 13.638s | 52.9% |

All scenarios passed: 45 top-level tests per original repetition; 43 afterward
in the integration package, plus the two fallback cases relocated to
`internal/cli`. Preserve assertions for permissions, file/path validation,
trust/content approval, header substitution, denied destinations, VM resource
persistence, and rejection of inherited `GH_TOKEN` as a credential source.

## Diagnostics isolation

Pass the diagnostics state directory and output writer explicitly. CLI setup
resolves XDG/HOME; VM and unit tests use private directories and buffers rather
than replacing process-wide stderr/environment. The history/recreation test now
runs in parallel. Keep final-drain, persistence, warning timing, and replay
assertions. Add a real CLI check for HOME fallback and bypassing invalid global
and repo configuration when reading saved diagnostics.

| Group | Before total / mean | After total / mean | Reduction |
| --- | --- | --- | --- |
| Diagnostics history test plus the five existing Docker tests | 98.929s / 32.976s | 69.745s / 23.248s | 29.5% |

All six scenarios passed in all three repetitions. Docker is the unchanged
companion workload: this comparison measures removal of diagnostics' serial
barrier, not a reduction in diagnostics collection time itself. The separate
CLI fallback/bypass check passed, as did repeated internal-package race checks.

## Launcher subprocess isolation

Set fixture environment variables only on the CLI subprocess rather than through
`t.Setenv`. Use explicit settings-file paths. Run the launcher argument/config
checks and mise-shim subprocess test in parallel. Remove the last settings helper
that discovered its fixture directory from the test process environment.

| Group | Before total / mean | After total / mean | Reduction |
| --- | --- | --- | --- |
| `launcher_test.go` plus `runtime_test.go` | 8.506s / 2.835s | 7.551s / 2.517s | 11.2% |

All twelve scenarios passed in all three repetitions. The standalone cohort's
gain is modest because process startup and the one-time launcher build remain.
A separate paired integration-package run immediately before/after this last
change passed all 105 top-level tests: **60.418s -> 57.240s** (5.3% reduction).
That package comparison is one pair, not a statistical confidence interval.

## Overall result

| Integration package, four workers | Time |
| --- | --- |
| Initial baseline (`be39c55`) | 95.360s |
| After configuration, credentials/settings, and diagnostics isolation | 60.418s |
| After launcher subprocess isolation | 57.240s |

The final package run takes **38.120s / 40.0% less time** than the fresh baseline.
External-network timing still varies; cohort improvements above use three
repetitions, while these package-level results are single runs.

All **105 regular integration-package tests** now run as parallel parents
(103 direct `t.Parallel` calls and two existing terminal wrappers). Subtests that
share a VM remain sequential. The apparent count change from 108 is four HOME
fallback cases moved to VM-free CLI tests, plus one new diagnostics CLI case;
no original scenario was dropped. There are now 146 regular top-level tests
across all packages. Live inference remains opt-in and was not run.

The only remaining `t.Setenv` calls under `tests/` belong to a terminal probe
already isolated in its child process. No lint-policy or README changes were made.

Validation: repeated cohort runs, repeated internal-package race checks, and
`golangci-lint run --build-tags live ./...` passed. The final
`go test -race -shuffle=on -count=1 -parallel 4 -timeout 3m ./...` passed all
packages (integration package 59.773s). An accidentally interrupted initial race
invocation is not counted as a test result; it was confirmed stopped before
starting its replacement.

## Reproduction and artifacts

Freeze a binary at each revision with:

```sh
go test -c -o /tmp/opencode/revision.test ./tests
```

From `tests/`, run each selected group sequentially:

```sh
go tool test2json -t -p opencode-sandbox/tests /tmp/opencode/revision.test \
  -test.v=test2json -test.count=3 -test.parallel=4 -test.timeout=5m \
  '-test.run=^(TestNameOne|TestNameTwo)$' > /tmp/opencode/revision-group.jsonl
```

Select all top-level `Test*` names in the files named by each group. For the
integration package, omit `-test.run` and use `-test.count=1`. Run the revision's
source alongside its binary if a test builds the CLI lazily.

Local raw logs use `/tmp/opencode/sequential-before-*.jsonl` and
`/tmp/opencode/sequential-after-*.jsonl`. The frozen name selection is
`/tmp/opencode/sequential-bench-groups.json`, and the timing summary is
`/tmp/opencode/sequential-bench-summary.jsonl`. These local artifacts are
ephemeral; this note preserves the measurements and methodology.

Per-group comparisons isolate the changed cohort. They are not additive suite
speedups: cohorts can overlap and parallel tests compete for worker slots.
