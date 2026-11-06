# Reusing Docker images across fresh Microsandbox VMs

**Recommendation: for a small, known image set, share a read-only directory of `docker save` archives and run guest `docker load`; for a reusable fully warmed VM baseline, use Microsandbox's existing disk snapshot/restore API. Version 0.7.8 already snapshots the separate owned `/var/lib/docker` disk and restores it privately. Do not share one writable Docker disk between concurrently running VMs.** A host registry/cache is also feasible without broad host access: native policy can allow just `host` + TCP + one port, before the existing host-deny rule. Registry behavior/configuration is intentionally left to the separate Docker-cache investigation. [R1, V1, V2, S1, S2, N1]

Research date: 2026-10-18. Primary-source inspection only; no VM operations, daemon changes, builds, or infrastructure changes. Context7 lookup failed because its monthly quota was exhausted; versioned upstream source and docs were inspected instead. Background delegation was attempted as required by the research skill but rejected by the subagent-depth limit. Only this findings file is added.

## Versions and current repository

- Both [`go.mod`](../../go.mod) and [`mise.toml`](../../mise.toml) pin **0.7.8**. Inspected upstream tag `v0.7.8` resolves to source commit **`7b7b9dc89e9a1c77801918f7833c3381d1754579`**. All versioned source links below refer to that release, not current website assumptions. [R1, P1]
- Current `main` observed at **`e52668369899e840223b5b8f2bd234f9d94d113d`**, 30 commits ahead. The comparison includes snapshot-catalog and volume-removal fixes, new published-port modification, and unrelated network/runtime/docs changes. The owned-disk restore implementation inspected here is already in 0.7.8, **not a future feature**. No upgrade is required to obtain the core volume/snapshot mechanisms described below. Do not infer that every current-main documentation example is in this pin. [P2]
- Current launcher uses a shared/layered OCI root plus a **private owned ext4 disk at `/var/lib/docker`**, default 10 GiB; image explicitly selects classic `overlay2` rather than the containerd image store. Owned disk survives stop/start but is removed with its VM. Worktree is a host bind mount. `Prepare` connects/creates, then ensures Docker is running. [R1]
- Existing `docs/research/microsandbox-dockerd-startup.md` was read for conventions, but its older repository-state description predates the current Docker implementation. This report uses the actual working-tree files, not that old state. [R1]

## Options ranked by implementation and operational burden

The ordering assumes **fresh, potentially concurrent VMs**, no large custom tooling, and a modest known baseline. A changing/unbounded image workload can make a standard registry cache preferable; this table does not attempt its Docker-specific evaluation.

| Rank | Approach | Burden and fit |
| --- | --- | --- |
| 1 | **Read-only host directory or directory named volume containing Docker image archives** | Small mount addition and guest `docker load`; no service, network exception, disk surgery, or snapshot lifecycle. Removes repeated upstream downloading for the supplied images, but each fresh VM still imports/unpacks them into its own disk. Updating archives is explicit. [V1, D1, D2] |
| 2 | **Disk snapshot of a clean warmed baseline; restore a new child per VM** | Native public API already available, including owned Docker disk. No repeated image import for the warmed set. Requires deliberate baseline refresh, snapshot retention, and a creation/restore path integrated into this launcher. Best native option when warm startup matters. [S1–S4] |
| 3 | **Host cache service with a one-port network exception** | Standard native networking, not a broad host grant. Requires service lifecycle and a new approved launcher configuration surface; Docker registry details are outside this note. Better candidate for arbitrary pulls than manually enumerated archives. [N1–N3, R1] |
| Conditional | **One persistent named disk reused by successive VMs** | Tiny mount change, but only safe for one attached writer at a time; carries *all* Docker state across jobs, not just image cache. Acceptable only if serialized operation and shared state are explicitly wanted. Not a concurrent-fresh-VM solution. [V1, V3] |
| Higher | **Full snapshots/live forks** | Supported and include owned storage, but preserve running processes/memory and impose execution-state compatibility. Unnecessary for merely caching images; prefer cold disk restore. [S1, S5] |
| Avoid | **Copy/edit Microsandbox's internal `disk.raw` or database records** | No need: public snapshot/restore already supports owned disks. Couples launcher to runtime journals/layer layout and risks corruption or broken lifecycle ownership. [S3, S4, V3] |

## What storage sharing actually means in 0.7.8

### Supported public mount choices

| Kind | Implementation and ownership | Cache consequence |
| --- | --- | --- |
| `m.Mount.Owned(m.OwnedVolumeOptions{Kind: m.VolumeKindDisk, SizeMiB: ...})` | Local-only managed raw ext4 initially; private sandbox lifetime; starts empty, persists across restart, removed with VM. No source path, name, seed, or clone parameter. [V1, V2] | Current Docker layout. Another fresh `CreateSandbox` gets an empty Docker disk; snapshot restore is the native way to get a private warmed copy. |
| `m.CreateVolume(..., m.WithVolumeKind(m.VolumeKindDisk), m.WithVolumeSize(...))`, then `m.Mount.Named(...)` | Named disk persists independently of sandbox. `Named` is existing-only; `NamedWith` supports explicit `existing`, `create`, `ensure-exists`. [V1, V2] | Useful for serialized reuse, not concurrent daemons. Name does not mean clone or CoW. |
| Directory named volume | Host directory exported through virtiofs, shareable between VMs; host/SDK filesystem access is supported. [V1] | Good for archives/files; not a drop-in shared Docker data root. Keep actual Docker store on its ext4 disk. |
| `m.Mount.Bind(hostDirectory, m.MountOptions{Readonly: true, ...})` | Supported host directory mount; read-only virtiofs also rejects writes server-side. [V1, V2] | Simplest offline archive source. Share only a dedicated cache directory, not host Docker socket or `/var/lib/docker`. |
| `m.Mount.Disk(hostImage, m.DiskOptions{...})` | Attach an explicitly user-managed raw/qcow2/vmdk image through virtio-blk; read-only option available. It attaches the selected file, **does not automatically clone it**. [V1, V2, S5] | Supports distributing a prebuilt filesystem image, but per-child writable-image creation would be your responsibility. |

There is no separate local “host volume” object to request: local uses bind mounts or named volumes. `GetDefaultVolume` and the organization's special host/default volume are cloud-only; they do not grant local host filesystem access. Owned volumes and disk-kind named volumes discussed here are local features. [V1, V2]

### Concurrent ext4 attachment: stronger restriction than “one writer”

**On this Linux pin, a writable named or host-image disk attachment obtains an exclusive nonblocking `flock`; a read-only attachment obtains a shared lock.** A writer therefore excludes both other writers **and read-only readers**, not merely a second writer. Multiple read-only attachments are allowed once no writer holds the disk. The lock is retained for runtime lifetime/handoff; stopping just Docker while leaving the VM running does not release its disk attachment. [V3]

That is the correct boundary for these ordinary ext4 disks, not a cluster filesystem with multi-host cache/journal coordination. Do not bypass locks to put two daemons on the same ext4 filesystem. “Shared read-only seed” is safe only as immutable data; native `Readonly` does **not** turn it into an automatic writable overlay for each Docker daemon. A functioning daemon still needs private writable state. [V1, V3; design inference]

Named directory volumes can safely expose files to several VMs in the ordinary filesystem-sharing sense, but that says nothing about application-level concurrency correctness. Use immutable, host-published archives, not concurrent modification of Docker internals. [V1; recommendation]

## Native warmed baseline: owned Docker disk is included

This is a supported public workflow in **v0.7.8**, not an undocumented internal-storage trick:

1. Create a dedicated baseline VM using the desired image and the same owned ext4 mount at `/var/lib/docker`.
2. Start Docker, pull/load the desired images, and remove unwanted containers/state. Quiesce and explicitly stop Docker cleanly before stopping the baseline VM; filesystem flush is not application quiescence. The current launcher does not explicitly gracefully stop Docker. [R1, S1]
3. Capture a **disk** snapshot using `m.Snapshot.Create(ctx, m.SnapshotCreateOptions{Name: "ready", FromSandbox: "docker-baseline"})`, or `GetSandbox(...).Snapshot(...)`.
4. Restore each new VM with `m.RestoreSandbox(ctx, snapshot, newName, ...)`. It boots fresh, but both root changes and the **owned Docker disk** are restored. Child disk writes never modify source or siblings. Run the launcher's normal Docker readiness step afterward. [S1–S4]

Illustrative API shape (not a complete launcher implementation):

```go
snap, err := m.Snapshot.Create(ctx, m.SnapshotCreateOptions{
    Name: "ready", FromSandbox: "docker-baseline",
})
// Handle err before restore.
child, err := m.RestoreSandbox(ctx, snap, "new-worker",
    m.WithRestoreConfig(m.RestoreConfig{
        NetworkPolicy: policy,
        Volumes: map[string]m.MountConfig{
            worktree: m.Mount.Bind(worktree, mountOptions),
        },
    }))
```

`WithRestoreConfig` replaces the whole config; do not put it after other options whose fields you want preserved. The owned `/var/lib/docker` mount is intentionally **not** remapped: owned storage restores automatically and explicit replacement is rejected. External binds recorded by the snapshot need explicit destination mappings; avoid blanket `WithDangerouslyInheritResources`. A baseline without project binds is easier to reuse across worktrees. [S2–S4]

**Integration caveats:** `RestoreSandbox` has its own option type, not the launcher's current `[]SandboxOption`; it returns a detached VM. Its public `RestoreConfig` supports host network policy and mount overrides but is not a general create-options passthrough: no replacement image, arbitrary environment, workdir, labels, or secret-creation options. Do not assume the current `connectWithSecrets` create path can simply be swapped for restore unchanged. Review credentials, TLS/DNS configuration, labels, and project initialization explicitly before implementation. A snapshot includes guest files, so prepare it without project credentials, private checkout contents, or unwanted Docker containers/volumes. [S2, R1; integration recommendation]

### CoW versus a full 10-GiB copy

The implementation is more useful than “copy the whole Docker disk,” but **not an unconditional zero-copy guarantee**:

- Restore reconstructs each owned disk's immutable captured layer chain and adds a new private **qcow2 writable head**. It never opens a captured layer as the child's writable head. This applies to the secondary owned disk, not only the root. [S3]
- The first immutable layer is hard-linked when possible; fallback uses `fast_copy`. Later layers require private inodes because qcow2 backing-path relocation modifies their headers; these use `fast_copy`. That helper tries filesystem reflink first, then a sparse-aware byte copy. [S3, S4, C1]
- Thus unchanged blocks in an immutable base can be physically shared and writes are private; on unsupported/cross-filesystem paths or larger layer histories, some captured data is copied. Sparse copy avoids materializing every unallocated byte but is still real I/O for allocated content. Do not promise constant-time cloning of a warm 10-GiB disk. [C1, S3, S4]
- `WithCowMemory()` concerns **full-snapshot RAM**, not whether the owned Docker disk gets its private CoW disk head. Disk-only templates do not need this option. [S2, S5]

Capture itself can involve copying/sealing disk state. Benchmark fresh restore and baseline refresh on the actual host filesystem rather than quoting memory-fork latency as Docker-disk clone performance. [S3, C1; recommendation]

### Full snapshot, fork, import, and template status

- Disk snapshot is the simplest “template” mechanism: stored reusable baseline, fresh boot, private owned storage. No separate `Template`/`CloneVolume` factory was found in the inspected Go public volume API. [S1, V2]
- Full snapshot captures disk + memory + execution; restore resumes processes and requires compatible runtime/kernel/geometry. Live `SandboxHandle.Fork`/`ForkMany` is implemented locally, preserves execution state, and also privately materializes owned volumes. Neither is needed to avoid Docker image downloads. [S5]
- Snapshot archives are supported: `Snapshot.Save`/`Load` and direct archive restore; `WithImage: true` includes the **Microsandbox base OCI image** for offline movement. Owned disks remain in the snapshot's captured storage. This is not `docker save` and not an OCI export of guest Docker's image store. [S1, S2]
- Named disks have distinct captured-volume selection during restore (`RestoreConfig.CapturedVolumes`), documented for restoring private copies of captured named disks. Do not equate this with plain named mount reuse or a standalone volume clone API; the owned-disk path already fits this repository better. External user disk images are not automatically converted into owned volumes. [S1, S2]
- No unimplemented/planned feature is required for the recommended local snapshot workflow. Cloud full snapshots/forks and local owned mounts are explicitly unsupported on cloud in the pinned docs. Future roadmap promises are not treated as available APIs. [S1, V1]

## Why baking `/var/lib/docker` into the VM OCI image does not solve this layout

Microsandbox caches and shares OCI **root filesystem layers** across VMs. That does not share the separate nested Docker daemon's image store. This repository already loads the outer VM image with `docker save ... | msb load` and uses `PullPolicyNever`; it is a different cache from Docker inside the VM. [R1, I1]

Owned volumes explicitly start empty and mounting at an existing image directory **hides its contents without copying them**. Therefore, embedding populated `/var/lib/docker` files in the outer OCI image would be hidden by the current owned mount. Docker-like named-volume image-directory “copy-up” should not be assumed here. [V1, V2]

Simple supported image-based alternative: put **Docker archives** under another image path (e.g. `/opt/docker-seed`) and `docker load` them after the owned disk is mounted and Docker starts. Outer OCI caching shares the archive bytes, but every VM still performs guest Docker import/unpacking. A dedicated read-only host mount avoids rebuilding the VM image whenever the seed changes. Neither needs Docker's daemon running during an ordinary Dockerfile build. [I1, V1, D1, D2; design inference]

Removing the Docker disk mount and relying on the layered root for a baked live Docker store changes the known storage architecture; this research did not establish that nested `overlay2` works correctly on that root. Not recommended as an untested shortcut. [R1; uncertainty]

## Narrow host cache connectivity under the current deny policy

**Native API can grant a specific host TCP service; current launcher settings cannot express it.** Current `networkPolicy` orders DNS allowance, then explicit denies for `host`, private/loopback/link-local/metadata/etc., then exact public hostname TCP/443 allowances. Adding `host.microsandbox.internal` to `allowed_hosts` does not override the earlier host deny and is not a safe implementation of local-cache access. Existing VMs also retain their old saved policy. [R1, N1]

For an intentionally approved host cache, insert only this kind of exception **before** the general host deny (illustrative port, not a selected cache configuration):

```go
m.PolicyRule{
    Action: m.PolicyActionAllow, Direction: m.PolicyDirectionEgress,
    Destination: "host", Protocol: m.PolicyProtocolTCP, Port: "5050",
}
```

Keep the default denies and all other protected-destination rules. No `public,host` profile, allow-all host access, blanket private-network access, disabled rebinding protection, or guest port publication is needed. First matching rule wins; the `host` destination means the sandbox gateway, not all private addresses. [N1–N3]

The native alias `host.microsandbox.internal` resolves to the gateway; gateway TCP connections are rewritten on the host to loopback on the **same port**, with IPv4/IPv6 fallback. A cache can therefore bind host loopback rather than all interfaces. This narrows network exposure but does not authenticate the service or restrict HTTP methods/paths: whichever host service occupies that port is reachable. Keep a stable dedicated port and handle registry HTTP/TLS trust correctly; native host permission is not Docker insecure-registry configuration. Multi-tenant operator isolation can impose an additional public-only floor that a sandbox policy cannot broaden. [N2, N3]

A narrowly shared read-only archive directory avoids all these network/TLS changes. Use native `Mount.Bind`/directory `Mount.Named` with `Readonly: true`; `Noexec`, `Nosuid`, `Nodev` are suitable additional mount flags for an archive cache. There is no launcher mount-configuration option today, so this still requires an explicit, reviewed small launcher change rather than arbitrary host-path access. Host publishes archives; consumers never need write permission. [R1, V1, V2; recommendation]

## Negative results and acceptance checks

- No automatic cross-VM Docker pull cache is provided by Microsandbox's OCI image cache, owned volumes, or named-volume names. No owned-disk seed/path parameter or standalone Go `CloneVolume` was found. Use archives, a registry service, or snapshot restore. [I1, V2, S1]
- Read-only shared disks do not supply a writable Docker data root. Writer + readers of the same disk are rejected on Linux; do not rely on an SDK automatically serializing jobs for you. [V3]
- Manual copying of an explicit **user-managed** stopped disk image followed by public `Mount.Disk` is different from editing internal Microsandbox storage. It is possible as host-side file management, but copy/reflink selection, disk consistency, per-VM paths, capacity, cleanup, and ownership become custom responsibilities. No public `Mount.Disk(... clone seed ...)` option was found. Prefer the native whole-sandbox disk snapshot. [V2, S3]
- No runtime test here proves a particular Docker image/backend workload survives snapshot restore. Later disposable acceptance test: preload an image, cleanly stop daemon/VM, snapshot, restore two children concurrently, verify image exists without upstream access, start Docker in both, write independent container/image state, delete one child, and verify sibling and source remain intact. Also test current network/secret initialization, per-worktree mounts, labels/cleanup, and restore speed/allocated host storage. [S1–S4; proposed test]

## Primary sources

- **R1:** Working tree: [`go.mod`](../../go.mod), [`mise.toml`](../../mise.toml), [`Dockerfile`](../../Dockerfile), [`internal/sandbox/sandbox.go`](../../internal/sandbox/sandbox.go), [`internal/sandbox/network.go`](../../internal/sandbox/network.go), [`docs/config/README.md`](../config/README.md), [`docs/research/microsandbox-dockerd-startup.md`](microsandbox-dockerd-startup.md). Observed state on research date.
- **P1:** [Pinned source commit](https://github.com/superradcompany/microsandbox/tree/7b7b9dc89e9a1c77801918f7833c3381d1754579).
- **P2:** [Observed main commit](https://github.com/superradcompany/microsandbox/commit/e52668369899e840223b5b8f2bd234f9d94d113d), [release-to-main comparison](https://github.com/superradcompany/microsandbox/compare/v0.7.8...e52668369899e840223b5b8f2bd234f9d94d113d).
- **V1:** [v0.7.8 volume guide: bind, owned, named, sharing, disk images](https://github.com/superradcompany/microsandbox/blob/v0.7.8/docs/sandboxes/volumes.mdx#L64-L398).
- **V2:** [Go mount factories/options source](https://github.com/superradcompany/microsandbox/blob/v0.7.8/sdk/go/options.go#L1770-L1990), [Go named-volume public API](https://github.com/superradcompany/microsandbox/blob/v0.7.8/sdk/go/volume.go#L16-L193), [owned volume contract](https://github.com/superradcompany/microsandbox/blob/v0.7.8/docs/sdk/go/volumes.mdx#L24-L40), [read-only server enforcement contract](https://github.com/superradcompany/microsandbox/blob/v0.7.8/docs/sdk/go/volumes.mdx#L967-L982).
- **V3:** [Linux named/host/owned disk exclusion and shared/exclusive locks](https://github.com/superradcompany/microsandbox/blob/v0.7.8/sdk/rust/lib/runtime/spawn.rs#L1948-L2107), [runtime lock lifetime/handoff](https://github.com/superradcompany/microsandbox/blob/v0.7.8/sdk/rust/lib/runtime/handle.rs#L130-L179).
- **S1:** [Disk snapshot workflow](https://github.com/superradcompany/microsandbox/blob/v0.7.8/docs/sandboxes/snapshots.mdx#L9-L164), [owned and external mounts](https://github.com/superradcompany/microsandbox/blob/v0.7.8/docs/sandboxes/snapshots.mdx#L638-L800), [flush limitations](https://github.com/superradcompany/microsandbox/blob/v0.7.8/docs/sandboxes/snapshots.mdx#L424-L493), [archive transport](https://github.com/superradcompany/microsandbox/blob/v0.7.8/docs/sandboxes/snapshots.mdx#L1159-L1230).
- **S2:** [Go snapshot options](https://github.com/superradcompany/microsandbox/blob/v0.7.8/sdk/go/snapshot.go#L10-L73), [actual public restore API and replacement semantics](https://github.com/superradcompany/microsandbox/blob/v0.7.8/sdk/go/restore.go#L12-L168), [restore reference](https://github.com/superradcompany/microsandbox/blob/v0.7.8/docs/sdk/go/snapshots.mdx#L843-L888).
- **S3:** [Owned-volume restoration: required private reconstruction, layer sharing/copying, qcow2 head](https://github.com/superradcompany/microsandbox/blob/v0.7.8/sdk/rust/lib/backend/local/snapshot/restore/owned.rs#L16-L222).
- **S4:** [Stopped capture includes owned volumes](https://github.com/superradcompany/microsandbox/blob/v0.7.8/sdk/rust/lib/backend/local/snapshot/create.rs#L1374-L1418), [file snapshot restore materialization](https://github.com/superradcompany/microsandbox/blob/v0.7.8/sdk/rust/lib/backend/local/snapshot/restore.rs#L186-L225), [layer copying](https://github.com/superradcompany/microsandbox/blob/v0.7.8/sdk/rust/lib/backend/local/snapshot/restore.rs#L486-L498), [hard-link then fast-copy fallback](https://github.com/superradcompany/microsandbox/blob/v0.7.8/sdk/rust/lib/backend/local/snapshot/create.rs#L1981-L2004), [owned inventory selected during disk restore](https://github.com/superradcompany/microsandbox/blob/v0.7.8/sdk/rust/lib/sandbox/builder.rs#L2131-L2149).
- **S5:** [Full snapshots and CoW memory](https://github.com/superradcompany/microsandbox/blob/v0.7.8/docs/sandboxes/snapshots.mdx#L166-L375), [forking and direct disk-image attachment semantics](https://github.com/superradcompany/microsandbox/blob/v0.7.8/docs/sandboxes/snapshots.mdx#L517-L634).
- **C1:** [Sparse-aware fast-copy algorithm and reflink fallback](https://github.com/superradcompany/microsandbox/blob/v0.7.8/crates/utils/lib/copy.rs#L1-L160).
- **I1:** [OCI root layer caching and pinning](https://github.com/superradcompany/microsandbox/blob/v0.7.8/docs/images/overview.mdx#L7-L56), [main local-image import guide, explicitly separate from Docker's store](https://github.com/superradcompany/microsandbox/blob/e52668369899e840223b5b8f2bd234f9d94d113d/docs/how-to/docker/local-images.mdx).
- **N1:** [Ordered native policy and port-scoped group rules](https://github.com/superradcompany/microsandbox/blob/v0.7.8/docs/networking/overview.mdx#L150-L238).
- **N2:** [Host alias and host-relative access](https://github.com/superradcompany/microsandbox/blob/v0.7.8/docs/networking/overview.mdx#L448-L458), [gateway TCP loopback rewrite](https://github.com/superradcompany/microsandbox/blob/v0.7.8/crates/network/lib/engine/netstack/poll.rs#L888-L905).
- **N3:** [Protected ranges and guest-vs-host localhost](https://github.com/superradcompany/microsandbox/blob/v0.7.8/docs/security/network.mdx#L20-L34), [operator multi-tenant isolation floor](https://github.com/superradcompany/microsandbox/blob/v0.7.8/docs/networking/overview.mdx#L52-L56).
- **D1:** [Docker image save](https://docs.docker.com/reference/cli/docker/image/save/) (current official reference; archives, not registry-cache details).
- **D2:** [Docker image load](https://docs.docker.com/reference/cli/docker/image/load/) (current official reference).
