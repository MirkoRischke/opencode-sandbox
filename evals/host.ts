import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { parse as yaml } from "yaml";
import type { OpenCode } from "@opencode/sdk";
import type { Variant } from "./adapter.ts";

export async function createEvalHost(
  directory: string,
  provider: Record<string, unknown>,
  variant: Variant,
): Promise<OpenCode.Interface> {
  const { OpenCode: SDK } = await import("@opencode/sdk");
  const { Plugin, Skill } = await import("@opencode/plugin");
  const skillFile = variant.skillDirectory && join(variant.skillDirectory, "opencode-sandbox/SKILL.md");
  const markdown = skillFile && await readFile(skillFile, "utf8");
  const parts = markdown && /^---\n([\s\S]*?)\n---\n([\s\S]*)$/.exec(markdown);
  if (skillFile && !parts) throw new Error(`Invalid skill frontmatter: ${skillFile}`);
  const frontmatter = parts && yaml(parts[1]);
  const config = { directory, project: false, claude: false, content: JSON.stringify({
    "$schema": "https://opencode.ai/config.json",
    providers: { litellm: provider },
    snapshots: false,
    websearch: false,
  }) };
  return SDK.create({
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
    config,
    models: { fetch: false },
  });
}
