#!/usr/bin/env bash
# Clean offline build: Wasl and NATS images with no layer cache and no container network. Dependencies come from the
# tracked bundles (pipeline/vendor, deploy/nats/build/vendor, deploy/offline, deploy/nats/offline). Base images must
# already be present locally; this does NOT prove an empty host. Needs BuildKit on Docker (DOCKER_BUILDKIT=1).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENGINE="${WASL_CONTAINER_ENGINE:-}"
if [ -z "$ENGINE" ]; then
  if command -v podman >/dev/null 2>&1; then ENGINE=podman; else ENGINE=docker; fi
fi
case "$ENGINE" in docker|podman) ;; *) echo "WASL_CONTAINER_ENGINE must be docker or podman" >&2; exit 2 ;; esac
WASL_TAG="${WASL_CLEAN_TAG:-wasl-offline-clean:local}"
NATS_TAG="${WASL_NATS_CLEAN_TAG:-wasl-nats-offline-clean:local}"
GO_VERSION="${GO_VERSION:-1.27.4}"
PYTHON_VERSION="${PYTHON_VERSION:-3.12}"
NATS_VERSION="${NATS_VERSION:-2.12.6}"
GO_IMAGE="golang:${GO_VERSION}-bookworm"
PY_IMAGE="python:${PYTHON_VERSION}-alpine"
NATS_IMAGE="nats:${NATS_VERSION}-alpine"

# Base images must be preloaded: the build never pulls.
for img in "$GO_IMAGE" "$PY_IMAGE" "$NATS_IMAGE"; do
  "$ENGINE" image inspect "$img" >/dev/null 2>&1 || { echo "base image $img is not loaded locally; clean builds never pull" >&2; exit 1; }
done

CONTEXT="$(mktemp -d "/tmp/wasl-offline-clean.XXXXXXXX")"
trap 'rm -rf "$CONTEXT"' EXIT
# Fresh context (under /tmp so a Podman VM can read it): tracked files only; every bundle is committed.
# Never .git, untracked files, build outputs or prior caches.
git -C "$ROOT" ls-files -z | tar -C "$ROOT" --null -T - -cf - | tar -C "$CONTEXT" -xf -
test -s "$CONTEXT/pipeline/vendor/modules.txt"
test -s "$CONTEXT/deploy/nats/build/vendor/modules.txt"
test -s "$CONTEXT/deploy/offline/SHA256SUMS"
test -s "$CONTEXT/deploy/nats/offline/SHA256SUMS"

PULL=(--pull=false)
if [ "$ENGINE" = podman ]; then PULL=(--pull-never); fi
BUILD_ID="$(date +%s)"
COMMON=(--no-cache --network=none "${PULL[@]}" --build-arg OFFLINE_PACKAGES=1 --build-arg "OFFLINE_BUILD_ID=$BUILD_ID")

echo "== engine: $ENGINE; build id: $BUILD_ID"
"$ENGINE" build "${COMMON[@]}" --build-arg OFFLINE_RUNTIME=0 --build-arg "GO_VERSION=$GO_VERSION" --build-arg "PYTHON_VERSION=$PYTHON_VERSION" --build-arg "PYTHON_IMAGE=$PY_IMAGE" --tag "$WASL_TAG" "$CONTEXT"
"$ENGINE" build "${COMMON[@]}" --build-arg "GO_VERSION=$GO_VERSION" --build-arg "NATS_VERSION=$NATS_VERSION" --tag "$NATS_TAG" "$CONTEXT/deploy/nats"

check_uid() {
  local uid
  uid="$("$ENGINE" image inspect --format '{{.Config.User}}' "$1")"
  [ "$uid" = "10001:10001" ] || { echo "$1 final user is '$uid', expected 10001:10001" >&2; exit 1; }
}
check_uid "$WASL_TAG"
check_uid "$NATS_TAG"
"$ENGINE" run --rm --network=none --entrypoint /app/bin/wsi7 "$WASL_TAG" -h >/dev/null
[ "$("$ENGINE" run --rm --network=none --entrypoint id "$WASL_TAG" -u)" = 10001 ]
NATS_OUT="$("$ENGINE" run --rm --network=none --entrypoint /usr/local/bin/nats-server "$NATS_TAG" -v)"
echo "$NATS_OUT"
[ "$NATS_OUT" = "nats-server: v${NATS_VERSION}" ] || { echo "unexpected NATS version" >&2; exit 1; }
[ "$("$ENGINE" run --rm --network=none --entrypoint id "$NATS_TAG" -u)" = 10001 ]
echo "== image IDs"
"$ENGINE" image inspect --format '{{.Id}} {{.Architecture}}' "$WASL_TAG" "$NATS_TAG"
