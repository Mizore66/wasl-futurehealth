#!/usr/bin/env bash
# ONE tightly bounded live smoke of the read-only agentshadow on a generated synthetic -demoguard cohort (docs/AGENT-SHADOW.md).
# External calls happen only here and only because the caller exported WASL_LLM_URL, WASL_LLM_KEY, WASL_LLM_MODEL and
# WASL_AGENTSHADOW_LIVE=1. Bounds: 3 items, 6 uncached provider requests, 3 minute deadline, 45 s per request.
# Reports coverage, agreement, fallback, latency and token cost only. Nothing is written outside RUN/agent_shadow/.
#
# A single local nats-server (external to the orchestrator, WASL_NATS_EXTERNAL=1) keeps the streams alive until the shadow
# has finished its slower live calls (file source: the hl7 ingest ledger needs a 3-node cluster). The orchestrator and its children never receive the provider variables.
#
# Usage: scripts/agent_shadow_smoke.sh COHORT_DIR RUN_DIR
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="${WASL_BIN:-$ROOT/bin}"
COHORT="${1:?synthetic cohort dir}"; RUN="${2:?run dir (new)}"
MODE="${WASL_SMOKE_MODE:-live}"   # cached = dry run of this script's wiring: no provider variables, no external call
if [ "$MODE" = live ]; then
  for v in WASL_LLM_URL WASL_LLM_KEY WASL_LLM_MODEL; do [ -n "${!v:-}" ] || { echo "missing $v" >&2; exit 2; }; done
  [ "${WASL_AGENTSHADOW_LIVE:-}" = 1 ] || { echo "live smoke needs WASL_AGENTSHADOW_LIVE=1" >&2; exit 2; }
fi
[ ! -e "$RUN" ] || { echo "run dir exists: $RUN" >&2; exit 2; }
PORT="${WASL_SMOKE_NATS_PORT:-14999}"
mkdir -p "$RUN.nats"
"$BIN/nats-server" -js -sd "$RUN.nats" -p "$PORT" -a 127.0.0.1 > "$RUN.nats.log" 2>&1 &
NATS=$!
trap 'kill $NATS 2>/dev/null || true' EXIT
sleep 1.5
NATS_ENV=(WASL_NATS_URL="nats://127.0.0.1:$PORT" WASL_NATS_EXTERNAL=1)
ARGS=(-data "$COHORT" -wasl "$ROOT/models" -preps 3 -threads 4 -source file -chaos 0 -ann hnsw
      -rerr 0 -rdouble none -rseed 1 -seed "${WASL_SEED:-9101}" -policy v7strict -modelset v3 -linkfloor 0.01 -r1review -demoguard -P 1 -noreview -replicas 1)
( cd "$BIN" && env -u WASL_LLM_URL -u WASL_LLM_KEY -u WASL_LLM_MODEL -u WASL_AGENTSHADOW_LIVE "${NATS_ENV[@]}" ./wsi7 orchestrate -run "$RUN" "${ARGS[@]}" > "$RUN.orch.stdout" 2>&1 ) &
ORCH=$!
( sleep 4; cd "$BIN" && exec env "${NATS_ENV[@]}" ./wsi7 agentshadow -run "$RUN" "${ARGS[@]}" -agentmode "$MODE" -agentitems 3 -agentcalls 6 \
    -agentdeadline 3m -agentprovider 45s -agentidle 10s -agentworker "$ROOT/scripts/agent_shadow_worker.py" > "$RUN.shadow.stdout" 2> "$RUN.shadow.stderr" ) &
SHADOW=$!
wait "$ORCH" || true
wait "$SHADOW" || true
cat "$RUN/agent_shadow/report.json"
