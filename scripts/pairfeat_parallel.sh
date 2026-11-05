#!/usr/bin/env bash
# Run one experiment arm in parallel, one isolated binary/NATS port range per seed, then create the standard arm layout.
# Usage: scripts/pairfeat_parallel.sh MODE ARM EXPERIMENT_DIR SEED...
# Development/stress/validation manifests and gates are unchanged; run stages in their existing order.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
[ "$#" -ge 4 ] || { echo "usage: scripts/pairfeat_parallel.sh MODE ARM EXPERIMENT_DIR SEED..." >&2; exit 2; }
MODE="$1"; ARM="$2"; EXP="$3"; shift 3
RUNNER="${PAIRFEAT_RUNNER:-$ROOT/scripts/pairfeat_experiment.sh}"
OUT="$EXP/$MODE-$ARM"
[ ! -e "$OUT" ] && [ ! -L "$OUT" ] || { echo "composite arm already exists: $OUT" >&2; exit 2; }
MAX_JOBS="${PAIRFEAT_MAX_JOBS:-3}"
BASE_OFFSET="${PAIRFEAT_PORT_OFFSET:-0}"
LOGS="$EXP/parallel-logs"
mkdir -p "$LOGS"
failed=0
pids=()
seeds=("$@")
i=0
for seed in "${seeds[@]}"; do
  if [ "${#pids[@]}" -ge "$MAX_JOBS" ]; then
    if ! wait "${pids[0]}"; then failed=1; fi
    pids=("${pids[@]:1}")
  fi
  work="$EXP/parallel-$MODE-$ARM-s$seed"
  bin_dir="$ROOT/bin/parallel-$MODE-$ARM-s$seed"
  offset=$((BASE_OFFSET + i * 100))
  PAIRFEAT_SEED="$seed" WASL_BIN="$bin_dir" PAIRFEAT_PORT_OFFSET="$offset" WASL_PORT_OFFSET="$offset" \
    "$RUNNER" "$work" "$MODE" "$ARM" > "$LOGS/$MODE-$ARM-s$seed.log" 2>&1 &
  pids+=("$!")
  i=$((i + 1))
done
for pid in "${pids[@]}"; do
  if ! wait "$pid"; then failed=1; fi
done
[ "$failed" -eq 0 ] || { echo "one or more parallel lanes failed; see $LOGS" >&2; exit 1; }
mkdir "$OUT"
first="$EXP/parallel-$MODE-$ARM-s${seeds[0]}"
cp "$first/build.json" "$OUT/build.json"
python3 -I - "$OUT" "$EXP" "$MODE" "$ARM" "${seeds[@]}" <<'PY'
import json, os, sys
out, exp, mode, arm = sys.argv[1:5]
seeds = sys.argv[5:]
for seed in seeds:
    with open(os.path.join(exp, "parallel-%s-%s-s%s" % (mode, arm, seed), "build.json")) as h:
        b = json.load(h)
    with open(os.path.join(out, "build.json")) as h:
        first = json.load(h)
    if (b.get("git_head"), b.get("binary_sha256")) != (first.get("git_head"), first.get("binary_sha256")):
        raise SystemExit("seed %s used a different git HEAD or binary" % seed)
    os.symlink(os.path.join(exp, "parallel-%s-%s-s%s" % (mode, arm, seed)), os.path.join(out, "v2-s%s" % seed))
PY
printf 'created %s with %d parallel seed lanes\n' "$OUT" "${#seeds[@]}"
