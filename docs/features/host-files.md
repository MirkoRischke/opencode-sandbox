# Host file mappings

Reuse host settings and skills, or let a workspace write to a specific host
directory outside the checkout. Use `[[files]]` to share selected paths. Beyond the
[workspace and required Git metadata](workspaces.md#choose-a-workspace), the
launcher does not share host agent configuration, instructions, or skills
implicitly.

1. [Choose a mapping mode](#choose-a-mapping-mode)
2. [Change or remove mappings](#change-or-remove-mappings)
3. [Share OpenCode configuration](#share-opencode-configuration)

## Choose a mapping mode

| Mode | Effect of guest edits | When host changes become visible |
| --- | --- | --- |
| `mount-ro` (default) | Blocked, even as guest root | Live |
| `mount-rw` | Writes and deletions affect the host directory immediately | Live |
| `copy` | Change only the guest copy | Read again on creation and snapshot restore, not on ordinary resume. |

For live mounts, applications may still need to reload changed files.

For example, mount OpenCode instructions and skills, and seed an editable
`.gitconfig`. Add entries to global `config.toml` or an approved
`.opencode-sandbox.toml`:

```toml
[[files]]
source = "~/.config/opencode/AGENTS.md"
target = "/root/.config/opencode/AGENTS.md"
mode = "mount-ro"

[[files]]
source = "~/.config/opencode/skills"
target = "/root/.config/opencode/skills"

[[files]]
source = "~/.config/opencode-sandbox/gitconfig"
target = "/root/.gitconfig"
mode = "copy"
```

To write directly to an existing host directory, add `mount-rw` to the
workspace's own `.opencode-sandbox.toml`. This mode is only available there
and requires [config approval](../config/README.md#workspace-overrides-and-trust).
Choose a directory whose files the guest may overwrite or delete:

```toml
[[files]]
source = "../shared-output"
target = "/mnt/shared-output"
mode = "mount-rw"
```

- Use an existing file or directory as `source`. Relative paths start at the
  config file's directory. `mount-rw` requires a directory. Use an absolute
  guest path other than `/` as `target`.
- Share only what the guest needs. A read-only directory still exposes all its
  contents.
- Copies overwrite matching files but keep files that exist only in the guest.
  Use [persistent workspace state](persistent-state.md) if you want guest edits
  to survive VM removal.

## Change or remove mappings

- File mappings accumulate across config files. Remove a mapping from the
  config that defines it.
- Recreate the VM to apply changes, including mode changes. Rebuild the
  [snapshot](snapshots.md) if you change or remove a mount target used by the
  baseline.
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
