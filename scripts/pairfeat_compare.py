"""Protected analyzer for the paired CRC pair-scorer experiment: baseline (default modelset) vs pairfeat1, both -linkfloor 0.01.

Usage
  pairfeat_compare.py MODE OUT_JSON baseline=ROOT pairfeat1=ROOT
      MODE dev-clean | dev-stress | validation-clean. OUT_JSON must sit in the experiment parent directory (the directory
      holding the arm roots and the manifests below). dev-clean also writes selection.json, dev-stress also writes stress.json
      (both write-once, next to OUT_JSON).
  pairfeat_compare.py freeze-validation EXPERIMENT_DIR
      Writes EXPERIMENT_DIR/validation-manifest.json (write-once) only when selection.json selected pairfeat1 and stress.json
      passed, all at the current git HEAD with unchanged model files.
  pairfeat_compare.py preflight MODE ARM EXPERIMENT_DIR
      Used by scripts/pairfeat_experiment.sh before any build; prints the arm's verified model-chain hashes as JSON.

Experiment parent layout (EXPERIMENT_DIR)
  artifact-manifest.json   training artifacts (written by encoder/fit_pairfeat1.py): git_head, baseline_chain_sha256,
                           pairfeat1_chain_sha256 (other keys ignored)
  selection.json           dev-clean result: selected_modelset "pairfeat1" only on a passing gate, else null
  stress.json              dev-stress result: pass true/false
  validation-manifest.json frozen by freeze-validation; validation-clean lanes and analysis need it plus the unchanged
                           artifact-manifest/selection/stress files whose SHA256 it records (still passing at this HEAD);
                           the validation OUT_JSON is write-once
  <arm roots>/build.json   per-arm identity written by the runner; <arm roots>/v2-s<seed>/ lanes

Gates (fixed before any result). Gains are pooled CRC closure-recall percentage points, candidate minus baseline:
  selection >= +2.0, validation >= +1.0, stress >= 0. Every gate also requires: equal truth_closed denominators; zero
  false closures, wrong-patient releases (total and by origin), replay mismatches, analysis errors and trace-write errors
  in BOTH arms; no increase in false-merge pairs, impure clusters or summed cross-person edges; candidate distinct
  link-review pairs <= 1.10 x baseline (baseline 0 requires candidate 0).
CRC tp = programmes_live.crc.pred_closed - false_closures (funnel attached_or_closed is a link attach rate, not recall).
Residual categories and reviewer wrong_final_signoffs are reported outcomes, not gate substitutions.
Small-n caveat: t intervals over 3 (dev) or 5 (validation) seeds are wide and descriptive only.
"""
import hashlib
import json
import math
import os
import statistics
import subprocess
import sys
from fractions import Fraction

MODES = {
    "dev-clean": {"split": "dev", "seeds": [307, 311, 313], "rerr": "0", "errungraded": "0"},
    "dev-stress": {"split": "dev", "seeds": [307, 311, 313], "rerr": "0.04", "errungraded": "0.25"},
    "validation-clean": {"split": "eval", "seeds": [401, 409, 419, 421, 431], "rerr": "0", "errungraded": "0"},
}
FLOOR = "0.01"
MIN_GAIN_PP = {"selection": 2.0, "validation": 1.0}
MAX_REVIEW_GROWTH = 0.10
ARMS = ("baseline", "pairfeat1")

POLICY = "v7strict"
CANDIDATE = "pairfeat1"
MODELSET = {"baseline": "", "pairfeat1": "pairfeat1"}
CHAIN_FILES = {
    "baseline": ("encoder_weights.json", "lgb_stream.json", "iso.json", "iso_v7.json"),
    "pairfeat1": ("encoder_pairfeat1.json", "lgb_stream_pairfeat1.json", "iso_pairfeat1.json", "iso_v7_pairfeat1.json"),
}
CHAIN_KEY = {"baseline": "baseline_chain_sha256", "pairfeat1": "pairfeat1_chain_sha256"}
GATE_KINDS = ("selection", "validation", "stress")
T_CRIT = {3: 4.303, 5: 2.776}  # two-sided 95%, df = n-1
SAFETY = ("false_closures", "wrong_released_total", "replay_mismatch", "analysis_errors", "trace_error")
CONTAM = ("false_merge_pairs", "impure_clusters", "cross_person_edges")
FUNNEL_CATEGORIES = ("candidate_not_generated", "candidate_below_review", "candidate_rejected_demographic",
                     "candidate_routed_review", "linked_report_not_attached", "attached_or_closed")
RESIDUAL = ("candidate_not_generated", "candidate_below_review", "candidate_rejected_demographic",
            "candidate_routed_review", "linked_report_not_attached", "attached_not_closed")
REVIEWER_SUMS = ("items", "hours", "distinct_pairs", "unanswered", "link_items", "wrong_final_signoffs")

ARTIFACT_MANIFEST = "artifact-manifest.json"
SELECTION = "selection.json"
STRESS = "stress.json"
VALIDATION_MANIFEST = "validation-manifest.json"
RESERVED = (ARTIFACT_MANIFEST, SELECTION, STRESS, VALIDATION_MANIFEST)

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.dirname(HERE)
RUNNER_REL = os.path.join("scripts", "pairfeat_experiment.sh")
ANALYZER_REL = os.path.join("scripts", "pairfeat_compare.py")

# Lane settings shared by every arm and mode (the runner builds commands only through lane_commands).
LANE = {"reader": "cached", "image": "off", "source": "fhir", "chaos": "0", "subkill": "1", "ann": "hnsw", "noise": "v2",
        "loops": "600", "distract": "200", "rdouble": "merge+close", "preps": "3", "threads": "8"}
WSI_CMDS = ("gen", "sim", "orchestrate", "audit", "report7")
MODEL_CMDS = ("orchestrate", "audit", "report7")  # the only commands that may (candidate) or must not (baseline) carry -modelset
PY_SCRIPTS = {"augment_cohort.py": "pipeline", "link_funnel.py": "scripts"}
GEN_ZERO = ("-lung", "-dr", "-breast", "-ldct", "-ifhas", "-cerv")


class PreflightError(ValueError):
    pass


class GuardError(ValueError):
    pass


def expected_settings(mode):
    m = MODES[mode]
    s = {"mode": mode, "split": m["split"], "seeds": list(m["seeds"]), "rerr": m["rerr"], "errungraded": m["errungraded"],
         "linkfloor": FLOOR, "policy": POLICY, "nats_kill": False, "cohort": "crc-only", "dual": True,
         "candidate_modelset": CANDIDATE, "candidate_modelset_commands": list(MODEL_CMDS), "arms": list(ARMS),
         "arms_sequential": True}
    s.update(LANE)
    return s


# ---------------------------------------------------------------- identity helpers

def _is_hex(v, n):
    return isinstance(v, str) and len(v) == n and all(c in "0123456789abcdef" for c in v)


def sha256_file(path):
    with open(path, "rb") as h:
        return hashlib.sha256(h.read()).hexdigest()


def git_head(repo):
    p = subprocess.run(["git", "-C", repo, "rev-parse", "HEAD"], capture_output=True, text=True)
    head = p.stdout.strip()
    if p.returncode != 0 or not _is_hex(head, 40):
        raise PreflightError("cannot read git HEAD")
    return head


def _read_json(path, what=None):
    what = what or os.path.basename(path)
    if os.path.islink(path) or not os.path.isfile(path):
        raise PreflightError("%s missing (or not a regular file): %s" % (what, path))
    with open(path) as h:
        return json.load(h)


def _write_once(path, obj):
    with open(path, "x") as h:  # FileExistsError if already frozen: decisions are never overwritten
        json.dump(obj, h, indent=1, sort_keys=True)
        h.write("\n")


def chain_hashes(repo, arm):
    out = {}
    for f in CHAIN_FILES[arm]:
        p = os.path.join(repo, "models", f)
        if os.path.islink(p) or not os.path.isfile(p):
            raise PreflightError("model file missing: models/" + f)
        out[f] = sha256_file(p)
    return out


def _check_chain_map(m, arm, what):
    if not isinstance(m, dict) or set(m) != set(CHAIN_FILES[arm]) or not all(_is_hex(v, 64) for v in m.values()):
        raise PreflightError("%s %s must map exactly %s to SHA256 values" % (what, CHAIN_KEY[arm], list(CHAIN_FILES[arm])))


def _check_maps(doc, what):
    for a in ARMS:
        _check_chain_map(doc.get(CHAIN_KEY[a]), a, what)


def _check_files_match(repo, arm, frozen, what):
    cur = chain_hashes(repo, arm)
    for f in CHAIN_FILES[arm]:
        if cur[f] != frozen[f]:
            raise PreflightError("models/%s SHA256 does not match %s" % (f, what))
    return cur


def check_artifact_manifest(am, head):
    if not isinstance(am, dict) or not _is_hex(am.get("git_head"), 40):
        raise PreflightError("artifact-manifest.json needs a full git_head")
    if am["git_head"] != head:
        raise PreflightError("artifact-manifest.json git_head does not match current HEAD")
    _check_maps(am, "artifact-manifest.json")


def check_selection(sel, am, head):
    if not isinstance(sel, dict):
        raise PreflightError("selection.json malformed")
    if sel.get("selected_modelset") != CANDIDATE or sel.get("pass") is not True:
        raise PreflightError("selection.json did not select pairfeat1")
    if sel.get("mode") != "dev-clean" or sel.get("seeds") != MODES["dev-clean"]["seeds"]:
        raise PreflightError("selection.json mode/seeds are not the dev-clean selection")
    if sel.get("git_head") != head:
        raise PreflightError("selection.json git_head does not match current HEAD")
    for a in ARMS:
        if sel.get(CHAIN_KEY[a]) != am[CHAIN_KEY[a]]:
            raise PreflightError("selection.json %s differs from artifact-manifest.json" % CHAIN_KEY[a])


def check_stress(st, am, head, selection_sha):
    if not isinstance(st, dict) or st.get("pass") is not True:
        raise PreflightError("stress.json did not pass")
    if st.get("mode") != "dev-stress" or st.get("seeds") != MODES["dev-stress"]["seeds"]:
        raise PreflightError("stress.json mode/seeds are not the dev-stress lanes")
    if st.get("git_head") != head:
        raise PreflightError("stress.json git_head does not match current HEAD")
    if st.get("selection_sha256") != selection_sha:
        raise PreflightError("stress.json was not computed against this selection.json")
    for a in ARMS:
        if st.get(CHAIN_KEY[a]) != am[CHAIN_KEY[a]]:
            raise PreflightError("stress.json %s differs from artifact-manifest.json" % CHAIN_KEY[a])


def check_validation_manifest(vm, repo, head):
    if not isinstance(vm, dict) or not _is_hex(vm.get("git_head"), 40):
        raise PreflightError("validation-manifest.json needs a full git_head")
    if vm["git_head"] != head:
        raise PreflightError("validation-manifest.json git_head does not match current HEAD")
    if vm.get("mode") != "validation-clean" or vm.get("seeds") != MODES["validation-clean"]["seeds"]:
        raise PreflightError("validation-manifest.json seeds/mode are not the frozen validation seeds")
    if vm.get("selected_modelset") != CANDIDATE:
        raise PreflightError("validation-manifest.json selected_modelset is not pairfeat1")
    if vm.get("settings") != expected_settings("validation-clean"):
        raise PreflightError("validation-manifest.json settings differ from the protected validation settings")
    if vm.get("runner_sha256") != sha256_file(os.path.join(repo, RUNNER_REL)):
        raise PreflightError("validation-manifest.json runner_sha256 does not match scripts/pairfeat_experiment.sh")
    if vm.get("analyzer_sha256") != sha256_file(os.path.join(repo, ANALYZER_REL)):
        raise PreflightError("validation-manifest.json analyzer_sha256 does not match scripts/pairfeat_compare.py")
    _check_maps(vm, "validation-manifest.json")


def check_frozen_validation(vm, exp_dir, repo, head):
    """A validation-manifest.json authorises validation only if it is what freeze-validation would write now: its own
    fields match HEAD, seeds, settings and the runner/analyzer SHA256, and the recorded artifact-manifest, selection and
    stress SHA256 still match those files, which must still be a passing selection and stress at this HEAD."""
    check_validation_manifest(vm, repo, head)
    ap, sp, tp = (os.path.join(exp_dir, n) for n in (ARTIFACT_MANIFEST, SELECTION, STRESS))
    for key, path in (("artifact_manifest_sha256", ap), ("selection_sha256", sp), ("stress_sha256", tp)):
        _read_json(path)  # regular file required
        if not _is_hex(vm.get(key), 64) or vm[key] != sha256_file(path):
            raise PreflightError("validation-manifest.json %s does not match %s" % (key, os.path.basename(path)))
    am = _read_json(ap)
    check_artifact_manifest(am, head)
    for a in ARMS:
        if vm[CHAIN_KEY[a]] != am[CHAIN_KEY[a]]:
            raise PreflightError("validation-manifest.json %s differs from artifact-manifest.json" % CHAIN_KEY[a])
    check_selection(_read_json(sp), am, head)
    check_stress(_read_json(tp), am, head, sha256_file(sp))


def preflight(mode, arm, exp_dir, repo, head):
    """All refusals that must happen before any build. Returns the arm's verified model-chain hashes."""
    if mode not in MODES:
        raise PreflightError("invalid mode: %r" % (mode,))
    if arm not in ARMS:
        raise PreflightError("invalid arm: %r" % (arm,))
    if mode == "validation-clean":  # authorised only by a frozen manifest whose upstream files still verify
        vm = _read_json(os.path.join(exp_dir, VALIDATION_MANIFEST))
        check_frozen_validation(vm, exp_dir, repo, head)
        return _check_files_match(repo, arm, vm[CHAIN_KEY[arm]], "validation-manifest.json")
    am = _read_json(os.path.join(exp_dir, ARTIFACT_MANIFEST))
    check_artifact_manifest(am, head)
    if mode == "dev-stress":
        check_selection(_read_json(os.path.join(exp_dir, SELECTION)), am, head)
    return _check_files_match(repo, arm, am[CHAIN_KEY[arm]], "artifact-manifest.json")


def freeze_validation(exp_dir, repo=REPO, head=None):
    """Freeze validation-manifest.json after a passing selection and a passing stress result (write-once)."""
    head = head or git_head(repo)
    vp = os.path.join(exp_dir, VALIDATION_MANIFEST)
    if os.path.lexists(vp):
        raise PreflightError("validation-manifest.json already exists; it is frozen and never rewritten")
    ap, sp, tp = (os.path.join(exp_dir, n) for n in (ARTIFACT_MANIFEST, SELECTION, STRESS))
    am = _read_json(ap)
    check_artifact_manifest(am, head)
    check_selection(_read_json(sp), am, head)
    check_stress(_read_json(tp), am, head, sha256_file(sp))
    for a in ARMS:
        _check_files_match(repo, a, am[CHAIN_KEY[a]], "artifact-manifest.json")
    vm = {"git_head": head, "mode": "validation-clean", "seeds": list(MODES["validation-clean"]["seeds"]),
          "selected_modelset": CANDIDATE,
          "baseline_chain_sha256": dict(am["baseline_chain_sha256"]), "pairfeat1_chain_sha256": dict(am["pairfeat1_chain_sha256"]),
          "runner_sha256": sha256_file(os.path.join(repo, RUNNER_REL)),
          "analyzer_sha256": sha256_file(os.path.join(repo, ANALYZER_REL)),
          "settings": expected_settings("validation-clean"),
          "artifact_manifest_sha256": sha256_file(ap), "selection_sha256": sha256_file(sp), "stress_sha256": sha256_file(tp),
          "note": "frozen before any validation lane; eval/v2 seeds are not inspected before this file exists"}
    _write_once(vp, vm)
    return vm


# ---------------------------------------------------------------- lane commands and guard

def lane_commands(root, bin_, o, seed, mode, arm):
    """(name, argv, log) for one lane. gen/sim/augment are identical in both arms (identical cohort and truth denominator);
    only the candidate's orchestrate/audit/report7 carry -modelset pairfeat1. Both arms use -linkfloor 0.01 and v7strict."""
    m = MODES[mode]
    split, rerr, erru = m["split"], m["rerr"], m["errungraded"]
    r = o + "/run/main"
    rd = ["-reader", LANE["reader"], "-image", LANE["image"]]
    lf = ["-linkfloor", FLOOR]
    ms = ["-modelset", CANDIDATE] if arm == CANDIDATE else []
    gen = [bin_, "gen", "-wasl", root + "/data", "-reports", root + "/data/evidence/reports43.json",
           "-pathology", root + "/data/evidence/pathology_templates.json",
           "-ecpool", root + "/patient-parsers/econsult_testset.json", "-tbpool", root + "/patient-parsers/teachback_testset.json",
           "-out", o + "/base", "-split", split, "-seed", str(seed), "-loops", LANE["loops"], "-distract", LANE["distract"],
           "-cerv", "0", "-noise", LANE["noise"], "-lung", "0", "-dr", "0", "-breast", "0", "-ldct", "0", "-ifhas", "0"]
    sim = [bin_, "sim", "-data", o + "/base", "-wasl", root + "/models", "-run", o + "/run/base"]
    aug = ["python3", "-I", root + "/pipeline/augment_cohort.py", o + "/base", o + "/run/base", o + "/cohort"]
    orch = [bin_, "orchestrate", "-data", o + "/cohort", "-wasl", root + "/models", "-run", r, "-preps", LANE["preps"],
            "-threads", LANE["threads"], "-source", LANE["source"], "-subkill", LANE["subkill"], "-chaos", LANE["chaos"],
            "-ann", LANE["ann"], "-rerr", rerr, "-errungraded", erru, "-rdouble", LANE["rdouble"], "-dual",
            "-rseed", str(seed), "-seed", str(seed), "-policy", POLICY, "-linkfunnel", r + "/link_funnel.ndjson"] + ms + lf + rd
    tail = ["-data", o + "/cohort", "-wasl", root + "/models", "-run", r, "-ann", LANE["ann"], "-dual", "-policy", POLICY,
            "-rerr", rerr, "-errungraded", erru] + ms + lf + rd
    cls = ["python3", "-I", root + "/scripts/link_funnel.py", r, o + "/cohort", o + "/funnel.json", split, rerr, erru]
    return [("gen", gen, "gen.log"), ("sim", sim, "sim.log"), ("augment", aug, "augment.log"),
            ("orchestrate", orch, "orchestrate.log"), ("audit", [bin_, "audit"] + tail, "audit.log"),
            ("report7", [bin_, "report7"] + tail, "report7.log"), ("classifier", cls, "classifier.log")]


def guard(argv, mode, arm, bin_):
    """Refuse any subprocess argument outside the protected settings for this mode and arm."""
    def bad(msg):
        raise GuardError(msg + ": " + " ".join(str(a) for a in argv[:2]))
    if mode not in MODES:
        raise GuardError("invalid mode")
    if arm not in ARMS:
        raise GuardError("invalid arm")
    if not isinstance(argv, list) or len(argv) < 3 or not all(isinstance(a, str) for a in argv):
        raise GuardError("malformed argv")
    m = MODES[mode]
    for a in argv:
        low = a.lower()
        if "frozen" in low or "b8" in low or "heldout" in low:
            bad("protected setting " + a)
        if "v7ltt" in low:
            bad("v7ltt is not permitted (pairfeat1 supports v7strict only)")
        if a.startswith("--") or (a.startswith("-") and "=" in a):
            bad("only single-dash separate-value flags are allowed: " + a)

    def val(flag):
        idx = [i for i, a in enumerate(argv) if a == flag]
        if len(idx) > 1:
            bad(flag + " repeated")
        if idx and idx[0] == len(argv) - 1:
            bad(flag + " without value")
        return argv[idx[0] + 1] if idx else None

    if argv[0] == bin_:
        cmd = argv[1]
        if cmd not in WSI_CMDS:
            bad("unauthorised wsi7 command")
    elif argv[0] == "python3" and argv[1] == "-I" and os.path.basename(argv[2]) in PY_SCRIPTS \
            and os.path.basename(os.path.dirname(argv[2])) == PY_SCRIPTS[os.path.basename(argv[2])]:
        cmd = os.path.basename(argv[2])
    else:
        bad("unauthorised command")

    ms = val("-modelset")
    if cmd in MODEL_CMDS:
        want = MODELSET[arm] or None
        if ms != want:
            bad("-modelset must be %r for arm %s" % (want, arm))
        if val("-reader") != LANE["reader"] or val("-image") != LANE["image"]:
            bad("reader must be cached, image off")
        if val("-policy") != POLICY:
            bad("policy must be v7strict")
        if val("-rerr") != m["rerr"] or val("-errungraded") != m["errungraded"]:
            bad("reviewer error settings")
        if val("-linkfloor") != FLOOR:
            bad("linkfloor must be exactly 0.01")
        if val("-ann") != LANE["ann"]:
            bad("ann must be hnsw")
    else:
        for f in ("-modelset", "-policy", "-linkfloor", "-rerr", "-errungraded", "-reader", "-image"):
            if f in argv:
                bad(f + " is only allowed on orchestrate/audit/report7")
    if cmd == "gen":
        if val("-split") != m["split"]:
            bad("gen split")
        if val("-noise") != LANE["noise"]:
            bad("gen noise must be exactly v2")
        s = val("-seed")
        if s is None or not s.isdigit() or int(s) not in m["seeds"]:
            bad("gen seed outside the authorised set")
        if val("-persons") is not None:
            bad("gen must use the whole split")
        if val("-loops") != LANE["loops"] or val("-distract") != LANE["distract"]:
            bad("gen loops/distract")
        for f in GEN_ZERO:
            if val(f) != "0":
                bad(f + " must be 0")
    else:
        if "-split" in argv or "-noise" in argv or "-persons" in argv:
            bad("-split/-noise/-persons only on gen")
    if cmd == "orchestrate":
        if val("-chaos") != LANE["chaos"] or val("-subkill") != LANE["subkill"]:
            bad("chaos 0 / subkill 1")
        if val("-source") != LANE["source"] or val("-rdouble") != LANE["rdouble"]:
            bad("source fhir / rdouble merge+close")
        s, rs = val("-seed"), val("-rseed")
        if s is None or s != rs or not s.isdigit() or int(s) not in m["seeds"]:
            bad("orchestrate seed/rseed")
    if cmd == "link_funnel.py" and argv[-3:] != [m["split"], m["rerr"], m["errungraded"]]:
        bad("classifier scope")


# ---------------------------------------------------------------- lane metrics

def t_interval(values):
    n = len(values)
    if n not in T_CRIT:
        raise ValueError("t critical value defined only for n=3 or n=5")
    return statistics.mean(values), T_CRIT[n] * statistics.stdev(values) / math.sqrt(n)


def _ival(values):
    if len(values) not in T_CRIT:
        return None
    m, h = t_interval(values)
    return {"mean_pp": m, "half_width_pp": h, "low_pp": m - h, "high_pp": m + h, "n": len(values)}


def _req(d, *path):
    for k in path:
        d = d[k]  # KeyError on absence is intended: never substitute zero
    return d


def check_funnel_floor(funnel, floor=FLOOR):
    cfg = funnel.get("config")
    if not isinstance(cfg, dict) or "review_floor" not in cfg:
        raise ValueError("funnel config.review_floor missing")
    v = cfg["review_floor"]
    if isinstance(v, bool) or not isinstance(v, (int, float)) or v != float(floor):
        raise ValueError("funnel review_floor %r != %s" % (v, floor))


def check_funnel_modelset(funnel, arm):
    cfg = funnel.get("config")
    if not isinstance(cfg, dict) or "modelset" not in cfg:
        raise ValueError("funnel config.modelset missing")
    if not isinstance(cfg["modelset"], str) or cfg["modelset"] != MODELSET[arm]:
        raise ValueError("funnel modelset %r does not match arm %s" % (cfg["modelset"], arm))


def check_funnel_scope(funnel, mode):
    m = MODES[mode]
    sc = funnel.get("scope")
    want = {"person_split": m["split"], "reviewer_error": float(m["rerr"]), "day15_ungraded_prep_reviewer_error": float(m["errungraded"])}
    if not isinstance(sc, dict) or any(k not in sc or sc[k] != v for k, v in want.items()):
        raise ValueError("funnel scope %r does not match mode %s" % (sc, mode))


def read_build(root, arm, mode):
    with open(os.path.join(root, "build.json")) as h:
        b = json.load(h)
    if not _is_hex(b.get("git_head"), 40) or not _is_hex(b.get("binary_sha256"), 64):
        raise ValueError("build.json needs full git_head and binary_sha256")
    if b.get("arm") != arm or b.get("modelset") != MODELSET[arm] or b.get("mode") != mode:
        raise ValueError("build.json arm/modelset/mode do not match %s %s" % (arm, mode))
    fl = b.get("floor")
    if isinstance(fl, bool) or not isinstance(fl, (int, float)) or fl != float(FLOOR):
        raise ValueError("build.json floor must be 0.01")
    try:
        _check_chain_map(b.get("model_chain_sha256"), arm, "build.json")
    except PreflightError as e:
        raise ValueError(str(e))
    return b


def review_load(queue, decisions):
    """Link-review burden from review_queue.json items and reviewer decision records (answered by item or by pair)."""
    items = [i for i in queue if "link" in (i.get("kinds") or []) and not i.get("reason")]
    key = lambda i: tuple(sorted(i["pair"]))
    answered_items = {d["item"] for d in decisions}
    answered_pairs = {key(i) for i in items if i["item"] in answered_items}
    un = sum(1 for i in items if i["item"] not in answered_items and key(i) not in answered_pairs)
    return {"items": len(items), "distinct_pairs": len({key(i) for i in items}), "unanswered": un}


def lane_metrics(seed, report7, funnel, status, load):
    crc = _req(report7, "programmes_live", "crc")
    truth = _req(crc, "truth_closed")
    tp = _req(crc, "pred_closed") - _req(crc, "false_closures")
    if tp < 0 or tp > truth:
        raise ValueError("derived CRC tp %d outside [0, truth_closed=%d]" % (tp, truth))
    if truth and abs(tp / truth - _req(crc, "recall")) > 0.00051:
        raise ValueError("derived CRC recall %.5f disagrees with reported %r" % (tp / truth, crc["recall"]))
    if _req(funnel, "truth_closed") != truth:
        raise ValueError("funnel/CRC truth_closed mismatch")
    cats = _req(funnel, "categories")
    unknown = set(cats) - set(FUNNEL_CATEGORIES)
    if unknown:
        raise ValueError("unknown funnel categories %s" % sorted(unknown))
    attached = cats.get("attached_or_closed", 0)
    if attached < tp:
        raise ValueError("attached_or_closed cannot be less than exact CRC tp")
    residual = {name: cats.get(name, 0) for name in
                ("candidate_not_generated", "candidate_below_review", "candidate_rejected_demographic",
                 "candidate_routed_review", "linked_report_not_attached")}
    residual["attached_not_closed"] = attached - tp
    if sum(residual.values()) != truth - tp:
        raise ValueError("residual categories do not partition CRC closure misses")
    lk = _req(report7, "linkage")
    org = _req(report7, "outreach_live", "by_origin")
    rv = _req(report7, "reviewer")
    allk = _req(rv, "by_kind", "all")
    return {
        "seed": seed, "tp": tp, "truth_closed": truth, "recall": tp / truth if truth else None,
        "categories": {c: cats.get(c, 0) for c in FUNNEL_CATEGORIES},
        "residual": residual,
        "false_closures": _req(crc, "false_closures"),
        "wrong_released_total": _req(report7, "outreach_live", "wrong_patient_released"),
        "wrong_released_by_origin": {k: _req(v, "wrong_released") for k, v in sorted(org.items())},
        "replay_mismatch": _req(report7, "live_vs_replay_final_mismatch"),
        "analysis_errors": _req(funnel, "analysis_errors"),
        "trace_error": 1 if status.get("link_funnel_error") else 0,
        "false_merge_pairs": _req(lk, "false_merge_pairs"),
        "impure_clusters": _req(lk, "impure_clusters"),
        "cross_person_edges": sum(_req(lk, "cross_person_edges_by_basis").values()),
        "reviewer": {"items": _req(allk, "items"), "hours": _req(allk, "sim_hours"),
                     "wrong_final_signoffs": _req(allk, "wrong_final_signoffs"),
                     "distinct_pairs": _req(load, "distinct_pairs"), "unanswered": _req(load, "unanswered"),
                     "link_items": _req(load, "items"),
                     "by_programme": _req(rv, "by_programme"), "by_origin": _req(rv, "by_origin")},
    }


def aggregate_arm(arm, lanes):
    if arm not in ARMS:
        raise ValueError("invalid arm " + repr(arm))
    a = {"arm": arm, "modelset": MODELSET[arm], "per_seed": {str(l["seed"]): l for l in lanes}}
    a["tp"] = sum(l["tp"] for l in lanes)
    a["truth_closed"] = sum(l["truth_closed"] for l in lanes)
    a["recall"] = a["tp"] / a["truth_closed"] if a["truth_closed"] else None
    a["per_seed_recall"] = {str(l["seed"]): l["recall"] for l in lanes}
    a["recall_t95"] = _ival([100.0 * l["recall"] for l in lanes]) if all(l["recall"] is not None for l in lanes) else None
    for k in SAFETY + CONTAM:
        a[k] = sum(l[k] for l in lanes)
    org = {}
    for l in lanes:
        for k, v in l["wrong_released_by_origin"].items():
            org[k] = org.get(k, 0) + v
    a["wrong_released_by_origin"] = org
    a["categories"] = {c: sum(l["categories"][c] for l in lanes) for c in FUNNEL_CATEGORIES}
    a["residual"] = {c: sum(l["residual"][c] for l in lanes) for c in RESIDUAL}
    a["reviewer"] = {k: sum(l["reviewer"][k] for l in lanes) for k in REVIEWER_SUMS}
    return a


def paired_gain_interval(base, cand):
    """Per-seed candidate-minus-baseline recall gains in percentage points, with a paired t interval."""
    if set(base["per_seed"]) != set(cand["per_seed"]):
        raise ValueError("arms do not cover the same seeds")
    gains = {}
    for s in sorted(base["per_seed"], key=int):
        b, c = base["per_seed"][s], cand["per_seed"][s]
        if b["truth_closed"] != c["truth_closed"] or not b["truth_closed"]:
            raise ValueError("seed %s truth_closed differs between arms or is zero" % s)
        gains[s] = 100.0 * (c["tp"] - b["tp"]) / b["truth_closed"]
    out = {"per_seed_gain_pp": gains}
    iv = _ival(list(gains.values()))
    if iv:
        out.update(iv)
    return out


def gate(base, cand, kind):
    if kind not in GATE_KINDS:
        raise ValueError("gate kind must be one of %s" % (GATE_KINDS,))
    reasons = []
    if base.get("arm") != "baseline" or cand.get("arm") != CANDIDATE:
        reasons.append("arms must be baseline vs pairfeat1")
    if base["truth_closed"] != cand["truth_closed"]:
        reasons.append("truth_closed differs between arms")
    need = MIN_GAIN_PP.get(kind, 0.0)
    gain = None
    if not base["truth_closed"] or not cand["truth_closed"]:
        reasons.append("truth_closed is zero")
    else:
        g = 100 * (Fraction(cand["tp"], cand["truth_closed"]) - Fraction(base["tp"], base["truth_closed"]))
        gain = float(g)
        if g < Fraction(need):
            reasons.append("gain %.4fpp below %.1fpp" % (gain, need))
    for label, a in (("", cand), ("baseline ", base)):
        for k in SAFETY:
            if a[k] != 0:
                reasons.append(label + k + " nonzero")
        if any(v != 0 for v in a["wrong_released_by_origin"].values()):
            reasons.append(label + "wrong_released_by_origin nonzero")
    for k in CONTAM:
        if cand[k] > base[k]:
            reasons.append(k + " increased")
    bd, cd = base["reviewer"]["distinct_pairs"], cand["reviewer"]["distinct_pairs"]
    limit = bd * (1 + Fraction(str(MAX_REVIEW_GROWTH)))
    if (bd == 0 and cd != 0) or cd > limit:
        reasons.append("distinct link-review pairs %d exceed %.2f (baseline %d +%d%%)" % (cd, float(limit), bd, round(100 * MAX_REVIEW_GROWTH)))
    return {"pass": not reasons, "kind": kind, "gain_pp": None if gain is None else round(gain, 4), "min_gain_pp": need,
            "distinct_pairs": {"baseline": bd, "pairfeat1": cd, "max_allowed": float(limit)}, "reasons": reasons}


def select(base, cand):
    g = gate(base, cand, "selection")
    return {"selected_modelset": CANDIDATE if g["pass"] else None, "pass": g["pass"], "gate": g,
            "baseline_recall": _recall(base), "pairfeat1_recall": _recall(cand)}


def _recall(a):
    return a["tp"] / a["truth_closed"] if a["truth_closed"] else None


def _load_lane(path, seed, arm, mode):
    def j(p):
        with open(os.path.join(path, p)) as h:
            return json.load(h)
    r = j("run/main/report7.json")
    q = j("run/main/review_queue.json")
    f = j("funnel.json")
    check_funnel_floor(f)
    check_funnel_modelset(f, arm)
    check_funnel_scope(f, mode)
    dec = []
    dp = os.path.join(path, "run/main/reviewer_decisions.ndjson")
    if os.path.exists(dp):
        with open(dp) as h:
            dec = [json.loads(x) for x in h if x.strip()]
    return lane_metrics(seed, r, f, j("run/main/status-linker.json"), review_load(q, dec))


# ---------------------------------------------------------------- CLI

def analyse(mode, out, roots, repo=REPO, head=None):
    exp = os.path.dirname(os.path.abspath(out))
    seeds = MODES[mode]["seeds"]
    try:
        if mode == "validation-clean":
            gov = _read_json(os.path.join(exp, VALIDATION_MANIFEST))
            check_frozen_validation(gov, exp, repo, head or git_head(repo))
            gov_name = VALIDATION_MANIFEST
        else:
            gov = _read_json(os.path.join(exp, ARTIFACT_MANIFEST))
            gov_name = ARTIFACT_MANIFEST
        _check_maps(gov, gov_name)
    except PreflightError as e:
        raise OSError(str(e)) if "missing" in str(e) else ValueError(str(e))
    builds, arms = {}, {}
    for a in ARMS:
        b = read_build(roots[a], a, mode)
        if b["model_chain_sha256"] != gov[CHAIN_KEY[a]]:
            raise ValueError("arm %s model_chain_sha256 disagrees with %s" % (a, gov_name))
        builds[a] = b
        arms[a] = aggregate_arm(a, [_load_lane(os.path.join(roots[a], "v2-s%d" % s), s, a, mode) for s in seeds])
        arms[a]["build"] = b
    if builds["baseline"]["git_head"] != builds[CANDIDATE]["git_head"]:
        raise ValueError("arms were built at different git heads")
    if builds["baseline"]["binary_sha256"] != builds[CANDIDATE]["binary_sha256"]:
        raise ValueError("arms used different binaries")
    head = builds["baseline"]["git_head"]
    if gov.get("git_head") != head:
        raise ValueError("arm builds are not at the %s git_head" % gov_name)
    base, cand = arms["baseline"], arms[CANDIDATE]
    res = {"mode": mode, "seeds": seeds, "arms": arms, "git_head": head, "binary_sha256": builds["baseline"]["binary_sha256"],
           "paired_gain_t95": paired_gain_interval(base, cand),
           "note": "simulated; t intervals use n=%d seeds: small-n, descriptive only" % len(seeds)}
    ident = {"mode": mode, "seeds": seeds, "git_head": head, "binary_sha256": res["binary_sha256"],
             "baseline_chain_sha256": gov["baseline_chain_sha256"], "pairfeat1_chain_sha256": gov["pairfeat1_chain_sha256"],
             "governing_manifest_sha256": sha256_file(os.path.join(exp, gov_name))}
    if mode == "dev-clean":
        sel = select(base, cand)
        res["gate"] = sel["gate"]
        res["selection"] = dict(sel, **ident)
        _write_once(os.path.join(exp, SELECTION), res["selection"])
    elif mode == "dev-stress":
        sp = os.path.join(exp, SELECTION)
        try:
            check_selection(_read_json(sp), gov, head)
        except PreflightError as e:
            raise ValueError("dev-stress needs a passing selection: " + str(e))
        g = gate(base, cand, "stress")
        res["gate"] = g
        res["stress"] = dict(ident, **{"pass": g["pass"], "gate": g, "selection_sha256": sha256_file(sp)})
        _write_once(os.path.join(exp, STRESS), res["stress"])
    else:
        res["gate"] = gate(base, cand, "validation")
        _write_once(out, res)  # the headline validation verdict is never overwritten
        return res
    with open(out, "w") as h:
        json.dump(res, h, indent=1, sort_keys=True)
    return res


def main(argv):
    if argv[:1] == ["freeze-validation"]:
        if len(argv) != 2:
            print("usage: pairfeat_compare.py freeze-validation EXPERIMENT_DIR", file=sys.stderr)
            return 2
        try:
            vm = freeze_validation(os.path.abspath(argv[1]))
        except PreflightError as e:
            print("freeze refused: " + str(e), file=sys.stderr)
            return 2
        print(json.dumps({"frozen": VALIDATION_MANIFEST, "git_head": vm["git_head"], "seeds": vm["seeds"]}, sort_keys=True))
        return 0
    if argv[:1] == ["preflight"]:
        if len(argv) != 4:
            print("usage: pairfeat_compare.py preflight MODE ARM EXPERIMENT_DIR", file=sys.stderr)
            return 2
        try:
            print(json.dumps(preflight(argv[1], argv[2], argv[3], REPO, git_head(REPO)), sort_keys=True))
        except PreflightError as e:
            print("preflight refused: " + str(e), file=sys.stderr)
            return 2
        return 0
    usage = "usage: pairfeat_compare.py MODE OUT_JSON baseline=ROOT pairfeat1=ROOT | freeze-validation EXPERIMENT_DIR"
    if len(argv) != 4 or argv[0] not in MODES or os.path.basename(argv[1]) in RESERVED:
        print(usage, file=sys.stderr)
        return 2
    roots = {}
    for a in argv[2:]:
        name, _, root = a.partition("=")
        if name not in ARMS or name in roots or not root:
            print("bad or repeated arm " + name, file=sys.stderr)
            return 2
        roots[name] = root
    res = analyse(argv[0], argv[1], roots)
    print(json.dumps(res["gate"], sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
