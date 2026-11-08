#!/usr/bin/env bash
# Off-vs-on parity for the read-only agentshadow (docs/AGENT-SHADOW.md): orchestrated runs of one fixed synthetic
# -demoguard cohort and seed. Arms:
#   base  (optional, WASL_BASE_BIN=dir with a wsi7 built BEFORE the shadow change)  shadow absent from the binary
#   off   current binary, no shadow process
#   off2  current binary again, no shadow (noise control: how much two identical runs differ by themselves)
#   on    current binary plus `wsi7 agentshadow` (cached mode, no external call) started beside the run
# Compares out_log.ndjson, fhir_log.ndjson, prep_log.ndjson, reviewer_decisions.ndjson and the outreach artifacts with
# timing and log metadata removed (scripts/agent_shadow_parity.py).
#
# Usage: WASL_PARITY_REVIEW=0|1 scripts/agent_shadow_parity.sh COHORT_DIR WORK_DIR
#   WASL_PARITY_REVIEW=0 (default) runs with -noreview: the whole pipeline is deterministic, so every artifact is
#   compared byte-for-byte. =1 adds the simulated reviewer, whose sign-off timing against the stream is not
#   deterministic even between two runs without the shadow; the control arm shows that floor.
# Needs bin/wsi7 (or WASL_BIN) and bin/nats-server beside it.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="${WASL_BIN:-$ROOT/bin}"
COHORT="${1:?cohort dir (generated, -demoguard compatible)}"; WORK="${2:?work dir}"
SEED="${WASL_SEED:-9101}"
REVIEW="${WASL_PARITY_REVIEW:-0}"
mkdir -p "$WORK"
ARGS=(-data "$COHORT" -wasl "$ROOT/models" -preps 3 -threads 4 -source hl7 -fdup 0 -freorder 0 -hl7drops 0 -subkill 0 -chaos 0 -ann hnsw -rerr 0 -rdouble none -rseed 1
      -seed "$SEED" -policy v7strict -modelset v3 -linkfloor 0.01 -r1review -demoguard -P 1)
[ "$REVIEW" = 1 ] || ARGS+=(-noreview)
ARMS=(off off2 on); [ -z "${WASL_BASE_BIN:-}" ] || ARMS=(base "${ARMS[@]}")
RUNS=()
for arm in "${ARMS[@]}"; do
  RUN="$WORK/run-$arm"; rm -rf "$RUN"; RUNS+=("$RUN")
  B="$BIN"; [ "$arm" != base ] || B="$WASL_BASE_BIN"
  if [ "$arm" = on ]; then
    # a separate, never-restarted process started beside the run, not by the orchestrator
    ( sleep 4; cd "$BIN" && exec ./wsi7 agentshadow -run "$RUN" "${ARGS[@]}" -agentmode cached \
        -agentworker "$ROOT/scripts/agent_shadow_worker.py" -agentidle 6s > "$WORK/shadow.stdout" 2> "$WORK/shadow.stderr" ) &
    SHADOW=$!
  fi
  ( cd "$B" && ./wsi7 orchestrate -run "$RUN" "${ARGS[@]}" > "$WORK/orch-$arm.stdout" 2>&1 )
  if [ "$arm" = on ]; then wait "$SHADOW" || true; fi
done
python3 -I "$ROOT/scripts/agent_shadow_parity.py" "${RUNS[@]}" | tee "$WORK/parity.json"
