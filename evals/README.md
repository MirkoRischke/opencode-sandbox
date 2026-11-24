# Skill evaluation POC

Docker-based Promptfoo evaluations with our custom provider and the official
embedded OpenCode V2 SDK (`@opencode/sdk`), using the configured LiteLLM provider.
One disposable container runs the whole suite. No Harbor.

## Setup

From the repository root:

```sh
mise install
cd evals
npm ci
npm run build:cli
npm run build:image
npm test
```

Requires Docker (Linux containers), Go with CGO/a C compiler, Node, Git, and tar.
Use the toolchain pinned in [`mise.toml`](../mise.toml).
The image copies installed `node_modules` and the built Linux CLI from an
allowlisted temporary build context, not the repository checkout. Build with
Linux dependencies matching the container architecture. Rebuild the image after
changing evaluation code, dependencies, or the CLI; skill edits are snapshotted
at run time and need no rebuild. npm currently reports transitive dependency
audit findings; this is a local POC, not a hardened service.

The runner reads only `providers.litellm` from your OpenCode JSON config and uses
`LLM_GATEWAY_KEY` from the environment. It does not import personal plugins,
instructions, or skills. The judge uses the same provider endpoint directly.

`npm test` runs the assertion and Promptfoo integration tests inside the image.
No LiteLLM calls are needed. Environment isolation is supplied by Docker.
`host.ts` uses public `OpenCode.create()`; no internal factory, core configuration
layer override, or system/global discovery workaround remains.

The host launcher prepares local/previous skill snapshots and a minimal LiteLLM
provider configuration. Only the prepared input directory (read-only) and the
run's result directory (writable) are mounted. The gateway key is passed through
an environment variable, not stored in the input config. Inputs are deleted after
the run; the container is removed automatically. No host HOME, checkout, personal
OpenCode config, or Docker socket is mounted.

The public host CA bundle is copied into the input directory and used by Node,
OpenSSL, and curl so the sandbox HTTPS proxy is trusted without disabling TLS
verification. Override its path with `EVAL_CA_BUNDLE`; the Linux default is
`/etc/ssl/certs/ca-certificates.crt`.

## Run

By default, compare three variants: no skill, the working-tree skill (including
uncommitted edits), and the skill checked in at `HEAD`:

```sh
npm run eval
```

If the working-tree skill hasn't changed, both skill variants have the same
content. To compare against another revision, set `EVAL_PREVIOUS_REF`:

`EVAL_PREVIOUS_REF=<commit-or-tag> npm run eval`

The previous variant extracts the entire directory using `git archive`; missing
revisions/paths fail rather than silently falling back. The local variant snapshots
the working-tree directory, including uncommitted edits. Results record the resolved
previous SHA and archive hashes (hashes include archive metadata, not just text).
Only `SKILL.md` is registered, while referenced sibling files remain available.

```sh
EVAL_CASE=network npm run eval
EVAL_CONCURRENCY=12 EVAL_REPETITIONS=7 npm run eval
```

The second command repeats each case seven times across all three variants; it
does not add distinct cases. Defaults: three variants, concurrency nine, one
repetition. `EVAL_MODEL` and `EVAL_JUDGE_MODEL` select LiteLLM model IDs; both default to
`gpt-6.2-sol`. Agent-result caching is disabled; provider-side prompt caching may
still happen. Dependencies and the CLI are not rebuilt in the eval hot path.

## Assertions

Add or edit native Promptfoo `TestCase` entries in `cases.ts`. Each entry keeps
fixture, task, and standard assertions together. No adapter or runner changes
are needed:

```ts
{
  description: "Change memory while preserving settings",
  vars: {
    caseID: "memory",
    fixture: { ".opencode-sandbox.toml": "[vm]\nmemory_mib = 8192\n" },
    task: "Set workspace sandbox VM memory to 4096 MiB. Only edit the local config.",
  },
  assert: [
    activatesSkillWhenInstalled(),
    hasOnlyWorkspaceConfig(),
    hasMemoryMiB(4096),
    {
      type: "llm-rubric",
      value: "The explanation accurately describes the memory change in the config.",
    },
  ],
}
```

Use named helpers from `assertions.ts` for deterministic checks. The helpers
handle TOML parsing and Promptfoo metadata so cases can describe just what to check.
Shared `defaultTest.assert` requires CLI validation and an existing config for
every case; these checks and case assertions all must pass (no ordering assumed).
`doesNotActivateSkill()` checks non-activation for all variants, while
`activatesSkillWhenInstalled()` expects activation only when a skill is installed.
Optional `llm-rubric` assertions inherit
the judge provider from `defaultTest.options.provider`. Other Promptfoo assertion
types can be added directly, without extending a custom schema.
Use `EVAL_CASE=<id>` to run just the new case across the three variants. Add
assertion regression tests to `assertions.test.ts` when appropriate.

- Activation: successful `skill` tool calls or SDK skill messages, not answer text.
  Config cases require activation only for installed-skill variants; unrelated
  cases require non-activation. The SDK's built-in Browser skill is present equally
  in all variants; other built-ins are removed by an evaluation plugin.
- Deterministic TypeScript: mandatory real `opencode-sandbox config validate`,
  parsed TOML values, preservation of inherited network defaults, and file inventory.
- LLM judge: Promptfoo `llm-rubric` compares the network-task explanation to the
  actual resulting config. CLI validation remains authoritative for validity.

All variants have web search disabled. Each agent has 30 seconds to finish;
timed-out trials are interrupted and reported as errors, not graded answers.
All variants receive identical task prompts. No prompt explicitly tells the agent
to invoke the candidate skill. Positive activation measures model choice.

Each run uses fresh HOME/XDG state and separate SDK hosts/databases per variant;
each trial has its own session and fixture-only workspace. SDK hosts are reused
across a variant's trials. Project config/instruction discovery is disabled.
An SDK plugin registers the candidate using the official skill transform API.
No personal configuration files are written. This is isolation for reproducibility,
not per-trial security isolation. Agents have allowed tool permissions inside
the container and can inspect sibling trials, evaluation code/assertions, and
provider credentials. Do not use this setup for hostile agents. Host filesystem
isolation is supplied by Docker, not OpenCode tool permission rules.

`results/run-*/summary.json` contains assertion results/timing. Standard Promptfoo
exports are `promptfoo.json` and `report.html` (side-by-side results); open the HTML
on the host. Promptfoo internal state lives in the disposable container, with cloud sharing
disabled. Judge credentials are referenced by environment variable,
not embedded in exported config. Per-trial `trace.json` contains actual SDK
messages/tool calls. Promptfoo reports agent token usage separately from judge
usage (`assertions`). Per-trial `result.json` and Promptfoo response metadata
also record agent-only time, model turns, tool calls by name, and peak input
tokens per model request. On timeout the harness tries to save a partial trace
after interruption. Recovered usage is marked partial; missing usage is marked
unavailable. These diagnostics do not change grading.

Workspaces are retained for debugging; HOME/XDG state and SDK databases are
discarded with the container. Metadata paths such as `/results/...` refer to
container paths; their contents are under the printed host artifact directory. Exit status is nonzero for failures/errors. Keep results private:
traces and runtime state can contain sensitive model/provider data.

`docker.json` records total wall time (including snapshot preparation, container
startup/exit), image identity, and exit status. `summary.json` records versions
and suite time, including SDK startup, agent runs, assertions, judging, and SDK
shutdown. Builds/installations are excluded. Target roughly a minute for small suites; benchmark
gateway concurrency before extrapolating to 20 cases across three variants.

## Measured POC results

Live runs on 2026-11-17 used the tools pinned at the time, the configured LiteLLM
gateway, agent/judge `gpt-6.1-sol`, and no Promptfoo agent-result caching:

| Workload | Concurrency | Judge calls | Result | Wall time |
| --- | ---: | ---: | --- | ---: |
| 3 cases x 3 variants | 9 | 3 | 9 passed, 0 failures/errors | 37 s |
| Same cases repeated 7 times x 3 variants | 12 | 21 | 63 passed, 0 failures/errors | 130 s |

Both use synthetic demo Git history. These establish integration and approximate
throughput, not skill improvement or a guarantee for longer, distinct scenarios.

### Docker switch verification

On 2026-11-17, the 3-case x 3-variant suite completed in **32.8 seconds**
(including Docker startup/exit; 31.7 seconds inside the container). Seven passed,
two failed, zero runtime errors. Local and synthetic-previous skills passed all
cases. The no-skill baseline asked for platform details instead of creating the
RAM config, and tested live npm connectivity instead of editing the network
config. Its HTTPS request succeeded with certificate verification enabled.
These are evaluation failures, not container failures; the launcher correctly
returned exit status 1. Artifacts: `results/run-1794955219862/`.
