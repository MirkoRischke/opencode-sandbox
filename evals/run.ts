import { mkdir, mkdtemp, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { execFileSync } from "node:child_process";
import { performance } from "node:perf_hooks";
import type { OpenCode } from "@opencode/sdk";
import { OpenCodeProvider, type Variant } from "./adapter.ts";
import { cases } from "./cases.ts";
import { createEvalHost } from "./host.ts";
import { validConfiguration } from "./assertions.ts";

if (process.env.EVAL_CONTAINER !== "1") throw new Error("Run evaluations with npm run eval (Docker required)");
const provider = JSON.parse(await readFile("/input/provider.json", "utf8"));
if (!process.env.LLM_GATEWAY_KEY) throw new Error("LLM_GATEWAY_KEY required");
const started = performance.now();
const artifacts = "/results";
const state = await mkdtemp("/tmp/eval-state-");
for (const key of ["HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"]) {
  process.env[key] = join(state, key);
  await mkdir(process.env[key]!, { recursive: true });
}
process.env.PROMPTFOO_DISABLE_TELEMETRY = "1";
execFileSync("opencode-sandbox", ["config", "validate"], { cwd: state });
const variants: Variant[] = JSON.parse(await readFile("/input/variants.json", "utf8"));
const hosts: OpenCode.Interface[] = [];
try {
  for (const variant of variants) {
    const directory = join(state, `host-${variant.name}`);
    await mkdir(directory);
    hosts.push(await createEvalHost(directory, provider, variant));
  }
  const { default: promptfoo } = await import("promptfoo");
  const repetitions = Number(process.env.EVAL_REPETITIONS ?? 1);
  if (!Number.isInteger(repetitions) || repetitions < 1) throw new Error("EVAL_REPETITIONS must be a positive integer");
  const selected = cases.filter(c => !process.env.EVAL_CASE || c.vars?.caseID === process.env.EVAL_CASE);
  if (!selected.length) throw new Error("EVAL_CASE must name an existing case");
  const suite = await promptfoo.evaluate({
    prompts: ["{{task}}"],
    providers: variants.map((v, i) => new OpenCodeProvider(hosts[i], v, artifacts)),
    tests: selected,
    writeLatestResults: true,
    sharing: false,
    outputPath: [join(artifacts, "promptfoo.json"), join(artifacts, "report.html")],
    defaultTest: {
      assert: [{ type: "javascript", value: validConfiguration }],
      options: {
        provider: {
          id: `openai:chat:${process.env.EVAL_JUDGE_MODEL ?? "gpt-6.2-sol"}`,
          config: {
            apiBaseUrl: provider.settings.baseURL,
            apiKeyEnvar: "LLM_GATEWAY_KEY",
            temperature: 1, // LiteLLM requires this value when GPT-6.2 reasoning is active.
          },
        },
      },
    },
  }, {
    maxConcurrency: Number(process.env.EVAL_CONCURRENCY ?? 9), cache: false,
    repeat: repetitions,
  });
  const results = await suite.toEvaluateSummary();
  await Promise.all(hosts.map(host => host.close()));
  hosts.length = 0;
  // Persist results, not the suite configuration containing runtime judge credentials.
  await writeFile(join(artifacts, "summary.json"), JSON.stringify({
    demoHistory: process.env.EVAL_DEMO_HISTORY === "1", variants,
    versions: { node: process.version, opencode: "2.3.0", promptfoo: "0.125.1" },
    elapsedSeconds: (performance.now() - started) / 1000, stats: results.stats,
    results: results.results,
  }, null, 2));
  console.log(JSON.stringify({ stats: results.stats, seconds: (performance.now() - started) / 1000, artifacts }, null, 2));
  process.exitCode = results.stats.failures || results.stats.errors ? 1 : 0;
} finally {
  await Promise.all(hosts.map(host => host.close()));
}
