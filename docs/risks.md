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
- **Data exfiltration:** the agent can send data to allowed destinations. Snapshot
  preparation runs project setup with public internet access regardless of the
  workspace allowlist, so only prepare snapshots from projects you trust.
- **Persistent OpenCode history:** the guest can read, change, or delete its
  workspace's host-backed OpenCode database. Linked worktrees have separate
  databases. See [OpenCode history](features/opencode-history.md).
- **Credential use:** guest code can read and use temporary AWS role credentials.
  I only use roles with limited permissions that I am willing to delegate to
  an agent. Subscription credentials stay in the [host LLM gateway](../llm-gateway/README.md).
  The guest receives only a placeholder for its gateway key, but can still make
  requests through the gateway and consume provider usage.
