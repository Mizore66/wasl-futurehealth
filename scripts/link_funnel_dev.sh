#!/usr/bin/env bash
# CRC link-funnel development baseline: three dev-split seeds, v2 noise, v7strict, cached reader, no paid model use.
# Usage: scripts/link_funnel_dev.sh NEW_EMPTY_WORK_DIR   (must be a new path under ~/wasl-ml-data; never reused or deleted)
# Diagnostic assumptions (not canonical stress settings): -chaos 0, no NATS kill, reviewer error 0, ungraded-prep reviewer error 0.
# Hard gates: CRC false closures, wrong-patient releases (total and per origin), live-vs-replay mismatches, incomplete funnel,
# funnel/CRC truth_closed agreement. False-merge pairs, impure clusters and cross-person edges are reported descriptively,
# not asserted zero; any increase belongs in a later paired remediation run.
# link_attach_rate is a linkage-stage metric; it is NOT CRC closure recall (that is report7 programmes_live.crc.recall).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export PATH="/opt/anaconda3/bin:$PATH"
SEEDS=(101 103 107)

[ "$#" -eq 1 ] && [ -n "$1" ] || { echo "usage: scripts/link_funnel_dev.sh NEW_EMPTY_WORK_DIR" >&2; exit 2; }
[ -n "${HOME:-}" ] || { echo "HOME is not set" >&2; exit 2; }
DATA_BASE="$HOME/wasl-ml-data"
[ -d "$DATA_BASE" ] || { echo "missing $DATA_BASE" >&2; exit 2; }
DATA_BASE="$(cd "$DATA_BASE" && pwd -P)"
REQ="$1"
case "$REQ" in "$HOME/wasl-ml-data"/*) ;; *) echo "work dir must be under ~/wasl-ml-data" >&2; exit 2;; esac
case "$REQ/" in */../*|*/./*|*//*) echo "work dir must not contain ., .. or empty segments" >&2; exit 2;; esac
[ ! -e "$REQ" ] && [ ! -L "$REQ" ] || { echo "work dir already exists; a new output root is required: $REQ" >&2; exit 2; }
PARENT="$(dirname "$REQ")"
mkdir -p "$PARENT"
PARENT="$(cd "$PARENT" && pwd -P)"
case "$PARENT/" in "$DATA_BASE"/*) ;; *) echo "work dir resolves outside ~/wasl-ml-data" >&2; exit 2;; esac
WORK="$PARENT/$(basename "$REQ")"
mkdir "$WORK"

"$ROOT/scripts/demo.sh" check
"$ROOT/scripts/demo.sh" build
trap 'rm -f "$ROOT/bin/wsi7"' EXIT

python3 -I - "$ROOT" "$WORK" "${SEEDS[@]}" <<'PY'
import json, os, subprocess, sys
root, work = sys.argv[1], sys.argv[2]
seeds = [int(x) for x in sys.argv[3:]]
assert seeds == [101, 103, 107], "seeds must be exactly 101,103,107"
bin_ = os.path.join(root, "bin", "wsi7")

def guard(argv):
    """Refuse any protected setting before a process starts."""
    def val(flag):
        v = [argv[i + 1] for i, a in enumerate(argv[:-1]) if a == flag]
        assert len(v) <= 1, flag + " repeated"
        return v[0] if v else None
    if argv[1] == "gen":
        assert val("-split") == "dev", "gen split must be exactly dev"
        assert val("-noise") == "v2", "gen noise must be exactly v2"
        assert int(val("-seed")) in seeds, "gen seed outside the authorised set"
        assert val("-persons") is None, "gen must use the whole dev split"
        for f in ("-lung", "-dr", "-breast", "-ldct", "-ifhas", "-cerv"):
            assert val(f) == "0", f + " must be 0"
    else:
        assert "-split" not in argv and "-noise" not in argv
    if argv[1] in ("orchestrate", "audit", "report7"):
        assert val("-reader") == "cached" and val("-image") == "off", "reader must be cached, image off"
        assert val("-policy") == "v7strict" and val("-rerr") == "0" and val("-errungraded") == "0"
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
    os.mkdir(o)
    os.mkdir(os.path.join(o, "run"))
    r = os.path.join(o, "run", "main")
    rd = ["-reader", "cached", "-image", "off"]
    run([bin_, "gen", "-wasl", root + "/data", "-reports", root + "/data/evidence/reports43.json",
         "-pathology", root + "/data/evidence/pathology_templates.json",
         "-ecpool", root + "/patient-parsers/econsult_testset.json", "-tbpool", root + "/patient-parsers/teachback_testset.json",
         "-out", o + "/base", "-split", "dev", "-seed", str(seed), "-loops", "600", "-distract", "200", "-cerv", "0",
         "-noise", "v2", "-lung", "0", "-dr", "0", "-breast", "0", "-ldct", "0", "-ifhas", "0"], o + "/gen.log")
    run([bin_, "sim", "-data", o + "/base", "-wasl", root + "/models", "-run", o + "/run/base"], o + "/sim.log")
    run(["python3", "-I", root + "/pipeline/augment_cohort.py", o + "/base", o + "/run/base", o + "/cohort"], o + "/augment.log")
    run([bin_, "orchestrate", "-data", o + "/cohort", "-wasl", root + "/models", "-run", r, "-preps", "3", "-threads", "8",
         "-source", "fhir", "-subkill", "1", "-chaos", "0", "-ann", "hnsw", "-rerr", "0", "-errungraded", "0",
         "-rdouble", "merge+close", "-dual", "-rseed", str(seed), "-seed", str(seed), "-policy", "v7strict",
         "-linkfunnel", r + "/link_funnel.ndjson"] + rd, o + "/orchestrate.log")
    tail = ["-data", o + "/cohort", "-wasl", root + "/models", "-run", r, "-ann", "hnsw", "-dual", "-policy", "v7strict",
            "-rerr", "0", "-errungraded", "0"] + rd
    run([bin_, "audit"] + tail, o + "/audit.log")
    run([bin_, "report7"] + tail, o + "/report7.log")
    run(["python3", "-I", root + "/scripts/link_funnel.py", r, o + "/cohort", o + "/funnel.json"], o + "/classifier.log")
PY

python3 -I - "$WORK" "${SEEDS[@]}" <<'PY'
import collections, json, os, sys
work, seeds = sys.argv[1], [int(x) for x in sys.argv[2:]]
assert seeds == [101, 103, 107]
ALLOWED = {"candidate_not_generated", "candidate_below_review", "candidate_rejected_demographic",
           "candidate_routed_review", "linked_report_not_attached", "attached_or_closed"}
out = {"scope": {"person_split": "dev", "dev_people_expected": 5284, "noise": "v2", "seeds": seeds, "policy": "v7strict",
                 "reader": "cached", "image": "off", "chaos": 0, "nats_kill": False, "reviewer_error": 0,
                 "day15_ungraded_prep_reviewer_error": 0, "diagnostic_only": True,
                 "note": "link_attach_rate is a linkage metric, not CRC closure recall"},
       "per_seed": {}, "pooled_categories": collections.Counter(), "pooled": {}}
tot = collections.Counter()
for seed in seeds:
    o = os.path.join(work, "v2-s%d" % seed)
    f = json.load(open(o + "/funnel.json"))
    r = json.load(open(o + "/run/main/report7.json"))
    crc = r["programmes_live"]["crc"]
    org = r["outreach_live"]["by_origin"]
    assert f["analysis_errors"] == 0, (seed, "incomplete funnel")
    assert set(f["categories"]) <= ALLOWED, (seed, "unexpected category")
    assert sum(f["categories"].values()) + f["analysis_errors"] == f["truth_closed"], (seed, "funnel does not reconcile")
    assert f["missed"] == f["truth_closed"] - f["categories"].get("attached_or_closed", 0), (seed, "missed arithmetic")
    assert f["truth_closed"] == crc["truth_closed"], (seed, "truth_closed mismatch")
    assert crc["false_closures"] == 0, (seed, "CRC false closure")
    assert r["outreach_live"]["wrong_patient_released"] == 0, (seed, "wrong-patient release")
    assert all(x["wrong_released"] == 0 for x in org.values()), (seed, "wrong-patient release by origin")
    assert r["live_vs_replay_final_mismatch"] == 0, (seed, "replay mismatch")
    out["pooled_categories"].update(f["categories"])
    tot["truth_closed"] += f["truth_closed"]; tot["missed"] += f["missed"]
    out["per_seed"][str(seed)] = {"funnel": f, "crc": crc, "false_closures": crc["false_closures"],
        "wrong_patient_released": r["outreach_live"]["wrong_patient_released"],
        "wrong_patient_released_by_origin": {k: v["wrong_released"] for k, v in sorted(org.items())},
        "linkage_contamination": r["linkage"], "reviewer_burden": r["reviewer"],
        "live_vs_replay_final_mismatch": r["live_vs_replay_final_mismatch"], "lane_complete": True}
out["pooled_categories"] = dict(sorted(out["pooled_categories"].items()))
out["pooled"] = {"truth_closed": tot["truth_closed"], "missed": tot["missed"],
                 "link_attach_rate": round((tot["truth_closed"] - tot["missed"]) / tot["truth_closed"], 6)}
with open(os.path.join(work, "summary.json"), "w") as h:
    json.dump(out, h, indent=1, sort_keys=True)
print(json.dumps(out["pooled_categories"], sort_keys=True))
PY
