# Limitations

- **Incomplete network diagnostics after snapshot restore:** with Microsandbox
  v0.7.8, restored VMs lose the HTTP denial responder required to log blocked
  HTTPS destinations when requests bypass DNS. Network blocking and DNS-denial
  diagnostics still work, but those HTTPS-only denials are missing from
  `diagnostics network`. Denials collected during snapshot
   preparation remain in the report. Normal workspace VMs always restore a
   baseline, so this limitation applies to all newly created workspace VMs.
- **OpenAI-only automatic authentication:** the launcher imports an active host
  OpenAI subscription OAuth credential. Other providers aren't explicitly
  blocked, but their authentication isn't set up automatically; a fresh VM
  currently requires the OpenAI import to succeed.
