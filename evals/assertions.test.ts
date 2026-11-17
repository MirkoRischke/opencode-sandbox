import assert from "node:assert/strict";
import { test } from "node:test";
import type { AssertionValueFunctionContext } from "promptfoo";
import { activationFor, configurationFor } from "./assertions.ts";
import { cases } from "./cases.ts";

const network = cases.find(c => c.id === "network")!;
const unrelated = cases.find(c => c.id === "unrelated")!;
const activation = activationFor(network);
const configuration = configurationFor(network);

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
  assert.equal(activationFor(unrelated)("", context({ activatedSkills: [] })).pass, true);
  assert.equal(activationFor(unrelated)("", context()).pass, false);
  assert.equal(activation("", context({ variant: { name: "none" }, activatedSkills: [] })).pass, true);
});

test("valid network extension passes", () => {
  assert.equal(configuration("", context()).pass, true);
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
  assert.equal(configurationFor(unrelated)("", context({ config: "[vm]\nmemory_mib = 8192\n" })).pass, true);
  assert.throws(() => configurationFor(unrelated)("", context()));
});

test("new cases supply their own checks and activation without runner changes", () => {
  const custom = {
    id: "custom", fixture: {}, prompt: "custom",
    assertions: {
      skill: "do-not-trigger" as const,
      check({ config }: { config: string }) { assert.equal(config, "expected"); },
    },
  };
  assert.equal(configurationFor(custom)("", context({ config: "expected" })).pass, true);
  assert.throws(() => configurationFor(custom)("", context()));
  assert.equal(activationFor(custom)("", context({ activatedSkills: [] })).pass, true);
});
