#!/usr/bin/env bash
# Stage 7: frozen final evaluation (docs/BACKEND-HANDOFF.md §7). Writes pipeline/results/final/.
#   1. FREEZE: refuses to run on a dirty tree; records code revision, policy, thresholds, model/data hashes, seeds BEFORE any run.
#   2. Held-out persons only (gen uses the Wikidata eval split). Two noise models: v1 (training-time) and v2 (independent).
#   3. Five seeds x {v1, v2}: full integrated NATS run (chaos on) + audit + tamper + report.
#   4. Aggregates with 95% intervals (t over seeds) and Clopper-Pearson upper bounds for zero-event safety metrics.
# Usage: scripts/final_eval.sh [seeds...]   (default 11 23 37 41 59). Env: WASL_SOURCE=fhir|hl7 (default fhir),
#   WASL_LUNG=N incidental lung-nodule loops per run (default 1088; 0 = screening cohort only), WASL_DR=N diabetic
#   retinopathy screening loops per run (default 1200; 0 = off), WASL_LUNGSURV=1 surveillance CT after a PET/CT
#   low-uptake lung closure (default 0 = off), WASL_WRONGEID=p / WASL_WRONGREF=p adversarial cohorts (relatives' Emirates
#   IDs / wrong referral numbers; pipeline/genadv.go; default 0 = off), WASL_LUNGSURV2=1 its later CTs at about 1 and 2 years and
#   WASL_LUNGSURVSTABLE=1 review items, truth and metrics for the stable-CT surveillance child (both default 0 = off), WASL_LDCT=N lung cancer screening LDCT loops (default 0 = off). Cohort size: every held-out
#   person (8,871; WASL_PERSONS caps it), WASL_CRC=2040 CRC and WASL_CERV=816 cervical loops: 1.36x the earlier 6,500
#   persons / 1,500 / 600 / 800, so the smallest name-origin group (Filipino) has about 360 loops per run, not 250.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
if [ $# -gt 0 ]; then SEEDS=("$@"); else SEEDS=(11 23 37 41 59); fi
FINAL="${WASL_FINAL_DIR:-$ROOT/pipeline/results/final}"; WORK="${WASL_FINAL_WORK:-$ROOT/out/final}"
# WASL_REPORTS: colonoscopy report corpus; WASL_MODELS: models dir (its gemma_cache.json is the cached second reader)
REPORTS="${WASL_REPORTS:-$ROOT/data/evidence/reports43.json}"; MODELS="${WASL_MODELS:-$ROOT/models}"; export REPORTS MODELS
DR="${WASL_DR:-1200}"; export DR   # diabetic retinopathy screening loops per run (third loop type; 0 = off)
LDCT="${WASL_LDCT:-0}"; export LDCT   # lung cancer screening LDCT loops per run (fifth loop type; default 0 = off)
IFHAS="${WASL_IFHAS:-0}"; export IFHAS   # IFHAS abnormal-result follow-up loops per run (pipeline/ifhas.go; 0 = off)
BREAST="${WASL_BREAST:-70}"; export BREAST   # breast screening recall loops per run (fourth loop type; 0 = off; 70 plus distractors fit the fewest held-out women next to CRC, cervical and DR)
LUNG="${WASL_LUNG:-1088}"; PERSONS="${WASL_PERSONS:-0}"; CRC="${WASL_CRC:-2040}"; CERV="${WASL_CERV:-816}"; export LUNG PERSONS CRC CERV
# pilot switches (pipeline/pilot.go), default off: WASL_SHADOW=1 shadow mode; WASL_AUTOAUDIT=p post-closure audit fraction
PARGS=(); [ "${WASL_SHADOW:-0}" = 0 ] || PARGS+=(-shadow); [ "${WASL_AUTOAUDIT:-0}" = 0 ] || PARGS+=(-autoaudit "$WASL_AUTOAUDIT")
# WASL_EIDGUARD=1: separate post-hoc wrong-EID closure-guard lane (default off); WASL_NOISES="v2": select noise lanes (default "v1 v2")
[ "${WASL_EIDGUARD:=0}" = 0 ] || PARGS+=(-eidguard)
# WASL_LUNGSURV=1: surveillance CT after a PET/CT low-uptake lung closure (pipeline/lungsurv.go), generator and engine; default on
GSURV=(); [ "${WASL_LUNGSURV:=1}" = 0 ] || { PARGS+=(-lungsurv); GSURV=(-lungsurv); }
# WASL_LUNGSURV2=1 (needs WASL_LUNGSURV=1): later surveillance CTs at about 1 and 2 years (BTS 2015); WASL_LUNGSURVSTABLE=1:
# stable-CT surveillance child with review items, truth and metrics (pipeline/lungsurv.go); generator and engine; default off
[ "${WASL_LUNGSURV2:=0}" = 0 ] || [ "$WASL_LUNGSURV" = 0 ] || { PARGS+=(-lungsurv2); GSURV+=(-lungsurv2); }
[ "${WASL_LUNGSURVSTABLE:=0}" = 0 ] || { PARGS+=(-lungsurvstable); GSURV+=(-lungsurvstable); }
# hand-off, default on: WASL_XREFLINK=1 a rejected intake that cites the referral but never linked raises a link question (pipeline/handoffref.go)
[ "${WASL_XREFLINK:=1}" = 0 ] || PARGS+=(-xreflink)
# WASL_REFREJECT=true (gen, default true): lung work-up and DR eye-clinic referrals can be rejected (pipeline/handoffext.go)
: "${WASL_REFREJECT:=true}"
# WASL_DRNEXT=1 (default on): next due DR rescreen / clinic review and recall_overdue (pipeline/drnext.go), generator and engine
[ "${WASL_DRNEXT:=1}" = 0 ] || { PARGS+=(-drnext); GSURV+=(-drnext); }
# WASL_SECONDIDCONFIRM=0: a contradicted second-identifier 'same' counts at once (-secondidconfirm=false; default on, v7.go)
[ "${WASL_SECONDIDCONFIRM:=1}" != 0 ] || PARGS+=(-secondidconfirm=false)
# adversarial generator cases (gen only, default off; pipeline/genadv.go): WASL_WRONGEID=p relatives' Emirates IDs on a
# share p of registrations (B8); WASL_WRONGREF=p wrong referral numbers on a share p of referral intakes (B9)
: "${WASL_WRONGEID:=0}" "${WASL_WRONGREF:=0}"
[ "$WASL_WRONGEID" = 0 ] || GSURV+=(-wrongeid "$WASL_WRONGEID")
[ "$WASL_WRONGREF" = 0 ] || GSURV+=(-wrongref "$WASL_WRONGREF")
# WASL_BREASTSHARE=1 (gen, default off): breast loops beyond the women with no other loop use women who hold another
# programme's loop (pipeline/genbreast.go); needed above about 85 breast loops per cohort
[ "${WASL_BREASTSHARE:=0}" = 0 ] || GSURV+=(-breastshare)
export WASL_LUNGSURV WASL_LUNGSURV2 WASL_LUNGSURVSTABLE WASL_XREFLINK WASL_REFREJECT WASL_DRNEXT WASL_SECONDIDCONFIRM WASL_WRONGEID WASL_WRONGREF WASL_BREASTSHARE WASL_EIDGUARD
if [ -n "$(git -C "$ROOT" status --porcelain -- pipeline booking scripts models data patient-parsers)" ] && [ -z "${WASL_ALLOW_DIRTY:-}" ]; then
  echo "refusing: uncommitted changes in code/models/data (the evaluation must be frozen to a commit)"; exit 1
fi
mkdir -p "$FINAL" "$WORK"
"$ROOT/scripts/demo.sh" check >/dev/null
"$ROOT/scripts/demo.sh" build >/dev/null
python3 "$ROOT/scripts/final_manifest.py" "$ROOT" "$FINAL/manifest.json" "${SEEDS[@]}"
run_one() {  # $1 noise, $2 seed; WASL_PORT_OFFSET picks this lane's ports
    local noise="$1" seed="$2"; local tag="$noise-s$seed"; local O="$WORK/$tag"
    # H7 simulated-reviewer stress test and mitigations, all off by default (same env as scripts/demo.sh)
    local HARGS=()
    [ -z "${WASL_REVIEW_RHO:-}" ] || HARGS+=(-rrho "$WASL_REVIEW_RHO")
    [ "${WASL_REVIEW_BLIND:-0}" = 0 ] || HARGS+=(-reviewblind)
    [ "${WASL_REVIEW_ROTATE:-0}" = 0 ] || HARGS+=(-reviewrotate)
    [ -z "${WASL_SENIOR_AUDIT:-}" ] || HARGS+=(-senioraudit "$WASL_SENIOR_AUDIT")
    if [ -s "$FINAL/$tag/report7.json" ]; then echo "skip $tag (done)"; return; fi
    rm -rf "$O"; mkdir -p "$O/run"
    B="$ROOT/bin"
    "$B/wsi7" gen -wasl "$ROOT/data" -reports "$REPORTS" -pathology "$ROOT/data/evidence/pathology_templates.json" -prepquery="${WASL_PREPQUERY:-true}" -handoff="${WASL_HANDOFF:-true}" -lungworkup="${WASL_LUNGWORKUP:-true}" -refreject="${WASL_REFREJECT:-false}" \
      -ecpool "$ROOT/patient-parsers/econsult_testset.json" -tbpool "$ROOT/patient-parsers/teachback_testset.json" \
      -out "$O/base" -seed "$seed" -loops "$CRC" -persons "$PERSONS" -cerv "$CERV" -noise "$noise" -lung "$LUNG" -lungdata "$ROOT/data/lung" -dr "$DR" -drdata "$ROOT/data/dr" -breast "$BREAST" -breastdata "$ROOT/data/breast" -ldct "$LDCT" -ldctdata "$ROOT/data/ldct" -ifhas "$IFHAS" -ifhasdata "$ROOT/data/ifhas" ${GSURV[@]+"${GSURV[@]}"} > /dev/null
    "$B/wsi7" sim -data "$O/base" -wasl "$MODELS" -run "$O/run/base" > /dev/null 2>&1
    python3 "$ROOT/pipeline/augment_cohort.py" "$O/base" "$O/run/base" "$O/cohort" > /dev/null
    R="$O/run/main"
    (cd "$B" && ./wsi7 orchestrate -prepquery="${WASL_PREPQUERY:-true}" -handoff="${WASL_HANDOFF:-true}" -lungworkup="${WASL_LUNGWORKUP:-true}" -data "$O/cohort" -wasl "$MODELS" -run "$R" -preps 3 -threads 8 -source "${WASL_SOURCE:-fhir}" \
        -subkill 1 -chaos 2 -natskill -ann hnsw -rerr 0.04 -rdouble merge+close -dual -rseed "$seed" -seed "$seed" -policy v7strict ${PARGS[@]+"${PARGS[@]}"} ${HARGS[@]+"${HARGS[@]}"} > "$O/orchestrate.log" 2>&1)
    (cd "$B" && ./wsi7 audit -data "$O/cohort" -wasl "$MODELS" -run "$R" -ann hnsw -dual -policy v7strict ${PARGS[@]+"${PARGS[@]}"} > "$R/audit.stdout" 2>&1)
    (cd "$B" && ./wsi7 tamper -role auditor -data "$O/cohort" -wasl "$MODELS" -run "$R" -ann hnsw -dual -policy v7strict ${PARGS[@]+"${PARGS[@]}"} -seed "$seed" > "$R/tamper.json" 2> "$R/tamper.log")
    (cd "$B" && ./wsi7 report7 -data "$O/cohort" -wasl "$MODELS" -run "$R" -ann hnsw -dual -policy v7strict ${PARGS[@]+"${PARGS[@]}"} > /dev/null)
    mkdir -p "$FINAL/$tag"
    cp "$R/report7.json" "$R/tamper.json" "$FINAL/$tag/"
    sed -i.bak -e "s#$WORK/#./out/final/#g" -e "s#$ROOT/#./#g" "$FINAL/$tag/report7.json" "$FINAL/$tag/tamper.json" && rm -f "$FINAL/$tag/"*.bak   # no local paths in committed results
    python3 "$ROOT/scripts/summary.py" "$R/report7.json" > "$FINAL/$tag/summary.txt"
    python3 "$ROOT/scripts/origin_null.py" "$O/cohort/truth.json" "$R/reviewer_decisions.ndjson" > "$FINAL/$tag/origin_null.json"   # review-load ratio vs no-bias reference
    python3 "$ROOT/scripts/subgroups.py" "$O/cohort/truth.json" "$R/reviewer_decisions.ndjson" "$R/out_log.ndjson" "$ROOT/data/persons3.json" > "$FINAL/$tag/subgroups.json"   # CRC by age band and sex
    echo "done $tag: $(sed -n 2p "$FINAL/$tag/summary.txt")"
    rm -rf "$O/run/main/nats-"* "$O/run/base"   # keep logs, drop NATS stores
}
JOBS="${WASL_JOBS:-3}"   # runs in parallel, each on its own ports; throughput_ev_s is then per-run under contention
TAGS=(); for noise in ${WASL_NOISES:-v1 v2}; do
  [ "$noise" = v1 ] || [ "$noise" = v2 ] || { echo "invalid WASL_NOISES entry: $noise" >&2; exit 2; }
  for seed in "${SEEDS[@]}"; do TAGS+=("$noise:$seed"); done
done
for ((lane = 0; lane < JOBS; lane++)); do
  ( export WASL_PORT_OFFSET=$((${WASL_PORT_BASE:-0} + lane * 100))
    for ((k = lane; k < ${#TAGS[@]}; k += JOBS)); do run_one "${TAGS[$k]%%:*}" "${TAGS[$k]##*:}"; done ) &
done
wait
python3 "$ROOT/scripts/final_aggregate.py" "$FINAL"
