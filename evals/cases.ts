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
      task: "Create this workspace's opencode-sandbox config with VM memory set to 4096 MiB. Only edit .opencode-sandbox.toml. The opencode-sandbox CLI is on PATH. Do not start VMs or edit global configuration. Briefly explain the result.",
    },
    assert: [
      { type: "javascript", value: expectSkillActivation(true) },
      { type: "javascript", value: (_output, context) => {
        const { config, files } = context.providerResponse!.metadata!;
        assert.deepEqual(files, [".opencode-sandbox.toml"]);
        assert.equal((parse(config).vm as Record<string, unknown>).memory_mib, 4096);
        return true;
      } },
    ],
  },
  {
    description: "Extend network access without replacing defaults",
    vars: {
      caseID: "network",
      fixture: { ".opencode-sandbox.toml": "[vm]\nmemory_mib = 8192\n" },
      task: "Update this workspace's opencode-sandbox config to also allow registry.npmjs.org, preserving existing settings and inherited default network access. Only edit .opencode-sandbox.toml. The opencode-sandbox CLI is on PATH. Do not start VMs or edit global configuration. Briefly explain the result.",
    },
    assert: [
      { type: "javascript", value: expectSkillActivation(true) },
      { type: "javascript", value: (_output, context) => {
        const { config, files } = context.providerResponse!.metadata!;
        assert.deepEqual(files, [".opencode-sandbox.toml"]);
        const parsed = parse(config);
        assert.equal((parsed.vm as Record<string, unknown>).memory_mib, 8192);
        const network = parsed.network as Record<string, unknown>;
        assert.ok(Array.isArray(network.additional_allowed_hosts));
        assert.ok(network.additional_allowed_hosts.includes("registry.npmjs.org"));
        assert.equal(Object.hasOwn(network, "allowed_hosts"), false);
        return true;
      } },
      {
        type: "llm-rubric",
        value: "The output JSON contains an answer and the actual config. The answer must accurately explain that registry.npmjs.org was added while retaining inherited default network access. Fail if it contradicts the actual config or claims unperformed VM operations.",
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
