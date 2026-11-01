#!/usr/bin/env bash
# One command for the on-premise package (docs/DEPLOY.md): build the image, start the 3-node NATS JetStream cluster,
# run the demo cohort end to end and serve the API on 127.0.0.1:${WASL_API_PORT:-8088}.
# If WASL_JS_KEY is unset, a random JetStream encryption key is made for this shell (not written to the repo or image;
# Docker stores container environment configuration on its host). A restart needs the same key while volumes remain.
# Usage: scripts/deploy_up.sh [extra `docker compose up` args, e.g. -d]
set -euo pipefail
cd "$(dirname "$0")/.."
command -v docker >/dev/null || { echo "docker not found (docs/DEPLOY.md)"; exit 1; }
if [ -z "${WASL_JS_KEY:-}" ]; then
  project="$(printf %s "${COMPOSE_PROJECT_NAME:-$(basename "$PWD")}" | tr '[:upper:]' '[:lower:]' | sed -e 's/^[^a-z0-9]*//' -e 's/[^a-z0-9_-]//g')"
  for volume in nats-0 nats-1 nats-2; do
    if ! volumes="$(docker volume ls -q --filter "label=com.docker.compose.project=$project" --filter "label=com.docker.compose.volume=$volume")"; then
      echo "Could not inspect Docker volumes; refusing to generate a key. Fix Docker or supply the original WASL_JS_KEY." >&2
      exit 1
    fi
    if [ -n "$volumes" ]; then
      cat >&2 <<'EOF'
WASL_JS_KEY is unset and this deployment's encrypted NATS volumes already exist.
Refusing to generate a different key, which would make those stores unreadable.
To keep the data, rerun with the original WASL_JS_KEY in the environment.
To discard this deployment's stores and run data, run `docker compose down -v`, then rerun this script.
EOF
      exit 1
    fi
  done
  WASL_JS_KEY="k$(od -An -tx1 -N32 /dev/urandom | tr -d ' \n')"
  echo "WASL_JS_KEY: random key generated (not written to repo or image; Docker host stores container environment config)"
fi
case "$WASL_JS_KEY" in [A-Za-z]*) ;; *) echo "WASL_JS_KEY must start with a letter"; exit 1;; esac
export WASL_JS_KEY
exec docker compose up --build "$@"
