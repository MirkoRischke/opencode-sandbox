# Launcher configuration

Set global options in `~/.config/opencode-sandbox/config.toml` (or under
`$XDG_CONFIG_HOME` if set). Add `.opencode-sandbox.toml` in your workspace
to override only what that directory needs. This is separate from OpenCode's
`opencode.json` and `cli.json`.

## Get defaults and validate

See [example.toml](example.toml) for a sample or print the current launcher
defaults with descriptions:

```sh
opencode-sandbox config default
opencode-sandbox config validate
```

`config default` prints built-in values, not your effective settings. It
doesn't load user config or trust state. If you use its output as a workspace
config, remove settings you want to inherit rather than replacing global
values with the defaults.

Run `config validate` from the target directory to check the merged settings
without approving them, reading credential values, or launching a VM. It
rejects malformed TOML, unknown keys, and invalid values.

## Workspace overrides and trust

The launcher loads built-in defaults, global config, then the config in the
exact current directory after resolving symlinks. It doesn't load config from
parent directories, even inside a Git checkout.

Tables merge recursively; specified scalars and arrays replace inherited
values. Empty arrays clear inherited arrays, while empty tables do not
clear inherited entries. Each named `[secrets.<name>]` definition is an
exception: a workspace entry replaces the entire global definition and must
provide all required fields. For example, this workspace config changes only
memory and inherits the other VM settings:

```toml
[vm]
memory_mib = 8192
```

Review a new or changed workspace config before approving it at launch.
Approval defaults to no and authorizes all settings, including host files
and credential destinations. An unapproved config fails in non-interactive
launches. Invalid config fails before approval; an absent workspace config
needs no approval.

Approvals are stored in
`${XDG_STATE_HOME:-~/.local/state}/opencode-sandbox/trust.json`.
Each approval binds the canonical workspace path to a SHA-256 hash of the
exact config bytes. Even a comment change or a moved/copied checkout requires
approval again; symlink aliases share approval. Delete the workspace's entry
or the trust file to require approval again. Malformed trust storage prevents
launch. The trust file is private and updated atomically. The launcher approves
the bytes it loaded before prompting and checks approval before reading
credentials or starting even an existing VM.

## Configure a feature

| Settings | Guide |
| --- | --- |
| `vm` | [VM resources](../features/workspaces.md#set-vm-resources), [custom images](../features/vm-images.md), and [guest Docker](../features/guest-docker.md) |
| `startup` | [Coding agents and startup command](../features/coding-agents.md) |
| `snapshot` | [Prepared workspace snapshots](../features/snapshots.md) |
| `files`, `state` | [Host file mappings](../features/host-files.md) and [persistent workspace state](../features/persistent-state.md) |
| `network` | [Internet policy and port access](../features/networking.md), [network diagnostics](../features/diagnostics.md#network-denials) |
| `secrets`, `aws` | [Proxied secrets](../features/secrets.md), [GitHub access](../features/github.md), and [AWS roles](../features/aws.md) |
| `logging` | [Launcher logs](../features/diagnostics.md#launcher-logs) |

See the [feature index](../features/README.md) for the full list.
