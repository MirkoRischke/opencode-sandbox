import { mkdir, mkdtemp, readFile, readdir, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import type { OpenCode } from "@opencode/sdk";
import type { MessageListOutput, SessionCreateOutput, SessionMessageInfo } from "@opencode/client";
import type { ApiProvider, CallApiContextParams, ProviderResponse } from "promptfoo";

const exec = promisify(execFile);
const workspaceConfigName = ".opencode-sandbox.toml";
const messageLimit = 200;
const agentTimeoutMs = 30_000;
const sdkTimeoutMs = 5_000;
export type Variant = { name: string; skillDirectory?: string; revision: string; hash?: string };

async function withDeadline<T>(ms: number, operation: (request: { signal: AbortSignal }) => Promise<T>): Promise<T> {
  const controller = new AbortController();
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    const deadline = new Promise<never>((_resolve, reject) => {
      timer = setTimeout(() => {
        reject(new Error(`Timed out after ${ms / 1000} seconds`));
        controller.abort();
      }, ms);
    });
    return await Promise.race([deadline, operation({ signal: controller.signal })]);
  } finally {
    clearTimeout(timer);
  }
}

async function prepareTrial(directory: string, vars: CallApiContextParams["vars"]) {
  const { fixture, parentConfig, globalConfig } = vars;
  const workspace = join(directory, "workspace");
  await mkdir(workspace);
  for (const [file, contents] of Object.entries(fixture as Record<string, string>)) {
    await writeFile(join(workspace, file), contents);
  }

  let parentPath: string | undefined;
  if (typeof parentConfig === "string") {
    parentPath = join(directory, workspaceConfigName);
    await writeFile(parentPath, parentConfig);
  }

  let globalPath: string | undefined;
  let shellEnvironment: Record<string, string> | undefined;
  if (typeof globalConfig === "string") {
    // Give parallel trials separate global launcher configs without changing process.env.
    const configHome = join(directory, "xdg-config");
    globalPath = join(configHome, "opencode-sandbox", "config.toml");
    await mkdir(dirname(globalPath), { recursive: true });
    await writeFile(globalPath, globalConfig);
    shellEnvironment = { XDG_CONFIG_HOME: configHome };
    for (const key of ["PATH", "HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "LANG", "SHELL"]) {
      const value = process.env[key];
      if (value !== undefined) shellEnvironment[key] = value;
    }
  }

  return { workspace, parentPath, globalPath, shellEnvironment };
}

async function readConfig(path: string) {
  try {
    return await readFile(path, "utf8");
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return null;
    throw error;
  }
}

async function inspectWorkspace(trial: Awaited<ReturnType<typeof prepareTrial>>) {
  const { workspace, parentPath, globalPath, shellEnvironment } = trial;
  const files = (await readdir(workspace, { recursive: true })).sort();
  const config = await readConfig(join(workspace, workspaceConfigName));
  const env = shellEnvironment && { ...process.env, ...shellEnvironment };
  let validation: { valid: boolean; output: string };
  try {
    const result = await exec("opencode-sandbox", ["config", "validate"], { cwd: workspace, env, timeout: 10_000 });
    validation = { valid: true, output: result.stdout + result.stderr };
  } catch (error) {
    validation = { valid: false, output: String(error) };
  }
  return {
    config, files, validation,
    ...(parentPath && { parentConfig: await readConfig(parentPath) }),
    ...(globalPath && { globalConfig: await readConfig(globalPath) }),
  };
}

function transcriptEvidence(data: SessionMessageInfo[]) {
  if (data.length === messageLimit) throw new Error("Trace reaches POC message limit; refusing incomplete assertions");
  const assistants = data.filter(message => message.type === "assistant");
  const failure = assistants.find(message => message.error);
  if (failure) throw new Error(JSON.stringify(failure.error));

  const parts = assistants.flatMap(message => message.content);
  return {
    answer: parts.flatMap(part => part.type === "text" ? [part.text] : []).join("\n"),
    activatedSkills: [
      ...data.filter(message => message.type === "skill").map(message => message.name),
      ...parts.flatMap(part =>
        part.type === "tool" && part.name === "skill" && part.state.status === "completed"
          ? [String((part.state.input as { id?: string }).id)] : []),
    ],
  };
}

export class OpenCodeProvider implements ApiProvider {
  private host: OpenCode.Interface;
  private variant: Variant;
  private artifacts: string;
  constructor(
    host: OpenCode.Interface,
    variant: Variant,
    artifacts: string,
  ) {
    this.host = host;
    this.variant = variant;
    this.artifacts = artifacts;
  }

  id() { return this.variant.name; }

  private async checkedSkillInventory(workspace: string, request: { signal: AbortSignal }) {
    const available = await this.host.skill.list({ location: { directory: workspace } }, request);
    const names = (available.data as { name: string }[]).map(skill => skill.name).sort();
    // Browser is supplied by the SDK after plugin transforms, not personal config.
    const expected = this.variant.skillDirectory ? ["Browser", "opencode-sandbox"] : ["Browser"];
    if (JSON.stringify(names) !== JSON.stringify(expected)) {
      throw new Error(`Unexpected skill inventory: ${JSON.stringify(names)}`);
    }
    return available;
  }

  async callApi(prompt: string, context?: CallApiContextParams): Promise<ProviderResponse> {
    const started = performance.now();
    const { vars } = context!;
    const { caseID } = vars;
    const directory = await mkdtemp(join(this.artifacts, `${caseID}-${this.variant.name}-`));
    const trial = await prepareTrial(directory, vars);
    const { workspace, shellEnvironment } = trial;
    const session = await withDeadline<SessionCreateOutput>(sdkTimeoutMs, request => this.host.sessions.create({
      location: { directory: workspace },
      model: { providerID: "litellm", id: process.env.EVAL_MODEL ?? "gpt-6.2-sol" },
      permissions: [{ action: "*", resource: "*", effect: "allow" }],
    }, request));
    let completed = false;
    try {
      if (shellEnvironment) {
        await withDeadline(sdkTimeoutMs, request =>
          this.host.session.environment({ sessionID: session.id, variables: shellEnvironment }, request));
      }
      let agentFinished = false;
      try {
        await withDeadline(agentTimeoutMs, async request => {
          await this.host.sessions.prompt({ sessionID: session.id, text: prompt }, request);
          request.signal.throwIfAborted();
          await this.host.session.wait({ sessionID: session.id }, request);
        });
        agentFinished = true;
      } finally {
        if (!agentFinished) {
          await withDeadline(sdkTimeoutMs, request =>
            this.host.session.interrupt({ sessionID: session.id }, request)).catch(() => {});
        }
      }

      const availableSkills = await withDeadline(sdkTimeoutMs, request => this.checkedSkillInventory(workspace, request));
      const messages = await withDeadline<MessageListOutput>(sdkTimeoutMs, request =>
        this.host.message.list({ sessionID: session.id, limit: messageLimit, order: "asc" }, request));
      const { answer, activatedSkills } = transcriptEvidence(messages.data);
      const result = await inspectWorkspace(trial);
      const metadata = {
        variant: this.variant, workspace, caseID,
        availableSkills, activatedSkills, ...result,
        elapsedMs: performance.now() - started,
      };
      await writeFile(join(directory, "trace.json"), JSON.stringify(messages, null, 2));
      await writeFile(join(directory, "result.json"), JSON.stringify({ answer, ...metadata }, null, 2));
      completed = true;
      return { output: JSON.stringify({ answer, config: result.config }), metadata };
    } finally {
      await withDeadline(sdkTimeoutMs, request => this.host.sessions.remove({ sessionID: session.id }, request))
        .catch(error => { if (completed) throw error; });
    }
  }
}
