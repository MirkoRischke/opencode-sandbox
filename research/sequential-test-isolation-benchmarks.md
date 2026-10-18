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
