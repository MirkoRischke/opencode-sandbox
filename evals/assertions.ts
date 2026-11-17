import assert from "node:assert/strict";
import type { AssertionValueFunctionContext } from "promptfoo";

export function expectSkillActivation(shouldActivate: boolean) {
  return (_output: string, context: AssertionValueFunctionContext) => {
    const meta = context.providerResponse!.metadata!;
    const expected = meta.variant.name !== "none" && shouldActivate;
    const activated = meta.activatedSkills.includes("opencode-sandbox");
    return { pass: activated === expected, score: Number(activated === expected),
      reason: `skill activation: expected ${expected}, observed ${activated}` };
  };
}

export function validConfiguration(_output: string, context: AssertionValueFunctionContext) {
  const meta = context.providerResponse!.metadata!;
  assert.equal(meta.validation.valid, true, meta.validation.output);
  assert.ok(meta.config, "config must exist");
  return { pass: true, score: 1, reason: "real CLI validation passed and config exists" };
}
