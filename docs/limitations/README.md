# Limitations

- **Best-effort network diagnostics:** restored VMs can report denied HTTPS
  hostnames even when requests bypass DNS, but not every denial produces a log
  record or includes a hostname. See [network diagnostics](../features/diagnostics.md#network-denials).
- **No nested virtualization in workspace VMs:** disk snapshot restore loses
  nested virtualization, so the VM test suite and full `mise run verify` must
  run outside the sandbox. See [MSB-14](microsandbox.md#msb-14--nested-virtualization-is-lost-on-disk-snapshot-restore).

See [Microsandbox limitations and bugs](microsandbox.md) for upstream evidence
and checks for future upgrades.
