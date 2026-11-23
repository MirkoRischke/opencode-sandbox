# Custom VM images

Bring an image with your project's tools already installed so every new VM
starts with the same environment. The default image is
`opencode-sandbox:opencode`. Docker is required on the host.

1. [Choose an image](#choose-an-image)
2. [Include the required tools](#include-the-required-tools)
3. [Apply an image change](#apply-an-image-change)

## Choose an image

Set `vm.image` in global `config.toml` or an approved `.opencode-sandbox.toml`:

```toml
[vm]
image = "my-custom-image:tag"
```

- **Local image:** build it first, for example with
  `docker build -t my-custom-image:tag .`.
- **Registry image:** use a reference such as `ghcr.io/you/dev-sandbox:v1`.
  For a private registry, run `docker login` on the host first.

If the tag exists locally, the launcher uses it. Run `docker pull` yourself to
update a locally cached registry tag.

## Include the required tools

Run `mise run build:base` and use `FROM opencode-sandbox:base` for a starting
image without a coding agent. Or extend a [bundled agent image](coding-agents.md).

Include Bash, Git, mise, the [startup executable](coding-agents.md), and a
system CA trust store for HTTPS. Add `gh` for [GitHub access](github.md), the
AWS CLI for [AWS role access](aws.md), or `dockerd` for
[guest Docker](guest-docker.md).

## Apply an image change

Updating the tag or changing `vm.image` does not change existing VMs or
snapshots. [Rebuild the snapshot and recreate the VM](snapshots.md#rebuild-a-baseline)
to use the new image.
