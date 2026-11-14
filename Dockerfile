ARG MISE_VERSION=2026.11.8
# renovate: datasource=npm depName=@opencode/cli
ARG OPENCODE_VERSION=2.3.0

FROM ghcr.io/jdx/mise:${MISE_VERSION} AS mise

FROM debian:13-slim

# Runtime prerequisites and common agent development/network debugging tools.
# Keep Debian package updates available when rebuilding the image.
# hadolint ignore=DL3008
RUN apt-get update && apt-get install -y --no-install-recommends \
    awscli \
    bind9-dnsutils \
    build-essential \
    ca-certificates \
    curl \
    file \
    git \
    gh \
    iproute2 \
    iputils-ping \
    jq \
    less \
    libatomic1 \
    lsof \
    netcat-openbsd \
    patch \
    procps \
    ripgrep \
    tree \
    util-linux \
    unzip \
    vim \
    xz-utils \
    && rm -rf /var/lib/apt/lists/*

# Docker runs inside the VM, not against the host's Docker socket. Suppress
# package service startup: Microsandbox's agent, not systemd, is guest PID 1.
# hadolint ignore=DL3008
RUN install -m 0755 -d /etc/apt/keyrings \
    && curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc \
    && . /etc/os-release \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian ${VERSION_CODENAME} stable" > /etc/apt/sources.list.d/docker.list \
    && printf '#!/bin/sh\nexit 101\n' > /usr/sbin/policy-rc.d \
    && chmod +x /usr/sbin/policy-rc.d \
    && apt-get update && apt-get install -y --no-install-recommends \
        docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin \
    && rm /usr/sbin/policy-rc.d \
    && rm -rf /var/lib/apt/lists/*

# Keep all image layers on the owned ext4 disk at /var/lib/docker. Engine 29+
# otherwise defaults to the containerd image store under /var/lib/containerd.
RUN mkdir -p /etc/docker \
    && printf '%s\n' '{"features":{"containerd-snapshotter":false},"storage-driver":"overlay2"}' > /etc/docker/daemon.json

COPY --from=mise /usr/local/bin/mise /usr/local/bin/mise

ENV MISE_DATA_DIR=/mise/data \
    MISE_CONFIG_DIR=/mise/config \
    MISE_CACHE_DIR=/mise/cache \
    MISE_STATE_DIR=/mise/state \
    PATH=/mise/data/shims:/root/.opencode/bin:$PATH \
    SHELL=/bin/bash \
    NODE_USE_SYSTEM_CA=1

ARG OPENCODE_VERSION
# Install the standalone binary; the official installer selects the platform.
RUN curl -fsSL https://opencode.ai/v2/install -o /tmp/install-opencode.sh \
    && bash /tmp/install-opencode.sh --version "${OPENCODE_VERSION}" --no-modify-path \
    && rm /tmp/install-opencode.sh \
    && mkdir -p /mise/data/shims \
    && opencode --version

# AWS CLI bundles its own CA file; use the trust store updated by Microsandbox.
ENV AWS_CA_BUNDLE=/etc/ssl/certs/ca-certificates.crt

WORKDIR /workspace
CMD ["/bin/bash"]
