#!/usr/bin/env bash
# Stage 7: frozen final evaluation (docs/BACKEND-HANDOFF.md §7). Writes pipeline/results/final/.
#   1. FREEZE: refuses to run on a dirty tree; records code revision, policy, thresholds, model/data hashes, seeds BEFORE any run.
#   2. Held-out persons only (gen uses the Wikidata eval split). Two noise models: v1 (training-time) and v2 (independent).
#   3. Five seeds x {v1, v2}: full integrated NATS run (chaos on) + audit + tamper + report.
#   4. Aggregates with 95% intervals (t over seeds) and Clopper-Pearson upper bounds for zero-event safety metrics.
# Usage: scripts/final_eval.sh [seeds...]   (default 11 23 37 41 59). Env: WASL_SOURCE=fhir|hl7 (default fhir).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
if [ $# -gt 0 ]; then SEEDS=("$@"); else SEEDS=(11 23 37 41 59); fi
FINAL="${WASL_FINAL_DIR:-$ROOT/pipeline/results/final}"; WORK="${WASL_FINAL_WORK:-$ROOT/out/final}"
if [ -n "$(git -C "$ROOT" status --porcelain -- pipeline booking scripts models data patient-parsers)" ] && [ -z "${WASL_ALLOW_DIRTY:-}" ]; then
  echo "refusing: uncommitted changes in code/models/data (the evaluation must be frozen to a commit)"; exit 1
fi
mkdir -p "$FINAL" "$WORK"
"$ROOT/scripts/demo.sh" check >/dev/null
"$ROOT/scripts/demo.sh" build >/dev/null
python3 - "$ROOT" "$FINAL/manifest.json" "${SEEDS[@]}" <<'P'
import json, sys, hashlib, subprocess, os, datetime
root, out, seeds = sys.argv[1], sys.argv[2], [int(s) for s in sys.argv[3:]]
h = lambda p: hashlib.sha256(open(os.path.join(root, p), 'rb').read()).hexdigest()
rev = subprocess.check_output(['git', '-C', root, 'rev-parse', 'HEAD']).decode().strip()
m = {"frozen_at": datetime.datetime.utcnow().isoformat() + "Z", "code_revision": rev,
     "policy": "v7strict", "thresholds": {"tau_high_messaging": 0.99, "tau_close_v7strict": 0.98, "uncorroborated_cap": 0.989, "calibrator": "models/iso_v7.json (stratified)", "dob_second_id_guard": True, "reviewer_error": 0.04, "double_review": "merge+close"},
     "reader": "cached (Gemma 4 26B answers, sha256-keyed; live 4B reported separately in pipeline/RESULTS.md stage 2)",
     "source": os.environ.get("WASL_SOURCE", "fhir"), "persons": "Wikidata eval split only (held out from encoder, scorer and calibrator training)",
     "noise_models": {"v1": "training-time corruptions (gen.go)", "v2": "independent held-out corruptions (noise2.go), never used for training/calibration/rule design"},
     "seeds": seeds, "chaos": "linker kill, closure+prep kill, ingest-adapter kill, NATS node kill",
     "hashes": {p: h(p) for p in ["models/encoder_weights.json", "models/lgb_stream.json", "models/iso.json", "models/iso_v7.json", "models/gemma_cache.json",
                                  "models/parse_cache.json", "data/persons3.json", "data/evidence/reports43.json", "data/evidence/pathology_templates.json"]},
     "labelling": "43 report texts NOT yet human double-labelled; kit in docs/labelling/ (blind sheets + Cohen kappa)",
     "limitations": ["the evaluation is frozen to this commit but the v7 rules were designed on earlier v1-noise seeds; v2 noise is the honest held-out test","synthetic cohorts, reviewers, capacity; no clinician labels",
                     "43 MTSamples report texts reused across loops; answer key LLM-labelled and cross-checked with a regex reader",
                     "cached 26B reader answers (deterministic); the live local 4B reader is evaluated separately",
                     "one Abu Dhabi-shaped provider topology; real Malaffi feeds not accessed"]}
json.dump(m, open(out, 'w'), indent=1)
print("frozen", rev[:12], "seeds", seeds)
P
for noise in v1 v2; do
  for seed in "${SEEDS[@]}"; do
    tag="$noise-s$seed"; O="$WORK/$tag"
    if [ -s "$FINAL/$tag/report7.json" ]; then echo "skip $tag (done)"; continue; fi
    rm -rf "$O"; mkdir -p "$O/run"
    B="$ROOT/bin"
    "$B/wsi7" gen -wasl "$ROOT/data" -reports "$ROOT/data/evidence/reports43.json" -pathology "$ROOT/data/evidence/pathology_templates.json" \
      -ecpool "$ROOT/patient-parsers/econsult_testset.json" -tbpool "$ROOT/patient-parsers/teachback_testset.json" \
      -out "$O/base" -seed "$seed" -persons 6500 -cerv 600 -noise "$noise" > /dev/null
    "$B/wsi7" sim -data "$O/base" -wasl "$ROOT/models" -run "$O/run/base" > /dev/null 2>&1
    python3 "$ROOT/pipeline/augment_cohort.py" "$O/base" "$O/run/base" "$O/cohort" > /dev/null
    R="$O/run/main"
    (cd "$B" && ./wsi7 orchestrate -data "$O/cohort" -wasl "$ROOT/models" -run "$R" -preps 3 -threads 8 -source "${WASL_SOURCE:-fhir}" \
        -subkill 1 -chaos 2 -natskill -ann hnsw -rerr 0.04 -rdouble merge+close -dual -rseed "$seed" -seed "$seed" -policy v7strict > "$O/orchestrate.log" 2>&1)
    (cd "$B" && ./wsi7 audit -data "$O/cohort" -wasl "$ROOT/models" -run "$R" -ann hnsw -dual -policy v7strict > "$R/audit.stdout" 2>&1)
    (cd "$B" && ./wsi7 tamper -role auditor -data "$O/cohort" -wasl "$ROOT/models" -run "$R" -ann hnsw -dual -policy v7strict -seed "$seed" > "$R/tamper.json" 2> "$R/tamper.log")
    (cd "$B" && ./wsi7 report7 -data "$O/cohort" -wasl "$ROOT/models" -run "$R" -ann hnsw -dual -policy v7strict > /dev/null)
    mkdir -p "$FINAL/$tag"
    cp "$R/report7.json" "$R/tamper.json" "$FINAL/$tag/"
    python3 "$ROOT/scripts/summary.py" "$R/report7.json" > "$FINAL/$tag/summary.txt"
    echo "done $tag: $(sed -n 2p "$FINAL/$tag/summary.txt")"
    rm -rf "$O/run/main/nats-"* "$O/run/base"   # keep logs, drop NATS stores
  done
done
python3 "$ROOT/scripts/final_aggregate.py" "$FINAL"
