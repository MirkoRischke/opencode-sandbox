# Microsandbox limitations and bugs

Tracker for upstream limitations encountered by this project, not an exhaustive
inventory of Microsandbox features. Use it when evaluating an SDK/runtime upgrade.

Baseline: **Go SDK and runtime v0.7.8**. Inventory recorded 2026-10-28 from
versioned source, existing experiments, tests, and the automatic-snapshot review.
Tests were not rerun when writing this document. No later release has been checked.

Entries distinguish confirmed API gaps, observed behavior, and unverified issues.
Documenting a security limitation does **not** make it safe or imply acceptance.
This tracker does not prescribe workarounds. Existing implementation adaptations
are described only to identify their impact and eventual removal criteria.

## MSB-01 — Go secret modification loses selected-header restrictions

**Status:** confirmed Go API gap; security impact. Last checked: v0.7.8.

Creation supports selected-header substitution, but `SecretModifySpec` has no
`HeaderFields` equivalent. Adding credentials after restore therefore permits
substitution in any header on the allowed hosts, instead of only configured
headers. An allowed endpoint could log or reflect a credential placed in an
arbitrary header. Host scoping remains, but the configured header contract is lost.

Automatic snapshots make this affect ordinary first launches as well as explicit
snapshot restores. The three selected-header failures reported during implementation
are behavioral regressions, not assertions to relax merely to make tests pass.

**Project decision (2026-10-28):** explicitly accept this limitation for automatic
and manual snapshot restores pending an upstream Microsandbox fix. Host restrictions
remain enforced; selected-header restrictions are not preserved after restore.
Keep the selected-header tests as evidence of the limitation and recheck them on
SDK/runtime upgrades; no local workaround is planned.

Rechecked 2026-10-28 with v0.7.8: reproduced all three selected-header failures.
Only those assertions are now isolated in explicitly skipped `selected_headers`
subtests; supported substitution, TLS, and host-scoping assertions still run.

- Evidence: [Go secret modification API](https://github.com/superradcompany/microsandbox/blob/v0.7.8/sdk/go/modify.go#L85-L106),
  [restore experiment](../research/repo-snapshot-experiment.md#restore-enrichment-tls-and-credentials).
- Project references: `internal/sandbox/snapshot.go` (`restoredWorktreePatch`),
  `tests/vm/secrets_test.go`.
- Upgrade check: configure Authorization-only substitution, restore, and verify
  Authorization is substituted while an arbitrary header retains its placeholder.
  Both creation and modification must preserve the configured restriction.

## MSB-02 — Restore cannot configure the full destination before boot

**Status:** confirmed Go restore API gap. Last checked: v0.7.8.

`RestoreConfig` accepts resource, mount, port, and host-policy overrides, but not
create-time secrets, labels, working directory, arbitrary environment, TLS settings,
or upstream proxy settings. Supplying creation options when connecting to the
already restored VM does not update it. The current launcher consequently performs
an additional stop/configure/start cycle before executing project commands.
Adding the first secret also requires a restart.

- Evidence: [restore API](https://github.com/superradcompany/microsandbox/blob/v0.7.8/sdk/go/restore.go#L12-L54),
  [experiment and restart observations](../research/repo-snapshot-experiment.md#restore-enrichment-tls-and-credentials).
- Project reference: `internal/sandbox/snapshot.go` (`configureRestoredWorktree`).
- Upgrade check: restore with destination configuration applied before the first
  boot, without a second boot or temporary bootstrap secret.

## MSB-03 — Restore cannot directly enable/configure TLS inspection

**Status:** confirmed API gap; custom-setting loss observed experimentally.
Last checked: v0.7.8.

Restore does not expose TLS interception configuration. Subsequent secret addition
can enable default interception, but does not reproduce custom interception ports
or custom upstream trust settings. The launcher currently uses a temporary
non-credential secret to activate inspection when restoring without real secrets.
This is an existing adaptation, not a supported direct TLS restore option.

- Evidence: [restore API](https://github.com/superradcompany/microsandbox/blob/v0.7.8/sdk/go/restore.go#L12-L54),
  [custom-port and CA experiment](../research/repo-snapshot-experiment.md#restore-enrichment-tls-and-credentials).
- Upgrade check: secret-free restore with explicit verified TLS interception;
  separately verify custom ports and upstream trust. Proxy migration was only
  source-reviewed in the experiment, not exercised.

## MSB-04 — Restores cannot enable the HTTP denial responder

**Status:** confirmed Go restore validation restriction. Last checked: v0.7.8.

Restore accepts only network rules and default ingress/egress actions, not the
`HTTP` configuration used for denial responses. The launcher must omit it on
restore. Denied connections can reset without hostname evidence, reducing network
diagnostic coverage; this does not itself remove destination enforcement.

- Evidence: [restore validation](https://github.com/superradcompany/microsandbox/blob/v0.7.8/sdk/go/restore.go),
  `internal/sandbox/snapshot.go` (`restoreWorktree`),
  `tests/vm/network_diagnostics_test.go`.
- Upgrade check: restore with the responder enabled; denied HTTPS produces the
  expected local denial response and diagnostic without contacting the upstream.

## MSB-05 — Go Modify cannot change network policy

**Status:** confirmed Go API gap. Last checked: v0.7.8.

`ModifyOptions` has no network-policy field. The API used by this project cannot
switch a running preparation VM from broad access to restricted access. This is
an SDK-surface finding, not proof that every runtime/backend lacks the capability.

- Evidence: [Go modification options](https://github.com/superradcompany/microsandbox/blob/v0.7.8/sdk/go/modify.go#L36-L83).
- Upgrade check: inspect the public API and verify policy persistence, failure
  semantics, and treatment of already-established connections before relying on
  any new transition capability.

## MSB-06 — Previous interception CA remains in restored system trust

**Status:** observed experimentally. Last checked: v0.7.8.

The destination runtime installs its CA, but the preparation VM's CA can remain
in the captured system trust bundle. Installing the new CA is not revocation of
the old one. Nested containers have separate trust stores; their CA behavior was
not validated by that experiment.

- Evidence: [CA transition experiment and source links](../research/repo-snapshot-experiment.md#restore-enrichment-tls-and-credentials).
- Upgrade check: use distinct preparation/destination CAs; confirm the destination
  CA works and determine whether the preparation CA is intentionally retained or
  removed. Verify container trust separately if required.

## MSB-07 — Network denial logs are not a complete audit

**Status:** known runtime observability limitation. Last checked: v0.7.8.

Not every denial emits a diagnostic or exposes a hostname. Early default-deny TCP
resets can be silent. Runtime debug logging includes unrelated records, with no
network-only verbosity filter or pre-disk Go log interception used by this project.
The saved network overview therefore cannot prove that no denied attempts occurred.

- Evidence: [network diagnostics documentation](config.md#network-diagnostics),
  `internal/sandbox/diagnostics.go`, `tests/vm/network_diagnostics_test.go`.
- Upgrade check: exercise DNS, TCP, TLS, and protected-destination denials; inspect
  event completeness and any new structured or scoped logging API.

## MSB-08 — TLS inspection can prevent direct-IP HTTPS

**Status:** documented integration limitation; precise failure modes need rechecking.
Last recorded: v0.7.8.

Allowing public IP destinations does not guarantee direct-IP HTTPS works while
verified TLS inspection is enabled. Network admission and TLS compatibility are
separate concerns.

- Evidence: [unrestricted networking documentation](config.md#network-policy).
- Upgrade check: use a controlled endpoint with a valid IP-SAN certificate and
  compare direct-IP versus hostname HTTPS without disabling verification.

## MSB-09 — Snapshot publication and removal have group constraints

**Status:** observed native semantics, not necessarily bugs. Last checked: v0.7.8.

Capturing a fresh, independent preparation VM does not automatically advance an
existing group's HEAD. Capture and explicit HEAD selection are separate operations,
not one atomic publication. Deleting HEAD while other members remain is refused,
even with force; deletion ordering matters. Experiments also left empty group
metadata directories after removing every member, with no public group-delete
command found.

- Evidence: [group/HEAD experiment](../research/repo-snapshot-experiment.md#native-grouphead-semantics-matter-for-fresh-only-preparation),
  [cleanup observations](../research/repo-snapshot-experiment.md#limits-cleanup-and-leftovers).
- Upgrade check: independent fresh captures, interrupted publication, member
  deletion ordering, and complete group cleanup through public APIs.

## Issues needing verification, not confirmed upstream bugs

### Restored root-disk metadata reports `SizeMiB = 0`

The automatic-snapshot implementation reported a failing metadata assertion in
`tests/vm/vm_settings_test.go`. Source review indicates restored managed disks
deliberately use unspecified size while retaining captured virtual capacity
([restore builder](https://github.com/superradcompany/microsandbox/blob/v0.7.8/sdk/rust/lib/sandbox/builder.rs#L2178-L2179)).
Rechecked 2026-10-28 with v0.7.8: the configured 5120 MiB writable root device
retains its capacity after restore and stop/resume; the root filesystem spans
more than 90% of that capacity. The test now measures those instead of requiring
nonzero size metadata. This is not evidence of lost capacity. Recheck metadata
and actual capacity on an upgrade.

### Native image-cache lookup failures in DNS fixtures

Five DNS fixture failures were reported during implementation. Review identified
fixtures passing an original Docker tag directly to native creation with
`PullPolicyNever`, bypassing this project's identity-addressed import. Rechecked
2026-10-28 with v0.7.8: all five tests pass using the source VM's already imported
image reference, with DNS-policy assertions unchanged. The fixture has been
corrected; no upstream cache or DNS-enforcement bug is established.

References: `tests/vm/dns_test.go`, `internal/sandbox/image.go`.

## Upgrade review checklist

- Record the candidate SDK and runtime versions separately.
- Recheck each entry against versioned source and its reproduction/test.
- Keep security assertions intact; do not redefine success around missing APIs.
- Record results per entry, including version and evidence; retain resolved history.
- Only mark fixed after verifying behavior, not merely finding a new API field.
- No upstream issue numbers are recorded here yet; add verified issue links when available.
