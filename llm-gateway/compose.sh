#!/usr/bin/env bash
set -euo pipefail

directory="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
compose=(docker compose --project-directory "$directory" --file "$directory/compose.yaml")

# Only startup needs the optional configuration. Stop/exec must still work when
# the user's config has become unreadable or invalid.
if [[ "${1:-}" == up ]]; then
  config_root="${XDG_CONFIG_HOME:-${HOME:?HOME is not defined}/.config}"
  config_file="$config_root/opencode-sandbox/llm-gateway/config.yaml"
  if [[ -e "$config_file" ]]; then
    if [[ ! -f "$config_file" || ! -r "$config_file" ]]; then
      printf 'Gateway config must be a readable regular file: %s\n' "$config_file" >&2
      exit 1
    fi
    export OPENCODE_SANDBOX_GATEWAY_CONFIG="$config_file"
    checksum="$(openssl dgst -sha256 -r "$config_file")"
    export OPENCODE_SANDBOX_GATEWAY_CONFIG_SHA256="${checksum%% *}"
    compose+=(--file "$directory/compose.config.yaml")
  fi
fi

exec "${compose[@]}" "$@"
