# Command-line tools for a small coding-agent image

Research date: **2026-10-19**. Recommendation, not an implementation change.

## Bottom line

Keep `debian:13-slim`; borrow the **OS utility layer**, not an entire CI runner.
The most useful small additions are **`jq ripgrep file procps patch`**; add
**`openssh-client`** if SSH Git URLs/submodules are supported. `fd-find`, `less`,
`tree`, and `rsync` are conveniences, not prerequisites. Build toolchains,
debuggers, browsers, database clients, and extra cloud CLIs should be opt-in.

This repo already installs `awscli ca-certificates curl git gh libatomic1
util-linux unzip xz-utils`, Docker Engine/CLI, Buildx, Compose, mise, Node 24, and
OpenCode **2.0.30** ([Dockerfile](../../Dockerfile)). Project initialization already
trusts/installs project mise tools ([configuration](../config/README.md#snapshots),
lines 272–275 at research time). Therefore, do not duplicate every project's
runtime/version manager in the image. Installed dependencies may already supply
some commands: distinguish “not explicitly installed” from “absent.”

## What the official images actually contain

Representative variants only; this is not a promise about every architecture,
tag, or release. Lists below are selected, explicitly evidenced packages/tools,
not complete dependency inventories. GitHub's runner is a **VM**, whereas the
other examples are container image recipes. Source recipes are not proof of the
exact contents of an independently pulled registry digest.

| Platform / inspected variant | Core utilities explicitly evidenced | Beyond a small OS baseline | Evidence |
| --- | --- | --- | --- |
| GitHub Actions **Ubuntu 24.04 x64**, image `20261012.334.1` (OS `24.04.5`) | Git, `gh`, curl, wget, jq, file, findutils, tar, unzip, zip, xz-utils, openssh-client, rsync, tree, dnsutils, iproute2, iputils-ping, netcat, shellcheck | Multiple compiler/runtime versions; CMake, Ninja; AWS/Azure/Google CLIs; Docker/Buildx/Compose, Kubernetes tools; browsers/drivers; database servers | Published software manifest [G] |
| AWS CodeBuild **Ubuntu standard 8.0**, Ubuntu 24.04, and **standard 7.0**, Ubuntu 22.04 | Git, `gh`, curl, wget, jq, file, findutils, less, patch, procps, openssh-client, rsync, tar, unzip, zip, xz-utils, dnsutils, iproute2, net-tools, shellcheck; 8.0 specifies **netcat-openbsd**, 7.0 spells it **netcat** | build-essential, pkg-config, development headers; multiple runtimes; AWS CLI v2/SAM/EKS tools; Docker/Buildx/Compose; browsers/drivers | Official Dockerfiles, `core`, `tools`, `runtimes` stages [A8], [A7] |
| Microsoft devcontainers **base-debian**, default **trixie** | curl, wget, CA certificates, GnuPG from buildpack-deps curl layer; Git feature; common-utils adds jq, less, tree, procps, lsof, htop, psmisc, openssh-client, rsync, unzip, bzip2, xz-utils, zip, iproute2, net-tools, strace, editors, sudo, zsh | Interactive developer comforts, not the universal language/toolchain stack | Dockerfile **plus feature configuration**, common-utils Debian installer, inherited curl Dockerfile [DB], [DC], [CU], [BP] |
| Microsoft devcontainers **universal**, Ubuntu **noble / 24.04** source recipe | common-utils layer above; Dockerfile explicitly adds jq, curl, make, unzip, zip, rsync, sqlite3, gettext, moreutils, inotify-tools | build-essential, CMake, Clang/LLVM/LLDB, GDB, Valgrind, cppcheck, DB headers; features request Git/Git LFS/`gh`, Docker-in-Docker, Kubernetes tools, .NET, Node, Python, PHP, Ruby, Java, Go, Conda | Dockerfile **and** feature configuration [UD], [UC] |
| OpenAI **codex-universal**, Ubuntu **24.04** source recipe | Git/Git LFS, curl, wget, jq, **ripgrep**, **fd-find**, openssh-client, rsync, dnsutils, iputils-ping, **netcat-openbsd**, sqlite3, moreutils, inotify-tools, unzip, zip, xz-utils | build-essential, pkg-config, many headers, CMake/Ninja/LLVM, ccache, language linters, and many versions of numerous runtimes | Official Dockerfile [O] |

**Interpretation:** jq, transfer/archive tools, Git/SSH, process inspection, and
basic filesystem/network diagnostics recur. Modern search tools are explicit in
the agent-specific Codex image, but `ripgrep`/`fd-find` are **not listed in the
selected GitHub manifest or the inspected CodeBuild/common-utils install lists**;
this is not evidence of their absence from all resulting images. Broad CI and
“universal” images optimize for many unrelated projects, not minimum size. Their
compiler suites and cloud/browser inventories are not a sensible minimum for
this sandbox. [G][A8][A7][CU][UC][O]

## Recommended tiers for this repo

These are engineering recommendations based on the inventories, not upstream
requirements. No image-size or startup benchmark was performed.

| Tier | Debian packages | Reason / boundary |
| --- | --- | --- |
| Small default additions | `jq ripgrep file procps patch` | JSON inspection without ad-hoc scripts; predictable shell `rg`; identify unknown files; `ps`/`pgrep`/`pkill`; apply ordinary patch files. jq/file recur in CI, procps/patch are explicit in CodeBuild, and ripgrep in Codex. [G][A8][CU][O] |
| Conditional default | `openssh-client` | Needed for SSH-based Git/remotes/submodules; unnecessary for strictly HTTPS-only operation. No SSH server needed. All compared recipes/manifests evidence the client. [G][A8][CU][O] |
| Small conveniences | `fd-find less tree rsync` | Faster filename discovery, paging, directory overview, incremental copies. `find` and existing agent glob/read tools remain adequate without fd/tree. [CU][O][FD] |
| Network/debug opt-in | `iproute2 bind9-dnsutils netcat-openbsd iputils-ping lsof strace` | Socket/routes, DNS, TCP checks, ICMP, open files, syscall traces. Useful when diagnosing services; not necessary for every edit/test loop. [G][A8][CU][DNS][NC] |
| Native-build opt-in | `build-essential pkg-config`, then project-specific `*-dev`; `cmake ninja-build autoconf automake libtool` only when needed | Native extensions and source builds need system compilers/headers even if mise supplies a language runtime. Avoid the universal-image header inventory. [A8][UD][O][PKG] |
| Project/debug opt-in | Python/Go/Rust/JDK/.NET runtimes; GDB/LLDB/Valgrind; browser dependencies; sqlite3/PostgreSQL clients; shellcheck; yq | Runtime versions belong in project mise configuration; debuggers, databases, browsers, and linters depend on the task. No evidence supports making them all mandatory. [G][UD][UC][O] |

Keep existing `aws`/`gh`/Docker because this sandbox already chooses them; their
presence elsewhere is corroboration, not a reason to add Azure CLI, gcloud,
kubectl, Helm, or Terraform. curl is already available: wget is a compatibility
convenience, not a second mandatory HTTP client. Do not add alternate shells,
sudo, SSH servers, or interactive UI tools solely to mirror devcontainers.

## OpenCode 2.0.30: is distro ripgrep required?

**No: the pinned V2 source implements a runtime fallback download.** This is
verified against tag `v2.0.30`, commit
`f16ef9ad8878279ea2be3cfab7baeed97aef61aa`, not V1 documentation. In
[`packages/core/src/ripgrep/binary.ts`][RG], `RipgrepBinary`:

1. Resolves a system `rg` (with OpenCode's global bin directory as an additional
   search location), then checks a cached `global.bin/rg`.
2. Otherwise downloads **ripgrep 15.1.0** from BurntSushi's GitHub releases for
   the current platform, extracts it using `tar -xzf` on Linux, and caches the
   executable under OpenCode's global bin directory.

Therefore `ripgrep` is **not a hard distro prerequisite for that code path**.
Nevertheless, installing it in the image avoids a first-use release download
(important with restricted networking), provides `rg` predictably to ordinary
shell commands, and avoids dependence on the OpenCode-managed cache surviving.
The fallback is a download/cache mechanism, not evidence that `rg` is embedded
in the OpenCode executable. This finding is source inspection, not an offline
execution test. Debian's currently indexed trixie ripgrep is **14.1.1**, so choosing
the distro package also chooses an older version than OpenCode's fallback;
OpenCode prefers the installed executable rather than upgrading it. [RG][PKG]

## Debian package / executable pitfalls

- **`ripgrep` → `rg`; `fd-find` → `fdfind`.** For scripts expecting `fd`, use an
  image-level symlink in PATH if that interface is desired; a shell alias does
  not reliably help noninteractive agent subprocesses. Upstream fd documents
  the Debian naming difference explicitly. [FD]
- **DNS:** Ubuntu's manifests/recipes use **`dnsutils`**, but do not blindly copy
  that name into Debian 13. The fetched trixie amd64 main package index lists
  **`bind9-dnsutils`**, not a `dnsutils` package stanza; trixie's `dig` manual is
  owned by `bind9-dnsutils`. Prefer that concrete package for this image. [PKG][DNS]
- **Netcat:** use **`netcat-openbsd`** explicitly and invoke **`nc`**. Avoid
  unspecified `netcat` alternatives; implementations have different options.
  The Debian OpenBSD nc manual documents no `-e`/`-c`; `-w` does not limit a
  listening socket's lifetime. [NC]
- **Process/socket tools:** `procps` supplies process utilities; `util-linux`
  is not a substitute. Use `iproute2` for `ip`/`ss`; don't install legacy
  `net-tools` merely because CI images do. [CU][PKG]
- CA certificate files do not imply the `openssl` command-line package is
  installed; `libssl-dev` is a build dependency, not a minimal TLS diagnostic
  requirement. Likewise, `pkg-config` is a package/interface name, not a
  language runtime. Keep explicit package declarations aligned with commands
  the image intends to guarantee, rather than incidental dependencies.

## Source snapshots and limitations

All sources accessed **2026-10-19**. GitHub links below pin inspected repository
snapshots to make claims auditable; repository commit dates are **not** registry
image publication dates. Devcontainer feature references use moving major tags
(`common-utils:2`, etc.); their source snapshot illustrates the current recipe,
not a resolved feature digest from a particular historical image. CodeBuild
standard tags are updated over time ([AWS image policy][AP]). No image was
pulled and no `dpkg-query` inventory was captured from a running container.
Context7 lookup was attempted but unavailable (monthly quota exceeded); official
source recipes/manifests and Debian's package index/manuals were used directly.

- **[G]** actions/runner-images snapshot `82d1b86` (2026-10-19), [Ubuntu 24.04 software manifest][G].
- **[A8]/[A7]** aws/aws-codebuild-docker-images snapshot `2cc2e02` (2026-09-10), [standard 8.0 Dockerfile][A8], [standard 7.0 Dockerfile][A7].
- **[DB]/[DC]/[UD]/[UC]** devcontainers/images snapshot `d394f01` (2026-10-16): [base Dockerfile][DB], [base features][DC], [universal Dockerfile][UD], [universal features][UC].
- **[CU]** devcontainers/features snapshot `94181c0` (2026-10-09), [common-utils main installer][CU].
- **[BP]** docker-library/buildpack-deps snapshot `fb12875` (2026-09-18), [trixie curl Dockerfile][BP].
- **[O]** openai/codex-universal snapshot `47f4f0e` (2026-05-02), [Dockerfile][O]; source defaults, not a claimed registry tag/digest.
- **[RG]** [OpenCode v2.0.30 ripgrep binary resolver][RG].
- **[PKG]** [Debian trixie main amd64 package index][PKG], fetched/decompressed to verify package stanzas; moving security/point-release contents.
- **[FD]/[DNS]/[NC]** [fd installation documentation][FD], [trixie dig manual][DNS], [trixie nc manual][NC].

[G]: https://github.com/actions/runner-images/blob/82d1b86ad393865982dd29535cb8d3adbc68e41e/images/ubuntu/Ubuntu2404-Readme.md
[A8]: https://github.com/aws/aws-codebuild-docker-images/blob/2cc2e02f70748027447d55b5e82229dd7d69bd70/ubuntu/standard/8.0/Dockerfile
[A7]: https://github.com/aws/aws-codebuild-docker-images/blob/2cc2e02f70748027447d55b5e82229dd7d69bd70/ubuntu/standard/7.0/Dockerfile
[AP]: https://docs.aws.amazon.com/codebuild/latest/userguide/build-env-ref-available.html
[DB]: https://github.com/devcontainers/images/blob/d394f016bd0b329c4563d659a479c8284e4fbe56/src/base-debian/.devcontainer/Dockerfile
[DC]: https://github.com/devcontainers/images/blob/d394f016bd0b329c4563d659a479c8284e4fbe56/src/base-debian/.devcontainer/devcontainer.json
[UD]: https://github.com/devcontainers/images/blob/d394f016bd0b329c4563d659a479c8284e4fbe56/src/universal/.devcontainer/Dockerfile
[UC]: https://github.com/devcontainers/images/blob/d394f016bd0b329c4563d659a479c8284e4fbe56/src/universal/.devcontainer/devcontainer.json
[CU]: https://github.com/devcontainers/features/blob/94181c0188ce13b8c145ab114e887715d45bd982/src/common-utils/main.sh
[BP]: https://github.com/docker-library/buildpack-deps/blob/fb12875aa41571f6288d524576174843fe738995/debian/trixie/curl/Dockerfile
[O]: https://github.com/openai/codex-universal/blob/47f4f0eb5337083e2f610db0d15558932cb4901d/Dockerfile
[RG]: https://github.com/anomalyco/opencode/blob/f16ef9ad8878279ea2be3cfab7baeed97aef61aa/packages/core/src/ripgrep/binary.ts
[PKG]: https://deb.debian.org/debian/dists/trixie/main/binary-amd64/Packages.xz
[FD]: https://github.com/sharkdp/fd#on-debian
[DNS]: https://manpages.debian.org/trixie/bind9-dnsutils/dig.1.en.html
[NC]: https://manpages.debian.org/trixie/netcat-openbsd/nc.1.en.html
