#!/bin/sh
# Refuse to start NATS with a wrong or missing WASL_JS_KEY on retained JetStream data (docs/DEPLOY.md).
# Without this guard nats-server starts anyway, cannot decrypt the retained stores, and the cluster then re-assigns
# and resets those streams, destroying the retained data. The marker holds only a domain-prefixed SHA-256 of the key.
set -eu
data="${WASL_NATS_DATA:-/data}"
marker="$data/.wasl-key-check"
if [ -z "${WASL_JS_KEY:-}" ]; then
  echo "WASL_JS_KEY is not set; refusing to start." >&2
  exit 1
fi
sum="$(printf 'wasl-js-key-check:%s' "$WASL_JS_KEY" | sha256sum | cut -d' ' -f1)"
if [ -f "$marker" ]; then
  if [ "$(cat "$marker")" != "$sum" ]; then
    echo "WASL_JS_KEY does not match the key these retained JetStream stores were created with; refusing to start." >&2
    echo "Supply the original key, or run 'compose down -v' to discard this deployment's data." >&2
    exit 1
  fi
elif [ -d "$data/jetstream" ] && [ -n "$(ls -A "$data/jetstream" 2>/dev/null)" ]; then
  echo "Retained JetStream data has no key marker (created before this check); the key cannot be verified. Starting."
else
  mkdir -p "$data"
  printf '%s\n' "$sum" > "$marker"
fi
exec docker-entrypoint.sh "$@"
