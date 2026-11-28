FROM opencode-sandbox:base

# renovate: datasource=npm depName=@opencode/cli
ARG OPENCODE_VERSION=2.4.2

ENV PATH=/root/.opencode/bin:$PATH

RUN curl -fsSL https://opencode.ai/v2/install -o /tmp/install-opencode.sh \
    && bash /tmp/install-opencode.sh --version "${OPENCODE_VERSION}" --no-modify-path \
    && rm /tmp/install-opencode.sh \
    && opencode --version
