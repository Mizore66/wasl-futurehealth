#!/usr/bin/env bash
# Regenerate the tracked offline wheel/APK bundles. Runs ONLINE; only for updating bundles.
#
# Usage: scripts/offline_dependencies.sh [--update-locks] [amd64 arm64]
#   default        re-fetch exactly what deploy/*/locks pin (apk name=version, pip --require-hashes)
#   --update-locks resolve the newest packages, cross-check wheel hashes against PyPI, rewrite the locks
# Env: ENGINE=podman|docker (default podman), TMPDIR (default /tmp).
#
# Base images are digest-pinned and pulled with an explicit --platform. No local tags are created or moved.
# Nothing is written under deploy/ until both architectures are fetched and the staged trees validate.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENGINE="${ENGINE:-podman}"
PYTHON_REF="docker.io/library/python@sha256:e064671ca452259bab50e8e2c7bd896e39e9a343bb06ca25234adf6fe18d0fc7"
NATS_REF="docker.io/library/nats@sha256:1cfc36e2e5e638243d8c722f72c954cd0ec4b15ee82fadbc718ce12e2b3c1652"
WASL_REL="3.24"   # python:3.12-alpine base
NATS_REL="3.22"   # nats:2.12.6-alpine base
WASL_APKS="bash util-linux ca-certificates krb5-libs zlib"
# Upgrades the NATS Dockerfile needs on top of the base, plus ca-certificates; no apk-tools/tzdata/alpine-release.
NATS_APKS="ca-certificates ca-certificates-bundle libcrypto3 libssl3 musl musl-utils zlib"
PIP_SPECS="pip==26.3 numpy==2.3.5 scipy==1.16.3 cryptography==50.0.2"

UPDATE=0
if [ "${1:-}" = "--update-locks" ]; then UPDATE=1; shift; fi
ARCHES=("$@")
[ ${#ARCHES[@]} -gt 0 ] || ARCHES=(amd64 arm64)
for a in "${ARCHES[@]}"; do case "$a" in amd64|arm64) ;; *) echo "unsupported arch: $a" >&2; exit 2 ;; esac; done
[ ${#ARCHES[@]} -eq 2 ] || { echo "both amd64 and arm64 are required" >&2; exit 2; }

TMPBASE="${TMPDIR:-/tmp}"; TMPBASE="${TMPBASE%/}"
STAGE="$(mktemp -d "$TMPBASE/wasl-offline-deps.XXXXXX")"
trap 'rm -rf "$STAGE" "$ROOT"/deploy/offline.new "$ROOT"/deploy/nats/offline.new' EXIT

# Runs inside the container. Args: kind machine release-prefix python-prefix mode
read -r -d '' INNER <<'EOS' || true
set -eu
kind=$1; mach=$2; relpre=$3; pypre=$4; mode=$5
cd /
[ "$(uname -m)" = "$mach" ] || { echo "uname -m $(uname -m) != $mach" >&2; exit 1; }
rel=$(cat /etc/alpine-release)
case "$rel" in "$relpre".*) ;; *) echo "alpine $rel != $relpre.x" >&2; exit 1 ;; esac
if [ "$kind" = wasl ]; then
  case "$(python -c 'import platform;print(platform.python_version())')" in "$pypre".*) ;; *) echo "python is not $pypre" >&2; exit 1 ;; esac
fi
mkdir -p /out/apk /out/wheels
echo "$rel" > /out/release
apk update
if [ "$mode" = pinned ]; then
  apk fetch --output /out/apk $(cat /in/apk.pins)
else
  if [ "$kind" = wasl ]; then apk fetch --recursive --output /out/apk $(cat /in/apk.names)
  else apk fetch --output /out/apk $(cat /in/apk.names); fi
fi
(cd /out/apk && ls) | sed -E 's/^(.*)-([0-9][^-]*-r[0-9]+)\.apk$/\1=\2/' | LC_ALL=C sort > /out/apk.lock
if [ "$mode" = pinned ]; then LC_ALL=C sort /in/apk.pins | diff - /out/apk.lock >&2; fi
if [ "$kind" = wasl ]; then
  if [ "$mode" = pinned ]; then
    python -m pip download --require-hashes --only-binary=:all: --no-deps --dest /out/wheels -r /in/wheels.txt
  else
    python -m pip download --only-binary=:all: --dest /out/wheels $(cat /in/pip.specs)
  fi
  python -I - "$mode" <<'PY'
import hashlib, json, os, re, sys, urllib.request
mode = sys.argv[1]
d = "/out/wheels"
out = []
for fn in sorted(os.listdir(d)):
    m = re.match(r"^([A-Za-z0-9_.]+)-([^-]+)(-\d[^-]*)?-[^-]+-[^-]+-[^-]+\.whl$", fn)
    name, ver = re.sub(r"[-_.]+", "-", m.group(1)).lower(), m.group(2)
    h = hashlib.sha256(open(os.path.join(d, fn), "rb").read()).hexdigest()
    if mode != "pinned":
        meta = json.load(urllib.request.urlopen("https://pypi.org/pypi/%s/%s/json" % (name, ver)))
        pub = [u["digests"]["sha256"] for u in meta["urls"] if u["filename"] == fn]
        if pub != [h]:
            sys.exit("PyPI hash mismatch for %s: %s vs %s" % (fn, pub, h))
    out.append("%s==%s --hash=sha256:%s  # %s" % (name, ver, h, fn))
open("/out/wheels.txt", "w").write("\n".join(out) + "\n")
PY
fi
EOS

fetch_arch() { # kind arch ref mach relpre mode names
  local kind=$1 arch=$2 ref=$3 mach=$4 relpre=$5 mode=$6 names=$7 dest=$8 lockdir=$9
  local in="$STAGE/in.$kind.$arch"; mkdir -p "$in" "$dest"
  echo "$names" > "$in/apk.names"; echo "$PIP_SPECS" > "$in/pip.specs"
  if [ "$mode" = pinned ]; then
    grep -v '^#' "$lockdir/apk-$arch.lock" > "$in/apk.pins"
    [ "$kind" = nats ] || cp "$lockdir/wheels-$arch.txt" "$in/wheels.txt"
  fi
  "$ENGINE" pull -q --platform "linux/$arch" "$ref" >/dev/null
  local cid
  cid="$("$ENGINE" create --pull=never --platform "linux/$arch" --entrypoint /bin/sh "$ref" -c "$INNER" sh "$kind" "$mach" "$relpre" 3.12 "$mode")"
  "$ENGINE" cp "$in" "$cid:/in"
  "$ENGINE" start -a "$cid" >&2 || { "$ENGINE" rm "$cid" >/dev/null; return 1; }
  "$ENGINE" cp "$cid:/out/." "$dest"
  "$ENGINE" rm "$cid" >/dev/null
}

assemble() { # kind outroot
  local kind=$1 out=$2 mode=$3 lockdir=$4 ref names rel
  if [ "$kind" = wasl ]; then ref=$PYTHON_REF; names=$WASL_APKS; rel=$WASL_REL; else ref=$NATS_REF; names=$NATS_APKS; rel=$NATS_REL; fi
  mkdir -p "$out/apk" "$out/wheels" "$out/locks"
  for arch in "${ARCHES[@]}"; do
    local mach; [ "$arch" = amd64 ] && mach=x86_64 || mach=aarch64
    local raw="$STAGE/raw.$kind.$arch"; mkdir -p "$raw"
    fetch_arch "$kind" "$arch" "$ref" "$mach" "$rel" "$mode" "$names" "$raw" "$lockdir"
    # Only wheel/APK files, no symlinks.
    find "$raw/apk" -mindepth 1 \( -type l -o ! -name '*.apk' \) | grep -q . && { echo "non-apk artifact" >&2; return 1; }
    find "$raw/wheels" -mindepth 1 \( -type l -o ! -name '*.whl' \) | grep -q . && { echo "non-wheel artifact" >&2; return 1; }
    mkdir -p "$out/apk/$arch"; cp "$raw"/apk/*.apk "$out/apk/$arch/"
    { echo "# alpine-release: $(cat "$raw/release")"; cat "$raw/apk.lock"; } > "$out/locks/apk-$arch.lock"
    if [ "$kind" = wasl ]; then
      mkdir -p "$out/wheels/$arch"; cp "$raw"/wheels/*.whl "$out/wheels/$arch/"
      { echo "# alpine-release: $(cat "$raw/release")"; cat "$raw/wheels.txt"; } > "$out/locks/wheels-$arch.txt"
    fi
  done
  [ "$kind" = wasl ] || rmdir "$out/wheels"
  python3 -I "$ROOT/scripts/offline_dependency_manifest.py" generate "$out"
  python3 -I "$ROOT/scripts/offline_dependency_manifest.py" check "$out" --profile "$kind"
}

mode=pinned
if [ "$UPDATE" = 1 ]; then mode=resolve; fi
if [ "$mode" = pinned ]; then
  for t in "$ROOT/deploy/offline/locks" "$ROOT/deploy/nats/offline/locks"; do [ -d "$t" ] || { echo "no locks at $t; run with --update-locks" >&2; exit 1; }; done
fi
assemble wasl "$STAGE/out.wasl" "$mode" "$ROOT/deploy/offline/locks"
assemble nats "$STAGE/out.nats" "$mode" "$ROOT/deploy/nats/offline/locks"

# Both architectures validated: replace atomically (same-filesystem rename), keeping the old tree until the new one is in place.
swap() { # staged final
  local staged=$1 final=$2
  rm -rf "$final.new" "$final.old"
  cp -R "$staged" "$final.new"
  [ ! -e "$final" ] || mv "$final" "$final.old"
  mv "$final.new" "$final"
  rm -rf "$final.old"
}
swap "$STAGE/out.wasl" "$ROOT/deploy/offline"
swap "$STAGE/out.nats" "$ROOT/deploy/nats/offline"
python3 -I "$ROOT/scripts/offline_dependency_manifest.py" check "$ROOT/deploy/offline" --profile wasl
python3 -I "$ROOT/scripts/offline_dependency_manifest.py" check "$ROOT/deploy/nats/offline" --profile nats
