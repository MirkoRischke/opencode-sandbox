# AWS test isolation: measured results

Date: 2026-10-18. Baseline: commit `7164ded`. Same Linux host, prepared
images, four-worker limit; no image builds/imports or overlapping VM test runs.

## Change under test

- Move `config.LoadDefaultConfig` and STS client construction from
  `sandbox.StartAWS` into CLI launch setup. Pass an explicit client and region.
- Keep credential discovery/profile/region precedence coverage through real
  launcher subprocesses with `command.Env`. Replace the guest TUI with a probe
  that checks credentials, permissions, and region, then exits.
- Keep lifecycle coverage through direct calls using a real STS client pointed
  at a local fake STS server and explicit fake source credentials.
- Remove test-process environment mutation from all ten AWS tests. Mark the
  independent tests and four independent region cases parallel.

This is one isolation/refactoring change with a separately measured scheduling
effect. It does not claim each individual source edit independently saves time.

## Results

Times below are test-process seconds, excluding compilation of the test binary.
AWS configurations each ran three repetitions in one process (`-test.count=3`).
Per-repetition times use JSON timestamps from the first top-level test's start
to the next repetition's start, or package completion for the final repetition.
`TestMain` startup precedes those timestamps; package totals include it.

| Configuration | AWS repetitions | Mean | Package total |
| --- | --- | --- | --- |
| Before, `-parallel 4` (AWS tests actually sequential) | 33.959, 34.114, 33.888 | 33.987s | 101.965s |
| After, forced serial (`-parallel 1`) | 36.051, 33.995, 34.168 | 34.738s | 104.219s |
| After, `-parallel 4` | 10.737, 10.401, 9.984 | 10.374s | 31.125s |

All three configurations passed all ten top-level AWS tests per repetition,
including their subtests. The new serial configuration is about **2.2% slower**;
the explicit dependencies enable parallel execution, rather than intrinsically
making individual operations faster. The parallel configuration takes **69.5%
less time** than the original AWS tests.

| Integration package (`tests`, `-parallel 4`) | Time | Top-level passes |
| --- | --- | --- |
| Before | 124.779s | 108 |
| After | 92.134s | 108 |

The integration package takes **32.645s / 26.2% less time**. This is one paired
package run, not a statistical confidence interval. External-network variability
still affects the broader package. Internal packages were validated separately;
these package timings are not presented as a fresh `./...` timing.

## Reproduction

Capture each revision's test binary before changing its source:

```sh
# At the baseline revision:
go test -c -o /tmp/opencode/aws-before.test ./tests
# After applying the change:
go test -c -o /tmp/opencode/aws-after.test ./tests
```

Run the binaries sequentially from `tests/`. For example:

```sh
go tool test2json -t -p opencode-sandbox/tests /tmp/opencode/aws-before.test \
  -test.v=test2json '-test.run=^TestAWS' -test.count=3 \
  -test.parallel=4 -test.timeout=3m > /tmp/opencode/aws-before.jsonl

go tool test2json -t -p opencode-sandbox/tests /tmp/opencode/aws-after.test \
  -test.v=test2json '-test.run=^TestAWS' -test.count=3 \
  -test.parallel=1 -test.timeout=3m > /tmp/opencode/aws-after-v2-serial.jsonl

go tool test2json -t -p opencode-sandbox/tests /tmp/opencode/aws-after.test \
  -test.v=test2json '-test.run=^TestAWS' -test.count=3 \
  -test.parallel=4 -test.timeout=3m > /tmp/opencode/aws-after-v2-parallel.jsonl
```

For the package comparisons, omit `-test.run`, use `-test.count=1`,
`-test.parallel=4`, and `-test.timeout=5m`. The measured logs are
`/tmp/opencode/suite-before-aws.jsonl` and
`/tmp/opencode/suite-after-v2-aws.jsonl`. These `/tmp` artifacts are local and
ephemeral; the results above preserve the relevant measurements.

The test binaries lazily build the launcher from source when needed, so keep
each revision's source in place while running its binary. The original AWS tests
do not invoke the launcher; the new tests' launcher-build cost is included.

## Correctness and rejected measurements

- Two focused AWS preflight cases passed before timing.
- An initial serial run failed: mise inspected the replacement OpenCode shim
  during preparation, prematurely running the guest assertions. It is excluded
  from all speedup claims. The fixture now executes the probe only for a
  no-argument invocation; all four region cases passed before restarting timing.
- A separate preflight exposed that overriding `HOME` changed the VM store
  location. The subprocess now preserves `HOME`, isolates launcher configuration
  with XDG overrides, and uses explicit AWS fixture paths.
- `go test -race -count=2 -parallel 4 -timeout 60s ./tests -run '^TestAWS'`
  passed (23.293s).
- `go test -race -count=1 -timeout 10s ./internal/...` passed.
- `golangci-lint run --build-tags live ./...` and `git diff --check` passed.

Decision: retain this change. It removes the hidden host-configuration dependency
and demonstrates a substantial package-level improvement while retaining all
existing top-level AWS scenarios and stronger CLI wiring coverage.
