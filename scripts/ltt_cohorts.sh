#!/usr/bin/env bash
# Learn-then-Test calibration data: N cohorts from the persons3 "cal" split (disjoint from the eval split used by
# scripts/final_eval.sh), held-out v2 noise, then per-tier loss counts via `wsi7 ltt` (offline replay, same cores).
# Usage: scripts/ltt_cohorts.sh [seeds...]  (default 101..106) -> pipeline/results/ltt/counts_<seed>.json
# Env: WASL_LTT_ALL=1 adds lung-nodule, DR and breast loops with the frozen evaluation's switches (-lungsurv, -drnext,
#   -refreject, -xreflink) and per-programme counts (-lttprog) -> pipeline/results/ltt-all/ (work dir out/ltt-all).
#   Cohort per seed (cal split, 3,588 persons, 779 women): CRC 825, cervical 330, distractors 283 (the frozen eval's
#   2,040 / 816 / 700 on 8,871 persons scaled to the cal split), lung 650 and DR 650 (raised so each reaches the 3,688
#   loops a zero-event per-programme certificate at alpha 0.001 needs over 6 cohorts), breast 16 (all that fits: the
#   fewest eligible women aged 40-90 with no other loop over seeds 101-106 is 18 = 16 loops + 2 distractors).
#   Sizes: WASL_LTT_CRC, WASL_LTT_CERV, WASL_LTT_DISTRACT, WASL_LTT_LUNG, WASL_LTT_DR, WASL_LTT_BREAST.
#   Unset (default): the CRC + cervical calibration of the committed certificate, commands unchanged.
# WASL_JOBS: cohorts in parallel (default 3). WASL_SECONDIDCONFIRM=0: -secondidconfirm=false (default on).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"; B="$ROOT/bin"
ALL="${WASL_LTT_ALL:-0}"
if [ "$ALL" = 0 ]; then
  W="${WASL_LTT_WORK:-$ROOT/out/ltt}"; R="${WASL_LTT_DIR:-$ROOT/pipeline/results/ltt}"
  GEN=(-persons 6500 -cerv 600)
  LTTA=()
else
  W="${WASL_LTT_WORK:-$ROOT/out/ltt-all}"; R="${WASL_LTT_DIR:-$ROOT/pipeline/results/ltt-all}"
  GEN=(-persons 6500 -loops "${WASL_LTT_CRC:-825}" -cerv "${WASL_LTT_CERV:-330}" -distract "${WASL_LTT_DISTRACT:-283}"
       -lung "${WASL_LTT_LUNG:-650}" -lungdata "$ROOT/data/lung" -dr "${WASL_LTT_DR:-650}" -drdata "$ROOT/data/dr"
       -breast "${WASL_LTT_BREAST:-16}" -breastdata "$ROOT/data/breast" -lungsurv -drnext -refreject=true)
  LTTA=(-lungsurv -drnext -xreflink -lttprog)
fi
[ "${WASL_SECONDIDCONFIRM:-1}" != 0 ] || LTTA+=(-secondidconfirm=false)
if [ $# -gt 0 ]; then SEEDS=("$@"); else SEEDS=(101 102 103 104 105 106); fi
mkdir -p "$W" "$R"
one() {
  s=$1; O="$W/cal-s$s"
  [ -s "$R/counts_$s.json" ] && { echo "skip $s"; return; }
  rm -rf "$O"; mkdir -p "$O"
  "$B/wsi7" gen -wasl "$ROOT/data" -reports "$ROOT/data/evidence/reports43.json" -pathology "$ROOT/data/evidence/pathology_templates.json" \
    -ecpool "$ROOT/patient-parsers/econsult_testset.json" -tbpool "$ROOT/patient-parsers/teachback_testset.json" \
    -out "$O/base" -seed "$s" "${GEN[@]}" -noise v2 -split cal > /dev/null
  "$B/wsi7" sim -data "$O/base" -wasl "$ROOT/models" -run "$O/run-base" > /dev/null 2>&1
  python3 "$ROOT/pipeline/augment_cohort.py" "$O/base" "$O/run-base" "$O/cohort" > /dev/null
  (cd "$B" && ./wsi7 ltt -data "$O/cohort" -wasl "$ROOT/models" -policy v7strict -ann hnsw -dual -rerr 0.04 -rdouble merge+close \
     -rseed "$s" ${LTTA[@]+"${LTTA[@]}"} -lttout "$R/counts_$s.json" 2> "$O/ltt.log")
  rm -rf "$O/run-base"; echo "done $s"
}
GEN_S="${GEN[*]}"; LTTA_S="${LTTA[*]-}"
export -f one; export ROOT B W R GEN_S LTTA_S
# arrays do not cross the xargs boundary: rebuild them from the exported strings (no element contains a space)
printf '%s\n' "${SEEDS[@]}" | xargs -P "${WASL_JOBS:-3}" -I{} bash -c 'read -ra GEN <<< "$GEN_S"; read -ra LTTA <<< "$LTTA_S"; one {}'
