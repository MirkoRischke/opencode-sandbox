import assert from "node:assert/strict";
import type { AssertionValueFunctionContext } from "promptfoo";
import { parse } from "smol-toml";

const configFile = ".opencode-sandbox.toml";

function check<Result>(evaluate: (context: AssertionValueFunctionContext) => Result) {
  return { type: "javascript" as const, value: (_output: string, context: AssertionValueFunctionContext) => evaluate(context) };
}

function metadata(context: AssertionValueFunctionContext) {
  return context.providerResponse!.metadata!;
}

function parsedConfig(context: AssertionValueFunctionContext) {
  const { config } = metadata(context);
  assert.ok(typeof config === "string", `${configFile} must exist`);
  return parse(config);
}

function skillActivation(shouldActivate: boolean) {
  return check(context => {
    const meta = metadata(context);
    const expected = meta.variant.name !== "none" && shouldActivate;
    const activated = meta.activatedSkills.includes("opencode-sandbox");
    return { pass: activated === expected, score: Number(activated === expected),
      reason: `skill activation: expected ${expected}, observed ${activated}` };
  });
}

export function activatesSkillWhenInstalled() {
  return skillActivation(true);
}

export function doesNotActivateSkill() {
  return skillActivation(false);
}

export function hasOnlyWorkspaceConfig() {
  return check(context => {
    assert.deepEqual(metadata(context).files, [configFile]);
    return true;
  });
}

export function hasMemoryMiB(expected: number) {
  return check(context => {
    const vm = parsedConfig(context).vm as Record<string, unknown> | undefined;
    assert.equal(vm?.memory_mib, expected, `vm.memory_mib must be ${expected}`);
    return true;
  });
}

export function hasCpuCount(expected: number) {
  return check(context => {
    const vm = parsedConfig(context).vm as Record<string, unknown> | undefined;
    assert.equal(vm?.cpus, expected, `vm.cpus must be ${expected}`);
    return true;
  });
}

export function allowsNetworkHost(host: string) {
  return check(context => {
    const network = parsedConfig(context).network as Record<string, unknown> | undefined;
    const hosts = network?.allowed_hosts;
    assert.ok(Array.isArray(hosts) && hosts.includes(host), `network.allowed_hosts must include ${host}`);
    return true;
  });
}

export function leavesWorkspaceConfigUnchanged() {
  return check(context => {
    const fixture = context.vars.fixture;
    assert.ok(fixture && typeof fixture === "object" && configFile in fixture, `fixture must provide ${configFile}`);
    const before = fixture[configFile];
    assert.equal(typeof before, "string", `fixture must provide ${configFile}`);
    assert.equal(metadata(context).config, before, `${configFile} must be unchanged`);
    return true;
  });
}

export function leavesInheritedConfigsUnchanged() {
  return check(context => {
    const meta = metadata(context);
    for (const layer of ["parentConfig", "globalConfig"] as const) {
      const before = context.vars[layer];
      assert.equal(typeof before, "string", `fixture must provide ${layer}`);
      assert.equal(meta[layer], before, `${layer} must be unchanged`);
    }
    return true;
  });
}

export function validConfiguration(_output: string, context: AssertionValueFunctionContext) {
  const meta = metadata(context);
  assert.equal(meta.validation.valid, true, meta.validation.output);
  assert.ok(meta.config, "config must exist");
  return { pass: true, score: 1, reason: "real CLI validation passed and config exists" };
}
