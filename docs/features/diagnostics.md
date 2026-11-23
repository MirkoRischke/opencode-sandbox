# Diagnostics and logs

Use diagnostics to find out why a launch failed or which network destinations
the VM tried to reach. You can inspect saved results with the VM stopped.

1. [Find launcher logs](#launcher-logs)
2. [Find blocked destinations](#network-denials)

## Launcher logs

From the workspace directory, run `opencode-sandbox diagnostics logs` to find
its host log directory. The JSON logs include debug details even when the
terminal shows only progress and errors.

Pass `--console-log-level debug` for more detail on one run, or set a
different default in your config:

```toml
[logging]
console_level = "warn" # debug, info, warn, or error
```

The default is `info`. Changing it doesn't affect saved logs. The launcher
generally keeps up to 20 log files per workspace for up to 30 days.

## Network denials

See which hosts the VM tried to reach when a tool or download fails. Enable
collection in your launcher config, then recreate the VM:

```toml
[network]
diagnostics = true
```

Reproduce the failed request. From the same workspace, print the saved summary:

```sh
opencode-sandbox diagnostics network
```

- Look for the failed request under `denied_targets`. Check `last_seen` to
  distinguish it from old denials before changing your allowlist. The report
  is [best effort](../limitations/README.md), not a complete audit.
- History survives VM removal and doesn't expire. To reset it, delete the
  `network.json` file shown at startup while the launcher is stopped.
- This also enables more runtime logging. Recreate the VM after disabling
  the setting to turn that logging off.
