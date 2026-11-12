#!/usr/bin/env bash
# Rebuild Wasl with no container-network access. Go dependencies come from tracked pipeline/vendor; base images and
# APK/Python package layers must already be cached. OFFLINE_BUILD_ID forces the Go compile layer to run again.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENGINE="${WASL_CONTAINER_ENGINE:-}"
if [ -z "$ENGINE" ]; then
  if command -v podman >/dev/null 2>&1; then ENGINE=podman; else ENGINE=docker; fi
fi
TAG="${WASL_OFFLINE_TAG:-wasl-offline:local}"
CONTEXT="$(mktemp -d "/tmp/wasl-offline.XXXXXXXX")"
trap 'rm -rf "$CONTEXT"' EXIT
# Include every tracked file plus non-ignored new files (notably a newly generated vendor directory), never .git or out.
{ git -C "$ROOT" ls-files -z; git -C "$ROOT" ls-files -coz --exclude-standard -- pipeline/vendor; } | tar -C "$ROOT" --null -T - -cf - | tar -C "$CONTEXT" -xf -
test -s "$CONTEXT/pipeline/vendor/modules.txt"
PULL=(--pull=false)
if [ "$ENGINE" = podman ]; then PULL=(--pull-never); fi
"$ENGINE" build --network=none "${PULL[@]}" --build-arg "OFFLINE_BUILD_ID=$(date +%s)" --tag "$TAG" "$CONTEXT"
