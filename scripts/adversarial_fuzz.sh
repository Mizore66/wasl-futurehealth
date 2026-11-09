#!/usr/bin/env bash
# Bounded native-Go fuzz smoke. Use FUZZTIME=60s for a longer reproducible run.
set -euo pipefail

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
DURATION=${FUZZTIME:-3s}
export GOTOOLCHAIN=${GOTOOLCHAIN:-local}

cd "$ROOT/pipeline"
for target in \
  FuzzNormaliseNeverPanics \
  FuzzHL7ParseAndResourceNeverPanics \
  FuzzMLLPBoundedFrame \
  FuzzSubscriptionStatusNeverPanics \
  FuzzAPIReviewBodyNeverPanics
do
  printf '\n== %s (%s per target) ==\n' "$target" "$DURATION"
  go test -run="^${target}$" -fuzz="^${target}$" -fuzztime="$DURATION" -parallel=4 -count=1
done
