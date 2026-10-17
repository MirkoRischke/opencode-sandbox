ARG MISE_VERSION=2026.10.12
ARG OPENCODE_VERSION=2.0.30

FROM ghcr.io/jdx/mise:${MISE_VERSION} AS mise

FROM debian:13-slim

# Copy only the standalone mise binary; Debian remains the base image.
COPY --from=mise /usr/local/bin/mise /usr/local/bin/mise

# Installation prerequisites, not the optional development/debugging toolset.
# Keep Debian package updates available when rebuilding the image.
# hadolint ignore=DL3008
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    git \
    gh \
    libatomic1 \
    unzip \
    xz-utils \
    && rm -rf /var/lib/apt/lists/*

ENV MISE_DATA_DIR=/mise/data \
    MISE_CONFIG_DIR=/mise/config \
    MISE_CACHE_DIR=/mise/cache \
    MISE_STATE_DIR=/mise/state \
    PATH=/mise/data/shims:$PATH \
    SHELL=/bin/bash \
    NODE_USE_SYSTEM_CA=1

ARG OPENCODE_VERSION
RUN mise use --global node@24 \
    && mise use --global opencode@${OPENCODE_VERSION} \
    && mise exec -- opencode --version

WORKDIR /workspace
CMD ["/bin/bash"]
