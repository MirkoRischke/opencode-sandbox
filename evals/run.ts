import { cp, mkdir, mkdtemp, readFile, writeFile } from "node:fs/promises";
import { homedir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { parse as yaml } from "yaml";
import { performance } from "node:perf_hooks";
import type { OpenCode } from "@opencode/sdk";
import { OpenCodeProvider, type Variant } from "./adapter.ts";
import { cases } from "./cases.ts";
import { validConfiguration } from "./assertions.ts";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const configHome = process.env.XDG_CONFIG_HOME ?? join(homedir(), ".config");
const personal = JSON.parse(await readFile(join(configHome, "opencode/opencode.json"), "utf8"));
const provider = personal.providers?.litellm;
if (!provider || !process.env.LLM_GATEWAY_KEY) throw new Error("LiteLLM provider and LLM_GATEWAY_KEY required");
const started = performance.now();
const artifacts = join(root, "evals/results", `run-${Date.now()}`);
await mkdir(artifacts, { recursive: true });
// Set isolation before importing Promptfoo or constructing OpenCode hosts.
const state = await mkdtemp(join(artifacts, "state-"));
for (const key of ["HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"]) {
  process.env[key] = join(state, key);
  await mkdir(process.env[key]!, { recursive: true });
}
delete process.env.OPENCODE_CONFIG_CONTENT;
delete process.env.OPENCODE_CONFIG;
delete process.env.OPENCODE_CONFIG_DIR;
process.env.PATH = `${join(root, "bin")}:${process.env.PATH}`;
process.env.PROMPTFOO_DISABLE_TELEMETRY = "1";
execFileSync("opencode-sandbox", ["config", "validate"], { cwd: state });

const skillPath = "skills/opencode-sandbox";
const local = await readFile(join(root, skillPath, "SKILL.md"), "utf8");
const localSnapshot = join(state, "local-skill");
await mkdir(localSnapshot);
await cp(join(root, skillPath), join(localSnapshot, "opencode-sandbox"), { recursive: true });
let gitRoot = root;
let ref = process.env.EVAL_PREVIOUS_REF;
if (process.argv.includes("--demo-history")) {
  // No historical skill exists yet. Demonstrate Git extraction without committing to the user's repo.
  gitRoot = join(state, "demo-history");
  await mkdir(join(gitRoot, skillPath), { recursive: true });
  await writeFile(join(gitRoot, skillPath, "SKILL.md"), local.replace(
    "   To extend network access, use `[network] additional_allowed_hosts`;\n   `allowed_hosts` replaces inherited defaults.\n", ""));
  execFileSync("git", ["init", "-q", gitRoot]);
  execFileSync("git", ["-C", gitRoot, "add", skillPath]);
  execFileSync("git", ["-C", gitRoot, "-c", "user.name=Eval demo", "-c", "user.email=eval@example.invalid", "commit", "-qm", "Synthetic previous skill for POC"]);
  ref = "HEAD";
}
if (!ref) throw new Error("Set EVAL_PREVIOUS_REF to a Git revision containing the skill, or use --demo-history for the initial POC");
const sha = execFileSync("git", ["-C", gitRoot, "rev-parse", "--verify", `${ref}^{commit}`], { encoding: "utf8" }).trim();
const previous = join(state, "previous");
await mkdir(previous);
const archive = execFileSync("git", ["-C", gitRoot, "archive", sha, skillPath]);
execFileSync("tar", ["-x", "-C", previous], { input: archive });
const hash = (data: Buffer) => createHash("sha256").update(data).digest("hex");
const variants: Variant[] = [
  { name: "none", revision: "none" },
  { name: "local", revision: "working-tree", skillDirectory: localSnapshot,
    hash: hash(execFileSync("tar", ["-c", "-C", localSnapshot, "opencode-sandbox"])) },
  { name: "previous", revision: sha, skillDirectory: join(previous, "skills"), hash: hash(archive) },
];
const hosts: OpenCode.Interface[] = [];
const { OpenCode: SDK } = await import("@opencode/sdk");
const { Plugin, Skill } = await import("@opencode/plugin");
try {
  for (const variant of variants) {
    const directory = join(state, `host-${variant.name}`);
    await mkdir(directory);
    const skillFile = variant.skillDirectory && join(variant.skillDirectory, "opencode-sandbox/SKILL.md");
    const markdown = skillFile && await readFile(skillFile, "utf8");
    const parts = markdown && /^---\n([\s\S]*?)\n---\n([\s\S]*)$/.exec(markdown);
    if (skillFile && !parts) throw new Error(`Invalid skill frontmatter: ${skillFile}`);
    const frontmatter = parts && yaml(parts[1]);
    hosts.push(await SDK.create({
      plugins: [Plugin.define({
        id: "eval.candidate-skill",
        async setup(ctx) {
          await ctx.skill.transform(editor => {
            for (const skill of editor.list()) editor.remove(skill.id);
            if (skillFile && parts) editor.add({
              id: Skill.ID.make(frontmatter.name), name: Skill.Name.make(frontmatter.name),
              description: frontmatter.description,
              path: Skill.Info.fields.path.make(skillFile), content: parts[2],
            });
          });
        },
      })],
      database: { path: join(directory, "opencode.db") },
      config: { directory, project: false, claude: false, content: JSON.stringify({
        "$schema": "https://opencode.ai/config.json",
        providers: { litellm: provider },
        skills: variant.skillDirectory ? [variant.skillDirectory] : [],
        snapshots: false,
      }) },
      models: { fetch: false },
    }));
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
          id: `openai:chat:${process.env.EVAL_JUDGE_MODEL ?? "gpt-6.1-sol"}`,
          config: { apiBaseUrl: provider.settings.baseURL, apiKeyEnvar: "LLM_GATEWAY_KEY" },
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
    demoHistory: process.argv.includes("--demo-history"), variants,
    elapsedSeconds: (performance.now() - started) / 1000, stats: results.stats,
    results: results.results,
  }, null, 2));
  console.log(JSON.stringify({ stats: results.stats, seconds: (performance.now() - started) / 1000, artifacts }, null, 2));
  process.exitCode = results.stats.failures || results.stats.errors ? 1 : 0;
} finally {
  await Promise.all(hosts.map(host => host.close()));
}
