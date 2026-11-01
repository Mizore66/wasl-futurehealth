#!/usr/bin/env bash
# One command for the on-premise package (docs/DEPLOY.md): build the image, start the 3-node NATS JetStream cluster,
# run the demo cohort end to end and serve the API on 127.0.0.1:${WASL_API_PORT:-8088}.
# If WASL_JS_KEY is unset, a random JetStream encryption key is made for this shell only (never written to disk);
# the encrypted stores are then unreadable after `docker compose down -v` or a restart with another key.
# Usage: scripts/deploy_up.sh [extra `docker compose up` args, e.g. -d]
set -euo pipefail
cd "$(dirname "$0")/.."
command -v docker >/dev/null || { echo "docker not found (docs/DEPLOY.md)"; exit 1; }
if [ -z "${WASL_JS_KEY:-}" ]; then
  WASL_JS_KEY="k$(od -An -tx1 -N32 /dev/urandom | tr -d ' \n')"
  echo "WASL_JS_KEY: random key for this run (kept in this process environment only)"
fi
case "$WASL_JS_KEY" in [A-Za-z]*) ;; *) echo "WASL_JS_KEY must start with a letter"; exit 1;; esac
export WASL_JS_KEY
exec docker compose up --build "$@"
