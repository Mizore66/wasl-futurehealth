#!/usr/bin/env bash
# Learn-then-Test calibration data: N cohorts from the persons3 "cal" split (disjoint from the eval split used by
# scripts/final_eval.sh), held-out v2 noise, then per-tier loss counts via `wsi7 ltt` (offline replay, same cores).
# Usage: scripts/ltt_cohorts.sh [seeds...]  (default 101..106) -> pipeline/results/ltt/counts_<seed>.json
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"; B="$ROOT/bin"; W="${WASL_LTT_WORK:-$ROOT/out/ltt}"; R="$ROOT/pipeline/results/ltt"
if [ $# -gt 0 ]; then SEEDS=("$@"); else SEEDS=(101 102 103 104 105 106); fi
mkdir -p "$W" "$R"
one() {
  s=$1; O="$W/cal-s$s"
  [ -s "$R/counts_$s.json" ] && { echo "skip $s"; return; }
  rm -rf "$O"; mkdir -p "$O"
  "$B/wsi7" gen -wasl "$ROOT/data" -reports "$ROOT/data/evidence/reports43.json" -pathology "$ROOT/data/evidence/pathology_templates.json" \
    -ecpool "$ROOT/patient-parsers/econsult_testset.json" -tbpool "$ROOT/patient-parsers/teachback_testset.json" \
    -out "$O/base" -seed "$s" -persons 6500 -cerv 600 -noise v2 -split cal > /dev/null
  "$B/wsi7" sim -data "$O/base" -wasl "$ROOT/models" -run "$O/run-base" > /dev/null 2>&1
  python3 "$ROOT/pipeline/augment_cohort.py" "$O/base" "$O/run-base" "$O/cohort" > /dev/null
  (cd "$B" && ./wsi7 ltt -data "$O/cohort" -wasl "$ROOT/models" -policy v7strict -ann hnsw -dual -rerr 0.04 -rdouble merge+close \
     -rseed "$s" -lttout "$R/counts_$s.json" 2> "$O/ltt.log")
  rm -rf "$O/run-base"; echo "done $s"
}
export -f one; export ROOT B W R
printf '%s\n' "${SEEDS[@]}" | xargs -P 3 -I{} bash -c 'one {}'
