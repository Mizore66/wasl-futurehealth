#!/usr/bin/env bash
# One-command verification of Wasl's frozen evaluation (`make verify`, or `make verify SEED=<any integer>`).
#
#   scripts/verify.sh            default (about 1-2 minutes, no Docker, no network): prints PASS/FAIL per claim
#     1. every committed file under pipeline/results/final matches pipeline/results/final/SHA256SUMS
#     2. the model/data inputs at the frozen code revision match the hashes manifest.json recorded before the runs
#     3. summary.json recomputes exactly from the per-seed results with scripts/final_aggregate.py
#     4. named fast Go tests for the safety properties (no closure on a wrong-patient link, tamper detection, consent hold)
#     5. web/data/evidence.json (if present) carries the same numbers as summary.json
#   scripts/verify.sh --ci       steps 1 and 5 only (seconds; the PR fast tier runs this)
#   scripts/verify.sh --seed N   judge-seed mode: a fresh held-out cohort generated with seed N, run end to end through the
#                                frozen code revision (gen -> linker + closure engine over a local 3-node NATS cluster with
#                                chaos -> audit -> tamper -> report7) and scored against the generator's truth. Reduced
#                                size: 600 CRC + 240 cervical loops (the frozen evaluation used 2,040 + 816 plus five
#                                other programmes); about 5-10 minutes on a laptop. Needs go, python3, git history and
#                                nats-server (bin/nats-server or on PATH). Run dir: out/verify/seed-N/.
#     env: WASL_VERIFY_REV=HEAD runs the current checkout instead of the frozen revision; WASL_VERIFY_NOISE=v1|v2
#          (default v2, the independent held-out corruptor); WASL_VERIFY_CRC / WASL_VERIFY_CERV loop counts;
#          WASL_PORT_OFFSET (default 500) moves the NATS/service ports away from a running demo.
# Exit status: 0 when every claim is PASS (SKIP is allowed), 1 on any FAIL, 2 on a usage or prerequisite error.
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CHECK=(python3 "$ROOT/scripts/verify_check.py")
MODE=default SEED=""
case "${1:-}" in
  "") ;;
  --ci) MODE=ci ;;
  --seed) MODE=seed; SEED="${2:-}" ;;
  *) sed -n 2,22p "$0"; exit 2 ;;
esac
command -v python3 >/dev/null || { echo "python3 is required"; exit 2; }
FAIL=0
step() { "$@" || FAIL=1; }

if [ "$MODE" != seed ]; then
  t0=$(date +%s)
  echo "Wasl verification ($MODE) at $(git -C "$ROOT" rev-parse --short=12 HEAD 2>/dev/null || echo 'no git')"
  step "${CHECK[@]}" sums check
  if [ "$MODE" = default ]; then
    step "${CHECK[@]}" inputs
    REC=$(mktemp); "${CHECK[@]}" recompute > "$REC" 2>&1 & RECPID=$!   # ~75 s of permutation tests; overlaps the Go build
    if command -v go >/dev/null; then
      CLAIMS=$(mktemp); GOLOG=$(mktemp)
      cat > "$CLAIMS" <<'J'
{
 "No loop closes on an untrusted or wrong-patient identity link (lung, DR, breast, LDCT, IFHAS; EID guard: relative's Emirates ID)": [
  "TestLungNoClosureOnWrongPatientLink", "TestDRNoClosureOnWrongPatientLink", "TestBreastNoClosureOnWrongPatientLink",
  "TestLDCTNoClosureOnWrongPatientLink", "TestIfhasNoClosureOnWrongPatientLink", "TestLinkFloorNearFactRaisesReviewWithoutClosing",
  "TestEIDWrongPersonDirectDoesNotClose", "TestEIDWrongPersonMultiHopDoesNotClose"],
 "A model merge cannot bridge two different people (conflicting Emirates IDs block bridge and transitive merges)": [
  "TestDemoModelMergeCannotBridge", "TestEIDConflictBlocksBridgeMerge", "TestEIDConflictBlocksTransitiveClusterMerge"],
 "Tampering with the run or ledgers is detected (edited, reordered, removed, forged events)": [
  "TestUndoLedgerTamperRejected", "TestLoadAPIVerifiedRejectsTamper", "TestOutcomeHandoffReplayRejectsTamperedHistory",
  "TestTamperRejectsRunWithoutClosedHumanSignedPrediction"],
 "Outreach is held until consent is granted and cancelled on opt-out": [
  "TestConsentHoldNeverDispatchesWithoutGrant", "TestConsentReferralBeforeConsentHoldsThenReleases",
  "TestConsentOptOutAfterQueueingCancels", "TestConsentNeverSharesAcrossUntrustedOrUnauthorizedRecords"]
}
J
      RUN=$(python3 -c "import json,sys; print('^(' + '|'.join(t for v in json.load(open(sys.argv[1])).values() for t in v) + ')\$')" "$CLAIMS")
      (cd "$ROOT/pipeline" && go test -count=1 -v -run "$RUN" . > "$GOLOG" 2>&1)
      step "${CHECK[@]}" gotests "$GOLOG" "$CLAIMS"
      grep -qE '^(ok|PASS)' "$GOLOG" || { echo "      go test output (tail):"; tail -20 "$GOLOG" | sed 's/^/      /'; }
      rm -f "$CLAIMS" "$GOLOG"
    else
      echo "FAIL  Go safety tests  (go not found; install Go $(sed -n 's/^go //p' "$ROOT/pipeline/go.mod") from https://go.dev/dl/)"; FAIL=1
    fi
    wait "$RECPID" || FAIL=1; cat "$REC"; rm -f "$REC"
  fi
  step "${CHECK[@]}" evidence
  echo "----"
  [ "$FAIL" = 0 ] && echo "ALL PASS ($(( $(date +%s) - t0 )) s)" || echo "FAILED ($(( $(date +%s) - t0 )) s)"
  exit "$FAIL"
fi

# ---- judge-seed mode ----
[[ "$SEED" =~ ^-?[0-9]+$ ]] || { echo "usage: scripts/verify.sh --seed <integer>"; exit 2; }
for c in go git; do command -v "$c" >/dev/null || { echo "$c is required for --seed"; exit 2; }; done
NATS="$ROOT/bin/nats-server"; [ -x "$NATS" ] || NATS="$(command -v nats-server || true)"
[ -n "$NATS" ] && [ -x "$NATS" ] || { echo "nats-server is required for --seed: brew install nats-server, or"
  echo "  GOBIN=\"$ROOT/bin\" go install github.com/nats-io/nats-server/v2@v2.12.6"; exit 2; }
MAN="$ROOT/pipeline/results/final/manifest.json"
FROZEN=$(python3 -c "import json,sys; print(json.load(open(sys.argv[1]))['code_revision'])" "$MAN")
REV="${WASL_VERIFY_REV:-$FROZEN}"; NOISE="${WASL_VERIFY_NOISE:-v2}"; CRC="${WASL_VERIFY_CRC:-600}"; CERV="${WASL_VERIFY_CERV:-240}"
[ "$NOISE" = v1 ] || [ "$NOISE" = v2 ] || { echo "WASL_VERIFY_NOISE must be v1 or v2"; exit 2; }
export WASL_PORT_OFFSET="${WASL_PORT_OFFSET:-500}" WASL_SYNTHETIC_DATA_ONLY=1
OUT="$ROOT/out/verify/seed-$SEED"; rm -rf "$OUT"; mkdir -p "$OUT/bin"
t0=$(date +%s); tl=$t0
lap() { local now; now=$(date +%s); printf '  %-11s %4d s\n' "$1" $((now - tl)); tl=$now; }
if [ "$REV" = HEAD ]; then
  SRC="$ROOT"; REVID="$(git -C "$ROOT" rev-parse HEAD)"
  [ -z "$(git -C "$ROOT" status --porcelain -- pipeline scripts models data patient-parsers)" ] || REVID="$REVID (with uncommitted changes)"
else
  git -C "$ROOT" cat-file -e "$REV^{commit}" 2>/dev/null || { echo "revision $REV is not in this clone: git fetch --unshallow (or WASL_VERIFY_REV=HEAD)"; exit 2; }
  SRC="$OUT/src"; REVID="$(git -C "$ROOT" rev-parse "$REV")"; mkdir -p "$SRC"
  git -C "$ROOT" archive "$REVID" | tar -x -C "$SRC"   # the frozen code, models and data, exactly as committed
fi
echo "Judge-seed run: seed $SEED, noise $NOISE, revision ${REVID:0:12}$([ "$REVID" = "$FROZEN" ] && echo ' (the frozen evaluation revision)'), $CRC CRC + $CERV cervical loops"
echo "Run dir: $OUT"
(cd "$SRC/pipeline" && go build -o "$OUT/bin/wsi7" .) > "$OUT/build.log" 2>&1 || { echo "FAIL  build (see $OUT/build.log)"; exit 1; }
cp "$NATS" "$OUT/bin/nats-server"; lap build
B="$OUT/bin"; D="$SRC/data"; M="$SRC/models"; R="$OUT/run/main"
# the frozen scripts/final_eval.sh run_one, with lung/DR/breast/LDCT/IFHAS loops off and smaller CRC/cervical counts
PARGS=(-lungsurv -xreflink -drnext)
run() { local name="$1"; shift; "$@" || { echo "FAIL  $name (exit $?; logs in $OUT)"; exit 1; }; lap "$name"; }
run gen "$B/wsi7" gen -wasl "$D" -reports "$D/evidence/reports43.json" -pathology "$D/evidence/pathology_templates.json" -prepquery=true -handoff=true \
  -lungworkup=true -refreject=true -ecpool "$SRC/patient-parsers/econsult_testset.json" -tbpool "$SRC/patient-parsers/teachback_testset.json" \
  -out "$OUT/base" -seed "$SEED" -loops "$CRC" -persons 0 -cerv "$CERV" -noise "$NOISE" -lung 0 -lungdata "$D/lung" -dr 0 -drdata "$D/dr" \
  -breast 0 -breastdata "$D/breast" -ldct 0 -ldctdata "$D/ldct" -ifhas 0 -ifhasdata "$D/ifhas" -lungsurv -drnext > "$OUT/gen.log" 2>&1
run sim "$B/wsi7" sim -data "$OUT/base" -wasl "$M" -run "$OUT/run/base" > "$OUT/sim.json" 2> "$OUT/sim.log"
run augment python3 "$SRC/pipeline/augment_cohort.py" "$OUT/base" "$OUT/run/base" "$OUT/cohort" > "$OUT/augment.log"
cd "$B" || exit 2
run orchestrate ./wsi7 orchestrate -prepquery=true -handoff=true -lungworkup=true -data "$OUT/cohort" -wasl "$M" -run "$R" -preps 3 -threads 8 \
  -source fhir -subkill 1 -chaos 2 -natskill -ann hnsw -rerr 0.04 -rdouble merge+close -dual -rseed "$SEED" -seed "$SEED" -policy v7strict "${PARGS[@]}" > "$OUT/orchestrate.log" 2>&1
run audit ./wsi7 audit -data "$OUT/cohort" -wasl "$M" -run "$R" -ann hnsw -dual -policy v7strict "${PARGS[@]}" > "$R/audit.stdout" 2>&1
run tamper ./wsi7 tamper -role auditor -data "$OUT/cohort" -wasl "$M" -run "$R" -ann hnsw -dual -policy v7strict "${PARGS[@]}" -seed "$SEED" > "$R/tamper.json" 2> "$R/tamper.log"
run report7 ./wsi7 report7 -data "$OUT/cohort" -wasl "$M" -run "$R" -ann hnsw -dual -policy v7strict "${PARGS[@]}" > /dev/null 2> "$R/report7.log"
rm -rf "$R"/nats-* "$OUT/run/base" "$OUT/src"   # keep logs, truth, report7.json and tamper.json; drop NATS stores and the source copy
echo "  total       $(( $(date +%s) - t0 )) s"
"${CHECK[@]}" seed "$R" "$SEED" "$NOISE" "$REVID" "$CRC" "$CERV"; rc=$?
echo "----"; echo "Run dir: $OUT (cohort/truth.json is the answer key; run/main/report7.json and tamper.json are the scored outputs)"
[ "$rc" = 0 ] && echo "ALL SAFETY CHECKS PASS for seed $SEED" || echo "FAILED for seed $SEED"
exit "$rc"
