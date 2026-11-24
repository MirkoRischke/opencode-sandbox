FROM node:24.22.3-trixie-slim
RUN apt-get update && apt-get install -y --no-install-recommends git ripgrep curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /opt/eval
COPY node_modules ./node_modules
COPY package.json *.ts ./
COPY opencode-sandbox /usr/local/bin/opencode-sandbox
ENV EVAL_CONTAINER=1 NODE_USE_SYSTEM_CA=1
CMD ["node", "run.ts"]
