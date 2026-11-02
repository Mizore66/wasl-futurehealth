#!/usr/bin/env bash
# One protected paired CRC pair-scorer lane set (all seeds of a mode) for one model arm. No paid model use.
# Usage: scripts/pairfeat_experiment.sh NEW_ROOT MODE ARM
#   MODE  dev-clean (dev; 307 311 313; rerr 0/0) | dev-stress (dev; 307 311 313; rerr 0.04, ungraded 0.25)
#         | validation-clean (eval; 401 409 419 421 431; rerr 0/0)
#   ARM   baseline (default modelset) | pairfeat1 (-modelset pairfeat1 on orchestrate/audit/report7 only)
# Both arms: -linkfloor 0.01, -policy v7strict; gen/sim/augment are identical, so cohort and truth denominator match.
# NEW_ROOT must be a new path under ~/wasl-ml-data (one root per arm; never reused or deleted). Its parent is the
# experiment directory, which holds (see scripts/pairfeat_compare.py):
#   artifact-manifest.json   training artifacts: git_head + baseline/pairfeat1 model-chain SHA256 (required by dev modes)
#   selection.json           written by `pairfeat_compare.py dev-clean`; dev-stress needs selected_modelset "pairfeat1"
#   stress.json              written by `pairfeat_compare.py dev-stress`; consumed by `pairfeat_compare.py freeze-validation`
#   validation-manifest.json written by `pairfeat_compare.py freeze-validation`; the only file validation-clean reads
# dev-clean/dev-stress require artifact-manifest.json git_head == current HEAD and the arm's four models/ files to match its
# map; validation-clean requires the frozen manifest's HEAD, seeds, settings, runner/analyzer SHA256 and the arm's hashes.
# All refusals happen before any build. PAIRFEAT_PREFLIGHT_ONLY=1 stops after the refusals (tests; nothing is built or created).
# The tracked tree must be clean and pipeline/ and scripts/ must have no untracked files before any build.
# Arms must run sequentially (shared bin/wsi7, removed on exit); build.json records git head, binary SHA256, mode, arm,
# modelset, floor 0.01 and the arm's model-chain SHA256. Cached reader, image off, FHIR, chaos 0, subkill 1, no NATS kill, HNSW.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export PATH="/opt/anaconda3/bin:$PATH"
while read -r v; do [ -n "$v" ] && unset "$v"; done < <(compgen -e | grep '^WASL_' || true)  # no env-driven settings

[ "$#" -eq 3 ] && [ -n "$1" ] || { echo "usage: scripts/pairfeat_experiment.sh NEW_ROOT MODE ARM" >&2; exit 2; }
REQ="$1"; MODE="$2"; ARM="$3"
case "$MODE" in
  dev-clean)        SPLIT=dev;  SEEDS="307 311 313";             RERR=0;    ERRU=0;;
  dev-stress)       SPLIT=dev;  SEEDS="307 311 313";             RERR=0.04; ERRU=0.25;;
  validation-clean) SPLIT=eval; SEEDS="401 409 419 421 431";     RERR=0;    ERRU=0;;
  *) echo "invalid mode: $MODE" >&2; exit 2;;
esac
case "$ARM" in baseline|pairfeat1) ;; *) echo "invalid arm: $ARM (allowed baseline pairfeat1)" >&2; exit 2;; esac
[ -n "${HOME:-}" ] || { echo "HOME is not set" >&2; exit 2; }
DATA_BASE="$HOME/wasl-ml-data"
[ -d "$DATA_BASE" ] || { echo "missing $DATA_BASE" >&2; exit 2; }
DATA_BASE="$(cd "$DATA_BASE" && pwd -P)"
case "$REQ" in "$HOME/wasl-ml-data"/*) ;; *) echo "root must be under ~/wasl-ml-data" >&2; exit 2;; esac
case "$REQ/" in */../*|*/./*|*//*) echo "root must not contain ., .. or empty segments" >&2; exit 2;; esac
[ ! -e "$REQ" ] && [ ! -L "$REQ" ] || { echo "root already exists; a new output root is required: $REQ" >&2; exit 2; }
PARENT="$(dirname "$REQ")"
[ -d "$PARENT" ] && [ ! -L "$PARENT" ] || { echo "parent experiment directory must exist and not be a symlink: $PARENT" >&2; exit 2; }
PARENT="$(cd "$PARENT" && pwd -P)"
case "$PARENT/" in "$DATA_BASE"/*) ;; *) echo "root resolves outside ~/wasl-ml-data" >&2; exit 2;; esac

# identity refusals (artifact manifest, passing selection, frozen validation manifest, model-chain SHA256) before any build
python3 -I "$ROOT/scripts/pairfeat_compare.py" preflight "$MODE" "$ARM" "$PARENT" >/dev/null \
  || { echo "preflight refused: $MODE $ARM lanes are not authorised by the manifests in $PARENT" >&2; exit 2; }
if [ "${PAIRFEAT_PREFLIGHT_ONLY:-}" = 1 ]; then echo "preflight ok ($MODE $ARM); PAIRFEAT_PREFLIGHT_ONLY set, nothing built"; exit 0; fi

DIRTY="$(git -C "$ROOT" status --porcelain --untracked-files=all -- pipeline scripts | grep -v '__pycache__' || true)"
TRACKED="$(git -C "$ROOT" diff HEAD --name-only)"
if [ -n "$DIRTY" ] || [ -n "$TRACKED" ]; then echo "working tree not clean (tracked changes or untracked files under pipeline/scripts); refusing to build" >&2; exit 2; fi

# independent re-check on the clean tree: the governing manifest is bound to this exact HEAD (and, for validation, these scripts)
HEAD="$(git -C "$ROOT" rev-parse HEAD)"
python3 -I - "$PARENT" "$MODE" "$HEAD" "$ROOT/scripts/pairfeat_experiment.sh" "$ROOT/scripts/pairfeat_compare.py" <<'PY' \
  || { echo "manifest is not bound to the current clean HEAD; refusing to build" >&2; exit 2; }
import hashlib, json, os, sys
parent, mode, head, runner, analyzer = sys.argv[1:6]
sha = lambda p: hashlib.sha256(open(p, "rb").read()).hexdigest()
name = "validation-manifest.json" if mode == "validation-clean" else "artifact-manifest.json"
with open(os.path.join(parent, name)) as h:
    m = json.load(h)
if m.get("git_head") != head:
    sys.exit(1)
if mode == "validation-clean" and (m.get("runner_sha256") != sha(runner) or m.get("analyzer_sha256") != sha(analyzer)):
    sys.exit(1)
if mode == "dev-stress":
    with open(os.path.join(parent, "selection.json")) as h:
        s = json.load(h)
    if s.get("git_head") != head or s.get("selected_modelset") != "pairfeat1" or s.get("pass") is not True:
        sys.exit(1)
PY

WORK="$PARENT/$(basename "$REQ")"
mkdir "$WORK"

"$ROOT/scripts/demo.sh" check
"$ROOT/scripts/demo.sh" build
trap 'rm -f "$ROOT/bin/wsi7"' EXIT

# per-arm identity before any lane: full git head + SHA256 of the freshly built binary + the arm's verified model chain
python3 -I - "$ROOT" "$WORK" "$PARENT" "$HEAD" "$ROOT/bin/wsi7" "$MODE" "$ARM" <<'PY'
import hashlib, json, os, sys
root, work, parent, head, binp, mode, arm = sys.argv[1:8]
sys.path.insert(0, os.path.join(root, "scripts"))
import pairfeat_compare as pfc
chain = pfc.preflight(mode, arm, parent, root, head)  # re-verified after the build, immediately before recording
with open(binp, "rb") as h:
    sha = hashlib.sha256(h.read()).hexdigest()
with open(os.path.join(work, "build.json"), "w") as h:
    json.dump({"git_head": head, "binary_sha256": sha, "mode": mode, "arm": arm, "modelset": pfc.MODELSET[arm],
               "floor": float(pfc.FLOOR), "policy": pfc.POLICY, "model_chain_sha256": chain}, h, indent=1, sort_keys=True)
PY

python3 -I - "$ROOT" "$WORK" "$MODE" "$ARM" "$SPLIT" "$RERR" "$ERRU" $SEEDS <<'PY'
import json, os, subprocess, sys
root, work, mode, arm, split, rerr, erru = sys.argv[1:8]
seeds = [int(x) for x in sys.argv[8:]]
sys.path.insert(0, os.path.join(root, "scripts"))
import pairfeat_compare as pfc
m = pfc.MODES[mode]
if (m["split"], m["seeds"], m["rerr"], m["errungraded"]) != (split, seeds, rerr, erru) or arm not in pfc.ARMS:
    sys.exit("mode settings mismatch")
bin_ = os.path.join(root, "bin", "wsi7")

def run(argv, log):
    try:
        pfc.guard(argv, mode, arm, bin_)
    except pfc.GuardError as e:
        sys.stderr.write("REFUSED by guard: %s\n" % e)
        sys.exit(2)
    with open(log, "wb") as h:
        p = subprocess.run(argv, stdout=h, stderr=subprocess.STDOUT)
    if p.returncode != 0:
        sys.stderr.write("FAILED (%d): %s\n  log: %s\n" % (p.returncode, " ".join(argv[:2]), log))
        sys.exit(p.returncode)

for seed in seeds:
    o = os.path.join(work, "v2-s%d" % seed)
    os.mkdir(o); os.mkdir(os.path.join(o, "run"))
    r = os.path.join(o, "run", "main")
    for name, argv, log in pfc.lane_commands(root, bin_, o, seed, mode, arm):
        if name == "classifier":
            with open(os.path.join(r, "status-linker.json")) as h:
                st = json.load(h)  # required: a missing file aborts the lane
            if st.get("link_funnel_error"):
                sys.exit("link funnel trace write failed (seed %d)" % seed)
        run(argv, os.path.join(o, log))
PY

python3 -I - "$ROOT" "$WORK" "$MODE" "$ARM" $SEEDS <<'PY'
import json, os, sys
root, work, mode, arm = sys.argv[1:5]
seeds = [int(x) for x in sys.argv[5:]]
sys.path.insert(0, os.path.join(root, "scripts"))
import pairfeat_compare as pfc
if pfc.MODES[mode]["seeds"] != seeds:
    sys.exit("seed mismatch")
lanes = []
for seed in seeds:
    lm = pfc._load_lane(os.path.join(work, "v2-s%d" % seed), seed, arm, mode)  # funnel floor 0.01, arm modelset, mode scope
    if lm["analysis_errors"] or lm["trace_error"] or lm["replay_mismatch"]:
        sys.exit("seed %d: analysis/trace-write error or replay mismatch" % seed)
    lanes.append(lm)
out = pfc.aggregate_arm(arm, lanes)
with open(os.path.join(work, "build.json")) as h:
    out["build"] = json.load(h)
out["scope"] = {"mode": mode, "arm": arm, "modelset": pfc.MODELSET[arm], "floor": float(pfc.FLOOR), "policy": pfc.POLICY,
                "seeds": seeds, "reader": "cached", "image": "off", "chaos": 0, "nats_kill": False,
                "note": "simulated; tp = report7 programmes_live.crc.pred_closed - false_closures; link attach rate is separate"}
with open(os.path.join(work, "summary.json"), "w") as h:
    json.dump(out, h, indent=1, sort_keys=True)
print(json.dumps({"arm": arm, "mode": mode, "recall": out["recall"], "tp": out["tp"], "truth_closed": out["truth_closed"]}, sort_keys=True))
PY
