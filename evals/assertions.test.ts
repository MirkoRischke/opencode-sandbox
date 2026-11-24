import assert from "node:assert/strict";
import { test } from "node:test";
import type { AssertionValueFunctionContext } from "promptfoo";
import {
  activatesSkillWhenInstalled, allowsNetworkHost, doesNotActivateSkill,
  hasCpuCount, hasMemoryMiB, hasOnlyWorkspaceConfig, leavesInheritedConfigsUnchanged,
  leavesWorkspaceConfigUnchanged, validConfiguration,
} from "./assertions.ts";
import { cases } from "./cases.ts";

const network = cases.find(c => c.vars?.caseID === "network")!;
const fixtureConfig = "[vm]\nmemory_mib = 8192\n";

function context(overrides = {}): AssertionValueFunctionContext {
  return { vars: { fixture: { ".opencode-sandbox.toml": fixtureConfig } }, providerResponse: { metadata: {
    variant: { name: "local" }, caseID: "network",
    activatedSkills: ["opencode-sandbox"],
    files: [".opencode-sandbox.toml"],
    config: '[vm]\nmemory_mib = 8192\n[network]\nallowed_hosts = ["registry.npmjs.org"]\n',
    validation: { valid: true, output: "Configuration valid." },
    ...overrides,
  } } } as unknown as AssertionValueFunctionContext;
}

test("activation checks both positive and negative expectations", () => {
  const activation = activatesSkillWhenInstalled().value;
  const noActivation = doesNotActivateSkill().value;
  assert.equal(activation("", context()).pass, true);
  assert.equal(activation("", context({ activatedSkills: [] })).pass, false);
  assert.equal(noActivation("", context({ activatedSkills: [] })).pass, true);
  assert.equal(noActivation("", context()).pass, false);
  assert.equal(activation("", context({ variant: { name: "none" }, activatedSkills: [] })).pass, true);
});

test("network host check requires the requested host", () => {
  const check = allowsNetworkHost("registry.npmjs.org").value;
  assert.equal(check("", context()), true);
  assert.throws(() => check("", context({ config: fixtureConfig })), /network.allowed_hosts/);
  assert.throws(() => check("", context({ config: '[network]\nallowed_hosts = ["example.org"]\n' })), /registry.npmjs.org/);
});

test("invalid CLI result cannot pass otherwise correct TOML", () => {
  assert.throws(() => validConfiguration("", context({ validation: { valid: false, output: "invalid" } })), /invalid/);
});

test("memory check requires the expected value and an existing config", () => {
  const check = hasMemoryMiB(8192).value;
  assert.equal(check("", context()), true);
  assert.throws(() => check("", context({ config: "[vm]\nmemory_mib = 4096\n" })), /vm.memory_mib/);
  assert.throws(() => check("", context({ config: null })), /must exist/);
});

test("CPU count check requires the supported key and preserves the requested count", () => {
  const check = hasCpuCount(2).value;
  assert.equal(check("", context({ config: "[vm]\ncpus = 2\n" })), true);
  assert.throws(() => check("", context({ config: "[vm]\ncores = 2\n" })), /vm.cpus/);
  assert.throws(() => check("", context({ config: "[vm]\ncpus = 4\n" })), /vm.cpus/);
});

test("workspace config must be the only file", () => {
  const check = hasOnlyWorkspaceConfig().value;
  assert.equal(check("", context()), true);
  assert.throws(() => check("", context({ files: [".opencode-sandbox.toml", "unexpected.txt"] })));
});

test("unrelated case requires byte-for-byte unchanged config", () => {
  const check = leavesWorkspaceConfigUnchanged().value;
  assert.equal(check("", context({ config: fixtureConfig })), true);
  assert.throws(() => check("", context({ config: fixtureConfig + "\n" })), /must be unchanged/);
});

test("inherited configs must both remain unchanged", () => {
  const check = leavesInheritedConfigsUnchanged().value;
  const vars = { parentConfig: "parent", globalConfig: "global" };
  const trial = (overrides = {}) => ({ ...context({ ...vars, ...overrides }), vars });
  assert.equal(check("", trial()), true);
  assert.throws(() => check("", trial({ parentConfig: "changed" })), /parentConfig must be unchanged/);
  assert.throws(() => check("", trial({ globalConfig: null })), /globalConfig must be unchanged/);
});

test("Promptfoo merges mandatory validation, supports built-ins, and repeats native cases", async () => {
  process.env.PROMPTFOO_DISABLE_TELEMETRY = "1";
  const { default: promptfoo } = await import("promptfoo");
  const suite = await promptfoo.evaluate({
    prompts: ["{{task}}"],
    providers: [{
      id: () => "fake-agent",
      async callApi(_prompt, ctx) {
        return {
          output: "56",
          metadata: context({ validation: { valid: ctx!.vars.valid, output: "invalid" } }).providerResponse!.metadata,
        };
      },
    }],
    defaultTest: { assert: [{ type: "javascript", value: validConfiguration }] },
    tests: [true, false].map(valid => ({
      vars: { ...network.vars, valid },
      assert: [...network.assert!.filter(a => a.type !== "llm-rubric"), { type: "contains" as const, value: "56" }],
    })),
  }, { cache: false, repeat: 2 });
  const { stats } = await suite.toEvaluateSummary();
  assert.equal(stats.successes, 2);
  assert.equal(stats.failures, 2);
  assert.equal(stats.errors, 0);
});
