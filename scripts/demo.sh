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
# WASL_API_TOKENS together for a non-default local configuration.
export WASL_SYNTHETIC_DATA_ONLY=1
export WASL_API_REVIEWER_TOKEN="${WASL_API_REVIEWER_TOKEN:-wasl-demo-reviewer-1-token}"
export WASL_API_REVIEWER_2_TOKEN="${WASL_API_REVIEWER_2_TOKEN:-wasl-demo-reviewer-2-token}"
export WASL_API_REVIEWER_3_TOKEN="${WASL_API_REVIEWER_3_TOKEN:-wasl-demo-reviewer-3-token}"
export WASL_API_AUDITOR_TOKEN="${WASL_API_AUDITOR_TOKEN:-wasl-demo-auditor-token}"
export WASL_API_COORDINATOR_TOKEN="${WASL_API_COORDINATOR_TOKEN:-wasl-demo-coordinator-token}"
export WASL_API_CLINICIAN_TOKEN="${WASL_API_CLINICIAN_TOKEN:-wasl-demo-clinician-token}"
export WASL_API_TOKENS="${WASL_API_TOKENS:-$WASL_API_REVIEWER_TOKEN:reviewer:smoke-reviewer-1,$WASL_API_REVIEWER_2_TOKEN:reviewer:smoke-reviewer-2,$WASL_API_REVIEWER_3_TOKEN:reviewer:smoke-reviewer-3,$WASL_API_AUDITOR_TOKEN:auditor:smoke-auditor,$WASL_API_COORDINATOR_TOKEN:coordinator:smoke-coordinator,$WASL_API_CLINICIAN_TOKEN:clinician:smoke-clinician}"
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
smoke() {  # real HTTP smoke: serve on loopback (WASL_SMOKE_PORT, default 18088 + WASL_PORT_OFFSET), poll /healthz, run scripts/api_smoke.py
  local port="${WASL_SMOKE_PORT:-$((18088 + ${WASL_PORT_OFFSET:-0}))}" wait="${WASL_SMOKE_WAIT:-60}" addr rc=0 i=0 ok=0
  addr="127.0.0.1:$port"
  SMOKE_PID=""; trap '[ -z "$SMOKE_PID" ] || kill "$SMOKE_PID" 2>/dev/null || true' EXIT
  serve "$addr" > "$OUT/smoke-serve.log" 2>&1 &
  SMOKE_PID=$!; local pid=$SMOKE_PID
  while [ "$i" -lt "$((wait * 2))" ]; do
    kill -0 "$pid" 2>/dev/null || break
    if curl -sf --max-time 2 "http://$addr/healthz" >/dev/null 2>&1; then ok=1; break; fi
    sleep 0.5; i=$((i + 1))
  done
  if [ "$ok" != 1 ]; then echo "smoke: server did not become healthy on $addr (see $OUT/smoke-serve.log)" >&2; tail -5 "$OUT/smoke-serve.log" >&2 || true; rc=1
  else python3 "$ROOT/scripts/api_smoke.py" --base-url "http://$addr" || rc=$?; fi
  kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; SMOKE_PID=""
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
  curl -sf --max-time "$MT" -H "Authorization: Bearer $WASL_API_REVIEWER_TOKEN" "http://$A/api/dashboard" |
    python3 "$ROOT/scripts/redacted_snapshot.py" dashboard > "$S/dashboard.json" || { echo "snapshot: dashboard failed" >&2; return 1; }
  curl -sf --max-time "$MT" -H "Authorization: Bearer $WASL_API_REVIEWER_TOKEN" "http://$A/api/fixtures" |
    python3 "$ROOT/scripts/redacted_snapshot.py" fixtures > "$S/fixtures.json" || { echo "snapshot: fixtures failed" >&2; return 1; }
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
# v2 (encrypted) bundles use the versioned keyring recovery_keys.json. A legacy HMAC-only bundle needs
# WASL_RECOVERY_LEGACY_HMAC=1 and WASL_RECOVERY_KEY_FILE pointing at the old backup_key; it is never a default.
recovery_key_file() { echo "${WASL_RECOVERY_KEY_FILE:-/run/secrets/recovery_keys.json}"; }
recovery_legacy_flag() { [ "${WASL_RECOVERY_LEGACY_HMAC:-0}" = "1" ] && echo "--legacy-hmac" || true; }
# Decrypt only the NATS payload into a private temporary directory (mode 0700) and remove it afterwards. Removal is
# logical cleanup, not a claim of physical erasure.
with_nats_payload() {
  local directory="$1" key="$2" action="$3" tmp rc=0
  tmp="$(mktemp -d "${TMPDIR:-/tmp}/wasl-nats-payload.XXXXXX")"; chmod 0700 "$tmp"
  if [ "${WASL_RECOVERY_LEGACY_HMAC:-0}" = "1" ]; then
    "$BIN/wsi7" "$action" -bundle "$directory/bundle/files/nats/nats.json" || rc=$?
  else
    python3 "$ROOT/scripts/recovery.py" extract-nats "$directory/bundle" "$tmp/nats.json" --key-file "$key" >/dev/null &&
      "$BIN/wsi7" "$action" -bundle "$tmp/nats.json" || rc=$?
  fi
  rm -rf "$tmp" || { echo "warning: could not remove $tmp" >&2; rc=1; }
  return "$rc"
}
recovery_backup() {
  local directory="${WASL_RECOVERY_DIR:-/backup}" key; key="$(recovery_key_file)"
  [ "${WASL_RECOVERY_LEGACY_HMAC:-0}" != "1" ] || { echo "legacy HMAC bundles are read-only; backup always writes v2" >&2; exit 1; }
  mkdir -p "$directory"
  [ ! -e "$directory/bundle" ] || { echo "recovery backup already exists: $directory/bundle" >&2; exit 1; }
  local plain; plain="$(mktemp -d "${TMPDIR:-/tmp}/wasl-nats-backup.XXXXXX")"; chmod 0700 "$plain"
  local rc=0
  { "$BIN/wsi7" backup-nats -bundle "$plain/nats.json" &&
    python3 "$ROOT/scripts/recovery.py" backup "$OUT" "$directory/bundle" --key-file "$key" --nats-json "$plain/nats.json"; } || rc=$?
  rm -rf "$plain" || { echo "warning: could not remove $plain" >&2; rc=1; }
  return "$rc"
}
recovery_restore() {
  local directory="${WASL_RECOVERY_DIR:-/backup}" key; key="$(recovery_key_file)"
  # Authentication and every payload hash are checked before NATS is contacted or the destination is created.
  python3 "$ROOT/scripts/recovery.py" verify "$directory/bundle" --key-file "$key" $(recovery_legacy_flag)
  with_nats_payload "$directory" "$key" restore-nats
  python3 "$ROOT/scripts/recovery.py" migrate "$directory/bundle" --key-file "$key" "$OUT" $(recovery_legacy_flag)
}
recovery_verify() {
  local directory="${WASL_RECOVERY_DIR:-/backup}" key; key="$(recovery_key_file)"
  python3 "$ROOT/scripts/recovery.py" verify "$directory/bundle" --key-file "$key" $(recovery_legacy_flag) >/dev/null
  with_nats_payload "$directory" "$key" verify-nats
}
validate_nats_config
case "${1:-all}" in
  recovery-backup) recovery_backup; exit 0;;
  recovery-restore) recovery_restore; exit 0;;
  recovery-verify) recovery_verify; exit 0;;
esac
case "${1:-all}" in cohort|run|audit|tamper|report|serve|smoke|snapshot|final) stale_bin;; esac
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
