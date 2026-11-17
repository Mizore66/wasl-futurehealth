#!/usr/bin/env bash
# Wasl one-command baseline: check assets → build → cohort → integrated NATS run → audit → tamper → report.
# Usage: scripts/demo.sh [all|check|build|cohort|run|audit|tamper|booking|report|test|serve|snapshot|final|container]   (default: all)
#   container: check, cohort, run, audit, tamper, booking, report with a prebuilt binary (Dockerfile / docker-compose.yml; docs/DEPLOY.md)
#   ltt [seeds...]: Learn-then-Test calibration of the closure tier on cal-split cohorts -> pipeline/results/ltt/ (~40 min); WASL_POLICY=v7ltt uses it
#     WASL_LTT_ALL=1 also calibrates on lung, DR and breast loops -> pipeline/results/ltt-all/ (per-programme counts; scripts/ltt_cohorts.sh)
#   smoke: serve on loopback (WASL_SMOKE_PORT, WASL_SMOKE_WAIT s), run scripts/api_smoke.py over real HTTP, stop the server; part of `all`, not `container`
#     WASL_SERVE_RUN=<lane dir> / WASL_SERVE_DATA=<cohort dir> make serve and smoke use an existing materialised lane
#   final [seeds...]: stage 7 frozen evaluation, 5 seeds x {v1, v2 noise} -> pipeline/results/final/ (~45 min)
# Outputs: out/cohort/ (synthetic data), out/run/<policy>/ (logs, report7.json, dashboard.json, review_queue.json).
# Env: WASL_POLICY=v7strict|v6, WASL_READER=cached|live, WASL_SOURCE=fhir|hl7 (defaults: v7strict, cached, fhir), WASL_SEED, WASL_OUT.
#      WASL_LUNG=N adds N incidental lung-nodule loops (Fleischner 2017, second loop type; default 0 = screening cohort only).
#      WASL_DR=N adds N diabetic retinopathy screening loops (doh2024dr, third loop type; default 0).
#      WASL_DRNEXT=1 (with WASL_DR) tracks the next DR due date and a recall_overdue flag (pipeline/drnext.go; default off).
#      WASL_LUNGSURV=1 tracks the surveillance CT due after a PET/CT low-uptake lung closure (child loop; default 0 = off).
#      WASL_LUNGSURV2=1 (with WASL_LUNGSURV=1) adds the later surveillance CTs at about 1 and 2 years (BTS 2015; default 0 = off).
#      WASL_LUNGSURVSTABLE=1 gives the stable-CT surveillance child review items, truth and metrics (default 0 = off).
#      WASL_REFREJECT=true lets lung work-up and DR eye-clinic referrals be rejected (hand-off packets; default false).
#      WASL_BREAST=N adds N breast screening recall loops (doh2025breast, fourth loop type; default 0).
#      WASL_WRONGEID=p / WASL_WRONGREF=p adversarial cohorts (pipeline/genadv.go; default 0 = off): relatives' Emirates IDs / wrong referral numbers.
#      WASL_BREASTSHARE=1 lets breast loops use women who hold another programme's loop once the others run out (default 0).
#      WASL_LDCT=N adds N lung cancer screening LDCT loops (acr2022lungrads, fifth loop type; default 0).
#      WASL_IFHAS=N adds N IFHAS abnormal-result follow-up loops (doh2025ifhas, sixth loop type; default 0).
#      WASL_MODELSET=X runs the alternative linkage chain models/{encoder_X,lgb_stream_X,iso_X,iso_v7_X}.json (default: unset = the default chain).
#      Pilot switches (default off): WASL_SHADOW=1 shadow mode (every automatic closure is confirmed by a clinician first);
#      WASL_AUTOAUDIT=p post-closure clinician audit of a fraction p of automatic closures (normal mode). See pipeline/pilot.go.
#      Deployment (docs/DEPLOY.md): WASL_BIN=<dir with wsi7>; WASL_NATS_URL=<external cluster> with WASL_NATS_EXTERNAL=1.
#      Scale runs (pipeline/results/scale/): WASL_PERSONS, WASL_LOOPS, WASL_DISTRACT, WASL_CERV, WASL_SPLIT=all (training
#      persons included: not a held-out accuracy estimate), WASL_CHAOSSPAN=<s>, WASL_QUIETTIMEOUT=<duration>. Unset = defaults.
# Live reader: llama-server -m gemma-3-4b-it-Q4_K_M.gguf --port 8791 -c 8192 -ngl 99 -np 4
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="${WASL_BIN:-$ROOT/bin}"; OUT="${WASL_OUT:-$ROOT/out}"; SEED="${WASL_SEED:-2026}"
POLICY="${WASL_POLICY:-v7strict}"   # v7strict (canonical) | v7ltt (closure tier from Learn-then-Test) | v6 (legacy comparison)
SOURCE="${WASL_SOURCE:-fhir}"      # fhir (mock Malaffi FHIR Subscription) | hl7 (HL7 v2 over MLLP, production-shaped)
READER="${WASL_READER:-cached}"    # cached (Gemma 26B answers, no model needed) | live (local llama-server) 
READER_URL="${WASL_READER_URL:-http://127.0.0.1:8791}"
# Local synthetic-demo API credentials. Every bearer token is bound server-side to its role and immutable actor; these
# published development values are not production secrets. Override both the individual client tokens and
# WASL_API_TOKENS together for a non-default local configuration. Credentials 1 and 2 are ordinary initial readers;
# credential 3 is the senior (senior_reviewer / senior_clinician) who alone may submit the r3 resolution read.
export WASL_SYNTHETIC_DATA_ONLY=1
export WASL_API_REVIEWER_TOKEN="${WASL_API_REVIEWER_TOKEN:-wasl-demo-reviewer-1-token}"
# The web server's generic shared reads (dashboard, lists, case pages) use their own reviewer credential bound to a
# non-reader actor, so a first read concealed from everyone but its author is never shown under an initial reader's identity.
export WASL_API_WEB_REVIEWER_TOKEN="${WASL_API_WEB_REVIEWER_TOKEN:-wasl-demo-web-reviewer-token}"
export WASL_API_REVIEWER_2_TOKEN="${WASL_API_REVIEWER_2_TOKEN:-wasl-demo-reviewer-2-token}"
export WASL_API_REVIEWER_3_TOKEN="${WASL_API_REVIEWER_3_TOKEN:-wasl-demo-reviewer-3-token}"
export WASL_API_AUDITOR_TOKEN="${WASL_API_AUDITOR_TOKEN:-wasl-demo-auditor-token}"
export WASL_API_COORDINATOR_TOKEN="${WASL_API_COORDINATOR_TOKEN:-wasl-demo-coordinator-token}"
export WASL_API_CLINICIAN_TOKEN="${WASL_API_CLINICIAN_TOKEN:-wasl-demo-clinician-token}"
export WASL_API_CLINICIAN_2_TOKEN="${WASL_API_CLINICIAN_2_TOKEN:-wasl-demo-clinician-2-token}"
export WASL_API_CLINICIAN_3_TOKEN="${WASL_API_CLINICIAN_3_TOKEN:-wasl-demo-clinician-3-token}"
export WASL_API_TOKENS="${WASL_API_TOKENS:-$WASL_API_REVIEWER_TOKEN:reviewer:smoke-reviewer-1,$WASL_API_WEB_REVIEWER_TOKEN:reviewer:smoke-web-reviewer,$WASL_API_REVIEWER_2_TOKEN:reviewer:smoke-reviewer-2,$WASL_API_REVIEWER_3_TOKEN:senior_reviewer:smoke-reviewer-3,$WASL_API_AUDITOR_TOKEN:auditor:smoke-auditor,$WASL_API_COORDINATOR_TOKEN:coordinator:smoke-coordinator,$WASL_API_CLINICIAN_TOKEN:clinician:smoke-clinician,$WASL_API_CLINICIAN_2_TOKEN:clinician:smoke-clinician-2,$WASL_API_CLINICIAN_3_TOKEN:senior_clinician:smoke-clinician-3}"
# The web server's clinician token answers the receiving unit's referral inbox, so its actor is the endoscopy unit's clinician.
export WASL_RECEIVING_UNIT_CLINICIANS="${WASL_RECEIVING_UNIT_CLINICIANS:-skmc-endo:smoke-clinician}"
export WASL_REFERRING_CLINICIANS="${WASL_REFERRING_CLINICIANS:-}"
export WASL_API_ORIGINS="${WASL_API_ORIGINS:-}"
COHORT="$OUT/cohort"; BASE="$OUT/cohort-base"; RUN="$OUT/run/$POLICY-$READER-$SOURCE"; [ "${WASL_IMAGE:-off}" = off ] || RUN="$RUN-image"
LUNG="${WASL_LUNG:-0}"             # incidental lung-nodule loops (second loop type); 0 = off
[ "$LUNG" = 0 ] || RUN="$RUN-lung"
DR="${WASL_DR:-0}"                 # diabetic retinopathy screening loops (third loop type); 0 = off
[ "$DR" = 0 ] || RUN="$RUN-dr"
BREAST="${WASL_BREAST:-0}"         # breast screening recall loops (fourth loop type); 0 = off
[ "$BREAST" = 0 ] || RUN="$RUN-breast"
LDCT="${WASL_LDCT:-0}"             # lung cancer screening LDCT loops (fifth loop type); 0 = off
[ "$LDCT" = 0 ] || RUN="$RUN-ldct"
IFHAS="${WASL_IFHAS:-0}"           # IFHAS abnormal-result follow-up loops (pipeline/ifhas.go); 0 = off
[ "$IFHAS" = 0 ] || { GARGS_IFHAS=(-ifhas "$IFHAS" -ifhasdata "$ROOT/data/ifhas"); RUN="$RUN-ifhas"; }
MODELSET="${WASL_MODELSET:-}"; [ -z "$MODELSET" ] || RUN="$RUN-$MODELSET"
MARGS=(); [ -z "$MODELSET" ] || MARGS=(-modelset "$MODELSET")
SHADOW="${WASL_SHADOW:-0}"; AUTOAUDIT="${WASL_AUTOAUDIT:-0}"   # pilot switches (pipeline/pilot.go); 0 = off
PARGS=(); [ "$SHADOW" = 0 ] || { PARGS+=(-shadow); RUN="$RUN-shadow"; }   # cohort generation never sees them
[ "$AUTOAUDIT" = 0 ] || { PARGS+=(-autoaudit "$AUTOAUDIT"); RUN="$RUN-autoaudit$AUTOAUDIT"; }
DRNEXT="${WASL_DRNEXT:-0}"         # DR next loop (pipeline/drnext.go): next due date + recall_overdue; gen adds follow-up visits; 0 = off
[ "$DRNEXT" = 0 ] || { PARGS+=(-drnext); RUN="$RUN-drnext"; }
LUNGSURV="${WASL_LUNGSURV:-0}"     # surveillance CT after a PET/CT low-uptake lung closure (pipeline/lungsurv.go); 0 = off
[ "$LUNGSURV" = 0 ] || { PARGS+=(-lungsurv); RUN="$RUN-lungsurv"; }   # engine side; the generator gets it through GARGS
# WASL_HISTCLOCK=1 judges look-back history (lung/DR/breast inputs arriving before day 0) at its arrival time, not at
# the day-0 clock (pipeline/linker.go HistClock; default off)
[ "${WASL_HISTCLOCK:-0}" = 0 ] || { PARGS+=(-histclock); RUN="$RUN-histclock"; }
LUNGSURV2="${WASL_LUNGSURV2:-0}"   # later surveillance CTs (1 and 2 years) after a PET/CT low-uptake closure; needs WASL_LUNGSURV=1; 0 = off
[ "$LUNGSURV2" = 0 ] || [ "$LUNGSURV" = 0 ] || { PARGS+=(-lungsurv2); RUN="$RUN-lungsurv2"; }
LUNGSURVSTABLE="${WASL_LUNGSURVSTABLE:-0}"   # stable-CT surveillance child: review items, truth and metrics (lungsurv.go); 0 = off
[ "$LUNGSURVSTABLE" = 0 ] || { PARGS+=(-lungsurvstable); RUN="$RUN-lungsurvstable"; }
# hand-off (default off): WASL_XREFLINK=1 a rejected intake that cites the referral but never linked raises a link question
[ "${WASL_XREFLINK:-0}" = 0 ] || { PARGS+=(-xreflink); RUN="$RUN-xreflink"; }
# WASL_SECONDIDCONFIRM=0: a contradicted second-identifier 'same' counts at once (-secondidconfirm=false; default on, v7.go)
[ "${WASL_SECONDIDCONFIRM:-1}" != 0 ] || { PARGS+=(-secondidconfirm=false); RUN="$RUN-nosecondidconfirm"; }
# DR / lung outreach gaps (pipeline/outreachgaps.go, default off): WASL_DRRECALLMSG=1 overdue next-due recall message (with
# WASL_DRNEXT), WASL_REJMSG=1 rejected-referral message for lung / DR, WASL_DRSEENLINK=1 untrusted later eye-clinic note asks
[ "${WASL_DRRECALLMSG:-0}" = 0 ] || { PARGS+=(-drrecallmsg); RUN="$RUN-drrecallmsg"; }
[ "${WASL_REJMSG:-0}" = 0 ] || { PARGS+=(-rejmsg); RUN="$RUN-rejmsg"; }
[ "${WASL_DRSEENLINK:-0}" = 0 ] || { PARGS+=(-drseenlink); RUN="$RUN-drseenlink"; }
IMAGE="${WASL_IMAGE:-off}"         # off (default) | cached (models/image_verdicts.json) | live (python image/serve.py <model> <images>)
RARGS=(-reader "$READER" -reader-url "$READER_URL" -image "$IMAGE" -imgcache "$ROOT/models/image_verdicts.json" ${MARGS[@]+"${MARGS[@]}"} ${PARGS[@]+"${PARGS[@]}"})
GARGS=(); [ "$IMAGE" = off ] || GARGS=(-imgphotos -imgcache "$ROOT/models/image_verdicts.json")
[ "$DRNEXT" = 0 ] || GARGS+=(-drnext)
[ "$LUNGSURV" = 0 ] || GARGS+=(-lungsurv)
# adversarial generator cases (default off; pipeline/genadv.go): WASL_WRONGEID=p a share p of registrations carries a
# relative's Emirates ID (B8); WASL_WRONGREF=p a share p of referral intakes cites the wrong referral (B9). Assumed rates.
WRONGEID="${WASL_WRONGEID:-0}"; WRONGREF="${WASL_WRONGREF:-0}"
[ "$WRONGEID" = 0 ] || { GARGS+=(-wrongeid "$WRONGEID"); RUN="$RUN-wrongeid$WRONGEID"; }
[ "$WRONGREF" = 0 ] || { GARGS+=(-wrongref "$WRONGREF"); RUN="$RUN-wrongref$WRONGREF"; }
# WASL_BREASTSHARE=1 (gen, default off): breast loops beyond the women with no other loop use women with another loop
[ "${WASL_BREASTSHARE:-0}" = 0 ] || GARGS+=(-breastshare)
# WASL_SHOWCASE=N (gen, default 0): after the other breast loops, one breast loop each for the first N women whose CRC and
# cervical loops both end open at an outreach step: the deterministic source of the published cross-programme person journey
[ "${WASL_SHOWCASE:-0}" = 0 ] || { GARGS+=(-breastshowcase "$WASL_SHOWCASE"); RUN="$RUN-showcase$WASL_SHOWCASE"; }
[ "$LUNGSURV2" = 0 ] || [ "$LUNGSURV" = 0 ] || GARGS+=(-lungsurv2)
[ "$LUNGSURVSTABLE" = 0 ] || GARGS+=(-lungsurvstable)
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
SARGS=()   # scale-run cohort overrides; unset keeps the default command line
[ -z "${WASL_LOOPS:-}" ] || SARGS+=(-loops "$WASL_LOOPS")
[ -z "${WASL_DISTRACT:-}" ] || SARGS+=(-distract "$WASL_DISTRACT")
[ -z "${WASL_SPLIT:-}" ] || SARGS+=(-split "$WASL_SPLIT")
OARGS=()
[ -z "${WASL_CHAOSSPAN:-}" ] || OARGS+=(-chaosspan "$WASL_CHAOSSPAN")
[ -z "${WASL_QUIETTIMEOUT:-}" ] || OARGS+=(-quiettimeout "$WASL_QUIETTIMEOUT")
need() { echo "MISSING: $1"; echo "  → $2"; exit 1; }
validate_nats_config() {
  if [ -n "${WASL_NATS_URL:-}" ] && [ "${WASL_NATS_EXTERNAL:-}" != 1 ]; then
    echo "configuration error: WASL_NATS_URL requires WASL_NATS_EXTERNAL=1" >&2; exit 2
  fi
  if [ -z "${WASL_NATS_URL:-}" ] && [ "${WASL_NATS_EXTERNAL:-}" = 1 ]; then
    echo "configuration error: WASL_NATS_EXTERNAL=1 requires WASL_NATS_URL" >&2; exit 2
  fi
}

check() {
  { [ -n "${WASL_BIN:-}" ] && [ -x "$BIN/wsi7" ]; } || command -v go >/dev/null || need "Go ≥1.22" "install from https://go.dev/dl/ (or: brew install go)"
  command -v python3 >/dev/null || need "python3" "install Python 3.10+"
  if [ "${WASL_NATS_EXTERNAL:-}" != 1 ]; then   # a valid external pair needs no local nats-server
  [ -x "$BIN/nats-server" ] || command -v nats-server >/dev/null || need "nats-server v2.10+" \
    "brew install nats-server   (or download from https://github.com/nats-io/nats-server/releases) — then: ln -s \$(command -v nats-server) bin/nats-server"
  [ -x "$BIN/nats-server" ] || { mkdir -p "$BIN"; ln -sf "$(command -v nats-server)" "$BIN/nats-server"; }
  fi
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
  if [ "$LDCT" != 0 ]; then
    for f in data/ldct/screen_reports.json data/ldct/workup_notes.json models/ldct_gemma_cache.json; do
      [ -s "$ROOT/$f" ] || need "$f" "committed in the repo (corpus: scripts/ldct_corpus.py; cache: wsi7 ldctcache with llama-server on :8791)"
    done
  fi
  if [ "$IFHAS" != 0 ]; then
    for f in data/ifhas/followup_notes.json models/ifhas_gemma_cache.json; do
      [ -s "$ROOT/$f" ] || need "$f" "committed in the repo (corpus: scripts/ifhas_corpus.py; cache: wsi7 ifhascache with llama-server on :8791)"
    done
  fi
  if [ -n "$MODELSET" ]; then for f in encoder_$MODELSET lgb_stream_$MODELSET iso_$MODELSET iso_v7_$MODELSET; do
    [ -s "$ROOT/models/$f.json" ] || need "models/$f.json" "WASL_MODELSET=$MODELSET needs the full chain (encoder/README.md)"; done; fi
  python3 -c "import numpy, scipy" 2>/dev/null || need "python3 numpy + scipy (booking engine)" "pip install numpy scipy"
  NPORTS="14822 14823 14824"; [ "${WASL_NATS_EXTERNAL:-}" != 1 ] || NPORTS=""
  for p in $NPORTS 28870 28880 28890; do p=$((p + ${WASL_PORT_OFFSET:-0}))
    if netstat -tln 2>/dev/null | grep -E ":${p}[[:space:]]" >/dev/null; then need "free port $p" "another Wasl run (or service) is listening; stop it first"; fi
  done
  echo "assets OK"
}
build()  { (cd "$ROOT/pipeline" && go vet . && go build -o "$BIN/wsi7" .); echo "built bin/wsi7"; }
test_()  { (cd "$ROOT/pipeline" && go test -count=1 .); }
cohort() {
  rm -rf "$BASE" "$COHORT" "$OUT/run/base"; mkdir -p "$BASE" "$COHORT"
  "$BIN/wsi7" gen -wasl "$ROOT/data" -reports "$ROOT/data/evidence/reports43.json" -pathology "$ROOT/data/evidence/pathology_templates.json" -prepquery="${WASL_PREPQUERY:-true}" -handoff="${WASL_HANDOFF:-true}" -lungworkup="${WASL_LUNGWORKUP:-true}" -refreject="${WASL_REFREJECT:-false}" \
    -ecpool "$ROOT/patient-parsers/econsult_testset.json" -tbpool "$ROOT/patient-parsers/teachback_testset.json" \
    -out "$BASE" -seed "$SEED" -persons "${WASL_PERSONS:-6500}" -cerv "${WASL_CERV:-600}" ${SARGS[@]+"${SARGS[@]}"} -lung "$LUNG" -lungdata "$ROOT/data/lung" -dr "$DR" -drdata "$ROOT/data/dr" -breast "$BREAST" -breastdata "$ROOT/data/breast" -ldct "$LDCT" -ldctdata "$ROOT/data/ldct" ${GARGS[@]+"${GARGS[@]}"} ${GARGS_IFHAS[@]+"${GARGS_IFHAS[@]}"} > "$OUT/gen.json"
  "$BIN/wsi7" sim -data "$BASE" -wasl "$ROOT/models" -run "$OUT/run/base" ${MARGS[@]+"${MARGS[@]}"} > "$OUT/run/base.json" 2> "$OUT/run/base.err"
  python3 "$ROOT/pipeline/augment_cohort.py" "$BASE" "$OUT/run/base" "$COHORT"
  echo "cohort → $COHORT"
}
run_() {
  if [ "$READER" = live ]; then  # ask the live model each unique report once, up front (ledger), so the stream isn't throttled
    (cd "$BIN" && ./wsi7 readerwarm -data "$COHORT" -run "$RUN" "${RARGS[@]}") || exit 1
  fi
  (cd "$BIN" && ./wsi7 orchestrate -prepquery="${WASL_PREPQUERY:-true}" -handoff="${WASL_HANDOFF:-true}" -lungworkup="${WASL_LUNGWORKUP:-true}" -data "$COHORT" -wasl "$ROOT/models" -run "$RUN" -preps 3 -threads 8 \
     -source "$SOURCE" -subkill 1 -chaos 2 -natskill -ann hnsw -rerr 0.04 -rdouble merge+close -dual -rseed 1 -seed "$SEED" -policy "$POLICY" "${RARGS[@]}" ${HARGS[@]+"${HARGS[@]}"} ${OARGS[@]+"${OARGS[@]}"} | tee "$RUN.stdout")
}
audit()  { (cd "$BIN" && ./wsi7 audit -data "$COHORT" -wasl "$ROOT/models" -run "$RUN" -ann hnsw -dual -policy "$POLICY" "${AARGS[@]}" > "$RUN/audit.stdout" 2>&1); echo "audit → $RUN/audit.stdout"; }
tamper() { (cd "$BIN" && ./wsi7 tamper -role auditor -data "$COHORT" -wasl "$ROOT/models" -run "$RUN" -ann hnsw -dual -policy "$POLICY" "${AARGS[@]}" > "$RUN/tamper.json" 2> "$RUN/tamper.log"); tail -3 "$RUN/tamper.log"; }
booking() {  # stage 6: simulated booking decision support over the run (WASL_SLOTS = slots per open case, default 1.2)
  # WASL_OPTIMISER=lookahead: multi-period rolling-horizon optimiser (booking/booking/lookahead.py; needs bin/wsi7); default myopic
  python3 "$ROOT/booking/suggest.py" "$RUN" --slots-per-fit "${WASL_SLOTS:-1.2}" ${WASL_MISALLOCATED:+--misallocated} ${WASL_OPTIMISER:+--optimiser "$WASL_OPTIMISER"}
  python3 "$ROOT/booking/recovery.py" "$RUN" --slots-per-fit "${WASL_SLOTS:-1.2}"
}
report() {
  (cd "$BIN" && ./wsi7 report7 -data "$COHORT" -wasl "$ROOT/models" -run "$RUN" -ann hnsw -dual -policy "$POLICY" "${AARGS[@]}" > /dev/null)
  python3 "$ROOT/scripts/summary.py" "$RUN/report7.json"
}
serve() {  # WASL_SERVE_RUN / WASL_SERVE_DATA serve an already materialised lane and cohort (no regeneration, no model calls)
  local addr="${1:-${WASL_API_ADDR:-127.0.0.1:8088}}" run="${WASL_SERVE_RUN:-$RUN}" data="${WASL_SERVE_DATA:-$COHORT}"
  printf '{"level":"INFO","msg":"api starting","address":"%s","run":"%s"}\n' "$addr" "$run"
  cd "$BIN" && exec ./wsi7 serve -run "$run" -data "$data" -wasl "$ROOT/models" -policy "$POLICY" ${MARGS[@]+"${MARGS[@]}"} ${PARGS[@]+"${PARGS[@]}"} -addr "$addr"
}
smoke_source_out_guard() {
  # Must run before even global OUT creation: a rejected destination must not
  # add directories to the original evidence. Resolve root symlinks first.
  python3 -I - "${WASL_SERVE_RUN:-$RUN}" "$OUT" <<'PY'
import os, sys
source, output = map(os.path.realpath, sys.argv[1:])
if os.path.commonpath((source, output)) == source:
    raise SystemExit("smoke: output is inside source run (or equal to it)")
PY
}
smoke() {  # Real-run checks + separate built synthetic software fixture, both over real loopback HTTP.
  smoke_source_out_guard
  local port="${WASL_SMOKE_PORT:-$((18088 + ${WASL_PORT_OFFSET:-0}))}" wait="${WASL_SMOKE_WAIT:-60}" addr fixture_addr stage rc=0 i=0 ok=0
  addr="127.0.0.1:$port"; fixture_addr="127.0.0.1:$((port + 1))"
  # Check both adjacent loopback ports before starting either listener. Bind
  # failures at startup still fail closed; health from another process is not proof.
  python3 -I - "$port" <<'PY'
import socket, sys
port = int(sys.argv[1])
if not 1 <= port < 65535:
    raise SystemExit("smoke: two adjacent ports required")
with socket.socket() as actual, socket.socket() as fixture:
    actual.bind(("127.0.0.1", port))
    fixture.bind(("127.0.0.1", port + 1))
PY
  mkdir -p "$OUT"
  stage=$(mktemp -d "$OUT/smoke-lanes.XXXXXX")
  # Preserve all source bytes/history; only newly created destination objects
  # receive private writable modes. A root run symlink is allowed, internal
  # symlinks/special files and recursive destinations are rejected before copy.
  python3 -I - "${WASL_SERVE_RUN:-$RUN}" "$stage/actual" <<'PY'
import os, shutil, stat, sys
source, destination = map(os.path.realpath, sys.argv[1:])
if os.path.commonpath((source, destination)) == source:
    raise SystemExit("smoke: destination is inside source run")
for root, dirs, files in os.walk(source, followlinks=False):
    for name in dirs + files:
        path = os.path.join(root, name)
        mode = os.lstat(path).st_mode
        if stat.S_ISLNK(mode):
            raise SystemExit("smoke: internal source symlink rejected: " + path)
        if not (stat.S_ISDIR(mode) or stat.S_ISREG(mode)):
            raise SystemExit("smoke: nonregular source entry rejected: " + path)
def copy_private(src, dst):
    os.mkdir(dst, 0o700)
    os.chmod(dst, 0o700)
    with os.scandir(src) as entries:
        for entry in entries:
            target = os.path.join(dst, entry.name)
            if entry.is_symlink():
                raise SystemExit("smoke: internal source symlink rejected: " + entry.path)
            if entry.is_dir(follow_symlinks=False):
                copy_private(entry.path, target)
            elif entry.is_file(follow_symlinks=False):
                fd = os.open(entry.path, os.O_RDONLY | os.O_NOFOLLOW)
                with os.fdopen(fd, "rb") as original:
                    if not stat.S_ISREG(os.fstat(original.fileno()).st_mode):
                        raise SystemExit("smoke: nonregular source file rejected")
                    out = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
                    with os.fdopen(out, "wb") as copied:
                        os.fchmod(copied.fileno(), 0o600)
                        shutil.copyfileobj(original, copied)
            else:
                raise SystemExit("smoke: nonregular source entry rejected: " + entry.path)
copy_private(source, destination)
PY
  "$BIN/wsi7" smoke-fixture -out "$stage/independent"
  printf 'smoke: retained writable actual lane %s; independent lane %s is synthetic software evidence only\n' "$stage/actual" "$stage/independent"
  SMOKE_PID=""; SMOKE_FIXTURE_PID=""
  trap '[ -z "$SMOKE_PID" ] || kill "$SMOKE_PID" 2>/dev/null || true; [ -z "$SMOKE_FIXTURE_PID" ] || kill "$SMOKE_FIXTURE_PID" 2>/dev/null || true' EXIT
  WASL_SERVE_RUN="$stage/actual" serve "$addr" > "$stage/actual-serve.log" 2>&1 &
  SMOKE_PID=$!; local pid=$SMOKE_PID
  # No model/pilot options or cohort data are needed for the explicit software lane.
  "$BIN/wsi7" serve -run "$stage/independent" -data "" -addr "$fixture_addr" > "$stage/independent-serve.log" 2>&1 &
  SMOKE_FIXTURE_PID=$!; local fixture_pid=$SMOKE_FIXTURE_PID
  while [ "$i" -lt "$((wait * 2))" ]; do
    kill -0 "$pid" 2>/dev/null && kill -0 "$fixture_pid" 2>/dev/null || break
    if curl -sf --max-time 2 "http://$addr/healthz" >/dev/null 2>&1 && curl -sf --max-time 2 "http://$fixture_addr/healthz" >/dev/null 2>&1; then
      # Both newly spawned processes must have actually acquired their listeners.
      if grep -q 'api listener ready' "$stage/actual-serve.log" && grep -q 'api listener ready' "$stage/independent-serve.log"; then ok=1; break; fi
    fi
    sleep 0.5; i=$((i + 1))
  done
  if [ "$ok" != 1 ]; then echo "smoke: both servers must become healthy (see $stage/actual-serve.log and $stage/independent-serve.log)" >&2; rc=1
  else python3 "$ROOT/scripts/api_smoke.py" --base-url "http://$addr" --workflow-base-url "http://$fixture_addr" --independent-base-url "http://$fixture_addr" > "$stage/smoke.log" 2>&1 || rc=$?; fi
  [ ! -f "$stage/smoke.log" ] || cat "$stage/smoke.log"
  kill "$pid" "$fixture_pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true; wait "$fixture_pid" 2>/dev/null || true
  SMOKE_PID=""; SMOKE_FIXTURE_PID=""
  return "$rc"
}
# Cleanup for snapshot: staging is always removed; if the swap into place was interrupted (SNAP_SWAP=1) the previous samples
# are restored from SNAP_OLD first. A SIGKILL cannot run this: it may leave <dir>.staging.* / <dir>.old.* beside the samples
# (and, mid-swap, the samples dir short of its old files, which are then in <dir>.old.*): move them back by hand.
snapshot_cleanup() {
  if [ -n "${SNAP_OLD:-}" ] && [ -d "$SNAP_OLD" ]; then
    if [ "${SNAP_SWAP:-0}" = 1 ]; then
      find "$SNAP_DIR" -maxdepth 1 -type f ! -name README.md -delete 2>/dev/null || true
      find "$SNAP_OLD" -maxdepth 1 -type f -exec mv {} "$SNAP_DIR"/ \; 2>/dev/null || true
    fi
    rm -rf "$SNAP_OLD"
  fi
  [ -z "${SNAP_STAGE:-}" ] || rm -rf "$SNAP_STAGE"
  SNAP_OLD=""; SNAP_STAGE=""; SNAP_SWAP=0
}
snapshot() {  # static API responses for frontend development (no backend needed): docs/api-samples/ (WASL_SNAPSHOT_DIR overrides)
  # Written into a clean staging directory first; all seven (or the WASL_SNAPSHOT_PROGRAMMES) programme pages must be nonempty. Only then
  # are the old samples (README.md kept) moved aside, the complete staging moved into place and the old ones deleted, so no stale
  # fixture file remains and a failure or INT/TERM at any point puts the previous samples back. Every curl is bounded by
  # WASL_SNAPSHOT_MAX_TIME seconds (default 30).
  # WASL_SNAPSHOT_PROGRAMMES=<comma list> scopes the required programme pages (default: all seven); each listed one must be nonempty.
  local A="${WASL_API_ADDR:-127.0.0.1:8088}" D="${WASL_SNAPSHOT_DIR:-$ROOT/docs/api-samples}" S tag id prog f
  local MT="${WASL_SNAPSHOT_MAX_TIME:-30}"
  local progs="${WASL_SNAPSHOT_PROGRAMMES:-crc,cervical,lung_nodule,diabetic_retinopathy,breast,lung_ldct,ifhas}"
  for prog in ${progs//,/ }; do case "$prog" in crc|cervical|lung_nodule|diabetic_retinopathy|breast|lung_ldct|ifhas) ;;
    *) echo "snapshot: unknown programme '$prog' in WASL_SNAPSHOT_PROGRAMMES" >&2; return 1;; esac; done
  SNAP_DIR="${D%/}"; SNAP_SWAP=0; SNAP_OLD=""
  S="$(mktemp -d "$SNAP_DIR.staging.XXXXXX")"
  SNAP_STAGE="$S"
  trap 'snapshot_cleanup' EXIT; trap 'snapshot_cleanup; exit 130' INT; trap 'snapshot_cleanup; exit 143' TERM  # nothing outlives the run
  curl -sf --max-time "$MT" "http://$A/healthz" |
    python3 "$ROOT/scripts/redacted_snapshot.py" health > "$S/healthz.json" || { echo "snapshot: healthz failed" >&2; return 1; }
  curl -sf --max-time "$MT" -H "Authorization: Bearer $WASL_API_WEB_REVIEWER_TOKEN" "http://$A/api/operations" |
    python3 "$ROOT/scripts/redacted_snapshot.py" operations > "$S/operations.json" || { echo "snapshot: operations failed" >&2; return 1; }
  curl -sf --max-time "$MT" -H "Authorization: Bearer $WASL_API_REVIEWER_TOKEN" "http://$A/api/dashboard" |
    python3 "$ROOT/scripts/redacted_snapshot.py" dashboard > "$S/dashboard.json" || { echo "snapshot: dashboard failed" >&2; return 1; }
  curl -sf --max-time "$MT" -H "Authorization: Bearer $WASL_API_REVIEWER_TOKEN" "http://$A/api/fixtures" |
    python3 "$ROOT/scripts/redacted_snapshot.py" fixtures > "$S/fixtures.json" || { echo "snapshot: fixtures failed" >&2; return 1; }
  curl -sf --max-time "$MT" -H "Authorization: Bearer $WASL_API_WEB_REVIEWER_TOKEN" "http://$A/api/recovery/slots" |
    python3 "$ROOT/scripts/redacted_snapshot.py" recovery-list > "$S/recovery_slots.json" || { echo "snapshot: recovery slots failed" >&2; return 1; }
  id=$(python3 -c "import json,sys; slots=json.load(open(sys.argv[1])).get('slots', []); print(slots[0]['slot_id'] if slots else '')" "$S/recovery_slots.json")
  if [ -n "$id" ]; then
    curl -sf --max-time "$MT" -H "Authorization: Bearer $WASL_API_WEB_REVIEWER_TOKEN" "http://$A/api/recovery/slots/$id" |
      python3 "$ROOT/scripts/redacted_snapshot.py" recovery-detail > "$S/recovery_slot.json" || { echo "snapshot: recovery detail failed" >&2; return 1; }
  fi
  # The published person is the API's person_journey fixture (pipeline/api_people.go pickPersonFixture). Only an explicit
  # WASL_SNAPSHOT_PLAIN_PERSON=1 publishes the evidence_closure person instead, unchecked, for runs without one.
  local check="$ROOT/scripts/person_showcase.py"
  id=$(python3 -c "import json,sys;print(json.load(open(sys.argv[1]))['fixtures'].get('person_journey', {}).get('id', ''))" "$S/fixtures.json")
  if [ -z "$id" ]; then
    [ "${WASL_SNAPSHOT_PLAIN_PERSON:-}" = 1 ] ||
      { echo "snapshot: run has no actionable cross-programme person_journey fixture; samples not replaced (WASL_SNAPSHOT_PLAIN_PERSON=1 publishes the evidence_closure person)" >&2; return 1; }
    echo "snapshot: WARNING: no person_journey fixture; publishing the evidence_closure person, which is not a cross-programme showcase" >&2
    id=$(python3 -c "import json,sys;print(json.load(open(sys.argv[1]))['fixtures']['evidence_closure']['id'])" "$S/fixtures.json")
    check=""
  fi
  { curl -sfG --max-time "$MT" -H "Authorization: Bearer $WASL_API_WEB_REVIEWER_TOKEN" --data-urlencode "case_id=$id" "http://$A/api/people" |
    python3 "$ROOT/scripts/redacted_snapshot.py" person > "$S/person_journey.json" &&
    { [ -z "$check" ] || python3 "$check" "$S/person_journey.json"; }; } || { echo "snapshot: person journey failed" >&2; return 1; }
  for prog in ${progs//,/ }; do
    if ! { curl -sf --max-time "$MT" -H "Authorization: Bearer $WASL_API_REVIEWER_TOKEN" "http://$A/api/cases?programme=$prog&limit=25" |
      python3 "$ROOT/scripts/redacted_snapshot.py" cases > "$S/cases_${prog}_page1.json" &&
      python3 -c "import json,sys;sys.exit(0 if json.load(open(sys.argv[1])).get('cases') else 1)" "$S/cases_${prog}_page1.json"; }; then
      echo "snapshot: programme page '$prog' is empty or failed; samples not replaced" >&2; return 1
    fi
  done
  for tag in $(python3 -c "import json,sys;print(' '.join(sorted(json.load(open(sys.argv[1]))['fixtures'])))" "$S/fixtures.json"); do
    id=$(python3 -c "import json,sys;print(json.load(open(sys.argv[1]))['fixtures'][sys.argv[2]]['id'])" "$S/fixtures.json" "$tag")
    # Public fixtures are built only from the redacted auditor export, never from raw clinical detail/audit routes.
    curl -sf --max-time "$MT" -H "Authorization: Bearer $WASL_API_AUDITOR_TOKEN" "http://$A/api/cases/$id/export" > "$S/$tag.export" &&
      python3 "$ROOT/scripts/redacted_snapshot.py" detail < "$S/$tag.export" > "$S/case_$tag.json" &&
      python3 "$ROOT/scripts/redacted_snapshot.py" audit < "$S/$tag.export" > "$S/case_${tag}_audit.json" ||
      { echo "snapshot: fixture $tag redacted export failed" >&2; return 1; }
  done
  python3 "$ROOT/scripts/redacted_snapshot.py" scan-dir "$S" ||
    { echo "snapshot: staged output contains a private canary; samples not replaced" >&2; return 1; }
  mkdir -p "$SNAP_DIR"
  SNAP_OLD="$(mktemp -d "$SNAP_DIR.old.XXXXXX")"
  SNAP_SWAP=1
  find "$SNAP_DIR" -maxdepth 1 -type f ! -name README.md -exec mv {} "$SNAP_OLD"/ \; || return 1
  for f in "$S"/*.json; do mv "$f" "$SNAP_DIR"/ || return 1; done
  SNAP_SWAP=0                   # complete: the old samples are deleted only now
  snapshot_cleanup; trap - EXIT INT TERM
  echo "API samples → $D"
}
# A run must use a binary built from the current sources. The 2 Nov X10 rerun used a bin/wsi7 older than its fixes and
# reproduced the old crashes; only a prebuilt WASL_BIN (container mode) is trusted as given.
stale_bin() {
  [ -z "${WASL_BIN:-}" ] || return 0
  [ -x "$BIN/wsi7" ] || return 0
  local newer; newer="$(find "$ROOT/pipeline" -maxdepth 1 \( -name '*.go' -o -name go.mod -o -name go.sum \) -newer "$BIN/wsi7" | head -1)"
  [ -z "$newer" ] || { echo "stale binary: $BIN/wsi7 is older than ${newer#$ROOT/}; run: scripts/demo.sh build" >&2; exit 2; }
}
# Encrypted bundles use the versioned keyring recovery_keys.json; backup writes chunked v3 bundles and v2 bundles stay
# restorable. A legacy HMAC-only bundle needs WASL_RECOVERY_LEGACY_HMAC=1 and WASL_RECOVERY_KEY_FILE pointing at the old
# backup_key; it is never a default.
recovery_key_file() { echo "${WASL_RECOVERY_KEY_FILE:-/run/secrets/recovery_keys.json}"; }
# The NATS payload is streamed from `backup-nats -stream` through an inherited pipe and encrypted chunk by chunk as it
# arrives: no plaintext NATS file is written to scratch space, and memory is bounded by the chunk and record sizes.
recovery_backup() {
  local directory="${WASL_RECOVERY_DIR:-/backup}" key; key="$(recovery_key_file)"
  [ "${WASL_RECOVERY_LEGACY_HMAC:-0}" != "1" ] || { echo "legacy HMAC bundles are read-only; backup always writes v3" >&2; exit 1; }
  mkdir -p "$directory"
  [ ! -e "$directory/bundle" ] || { echo "recovery backup already exists: $directory/bundle" >&2; exit 1; }
  local chunk=()
  [ -z "${WASL_RECOVERY_CHUNK_BYTES:-}" ] || chunk=(--chunk-size "$WASL_RECOVERY_CHUNK_BYTES")
  python3 "$ROOT/scripts/recovery.py" backup "$OUT" "$directory/bundle" --key-file "$key" \
    --nats-command "$(printf '%q' "$BIN/wsi7") backup-nats -stream -bundle" ${chunk[@]+"${chunk[@]}"}
}
# Order: authenticate, stage and decrypt the files, validate the NATS payload offline (check-nats), restore NATS, and
# only then install the file generation. A file-side failure (wrong key, tamper, populated $OUT without
# WASL_RECOVERY_REPLACE=1, staging error) or a NATS payload validation failure therefore happens before NATS is touched.
# If restore-nats itself fails, JetStream may be partly populated: purge it manually before retrying (the empty-target
# guard is deliberately not relaxed). A chunked bundle's NATS payload is streamed through a pipe and never written as a
# plaintext file; a v2 bundle's payload lives in a private temp dir that is removed afterwards (logical cleanup, not
# physical erasure).
recovery_restore() {
  local directory="${WASL_RECOVERY_DIR:-/backup}" key; key="$(recovery_key_file)"
  local extra=()
  [ "${WASL_RECOVERY_LEGACY_HMAC:-0}" != "1" ] || extra+=(--legacy-hmac)
  [ "${WASL_RECOVERY_REPLACE:-0}" != "1" ] || extra+=(--replace)
  python3 "$ROOT/scripts/recovery.py" migrate "$directory/bundle" --key-file "$key" "$OUT" \
    --nats-check-command "$(printf '%q' "$BIN/wsi7") check-nats -bundle" \
    --nats-restore-command "$(printf '%q' "$BIN/wsi7") restore-nats -bundle" ${extra[@]+"${extra[@]}"}
}
recovery_verify() {
  local directory="${WASL_RECOVERY_DIR:-/backup}" key; key="$(recovery_key_file)"
  local extra=()
  [ "${WASL_RECOVERY_LEGACY_HMAC:-0}" != "1" ] || extra+=(--legacy-hmac)
  python3 "$ROOT/scripts/recovery.py" verify "$directory/bundle" --key-file "$key" ${extra[@]+"${extra[@]}"} >/dev/null
  if [ "${WASL_RECOVERY_LEGACY_HMAC:-0}" = "1" ]; then
    "$BIN/wsi7" verify-nats -bundle "$directory/bundle/files/nats/nats.json"
  else
    # The authenticated payload is piped straight into the read-only cluster comparison; nothing is staged on disk.
    # pipefail (set above) makes an extraction failure fail the verification.
    python3 "$ROOT/scripts/recovery.py" extract-nats "$directory/bundle" - --key-file "$key" |
      "$BIN/wsi7" verify-nats -bundle -
  fi
}
validate_nats_config
case "${1:-all}" in
  recovery-backup) recovery_backup; exit 0;;
  recovery-restore) recovery_restore; exit 0;;
  recovery-verify) recovery_verify; exit 0;;
esac
case "${1:-all}" in cohort|run|audit|tamper|report|serve|smoke|snapshot|final) stale_bin;; esac
[ "${1:-all}" != smoke ] || smoke_source_out_guard
mkdir -p "$OUT/run"
case "${1:-all}" in
  check) check;; build) build;; test) test_;; cohort) cohort;; run) run_;; audit) audit;; tamper) tamper;; report) report;;
  serve) serve;; smoke) smoke;; snapshot) snapshot;;
  all) check; build; test_; cohort; run_; audit; tamper; booking; report; smoke;;
  container) check; cohort; run_; audit; tamper; booking; report;;   # prebuilt binary (WASL_BIN), no Go toolchain or tests
  booking) booking;;
  ltt) shift; LD="$ROOT/pipeline/results/ltt"; [ "${WASL_LTT_ALL:-0}" = 0 ] || LD="$ROOT/pipeline/results/ltt-all"
       "$ROOT/scripts/ltt_cohorts.sh" "$@" && python3 "$ROOT/scripts/ltt_select.py" "${WASL_LTT_DIR:-$LD}";;
  final) shift; exec "$ROOT/scripts/final_eval.sh" "$@";;
  *) echo "usage: $0 [all|check|build|test|cohort|run|audit|tamper|booking|report|serve|smoke|snapshot|final|container]"; exit 2;;
esac
