# Log retention pilot

Scope: local quality tooling and one workflow, host log retention. Baseline
production and tests are from `30e2eae`; tooling is in `b516205` and the focused
test changes in `a5162d9`. Other repository hotspots remain outside this pilot.

## Completion criteria

- Local baseline and comparison commands produce saved JSON and Markdown.
- Retention functions stay within cognitive 15 and cyclomatic 10, including
  newly extracted helpers.
- Logging tests stay within cognitive 10, cyclomatic 10, and length 60 warnings.
- Existing retention behavior is preserved; test purposes and failures are clear.
- Tests, race checks, lint, and scoped regression experiments pass.
- Work is committed on `codex/readability-quality`.

## Measurements

Measured with the pinned golangci-lint 2.15.0 and `scripts/quality.yml` on Linux.
Totals include extracted functions, not just the old entry point.

| Measure | Before | After |
|---|---:|---:|
| Retention workflow cognitive total | 55 | 35 |
| Retention highest cognitive complexity | 55 | 11 |
| Retention highest cyclomatic complexity | 26 | 9 |
| Logging tests cognitive total | 22 | 15 |
| Logging tests highest cognitive complexity | 13 | 4 |
| Logging package statement coverage | 76.4% | 77.3% |

The new retention functions have cognitive/cyclomatic scores of 7/6 (`cleanup`),
11/9 (`cleanupDirectory`), 11/8 (`managedSegments`), and 6/7
(`removeInactiveSegment`). No logging function or test exceeds its thresholds.
Coverage is supporting evidence; exceptional filesystem failure paths remain
partially uncovered.

## Readability review

Reviewed both versions and the extracted helpers using `quality-review.md`.
These are evidence-based judgments, separate from numeric scores.

| Category | Status | Evidence |
|---|---|---|
| Implementation ownership | Resolved | One nested cleanup function previously mixed traversal, discovery, retention decisions, locking, and removal. Named functions now own those responsibilities; only `removeInactiveSegment` owns its file handle and lock. |
| Test purpose | Resolved | The combined active-file/unknown-file test became focused scenarios for expiration, active-file lifecycle, unmanaged entries, and unmanaged directories. |
| Test visibility | Resolved | Per-workspace count assertions now verify all three oldest files disappear. The age cutoff and timestamp tie rule have explicit expected outcomes. |
| Test structure | Resolved | Concurrent writer setup moved to `writeRotatingLogs`; the test retains its writer count, retained-file limit, and JSON assertions. |
| Failure clarity | Resolved | File-specific assertions replace boolean existence checks; malformed JSON failures include the file and record. |

No new readability problem identified within this workflow. Splitting functions
increases function count; the cognitive total decreases even including helpers.

## Verification

- `python3 -m unittest discover -s scripts`: five tooling tests pass.
- `mise run quality baseline` and `mise run quality compare`: saved snapshots
  and reports work; unchanged scores do not appear as changes when lines move.
- `golangci-lint run --build-tags live ./...`: zero issues.
- `go test -race -count=1 -timeout 30s ./internal/...`: passes.
- `go test -race -shuffle=on -count=20 -timeout 60s ./internal/logging`: passes.
- Six temporary mutations, run in an isolated copy, are caught by focused tests:
  ignore expiration, ignore count, delete active segments, reverse age ordering,
  reverse timestamp tie ordering, and prune unmanaged directories.

The baseline and current snapshots remain in ignored `.quality/`. Run
`mise run quality compare` to inspect them; use `mise run quality baseline`
when intentionally starting a new comparison. The tooling reports numeric
changes automatically. For a readability comparison, ask an agent to apply
`quality-review.md` against the baseline revision and current code.
