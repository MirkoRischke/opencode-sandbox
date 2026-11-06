# Limitations

- **Incomplete network diagnostics after snapshot restore:** with Microsandbox
  v0.7.8, restored VMs lose the HTTP denial responder required to log blocked
  HTTPS destinations when requests bypass DNS. Network blocking and DNS-denial
  diagnostics still work, but those HTTPS-only denials are missing from
  `diagnostics network`. Denials collected during snapshot
   preparation remain in the report. Normal workspace VMs always restore a
   baseline, so this limitation applies to all newly created workspace VMs.

See [Microsandbox limitations and bugs](microsandbox.md) for upstream evidence
and checks for future upgrades.
