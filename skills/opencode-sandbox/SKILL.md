---
name: opencode-sandbox
description: Use when asked what opencode-sandbox can do, how to configure it, or how to troubleshoot it.
---

# opencode-sandbox

For questions about what the sandbox can do or how to use it, start at the
[feature index](references/features/README.md) and read the relevant guide.
For questions about effective settings, use `opencode-sandbox config show` in the
workspace and consult the [configuration guide](references/config/README.md).
Keep explanation-only requests read-only.

## Edit workspace configuration

1. Read the existing `.opencode-sandbox.toml`, if present.
2. Consult `opencode-sandbox config default` for supported fields and defaults.
3. Edit only `.opencode-sandbox.toml`, preserving existing settings.
   For network access, add hosts to `[network] allowed_hosts`;
   inherited entries stay in effect.
4. Run `opencode-sandbox config validate` from the workspace and correct errors.
5. Briefly explain the actual change and validation result.

Keep global configuration unchanged. Config editing needs no VM lifecycle commands.
