# Host file mappings

Reuse your host settings and skills without sharing your whole home directory.
Use `[[files]]` to share selected files or directories. Beyond the
[workspace and required Git metadata](workspaces.md#choose-a-workspace), the
launcher does not share host agent configuration, instructions, or skills
implicitly.

1. [Mount read-only or copy](#mount-read-only-or-copy)
2. [Change or remove mappings](#change-or-remove-mappings)
3. [Share OpenCode configuration](#share-opencode-configuration)

## Mount read-only or copy

| Mode | Guest can edit? | When host changes become visible |
| --- | --- | --- |
| `mount` (default) | No, even as guest root | Host edits are live; application reload behavior still applies. |
| `copy` | Yes, without changing the host source | Read again on creation and snapshot restore, not on ordinary resume. |

For example, mount OpenCode instructions and skills, and seed an editable
`.gitconfig`. Add entries to global `config.toml` or an approved
`.opencode-sandbox.toml`:

```toml
[[files]]
source = "~/.config/opencode/AGENTS.md"
target = "/root/.config/opencode/AGENTS.md"

[[files]]
source = "~/.config/opencode/skills"
target = "/root/.config/opencode/skills"

[[files]]
source = "~/.config/opencode-sandbox/gitconfig"
target = "/root/.gitconfig"
mode = "copy"
```

- Use an existing file or directory as `source`. Relative paths start at the
  config file's directory. Use an absolute guest path other than `/` as `target`.
- Share only what the guest needs. A read-only directory still exposes all its
  contents.
- Copies overwrite matching files but keep files that exist only in the guest.
  Use [persistent workspace state](persistent-state.md) if you want guest edits
  to survive VM removal.

## Change or remove mappings

- A workspace list replaces the global list. Set `files = []` at the top level
  to clear it.
- Recreate the VM to apply changes. Rebuild the [snapshot](snapshots.md) if
  you change or remove a mount target used by the baseline.
- Mounted contents are not captured in snapshots. Copied files are. Removing
  a copy entry does not erase a copy already saved in the baseline.

## Share OpenCode configuration

For `opencode.json`, use `/root/.config/opencode/opencode.json` for user
configuration or `/etc/opencode/opencode.json` for system configuration that
overrides project settings. For `cli.json`, use `/root/.config/opencode/cli.json`.
The settings UI cannot save to a read-only mount.

See [gateway setup](llm-gateway.md#configure-opencode) for a complete provider
example and [sandbox instructions](sandbox-instructions.md) for the generated
instructions file.
