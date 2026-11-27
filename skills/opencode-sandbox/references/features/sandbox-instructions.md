# Sandbox instructions

Give your agent a short guide to its workspace, network limits, read-only
files, and [GitHub App access](github.md). It can choose commands that work in
the VM instead of guessing what it can access.

For OpenCode, add the generated guest file to `instructions` in `opencode.json`:

```json
{
  "instructions": ["/etc/opencode-sandbox/agent-sandbox-instructions.md"]
}
```

If the list already exists, append the path. Use
[file mappings](host-files.md#share-opencode-configuration) to bring your config
into the VM. The instructions file is already there on new VMs.

Loading it is optional. The VM enforces its restrictions either way.
