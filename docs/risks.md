# Accepted risks

This tool reduces risk; it does not eliminate it. I accept the following risks for my own workflow.

- **Repository access:** the agent can read and modify files in the mounted repository, including sensitive content stored there.
- **Host-side execution:** the workspace and required Git metadata are writable
  from the guest. The agent can write harmful code, scripts, or configuration that
  host tools later execute—for example, mise or direnv configuration, Git
  hooks/configuration, and editor or language-server configuration. Linked
  worktrees may share affected Git metadata with other checkouts. The VM does not
  protect against execution triggered by host tools consuming these changes.
  Review changes before trusting or running them; automatic host tooling may
  execute them before review.
- **Data exfiltration:** network restrictions do not guarantee that repository contents cannot be leaked. The agent might abuse allowed domains to exfiltrate data.
- **Shared OpenCode database (experimental):** linked-worktree VMs can read and
  modify the same host-backed OpenCode database, including database-resident state
  beyond session transcripts. Concurrent writers can corrupt or lose history;
  Microsandbox does not coordinate file locks across VMs. This risk is explicitly
  accepted for experimentation, not a claim of supported concurrency. See
  [OpenCode history](features/opencode-history.md).
- **Guest-readable credentials:** unlike custom proxy secrets, AWS role
  credentials and imported OpenAI credentials can be read and used by guest code.
  - AWS role credentials are temporary. I only use roles with limited permissions
    that I am willing to delegate to an agent.
  - Imported OpenAI credentials are stored inside the VM. A host-side process
    handling LLM provider authentication would be preferable, but this tool does
    not provide one.
