# Sandbox instructions

The launcher writes sandbox-related instructions as Markdown at this stable
path inside the VM:

```text
/etc/opencode-sandbox/agent-sandbox-instructions.md
```

The file describes the workspace, effective network restrictions, and read-only
host mounts. The launcher writes it when creating or restoring a VM, before
starting the agent. Resuming an existing VM does not regenerate it. The wording
and headings may change.

The launcher does not configure your agent to load the file. For OpenCode, add
the guest path to your own `opencode.json`:

```json
{
  "instructions": ["/etc/opencode-sandbox/agent-sandbox-instructions.md"]
}
```

If you already have an `instructions` list, append the path. See
[file mappings](../config/README.md#read-only-host-mounts) to bring your config
into the VM. You don't need to mount the instruction file itself.

Loading the file is optional. It explains the restrictions; the VM enforces
them whether or not the agent loads it.
