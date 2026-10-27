#!/usr/bin/env bash
# Wasl one-command baseline: check assets → build → cohort → integrated NATS run → audit → tamper → report.
# Usage: scripts/demo.sh [all|check|build|cohort|run|audit|tamper|report|test]   (default: all)
# Outputs: out/cohort/ (synthetic data), out/run/<policy>/ (logs, report7.json, dashboard.json, review_queue.json).
# Env: WASL_POLICY=v7strict|v6 (default v7strict), WASL_SEED (default 2026), WASL_OUT (default ./out).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/bin"; OUT="${WASL_OUT:-$ROOT/out}"; SEED="${WASL_SEED:-2026}"
POLICY="${WASL_POLICY:-v7strict}"   # v7strict (canonical) | v6 (legacy comparison)
COHORT="$OUT/cohort"; BASE="$OUT/cohort-base"; RUN="$OUT/run/$POLICY"
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
  for p in 14822 14823 14824 28880 28890; do
    if lsof -nP -iTCP:$p -sTCP:LISTEN >/dev/null 2>&1; then need "free port $p" "another Wasl run (or service) is listening; stop it first"; fi
  done
  echo "assets OK"
}
build()  { (cd "$ROOT/pipeline" && go vet . && go build -o "$BIN/wsi7" .); echo "built bin/wsi7"; }
test_()  { (cd "$ROOT/pipeline" && go test -count=1 .); }
cohort() {
  rm -rf "$BASE" "$COHORT" "$OUT/run/base"; mkdir -p "$BASE" "$COHORT"
  "$BIN/wsi7" gen -wasl "$ROOT/data" -reports "$ROOT/data/evidence/reports43.json" -pathology "$ROOT/data/evidence/pathology_templates.json" \
    -ecpool "$ROOT/patient-parsers/econsult_testset.json" -tbpool "$ROOT/patient-parsers/teachback_testset.json" \
    -out "$BASE" -seed "$SEED" -persons 6500 -cerv 600 > "$OUT/gen.json"
  "$BIN/wsi7" sim -data "$BASE" -wasl "$ROOT/models" -run "$OUT/run/base" > "$OUT/run/base.json" 2> "$OUT/run/base.err"
  python3 "$ROOT/pipeline/augment_cohort.py" "$BASE" "$OUT/run/base" "$COHORT"
  echo "cohort → $COHORT"
}
run_() {
  (cd "$BIN" && ./wsi7 orchestrate -data "$COHORT" -wasl "$ROOT/models" -run "$RUN" -preps 3 -threads 8 \
     -source fhir -subkill 1 -chaos 2 -natskill -ann hnsw -rerr 0.04 -rdouble merge+close -dual -rseed 1 -seed "$SEED" -policy "$POLICY" | tee "$OUT/run/$POLICY.stdout")
}
audit()  { (cd "$BIN" && ./wsi7 audit -data "$COHORT" -wasl "$ROOT/models" -run "$RUN" -ann hnsw -dual -policy "$POLICY" > "$RUN/audit.stdout" 2>&1); echo "audit → $RUN/audit.stdout"; }
tamper() { (cd "$BIN" && ./wsi7 tamper -role auditor -data "$COHORT" -wasl "$ROOT/models" -run "$RUN" -ann hnsw -dual -policy "$POLICY" > "$RUN/tamper.json" 2> "$RUN/tamper.log"); tail -3 "$RUN/tamper.log"; }
report() {
  (cd "$BIN" && ./wsi7 report7 -data "$COHORT" -wasl "$ROOT/models" -run "$RUN" -ann hnsw -dual -policy "$POLICY" > /dev/null)
  python3 "$ROOT/scripts/summary.py" "$RUN/report7.json"
}
mkdir -p "$OUT/run"
case "${1:-all}" in
  check) check;; build) build;; test) test_;; cohort) cohort;; run) run_;; audit) audit;; tamper) tamper;; report) report;;
  all) check; build; test_; cohort; run_; audit; tamper; report;;
  *) echo "usage: $0 [all|check|build|test|cohort|run|audit|tamper|report]"; exit 2;;
esac
