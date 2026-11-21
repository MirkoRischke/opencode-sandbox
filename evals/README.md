# Skill evaluation POC

Host-based Promptfoo evaluations with the official embedded OpenCode V2 SDK
(`@opencode/sdk`), using the configured LiteLLM provider. No Docker or Harbor.

## Setup

From the repository root:

```sh
mise install
cd evals
npm ci
npm run build:cli
npm test
```

Requires Go with CGO/a C compiler, Node, Git, and tar. Use the toolchain pinned
in [`mise.toml`](../mise.toml). npm currently reports transitive dependency
audit findings; this is a local POC, not a hardened service.

The runner reads only `providers.litellm` from your OpenCode JSON config and uses
`LLM_GATEWAY_KEY` from the environment. It does not import personal plugins,
instructions, or skills. The judge uses the same provider endpoint directly.

## Run

To demonstrate all three variants without choosing an earlier skill revision:

```sh
npm run eval -- --demo-history
```

This creates a temporary Git repository with a **synthetic** earlier skill lacking
the network-extension guidance. It is a plumbing demonstration, not a historical
quality comparison. It neither commits to nor modifies this repository's history.

For a real comparison, select an earlier revision containing a different
version of `skills/opencode-sandbox/`:

```sh
EVAL_PREVIOUS_REF=<commit-or-tag> npm run eval
```

The previous variant extracts the entire directory using `git archive`; missing
revisions/paths fail rather than silently falling back. The local variant snapshots
the working-tree directory, including uncommitted edits. Results record the resolved
previous SHA and archive hashes (hashes include archive metadata, not just text).
Only `SKILL.md` is registered, while referenced sibling files remain available.

```sh
EVAL_CASE=network npm run eval -- --demo-history
EVAL_CONCURRENCY=12 EVAL_REPETITIONS=7 npm run eval -- --demo-history
```

The second command is a 63-agent-run throughput probe, **not 21 distinct cases**.
Defaults: three cases, three variants, concurrency nine, one repetition. Optional
`EVAL_MODEL` and `EVAL_JUDGE_MODEL` select LiteLLM model IDs; both default to
`gpt-6.1-sol`. Agent-result caching is disabled; provider-side prompt caching may
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
    { type: "javascript", value: expectSkillActivation(true) },
    { type: "javascript", value: (_output, context) => {
      const { config, files } = context.providerResponse!.metadata!;
      assert.deepEqual(files, [".opencode-sandbox.toml"]);
      assert.equal((parse(config).vm as Record<string, unknown>).memory_mib, 4096);
      return true;
    } },
    {
      type: "llm-rubric",
      value: "The explanation accurately describes the memory change in the config.",
    },
  ],
}
```

Inline JavaScript assertions use ordinary TypeScript/Node assertions: throw to
fail and return `true` to pass. `files` is the sorted workspace inventory.
Shared `defaultTest.assert` requires CLI validation and an existing config for
every case; these checks and case assertions all must pass (no ordering assumed).
`expectSkillActivation(false)` checks non-activation; the helper always expects
non-activation for the no-skill baseline. Optional `llm-rubric` assertions inherit
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

All variants receive identical task prompts. No prompt explicitly tells the agent
to invoke the candidate skill. Positive activation measures model choice.

Each run uses fresh HOME/XDG state and separate SDK hosts/databases per variant;
each trial has its own session and fixture-only workspace. SDK hosts are reused
across a variant's trials. Project config/instruction discovery is disabled.
An SDK plugin registers the candidate using the official skill transform API.
No personal configuration files are written. This is isolation for reproducibility,
not a security sandbox; agents run with allowed tool permissions inside the VM.

`results/run-*/summary.json` contains assertion results/timing. Standard Promptfoo
exports are `promptfoo.json` and `report.html` (side-by-side results); open the HTML
on the host. Promptfoo also persists results in the run's isolated HOME, with
cloud sharing disabled. Judge credentials are referenced by environment variable,
not embedded in exported config. Per-trial `trace.json` contains actual SDK
messages/tool calls. Workspaces and isolated state are retained
for debugging. Exit status is nonzero for failures/errors. Keep results private:
traces and runtime state can contain sensitive model/provider data.

Runtime includes SDK startup, workspace preparation, agent runs, assertions,
judging, and SDK shutdown. Target roughly a minute for small suites; benchmark
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
