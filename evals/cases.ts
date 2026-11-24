import type { TestCase } from "promptfoo";
import {
  activatesSkillWhenInstalled, allowsNetworkHost, doesNotActivateSkill,
  hasCpuCount, hasMemoryMiB, hasOnlyWorkspaceConfig, leavesInheritedConfigsUnchanged,
  leavesWorkspaceConfigUnchanged,
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
    description: "Repair an invalid workspace config",
    vars: {
      caseID: "repair",
      fixture: { ".opencode-sandbox.toml": "[vm]\ncores = 2\nmemory_mib = 4096\n" },
      task: "The sandbox does not start because of an invalid config, fix it",
    },
    assert: [
      activatesSkillWhenInstalled(),
      hasOnlyWorkspaceConfig(),
      hasCpuCount(2),
      hasMemoryMiB(4096),
    ],
  },
  {
    description: "Explain effective settings across configuration layers",
    vars: {
      caseID: "layers",
      globalConfig: '[vm]\nmemory_mib = 8192\ncpus = 2\n[network]\nallowed_hosts = ["global.example.com"]\n',
      parentConfig: '[vm]\ncpus = 4\n[network]\nallowed_hosts = ["team.example.com"]\n',
      fixture: { ".opencode-sandbox.toml": '[vm]\nmemory_mib = 4096\n[network]\nallowed_hosts = ["project.example.com"]\n' },
      task: "Before I start the sandbox, tell me how much RAM and how many CPUs it will use, which hosts are on its allowlist, and where each setting comes from. If I remove the memory setting from this project's config, how much RAM would it use? Don't change anything.",
    },
    assert: [
      hasOnlyWorkspaceConfig(),
      leavesWorkspaceConfigUnchanged(),
      leavesInheritedConfigsUnchanged(),
      {
        type: "llm-rubric",
        value: "Grade the answer, not the config field. It must identify 4096 MiB (4 GiB) from the current directory and 4 CPUs from the parent directory. The allowlist must contain global.example.com from the global config, team.example.com from the parent, and project.example.com from the current directory, with each source correctly attributed. Removing the local memory setting would fall back to 8192 MiB (8 GiB) from the global config. Accept equivalent wording. Fail on missing or contradictory information.",
      },
    ],
  },
  {
    description: "Recognize that mounted workspace storage cannot be capped by the sandbox",
    vars: {
      caseID: "workspace-quota",
      fixture: { ".opencode-sandbox.toml": "[vm]\nmemory_mib = 4096\nroot_disk_mib = 4096\n" },
      task: "Can you cap this project's folder at 2 GiB so the sandbox can't fill up my host disk? If there's a sandbox setting for that, configure it.",
    },
    assert: [
      hasOnlyWorkspaceConfig(),
      leavesWorkspaceConfigUnchanged(),
      {
        type: "llm-rubric",
        value: "Pass if the answer clearly states that opencode-sandbox cannot cap the host-mounted project folder. Fail if it invents a sandbox quota setting, presents guest disk settings as a way to cap the project folder, or claims the cap was applied. Do not require naming specific disk settings or suggesting alternatives.",
      },
    ],
  },
  {
    description: "Reuse Docker images through snapshot preparation",
    vars: {
      caseID: "snapshot-images",
      fixture: { ".opencode-sandbox.toml": '[network]\nallowed_hosts = ["registry.npmjs.org"]\n' },
      task: "I keep downloading the same Docker images whenever I start a fresh sandbox. Is there a way to download them once and reuse them? Which domains do I need to add to the network allowlist? Just explain; don't change anything.",
    },
    assert: [
      hasOnlyWorkspaceConfig(),
      leavesWorkspaceConfigUnchanged(),
      {
        type: "llm-rubric",
        value: "Pass only if the answer recommends pulling the Docker images in a custom setup script configured via [snapshot] command, explains that new VMs created from the prepared baseline reuse the images, and states that no registry domains need to be added for those preparation pulls because the workspace public internet allowlist does not apply during snapshot preparation. The normal policy still applies to pulls in a regular workspace VM. Fail if it claims the work was performed or that preparation bypasses restrictions on access to host services. Do not require a full inventory of snapshot contents.",
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
