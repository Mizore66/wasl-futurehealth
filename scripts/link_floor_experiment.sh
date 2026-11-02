#!/usr/bin/env bash
# One protected CRC review-floor lane set (all seeds of a mode) for one floor arm. No paid model use.
# Usage: scripts/link_floor_experiment.sh NEW_ROOT MODE FLOOR
#   MODE  dev-clean (dev; 101 103 107; rerr 0/0) | dev-stress (dev; 101 103 107; rerr 0.04, ungraded 0.25)
#         | validation-clean (eval; 211 223 227 229 233; rerr 0/0)
#   FLOOR 0.2 | 0.05 | 0.01
# NEW_ROOT must be a new path under ~/wasl-ml-data (one root per arm; never reused or deleted).
# validation-clean refuses to start unless ../validation-manifest.json (written by the controller, never here) matches the
# current git HEAD, the analyzer SHA256 and the five validation seeds, and the floor is 0.2 (baseline) or the manifest's selected floor.
# dev-stress likewise needs ../selection.json (written by link_floor_compare.py dev-clean) with non-null selected_floor, and
# allows only floor 0.2 or exactly that selected floor.
# The tracked tree must be clean and pipeline/ and scripts/ must have no untracked files before any build.
# Cached reader, image off, chaos 0, subkill 1, no NATS kill.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export PATH="/opt/anaconda3/bin:$PATH"

[ "$#" -eq 3 ] && [ -n "$1" ] || { echo "usage: scripts/link_floor_experiment.sh NEW_ROOT MODE FLOOR" >&2; exit 2; }
REQ="$1"; MODE="$2"; FLOOR="$3"
case "$MODE" in
  dev-clean)        SPLIT=dev;  SEEDS="101 103 107";             RERR=0;    ERRU=0;;
  dev-stress)       SPLIT=dev;  SEEDS="101 103 107";             RERR=0.04; ERRU=0.25;;
  validation-clean) SPLIT=eval; SEEDS="211 223 227 229 233";     RERR=0;    ERRU=0;;
  *) echo "invalid mode: $MODE" >&2; exit 2;;
esac
case "$FLOOR" in 0.2|0.05|0.01) ;; *) echo "invalid floor: $FLOOR (allowed 0.2 0.05 0.01)" >&2; exit 2;; esac
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

if [ "$MODE" = dev-stress ]; then
  SEL="$PARENT/selection.json"
  [ -f "$SEL" ] && [ ! -L "$SEL" ] || { echo "selection.json missing in $PARENT; dev-stress is refused" >&2; exit 2; }
  python3 -I - "$SEL" "$FLOOR" <<'PY' || { echo "dev-stress floor must be 0.2 or the selected arm in selection.json" >&2; exit 2; }
import json, sys
with open(sys.argv[1]) as h:
    sel = json.load(h).get("selected_floor")
assert isinstance(sel, (int, float)) and not isinstance(sel, bool) and sel in (0.05, 0.01), "no selected floor"
assert float(sys.argv[2]) in (0.2, float(sel)), "floor not authorised"
PY
fi

if [ "$MODE" = validation-clean ]; then
  MAN="$PARENT/validation-manifest.json"
  [ -f "$MAN" ] && [ ! -L "$MAN" ] || { echo "validation-manifest.json missing in $PARENT; validation lanes are refused" >&2; exit 2; }
  HEAD="$(git -C "$ROOT" rev-parse HEAD)"
  python3 -I - "$MAN" "$HEAD" "$ROOT/scripts/link_floor_compare.py" "$FLOOR" "$SEEDS" <<'PY' || { echo "validation manifest does not match frozen inputs" >&2; exit 2; }
import hashlib, json, sys
man, head, analyzer, floor, seeds = sys.argv[1:6]
with open(man) as h:
    m = json.load(h)
with open(analyzer, "rb") as h:
    sha = hashlib.sha256(h.read()).hexdigest()
assert m.get("git_head") == head, "git head"
assert m.get("analyzer_sha256") == sha, "analyzer sha256"
assert m.get("seeds") == [int(x) for x in seeds.split()], "seeds"
sel = m.get("selected_floor")
assert isinstance(sel, (int, float)) and not isinstance(sel, bool) and sel in (0.05, 0.01), "selected floor"
assert float(floor) in (0.2, float(sel)), "floor is neither baseline nor selected"
PY
fi

DIRTY="$(git -C "$ROOT" status --porcelain --untracked-files=all -- pipeline scripts | grep -v '__pycache__' || true)"
TRACKED="$(git -C "$ROOT" diff HEAD --name-only)"
if [ -n "$DIRTY" ] || [ -n "$TRACKED" ]; then echo "working tree not clean (tracked changes or untracked files under pipeline/scripts); refusing to build" >&2; exit 2; fi

WORK="$PARENT/$(basename "$REQ")"
mkdir "$WORK"

"$ROOT/scripts/demo.sh" check
"$ROOT/scripts/demo.sh" build
trap 'rm -f "$ROOT/bin/wsi7"' EXIT

python3 -I - "$ROOT" "$WORK" "$MODE" "$SPLIT" "$FLOOR" "$RERR" "$ERRU" $SEEDS <<'PY'
import json, os, subprocess, sys
root, work, mode, split, floor, rerr, erru = sys.argv[1:8]
seeds = [int(x) for x in sys.argv[8:]]
ok = {"dev-clean": ("dev", [101, 103, 107], "0", "0"), "dev-stress": ("dev", [101, 103, 107], "0.04", "0.25"),
      "validation-clean": ("eval", [211, 223, 227, 229, 233], "0", "0")}
assert ok[mode] == (split, seeds, rerr, erru), "mode settings mismatch"
assert floor in ("0.2", "0.05", "0.01")
bin_ = os.path.join(root, "bin", "wsi7")

def guard(argv):
    def val(flag):
        v = [argv[i + 1] for i, a in enumerate(argv[:-1]) if a == flag]
        assert len(v) <= 1, flag + " repeated"
        return v[0] if v else None
    if argv[1] == "gen":
        assert val("-split") == split, "gen split"
        assert val("-noise") == "v2", "gen noise must be exactly v2"
        assert int(val("-seed")) in seeds, "gen seed outside the authorised set"
        assert val("-persons") is None, "gen must use the whole split"
        for f in ("-lung", "-dr", "-breast", "-ldct", "-ifhas", "-cerv"):
            assert val(f) == "0", f + " must be 0"
    else:
        assert "-split" not in argv and "-noise" not in argv
    if argv[1] in ("orchestrate", "audit", "report7"):
        assert val("-reader") == "cached" and val("-image") == "off", "reader must be cached, image off"
        assert val("-policy") == "v7strict" and val("-rerr") == rerr and val("-errungraded") == erru
        assert val("-linkfloor") == floor, "linkfloor"
    if argv[1] == "orchestrate":
        assert val("-chaos") == "0" and val("-subkill") == "1", "chaos 0 / subkill 1"
    for a in argv:
        assert "frozen" not in a and "b8" not in a.lower() and "heldout" not in a, "protected setting: " + a

def run(argv, log):
    guard(argv)
    with open(log, "wb") as h:
        p = subprocess.run(argv, stdout=h, stderr=subprocess.STDOUT)
    if p.returncode != 0:
        sys.stderr.write("FAILED (%d): %s\n  log: %s\n" % (p.returncode, " ".join(argv[:2]), log))
        sys.exit(p.returncode)

for seed in seeds:
    o = os.path.join(work, "v2-s%d" % seed)
    os.mkdir(o); os.mkdir(os.path.join(o, "run"))
    r = os.path.join(o, "run", "main")
    rd = ["-reader", "cached", "-image", "off"]
    lf = ["-linkfloor", floor]
    run([bin_, "gen", "-wasl", root + "/data", "-reports", root + "/data/evidence/reports43.json",
         "-pathology", root + "/data/evidence/pathology_templates.json",
         "-ecpool", root + "/patient-parsers/econsult_testset.json", "-tbpool", root + "/patient-parsers/teachback_testset.json",
         "-out", o + "/base", "-split", split, "-seed", str(seed), "-loops", "600", "-distract", "200", "-cerv", "0",
         "-noise", "v2", "-lung", "0", "-dr", "0", "-breast", "0", "-ldct", "0", "-ifhas", "0"], o + "/gen.log")
    run([bin_, "sim", "-data", o + "/base", "-wasl", root + "/models", "-run", o + "/run/base"], o + "/sim.log")
    run(["python3", "-I", root + "/pipeline/augment_cohort.py", o + "/base", o + "/run/base", o + "/cohort"], o + "/augment.log")
    run([bin_, "orchestrate", "-data", o + "/cohort", "-wasl", root + "/models", "-run", r, "-preps", "3", "-threads", "8",
         "-source", "fhir", "-subkill", "1", "-chaos", "0", "-ann", "hnsw", "-rerr", rerr, "-errungraded", erru,
         "-rdouble", "merge+close", "-dual", "-rseed", str(seed), "-seed", str(seed), "-policy", "v7strict",
         "-linkfunnel", r + "/link_funnel.ndjson"] + lf + rd, o + "/orchestrate.log")
    tail = ["-data", o + "/cohort", "-wasl", root + "/models", "-run", r, "-ann", "hnsw", "-dual", "-policy", "v7strict",
            "-rerr", rerr, "-errungraded", erru] + lf + rd
    run([bin_, "audit"] + tail, o + "/audit.log")
    run([bin_, "report7"] + tail, o + "/report7.log")
    with open(os.path.join(r, "status-linker.json")) as h:
        st = json.load(h)  # required: a missing file aborts the lane
    assert not st.get("link_funnel_error"), "link funnel trace write failed"
    run(["python3", "-I", root + "/scripts/link_funnel.py", r, o + "/cohort", o + "/funnel.json", split, rerr, erru], o + "/classifier.log")
PY

python3 -I - "$ROOT" "$WORK" "$MODE" "$FLOOR" $SEEDS <<'PY'
import json, os, sys
root, work, mode, floor = sys.argv[1:5]
seeds = [int(x) for x in sys.argv[5:]]
sys.path.insert(0, os.path.join(root, "scripts"))
import link_floor_compare as lfc
assert lfc.MODES[mode]["seeds"] == seeds
lanes = []
for seed in seeds:
    m = lfc._load_lane(os.path.join(work, "v2-s%d" % seed), seed, floor)  # also requires funnel config.review_floor == floor
    assert m["analysis_errors"] == 0 and m["trace_error"] == 0, (seed, "analysis or trace-write error")
    assert m["replay_mismatch"] == 0, (seed, "replay mismatch")
    lanes.append(m)
out = lfc.aggregate_arm(float(floor), lanes)
out["scope"] = {"mode": mode, "floor": float(floor), "seeds": seeds, "reader": "cached", "image": "off", "chaos": 0, "nats_kill": False,
                "note": "tp is report7 programmes_live.crc.tp; link attach rate is separate"}
with open(os.path.join(work, "summary.json"), "w") as h:
    json.dump(out, h, indent=1, sort_keys=True)
print(json.dumps({"floor": floor, "recall": out["recall"], "tp": out["tp"], "truth_closed": out["truth_closed"]}, sort_keys=True))
PY
