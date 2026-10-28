# Microsandbox v0.7.8 → v0.7.11 upgrade research

Recorded **2026-10-28**. Candidate: **Go SDK v0.7.11 and runtime v0.7.11**.

## Outcome

**Implementation follow-up (2026-10-28):** the findings below describe the upgrade
before adaptation. The tool now forwards secret substitution and related policy
fields during restore. All three selected-header skips were removed and their
targeted VM tests passed on SDK/runtime v0.7.11. MSB-01 is resolved for this tool;
the recommendation to implement policy preservation below is complete. See the
[updated tracker](../docs/microsandbox-limitations.md#msb-01--go-secret-modification-loses-selected-header-restrictions).

- **Upgrade verified by parent session:** `mise run upgrade-microsandbox 0.7.11`
  passed lint and the full standard suite after correcting one obsolete denial
  responder expectation. Launcher installed; dotfiles version pin updated;
  Chezmoi was **not** applied.
- **MSB-01 remains behaviorally present in this tool.** Upstream added the missing
  secret-policy modification API in v0.7.10, but the tool does not send it. Three
  temporarily unskipped selected-header tests still fail on v0.7.11. An upgrade
  alone does not narrow credential exposure.
- **MSB-04 default behavior improved:** restored VMs now have the HTTP denial
  responder enabled by default. The parent verified blocked HTTPS with hostname
  observations. Explicit restore-time HTTP configuration is still unavailable.
- **MSB-08 has a source-level fix candidate:** no-SNI, direct-IP TLS interception
  now authenticates the destination IP; no controlled IP-SAN experiment was run
  for this research.
- Keep the stop/modify/start enrichment, temporary TLS bootstrap, explicit snapshot
  HEAD publication, and ordered deletion. Source does not remove their prerequisites.

## Scope and evidence

This research session **ran no VMs, tests, or implementation changes**. Behavioral
results below are explicitly attributed to the parent upgrade session. Reading
upstream tests is source evidence, not a claim those tests passed locally.

Read all four tagged GitHub release records, compared actual extracted source
between each adjacent tag, and reviewed the combined Go API/native binding,
Rust sandbox, network, agent trust-store, snapshot, and image-cache changes against
this tool's integration. Baseline was the clean cached v0.7.8 checkout; v0.7.9,
v0.7.10, and v0.7.11 were fetched from first-party GitHub tag archives into
`/tmp/opencode/microsandbox-upgrade-source`. The v0.7.11 module-cache `modify.go`
also matches the tag source byte-for-byte. GitHub links below remain the durable
evidence; temporary downloads are not required to understand the findings.

Context7 was consulted for current library/API documentation. Its results point at
`main` and include older examples, so **tagged source, not current-index snippets,
determines version-specific conclusions**.

Primary release records and complete adjacent comparisons:

| Release | Published (UTC) | Primary evidence |
| --- | --- | --- |
| v0.7.8 baseline | 2026-10-09 | [Release](https://github.com/superradcompany/microsandbox/releases/tag/v0.7.8) |
| v0.7.9 | 2026-10-22 | [Release](https://github.com/superradcompany/microsandbox/releases/tag/v0.7.9), [actual changes](https://github.com/superradcompany/microsandbox/compare/v0.7.8...v0.7.9) |
| v0.7.10 | 2026-10-23 | [Release](https://github.com/superradcompany/microsandbox/releases/tag/v0.7.10), [actual changes](https://github.com/superradcompany/microsandbox/compare/v0.7.9...v0.7.10) |
| v0.7.11 | 2026-10-24 | [Release](https://github.com/superradcompany/microsandbox/releases/tag/v0.7.11), [actual changes](https://github.com/superradcompany/microsandbox/compare/v0.7.10...v0.7.11) |

## Changes material to this tool

### v0.7.9: denial defaults, transport, trust installation

**JSON HTTP 403 denials are now the default**, including network denials and
supported secret-policy violations. Separate network/secret messages and legacy
text/explicit opt-out remain available. The authoritative default is
[`HttpConfig::default`](https://github.com/superradcompany/microsandbox/blob/v0.7.9/packages/microsandbox-types/rust/lib/domain.rs#L1954-L1964),
also retained in [v0.7.11](https://github.com/superradcompany/microsandbox/blob/v0.7.11/packages/microsandbox-types/rust/lib/domain.rs#L1961-L1970).
[PR #1825](https://github.com/superradcompany/microsandbox/pull/1825) describes the
intent; [actual TCP denial handling](https://github.com/superradcompany/microsandbox/blob/v0.7.11/crates/network/lib/engine/tcp/proxy.rs#L1098-L1120)
and [TLS denial handling](https://github.com/superradcompany/microsandbox/blob/v0.7.11/crates/network/lib/engine/tls/proxy.rs#L284-L352)
show the local response path. Supported TLS denials do not open an upstream connection.

Impact: `restoreWorktree` deliberately strips `policy.HTTP` because restore validation
rejects it. That omission no longer disables the responder. Existing tests expecting
resets or `deny_response=false` require correction—not weakening policy enforcement.
An HTTP 403 is a transport-level success to plain `curl`; use `--fail` or check
status when deciding whether a request succeeded. Diagnostics are still not an audit.

The same change supports **direct-IP intercepted TLS without SNI**. The
[proxy](https://github.com/superradcompany/microsandbox/blob/v0.7.11/crates/network/lib/engine/tls/proxy.rs#L155-L198)
uses the original guest destination IP for certificate and secret identity; cached
DNS information may inform network policy but must not become the certificate
identity. Upstream added [IPv4/IPv6 integration cases](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/rust/tests/tls_intercept.rs#L892-L1019).
This changes MSB-08's source assessment, not its local verification status.

**Execution buffers are bounded without a public output-backpressure option.**
Release notes include both the optional feature and its revert; the shipped design
is the subsequent bounded-credit transport. The
[compatibility contract](https://github.com/superradcompany/microsandbox/blob/v0.7.11/COMPATIBILITY.md#execution-output-backpressure)
specifies 128 executions, 64 KiB stdin allowance, 1 MiB combined output allowance,
and 16 KiB chunks, with acknowledgements replenishing credit. Collection is not a
total-output-size limit. This tool already drains `ExecStream` events continuously;
do not introduce an obsolete optional-backpressure setting. Relevant changes:
[PR #1839](https://github.com/superradcompany/microsandbox/pull/1839),
[`exec/session.rs`](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/rust/lib/sandbox/exec/session.rs).

**CA installation becomes atomic and idempotent, not revoking.**
[PR #1856](https://github.com/superradcompany/microsandbox/pull/1856) changes bundle
updates to synced temporary-file replacement, preserves symlink targets and file
metadata, and skips an already-present PEM. It still reads all existing certificates
and appends a different PEM. Captured preparation CA trust therefore remains a
concern. See [v0.7.11 `tls.rs`](https://github.com/superradcompany/microsandbox/blob/v0.7.11/crates/agentd/lib/tls.rs#L181-L266).

**Published ports and live labels:** Go `ModifyOptions` gains `Ports`/`PortsRemove`;
Rust applies label modifications live. These do not provide network-policy changes
or a complete restore configuration. Current creation/restore port mappings and
stop/modify/start remain valid. See [Go API](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/go/modify.go#L36-L97),
[PR #1789](https://github.com/superradcompany/microsandbox/pull/1789), and
[PR #1881](https://github.com/superradcompany/microsandbox/pull/1881).

Network/runtime reliability changes include releasing completed published TCP
connections, preserving IPv4 fallback when IPv6 stalls, partial relay-frame reads,
bulk cancellation/failure delivery, and off-worker metrics snapshots. These are
relevant upstream reliability fixes, not reasons to change this tool's policy or
error handling. The [v0.7.9 release](https://github.com/superradcompany/microsandbox/releases/tag/v0.7.9)
links the individual primary patches. The default inbound published TCP limit also
changes from 256 to unlimited; [compatibility details](https://github.com/superradcompany/microsandbox/blob/v0.7.11/COMPATIBILITY.md#5-launcher-to-runtime-process-protocol)
and [`MaxInboundTCPConnections`](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/go/restore.go#L50-L56)
document the optional explicit cap. The project does not currently request a cap.

### v0.7.10: secret modification parity and native dependency changes

**The missing MSB-01 API is now available.**
[PR #1880](https://github.com/superradcompany/microsandbox/pull/1880), fixing
[#1835](https://github.com/superradcompany/microsandbox/issues/1835), adds
`Substitution`, `AllowPlaceholderFor`, `ViolationAction`, and `RequireTLSIdentity`
to Go `SecretModifySpec`. `Substitution.HeaderFields` expresses the original
creation-time restriction. The serializer sends `header_fields` as an array rather
than null; an explicitly supplied substitution replaces the **whole** stored rule.
Omitted policy keeps the stored/default policy. An empty header list means all
headers, not no headers. Policy edits on running sandboxes require restart/next-start.
See [public API and capability gate](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/go/modify.go#L99-L276),
[wire conversion](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/go/modify.go#L394-L452),
and [upstream dry-run integration coverage](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/go/integration/lifecycle_test.go#L184-L255).

Impact: `internal/sandbox/snapshot.go:restoredWorktreePatch` still sends only
value/placeholder/allowed hosts. **The integrated security limitation persists**;
this is no longer accurately described as an unavoidable v0.7.11 Go API gap.
Recommended follow-up: explicitly preserve the configured substitution policy on
restore, then rerun all three selected-header tests before removing skips or the
accepted-limitation documentation. No implementation change is made by this report.

**Anonymous registry auth** is opt-in and bypasses ambient keyring/config/Docker
credentials ([PR #1900](https://github.com/superradcompany/microsandbox/pull/1900),
[`WithRegistryAnonymousAuth`](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/go/options.go)).
This project pulls via Docker, imports immutable-ID archives, then creates with
`PullPolicyNever`; it need not adopt native anonymous registry auth for this upgrade.

**Security/dependencies:** [PR #1907](https://github.com/superradcompany/microsandbox/pull/1907)
updates security-sensitive dependencies and removes credential/log output from
examples and failure messages. Compare actual locks rather than assuming every
dependency named in that PR changed after this baseline: baseline `pageant` is
already 0.2.3, while `rustls` moves 0.23.45 → 0.23.46, `rustls-webpki`
0.103.15 → 0.103.16, and the 0.8-series `rand` 0.8.5 → 0.8.8. The published
`msb_smoltcp` 0.14.0 fork replaces the prior network dependency source
([PR #1906](https://github.com/superradcompany/microsandbox/pull/1906)). Evidence:
[baseline lock](https://github.com/superradcompany/microsandbox/blob/v0.7.8/Cargo.lock),
[candidate lock](https://github.com/superradcompany/microsandbox/blob/v0.7.11/Cargo.lock),
[candidate manifest](https://github.com/superradcompany/microsandbox/blob/v0.7.11/Cargo.toml#L155-L156).
These are favorable maintenance changes, **not a security audit or proof that all
vulnerabilities are fixed**. v0.7.9 also updates packaged firmware to Linux 6.12.113
([PR #1876](https://github.com/superradcompany/microsandbox/pull/1876)).

### v0.7.11: interactive execution, caller intent, cache safety

**Terminal attach acknowledges output** after writing it, avoiding a hang while
the guest waits for output credit before sending exit. Large stdin transfers no
longer block output/detach. Directly relevant to `Sandbox.Open` → `AttachWith`.
See [PR #1917](https://github.com/superradcompany/microsandbox/pull/1917),
[`attach.rs`](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/rust/lib/sandbox/attach.rs),
and [upstream terminal tests](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/rust/tests/attach_output.rs).
Zero-length stdin writes become a no-op rather than EOF
([PR #1931](https://github.com/superradcompany/microsandbox/pull/1931),
[`exec/io.rs`](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/rust/lib/sandbox/exec/io.rs));
the tool's explicit stdin close remains correct.

**Caller overrides survive configuration rebuilds.** Explicit caller intent is
stored separately from resolved defaults, guarded by a digest of the corresponding
config. This matters to stop/modify/start and restore rebuilds, including mounts,
clears, resource selections, and global/managed settings. Do not bypass the SDK's
configuration path or infer the Go restore API has gained new fields.
See [PR #1913](https://github.com/superradcompany/microsandbox/pull/1913),
[`caller_config_overrides.rs`](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/rust/lib/sandbox/caller_config_overrides.rs),
and [restore builder](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/rust/lib/sandbox/restore_builder.rs).

**Image pruning gains safer leases/locking** without serializing unrelated work,
relevant to archive import and concurrent runtime operations. It does not replace
the project's immutable Docker identity addressing. See
[PR #1919](https://github.com/superradcompany/microsandbox/pull/1919),
[`storage_lease.rs`](https://github.com/superradcompany/microsandbox/blob/v0.7.11/crates/image/lib/storage_lease.rs),
and [`image/cleanup.rs`](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/rust/lib/image/cleanup.rs).
Do **not** count interrupted-creation cleanup PR #1918 as a shipped fix merely
because release notes list it: [PR #1928](https://github.com/superradcompany/microsandbox/pull/1928)
reverts that work before release. The actual tag contains the revert.

**Authenticated HTTP CONNECT proxies** become exposed across SDKs. Credentials
are host-resolved from a password source and require runtime capability support;
they are not guest environment credentials. This tool has no such proxy setting,
so no new configuration is warranted. Basic auth is not itself encrypted transport
to a proxy. See [PR #1914](https://github.com/superradcompany/microsandbox/pull/1914),
[Go proxy documentation](https://github.com/superradcompany/microsandbox/blob/v0.7.11/docs/sdk/go/proxies.mdx),
and [native validation](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/go/native/src/lib.rs#L1389-L1474).

## Complete tracker disposition

“Source” below means this research; “parent” means independently supplied upgrade
verification evidence. A passing general suite is not proof of every specialized
upgrade check, especially skipped assertions and exploratory trust/IP tests.

| Entry | v0.7.11 assessment and evidence | Remaining action |
| --- | --- | --- |
| MSB-01 selected headers | API gap addressed in v0.7.10 ([Go API](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/go/modify.go#L99-L136)); current tool still omits policy. Parent deliberately unskipped all three tests: **all fail**, nil header fields and fake credentials substituted in `X-Untrusted`. | Preserve policy with the new API; verify before removing skips. Still security-relevant today. |
| MSB-02 complete preboot destination config | Source: [RestoreConfig](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/go/restore.go#L15-L57) still lacks secrets, labels, workdir, arbitrary env, TLS, proxy options. Live labels do not close this gap. | Keep enrichment stop/modify/start; no second-boot removal justified. |
| MSB-03 direct TLS restore settings | Source: same restore API has no TLS settings and rejects non-policy network fields. | Keep temporary bootstrap for secret-free restore; custom ports/upstream CA experiment still outstanding. |
| MSB-04 restore denial responder | Source: HTTP override still [rejected](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/go/restore.go#L156-L163); omitted config now defaults to responder enabled. Parent positively verified restored HTTPS denial hostname evidence. | Default-path missing diagnostics resolved by tested new default; retain API restriction/history. |
| MSB-05 modify network policy | Source: [ModifyOptions](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/go/modify.go#L36-L90) adds ports, not arbitrary policy. | Do not replace disposable preparation + restricted restore with a live policy transition. |
| MSB-06 captured old CA | Source: [bundle append](https://github.com/superradcompany/microsandbox/blob/v0.7.11/crates/agentd/lib/tls.rs#L181-L213) retains previous contents; atomicity/deduplication is not revocation. | Distinct-CA and nested-container trust verification still outstanding. |
| MSB-07 incomplete audit | Source: default responder improves supported HTTP/HTTPS visibility, but [bypass and failed-response paths remain](https://github.com/superradcompany/microsandbox/blob/v0.7.11/crates/network/lib/engine/tls/proxy.rs#L199-L223). Tool remains a system-debug-log parser. | Keep “not a complete audit”; test DNS/TCP/TLS/protected cases separately when needed. |
| MSB-08 direct-IP TLS | Source: destination-IP fallback and upstream IPv4/IPv6 tests added in v0.7.9. | Controlled valid-IP-SAN comparison still required; do not mark behavior fixed or disable verification. |
| MSB-09 group publication/removal | Source: [group.rs](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/rust/lib/backend/local/snapshot/group.rs) is byte-identical to v0.7.8. Divergent captures do not implicitly select HEAD; removing HEAD with other members is refused; empty metadata retained. | Keep explicit HEAD selection and deletion ordering; no new group-delete API identified. |
| Root disk `SizeMiB=0` verification entry | Source: [builder](https://github.com/superradcompany/microsandbox/blob/v0.7.11/sdk/rust/lib/sandbox/builder.rs#L2315-L2319) still maps managed captured roots to unspecified size (`None`). Parent full suite passes its capacity-based tests. | Keep capacity/filesystem assertions, not a nonzero-metadata requirement. |
| DNS image-cache verification entry | Source: new native anonymous auth/cache locking does not change the tool's Docker import identity seam. Parent standard suite passes corrected fixtures. | Keep imported reference with `PullPolicyNever`; no upstream DNS/cache bug established. |

## Parent behavioral evidence, verbatim scope

1. Initial full verification failed only at the old
   `TestSnapshotRestoreBlocksHTTPSWithoutDenialDiagnostics` expectation that
   `network.http.deny_response` be false. Parent reproduced this twice using
   `mise exec -- go test -count=2 -run '^TestSnapshotRestoreBlocksHTTPSWithoutDenialDiagnostics$' -v -timeout 1m ./tests/vm`.
2. Parent renamed it `TestSnapshotRestoreBlocksHTTPSWithDenialDiagnostics` and
   asserted both `deny_response=true` and positive `example.org` hostname
   observations after blocked
   `curl --resolve example.org:443:1.1.1.1 https://example.org`, avoiding DNS-based
   diagnostic evidence. Updated test passed twice targeted and once in full suite.
3. Parent temporarily removed the three MSB-01 skips and ran:

   ```sh
   mise exec -- go test -count=1 -run '^(TestGitHubInjectionIsRestrictedToAuthorizationOnExactGitHubHosts|TestGitHubCredentialSubstitutionWorksOutsideGuestAndExcludesOtherHeaders|TestConfiguredSecretsUseRelativeFilesAndSubstituteOnlySelectedHeaders)$/selected_headers$' -v -timeout 1m ./tests/vm
   ```

   All three failed: selected-header metadata absent; fake secret values appeared
   in arbitrary `X-Untrusted` headers. Parent restored the skips with v0.7.11
   recheck messages. **Those security assertions did not pass.**
4. Parent then reported the complete `mise run upgrade-microsandbox 0.7.11`
   successful, including lint and standard tests; documented skips remain skips.
   This does not establish manual terminal behavior, distinct-CA revocation,
   direct-IP certificate compatibility, interrupted publication recovery, or
   exhaustive denial observability.

## Recommendations

1. Accept the verified SDK/runtime upgrade with the corrected denial-default test
   and documentation. No broader implementation changes are required for basic
   upgrade compatibility by the supplied standard-suite evidence.
2. Treat policy preservation through the newly available `SecretModifySpec`
   fields as a **separate security follow-up**, not as automatically fixed by this
   upgrade. Keep the explicit accepted limitation and skipped evidence until the
   integration is changed and the security assertions pass.
3. Retain existing restore enrichment, TLS bootstrap, identity-addressed imports,
   snapshot HEAD selection, and ordered removal. Avoid adding proxy, dynamic-port,
   backend, or registry configuration merely because upstream added it.
4. Recheck MSB-08 with a controlled valid-IP-SAN endpoint before promising direct-IP
   HTTPS. Keep MSB-06 and MSB-07 security caveats; atomic CA writes and readable 403s
   respectively improve reliability and visibility, not trust revocation or audit
   completeness.

Tracker updated: [`docs/microsandbox-limitations.md`](../docs/microsandbox-limitations.md).
This research changed no code, tests, or README.
