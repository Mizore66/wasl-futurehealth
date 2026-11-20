#!/usr/bin/env bash
# Public FEBRL4 benchmark, end to end (pipeline/febrl.go; results in pipeline/results/benchmarks/).
#   1. throwaway virtualenv under $BENCH_TMP (default /tmp/wasl-bench) with recordlinkage==0.16 (third-party, never a
#      repo dependency); FEBRL4 dataset4a.csv / dataset4b.csv copied out of that package into a fresh directory and
#      checked against the sha256 recorded below
#   2. builds bin/wsi7 and runs Wasl's linker on both files: config names_dob (closest to unchanged) and names_dob_ssid
#      (soc_sec_id wrapped into the Emirates-ID slot; an adaptation)
#   3. reruns the published recordlinkage baseline plus a same-fields variant (scripts/bench_febrl_recordlinkage.py)
#   4. rebuilds pipeline/results/benchmarks/summary.json (scripts/bench_summary.py, stdlib only)
# Usage: scripts/bench_febrl.sh            Env: BENCH_TMP=/tmp/wasl-bench  PYTHON=python3
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
T="${BENCH_TMP:-/tmp/wasl-bench}"; PY="${PYTHON:-python3}"
OUT="$ROOT/pipeline/results/benchmarks"; mkdir -p "$OUT/febrl" "$T"
SHA_A=07c7cb3f0a8d88180e80317f2a60499dee4e8324a44c38059f4e7fed0a8b4488
SHA_B=2eed76c99fa2237be3ec013a123427926d4158abcb3a8f65874d6c7f1358cf2c
[ -x "$T/venv/bin/python" ] || "$PY" -m venv "$T/venv"
"$T/venv/bin/python" -m pip -q install "recordlinkage==0.16"
PKG="$("$T/venv/bin/python" -I -c 'import os, recordlinkage.datasets as d; print(os.path.join(os.path.dirname(d.__file__), "febrl"))')"
D="$T/febrl"; rm -rf "$D"; mkdir -p "$D"
cp "$PKG/dataset4a.csv" "$PKG/dataset4b.csv" "$D/"
echo "$SHA_A  $D/dataset4a.csv" | shasum -a 256 -c -
echo "$SHA_B  $D/dataset4b.csv" | shasum -a 256 -c -
(cd "$ROOT/pipeline" && go build -o "$ROOT/bin/wsi7" .)
for c in names_dob names_dob_ssid; do
  "$ROOT/bin/wsi7" febrl -a "$D/dataset4a.csv" -b "$D/dataset4b.csv" -wasl "$ROOT/models" -config "$c" -out "$OUT/febrl/wasl_$c.json"
  echo "wasl $c -> $OUT/febrl/wasl_$c.json"
done
(cd "$T" && "$T/venv/bin/python" -I "$ROOT/scripts/bench_febrl_recordlinkage.py" "$OUT/febrl/recordlinkage.json")
python3 -I "$ROOT/scripts/bench_summary.py" "$OUT"
