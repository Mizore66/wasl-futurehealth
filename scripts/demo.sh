#!/usr/bin/env bash
# Wasl one-command baseline: check assets → build → cohort → integrated NATS run → audit → tamper → report.
# Usage: scripts/demo.sh [all|check|build|cohort|run|audit|tamper|booking|report|test|serve|snapshot|final]   (default: all)
#   ltt [seeds...]: Learn-then-Test calibration of the closure tier on cal-split cohorts -> pipeline/results/ltt/ (~40 min); WASL_POLICY=v7ltt uses it
#     WASL_LTT_ALL=1 also calibrates on lung, DR and breast loops -> pipeline/results/ltt-all/ (per-programme counts; scripts/ltt_cohorts.sh)
#   final [seeds...]: stage 7 frozen evaluation, 5 seeds x {v1, v2 noise} -> pipeline/results/final/ (~45 min)
# Outputs: out/cohort/ (synthetic data), out/run/<policy>/ (logs, report7.json, dashboard.json, review_queue.json).
# Env: WASL_POLICY=v7strict|v6, WASL_READER=cached|live, WASL_SOURCE=fhir|hl7 (defaults: v7strict, cached, fhir), WASL_SEED, WASL_OUT.
#      WASL_LUNG=N adds N incidental lung-nodule loops (Fleischner 2017, second loop type; default 0 = screening cohort only).
#      WASL_DR=N adds N diabetic retinopathy screening loops (doh2024dr, third loop type; default 0).
#      WASL_DRNEXT=1 (with WASL_DR) tracks the next DR due date and a recall_overdue flag (pipeline/drnext.go; default off).
#      WASL_LUNGSURV=1 tracks the surveillance CT due after a PET/CT low-uptake lung closure (child loop; default 0 = off).
#      WASL_REFREJECT=true lets lung work-up and DR eye-clinic referrals be rejected (hand-off packets; default false).
#      WASL_BREAST=N adds N breast screening recall loops (doh2025breast, fourth loop type; default 0).
#      WASL_MODELSET=X runs the alternative linkage chain models/{encoder_X,lgb_stream_X,iso_X,iso_v7_X}.json (default: unset = the default chain).
#      Pilot switches (default off): WASL_SHADOW=1 shadow mode (every automatic closure is confirmed by a clinician first);
#      WASL_AUTOAUDIT=p post-closure clinician audit of a fraction p of automatic closures (normal mode). See pipeline/pilot.go.
# Live reader: llama-server -m gemma-3-4b-it-Q4_K_M.gguf --port 8791 -c 8192 -ngl 99 -np 4
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/bin"; OUT="${WASL_OUT:-$ROOT/out}"; SEED="${WASL_SEED:-2026}"
POLICY="${WASL_POLICY:-v7strict}"   # v7strict (canonical) | v7ltt (closure tier from Learn-then-Test) | v6 (legacy comparison)
SOURCE="${WASL_SOURCE:-fhir}"      # fhir (mock Malaffi FHIR Subscription) | hl7 (HL7 v2 over MLLP, production-shaped)
READER="${WASL_READER:-cached}"    # cached (Gemma 26B answers, no model needed) | live (local llama-server) 
READER_URL="${WASL_READER_URL:-http://127.0.0.1:8791}"
COHORT="$OUT/cohort"; BASE="$OUT/cohort-base"; RUN="$OUT/run/$POLICY-$READER-$SOURCE"; [ "${WASL_IMAGE:-off}" = off ] || RUN="$RUN-image"
LUNG="${WASL_LUNG:-0}"             # incidental lung-nodule loops (second loop type); 0 = off
[ "$LUNG" = 0 ] || RUN="$RUN-lung"
DR="${WASL_DR:-0}"                 # diabetic retinopathy screening loops (third loop type); 0 = off
[ "$DR" = 0 ] || RUN="$RUN-dr"
BREAST="${WASL_BREAST:-0}"         # breast screening recall loops (fourth loop type); 0 = off
[ "$BREAST" = 0 ] || RUN="$RUN-breast"
MODELSET="${WASL_MODELSET:-}"; [ -z "$MODELSET" ] || RUN="$RUN-$MODELSET"
MARGS=(); [ -z "$MODELSET" ] || MARGS=(-modelset "$MODELSET")
SHADOW="${WASL_SHADOW:-0}"; AUTOAUDIT="${WASL_AUTOAUDIT:-0}"   # pilot switches (pipeline/pilot.go); 0 = off
PARGS=(); [ "$SHADOW" = 0 ] || { PARGS+=(-shadow); RUN="$RUN-shadow"; }   # cohort generation never sees them
[ "$AUTOAUDIT" = 0 ] || { PARGS+=(-autoaudit "$AUTOAUDIT"); RUN="$RUN-autoaudit$AUTOAUDIT"; }
DRNEXT="${WASL_DRNEXT:-0}"         # DR next loop (pipeline/drnext.go): next due date + recall_overdue; gen adds follow-up visits; 0 = off
[ "$DRNEXT" = 0 ] || { PARGS+=(-drnext); RUN="$RUN-drnext"; }
LUNGSURV="${WASL_LUNGSURV:-0}"     # surveillance CT after a PET/CT low-uptake lung closure (pipeline/lungsurv.go); 0 = off
[ "$LUNGSURV" = 0 ] || { PARGS+=(-lungsurv); RUN="$RUN-lungsurv"; }   # engine side; the generator gets it through GARGS
# hand-off (default off): WASL_XREFLINK=1 a rejected intake that cites the referral but never linked raises a link question
[ "${WASL_XREFLINK:-0}" = 0 ] || { PARGS+=(-xreflink); RUN="$RUN-xreflink"; }
# WASL_SECONDIDCONFIRM=0: a contradicted second-identifier 'same' counts at once (-secondidconfirm=false; default on, v7.go)
[ "${WASL_SECONDIDCONFIRM:-1}" != 0 ] || { PARGS+=(-secondidconfirm=false); RUN="$RUN-nosecondidconfirm"; }
IMAGE="${WASL_IMAGE:-off}"         # off (default) | cached (models/image_verdicts.json) | live (python image/serve.py <model> <images>)
RARGS=(-reader "$READER" -reader-url "$READER_URL" -image "$IMAGE" -imgcache "$ROOT/models/image_verdicts.json" ${MARGS[@]+"${MARGS[@]}"} ${PARGS[@]+"${PARGS[@]}"})
GARGS=(); [ "$IMAGE" = off ] || GARGS=(-imgphotos -imgcache "$ROOT/models/image_verdicts.json")
[ "$DRNEXT" = 0 ] || GARGS+=(-drnext)
[ "$LUNGSURV" = 0 ] || GARGS+=(-lungsurv)
# H7 simulated-reviewer stress test and mitigations (all off by default): WASL_REVIEW_RHO=0.5, WASL_REVIEW_BLIND=1,
# WASL_REVIEW_ROTATE=1, WASL_SENIOR_AUDIT=0.02. Error rates and correlations are assumed stress-test values.
HARGS=()
[ -z "${WASL_REVIEW_RHO:-}" ] || HARGS+=(-rrho "$WASL_REVIEW_RHO")
[ "${WASL_REVIEW_BLIND:-0}" = 0 ] || HARGS+=(-reviewblind)
[ "${WASL_REVIEW_ROTATE:-0}" = 0 ] || HARGS+=(-reviewrotate)
[ -z "${WASL_SENIOR_AUDIT:-}" ] || HARGS+=(-senioraudit "$WASL_SENIOR_AUDIT")
# audit/tamper/report never call the model: they replay the run's reader ledger
AARGS=(-reader cached); [ "$READER" = cached ] || AARGS=(-reader replay)
IMG_A=cached; [ "$IMAGE" = live ] && IMG_A=replay; [ "$IMAGE" = off ] && IMG_A=off; AARGS+=(-image "$IMG_A" -imgcache "$ROOT/models/image_verdicts.json" ${MARGS[@]+"${MARGS[@]}"} ${PARGS[@]+"${PARGS[@]}"})
need() { echo "MISSING: $1"; echo "  → $2"; exit 1; }

check() {
  command -v go >/dev/null      || need "Go ≥1.22" "install from https://go.dev/dl/ (or: brew install go)"
  command -v python3 >/dev/null || need "python3" "install Python 3.10+"
  [ -x "$BIN/nats-server" ] || command -v nats-server >/dev/null || need "nats-server v2.10+" \
    "brew install nats-server   (or download from https://github.com/nats-io/nats-server/releases) — then: ln -s \$(command -v nats-server) bin/nats-server"
  [ -x "$BIN/nats-server" ] || { mkdir -p "$BIN"; ln -sf "$(command -v nats-server)" "$BIN/nats-server"; }
  for f in data/persons3.json models/encoder_weights.json models/lgb_stream.json models/iso.json models/gemma_cache.json models/parse_cache.json models/iso_v7.json \
           data/evidence/reports43.json data/evidence/pathology_templates.json patient-parsers/econsult_testset.json patient-parsers/teachback_testset.json; do
    [ -s "$ROOT/$f" ] || need "$f" "it is committed in the repo — re-clone or git lfs/pull"
  done
  if [ "$LUNG" != 0 ]; then
    for f in data/lung/finding_reports.json data/lung/followup_reports.json models/lung_gemma_cache.json; do
      [ -s "$ROOT/$f" ] || need "$f" "committed in the repo (corpus: scripts/lung_corpus.py; cache: wsi7 lungcache with llama-server on :8791)"
    done
  fi
  if [ "$DR" != 0 ]; then
    for f in data/dr/screen_reports.json data/dr/clinic_notes.json models/dr_gemma_cache.json; do
      [ -s "$ROOT/$f" ] || need "$f" "committed in the repo (corpus: scripts/dr_corpus.py; cache: wsi7 drcache with llama-server on :8791)"
    done
  fi
  if [ "$BREAST" != 0 ]; then
    for f in data/breast/screen_reports.json data/breast/assess_notes.json models/breast_gemma_cache.json; do
      [ -s "$ROOT/$f" ] || need "$f" "committed in the repo (corpus: scripts/breast_corpus.py; cache: wsi7 breastcache with llama-server on :8791)"
    done
  fi
  if [ -n "$MODELSET" ]; then for f in encoder_$MODELSET lgb_stream_$MODELSET iso_$MODELSET iso_v7_$MODELSET; do
    [ -s "$ROOT/models/$f.json" ] || need "models/$f.json" "WASL_MODELSET=$MODELSET needs the full chain (encoder/README.md)"; done; fi
  python3 -c "import numpy, scipy" 2>/dev/null || need "python3 numpy + scipy (booking engine)" "pip install numpy scipy"
  for p in 14822 14823 14824 28870 28880 28890; do p=$((p + ${WASL_PORT_OFFSET:-0}))
    if lsof -nP -iTCP:$p -sTCP:LISTEN >/dev/null 2>&1; then need "free port $p" "another Wasl run (or service) is listening; stop it first"; fi
  done
  echo "assets OK"
}
build()  { (cd "$ROOT/pipeline" && go vet . && go build -o "$BIN/wsi7" .); echo "built bin/wsi7"; }
test_()  { (cd "$ROOT/pipeline" && go test -count=1 .); }
cohort() {
  rm -rf "$BASE" "$COHORT" "$OUT/run/base"; mkdir -p "$BASE" "$COHORT"
  "$BIN/wsi7" gen -wasl "$ROOT/data" -reports "$ROOT/data/evidence/reports43.json" -pathology "$ROOT/data/evidence/pathology_templates.json" -prepquery="${WASL_PREPQUERY:-true}" -handoff="${WASL_HANDOFF:-true}" -lungworkup="${WASL_LUNGWORKUP:-true}" -refreject="${WASL_REFREJECT:-false}" \
    -ecpool "$ROOT/patient-parsers/econsult_testset.json" -tbpool "$ROOT/patient-parsers/teachback_testset.json" \
    -out "$BASE" -seed "$SEED" -persons 6500 -cerv 600 -lung "$LUNG" -lungdata "$ROOT/data/lung" -dr "$DR" -drdata "$ROOT/data/dr" -breast "$BREAST" -breastdata "$ROOT/data/breast" ${GARGS[@]+"${GARGS[@]}"} > "$OUT/gen.json"
  "$BIN/wsi7" sim -data "$BASE" -wasl "$ROOT/models" -run "$OUT/run/base" ${MARGS[@]+"${MARGS[@]}"} > "$OUT/run/base.json" 2> "$OUT/run/base.err"
  python3 "$ROOT/pipeline/augment_cohort.py" "$BASE" "$OUT/run/base" "$COHORT"
  echo "cohort → $COHORT"
}
run_() {
  if [ "$READER" = live ]; then  # ask the live model each unique report once, up front (ledger), so the stream isn't throttled
    (cd "$BIN" && ./wsi7 readerwarm -data "$COHORT" -run "$RUN" "${RARGS[@]}") || exit 1
  fi
  (cd "$BIN" && ./wsi7 orchestrate -prepquery="${WASL_PREPQUERY:-true}" -handoff="${WASL_HANDOFF:-true}" -lungworkup="${WASL_LUNGWORKUP:-true}" -data "$COHORT" -wasl "$ROOT/models" -run "$RUN" -preps 3 -threads 8 \
     -source "$SOURCE" -subkill 1 -chaos 2 -natskill -ann hnsw -rerr 0.04 -rdouble merge+close -dual -rseed 1 -seed "$SEED" -policy "$POLICY" "${RARGS[@]}" ${HARGS[@]+"${HARGS[@]}"} | tee "$RUN.stdout")
}
audit()  { (cd "$BIN" && ./wsi7 audit -data "$COHORT" -wasl "$ROOT/models" -run "$RUN" -ann hnsw -dual -policy "$POLICY" "${AARGS[@]}" > "$RUN/audit.stdout" 2>&1); echo "audit → $RUN/audit.stdout"; }
tamper() { (cd "$BIN" && ./wsi7 tamper -role auditor -data "$COHORT" -wasl "$ROOT/models" -run "$RUN" -ann hnsw -dual -policy "$POLICY" "${AARGS[@]}" > "$RUN/tamper.json" 2> "$RUN/tamper.log"); tail -3 "$RUN/tamper.log"; }
booking() {  # stage 6: simulated booking decision support over the run (WASL_SLOTS = slots per open case, default 1.2)
  python3 "$ROOT/booking/suggest.py" "$RUN" --slots-per-fit "${WASL_SLOTS:-1.2}" ${WASL_MISALLOCATED:+--misallocated}
}
report() {
  (cd "$BIN" && ./wsi7 report7 -data "$COHORT" -wasl "$ROOT/models" -run "$RUN" -ann hnsw -dual -policy "$POLICY" "${AARGS[@]}" > /dev/null)
  python3 "$ROOT/scripts/summary.py" "$RUN/report7.json"
}
serve() {
  echo "Wasl API on http://${WASL_API_ADDR:-127.0.0.1:8088} (run: $RUN)"
  (cd "$BIN" && exec ./wsi7 serve -run "$RUN" -data "$COHORT" -wasl "$ROOT/models" -policy "$POLICY" ${MARGS[@]+"${MARGS[@]}"} ${PARGS[@]+"${PARGS[@]}"} -addr "${WASL_API_ADDR:-127.0.0.1:8088}")
}
snapshot() {  # write static API responses for frontend development (no backend needed): docs/api-samples/
  local A="${WASL_API_ADDR:-127.0.0.1:8088}" D="$ROOT/docs/api-samples"
  mkdir -p "$D"
  curl -sf "http://$A/healthz" > "$D/healthz.json"
  curl -sf "http://$A/api/dashboard" > "$D/dashboard.json"
  curl -sf "http://$A/api/fixtures" > "$D/fixtures.json"
  for prog in crc cervical lung_nodule diabetic_retinopathy breast; do   # first page per programme present in the run
    curl -sf "http://$A/api/cases?programme=$prog&limit=25" > "$D/cases_${prog}_page1.json.tmp" &&
      python3 -c "import json,sys;sys.exit(0 if json.load(open('$D/cases_${prog}_page1.json.tmp')).get('cases') else 1)" &&
      mv "$D/cases_${prog}_page1.json.tmp" "$D/cases_${prog}_page1.json" || rm -f "$D/cases_${prog}_page1.json.tmp"
  done
  for tag in $(python3 -c "import json;print(' '.join(sorted(json.load(open('$D/fixtures.json'))['fixtures'])))"); do
    id=$(python3 -c "import json;print(json.load(open('$D/fixtures.json'))['fixtures']['$tag']['id'])")
    curl -sf "http://$A/api/cases/$id" > "$D/case_$tag.json"
    curl -sf "http://$A/api/cases/$id/audit" > "$D/case_${tag}_audit.json"
  done
  echo "API samples → $D"
}
mkdir -p "$OUT/run"
case "${1:-all}" in
  check) check;; build) build;; test) test_;; cohort) cohort;; run) run_;; audit) audit;; tamper) tamper;; report) report;;
  serve) serve;; snapshot) snapshot;;
  all) check; build; test_; cohort; run_; audit; tamper; booking; report;;
  booking) booking;;
  ltt) shift; LD="$ROOT/pipeline/results/ltt"; [ "${WASL_LTT_ALL:-0}" = 0 ] || LD="$ROOT/pipeline/results/ltt-all"
       "$ROOT/scripts/ltt_cohorts.sh" "$@" && python3 "$ROOT/scripts/ltt_select.py" "${WASL_LTT_DIR:-$LD}";;
  final) shift; exec "$ROOT/scripts/final_eval.sh" "$@";;
  *) echo "usage: $0 [all|check|build|test|cohort|run|audit|tamper|booking|report|serve|snapshot|final]"; exit 2;;
esac
