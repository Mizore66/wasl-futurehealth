#!/usr/bin/env bash
# Bounded native-Go fuzz smoke. Use FUZZTIME=60s for a longer reproducible run.
set -euo pipefail

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
DURATION=${FUZZTIME:-3s}
LOG_DIR=${FUZZLOGDIR:-"$ROOT/out/adversarial-fuzz/$(date -u +%Y%m%dT%H%M%SZ)-$$"}
export GOTOOLCHAIN=${GOTOOLCHAIN:-local}
mkdir -p "$LOG_DIR"
LOG_DIR=$(CDPATH= cd -- "$LOG_DIR" && pwd)

cd "$ROOT/pipeline"
for target in \
  FuzzNormaliseNeverPanics \
  FuzzHL7ParseAndResourceNeverPanics \
  FuzzMLLPBoundedFrame \
  FuzzSubscriptionStatusNeverPanics \
  FuzzAPIReviewBodyNeverPanics
do
  printf '\n== %s (%s per target) ==\n' "$target" "$DURATION"
  log="$LOG_DIR/$target.log"
  if ! go test -run="^${target}$" -fuzz="^${target}$" -fuzztime="$DURATION" -parallel=4 -count=1 -v 2>&1 | tee "$log"; then
    printf 'fuzz target failed; raw log: %s\n' "$log" >&2
    exit 1
  fi

  baseline=
  executions=
  while IFS= read -r line; do
    if [[ "$line" =~ gathering[[:space:]]baseline[[:space:]]coverage:[[:space:]][0-9]+/([0-9]+)[[:space:]]completed,[[:space:]]now[[:space:]]fuzzing ]]; then
      baseline=${BASH_REMATCH[1]}
    fi
    if [[ "$line" =~ execs:[[:space:]]([0-9]+) ]]; then
      executions=${BASH_REMATCH[1]}
    fi
  done < "$log"
  if [[ -z "$baseline" || -z "$executions" ]] || (( 10#$executions <= 10#$baseline )); then
    printf 'fuzz target did not report generated executions beyond its baseline; raw log: %s\n' "$log" >&2
    exit 1
  fi
  if ! grep -q -- "--- PASS: ${target}" "$log"; then
    printf 'fuzz target did not report its own PASS; raw log: %s\n' "$log" >&2
    exit 1
  fi
  printf 'generated executions: %d (total %d, baseline %d); raw log: %s\n' \
    "$((10#$executions - 10#$baseline))" "$((10#$executions))" "$((10#$baseline))" "$log"
done
printf '\nAll fuzz targets generated executions and passed. Raw logs: %s\n' "$LOG_DIR"
