#!/usr/bin/env bash
# Copy only runtime secret files into a new container volume without a host bind mount or logging values.
set -euo pipefail
if [ "$#" -ne 2 ]; then
  echo "usage: $0 <new-volume> <secret-directory>" >&2
  exit 2
fi
engine="${WASL_CONTAINER_ENGINE:-podman}"
volume="$1"
directory="$2"
case "$volume" in *[!A-Za-z0-9_.-]*|"") echo "invalid secret volume name" >&2; exit 2;; esac
[ -d "$directory" ] || { echo "secret directory not found" >&2; exit 1; }
files=(js_key nats_password route_password api_users.json api_coordinator_password api_clinician_password
       api_reviewer_password api_auditor_password backup_key recovery_keys.json ca.crt server.crt server.key)
for file in "${files[@]}"; do
  [ -f "$directory/$file" ] || { echo "required secret file missing: $file" >&2; exit 1; }
done
if "$engine" volume inspect "$volume" >/dev/null 2>&1; then
  echo "refusing to overwrite existing secret volume: $volume" >&2
  exit 1
fi
"$engine" volume create "$volume" >/dev/null
name="wasl-secret-load-${volume}-$$"
runtime="$(mktemp -d "${TMPDIR:-/tmp}/wasl-runtime-secrets.XXXXXX")"
trap '"$engine" rm -f "$name" >/dev/null 2>&1 || true; rm -rf "$runtime"' EXIT
for file in "${files[@]}"; do cp "$directory/$file" "$runtime/$file"; done
# CA signing keys stay in the offline bootstrap directory and are never loaded into service containers.
"$engine" create --name "$name" --user 0:0 -v "$volume":/run/secrets docker.io/library/alpine:3.23 true >/dev/null
"$engine" cp "$runtime"/. "$name":/run/secrets/
printf '{"level":"INFO","msg":"secret volume populated; values not logged","volume":"%s"}\n' "$volume"
