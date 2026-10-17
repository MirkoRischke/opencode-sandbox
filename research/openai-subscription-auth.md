# OpenAI subscription authentication in worktree VMs

Research date: 2026-10-17. Target: host and guest OpenCode **2.0.30**. Research only: no credentials read, authentication attempted, services started, VM changes, or model requests. This file is the only intended artifact.

## Conclusion

**Credential-only provisioning is available in 2.0.30, including real `auth export` / `auth import` CLI commands. However, copying one renewable OAuth session into the host and several independently refreshing guests is not a safe default.** Use fresh guest state plus exactly one selected credential; preserve guest-owned state thereafter. For parallel VMs, prefer separately authorized renewable sessions per persistent VM. If zero additional interactive logins is non-negotiable, a single refresh owner and custom narrowly scoped broker/proxy need further design and validation—not merely a reusable token-bearing image. [S2–S9, O2–O4]

An API key is **not** a way to spend the existing ChatGPT subscription: OpenAI distinguishes ChatGPT subscription sign-in from API-key usage billed through Platform. There is also a newer subscription-supported **Sign in with ChatGPT (SIWC)** path, already present in the 2.0.30 source; do not confuse it with legacy Codex OAuth or assume which kind the existing host login uses. [O1, O2, S6, S7]

## Evidence and version confidence

- GitHub's official `v2.0.30` tag resolves to commit `f16ef9ad8878279ea2be3cfab7baeed97aef61aa`. Findings below were checked against that tag, not inferred solely from today's docs. This verifies release-source behavior, **not** the identity/runtime behavior of the installed host/guest binaries. [S1]
- Full current V2 CLI/provider/API pages and public OpenAPI were fetched. The OpenAPI describes itself as experimental and reports contract version `0.0.1`, not product version `2.0.30`; it is not independently a release-pinned compatibility guarantee. Relevant credential shapes/routes also exist in the tagged release. [D1–D4, S2, S3]
- Important documentation gap: current provider instructions show legacy device login and omit export/import and the newer SIWC method. Tagged source explicitly contains all three. Source-backed details below are labeled accordingly; actual installed method discovery/help remains a validation step. [D1, S4–S7]

## Options compared

| Route | Subscription / support evidence | Friction and security judgment |
| --- | --- | --- |
| Independent device login in each guest | Documented OpenCode flow; tagged method `chatgpt-headless`, label **Codex device code (legacy)**. Guest polls OpenAI; human enters displayed code in host browser. No callback into VM required. [D1, S6] | Best network fit. One initial human action per persistent VM, not every launch. Distinct authorizations avoid deliberately cloning the same refresh token; independent-session behavior still needs testing. |
| Independent browser login in guest | Tagged legacy `chatgpt-browser` listens on guest `localhost:1455`, with `1457` fallback. New `chatgpt-token-sharing` / **Sign in with ChatGPT** listens on guest `127.0.0.1` at an ephemeral port. Both are automatic callbacks, not paste-code methods. [S6, S7] | Host-browser loopback reaches host, not guest. Requires explicit, temporary callback forwarding into the guest listener, or browser in guest. Default networking does not provide this automatically. Never forward the host OpenCode execution service. [O4; architectural deduction from S6, S7] |
| Credential-only CLI/API provisioning | Release CLI contains `auth export [target]`, `auth import [file]` (stdin when omitted); API exposes secret-bearing GET and credential creation POST. [S2–S5] | Lowest implementation effort; fresh guest DB stays free of old host sessions. Export targets an **integration**, not one account; trusted selection is mandatory. Copying a session also copies its renewal authority and refresh race risk. |
| Reusable credential-bearing seed/snapshot | Credential schema/import supports the data, but neither the reviewed V2 docs nor release source establishes a safe multi-VM refresh protocol. [D1, S3, S5, S8] | Reject as parallel-cloning default. Snapshot rollback resurrects stale tokens; a static seed decays after rotation. Reuse a secret-free base image; provision selected current credential separately. |
| Narrow host credential broker | OpenCode has a generic external-credential type, but the reviewed OpenAI/SIWC implementations use their own OAuth methods and refresh routines; no ready-made OpenAI host broker was found in these sources. [S3, S6–S8] | Custom engineering. One refresh owner could dispense short-lived access, but OpenCode integration/adapters and guest refresh suppression are required. Refresh tokens never enter guest; access tokens still do. Cannot just replace OAuth with an ordinary API-key value and assume identical routing/model behavior. |
| Host inference-only proxy | V2 documents configurable `baseURL` and custom providers; SIWC documents eligible Responses requests. These are primitives, **not** a supported turnkey OpenCode subscription gateway. [D5, O2, O6] | Custom engineering; can keep all OpenAI tokens outside guest. Must allow only inference endpoints, bound request sizes/usage, redact secrets, handle streaming, and avoid arbitrary upstream forwarding. Adds one narrowly allowed host/guest communication channel, contrary to default network isolation unless explicitly approved. |

The judgment/recommendation column is architectural analysis, not a vendor support promise. All OpenCode TUI, sessions, tools, and execution services remain inside each VM; a broker/proxy, if chosen, must not execute tools or expose host filesystem/session APIs. [D2, D3; requirements supplied for this research]

## What credential-only transfer actually supports

### Release CLI (source-verified, not shown on current provider page)

`opencode auth export openai` prints a JSON array of **all saved OpenAI accounts**, including their secret values. Internally it first calls the unfiltered credential-list API, then filters by integration in the trusted CLI process. It does not offer a credential-ID selector. Omitting `openai` exports every integration. Do not run it into a TUI transcript, agent tool output, shared log, or shared worktree. No such command was executed during this research. [S4]

`opencode auth import` reads that array from stdin; `opencode auth import <file>` reads a file. Entries include `id`, `integrationID`, `label`, `active`, and `value`. Import skips existing IDs instead of updating them. Existing destination integration selections are preserved; new integrations adopt exported selection. Therefore **import is initialization, not token synchronization**. Re-importing newer values under the same ID will not update a guest's stale tokens. [S3, S5]

### Published API, independently checked against 2.0.30

- `GET /api/credential` explicitly returns **every stored integration credential, including its secret value**. It has no provider/account filter in the checked contract. Do not give a guest this host capability, even through an ostensibly credential-only pass-through. [D4, S2]
- `POST /api/credential` requires `integrationID` and `value`; optional `id`, `label`, `activate`. Creation returns the full secret-bearing entry too. Omit host record IDs when creating a distinct guest record unless identity preservation is specifically needed. [D4, S2, S3]
- OAuth value: `type: "oauth"`, `methodID`, `refresh`, `access`, `expires`, optional `metadata`. Tagged provider code computes `expires` as epoch **milliseconds**. Preserve the actual method and all metadata: legacy metadata includes `accountID`; SIWC includes `clientID` and `scopes`. Never reinterpret a legacy token as SIWC by changing its method label. [S3, S6, S7]
- `PATCH /api/credential/{credentialID}` updates **label only**, although the internal core service can update secret values. DELETE removes a local record; POST `.../activate` selects it. A custom external token synchronizer cannot assume the public PATCH supports rotating tokens. [S2, S8, S9]
- Use the supported API/CLI, not SQLite manipulation. The V2 docs say saved credentials live in the server SQLite DB and legacy `auth.json` is migration input, not the current writeback store. Do not copy the host DB or entire data directory. [D1]

## Refresh ownership: the critical constraint

For **SIWC**, OpenAI explicitly says successful refresh returns a replacement refresh token and refreshes for the same session must be serialized. Its current token reference specifies one-hour access tokens and 30-day renewable refresh tokens. Those documented lifetimes belong to SIWC; do not automatically apply them to legacy Codex OAuth. [O3, O5]

Both release OpenCode paths save replacement tokens. The integration resolver refreshes when expiry is within five minutes, then updates its own stored credential. The reviewed resolver has no visible cross-process/distributed refresh lock; plugin-local semaphores around reload do not coordinate host plus separate VM databases. Even one VM can have multiple project/location activity, so in-process refresh safety deserves testing too. [S6–S8]

**Consequences (analysis):** host + guest A + guest B copied from one record can all attempt renewal using one refresh token. Independent DBs cannot learn each other's replacement. Even serial refreshes fail to solve stale copies unless the latest token is propagated before every subsequent refresh. A guest can also intentionally refresh or exfiltrate its token because it is unrestricted. A lock external to stock clients does not constrain clients that never consult it. [S5–S8, O3]

For **legacy Codex OAuth**, the reviewed release code posts `grant_type=refresh_token` to `/oauth/token` and overwrites the saved refresh token. This proves replacement handling, but not the provider's exact rotation/reuse grace period, token-family invalidation policy, or concurrency guarantees. No guarantee for cloning legacy refresh tokens was established. Treat shared-session fan-out as unsafe until explicitly validated, not as known to break on the first race. [S6]

OpenAI's current SIWC VM procedure describes **transferring an existing session**, then letting the VM own later refreshes. It does not promise safe simultaneous laptop + N-VM refreshers. Its separate client/host model permits one client registration across multiple distinct hosts; that is not permission to independently rotate one cloned session. [O2–O4]

## SIWC is genuinely relevant, but not identical to legacy Codex

Release `chatgpt-token-sharing` registers a user-owned client using `dynamic_agent_client`, requests `chatgpt.tokens.use.direct`, validates the ID token, stores issued client metadata, and sends requests to `https://api.openai.com/v1`. Using that URL does **not** by itself imply API billing: authorization type/grant matters. Legacy methods instead use the Codex backend and a fixed client ID. [S6, S7, O1, O2]

SIWC is an official optional capability for open-source/locally hosted apps to use eligible ChatGPT-plan Responses requests. OpenAI documents a stable distinct `ext_agent_host_id` per VM, secure selected-credential transfer, and VM ownership of later refresh. However, release credential export includes only credential records, not the ChatGPT plugin KV host-ID state; the release plugin creates its host ID during authorization, and its saved credential metadata does not retain `id_token`. A round-trip export/import is therefore **not proven to implement the whole current SIWC VM guide**. Do not copy all plugin/host state to fill that gap. [O2–O4, S4, S7]

SIWC preview requirements include `store:false` and `stream:true`; not all Responses capabilities are available. A custom proxy/provider must honor the actual SIWC contract rather than assume all API-key Responses features work. OpenCode's release SIWC provider also forces HTTP and summary compaction. [O6, S7]

Current OpenAI docs additionally describe **Codex access tokens for Enterprise automation**, requiring an Enterprise workspace and administrator permission. This is not a demonstrated solution for an unspecified personal subscription, and 2.0.30 compatibility was not established. Installing Codex CLI or using its app-server would add a different execution surface, not automatically authenticate the required OpenCode-in-VM setup. [O1]

## Security and operations

- **Unrestricted guest means credential exposure.** If OpenCode can read/refresh the guest credential, the unrestricted agent can obtain it through its local storage/API/process capabilities and exfiltrate it over allowed public internet. File permissions protect other users, not that same unrestricted guest. A token-bearing guest cannot provide host-level secrecy for that token. [D1, D3, S2; threat-model deduction]
- **Trusted selection boundary.** A future host provisioning helper must select exactly one expected OpenAI credential ID/account and reject missing/ambiguous results. Provider-only filtering is insufficient when several OpenAI accounts exist. The export/list implementation temporarily handles every host integration secret; a genuinely least-privilege host API for one secret was not found. Never expose its unfiltered result to guest. [S2, S4]
- **Secret transport proposal:** host-initiated, one-shot authenticated VM control/exec input, or an explicitly established encrypted transport. Avoid secret command arguments, inherited environment dumps, shared Git worktree files, build layers, snapshots, terminal output, crash reports, and logs. A protected ephemeral file is fallback only; plaintext removal is not guaranteed erasure from snapshots. Validate what the chosen Microsandbox control channel logs before selecting it. These are proposed controls, not verified Microsandbox capabilities. [O3, O4; design analysis]
- **Local logout is not remote revocation.** 2.0.30 `auth logout` invokes credential DELETE, and the inspected removal path deletes SQLite state without an OpenAI revocation call. Removing the host record does not erase guest copies. OpenAI SIWC docs require revoking the renewable session and tell compromised users to disconnect the app in ChatGPT Settings; transferred-session host-specific revocation is currently unavailable. Test actual revocation scope; do not promise per-VM isolation when sessions are shared. [S9, S10, O3, O4]
- **Persistence:** guest DB belongs to that VM only; restart should retain its latest rotated tokens. Restoring old VM snapshots or rebuilding from an old secret seed can require recovery/login. Do not use a live writable host DB, shared writable credential DB, or shared credential file as a shortcut. [D1, S5–S8, O3; operational deduction]
- **Git isolation caveat:** authentication isolation does not repair writable shared Git metadata. Given the supplied mount design, an unrestricted guest can modify that mounted metadata; host automation must not blindly trust guest-created repository/config/hook content. This is an independent boundary requiring review, not solved by any auth choice. [Threat model supplied for this research]

## Practical recommendation

1. **First prove credential-only bootstrap with one persistent VM, not a fleet.** After separate authorization for implementation/testing, use the release export/import path or equivalent POST to initialize a completely new guest DB with one selected credential. Prefer a separately obtained VM-dedicated session; avoid transferring the host's active session while the host continues refreshing it. Keep only guest-owned rotated state after provisioning. [S4–S8, O4]
2. **For multiple worktrees, choose explicitly:** accept one device/browser authorization per persistent VM (simplest supported operational route), or authorize engineering a single-owner inference proxy/broker (no repeated VM login, stronger host-secret boundary, more custom work). Existing host session cloning cannot currently be recommended as the reliable zero-login parallel solution. [D1, D5, S6–S8, O3]
3. If using a transferred host session for a **single** VM, make ownership transfer real: stop other consumers of that session. Removing local copies only is different from remote revocation; do not revoke the session you intend the VM to continue using. A static export is not a durable fleet seed. [S5, S9, S10, O3, O4]

## Validation plan — future, authorized work only

No steps below were executed. Separate non-inference checks from any later explicit permission to make a subscription-funded test request.

1. **Installed release check:** harmless guest `--version` and `auth export --help` / `auth import --help`; confirm method discovery matches source. Inspect the guest's own served OpenAPI only after authorization to start a guest service. Do not query host secrets merely to verify schema. [D3, S2, S4, S5]
2. **Freshness:** clean guest HOME/XDG/DB, no inherited API-key variables, no host DB/config/session directories mounted. In an isolated fixture, import one synthetic credential and verify one record, no sessions, and no other integration secrets; verify duplicate-ID import skips rather than updates. Do not infer real login success from schema acceptance. [D1, S3, S5]
3. **Callback/network:** device flow should need only guest outbound public access plus host-browser interaction. For browser flow, test temporary forwarding to the *guest callback only*, including SIWC ephemeral ports; verify host/private networks and host service remain unreachable afterward. [S6, S7, O4]
4. **Dedicated real session:** with user-controlled secrets outside transcripts, bootstrap one VM and verify metadata/method preserved, intended account selected, restart persistence, and no API-key fallback. Credential export/import may start a service; keep all guest calls directed inside that VM. [D1–D3, S4–S7]
5. **Renewal:** allow natural expiry or use an isolated authorized fixture; test sequential and parallel location use, reboot after refresh, and old snapshot recovery. Do not force an early rotation of the user's production host token. Use disposable sessions for deliberate race tests; record only redacted token fingerprints/status. SIWC tests should verify replacement storage and single refresh ownership. [S8, O3]
6. **Revocation:** distinguish guest record deletion, host record deletion, and ChatGPT-side disconnect/revocation. Test whether other separately authorized VM sessions survive; shared-token-family revocation scope remains an empirical unknown. Retain a recovery login procedure. [S9, S10, O3, O4]
7. **Before fleet rollout:** user must approve the credential-exposure tradeoff, and any host communication exception for a custom broker/proxy. Test secret-free logs/control-channel transport and refusal of non-inference routes. Only with separate authorization, make a minimal real request and verify ChatGPT usage attribution, not merely model availability or zero local cost display. [O1–O3, D5; proposed acceptance criteria]

## Primary sources

### Current V2 documentation (not release-pinned)

- **D1:** [CLI provider accounts, OAuth, storage](https://opencode.ai/v2/docs/cli/providers)
- **D2:** [CLI intro and background-service ownership](https://opencode.ai/v2/docs/cli)
- **D3:** [API reference](https://opencode.ai/v2/docs/api)
- **D4:** [Published OpenAPI JSON](https://opencode.ai/v2/openapi.json)
- **D5:** [Providers, endpoint overrides, custom runtimes](https://opencode.ai/v2/docs/providers)

### Official OpenCode 2.0.30 source

- **S1:** [Tag resolution](https://api.github.com/repos/anomalyco/opencode/git/ref/tags/v2.0.30)
- **S2:** [Credential HTTP contract](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/protocol/src/groups/credential.ts)
- **S3:** [Credential schemas](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/schema/src/credential.ts)
- **S4:** [CLI definitions](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/cli/src/commands/commands.ts), [export implementation](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/cli/src/commands/handlers/auth/export.ts)
- **S5:** [Import implementation](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/cli/src/commands/handlers/auth/import.ts)
- **S6:** [Legacy browser/device OAuth and Codex routing](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/core/src/plugin/provider/openai.ts)
- **S7:** [SIWC OAuth, host ID, metadata, refresh, provider routing](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/core/src/plugin/provider/chatgpt.ts)
- **S8:** [Integration resolution and refresh](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/core/src/integration.ts)
- **S9:** [Core credential storage/removal](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/core/src/credential.ts), [HTTP handlers](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/server/src/handlers/credential.ts)
- **S10:** [CLI logout implementation](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/cli/src/commands/handlers/auth/logout.ts)

### Current OpenAI documentation (protocol facts; not OpenCode compatibility promises)

- **O1:** [Codex authentication: subscription vs API billing, caching, Enterprise access tokens](https://developers.openai.com/codex/auth/)
- **O2:** [SIWC ChatGPT plan usage overview](https://developers.openai.com/siwc/token-sharing-open-source)
- **O3:** [Accounts/sessions: serialize rotation, revocation, credential security](https://developers.openai.com/siwc/token-sharing-open-source/profiles-and-sessions)
- **O4:** [Self-hosted VMs: transfer, host IDs, refresh ownership, revocation limitations](https://developers.openai.com/siwc/token-sharing-open-source/self-hosted-vms)
- **O5:** [SIWC token reference and lifetimes](https://developers.openai.com/siwc/token-sharing-open-source/token-reference)
- **O6:** [SIWC overview's inference requirements](https://developers.openai.com/siwc/token-sharing-open-source), [Models and inference](https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference)
