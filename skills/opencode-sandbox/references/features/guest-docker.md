# Docker inside the VM

Run project dependencies or build images inside the VM without giving your
agent access to the host Docker socket. The supplied images include Docker,
Buildx, and Compose.

1. [Run containers](#run-containers)
2. [Set disk space](#set-disk-space)
3. [Connect containers to the network](#connect-containers-to-the-network)

## Run containers

Docker starts automatically when installed. Run commands like
`docker compose up` or `docker build` inside the VM. If startup fails, the
launcher shows recent Docker logs.

## Set disk space

Each VM has a private Docker disk. Images, containers, named volumes, and
build cache survive a restart but are lost when you remove the VM.

To increase the default of 10240 MiB, set `vm.docker_disk_mib` in your config,
then [rebuild the snapshot and recreate the VM](snapshots.md#rebuild-a-baseline).

## Connect containers to the network

- **Open a container service on the host:** publish its port to the VM
  (for example, with `-p 8080:80`), then add a
  [host-to-VM port mapping](networking.md#host-to-vm).
- **Pull images or install packages:** allow the registry, sign-in, and
  download hosts your builds need, or pull images during
  [snapshot preparation](snapshots.md). Registries are not allowed by default.
- **Fix HTTPS certificate errors:** containers don't inherit VM certificates.
  If a container needs HTTPS through the sandbox's inspected network, make
  it trust the sandbox CA.
