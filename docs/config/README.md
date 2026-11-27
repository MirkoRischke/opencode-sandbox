# Launcher configuration

Set global options in `~/.config/opencode-sandbox/config.toml` (or under
`$XDG_CONFIG_HOME` if set). Add `.opencode-sandbox.toml` in a workspace or a
parent directory for settings shared by its descendants. This is separate from OpenCode's
`opencode.json` and `cli.json`.

## Inspect and validate

See [example.toml](example.toml) for a sample or print the current launcher
defaults with descriptions:

```sh
opencode-sandbox config default
opencode-sandbox config validate
opencode-sandbox config show
```

`config default` prints built-in values, not your effective settings. It
doesn't load user config or trust state. If you use its output as a workspace
config, remove settings you want to inherit rather than re-declaring defaults.

Run `config validate` from the target directory to check the merged settings
without approving them, reading credential values, or launching a VM. It
rejects malformed TOML, unknown keys, and invalid values.

Run `config show` from the target directory to print the validated, merged
launcher settings as TOML on stdout, with resolved paths. It then lists the
built-in defaults and the config files used, in merge order, on stderr.
Like `config validate`, it does not check or change trust, read credentials, or launch a VM.
It does not include launch-time CLI overrides.

Treat the output as a preview, not a replacement for a config layer. Saving
it as `.opencode-sandbox.toml` can duplicate inherited list entries on the
next merge.

## Workspace overrides and trust

The launcher loads built-in defaults and global config, then layers every
`.opencode-sandbox.toml` from the filesystem root down to the current directory
after resolving symlinks. This does not change which directory is mounted as
the writable workspace.

For example, put shared mounts or secret definitions in
`~/workspace/my-org/.opencode-sandbox.toml`. A repo under `my-org` inherits those
settings and can add its own in the repo's `.opencode-sandbox.toml`.

Scalars and `startup.command` use the closest value specified. Lists of files,
state, network hosts, and port mappings accumulate; empty lists add nothing
to previously configured entries. The VM-to-host port `[4000]` is a fallback
used only when no file sets `network.vm_to_host.ports`. Duplicate port mappings
and overlapping mount targets are errors. Each named `[secrets.<name>]` entry
replaces the entire inherited definition and must provide all required fields.
Relative paths resolve against the directory of the config that sets them.
For example, this workspace config changes only memory:

```toml
[vm]
memory_mib = 8192
```

Review each new or changed config before approving it at launch, including
the global file. A parent config applies to every workspace below it.
Approval defaults to no and authorizes all settings, including host files
and credential destinations. An unapproved config fails in non-interactive
launches. Invalid config fails before approval; an absent workspace config
needs no approval.

Approvals are stored in
`${XDG_STATE_HOME:-~/.local/state}/opencode-sandbox/trust.json`.
Each approval binds the config's location to a SHA-256 hash of the
exact config bytes. Even a comment change or a moved/copied checkout requires
approval again; symlink aliases of a directory share approval. Delete the config's entry
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
| `secrets`, `github`, `aws` | [Proxied secrets](../features/secrets.md), [GitHub access](../features/github.md), and [AWS roles](../features/aws.md) |
| `logging` | [Launcher logs](../features/diagnostics.md#launcher-logs) |

See the [feature index](../features/README.md) for the full list.
