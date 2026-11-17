import assert from "node:assert/strict";
import { test } from "node:test";
import type { AssertionValueFunctionContext, TestCase } from "promptfoo";
import { expectSkillActivation, validConfiguration } from "./assertions.ts";
import { cases } from "./cases.ts";

const network = cases.find(c => c.vars?.caseID === "network")!;
const unrelated = cases.find(c => c.vars?.caseID === "unrelated")!;
const activation = expectSkillActivation(true);

function inlineCheck(testCase: TestCase, index: number) {
  const assertion = testCase.assert![index];
  if (assertion.type !== "javascript" || typeof assertion.value !== "function") {
    throw new Error("Expected inline JavaScript assertion");
  }
  return assertion.value;
}

// Activation returns a grading object; configuration checks return true.
const configuration = (_output: string, ctx: AssertionValueFunctionContext) => {
  validConfiguration("", ctx);
  assert.equal(inlineCheck(network, 1)("", ctx), true);
  return true;
};

function context(overrides = {}): AssertionValueFunctionContext {
  return { providerResponse: { metadata: {
    variant: { name: "local" }, caseID: "network",
    activatedSkills: ["opencode-sandbox"],
    files: [".opencode-sandbox.toml"],
    config: '[vm]\nmemory_mib = 8192\n[network]\nadditional_allowed_hosts = ["registry.npmjs.org"]\n',
    validation: { valid: true, output: "Configuration valid." },
    ...overrides,
  } } } as unknown as AssertionValueFunctionContext;
}

test("activation checks both positive and negative expectations", () => {
  assert.equal(activation("", context()).pass, true);
  assert.equal(activation("", context({ activatedSkills: [] })).pass, false);
  assert.equal(expectSkillActivation(false)("", context({ activatedSkills: [] })).pass, true);
  assert.equal(expectSkillActivation(false)("", context()).pass, false);
  assert.equal(activation("", context({ variant: { name: "none" }, activatedSkills: [] })).pass, true);
});

test("valid network extension passes", () => {
  assert.equal(configuration("", context()), true);
});

test("invalid CLI result cannot pass otherwise correct TOML", () => {
  assert.throws(() => configuration("", context({ validation: { valid: false, output: "invalid" } })));
});

test("replacement host list, changed memory, missing config, and extra files fail", () => {
  const config = context().providerResponse!.metadata!.config as string;
  for (const overrides of [
    { config: config.replace("additional_allowed_hosts", "allowed_hosts") },
    { config: config.replace("8192", "4096") },
    { config: null },
    { files: [".opencode-sandbox.toml", "unexpected.txt"] },
  ]) assert.throws(() => configuration("", context(overrides)));
});

test("unrelated case requires byte-for-byte unchanged config", () => {
  const check = inlineCheck(unrelated, 1);
  assert.equal(check("", context({ config: "[vm]\nmemory_mib = 8192\n" })), true);
  assert.throws(() => check("", context()));
});

test("new native cases supply their own checks without runner changes", () => {
  const custom: TestCase = {
    vars: { fixture: {}, task: "custom" },
    assert: [{
      type: "javascript",
      value: (_output, ctx) => {
        assert.equal(ctx.providerResponse!.metadata!.config, "expected");
        return true;
      },
    }],
  };
  const check = inlineCheck(custom, 0);
  assert.equal(check("", context({ config: "expected" })), true);
  assert.throws(() => check("", context()));
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
      vars: { task: "7 times 8", valid },
      assert: [{ type: "contains" as const, value: "56" }],
    })),
  }, { cache: false, repeat: 2 });
  const { stats } = await suite.toEvaluateSummary();
  assert.equal(stats.successes, 2);
  assert.equal(stats.failures, 2);
  assert.equal(stats.errors, 0);
});
