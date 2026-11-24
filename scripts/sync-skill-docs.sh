#!/bin/sh
set -eu

cd "$(dirname "$0")/.."
references=skills/opencode-sandbox/references
staging=$(mktemp -d)
trap 'rm -rf -- "$staging"' EXIT

cp -R docs/features docs/config docs/limitations docs/risks.md "$staging/"
# Keep research citations usable without bundling developer notes.
sed -i 's|](\.\./research/|](https://github.com/MirkoRischke/opencode-sandbox/blob/main/docs/research/|g' \
    "$staging"/limitations/*.md

case "${1:-}" in
    --check)
        if ! diff -qr --no-dereference "$staging" "$references"; then
            echo 'Skill references are out of sync. Run mise run skill:sync.' >&2
            exit 1
        fi
        ;;
    '')
        # This directory contains only generated files. Replace it to remove stale copies.
        rm -rf -- "$references"
        cp -R "$staging/." "$references"
        chmod 755 "$references"
        ;;
    *)
        echo "Usage: $0 [--check]" >&2
        exit 2
        ;;
esac
