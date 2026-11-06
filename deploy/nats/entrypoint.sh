#!/bin/sh
# Refuse to start NATS with a wrong or missing WASL_JS_KEY on retained JetStream data (docs/DEPLOY.md).
# Without this guard nats-server starts anyway, cannot decrypt the retained stores, and the cluster then re-assigns
# and resets those streams, destroying the retained data. The marker holds only a domain-prefixed SHA-256 of the key.
# On a fresh volume the marker is written only after JetStream reports healthy, so a failed first start (for example a
# malformed key) leaves nothing behind that would block the corrected key.
set -eu
data="${WASL_NATS_DATA:-/data}"
marker="$data/.wasl-key-check"
health="http://127.0.0.1:8222/healthz?js-enabled-only=true"
if [ -z "${WASL_JS_KEY:-}" ]; then
  echo "WASL_JS_KEY is not set; refusing to start." >&2
  exit 1
fi
sum="$(printf 'wasl-js-key-check:%s' "$WASL_JS_KEY" | sha256sum | cut -d' ' -f1)"
if [ -z "$sum" ]; then
  echo "Could not compute the key check hash; refusing to start." >&2
  exit 1
fi
record=0
if [ -f "$marker" ]; then
  if [ "$(cat "$marker")" != "$sum" ]; then
    echo "WASL_JS_KEY does not match the key these retained JetStream stores were created with; refusing to start." >&2
    echo "Supply the original key, or run 'compose down -v' to discard this deployment's data." >&2
    exit 1
  fi
elif [ -d "$data/jetstream" ] && [ -n "$(ls -A "$data/jetstream" 2>/dev/null)" ]; then
  echo "Retained JetStream data has no key marker (created before this check); the key cannot be verified. Starting."
else
  record=1
fi

# Private, atomic marker write: temp file in the same directory, then rename.
write_marker() {
  mkdir -p "$data"
  umask 077
  tmp="$(mktemp "$data/.wasl-key-check.XXXXXX")" || return 1
  if printf '%s\n' "$sum" > "$tmp" && mv "$tmp" "$marker"; then
    return 0
  fi
  rm -f "$tmp"
  return 1
}

docker-entrypoint.sh "$@" &
pid=$!
trap 'kill -TERM "$pid" 2>/dev/null || true' TERM INT

watcher=""
if [ "$record" = 1 ]; then
  (
    while kill -0 "$pid" 2>/dev/null; do
      if wget -q --spider "$health" 2>/dev/null; then
        write_marker || echo "Could not record the key check marker." >&2
        exit 0
      fi
      sleep 1
    done
  ) &
  watcher=$!
fi

# Wait on the same NATS process; a trapped signal interrupts wait, so repeat until the process is really gone.
rc=0
while :; do
  rc=0
  wait "$pid" || rc=$?
  kill -0 "$pid" 2>/dev/null || break
done
if [ -n "$watcher" ]; then
  # a healthy start that exits straight away still gets its marker; otherwise stop the poller
  wait "$watcher" 2>/dev/null || true
fi
exit "$rc"
