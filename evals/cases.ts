import assert from "node:assert/strict";
import { parse } from "smol-toml";
import type { TestCase } from "promptfoo";
import { expectSkillActivation } from "./assertions.ts";

export const cases: TestCase[] = [
  {
    description: "Create workspace config",
    vars: {
      caseID: "create",
      fixture: {},
      task: "I want the sandbox to use 1234MB of RAM",
    },
    assert: [
      { type: "javascript", value: expectSkillActivation(true) },
      { type: "javascript", value: (_output, context) => {
        const { config, files } = context.providerResponse!.metadata!;
        assert.deepEqual(files, [".opencode-sandbox.toml"]);
        // Treat informal MB as the CLI's MiB resource unit.
        assert.equal((parse(config).vm as Record<string, unknown>).memory_mib, 1234);
        return true;
      } },
    ],
  },
  {
    description: "Allow a project-specific network host",
    vars: {
      caseID: "network",
      fixture: { ".opencode-sandbox.toml": "[vm]\nmemory_mib = 8192\n" },
      task: "I want to be able to reach registry.npmjs.org from within my sandbox",
    },
    assert: [
      { type: "javascript", value: expectSkillActivation(true) },
      { type: "javascript", value: (_output, context) => {
        const { config, files } = context.providerResponse!.metadata!;
        assert.deepEqual(files, [".opencode-sandbox.toml"]);
        const parsed = parse(config);
        assert.equal((parsed.vm as Record<string, unknown>).memory_mib, 8192);
        const network = parsed.network as Record<string, unknown>;
        assert.ok(Array.isArray(network.allowed_hosts));
        assert.ok(network.allowed_hosts.includes("registry.npmjs.org"));
        return true;
      } },
      {
        type: "llm-rubric",
        value: "The output JSON contains an answer and the actual config. The answer must accurately explain that registry.npmjs.org was allowed. Fail if it contradicts the actual config or claims unperformed VM operations.",
      },
    ],
  },
  {
    description: "Answer unrelated question without loading the skill",
    vars: {
      caseID: "unrelated",
      fixture: { ".opencode-sandbox.toml": "[vm]\nmemory_mib = 8192\n" },
      task: "What is 7 times 8? Answer directly without changing files.",
    },
    assert: [
      { type: "javascript", value: expectSkillActivation(false) },
      { type: "javascript", value: (_output, context) => {
        const { config, files } = context.providerResponse!.metadata!;
        assert.deepEqual(files, [".opencode-sandbox.toml"]);
        assert.equal(config, "[vm]\nmemory_mib = 8192\n");
        return true;
      } },
    ],
  },
];
