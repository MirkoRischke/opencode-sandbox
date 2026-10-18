# Docker-native image reuse across isolated Microsandbox VMs

Researched 2026-10-18. Scope: stock Docker/Distribution/BuildKit mechanisms;
no custom cache daemon, proxy, or image-store implementation. Documentation and
source inspection only; no infrastructure changed and no runtime validation.

## Conclusions and ranking

1. **Small, known image set: `docker save` once, `docker load` in each new VM.**
   Simplest option under the current network restrictions. A common host archive
   directory is a distribution source, not a live Docker data directory. Each VM
   still imports/unpacks layers into its own ext4 disk. This works for images
   originally obtained from Hub, GHCR, public/private ECR, or a local build. [S1][S2]
2. **Changing/arbitrary Hub images: one stock Distribution pull-through cache**
   plus `registry-mirrors` in guest Docker configuration. This is Docker's
   documented solution for multiple daemons/VMs repeatedly downloading images.
   However, **a cache on the host/private LAN is unreachable under this repo's
   current policy**. A permitted public HTTPS endpoint is necessary unless the
   policy changes; deploying that endpoint is real operational work. [S3][R3]
3. **Known images from several registries: a normal, curated registry** is simpler
   than operating a transparent multi-registry caching system. Pull, tag, push
   selected images once; guests use the curated registry names. Requires the same
   network/TLS/access-control work as a cache and manual refreshes. [S3][S4]
4. **Build-heavy workloads: add native BuildKit external cache separately**,
   preferably with a per-VM `docker-container` builder when registry/local cache
   export is required. This is not a replacement for runtime image-pull caching.
   It adds builder setup and build flags, so do not add it merely to accelerate
   `docker run`/Compose image acquisition. [S5][S6][S7]
5. **Managed ECR pull-through cache** is worth considering only if AWS operations
   are already acceptable. It supports several upstreams including GHCR, unlike
   Docker's `registry-mirrors` setting, but changes image names and adds IAM,
   secrets, authentication renewal, storage costs and lifecycle decisions. [S8][S9]

**Do not share a live `/var/lib/docker` between daemons. No supported secondary
read-only live image store was found for the classic Docker `overlay2` setup.**
This is stronger than guessing from OverlayFS terminology: Docker explicitly
requires a dedicated data directory per daemon, and its current classic layer
store/driver configuration has no additional-image-store setting. [S10][S11][S12]

None of the recommended download-distribution options deduplicates the unpacked
Docker filesystem across private VM disks. Avoiding both downloads **and** repeated
unpacking/physical storage requires a different storage/snapshot arrangement,
not a registry-mirror switch. No such arrangement is asserted here to be supported
by the installed Microsandbox version.

## What is actually being shared?

| Mechanism | Avoids repeat upstream layer transfer? | Still transfers/imports into each fresh VM? | Avoids unpacking in each fresh VM? | Shares build results? | Shares writable runtime state? |
| --- | --- | --- | --- | --- | --- |
| Common save archive + load | Yes, for archived images | Yes, local file reads/import | No | Image output only, not a general BuildKit cache | No |
| Pull-through registry | Yes, for retained cache hits; metadata/refresh requests remain | Yes, registry-to-VM transfer | No | Can cache build base-image pulls; not arbitrary build steps | No |
| Curated ordinary registry | Yes, once explicitly populated | Yes | No | Only if build-cache artifacts are explicitly exported there | No |
| BuildKit local/registry external cache | Sometimes; build-result reuse reduces work | Yes, needed cache content is imported | Not a shared mounted filesystem; materialization remains local | Yes | No |
| Already-populated private Docker disk | Yes, until explicit refresh or missing image | Not for existing local image | Already unpacked | Existing local build cache also persists | Within that VM only |
| One live data-root attached to several daemons | Not a supported solution | — | — | — | Would couple mutable state; reject |

Registry CAS stores a blob once within that registry; it does not eliminate the
separate copies in each guest's `overlay2` store. OverlayFS sharing/page-cache
efficiency documented for containers on one Docker host must not be interpreted
as sharing across independent VM kernels/disks. [S13][S14]

## Repository constraints checked

- [Dockerfile](../Dockerfile), lines 26–42: guest-installed Engine, Buildx and
  Compose; `features.containerd-snapshotter=false` and `storage-driver=overlay2`.
  Docker package versions are not pinned, so the actual installed Engine/Buildx/
  embedded BuildKit versions must be checked before relying on version-specific
  details below. Fresh Engine 29+ installations otherwise default to the
  containerd image store. [R1][S10][S15]
- [docs/config.md](../docs/config.md#docker-inside-the-vm): private VM-owned ext4
  `/var/lib/docker`; stop/start preserves it, VM removal deletes it; no host
  Docker socket. Microsandbox's outer VM image store and guest Docker images are
  separate. Loading a VM template with `msb load` does not populate guest Docker.
  [R2]
- [internal/sandbox/docker.go](../internal/sandbox/docker.go): `dockerd` starts
  without overriding the image's daemon configuration. Thus a custom VM image can
  include Docker's documented mirror configuration without a new cache client.
  There is currently no automatic image archive import. [R4]
- [internal/sandbox/network.go](../internal/sandbox/network.go), lines 24–40:
  host, private, loopback, link-local and metadata destinations are explicitly
  denied before exact-hostname TCP/443 allowances. Merely adding a DNS name that
  resolves to a private cache does not make it reachable. A typical tutorial's
  `localhost:5000`/host-port cache is not usable across these VMs unchanged. [R3]
- [internal/sandbox/sandbox.go](../internal/sandbox/sandbox.go), lines 64–78:
  current mounts are the worktree, external Git common directory when needed,
  and owned Docker disk. **An arbitrary common host archive/cache directory is
  not currently mounted by configuration.** Use an archive in an already mounted
  worktree for a manual trial; a dedicated shared-directory mount would need
  explicit launcher/SDK wiring. Read-only mount support/semantics in the installed
  Microsandbox version were not validated in this Docker-focused research. [R5]

## Live storage: supported sharing versus look-alikes

### Shared live data-root: explicitly ruled out

Docker's daemon documentation says: “Make sure you use a dedicated directory for
each daemon. If two daemons share the same directory, for example an NFS share,
you will experience errors that are difficult to troubleshoot.” The data-root
contains more than image bytes: images, containers, volumes and other daemon
state. The current classic backend keeps it together under `/var/lib/docker`.
[S10]

Changing `data-root` relocates a daemon's own store; it does not turn it into a
multi-daemon store. Mounting the same ext4 disk read-write in multiple VMs is not
made safe by giving each daemon a different socket or PID file. This proposal
should be rejected without trying to repair it with locks/symlinks. [S10][R2]

### Secondary read-only live image store: not a Docker `overlay2` feature

Evidence checked rather than assuming the answer:

- Docker's overlay2 documentation describes read-only image `lowerdir` layers
  **inside its own managed store**, and warns not to manipulate `/var/lib/docker`
  files/directories directly. That is not a documented facility for registering
  another daemon's read-only store. [S14]
- Moby source at commit `3f38b15147b14ce0ce7b11558a1aa9e9f14db103` (current master
  when inspected) has a classic `StoreOptions` with one `Root`, graph driver,
  driver options and identity mapping; layer metadata is derived from that root.
  `overlay2.parseOptions` accepts `overlay2.size` and rejects unknown options;
  there is no secondary image-store path. The driver also writes a `committed`
  marker into a parent layer when creating a child and creates/removes mount
  directories, so “the image's diff is read-only to a running container” does
  not imply “the entire image store can be mounted read-only.” [S11][S12]
- The often-cited `additionalimagestores`/`imagestore` configuration is real, but
  belongs to **containers/storage**, used by a different engine stack, not Docker
  Engine's `overlay2` configuration. It cannot be pasted into `daemon.json`.
  Replacing Docker with that stack is outside the requested scope. [S16]

**Bounded conclusion:** no documented/supported secondary live read-only store
for this classic Docker backend was located; do not claim that every possible
container runtime or future Docker backend lacks this facility. Shared archive
files, registry storage and BuildKit OCI-layout cache exports are supported ways
to share immutable distribution data, but none is a shared live Docker layer store.

Switching to the containerd image store is not a demonstrated solution to
multi-daemon filesystem sharing, and would undo this repo's deliberate storage
choice. Containerd stores compressed and unpacked layers, often increasing disk
usage, and uses `/var/lib/containerd` separately from Docker's data-root. [S15]

## Stock Distribution pull-through cache

### Smallest operational shape

One pinned official `registry` image, one persistent registry storage directory,
one YAML file, TLS on reachable port 443, plus guest `daemon.json` settings.
Distribution can terminate TLS itself; a custom reverse proxy is not required.
Add authentication/access restrictions when necessary; persistence, certificate
renewal and disk monitoring remain operator responsibilities. [S3][S4][S17]

Illustrative **server configuration fragment**, not a deployed or hardened setup
(Distribution v3 documentation uses `/etc/distribution/config.yml`; Docker's older
mirror example says `/etc/docker/registry/config.yml`): [S3][S4][S17]

```yaml
version: 0.1
storage:
  filesystem:
    rootdirectory: /var/lib/registry
  delete:
    enabled: true
http:
  addr: :443
  tls:
    certificate: /certs/fullchain.pem
    key: /certs/privkey.pem
proxy:
  remoteurl: https://registry-1.docker.io
  ttl: 168h
```

Guest configuration must **retain** the existing storage choices: [S3][R1]

```json
{
  "features": {"containerd-snapshotter": false},
  "storage-driver": "overlay2",
  "registry-mirrors": ["https://hub-cache.example.com"]
}
```

Guest policy also needs the cache hostname. Server-side upstream access includes
registry authentication and any blob redirects; guest-side authentication/blob
redirect hosts depend on the selected deployment. A public IP alone does not
justify removing host/private protections. If direct Hub access remains allowed,
Docker has an upstream endpoint after mirror endpoints and may fall back to it;
the mirror setting is not an enforcement boundary. [R3][S18]

### Hub versus GHCR/ECR: two distinct limitations

1. **Docker `registry-mirrors` is a Hub mirror list**, not a per-origin registry
   routing map. Adding three entries does not mean Hub, GHCR and ECR respectively;
   they are alternative mirrors of Hub. Classic pull endpoint source branches on
   the Hub hostname, otherwise using the requested registry directly. [S3][S18]
2. **Distribution proxy supports one upstream per instance**, not one universal
   multi-upstream namespace. Its current documentation explicitly says one upstream
   at a time, while also noting the Docker daemon's Hub-only mirror limitation.
   This is more precise than interpreting Docker's older “Only the central Hub
   can be mirrored” text as a universal prohibition in Distribution itself. [S4]

For a GHCR upstream, a separate stock Distribution instance and explicit image
names such as `ghcr-cache.example.com/org/image:tag` are the plausible stock
shape. Likewise a separate ECR upstream needs a separate instance/endpoint and
appropriate authentication. **End-to-end GHCR/ECR proxy compatibility was not
tested**, so this is not an unconditional recommendation. Public GHCR can be
pulled anonymously; private GHCR normally needs a classic PAT with package access.
Private ECR Docker authorization tokens expire after 12 hours, making a static
`proxy.password` operationally insufficient. Distribution's current configuration
documents an `exec` credential provider, but wiring/testing an ECR helper adds
complexity and is not pursued here. [S4][S9][S17][S19]

Newer Moby source also reads per-registry containerd-style `hosts.toml` in its
`RegistryHosts` resolver; the embedded builder uses that resolver. This does not
change the documented semantics of `registry-mirrors`, and the classic image-pull
endpoint path above remains separate. Treat arbitrary registry remapping through
that newer resolver as **version-/code-path-specific and unverified for the
installed Engine**, not as a supported transparent GHCR fix for this configuration.
[S18][S20][S21]

### Security, rate limits, freshness, offline behavior and retention

- **Credential authority expands:** Docker explicitly warns that every private
  Hub resource readable by `proxy.username/password` becomes available through
  that mirror. A shared account is not per-VM upstream authorization. Do not put
  personal/broad upstream credentials behind an anonymous mirror. The same design
  risk applies to a GHCR/ECR upstream using a shared identity. Cached private
  bytes remain sensitive even after upstream credentials are revoked. [S3][S4]
- Registry authentication is separate from upstream authentication. Stock
  `htpasswd` authenticates users, not each upstream user's original repository
  entitlements; it should not be mistaken for fine-grained tenant authorization.
  Avoid mixing trust groups. Authentication of a transparent daemon mirror with
  a distinct downstream credential must be validated with the actual client;
  this report does not assume `docker login cache-host` alone solves that flow.
  [S17][S18]
- **Rates:** Hub mirrors remain subject to Docker's fair-use policy. At research
  time Hub lists 100 unauthenticated pulls per six hours per IPv4/IPv6 /64,
  200 for authenticated Personal, and unlimited authenticated Pro/Team/Business
  pulls subject to fair use; abuse limits apply independently. A cache aggregates
  misses/refresh traffic behind its own IP/account. Do not promise that cached
  layer bytes eliminate manifest checks or all rate accounting. These limits are
  policy data and can change. [S3][S22]
- **Mutable tags:** Distribution checks upstream when pulling by tag. Pin digests
  for reproducibility; that does not prevent eviction. In contrast, an already
  loaded local image used by `docker run` with default `--pull=missing` need not
  refresh its tag at all. An explicit `docker pull`/`--pull=always` changes that
  behavior. [S3][S23]
- **Offline is conditional, not impossible:** Distribution v3.0.0 tag service
  tries upstream first, then falls back to its local tag association on failure;
  manifest and blob services try local content first. Thus a completely warmed,
  retained image can be served during upstream outage, including by a previously
  cached tag. A missing platform manifest, missing blob, expired entry or changed
  tag pointing at incomplete content can still fail; upstream attempts can add
  latency. This is source-observed behavior, not an air-gap completeness guarantee.
  A curated registry or complete save archive is clearer for deliberate offline
  distribution. [S24][S25][S26]
- **Retention:** configure `storage.delete.enabled=true` for cache expiry cleanup;
  use `filesystem` storage as the official mirror recipe recommends. v3.0.0 source
  defaults proxy TTL to seven days and treats non-positive TTL as disabled expiry.
  TTL is not a byte quota/LRU capacity policy; disabling it exchanges fewer misses
  for unbounded growth. Pin/test the chosen release before using special values.
  Manual registry garbage collection is a separate operation and requires the
  registry stopped or read-only to avoid corrupting uploads. [S3][S17][S27][S13]
- **Concurrent cold misses:** do not promise exactly one upstream transfer per
  blob. Docker's mirror page says a single cache suppresses duplicates, but
  Distribution's current recipe warns concurrent in-flight pulls make multiple
  requests, and v3.0.0 blob source sends an already-in-flight request directly to
  upstream. Warm-cache reuse is the defensible claim. [S3][S4][S26]
- A proxy-mode registry rejects image pushes in the inspected source. It is not
  also the place to export BuildKit cache or push internal output images. Use an
  ordinary writable registry for those artifacts. [S25][S26]

## `docker save` / `docker load`: the low-operations fallback

Illustrative manual workflow, performed only when chosen by the operator: [S1][S2]

```sh
# Host or another trusted producer; pull/authenticate there as appropriate.
docker pull alpine:3.22
docker pull ghcr.io/ORG/IMAGE:TAG
docker save -o images.tar alpine:3.22 ghcr.io/ORG/IMAGE:TAG

# Each fresh VM, after its private daemon is ready and archive is accessible.
docker load -i /shared-images/images.tar
docker run --pull=never --rm alpine:3.22 true
```

`/shared-images` is illustrative, **not an existing launcher mount**. A single
archive may contain several images and their parents/tags; `load` accepts common
compression formats and restores tags. Prefer a deliberately selected platform;
classic Docker is not a multi-platform image store. `save/load --platform` needs
API 1.48+, and can only select platform content actually available. [S1][S2][S15]
If the producing host uses a newer containerd store with a lazy remote snapshotter,
ensure the archive is complete: Docker warns that saving lazily pulled images can
fail or silently produce incomplete archives. This caveat concerns the producer,
even though these consuming VMs deliberately use classic overlay2. [S15]

Operational caveats:

- Publish versioned, complete archives from a trusted producer; let guests read
  but not overwrite the common source. A writable shared archive/cache is a
  cross-VM poisoning/deletion channel even when live Docker disks stay isolated.
  Reading a private image archive grants access to that image's contents without
  consulting its original registry authorization. This is a security consequence
  of distributing the image bytes, not an additional registry feature.
- No online daemon/service, TLS endpoint, registry auth inside each guest, or
  upstream rate check is needed just to load. However, every new VM must read the
  archive and register/extract missing layers locally. Moby's classic loader
  reuses a layer chain already present in that daemon, otherwise loads it; it does
  not reference the archive as its live filesystem. [S2][S28]
- Refresh is manual; tags in an archive are a snapshot. Explicit pull policies,
  Compose settings and build metadata resolution may still contact the registry.
  `docker run --pull=never` makes the intended offline runtime behavior explicit.
  Loading image output is not importing a full BuildKit cache database. [S23][S5]
- **Digest caveat for classic storage:** the inspected classic tar manifest
  records `RepoTags`, not `RepoDigests`; the loader restores tags with `AddTag`.
  Do not assume `docker load` restores a `registry/name@sha256:...` reference just
  because the source image was pulled by that digest. Validate the exact consumer
  reference (especially digest-pinned Compose/test fixtures); an image ID/local
  tag is not the same identifier as a registry manifest digest. [S28][S29]
- No automatic retention/refresh/loader is supplied by Docker. Keep this manual
  or use only a small explicit initialization step if later requested; do not
  turn it into a home-grown content-addressed synchronization system.

### Preloading into the VM image: why a Dockerfile alone is insufficient

Embedding a save archive at a non-mounted path such as `/opt/image-seed/images.tar`
in the outer VM image, then loading after guest `dockerd` starts, is conceptually
the same fallback. It trades a common host archive directory for a bigger VM
template and template refreshes; each VM still unpacks into its private Docker
disk. It is not automatic with the current launcher. [R1][R4][S2]

Pre-populating `/var/lib/docker` in that outer image is **not equivalent**: the
launcher mounts a separate owned disk there. Do not assume image-root seed files
appear in that disk or that Microsandbox performs Docker-style empty-volume
copy-up. This behavior was not runtime-tested; use an archive outside that mount
and an explicit load instead of depending on undocumented seed propagation.
Likewise, a stopped disk snapshot/clone containing a fully initialized daemon
would be a different provisioning strategy, not shared live storage; compatibility,
clean shutdown, clone support and physical CoW savings need separate Microsandbox
verification. [R2][R5]

## Builds: mirrors and external cache solve different problems

### Pull mirror affects source-image acquisition, not all build work

The default Docker builder's image-source wiring uses the daemon's registry-host
resolver, whose legacy mirror configuration applies to Hub. It can benefit from
the Hub cache for Dockerfile base-image/front-end acquisition, but this alone does
not share results of `RUN`, compilation, package-manager downloads or arbitrary
HTTP requests across VMs. A cache hit on a build step can avoid re-running that
step; an image mirror cannot invent that step's cache record. [S20][S21][S5]

A separate `docker-container` builder has its own BuildKit daemon/state volume.
Configure its mirrors explicitly in `buildkitd.toml`; do not assume guest
`daemon.json` transparently configures that builder's base-image pulls. The
`moby/buildkit` container image itself is acquired through guest Docker first.
BuildKit TLS trust must also work inside that container; this repo warns that
containers do not inherit Microsandbox's injected guest CA automatically. [S6][S7][R2]

```toml
[registry."docker.io"]
  mirrors = ["hub-cache.example.com"]
```

```sh
docker buildx create --name shared-cache-builder --use \
  --driver docker-container --buildkitd-config ./buildkitd.toml
```

This creates a builder **inside each VM**. No shared remote builder or host Docker
socket is proposed. BuildKit supports per-registry mirror entries, unlike the
daemon's single Hub mirror list, but each upstream cache still needs a suitable
server/configuration and permission to reach it. [S6][S30]

### External registry/local build cache

For build results use native export/import, for example: [S5][S31]

```sh
docker buildx build --builder shared-cache-builder --load -t app:dev \
  --cache-from type=registry,ref=registry.example.com/team/app-cache:main \
  --cache-to type=registry,ref=registry.example.com/team/app-cache:branch,mode=max .
```

Or a shared **export directory**, not a shared live BuildKit root: [S32]

```sh
docker buildx build --builder shared-cache-builder --load -t app:dev \
  --cache-from type=local,src=/shared-build-cache/main \
  --cache-to type=local,dest=/shared-build-cache/branch,mode=max .
```

- With this repo's classic store, use `docker-container` for registry/local
  **cache export**; current Docker documentation requires the containerd image
  store for those backends on the default `docker` driver. Do not casually enable
  containerd just to follow an example. [S5][S31][S32]
- **Important exception/precision:** inline cache remains a smaller option when
  already publishing output images. Docker documents `BUILDKIT_INLINE_CACHE=1`
  and import from the output image. Inspected classic Moby builder source has
  inline exporter plus registry-inline importer, while separate registry/local
  exporters exist in its snapshotter path. Therefore the broad documentation
  statement about external backends should not be read as “classic Docker has no
  cross-build cache at all.” Validate installed versions/commands; prefer inline
  first if it meets the build's needs. [S21][S33]
- Registry/local exports are explicit (`--cache-to` / `--cache-from`); runtime
  `docker pull`/`docker run` does not consult them. `mode=max` can export
  intermediate-stage records; it does not create a universal registry mirror.
  Separate-builder output needs `--load` for the guest Docker image store or
  `--push` to publish it. [S5][S7]
- Cache destinations need ownership/scoping. Repeated writes to one cache
  location overwrite its cache manifest; do not have untrusted VMs race on the
  same branch/ref/directory. Prefer trusted read-only baseline imports and
  separate export destinations. Local exports keep old blobs by default and can
  grow. Current local-cache docs add `reset=true` in **Buildx 0.35.0+**; do not
  assume that option exists in the installed plugin or is safe with live readers.
  No cross-VM concurrent-writer guarantee was verified. [S5][S32]
- Build cache may contain source/private output. Use BuildKit secrets rather
  than `COPY`/`ARG` for credentials and do not grant cache write access across
  unrelated trust domains. [S5]

## Ordinary registry and managed ECR alternatives

**Ordinary curated registry:** Docker itself recommends manually pulling/pushing
a well-delimited image set instead of a pull-through cache. It works with images
from different origins after retagging, offers a deliberately retained offline
catalog, and can also hold image outputs/BuildKit cache in separate repositories.
It does not preserve upstream names automatically, auto-refresh tags, or eliminate
per-VM transfer/unpack. Keep clients unable to replace trusted baseline images;
retention/manual GC still needs operations. [S3][S13][S31]

**ECR managed pull-through:** AWS currently documents upstream support for public
ECR, Kubernetes registry, Quay, Hub, GHCR, GitLab SaaS, Azure Container Registry,
Chainguard and private ECR, with different auth requirements. Consumers pull via
ECR private registry URI/prefixes, not unchanged `ghcr.io/...` names. AWS checks
cached tags on a 24-hour validation window and serves the last cached image if
refresh fails; tag immutability prevents same-tag refresh. IAM/secrets determine
who can cause imports and who can read cached contents; upstream authority must
not exceed intended downstream readers. ECR repository lifecycle rules, IAM,
costs and 12-hour Docker authentication tokens make this more operationally
involved than an archive, although no custom caching service is needed. [S8][S9]

Neither ECR nor a public remote registry avoids transferring the retained bytes
over the guest's network on each fresh VM. Their advantage is avoiding repeated
origin downloads/credentials and improving availability, not local-host physical
disk deduplication. ECR endpoint/API/blob destinations must fit this repo's exact
hostname policy; the required full host set was not established here. [R3][S8]

## Remaining decisions / verification before implementation

1. Is the goal upstream bandwidth/rate reduction, startup/unpack latency, physical
   disk savings, faster builds, or all four? The simple mechanisms above do not
   solve all four simultaneously.
2. What proportion is Hub vs GHCR vs ECR; how many known images/platforms; which
   are private and which consumers are mutually trusted?
3. Must fresh VMs work completely offline? Are digest-pinned runtime references
   mandatory? That changes the archive validation requirements.
4. Is a permitted public HTTPS cache acceptable, or must existing host/private
   network denial remain unchanged? With unchanged denial and no remote service,
   an accessible archive is the smallest candidate.
5. What are installed Engine, Buildx, embedded BuildKit and Distribution versions?
   Validate mirror auth/fallback, complete offline pulls and cache-export support
   on those exact versions. Source commit observations are not a release promise.
6. Does the installed Microsandbox provide the required read-only shared-directory
   mount or clean per-VM disk clone/snapshot workflow? This note intentionally
   does not infer either feature from Docker's storage behavior.
7. Who publishes/refreshes image archives or curated registry contents, and what
   bounds their retained size? Avoid replacing a small manual process with a
   custom distribution system.

## Sources

Repository sources (local primary evidence):

- [R1] [Dockerfile](../Dockerfile), especially lines 26–42.
- [R2] [docs/config.md](../docs/config.md#docker-inside-the-vm), Docker/custom-image/network sections.
- [R3] [internal/sandbox/network.go](../internal/sandbox/network.go), `networkPolicy`.
- [R4] [internal/sandbox/docker.go](../internal/sandbox/docker.go), daemon initialization.
- [R5] [internal/sandbox/sandbox.go](../internal/sandbox/sandbox.go), mount construction.

External primary documentation/source (retrieved on research date; live docs may change):

- [S1] Docker image save: <https://docs.docker.com/reference/cli/docker/image/save/>
- [S2] Docker image load: <https://docs.docker.com/reference/cli/docker/image/load/>
- [S3] Docker Hub mirror recipe: <https://docs.docker.com/docker-hub/image-library/mirror/>
- [S4] Distribution mirror recipe, including one-upstream and daemon limitations: <https://distribution.github.io/distribution/recipes/mirror/>
- [S5] Docker build cache backends: <https://docs.docker.com/build/cache/backends/>
- [S6] BuildKit mirrors/certificates: <https://docs.docker.com/build/buildkit/configure/>
- [S7] Docker-container builder driver: <https://docs.docker.com/build/builders/drivers/docker-container/>
- [S8] AWS ECR pull-through cache, including considerations: <https://docs.aws.amazon.com/AmazonECR/latest/userguide/pull-through-cache.html>
- [S9] AWS ECR authentication/token lifetime: <https://docs.aws.amazon.com/AmazonECR/latest/userguide/registry_auth.html>
- [S10] Docker daemon data directory and dedicated-directory warning: <https://docs.docker.com/engine/daemon/#daemon-data-directory>
- [S11] Moby classic layer store, pinned source: <https://github.com/moby/moby/blob/3f38b15147b14ce0ce7b11558a1aa9e9f14db103/daemon/internal/layer/layer_store.go>
- [S12] Moby overlay2 option parser and layer/mount lifecycle, pinned source: <https://github.com/moby/moby/blob/3f38b15147b14ce0ce7b11558a1aa9e9f14db103/daemon/graphdriver/overlay2/overlay.go>
- [S13] Distribution CAS/GC and stop/read-only requirement: <https://distribution.github.io/distribution/about/garbage-collection/>
- [S14] Docker overlay2 layers and management warning: <https://docs.docker.com/engine/storage/drivers/overlayfs-driver/>
- [S15] Docker containerd image store, defaults, disk use, classic limitations: <https://docs.docker.com/engine/storage/containerd/>
- [S16] containers/storage (not Docker) `additionalimagestores`: <https://github.com/containers/storage/blob/main/docs/containers-storage.conf.5.md>
- [S17] Distribution configuration, TLS/auth/proxy/exec/maintenance: <https://distribution.github.io/distribution/about/configuration/>
- [S18] Moby classic registry endpoint selection, pinned source: <https://github.com/moby/moby/blob/3f38b15147b14ce0ce7b11558a1aa9e9f14db103/daemon/pkg/registry/service_v2.go>
- [S19] GitHub Container Registry access/authentication: <https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry>
- [S20] Moby registry-host resolver and legacy Hub mirrors, pinned source: <https://github.com/moby/moby/blob/3f38b15147b14ce0ce7b11558a1aa9e9f14db103/daemon/hosts.go>
- [S21] Moby classic versus snapshotter BuildKit controller configuration, pinned source: <https://github.com/moby/moby/blob/3f38b15147b14ce0ce7b11558a1aa9e9f14db103/daemon/internal/builder-next/controller.go>
- [S22] Docker Hub current limits/fair use: <https://docs.docker.com/docker-hub/usage/>
- [S23] Docker run pull policy: <https://docs.docker.com/reference/cli/docker/container/run/#pull>
- [S24] Distribution **v3.0.0** tag fallback: <https://github.com/distribution/distribution/blob/v3.0.0/registry/proxy/proxytagservice.go>
- [S25] Distribution **v3.0.0** local-first manifests/rejected pushes: <https://github.com/distribution/distribution/blob/v3.0.0/registry/proxy/proxymanifeststore.go>
- [S26] Distribution **v3.0.0** local-first blobs/in-flight upstream behavior: <https://github.com/distribution/distribution/blob/v3.0.0/registry/proxy/proxyblobstore.go>
- [S27] Distribution **v3.0.0** TTL/default/expiry implementation: <https://github.com/distribution/distribution/blob/v3.0.0/registry/proxy/proxyregistry.go>
- [S28] Moby classic image loader, pinned source: <https://github.com/moby/moby/blob/3f38b15147b14ce0ce7b11558a1aa9e9f14db103/daemon/internal/image/tarexport/load.go>
- [S29] Moby classic archive manifest fields, pinned source: <https://github.com/moby/moby/blob/3f38b15147b14ce0ce7b11558a1aa9e9f14db103/daemon/internal/image/tarexport/tarexport.go>
- [S30] BuildKit per-registry configuration: <https://docs.docker.com/build/buildkit/toml-configuration/>
- [S31] Registry build cache backend: <https://docs.docker.com/build/cache/backends/registry/>
- [S32] Local build cache backend and versioned cleanup options: <https://docs.docker.com/build/cache/backends/local/>
- [S33] Inline build cache: <https://docs.docker.com/build/cache/backends/inline/>

Research tooling note: Context7 Docker documentation lookup was used first, then
claims were checked against the primary URLs above. Later Context7 requests hit
its monthly quota; AWS/GitHub/Distribution facts were read from their official
documentation/source directly instead.
