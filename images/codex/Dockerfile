FROM opencode-sandbox:base

# renovate: datasource=github-releases depName=openai/codex
ARG CODEX_VERSION=0.171.0

# Keep the installed package separate from runtime state in ~/.codex.
RUN curl -fsSL "https://raw.githubusercontent.com/openai/codex/rust-v${CODEX_VERSION}/scripts/install/install.sh" -o /tmp/install-codex.sh \
    && CODEX_HOME=/opt/codex CODEX_INSTALL_DIR=/usr/local/bin CODEX_NON_INTERACTIVE=1 \
        sh /tmp/install-codex.sh --release "${CODEX_VERSION}" \
    && CODEX_HOME=/opt/codex codex --version \
    && rm /tmp/install-codex.sh \
    && rm -rf /opt/codex/tmp
