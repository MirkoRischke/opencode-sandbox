import assert from "node:assert/strict";
import type { AssertionValueFunctionContext } from "promptfoo";
import type { EvalCase } from "./cases.ts";

export function activationFor(testCase: EvalCase) {
  return (_output: string, context: AssertionValueFunctionContext) => {
    const meta = context.providerResponse!.metadata!;
    const expected = meta.variant.name !== "none" && testCase.assertions.skill === "trigger";
    const activated = meta.activatedSkills.includes("opencode-sandbox");
    return { pass: activated === expected, score: Number(activated === expected),
      reason: `skill activation: expected ${expected}, observed ${activated}` };
  };
}

export function configurationFor(testCase: EvalCase) {
  return (_output: string, context: AssertionValueFunctionContext) => {
    const meta = context.providerResponse!.metadata!;
    assert.equal(meta.validation.valid, true, meta.validation.output);
    assert.ok(meta.config, "config must exist");
    testCase.assertions.check({ config: meta.config, files: [...meta.files].sort() });
    return { pass: true, score: 1, reason: "real CLI validation and fixture assertions passed" };
  };
}
