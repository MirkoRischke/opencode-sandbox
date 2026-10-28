# Microsandbox limitations and bugs

Tracker for upstream limitations encountered by this project, not an exhaustive
inventory of Microsandbox features. Use it when evaluating an SDK/runtime upgrade.

Baseline: **Go SDK and runtime v0.7.8**. Inventory recorded 2026-10-28 from
versioned source, existing experiments, tests, and the automatic-snapshot review.
Tests were not rerun when writing the original inventory. Source rechecked through
**Go SDK/runtime v0.7.11** on 2026-10-28; see the
[upgrade research](../research/microsandbox-v0.7.11-upgrade.md). Source rechecks below
are not behavioral verification. Separately attributed parent-session test results
do not imply every upgrade check was exercised.

Entries distinguish confirmed API gaps, observed behavior, and unverified issues.
Documenting a security limitation does **not** make it safe or imply acceptance.
This tracker does not prescribe workarounds. Existing implementation adaptations
are described only to identify their impact and eventual removal criteria.

## MSB-01 — Go secret modification loses selected-header restrictions

**Status:** resolved for this tool with Go SDK/runtime v0.7.11. Original Go API gap
addressed upstream in v0.7.10; restore now preserves the substitution policy.
Last source and reproduced behavior check: v0.7.11 (2026-10-28).

In v0.7.8, creation supports selected-header substitution, but `SecretModifySpec` has no
`HeaderFields` equivalent. Adding credentials after restore therefore permits
substitution in any header on the allowed hosts, instead of only configured
headers. An allowed endpoint could log or reflect a credential placed in an
arbitrary header. Host scoping remains, but the configured header contract is lost.

Automatic snapshots make this affect ordinary first launches as well as explicit
snapshot restores. The three selected-header failures reported during implementation
are behavioral regressions, not assertions to relax merely to make tests pass.

**Historical project decision (2026-10-28; superseded by the fix below):** explicitly accept this limitation for automatic
and manual snapshot restores pending an upstream Microsandbox fix. Host restrictions
remain enforced; selected-header restrictions are not preserved after restore.
Keep the selected-header tests as evidence of the limitation and recheck them on
SDK/runtime upgrades; no local workaround is planned.

Rechecked 2026-10-28 with v0.7.8: reproduced all three selected-header failures.
Only those assertions are now isolated in explicitly skipped `selected_headers`
subtests; supported substitution, TLS, and host-scoping assertions still run.

Source rechecked 2026-10-28 at v0.7.11: [PR #1880](https://github.com/superradcompany/microsandbox/pull/1880)
(released v0.7.10; fixes upstream issue #1835) adds
[`SecretModifySpec.Substitution`](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/go/modify.go#L99-L136),
including `SecretSubstitution.HeaderFields`, and rejects unsupported native libraries.
The tool's `restoredWorktreePatch` still omits this policy; upgrading alone therefore
does not restore header restrictions. The parent upgrade session temporarily removed
all three `selected_headers` skips and reran their targeted VM tests with v0.7.11:
all three failed (`header_fields` nil instead of `[authorization]`, and arbitrary
`X-Untrusted` headers received fake credentials instead of retaining placeholders).
Skips were restored with v0.7.11 recheck messages. The integrated limitation is
behaviorally confirmed, not evidence that the newly available policy API fails.
The historical acceptance above is not evidence that the new API was evaluated.

Follow-up verified 2026-10-28 with Go SDK/runtime v0.7.11: `restoredWorktreePatch`
now passes the original `Substitution` policy, placeholder passthrough hosts,
violation action, and TLS-identity requirement to `SecretModifySpec`. All three
`selected_headers` skips were removed. Targeted VM tests now pass: stored header
fields remain Authorization-only, authorized substitution works, and unrelated
`X-Untrusted` headers retain their placeholders. The unit test also checks policy
preservation. The accepted limitation above no longer applies.
Full `mise run verify` (lint and standard suite, including all three re-enabled
assertions) passed; the updated launcher was installed with `mise run install`.

- Evidence: [Go secret modification API](https://github.com/superradcompany/microsandbox/blob/v0.7.8/sdk/go/modify.go#L85-L106),
  [restore experiment](../research/repo-snapshot-experiment.md#restore-enrichment-tls-and-credentials).
- Project references: `internal/sandbox/snapshot.go` (`restoredWorktreePatch`),
  `tests/vm/secrets_test.go`.
- Upgrade check: configure Authorization-only substitution, restore, and verify
  Authorization is substituted while an arbitrary header retains its placeholder.
  Both creation and modification must preserve the configured restriction.

## MSB-02 — Restore cannot configure the full destination before boot

**Status:** confirmed Go restore API gap. Last checked: v0.7.8.

Source rechecked 2026-10-28 at v0.7.11:
[`RestoreConfig`](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/go/restore.go#L15-L57)
still lacks these destination settings. Live label modification added in v0.7.9
does not supply restore-time secrets/workdir/TLS or eliminate the enrichment cycle.

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

Source rechecked 2026-10-28 at v0.7.11:
[`RestoreConfig` and its validation](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/go/restore.go#L15-L57)
still expose no TLS configuration. The temporary-secret adaptation remains necessary
for the current Go integration; custom TLS ports/trust were not retested.

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

## MSB-04 — Restore HTTP overrides unavailable; default responder now enabled

**Status:** confirmed Go restore validation restriction. Last checked: v0.7.8.

Source rechecked 2026-10-28 at v0.7.11: explicit HTTP overrides are
[still rejected](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/go/restore.go#L156-L163),
but [PR #1825](https://github.com/superradcompany/microsandbox/pull/1825) changed the
runtime default in v0.7.9 to
[`deny_response: true` with JSON](https://github.com/superradcompany/microsandbox/blob/v0.7.11/packages/microsandbox-types/rust/lib/domain.rs#L1961-L1970).
Thus omission no longer means the responder is disabled: the reset description
below describes v0.7.8, not a universal v0.7.11 restore limitation.
The parent upgrade session reproduced the old test's `false`-versus-`true`
configuration failure twice with v0.7.11, then replaced it with
`TestSnapshotRestoreBlocksHTTPSWithDenialDiagnostics`: it asserts the enabled
responder and positive `example.org` hostname observations after blocked
`curl --resolve example.org:443:1.1.1.1 https://example.org` (bypassing DNS).
The updated test passed twice targeted and once in the full upgrade suite.
The original missing-responder behavior is resolved for this tested default path;
the explicit HTTP override restriction remains. This research session ran no tests.

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

Source rechecked 2026-10-28 at v0.7.11:
[`ModifyOptions`](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/go/modify.go#L36-L90)
adds published-port modification, not general network-policy modification.

`ModifyOptions` has no network-policy field. The API used by this project cannot
switch a running preparation VM from broad access to restricted access. This is
an SDK-surface finding, not proof that every runtime/backend lacks the capability.

- Evidence: [Go modification options](https://github.com/superradcompany/microsandbox/blob/v0.7.8/sdk/go/modify.go#L36-L83).
- Upgrade check: inspect the public API and verify policy persistence, failure
  semantics, and treatment of already-established connections before relying on
  any new transition capability.

## MSB-06 — Previous interception CA remains in restored system trust

**Status:** observed experimentally. Last checked: v0.7.8.

Source rechecked 2026-10-28 at v0.7.11: v0.7.9's atomic CA-bundle replacement
([PR #1856](https://github.com/superradcompany/microsandbox/pull/1856)) prevents partial
writes and duplicate copies of the same CA. It still
[appends to the existing bundle](https://github.com/superradcompany/microsandbox/blob/v0.7.11/crates/agentd/lib/tls.rs#L181-L213),
without removing a different captured CA. This is not a revocation fix; no new CA
transition or nested-container experiment was performed.

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

Source rechecked 2026-10-28 at v0.7.11: default JSON denial responses improve supported
HTTP/HTTPS diagnostics ([PR #1825](https://github.com/superradcompany/microsandbox/pull/1825)),
including restores that omit HTTP options. This does not establish complete logging:
[TLS denial handling](https://github.com/superradcompany/microsandbox/blob/v0.7.11/crates/network/lib/engine/tls/proxy.rs#L199-L223)
still has bypass/response-failure paths, and the tool still parses system debug logs.
No exhaustive denial matrix was run in this source review.

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

Source rechecked 2026-10-28 at v0.7.11: [PR #1825](https://github.com/superradcompany/microsandbox/pull/1825)
(v0.7.9) explicitly adds no-SNI direct-IP TLS support. The
[proxy](https://github.com/superradcompany/microsandbox/blob/v0.7.11/crates/network/lib/engine/tls/proxy.rs#L155-L198)
binds certificate and secret identity to the original destination IP, not a cached
DNS name. Upstream includes IPv4/IPv6 tests in `sdk/rust/tests/tls_intercept.rs`.
This is a source-level fix candidate; the controlled valid-IP-SAN upgrade check
below has not been run by this research session. Do not disable verification.

Allowing public IP destinations does not guarantee direct-IP HTTPS works while
verified TLS inspection is enabled. Network admission and TLS compatibility are
separate concerns.

- Evidence: [unrestricted networking documentation](config.md#network-policy).
- Upgrade check: use a controlled endpoint with a valid IP-SAN certificate and
  compare direct-IP versus hostname HTTPS without disabling verification.

## MSB-09 — Snapshot publication and removal have group constraints

**Status:** observed native semantics, not necessarily bugs. Last checked: v0.7.8.

Source rechecked 2026-10-28 at v0.7.11:
[`group.rs`](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/rust/lib/backend/local/snapshot/group.rs)
is byte-identical to v0.7.8. Explicit HEAD selection, the HEAD deletion guard,
and retention of empty group metadata remain in source. New snapshot import and
archive fixes do not replace the tool's publication/deletion ordering.

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

Source rechecked 2026-10-28 at v0.7.11: the
[restore builder](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/rust/lib/sandbox/builder.rs#L2315-L2319)
still maps captured managed root disks to `size_mib: None`. This supports retaining
the capacity-based assertion, not asserting nonzero metadata. Capacity was not
measured by this source review.

### Native image-cache lookup failures in DNS fixtures

Five DNS fixture failures were reported during implementation. Review identified
fixtures passing an original Docker tag directly to native creation with
`PullPolicyNever`, bypassing this project's identity-addressed import. Rechecked
2026-10-28 with v0.7.8: all five tests pass using the source VM's already imported
image reference, with DNS-policy assertions unchanged. The fixture has been
corrected; no upstream cache or DNS-enforcement bug is established.

References: `tests/vm/dns_test.go`, `internal/sandbox/image.go`.

Source rechecked 2026-10-28 at v0.7.11: anonymous native registry auth (v0.7.10) and
[image-cache/pruning changes](https://github.com/superradcompany/microsandbox/pull/1919)
(v0.7.11) do not establish an upstream DNS defect or justify reverting the fixture's
identity-addressed image reference. The project still imports Docker archives and
uses `PullPolicyNever`; this source review did not run DNS fixtures.

## Upgrade review checklist

- Record the candidate SDK and runtime versions separately.
- Recheck each entry against versioned source and its reproduction/test.
- Keep security assertions intact; do not redefine success around missing APIs.
- Record results per entry, including version and evidence; retain resolved history.
- Only mark fixed after verifying behavior, not merely finding a new API field.
- MSB-01 maps to upstream [issue #1835](https://github.com/superradcompany/microsandbox/issues/1835)
  and [PR #1880](https://github.com/superradcompany/microsandbox/pull/1880); add other
  verified issue links when available.
