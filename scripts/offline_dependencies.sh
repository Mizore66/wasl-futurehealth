#!/usr/bin/env bash
# Regenerate the tracked offline wheel/APK bundles. Runs ONLINE; only for updating bundles.
# Usage: scripts/offline_dependencies.sh [amd64 arm64]   (env: ENGINE=podman|docker, IMAGE=...)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENGINE="${ENGINE:-podman}"
# Per-arch local tags of python:3.12-alpine (create with: podman pull --platform linux/ARCH ... then podman tag).
IMAGE_PREFIX="${IMAGE_PREFIX:-localhost/wasl-offline-gen-python}"
WASL_APKS="bash util-linux ca-certificates krb5-libs zlib"
NATS_APKS="ca-certificates krb5-libs zlib"
PIP_SPECS="pip==26.3 numpy==2.3.5 scipy==1.16.3 cryptography==50.0.2"
ARCHES=("$@")
[ ${#ARCHES[@]} -gt 0 ] || ARCHES=(amd64 arm64)

mkdir -p /private/tmp
WORK="$(mktemp -d /private/tmp/wasl-offline-deps.XXXXXX)"
trap 'rm -rf "$WORK"' EXIT

for arch in "${ARCHES[@]}"; do
  case "$arch" in amd64|arm64) ;; *) echo "unsupported arch: $arch" >&2; exit 2 ;; esac
  mkdir -p "$WORK/$arch"
  # Generation is online for package fetches; the base image must already be tagged locally (never pulled here,
  # so a pull cannot overwrite the shared python:3.12-alpine tag).
  # No host bind mount: artifacts are produced in a container and copied out with `cp`.
  cid="$("$ENGINE" create --pull=never --platform "linux/$arch" "$IMAGE_PREFIX:$arch" sh -euc "
    mkdir -p /out/wheels /out/apk /out/nats-apk
    pip download --only-binary=:all: --dest /out/wheels $PIP_SPECS
    apk update
    apk fetch --recursive --output /out/apk $WASL_APKS
    apk fetch --recursive --output /out/nats-apk $NATS_APKS
  ")"
  "$ENGINE" start -a "$cid"
  "$ENGINE" cp "$cid:/out/." "$WORK/$arch"
  "$ENGINE" rm "$cid" >/dev/null
  # Reject anything that is not a wheel/APK or is a symlink.
  find "$WORK/$arch/wheels" -mindepth 1 \( -type l -o ! -name '*.whl' \) -print -quit | grep -q . && { echo "non-wheel artifact" >&2; exit 1; }
  find "$WORK/$arch/apk" "$WORK/$arch/nats-apk" -mindepth 1 \( -type l -o ! -name '*.apk' \) -print -quit | grep -q . && { echo "non-apk artifact" >&2; exit 1; }
  for d in deploy/offline/wheels deploy/offline/apk deploy/nats/offline/apk; do
    rm -rf "${ROOT:?}/$d/$arch"; mkdir -p "$ROOT/$d/$arch"
  done
  cp "$WORK/$arch"/wheels/*.whl "$ROOT/deploy/offline/wheels/$arch/"
  cp "$WORK/$arch"/apk/*.apk "$ROOT/deploy/offline/apk/$arch/"
  cp "$WORK/$arch"/nats-apk/*.apk "$ROOT/deploy/nats/offline/apk/$arch/"
done

# Data-only hashing; nothing from the bundles is executed.
python3 -I "$ROOT/scripts/offline_dependency_manifest.py" generate "$ROOT/deploy/offline"
python3 -I "$ROOT/scripts/offline_dependency_manifest.py" generate "$ROOT/deploy/nats/offline"
python3 -I "$ROOT/scripts/offline_dependency_manifest.py" check "$ROOT/deploy/offline"
python3 -I "$ROOT/scripts/offline_dependency_manifest.py" check "$ROOT/deploy/nats/offline"
