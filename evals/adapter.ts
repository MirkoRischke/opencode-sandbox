import { mkdir, mkdtemp, readFile, readdir, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import type { OpenCode } from "@opencode/sdk";
import type { SessionMessageInfo } from "@opencode/client";
import type { ApiProvider, CallApiContextParams, ProviderResponse } from "promptfoo";
import { cases } from "./cases.ts";

const exec = promisify(execFile);
export type Variant = { name: string; skillDirectory?: string; revision: string; hash?: string };

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

  async callApi(prompt: string, context?: CallApiContextParams): Promise<ProviderResponse> {
    const started = performance.now();
    const fixture = cases.find(c => c.id === context?.vars.caseID);
    if (!fixture) throw new Error("Unknown fixture case");
    const trial = await mkdtemp(join(this.artifacts, `${fixture.id}-${this.variant.name}-`));
    const workspace = join(trial, "workspace");
    await mkdir(workspace);
    for (const [file, contents] of Object.entries(fixture.fixture)) {
      await writeFile(join(workspace, file), contents);
    }
    const request = { signal: AbortSignal.timeout(90_000) };
    const session = await this.host.sessions.create({
      location: { directory: workspace },
      model: { providerID: "litellm", id: process.env.EVAL_MODEL ?? "gpt-6.1-sol" },
      permissions: [{ action: "*", resource: "*", effect: "allow" }],
    }, request);
    try {
      await this.host.sessions.prompt({ sessionID: session.id, text: prompt }, request);
      await this.host.session.wait({ sessionID: session.id }, request);
      const available = await this.host.skill.list({ location: { directory: workspace } });
      const skillNames = (available.data as Array<{name: string}>).map(s => s.name).sort();
      // Browser is supplied by the SDK after plugin transforms, not personal config.
      const expected = this.variant.skillDirectory ? ["Browser", "opencode-sandbox"] : ["Browser"];
      if (JSON.stringify(skillNames) !== JSON.stringify(expected)) {
        throw new Error(`Unexpected skill inventory: ${JSON.stringify(skillNames)}`);
      }
      const messages = await this.host.message.list({ sessionID: session.id, limit: 200, order: "asc" }, request);
      if (messages.data.length === 200) throw new Error("Trace reaches POC message limit; refusing incomplete assertions");
      const data = messages.data as SessionMessageInfo[];
      const assistants = data.filter(m => m.type === "assistant");
      const failure = assistants.find(m => m.error);
      if (failure) throw new Error(JSON.stringify(failure.error));
      const answer = assistants.flatMap(m => m.content.flatMap(p => p.type === "text" ? [p.text] : [])).join("\n");
      const files = await readdir(workspace, { recursive: true });
      const config = await readFile(join(workspace, ".opencode-sandbox.toml"), "utf8").catch((error: NodeJS.ErrnoException) => {
        if (error.code === "ENOENT") return null;
        throw error;
      });
      let validation: { valid: boolean; output: string };
      try {
        const result = await exec("opencode-sandbox", ["config", "validate"], { cwd: workspace, timeout: 10_000 });
        validation = { valid: true, output: result.stdout + result.stderr };
      } catch (error) {
        validation = { valid: false, output: String(error) };
      }
      const metadata = {
        variant: this.variant, workspace, caseID: fixture.id,
        availableSkills: available,
        activatedSkills: [
          ...data.filter(m => m.type === "skill").map(m => m.name),
          ...assistants.flatMap(m => m.content.flatMap(p =>
            p.type === "tool" && p.name === "skill" && p.state.status === "completed"
              ? [String((p.state.input as { id?: string }).id)] : [])),
        ],
        config, files, validation, elapsedMs: performance.now() - started,
      };
      await writeFile(join(trial, "trace.json"), JSON.stringify(messages, null, 2));
      await writeFile(join(trial, "result.json"), JSON.stringify({ answer, ...metadata }, null, 2));
      return { output: JSON.stringify({ answer, config }), metadata };
    } finally {
      await this.host.sessions.remove({ sessionID: session.id });
    }
  }
}
