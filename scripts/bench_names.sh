#!/usr/bin/env bash
# Public Arabic-English name pairs against the name encoder alone (pipeline/namepairs.go; results in
# pipeline/results/benchmarks/names/). Data: Sayedyousef/arabic-name-pairs (Hugging Face, CC-BY-4.0), pinned revision,
# downloaded into $BENCH_TMP (never committed). Dedup needs the encoder's training sources: NETransliteration wd_arabic
# (downloaded here) and the mined Wikidata variants (encoder/mine_wikidata.py output; set ENC_RAW to its directory).
# Usage: scripts/bench_names.sh            Env: BENCH_TMP=/tmp/wasl-bench  ENC_RAW=~/wasl-ml-data/encoder/raw
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
T="${BENCH_TMP:-/tmp/wasl-bench}"; RAW="${ENC_RAW:-$HOME/wasl-ml-data/encoder/raw}"
OUT="$ROOT/pipeline/results/benchmarks"; mkdir -p "$OUT/names" "$T/names"
REV=12991aafd364d14dc0a47517576d7d2c6ed864a6
SHA_PAIRS=b35b2de4d1d584df374621199bd4b9fbb1aa8ad0dd198ed7c35e81d8a02dc71d
SHA_NETRANS=2c4511e1828b2d857467eb6271f244b3aaad7fdaaadc76e0c1f048f2c4dadf07
SHA_VARIANTS=b82b4b530a5fbe0ab86ce81dd4362a13ba0842201b9fdb31ea2e273b64c231b8
P="$T/names/dict_FINAL.json"
[ -s "$P" ] || curl -sSfL -o "$P" "https://huggingface.co/datasets/Sayedyousef/arabic-name-pairs/resolve/$REV/dict_FINAL.json"
echo "$SHA_PAIRS  $P" | shasum -a 256 -c -
N="$RAW/wd_arabic"
[ -s "$N" ] || { mkdir -p "$RAW"; curl -sSfL -o "$N" https://raw.githubusercontent.com/steveash/NETransliteration-COLING2018/master/data/wd_arabic; }
echo "$SHA_NETRANS  $N" | shasum -a 256 -c -
V="$RAW/wikidata_variants.jsonl"
[ -s "$V" ] || { echo "missing $V: run encoder/mine_wikidata.py data/persons3.json $V (about 2 h) or set ENC_RAW"; exit 1; }
echo "$SHA_VARIANTS  $V" | shasum -a 256 -c - || echo "warning: $V differs from the mined file used for the committed result"
(cd "$ROOT/pipeline" && go build -o "$ROOT/bin/wsi7" .)
"$ROOT/bin/wsi7" namepairs -pairs "$P" -encfile "$ROOT/models/encoder_weights.json" -netrans "$N" -variants "$V" \
  -persons "$ROOT/data/persons3.json" -seed 1 -out "$OUT/names/encoder_namepairs.json"
python3 -I "$ROOT/scripts/bench_summary.py" "$OUT"
