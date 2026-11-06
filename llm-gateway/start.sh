#!/usr/bin/env bash
set -euo pipefail

cd -- "$(dirname -- "${BASH_SOURCE[0]}")"

if [[ ! -e .env ]]; then
  master_key="sk-$(openssl rand -hex 32)"
  salt_key="sk-$(openssl rand -hex 32)"
  postgres_password="$(openssl rand -hex 32)"
  (
    umask 077
    set -o noclobber
    printf 'LITELLM_MASTER_KEY=%s\nLITELLM_SALT_KEY=%s\nPOSTGRES_PASSWORD=%s\n' \
      "$master_key" "$salt_key" "$postgres_password" > .env
  )
fi

docker compose up --wait --wait-timeout 180
printf '\nGateway dashboard: http://localhost:4000/ui\nUsername: admin\nPassword: LITELLM_MASTER_KEY in llm-gateway/.env\nChatGPT login: mise run gateway:authenticate\n'
