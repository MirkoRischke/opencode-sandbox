# Parallel-by-default Go tests: scheduling and enforcement

**Recommendation:** keep explicit `t.Parallel()` at independent top-level tests, keep `-parallel 4`, and enable **`paralleltest` with `ignore-missing-subtests: true`**, not `tparallel`. Reuse the existing explained, specific `nolint` policy. **For this request's stronger “review every sequential exception” requirement, also use narrowly test-scoped `forbidigo` rules for process-global environment/directory mutation.** `paralleltest` alone silently exempts direct `t.Setenv`/`t.Chdir` calls, so it cannot enforce that stronger requirement by itself. The additional rule closes that common exemption path without inventing a custom analyzer. This is lint-enforced explicit opt-in, **not** an automatic Go scheduler setting. [G1, P1, L1, F1]

Research date: 2026-10-18. Research only: no test/tooling implementation, README, or AGENTS changes. Context7 consulted for golangci-lint; versioned upstream source checked for precise behavior. Background delegation was attempted per research skill but rejected by the subagent depth limit. Temporary analyzer experiments live outside the repository at `/tmp/opencode/parallel-policy-research`.

## Repository baseline

- `go.mod` and `mise.toml` pin **Go 1.27.2**; installed `go version` confirmed `go1.27.2 linux/amd64`. `mise.toml` pins **golangci-lint 2.15.0**; installed CLI confirmed that version (built with Go 1.27.0). Its upstream dependency list pins **paralleltest 1.0.16**, **tparallel 0.3.2**, and **forbidigo/v2 2.3.1**. Sources below use these versions, not just latest documentation. [R1, L2]
- `.golangci.yml` uses v2 configuration and an explicit linter allowlist. `nolintlint` is already enabled with `require-explanation: true` and `require-specific: true`; neither parallelism linter nor `forbidigo` is enabled. `mise lint` runs `golangci-lint run ./...` and hadolint. No new command is needed. [R1]
- `mise test` runs `go test -count=1 -v -parallel 4 -timeout 10m ./...`. `test:unit` and `test:live` do not specify this limit. Preserve the current main test command: its limit is useful after adding parallel calls, but does not supply them. [R1, G1]
- Prior-audit input supplied to this research: **108 top-level tests in `tests`, 36 parallel-capable, 72 sequential; 69 sequential tests mutate process-wide environment directly or via helpers**. These are not newly recounted here; “parallel-capable” includes helper-mediated scheduling and is not a literal count of `t.Parallel` statements. Current source confirms direct environment mutation and helper-mediated cases throughout `tests`. [R2]

## There is no Go global parallel-by-default switch

In the pinned Go toolchain, `-parallel n` explicitly applies to test functions that **call `t.Parallel`**. It defaults to `GOMAXPROCS`, applies within one test binary, and is separate from `go test -p`, which controls concurrent package jobs. `GOFLAGS=-parallel=4`, changing `GOMAXPROCS`, or increasing `-p` therefore cannot make sequential functions parallel. No documented Go test flag or supported `TestMain` switch reverses the explicit `T.Parallel` contract. The pinned `testing` implementation marks each individual `T` parallel only through `T.Parallel`. [G1, G2]

`t.Parallel()` pauses the calling test until its parent's barrier opens; it does **not** make the function return. Tests marked parallel run with other parallel tests. A normal `t.Run` blocks until its child returns or calls `Parallel`, so a parallel top-level test may safely have sequential child steps while the overall scenario runs alongside other independent scenarios. Put the top-level call before expensive setup if that setup should occupy a worker slot. [G2]

The four-worker setting is **not a global four-VM quota**: packages have separate binaries, a test can create multiple VMs, and manually spawned child processes have their own schedulers. Current terminal-test parents wait for their one selected child while occupying a parallel slot, which preserves useful bounding for that path; their child command does not itself receive `-test.parallel=4`. [G1, R3; scheduling consequence inferred from G2]

## What the two linters actually enforce

| Tool | Precise relevant behavior | Fit here |
| --- | --- | --- |
| `paralleltest` 1.0.16 | AST analyzer for `Test*` functions with a recognized `*testing.T` parameter in `_test.go` files. Reports missing top-level parallel calls unless a recognized blocking method is found; also checks certain `t.Run`/range shapes unless subtest checks are disabled. Does not execute or rewrite tests. [P1] | Best existing enforcement starting point, with caveats below. |
| `tparallel` 0.3.2 | SSA-based consistency/lifetime checker, **not** a general missing-parallel policy. Skips tests with no detected subtests. Compares whether the parent calls Parallel against whether **any** detected child does, and reports a mismatch. With parallel children, reports parent `defer` when no parent Cleanup is detected. Neither parent nor children parallel is accepted; mixed children are not individually required once one is parallel. [T1] | Wrong default policy: valid parallel-parent/sequential-VM-steps tests are flagged, while fully sequential tests pass. Do not enable for this goal. |

### `paralleltest` configuration and limits

The pinned golangci-lint adapter exposes exactly these user settings: [L1, L3]

- `ignore-missing` (default false): true disables missing-call enforcement; **leave false**.
- `ignore-missing-subtests` (default false): true permits sequential children while retaining top-level checks. **Set true** for this repository.
- `check-cleanup` (default false): checks top-level `defer` when the analyzer detects top-level Parallel. **Leave false** here. Its implementation flags every such top-level defer, even if there are no parallel children; `defer` in a parallel top-level test is not inherently wrong. Actual lifetime rule: a parent's defer runs before its parallel children finish; Cleanup runs after descendants. [P1, G2]

The adapter also disables its old loop-variable check automatically for Go >=1.22. No `ignore-toplevel`, helper allowlist, “require a reason for Setenv,” or option to disable automatic environment exemptions exists in this pinned settings structure. Do not copy standalone `-ignoreloopVar` into YAML. [L1, L3]

Important implementation limits: [P1]

1. **Silent environment exemptions.** Recognized direct `t.Setenv` and `t.Chdir` calls suppress a missing top-level diagnostic; recognized range subtest blocking calls can exempt the parent too. Such tests need no `nolint`, so `nolintlint` has no explanation to validate. Adding `//nolint:paralleltest` merely to document such a test can produce an **unused directive** warning.
2. **Ordinary helper calls are not followed for top-level checks.** A helper calling Parallel does not satisfy the top-level check; a helper calling Setenv does not automatically exempt its caller. There is limited separate inspection of named/builder functions passed as subtest callbacks, not general interprocedural analysis.
3. **Syntactic, not a safety proof.** It primarily handles top-level expression statements and range statements; a valid conditional Parallel call can be missed, and nested callbacks/receiver-name matching are imperfect. It does not prove an unconditional call on the correct `T`, isolation of a shared VM, or safe production helper behavior. Keep direct, unconditional top-level calls as the normal style and review exceptional shapes.
4. **Build selection matters.** A normal lint run does not cover an opt-in `//go:build live` test. If the policy includes that test, lint with `--build-tags live` as an additional existing-command invocation; do not execute the live credential test merely to lint it. [R4, L4]

`tparallel` has no golangci-lint settings. Its helper detection is also limited: `IsCalled` examines the first SSA block and one layer of statically called functions; it is not an arbitrary call-graph/runtime analysis. Its README's parent/child consistency rule must not be confused with Go legality. [T1–T3, L3]

## Smallest integration that meets the reviewed-exceptions goal

### Proposed configuration (merge into existing blocks; do not replace allowlist)

```yaml
linters:
  enable:
    # Keep all existing entries, including nolintlint.
    - paralleltest
    - forbidigo
  settings:
    paralleltest:
      ignore-missing: false
      ignore-missing-subtests: true
      check-cleanup: false
    nolintlint:
      require-explanation: true   # Already present.
      require-specific: true      # Already present.
      allow-unused: false         # Default; spell out if desired.
    forbidigo:
      analyze-types: true
      forbid:
        - pattern: '^testing\.T\.(Setenv|Chdir)$'
          msg: 'Use isolated inputs, or explain this process-global test exception.'
        - pattern: '^os\.(Setenv|Unsetenv|Clearenv|Chdir)$'
          msg: 'Use child-process Env, or explain this process-global test exception.'
  exclusions:
    rules:
      - path-except: '_test\.go$'
        linters:
          - forbidigo
```

The extra linter is justified specifically by `paralleltest`'s implicit exemptions. Type-aware matching expands method references to `package.Type.Method` (pointers treated as their element type), so `testing.T.Setenv` matches `t.Setenv` regardless of the local `t` name. The exclusion scopes these new restrictions to test files rather than production environment-handling code. `forbidigo`'s standalone `//permit` escape hatch is disabled by its golangci-lint integration; use explained `nolint`. [F1, L3, L5]

**Guarantee and boundary:** ordinary missing-Parallel tests produce `paralleltest` findings; direct process-global mutation produces `forbidigo` findings even where `paralleltest` silently exempts it. Existing `nolintlint` then requires a specific named suppression with a nonempty explanation, and detects unused suppressions. This makes the known direct/helper patterns reviewable, not a formal guarantee for every possible Go AST/call-graph shape. Reasons are text, not proof or reviewer approval. Helper-level exceptions cover multiple callers; review helper changes and callers. Interface-based mutation or mutation inside production code is outside the narrow `forbidigo` rule's reach. [P1, F1, L6]

If only a **best-effort default** is wanted, omit `forbidigo` and accept the silent environment exemptions explicitly. If an absolute invariant over every top-level test is mandatory despite unusual code shapes, a small dedicated policy analyzer/checker is the justified next step: inspect actual test declarations, require a direct top-level call or a named reason-bearing exception, and recognize only the terminal wrapper as a special reviewed form. Neither stock linter offers that exact configurable invariant. Do not build that extra tool merely to reproduce common lint behavior. [Design recommendation from P1/T1 limitations]

### Examples of the intended policy

Independent scenario, sequential steps sharing one VM:

```go
func TestScenario(t *testing.T) {
    t.Parallel()
    s := startSandbox(t)
    // Deliberately sequential: both steps mutate this one VM.
    t.Run("configure", func(t *testing.T) { /* use s */ })
    t.Run("verify", func(t *testing.T) { /* use s */ })
}
```

Do **not** add Parallel to every child to satisfy a linter. Existing `TestDefaultToolsAreAvailable` shares its sandbox across subtests; terminal fallback subtests rewrite the same VM's executable and `.bashrc`. Top-level concurrency is the useful default, not concurrency between steps of a stateful scenario. `ignore-missing-subtests: true` accommodates both. If independent children are later made parallel, register parent-owned resource cleanup with `t.Cleanup` or a correctly scoped grouping `t.Run`, not a parent defer that closes resources before children run. [R3, R5, G2]

Genuine direct environment-behavior exception:

```go
//nolint:forbidigo // Verifies HOME/XDG lookup in-process; environment must be process-global.
func TestHostConfigLookup(t *testing.T) {
    t.Setenv("HOME", t.TempDir())
    // No t.Parallel: Go forbids this combination.
}
```

Helper-mediated exception needs a missing-call suppression at the test as well as an explained mutation exception at the helper:

```go
//nolint:forbidigo // Shared setup for tests of in-process host environment lookup.
func setHostEnvironment(t *testing.T) {
    t.Helper()
    t.Setenv("HOME", t.TempDir())
}

//nolint:paralleltest // setHostEnvironment changes process-global HOME for lookup under test.
func TestHostLookupWithHelper(t *testing.T) {
    setHostEnvironment(t)
}
```

Function-scoped `//nolint:linter // reason` suppressions are supported. Prefer them over file/package suppressions or blanket path exclusions. Keep only suppressions that actually suppress diagnostics. `nolintlint` does not require meaningful rationale, limit exception count, or authenticate review; code review must do that. [L5, L6]

### Terminal re-execution must remain special

`runInTerminal` checks `SANDBOX_TERMINAL_TEST_CHILD` **before** calling Parallel. In the parent it calls `t.Parallel`, selects exactly the current test with `-test.run=^<name>$`, launches a controlling-terminal child, and waits. In that isolated child it returns false without Parallel, allowing the body to use Setenv. **Do not move an unconditional Parallel call into these test bodies or add a second call in the parent.** Go panics on duplicate Parallel and on Setenv with a parallel test/ancestor. [R3, G2]

`paralleltest` does not follow the wrapper. For the fallback test without direct Setenv, use e.g. `//nolint:paralleltest // runInTerminal schedules only the parent; child stays sequential for VM/terminal isolation.` For the environment-forwarding test, direct Setenv already triggers the implicit paralleltest exemption: with the proposed rules, suppress **forbidigo**, explaining that mutation occurs only in the isolated terminal child. Do not add an unused paralleltest suppression. These are wrapper/analysis exceptions, not evidence that the parent was scheduled sequentially. [R3, P1, L6]

## Environment isolation is the main migration, not the lint toggle

`T.Setenv` affects the whole process. Go checks the current `T` and **all ancestors**: a sequential child under a parallel parent still cannot call it. Calling Setenv before Parallel also fails. A mutex around Setenv does not override this runtime rule, and raw `os.Setenv` only bypasses the guard, not shared-state interference. [G2]

For tests where environment is only fixture plumbing, prefer explicit config/settings inputs or child-process `exec.Cmd.Env`. Reserve in-process environment mutation for tests actually verifying host environment lookup/isolation. `Cmd.Env` sets the new child's environment; it need not mutate the parent. This should reduce the large exception population instead of blessing all 69 environment-mutating tests indefinitely. Whether each current helper can switch to explicit inputs is a separate implementation audit. [G3, R2; migration recommendation]

Do not replace the existing test runner with a custom goroutine launcher or process-per-test scheduler just to get defaults. `T.Parallel` uses parent barriers and testing's concurrency accounting; a custom launcher must reproduce that behavior. Process-per-test execution could isolate environment but adds test discovery, filtering, output/exit aggregation, fixture lifecycle, and capacity management. The existing terminal wrapper is a justified local exception, not a reason to redesign the entire runner. [G1, G2, R3; design tradeoff]

## Verification performed and remaining uncertainty

1. Read repository tooling/helpers and pinned Go source; queried installed versions. Repository was clean before research. No VM/image/live-service tests were run.
2. Ran a read-only lint probe: `golangci-lint run --no-config --enable-only paralleltest,tparallel --issues-exit-code 0 ./...`. It reported missing-call findings for ordinary/helper-driven tests, but not direct Setenv tests, and a `tparallel` complaint that `TestDefaultToolsAreAvailable`'s children should call Parallel. The printed **50 paralleltest / 1 tparallel** findings are probe output, **not an exhaustive census**: this run used default reporting limits and no live build tag.
3. Ran isolated synthetic lint fixtures with the proposed three-linter policy. Verified: direct Setenv produces forbidigo but no missing-Parallel finding; helper-mediated environment mutation produces a caller paralleltest finding plus helper forbidigo finding; parallel parent with two sequential children passes; conditional Parallel can be missed; explained suppressions work; unnecessary `nolint:paralleltest` on a directly exempt Setenv test produces an unused-directive finding; raw `os.Setenv` matches the additional forbidigo pattern. No fixture tests needed execution.
4. Not verified: full post-migration suite safety/performance, exact final exception count, CI actually gating merges on `mise lint`, lint with the live build tag, every possible alias/interface/helper shape, and the actual number of concurrently running VMs under terminal re-execution. Main findings about flags/linter algorithms are source-verified; migration costs and speedup remain unmeasured.

## Sources

Repository sources (relative links are local primary evidence):

- **R1:** [`go.mod`](../go.mod), [`mise.toml`](../mise.toml), [`.golangci.yml`](../.golangci.yml).
- **R2:** Prior audit supplied in task; examples: [`tests/aws_helpers_test.go`](../tests/aws_helpers_test.go), [`tests/tui_helpers_test.go`](../tests/tui_helpers_test.go), [`tests/settings_test.go`](../tests/settings_test.go), [`tests/launcher_test.go`](../tests/launcher_test.go).
- **R3:** [`tests/terminal_helpers_test.go`](../tests/terminal_helpers_test.go), [`tests/terminal_test.go`](../tests/terminal_test.go).
- **R4:** [`tests/openai_test.go`](../tests/openai_test.go), [`tests/opencode_helpers_test.go`](../tests/opencode_helpers_test.go): `//go:build live`.
- **R5:** [`tests/smoke_test.go`](../tests/smoke_test.go).

Upstream primary sources:

- **G1:** [Go 1.27.2 `go test` flags and test-command source](https://github.com/golang/go/blob/go1.27.2/src/cmd/go/internal/test/test.go), especially `HelpTestflag`'s `-parallel` and `-shuffle` definitions.
- **G2:** [Go 1.27.2 `testing` source](https://github.com/golang/go/blob/go1.27.2/src/testing/testing.go): subtest documentation; `T.Parallel`, `T.Run`, `T.checkParallel`, `T.Setenv`, `T.Chdir`, `Cleanup`, `tRunner`.
- **G3:** [Go 1.27.2 `os/exec.Cmd.Env` source/documentation](https://github.com/golang/go/blob/go1.27.2/src/os/exec/exec.go).
- **P1:** [paralleltest 1.0.16 analyzer](https://github.com/kunwardeep/paralleltest/blob/v1.0.16/pkg/paralleltest/paralleltest.go), especially `analyzeTestFunction`, `parallelBlockingMethodIsCalledInTestFunction`, `isTestFunction`.
- **T1:** [tparallel 0.3.2 analyzer](https://github.com/moricho/tparallel/blob/v0.3.2/tparallel.go).
- **T2:** [tparallel 0.3.2 SSA function inspection](https://github.com/moricho/tparallel/blob/v0.3.2/pkg/ssafunc/ssafunc.go).
- **T3:** [tparallel 0.3.2 called-function inspection](https://github.com/moricho/tparallel/blob/v0.3.2/pkg/ssainstr/ssainstr.go).
- **L1:** [golangci-lint 2.15.0 paralleltest adapter](https://github.com/golangci/golangci-lint/blob/v2.15.0/pkg/golinters/paralleltest/paralleltest.go).
- **L2:** [golangci-lint 2.15.0 dependencies](https://github.com/golangci/golangci-lint/blob/v2.15.0/go.mod).
- **L3:** [golangci-lint 2.15.0 reference configuration](https://github.com/golangci/golangci-lint/blob/v2.15.0/.golangci.reference.yml); [official linter settings](https://golangci-lint.run/docs/linters/configuration/#paralleltest) (includes tparallel's absence of settings).
- **L4:** [golangci-lint CLI configuration](https://golangci-lint.run/docs/configuration/cli/) (`--build-tags`).
- **L5:** [Official suppression/exclusion documentation](https://golangci-lint.run/docs/linters/false-positives/).
- **L6:** [golangci-lint 2.15.0 nolintlint implementation](https://github.com/golangci/golangci-lint/blob/v2.15.0/pkg/golinters/nolintlint/internal/nolintlint.go); [adapter enabling unused/explanation/specific checks](https://github.com/golangci/golangci-lint/blob/v2.15.0/pkg/golinters/nolintlint/nolintlint.go).
- **F1:** [forbidigo 2.3.1 original README](https://github.com/ashanbrown/forbidigo/blob/v2.3.1/README.md): type-aware matching, pointers, interface/embedded-type limits, and disabled `//permit` in golangci-lint.
