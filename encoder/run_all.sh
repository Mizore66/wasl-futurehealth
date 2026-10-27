#!/usr/bin/env bash
# End-to-end: data -> parity -> baseline -> train -> evaluate -> export check -> scorer/calibrator refit.
# Usage: encoder/run_all.sh [SCRATCH_DIR]   (default ~/wasl-ml-data/encoder; needs ~1 GB and a Python with
#        torch, scikit-learn, lightgbm, scipy, numpy: set PY=/path/to/python)
# Steps 1 (Wikidata mining, ~2 h at 1 request/s) and 4 (training, ~1 h on an M2 Pro GPU) dominate the run time.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
S="${1:-$HOME/wasl-ml-data/encoder}"; PY="${PY:-python3}"; SET="${SET:-real_v2}"
mkdir -p "$S/raw" "$S/work" "$S/out"
(cd "$ROOT/pipeline" && go build -o "$ROOT/bin/wsi7" .)
WSI="$ROOT/bin/wsi7"

# 1. data (CC0): NETransliteration wd_arabic + Wikidata labels/aliases for the persons3 QIDs
[ -s "$S/raw/wd_arabic" ] || curl -sSL -o "$S/raw/wd_arabic" \
  https://raw.githubusercontent.com/steveash/NETransliteration-COLING2018/master/data/wd_arabic
"$PY" "$ROOT/encoder/mine_wikidata.py" "$ROOT/data/persons3.json" "$S/raw/wikidata_variants.jsonl"
COMMON=(--variants "$S/raw/wikidata_variants.jsonl" --wsi "$WSI" --work "$S/work")

# 2. the PyTorch twin must reproduce the Go encoder before anything is trained
"$PY" "$ROOT/encoder/parity.py" "$ROOT/models/encoder_v3.json" "$WSI" "$S/work"

# 3. honest baseline: current encoder on the held-out real-variant test + synthetic protocol (eval split)
"$PY" "$ROOT/encoder/eval.py" "${COMMON[@]}" --enc "$ROOT/models/encoder_v3.json" --out "$S/out/heldout_old.json"

# 4. retrain (train split; dev selects the checkpoint)
"$PY" "$ROOT/encoder/train.py" "${COMMON[@]}" --netrans "$S/raw/wd_arabic" --out "$S/out/train_$SET" --epochs 40
cp "$S/out/train_$SET/best.json" "$S/out/encoder_$SET.json"

# 5. evaluate new vs old on the eval split; Go must match PyTorch on the exported weights
"$PY" "$ROOT/encoder/eval.py" "${COMMON[@]}" --enc "$ROOT/models/encoder_v3.json" --enc "$S/out/encoder_$SET.json" \
  --out "$S/out/heldout_$SET.json"
"$PY" "$ROOT/encoder/parity.py" "$S/out/encoder_$SET.json" "$WSI" "$S/work"

# 6. pair scorer + calibrators on the new cosines: 5 train-split cohorts of runtime size, 1 cal-split cohort
for s in 11 12 13 14 15; do
  "$WSI" featdump -wasl "$ROOT/data" -encfile "$S/out/encoder_$SET.json" -split train -persons 5000 -seed "$s" -out "$S/work/feat_train_${SET}_c$s.csv"
done
"$WSI" featdump -wasl "$ROOT/data" -encfile "$S/out/encoder_$SET.json" -split cal -seed 1 -out "$S/work/feat_cal_$SET.csv"
"$PY" "$ROOT/encoder/fit_scorer.py" $(for s in 11 12 13 14 15; do echo --train "$S/work/feat_train_${SET}_c$s.csv"; done) \
  --cal "$S/work/feat_cal_$SET.csv" --outdir "$S/out/chain_$SET" --suffix "$SET"
echo "chain in $S/out/chain_$SET (+ $S/out/encoder_$SET.json). Integrated check: copy into models/ and run"
echo "  WASL_MODELSET=$SET WASL_OUT=$S/out scripts/demo.sh all"
