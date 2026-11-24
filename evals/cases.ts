import type { TestCase } from "promptfoo";
import {
  activatesSkillWhenInstalled, allowsNetworkHost, doesNotActivateSkill,
  hasMemoryMiB, hasOnlyWorkspaceConfig, leavesWorkspaceConfigUnchanged,
} from "./assertions.ts";

export const cases: TestCase[] = [
  {
    description: "Create workspace config",
    vars: {
      caseID: "create",
      fixture: {},
      task: "I want the sandbox to use 1234MB of RAM",
    },
    assert: [
      activatesSkillWhenInstalled(),
      hasOnlyWorkspaceConfig(),
      hasMemoryMiB(1234),
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
      activatesSkillWhenInstalled(),
      hasOnlyWorkspaceConfig(),
      hasMemoryMiB(8192),
      allowsNetworkHost("registry.npmjs.org"),
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
      doesNotActivateSkill(),
      hasOnlyWorkspaceConfig(),
      leavesWorkspaceConfigUnchanged(),
    ],
  },
];
