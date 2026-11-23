---
name: opencode-sandbox
description: Create or edit workspace-local opencode-sandbox TOML configuration, including VM resources and network access. Use when asked to configure the sandbox; unrelated programming questions do not need this skill.
---

# Workspace sandbox configuration

1. Read the existing `.opencode-sandbox.toml`, if present.
2. Consult `opencode-sandbox config default` for supported fields and defaults.
3. Edit only `.opencode-sandbox.toml`, preserving existing settings.
   For network access, set `[network] allowed_hosts`;
   it replaces the inherited list.
4. Run `opencode-sandbox config validate` from the workspace and correct errors.
5. Briefly explain the actual change and validation result.

Keep global configuration unchanged. Config editing needs no VM lifecycle commands.
