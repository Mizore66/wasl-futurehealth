#!/usr/bin/env bash
# Generate or verify source evidence, or scan already-built release images.
set -euo pipefail

MODE="${1:-verify}"
if [[ $# -gt 0 ]]; then shift; fi
ROOT="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
PYTHON="${PYTHON:-python3}"
TRIVY="${TRIVY:-trivy}"
EXPECTED_TRIVY_VERSION="0.76.2"
CONFIG="$ROOT/supply-chain/trivy.yaml"
TRACKED="$ROOT/supply-chain"
TOOL="$ROOT/scripts/supply_chain.py"
OFFLINE="${WASL_SUPPLY_CHAIN_OFFLINE:-0}"

command -v "$PYTHON" >/dev/null
command -v "$TRIVY" >/dev/null
[[ "$OFFLINE" == 0 || "$OFFLINE" == 1 ]] || { echo "WASL_SUPPLY_CHAIN_OFFLINE must be 0 or 1" >&2; exit 2; }

TEMP="$(mktemp -d "${TMPDIR:-/tmp}/wasl-supply-chain.XXXXXXXX")"
trap 'rm -rf "$TEMP"' EXIT

"$TRIVY" version --format json > "$TEMP/trivy-version.json"
ACTUAL_VERSION="$("$PYTHON" -I -c 'import json,sys; print(json.load(open(sys.argv[1]))["Version"])' "$TEMP/trivy-version.json")"
if [[ "$ACTUAL_VERSION" != "$EXPECTED_TRIVY_VERSION" ]]; then
  echo "Trivy $EXPECTED_TRIVY_VERSION is required; found $ACTUAL_VERSION" >&2
  exit 2
fi

COMMON=(
  --config "$CONFIG"
  --ignorefile ""
  --skip-java-db-update
  --skip-vex-repo-update
  --disable-telemetry
  --skip-version-check
  --no-progress
)
FS_COMMON=("${COMMON[@]}" --include-dev-deps)
OFFLINE_ARGS=()
if [[ "$OFFLINE" == 1 ]]; then
  OFFLINE_ARGS=(--offline-scan --skip-db-update)
fi
MANIFEST_OFFLINE=()
if [[ "$OFFLINE" == 1 ]]; then MANIFEST_OFFLINE=(--offline); fi
IMAGE_SOURCE_ARGS=()
if [[ -n "${TRIVY_IMAGE_SOURCE:-}" ]]; then IMAGE_SOURCE_ARGS=(--image-src "$TRIVY_IMAGE_SOURCE"); fi
if [[ -n "${TRIVY_PODMAN_HOST:-}" ]]; then IMAGE_SOURCE_ARGS+=(--podman-host "$TRIVY_PODMAN_HOST"); fi

case "$MODE" in
  generate|verify)
    if [[ $# -ne 0 ]]; then echo "Usage: $0 [generate|verify]" >&2; exit 2; fi
    if [[ "$MODE" == generate ]]; then
      DESTINATION="$TRACKED"
      CONTEXT_ARGS=()
    else
      DESTINATION="${WASL_SUPPLY_CHAIN_VERIFY_OUTPUT:-$TEMP/verified}"
      CONTEXT_ARGS=(--tracked "$TRACKED/vulnerabilities.json")
    fi
    mkdir -p "$DESTINATION"
    "$PYTHON" -I "$TOOL" context --root "$ROOT" ${CONTEXT_ARGS[@]+"${CONTEXT_ARGS[@]}"} --output "$TEMP/context.json"

    "$TRIVY" fs "${FS_COMMON[@]}" ${OFFLINE_ARGS[@]+"${OFFLINE_ARGS[@]}"} --format cyclonedx --scanners license \
      --output "$TEMP/repository.raw.json" "$ROOT"
    "$PYTHON" -I "$TOOL" augment-sbom --input "$TEMP/repository.raw.json" --context "$TEMP/context.json" \
      --output "$DESTINATION/repository.cdx.json"

    set +e
    "$TRIVY" fs "${FS_COMMON[@]}" ${OFFLINE_ARGS[@]+"${OFFLINE_ARGS[@]}"} --format json --scanners vuln --exit-code 1 \
      --output "$TEMP/vulnerabilities.raw.json" "$ROOT"
    VULNERABILITY_RC=$?
    set -e
    if [[ $VULNERABILITY_RC -gt 1 ]]; then exit "$VULNERABILITY_RC"; fi
    "$TRIVY" version --format json > "$TEMP/trivy-version.json"
    "$PYTHON" -I "$TOOL" normalize-vulnerabilities --input "$TEMP/vulnerabilities.raw.json" \
      --context "$TEMP/context.json" --scanner "$TEMP/trivy-version.json" --output "$DESTINATION/vulnerabilities.json"

    "$TRIVY" fs "${FS_COMMON[@]}" ${OFFLINE_ARGS[@]+"${OFFLINE_ARGS[@]}"} --format json --scanners license \
      --output "$TEMP/licenses.raw.json" "$ROOT"
    "$PYTHON" -I "$TOOL" normalize-licenses --input "$TEMP/licenses.raw.json" \
      --context "$TEMP/context.json" --scanner "$TEMP/trivy-version.json" --output "$DESTINATION/licenses.json"
    "$PYTHON" -I "$TOOL" manifest --directory "$DESTINATION" --context "$TEMP/context.json" \
      --scanner "$TEMP/trivy-version.json" ${MANIFEST_OFFLINE[@]+"${MANIFEST_OFFLINE[@]}"} --output "$DESTINATION/manifest.json"

    if [[ "$MODE" == verify ]]; then
      "$PYTHON" -I "$TOOL" compare --generated "$DESTINATION" --tracked "$TRACKED"
    fi
    # Offline: validates the notice manifest and the exact generated bundle; a failure aborts under set -e.
    "$PYTHON" -I "$ROOT/scripts/notices.py" check --root "$ROOT"
    "$PYTHON" -I "$TOOL" gate --input "$DESTINATION/vulnerabilities.json"
    ;;

  image)
    if [[ $# -lt 1 ]]; then echo "Usage: $0 image IMAGE [IMAGE ...]" >&2; exit 2; fi
    "$PYTHON" -I "$TOOL" context --root "$ROOT" --output "$TEMP/context.json"
    DESTINATION="${WASL_SUPPLY_CHAIN_OUTPUT:-$ROOT/out/supply-chain}/images"
    FAILED=0
    for IMAGE in "$@"; do
      SAFE_NAME="$(printf '%s' "$IMAGE" | tr -c 'A-Za-z0-9_.-' '_')"
      IMAGE_DESTINATION="$DESTINATION/$SAFE_NAME"
      mkdir -p "$IMAGE_DESTINATION"
      "$TRIVY" image "${COMMON[@]}" ${OFFLINE_ARGS[@]+"${OFFLINE_ARGS[@]}"} ${IMAGE_SOURCE_ARGS[@]+"${IMAGE_SOURCE_ARGS[@]}"} --format cyclonedx --scanners vuln,license \
        --output "$TEMP/image.raw.json" "$IMAGE"
      "$PYTHON" -I "$TOOL" bind-image-sbom --input "$TEMP/image.raw.json" --context "$TEMP/context.json" \
        --image "$IMAGE" --output "$IMAGE_DESTINATION/repository.cdx.json"
      set +e
      "$TRIVY" image "${COMMON[@]}" ${OFFLINE_ARGS[@]+"${OFFLINE_ARGS[@]}"} ${IMAGE_SOURCE_ARGS[@]+"${IMAGE_SOURCE_ARGS[@]}"} --format json --scanners vuln --exit-code 1 \
        --output "$TEMP/image-vulnerabilities.raw.json" "$IMAGE"
      VULNERABILITY_RC=$?
      set -e
      if [[ $VULNERABILITY_RC -gt 1 ]]; then exit "$VULNERABILITY_RC"; fi
      "$TRIVY" version --format json > "$TEMP/trivy-version.json"
      "$PYTHON" -I "$TOOL" normalize-vulnerabilities --input "$TEMP/image-vulnerabilities.raw.json" \
        --context "$TEMP/context.json" --scanner "$TEMP/trivy-version.json" --output "$IMAGE_DESTINATION/vulnerabilities.json"
      "$TRIVY" image "${COMMON[@]}" ${OFFLINE_ARGS[@]+"${OFFLINE_ARGS[@]}"} ${IMAGE_SOURCE_ARGS[@]+"${IMAGE_SOURCE_ARGS[@]}"} --format json --scanners license \
        --output "$TEMP/image-licenses.raw.json" "$IMAGE"
      "$PYTHON" -I "$TOOL" normalize-licenses --input "$TEMP/image-licenses.raw.json" \
        --context "$TEMP/context.json" --scanner "$TEMP/trivy-version.json" --output "$IMAGE_DESTINATION/licenses.json"
      "$PYTHON" -I "$TOOL" manifest --directory "$IMAGE_DESTINATION" --context "$TEMP/context.json" \
        --scanner "$TEMP/trivy-version.json" ${MANIFEST_OFFLINE[@]+"${MANIFEST_OFFLINE[@]}"} --output "$IMAGE_DESTINATION/manifest.json"
      set +e
      "$PYTHON" -I "$TOOL" gate --input "$IMAGE_DESTINATION/vulnerabilities.json"
      GATE_RC=$?
      set -e
      if [[ $GATE_RC -ne 0 ]]; then FAILED=1; fi
    done
    exit "$FAILED"
    ;;

  *) echo "Usage: $0 [generate|verify|image IMAGE ...]" >&2; exit 2 ;;
esac
