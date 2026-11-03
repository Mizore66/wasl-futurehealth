#!/usr/bin/env bash
# One protected paired CRC pair-scorer lane set (all seeds of a mode) for one model arm. No paid model use.
# Usage: scripts/pairfeat_experiment.sh NEW_ROOT MODE ARM
#   MODE  dev-clean (dev; candidate dev seeds; rerr 0/0) | dev-stress (dev; candidate dev seeds; rerr 0.04, ungraded 0.25)
#         | validation-clean (eval; 401 409 419 421 431, r1review 601 607 613 617 619; rerr 0/0)
#   ARM   baseline (default modelset) | pairfeat1 | pairfeat2 | pairfeat3 | pairfeat4 | pairfeat5 (-modelset <candidate> on orchestrate/audit/report7 only)
#         | r1review (-modelset pairfeat5 plus -r1review on orchestrate/audit/report7 only)
# Candidate dev seeds: pairfeat1 307 311 313; pairfeat2 317 331 337; pairfeat3 347 353 367; pairfeat4 433 449 461; pairfeat5 521 541 547;
# r1review 557 563 569. An r1review experiment (named by its write-once experiment.json, `pairfeat_compare.py init-experiment`,
# over a pairfeat5 artifact-manifest.json) serves three arms: baseline, pairfeat5 (reference; exactly a pairfeat5 lane) and
# r1review (the pairfeat5 chain plus -r1review), all on r1review's seeds. Every arm of an r1review experiment runs gen with
# -imgphotos and orchestrate/audit/report7 with -image cached (both -imgcache models/image_verdicts.json); pairfeat1-pairfeat5
# experiments keep the image reader off (spec amendment 2).
# The experiment parent serves exactly one candidate,
# inferred from the single <candidate>_chain_sha256 key of its artifact-manifest.json (or, for validation, the candidate
# its frozen validation-manifest.json records); ARM must be baseline or that candidate, and the baseline arm uses the
# candidate's seeds.
# Both arms: -linkfloor 0.01, -policy v7strict; gen/sim/augment are identical, so cohort and truth denominator match.
# NEW_ROOT must be a new path under ~/wasl-ml-data (one root per arm; never reused or deleted). Its parent is the
# experiment directory, which holds (see scripts/pairfeat_compare.py):
#   artifact-manifest.json   training artifacts: git_head + baseline and one candidate model-chain SHA256 (required by dev modes)
#   selection.json           written by `pairfeat_compare.py dev-clean`; dev-stress needs selected_modelset = the candidate
#   stress.json              written by `pairfeat_compare.py dev-stress`; consumed by `pairfeat_compare.py freeze-validation`
#   validation-manifest.json written by `pairfeat_compare.py freeze-validation`; authorises validation-clean only while the
#                            artifact-manifest/selection/stress files it hashes are unchanged and still pass
# dev-clean/dev-stress require artifact-manifest.json git_head == current HEAD and the arm's four models/ files to match its
# map; validation-clean requires the frozen manifest's HEAD, seeds, settings, runner/analyzer SHA256, upstream file SHA256
# (re-verified selection and stress) and the arm's hashes.
# All refusals happen before any build. PAIRFEAT_PREFLIGHT_ONLY=1 stops after the refusals (tests; nothing is built or created).
# The tracked tree must be clean and pipeline/ and scripts/ must have no untracked files before any build.
# Arms must run sequentially (shared bin/wsi7, removed on exit); build.json records git head, binary SHA256, mode, arm,
# candidate, modelset, floor 0.01 and the arm's model-chain SHA256 (r1review experiments: also image cached, imgphotos and the
# image_verdicts.json SHA256). Cached reader, image off (r1review experiments: cached image reader), FHIR, chaos 0, subkill 1,
# no NATS kill, HNSW.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export PATH="/opt/anaconda3/bin:$PATH"
while read -r v; do [ -n "$v" ] && unset "$v"; done < <(compgen -e | grep '^WASL_' || true)  # no env-driven settings

[ "$#" -eq 3 ] && [ -n "$1" ] || { echo "usage: scripts/pairfeat_experiment.sh NEW_ROOT MODE ARM" >&2; exit 2; }
REQ="$1"; MODE="$2"; ARM="$3"
case "$MODE" in
  dev-clean)        SPLIT=dev;  RERR=0;    ERRU=0;;
  dev-stress)       SPLIT=dev;  RERR=0.04; ERRU=0.25;;
  validation-clean) SPLIT=eval; RERR=0;    ERRU=0;;
  *) echo "invalid mode: $MODE" >&2; exit 2;;
esac
case "$ARM" in baseline|pairfeat1|pairfeat2|pairfeat3|pairfeat4|pairfeat5) ;; r1review) ;; *) echo "invalid arm: $ARM (allowed baseline pairfeat1 pairfeat2 pairfeat3 pairfeat4 pairfeat5 r1review)" >&2; exit 2;; esac
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

# identity refusals (artifact manifest + its one candidate, passing selection, frozen validation manifest, model-chain
# SHA256) before any build
PF="$(python3 -I "$ROOT/scripts/pairfeat_compare.py" preflight "$MODE" "$ARM" "$PARENT")" \
  || { echo "preflight refused: $MODE $ARM lanes are not authorised by the manifests in $PARENT" >&2; exit 2; }
CAND="$(printf '%s\n' "$PF" | python3 -I -c 'import json, sys; print(json.load(sys.stdin)["candidate"])')" \
  || { echo "preflight refused: no experiment candidate reported for $PARENT" >&2; exit 2; }
# independent seed table (cross-checked against pairfeat_compare.CANDIDATES before any lane); CHAIN is the model chain
# (manifest key) the candidate runs, REF a three-arm candidate's reference arm
VAL_SEEDS="401 409 419 421 431"
CHAIN="$CAND"; REF=""
case "$CAND" in
  pairfeat1) DEV_SEEDS="307 311 313";;
  pairfeat2) DEV_SEEDS="317 331 337";;
  pairfeat3) DEV_SEEDS="347 353 367";;
  pairfeat4) DEV_SEEDS="433 449 461";;
  pairfeat5) DEV_SEEDS="521 541 547";;
  r1review) DEV_SEEDS="557 563 569"; VAL_SEEDS="601 607 613 617 619"; CHAIN=pairfeat5; REF=pairfeat5;;
  *) echo "preflight refused: unknown experiment candidate: $CAND" >&2; exit 2;;
esac
case "$ARM" in baseline|"$CAND") ;; *) [ -n "$REF" ] && [ "$ARM" = "$REF" ] || { echo "arm $ARM is not served by the $CAND experiment in $PARENT" >&2; exit 2; };; esac
if [ "$MODE" = validation-clean ]; then SEEDS="$VAL_SEEDS"; else SEEDS="$DEV_SEEDS"; fi
if [ "${PAIRFEAT_PREFLIGHT_ONLY:-}" = 1 ]; then
  echo "preflight ok ($MODE $ARM; candidate $CAND seeds $SEEDS); PAIRFEAT_PREFLIGHT_ONLY set, nothing built"; exit 0
fi

DIRTY="$(git -C "$ROOT" status --porcelain --untracked-files=all -- pipeline scripts | grep -v '__pycache__' || true)"
TRACKED="$(git -C "$ROOT" diff HEAD --name-only)"
if [ -n "$DIRTY" ] || [ -n "$TRACKED" ]; then echo "working tree not clean (tracked changes or untracked files under pipeline/scripts); refusing to build" >&2; exit 2; fi

# independent re-check on the clean tree: the governing manifest is bound to this exact HEAD (and, for validation, these scripts)
HEAD="$(git -C "$ROOT" rev-parse HEAD)"
python3 -I - "$PARENT" "$MODE" "$HEAD" "$ROOT/scripts/pairfeat_experiment.sh" "$ROOT/scripts/pairfeat_compare.py" "$CAND" "$CHAIN" <<'PY' \
  || { echo "manifest is not bound to the current clean HEAD and candidate; refusing to build" >&2; exit 2; }
import hashlib, json, os, sys
parent, mode, head, runner, analyzer, cand, chain = sys.argv[1:8]
sha = lambda p: hashlib.sha256(open(p, "rb").read()).hexdigest()
name = "validation-manifest.json" if mode == "validation-clean" else "artifact-manifest.json"
with open(os.path.join(parent, name)) as h:
    m = json.load(h)
if m.get("git_head") != head:
    sys.exit(1)
if [k for k in m if k.endswith("_chain_sha256") and k != "baseline_chain_sha256"] != [chain + "_chain_sha256"]:
    sys.exit(1)  # exactly one candidate chain, and it is the chain this run's candidate runs
ep = os.path.join(parent, "experiment.json")
if chain != cand:  # a candidate borrowing a chain is named only by experiment.json
    if not os.path.isfile(ep) or os.path.islink(ep):
        sys.exit(1)
    with open(ep) as h:
        e = json.load(h)
    if not isinstance(e, dict) or e.get("candidate") != cand or e.get("reference") != chain:
        sys.exit(1)
elif os.path.lexists(ep):
    sys.exit(1)  # a parent with experiment.json serves only the candidate it names
if mode == "validation-clean" and (m.get("runner_sha256") != sha(runner) or m.get("analyzer_sha256") != sha(analyzer)
                                   or m.get("candidate") != cand):
    sys.exit(1)
if mode == "dev-stress":
    with open(os.path.join(parent, "selection.json")) as h:
        s = json.load(h)
    if s.get("git_head") != head or s.get("selected_modelset") != cand or s.get("candidate") != cand or s.get("pass") is not True:
        sys.exit(1)
PY

WORK="$PARENT/$(basename "$REQ")"
mkdir "$WORK"

"$ROOT/scripts/demo.sh" check
"$ROOT/scripts/demo.sh" build
trap 'rm -f "$ROOT/bin/wsi7"' EXIT

# per-arm identity before any lane: full git head + SHA256 of the freshly built binary + the arm's verified model chain
python3 -I - "$ROOT" "$WORK" "$PARENT" "$HEAD" "$ROOT/bin/wsi7" "$MODE" "$ARM" "$CAND" <<'PY'
import hashlib, json, os, sys
root, work, parent, head, binp, mode, arm, cand = sys.argv[1:9]
sys.path.insert(0, os.path.join(root, "scripts"))
import pairfeat_compare as pfc
got, chain = pfc.preflight_identity(mode, arm, parent, root, head)  # re-verified after the build, immediately before recording
if got != cand:
    sys.exit("experiment candidate changed since preflight (%s != %s)" % (got, cand))
with open(binp, "rb") as h:
    sha = hashlib.sha256(h.read()).hexdigest()
with open(os.path.join(work, "build.json"), "w") as h:
    b = {"git_head": head, "binary_sha256": sha, "mode": mode, "arm": arm, "candidate": cand, "modelset": pfc.MODELSET[arm],
         "floor": float(pfc.FLOOR), "policy": pfc.POLICY, "model_chain_sha256": chain}
    if pfc.extra_flags(arm):  # r1review arm only: recorded so the analysis can tell its lanes from pairfeat5 lanes
        b["extra_flags"] = pfc.extra_flags(arm)
    b.update(pfc.image_build(root, cand))  # r1review experiments (every arm): image cached, imgphotos, verdict-cache SHA256
    json.dump(b, h, indent=1, sort_keys=True)
PY

python3 -I - "$ROOT" "$WORK" "$MODE" "$ARM" "$CAND" "$SPLIT" "$RERR" "$ERRU" $SEEDS <<'PY'
import json, os, subprocess, sys
root, work, mode, arm, cand, split, rerr, erru = sys.argv[1:9]
seeds = [int(x) for x in sys.argv[9:]]
sys.path.insert(0, os.path.join(root, "scripts"))
import pairfeat_compare as pfc
m = pfc.mode_settings(mode, cand)
if (m["split"], m["seeds"], m["rerr"], m["errungraded"]) != (split, seeds, rerr, erru) or arm not in pfc.arms(cand):
    sys.exit("mode settings mismatch")
bin_ = os.path.join(root, "bin", "wsi7")

def run(argv, log):
    try:
        pfc.guard(argv, mode, arm, bin_, cand)
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
    for name, argv, log in pfc.lane_commands(root, bin_, o, seed, mode, arm, candidate=cand):
        if name == "classifier":
            with open(os.path.join(r, "status-linker.json")) as h:
                st = json.load(h)  # required: a missing file aborts the lane
            if st.get("link_funnel_error"):
                sys.exit("link funnel trace write failed (seed %d)" % seed)
        run(argv, os.path.join(o, log))
PY

python3 -I - "$ROOT" "$WORK" "$MODE" "$ARM" "$CAND" $SEEDS <<'PY'
import json, os, sys
root, work, mode, arm, cand = sys.argv[1:6]
seeds = [int(x) for x in sys.argv[6:]]
sys.path.insert(0, os.path.join(root, "scripts"))
import pairfeat_compare as pfc
if pfc.mode_settings(mode, cand)["seeds"] != seeds or arm not in pfc.arms(cand):
    sys.exit("seed mismatch")
lanes = []
for seed in seeds:
    lm = pfc._load_lane(os.path.join(work, "v2-s%d" % seed), seed, arm, mode, cand)  # floor 0.01, arm modelset, mode scope; wrong_pair: truth pk_qid; pairfeat5/r1review non-baseline arms: capped-pair report from link_funnel.ndjson; r1review experiments: completion_undocumented items
    if lm["analysis_errors"] or lm["trace_error"] or lm["replay_mismatch"]:
        sys.exit("seed %d: analysis/trace-write error or replay mismatch" % seed)
    lanes.append(lm)
out = pfc.aggregate_arm(arm, lanes)
out["build"] = pfc.read_build(work, arm, mode, cand)
out["scope"] = {"mode": mode, "arm": arm, "candidate": cand, "modelset": pfc.MODELSET[arm], "floor": float(pfc.FLOOR), "policy": pfc.POLICY,
                **({"extra_flags": pfc.extra_flags(arm)} if pfc.extra_flags(arm) else {}),
                "seeds": seeds, "reader": "cached", "image": pfc.lane_image_mode(cand), "chaos": 0, "nats_kill": False,
                **({"imgphotos": True} if pfc.lane_image(cand) else {}),
                "note": "simulated; tp = report7 programmes_live.crc.pred_closed - false_closures; link attach rate is separate"}
with open(os.path.join(work, "summary.json"), "w") as h:
    json.dump(out, h, indent=1, sort_keys=True)
print(json.dumps({"arm": arm, "candidate": cand, "mode": mode, "recall": out["recall"], "tp": out["tp"], "truth_closed": out["truth_closed"]}, sort_keys=True))
PY
