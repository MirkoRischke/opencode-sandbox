# Remote TUI plugin isolation

**Decision: keep the TUI in the guest. A host TUI is not established safe against an untrusted guest server.** In OpenCode **2.0.30**, `plugins: ["-*"]` is an activation directive, not a pre-import code-execution barrier: discovered and server-advertised external TUI modules are evaluated before that directive is applied. Retaining host appearance is supported; retaining it does not establish isolation.

Research date: 2026-10-17. Threat: guest/project/server-controlled code executing in the trusted host client. Connection authentication is not a blocker for this trusted single-user host, and would not make an authenticated malicious server trustworthy.

## Evidence scope

- **Current documentation:** only official `/v2/docs/` pages, retrieved on the research date; these are not version-pinned.
- **Pinned implementation:** public release tag `v2.0.30`, resolved to [`f16ef9ad8878279ea2be3cfab7baeed97aef61aa`](https://github.com/anomalyco/opencode/commit/f16ef9ad8878279ea2be3cfab7baeed97aef61aa). Findings below are static source verification, not an executed exploit.
- **Current V2 development snapshot:** narrowly compared the TUI loader at [`4617210822bdbb31e5749751afbfb4584ccedac8`](https://github.com/anomalyco/opencode/blob/4617210822bdbb31e5749751afbfb4584ccedac8/packages/tui/src/plugin/context.tsx#L284-L398), the `v2` branch head observed during research. It retains the same discover → server inventory → CLI directives ordering and import-before-disable structure, including `Host.load` at line 678. This is not a complete latest-version audit or a claim about the latest published V2 release.
- The installed **host client version was not inspected**. Pinning the guest server alone does not pin the host loader whose behavior matters here.
- No host credentials, config, installed package metadata, or sessions inspected; no remote payload executed; no full recursive GitHub source-tree request made. Only this Markdown file changed.

## What the V2 documentation promises

1. The [CLI plugins guide](https://opencode.ai/v2/docs/cli/plugins) explicitly says the CLI obtains active plugins from the connected server, including a remote server, and automatically loads their TUI components. CLI-only plugins also run locally. Global and project plugin discovery exists independently.
2. The same guide documents ordered negative ID/wildcard directives. The [general plugins guide](https://opencode.ai/v2/docs/plugins#control) explicitly defines `*` as every plugin, `.*` as an ID prefix, and later IDs as re-enablement. Neither page promises that disabled modules are never imported.
3. [CLI settings](https://opencode.ai/v2/docs/cli/config) are global, not project-local. `OPENCODE_CLI_CONFIG_CONTENT` overlays global `cli.json`; nested objects merge, arrays/scalars replace. Thus an inline plugin array can preserve unrelated host theme/keybind settings while replacing the global plugin array.
4. The current [policies guide](https://opencode.ai/v2/docs/policies) uses **`integration.use` with resource `plugin:<target>`**, not `plugin.use`. It describes server/project configuration policy, precedence, and plugin loading controls; it does not document a host-TUI remote-plugin trust allowlist or a no-import boundary. It explicitly says policies do not sandbox plugin code.

## Verified 2.0.30 loader behavior

### `"-*"` matches everything, but too late

The decisive source is [`packages/tui/src/plugin/context.tsx`](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/tui/src/plugin/context.tsx):

| Stage | Verified behavior |
| --- | --- |
| Inventory selection, [lines 95–104](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/tui/src/plugin/context.tsx#L95-L104) | Accepts active server records with `features.tui === true` and package/local sources. No remote-origin trust check here. |
| Ordered targets, [lines 288–309](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/tui/src/plugin/context.tsx#L288-L309) | Local discovery first; server inventory second; host CLI plugin directives last. Remote local paths become `path.dirname(plugin.source.path)` on the host. |
| Builtins, [lines 313–316](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/tui/src/plugin/context.tsx#L313-L316) | Imports builtin definitions and initially enables them. |
| Resolve external, [lines 332–398](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/tui/src/plugin/context.tsx#L332-L398) | Resolves/imports each external target, then records its exported ID with `enabled: true`. |
| Negative directive, [lines 321–323](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/tui/src/plugin/context.tsx#L321-L323) | Only changes `enabled` on definitions already accumulated. Does not prevent earlier resolution/import. |
| Matching, [lines 648–649](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/tui/src/plugin/context.tsx#L648-L649) | `*` matches every ID. Therefore `-*` is accepted and does disable accumulated definitions. |
| Activation, [lines 442–446](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/tui/src/plugin/context.tsx#L442-L446) | Skips setup for disabled definitions. This is later than import. |

`resolvePlugin()` [lines 667–695](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/tui/src/plugin/context.tsx#L667-L695) resolves the `./tui` entrypoint and either calls `readSource(entrypoint)` for a local directory or `Host.load(entrypoint)` for a package. **Import even precedes validation of the exported plugin definition.** A module need not finish loading successfully to have already performed top-level side effects.

This is real runtime evaluation, not metadata inspection:

- [`Host.load`](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/plugin/src/host.ts#L45-L47) calls `importModule`.
- The [Bun implementation](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/util/src/runtime/import.bun.ts#L3-L5) uses `import(specifier)`.
- The [Node implementation](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/util/src/runtime/import.node.ts#L8-L11) invokes dynamic import with the main-context loader and `runInThisContext()`, not an isolated plugin sandbox.
- [Local source loading](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/plugin/src/source.ts#L29-L39) calls `prepared.load()`; both [Bun](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/plugin/src/source.bun.ts) and [Node](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/plugin/src/source.node.ts) preparation lead to `Host.load`.

**Consequence:** with an accessible external TUI module, top-level code can execute on the host even when `setup()` never runs and `/plugins` eventually reports it inactive. An inactive UI status is not proof of isolation.

### Host config precedence does not repair this

The [pinned CLI config service](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/cli/src/config/config.ts#L30-L34) reads inline configuration locally. Its [merge implementation](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/cli/src/config/config.ts#L117-L142) overlays values recursively except arrays. The TUI [loads configuration before constructing the plugin provider](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/tui/src/app.tsx#L212-L223).

Thus the defect is **not** that environment policy arrives after initial setup. The correct host config can already be present; the loader still processes external imports before its final disable directive. The ordinary remote inventory cannot directly rewrite host `cli.json`, but it need not override the directive to obtain import-time execution.

There is also an adversarial ordering caveat: [`mergePluginTargets`](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/tui/src/plugin/discovery.ts#L55-L66) deduplicates with JavaScript `Map.set`, preserving the first insertion position while replacing the value. A dishonest server can advertise package target `"-*"` before another external target. The host `"-*"` replaces that entry without moving it to the end, so the later target can be imported **and enabled after the disable**. The [source schema](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/schema/src/plugin.ts#L8-L20) permits an arbitrary string package target. This is a static control-flow inference, not a payload-tested result; the import-before-disable finding alone already determines no-go.

### Discovery and guest-writable mounted paths

- The [TUI startup](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/tui/src/app.tsx#L212-L223) computes plugin directories from **host `process.cwd()` and host global config**, even with a remote server.
- [`localPluginDirectories`](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/tui/src/plugin/discovery.ts#L8-L21) includes global config and project `.opencode` paths. [`discoverPluginTargets`](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/tui/src/plugin/discovery.ts#L24-L52) accepts immediate directories and directory symlinks. In this pinned TUI implementation, bare plugin files are not discovered as TUI targets, and explicitly configured local files are [skipped](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/tui/src/plugin/context.tsx#L332-L341). This is narrower than the general server plugin discovery documentation; directories remain enough for execution.
- A guest-writable mounted checkout can therefore supply `.opencode/plugins/<name>/tui.*` or package exports, or mutate dependencies of a local plugin. The loader's local-source fingerprinting/watching intentionally reloads changed code; hashing is freshness tracking, **not trust verification**. It also tracks local dependency graphs ([Bun](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/plugin/src/source.bun.ts), [Node](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/plugin/src/source.node.ts)).
- Server-advertised absolute paths are interpreted on the host, not downloaded from the guest. Different guest/host paths may happen to fail resolution, but path mismatch is not a security boundary. A mount at an accessible advertised path, or a host launch inside that checkout, supplies a direct route.
- For normal server-advertised package records, `install: false` means **resolve existing host packages**, not automatically install missing packages. The [CLI callback](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/cli/src/commands/handlers/default.ts#L155-L157) selects `npm.resolve` versus `npm.add`. Local discovery and explicit CLI declarations use `install: true`. Missing cache entries can limit a particular scenario, but do not remove the local-path route or make the client fail-closed.

### Allowlists, policy, and builtins

- No pre-import host trust allowlist, origin authorization, `plugin.use` check, or `integration.use` check exists in the inspected TUI reconciliation/import path. A sequence such as `["-*", "trusted.id"]` filters setup by an ID learned **from already evaluated code**; it is not code provenance validation.
- Pinned [policy schema](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/schema/src/config/policy.ts#L8-L12) supports only `provider.use`, `tool.use`, and `integration.use`. Do not recommend an invented `plugin.use` policy.
- A policy on the guest server can reduce an honest server's inventory, but an untrusted server can fabricate that inventory. A host server-side `opencode.json` policy is not shown to be consulted by this remote TUI loader. Neither is an enforceable substitute for a host pre-import guard.
- [`builtins.ts`](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/tui/src/plugin/builtins.ts) imports bundled UI feature plugins, including notifications, footers/sidebar, diff viewer, Merman, and LaTeX. `-*` suppresses their setup as well; it does not prevent their modules from being imported. Losing built-in UI features is an appearance/functionality tradeoff, not extra isolation.
- The docs' nonremovable `opencode.config.policy` and `opencode.provider.opencode` are [server plugin exceptions](https://opencode.ai/v2/docs/plugins#control), not a host-TUI safety switch.

## Appearance convenience versus security

This exact inline override is supported by the pinned configuration/selector code and preserves unrelated global appearance settings:

```sh
OPENCODE_CLI_CONFIG_CONTENT='{"plugins":["-*"]}' opencode --server <guest-url>
```

**Not a safe launcher recommendation.** It replaces global plugin declarations and normally prevents setup for accumulated plugins, but leaves import-time execution and the adversarial ordering caveat above. `plugins: []` is weaker still: discovery and remote inventory remain inputs. A separate `XDG_CONFIG_HOME`, trusted launch directory, or keeping the checkout off the host can reduce exposure, but none is verified here as an all-path remote code-execution barrier.

For appearance without moving execution to the host, keep the TUI in the guest and copy only vetted declarative CLI settings/theme data into a guest-owned config. Do not share the host config directory or credentials into the guest merely to reuse appearance. A built-in theme avoids copying executable plugins or plugin dependencies.

## Other dynamic imports and remaining limits

- Plugin entrypoints can import arbitrary transitive dependencies; limiting only the named entrypoint or exported ID would not suffice.
- The inspected theme-discovery path reads [`.json` with `JSON.parse`](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/tui/src/theme/discovery.ts), rather than importing theme JavaScript. This supports separating declarative appearance from plugin execution; it is not a complete theme/rendering security audit.
- The inspected [TUI startup](https://github.com/anomalyco/opencode/blob/v2.0.30/packages/tui/src/app.tsx#L259-L261) has a simulation frontend dynamic import gated by host `OPENCODE_DRIVE`; plugin builtins and runtime-support imports also exist. No exhaustive audit of every CLI/TUI dynamic import, renderer/native parser, RPC entrypoint consumer, terminal escape sequence, external-editor/opener action, package lifecycle, or update path was performed. No claim of absence of other guest-to-host execution routes is made.
- No isolated integration tests were run for malicious inventory, import side effects, hot reload, reconnect/location changes, or runtime-specific resolution. Such tests should run in a disposable isolated client environment, **not the trusted host**.
- The current host binary/build and its dependency tree remain unverified. The latest V2 snapshot comparison covers the main loader only. Future fixes require a new pinned review; documentation alone cannot establish a no-bypass boundary.

## Actionable go/no-go

**No-go for the unsandboxed host TUI under this threat model. Keep guest TUI execution.** There is no exact verified configuration here that prevents all guest-controlled local code evaluation while retaining a normal host client.

A future host-client go decision needs a pinned client with a host-owned, fail-closed switch that skips **all** external discovery, resolution, installation, and import before reading remote plugin inventory; cannot be overridden by remote metadata, IDs, deduplication, reload, or reconnect; and cannot resolve bundled/trusted code through guest-writable files/dependencies. It also needs an audit of other server-directed import/execute paths and isolated regression tests. Alternatively, put the client itself behind an OS sandbox/VM boundary that withholds host secrets and writable host paths—then plugin execution is contained rather than disabled.
