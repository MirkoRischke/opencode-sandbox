import { cp, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { homedir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { execFileSync, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { performance } from "node:perf_hooks";
import type { Variant } from "./adapter.ts";

const directory = dirname(fileURLToPath(import.meta.url));
const root = resolve(directory, "..");
const image = "opencode-sandbox-evals-v2:local";
const args = process.argv.slice(2);
const command = args.shift();
const imageID = () => execFileSync("docker", ["image", "inspect", image, "--format", "{{.Id}}"], { encoding: "utf8" }).trim();
if (command === "build") {
  // An allowlisted context prevents checkout/config files entering the image.
  await mkdir(join(directory, "results"), { recursive: true });
  const context = await mkdtemp(join(directory, "results/build-"));
  try {
    for (const file of ["Dockerfile", "package.json", "run.ts", "adapter.ts", "host.ts", "cases.ts", "assertions.ts", "assertions.test.ts"]) {
      await cp(join(directory, file), join(context, file));
    }
    await cp(join(directory, "node_modules"), join(context, "node_modules"), { recursive: true });
    await cp(join(root, "bin/opencode-sandbox"), join(context, "opencode-sandbox"));
    execFileSync("docker", ["build", "-t", image, context], { stdio: "inherit" });
  } finally {
    await rm(context, { recursive: true, force: true });
  }
} else if (command === "test") {
  const result = spawnSync("docker", ["run", "--rm", image, "node", "--test", "assertions.test.ts"], { stdio: "inherit" });
  if (result.error) throw result.error;
  process.exitCode = result.status ?? 1;
} else if (command === "run") {
  if (args.some(arg => arg !== "--demo-history")) throw new Error("Only --demo-history is supported");
  if (!process.env.LLM_GATEWAY_KEY) throw new Error("LLM_GATEWAY_KEY required");
  const resolvedImage = imageID();
  const personal = JSON.parse(await readFile(join(process.env.XDG_CONFIG_HOME ?? join(homedir(), ".config"), "opencode/opencode.json"), "utf8"));
  const provider = personal.providers?.litellm;
  if (!provider) throw new Error("LiteLLM provider required");
  // Do not copy personal provider extras or persist a literal credential.
  const minimalProvider = {
    name: provider.name, package: provider.package, models: provider.models,
    settings: { baseURL: provider.settings.baseURL, apiKey: "{env:LLM_GATEWAY_KEY}" },
  };
  const artifacts = join(directory, "results", `run-${Date.now()}`);
  await mkdir(artifacts, { recursive: true });
  const input = await mkdtemp(join(directory, "results/input-"));
  const started = performance.now();
  try {
    const skillPath = "skills/opencode-sandbox";
    const local = await readFile(join(root, skillPath, "SKILL.md"), "utf8");
    await mkdir(join(input, "local"));
    await cp(join(root, skillPath), join(input, "local/opencode-sandbox"), { recursive: true });
    let gitRoot = root;
    let ref = process.env.EVAL_PREVIOUS_REF;
    const demo = args.includes("--demo-history");
    if (demo) {
      gitRoot = join(input, "demo-history");
      await mkdir(join(gitRoot, skillPath), { recursive: true });
      await writeFile(join(gitRoot, skillPath, "SKILL.md"), local.replace(
        "   For network access, add hosts to `[network] allowed_hosts`;\n   inherited entries stay in effect.\n", ""));
      execFileSync("git", ["init", "-q", gitRoot]);
      execFileSync("git", ["-C", gitRoot, "add", skillPath]);
      execFileSync("git", ["-C", gitRoot, "-c", "user.name=Eval demo", "-c", "user.email=eval@example.invalid", "commit", "-qm", "Synthetic previous skill for POC"]);
      ref = "HEAD";
    }
    if (!ref) throw new Error("Set EVAL_PREVIOUS_REF or use --demo-history");
    const sha = execFileSync("git", ["-C", gitRoot, "rev-parse", "--verify", `${ref}^{commit}`], { encoding: "utf8" }).trim();
    const archive = execFileSync("git", ["-C", gitRoot, "archive", sha, skillPath]);
    await mkdir(join(input, "previous"));
    execFileSync("tar", ["-x", "-C", join(input, "previous")], { input: archive });
    if (demo) await rm(gitRoot, { recursive: true });
    const hash = (data: Buffer) => createHash("sha256").update(data).digest("hex");
    const variants: Variant[] = [
      { name: "none", revision: "none" },
      { name: "local", revision: "working-tree", skillDirectory: "/input/local", hash: hash(execFileSync("tar", ["-c", "-C", join(input, "local"), "opencode-sandbox"])) },
      { name: "previous", revision: sha, skillDirectory: "/input/previous/skills", hash: hash(archive) },
    ];
    await writeFile(join(input, "provider.json"), JSON.stringify(minimalProvider), { mode: 0o600 });
    await writeFile(join(input, "variants.json"), JSON.stringify(variants));
    // Copy public trust material only, never mount the host's config directories.
    await cp(process.env.EVAL_CA_BUNDLE ?? "/etc/ssl/certs/ca-certificates.crt", join(input, "ca.pem"));
    const dockerArgs = ["run", "--rm", "--env", "LLM_GATEWAY_KEY",
      "--env", "NODE_EXTRA_CA_CERTS=/input/ca.pem", "--env", "CURL_CA_BUNDLE=/input/ca.pem",
      "--env", "SSL_CERT_FILE=/input/ca.pem", "--env", `EVAL_DEMO_HISTORY=${demo ? "1" : "0"}`,
      "--mount", `type=bind,source=${input},target=/input,readonly`,
      "--mount", `type=bind,source=${artifacts},target=/results`];
    for (const key of ["EVAL_CASE", "EVAL_CONCURRENCY", "EVAL_REPETITIONS", "EVAL_MODEL", "EVAL_JUDGE_MODEL"]) {
      if (process.env[key] !== undefined) dockerArgs.push("--env", key);
    }
    const result = spawnSync("docker", [...dockerArgs, resolvedImage], { stdio: "inherit" });
    if (result.error) throw result.error;
    const seconds = (performance.now() - started) / 1000;
    await writeFile(join(artifacts, "docker.json"), JSON.stringify({ image, imageID: resolvedImage, seconds, exitCode: result.status }, null, 2));
    console.log(JSON.stringify({ artifacts, seconds, exitCode: result.status }, null, 2));
    process.exitCode = result.status ?? 1;
  } finally {
    await rm(input, { recursive: true, force: true });
  }
} else {
  throw new Error("Expected build, test, or run");
}
