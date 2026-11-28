FROM opencode-sandbox:base

# renovate: datasource=github-releases depName=github/copilot-cli
ARG COPILOT_VERSION=1.0.117

RUN curl -fsSL "https://raw.githubusercontent.com/github/copilot-cli/v${COPILOT_VERSION}/install.sh" -o /tmp/install-copilot.sh \
    && VERSION="${COPILOT_VERSION}" PREFIX=/usr/local bash /tmp/install-copilot.sh \
    && rm /tmp/install-copilot.sh \
    && copilot --version
