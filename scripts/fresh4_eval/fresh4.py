#!/usr/bin/env python3
"""Fresh4 independent LDCT/IFHAS reader and integrated evaluation harness.

The production readers and integrated pipeline are not changed here. This module freezes a blind,
independently authored corpus, ingests two label files, applies a disagreements-only third-pass
adjudication, runs the shipped reader adapter once, and evaluates pre-registered gates with exact
numerators and denominators. Standard library only; cache construction is a separate shipped Go command.
"""
import argparse
import contextlib
import copy
import hashlib
import json
import math
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(HERE, "..", "fresh2_eval"))
import fresh2 as F  # noqa: E402

ROOT = "data/fresh4"
P4 = F.make_paths(ROOT)
PER_PROGRAMME = 50
INTEGRATED_SEEDS = [1103, 1109, 1117, 1121, 1123]
ADJUDICATION_SCHEMA = "fresh4-adjudication/v1"
ADJUDICATED_KEY_SCHEMA = "fresh4-adjudicated-key/v1"
SCORE_SCHEMA = "fresh4-score/v1"
ADJUDICATION = ROOT + "/adjudication.json"
ADJUDICATION_INPUT = ROOT + "/adjudication-input.json"
ADJUDICATED_KEY = ROOT + "/adjudicated-key.json"
BLIND_INPUT = ROOT + "/labeller-input.json"
BLIND_MANIFEST = ROOT + "/LABELLER-INPUT-MANIFEST.json"
POST_AGREEMENT = ROOT + "/post-adjudication-agreement.json"
PROTOCOL_PROTECTED = [
    "scripts/fresh2_eval/fresh2.py", "scripts/fresh2_eval/LABELLING-GUIDE.md",
    "scripts/fresh4_eval/fresh4.py", "scripts/fresh4_eval/test_fresh4.py",
    "scripts/fresh4_eval/PROTOCOL.md", "scripts/fresh4_eval/AUTHOR-PROMPT.md",
    "scripts/fresh4_eval/LABELLER-PROMPT-TEMPLATE.md", "scripts/fresh4_eval/ADJUDICATOR-PROMPT-TEMPLATE.md",
]
CORPUS_AUX = [BLIND_INPUT, BLIND_MANIFEST]
LABEL_AUX = [ADJUDICATION_INPUT, ADJUDICATION, ADJUDICATED_KEY, POST_AGREEMENT]
EXPECTED_IDS = (
    [f"f4l-{n:03d}" for n in range(1, 51)] +
    [f"f4i-{n:03d}" for n in range(1, 51)]
)


@contextlib.contextmanager
def fresh4_environment():
    old = {k: os.environ.get(k) for k in ("FRESH_EVAL_DIR", "FRESH_EVAL_PER_PROGRAMME")}
    os.environ["FRESH_EVAL_DIR"] = ROOT
    os.environ["FRESH_EVAL_PER_PROGRAMME"] = str(PER_PROGRAMME)
    try:
        yield
    finally:
        for k, v in old.items():
            if v is None:
                os.environ.pop(k, None)
            else:
                os.environ[k] = v


def sha_file(path):
    with open(path, "rb") as f:
        return hashlib.sha256(f.read()).hexdigest()


def validate_fresh4_corpus(c, known_texts=None):
    with fresh4_environment():
        errs = F.validate_corpus(c, known_texts)
    ids = [it.get("id") for it in c.get("items", []) if isinstance(it, dict)]
    if ids != EXPECTED_IDS:
        missing = [i for i in EXPECTED_IDS if i not in ids]
        extra = [i for i in ids if i not in EXPECTED_IDS]
        errs.append("fresh4 ids/order must be f4l-001..050 then f4i-001..050; missing=%r extra=%r" % (missing, extra))
    for it in c.get("items", []):
        if not isinstance(it, dict):
            continue
        n = int(it["id"].split("-")[1])
        want = ("ldct-screen" if it["id"].startswith("f4l-") and n <= 25 else
                "ldct-workup" if it["id"].startswith("f4l-") else
                "ifhas-followup" if n <= 25 else "ifhas-screen")
        if it.get("kind") != want:
            errs.append("%s: kind must be %s for its pre-registered adversarial block" % (it["id"], want))
    return errs


def make_blind_input(c):
    """Only the guide and this return value may be shown to a blind labeller."""
    return {"schema": c["schema"], "items": [copy.deepcopy(it) for it in c["items"]]}


def make_disagreement_input(c, key):
    disputed = {it["id"]: it for it in key["items"] if it["status"] == "unadjudicated"}
    rows = []
    for item in c["items"]:
        if item["id"] not in disputed:
            continue
        row = copy.deepcopy(item)
        row["labels"] = copy.deepcopy(disputed[item["id"]]["labels"])
        rows.append(row)
    return {"schema": "fresh4-adjudication-input/v1", "corpus_sha256": F.canon_sha(c),
            "instruction": "Disagreements only. Use the guide and text; no reader output, score or agreed item is present.",
            "items": rows}


def apply_adjudication(key, adj):
    if not isinstance(adj, dict) or adj.get("schema") != ADJUDICATION_SCHEMA:
        raise F.HarnessError("adjudication schema must be " + ADJUDICATION_SCHEMA)
    if adj.get("corpus_sha256") != key.get("corpus_sha256"):
        raise F.HarnessError("adjudication corpus hash differs from key")
    who = adj.get("adjudicator")
    if (not isinstance(who, dict) or not who.get("id") or who.get("session_isolated") is not True
            or who.get("saw_reader_outputs") is not False or who.get("saw_scores") is not False):
        raise F.HarnessError("adjudicator must declare isolation and no reader outputs or scores")
    rows = adj.get("items")
    if not isinstance(rows, list):
        raise F.HarnessError("adjudication items must be a list")
    expected = {it["id"]: it for it in key["items"] if it["status"] == "unadjudicated"}
    seen = set()
    decisions = {}
    for row in rows:
        if not isinstance(row, dict):
            raise F.HarnessError("adjudication row is not an object")
        iid = row.get("id")
        if iid not in expected:
            raise F.HarnessError("adjudicator may decide only a disagreement; invalid id %r" % iid)
        if iid in seen:
            raise F.HarnessError("duplicate adjudication for " + iid)
        seen.add(iid)
        kind = expected[iid]["kind"]
        resolution = row.get("resolution")
        if resolution not in ("adjudicated", "unresolved"):
            raise F.HarnessError("%s: resolution must be adjudicated or unresolved" % iid)
        if not isinstance(row.get("rationale"), str) or not row["rationale"].strip():
            raise F.HarnessError("%s: rationale required" % iid)
        if row.get("uncertainty") not in ("low", "moderate", "high"):
            raise F.HarnessError("%s: uncertainty must be low, moderate or high" % iid)
        k = row.get("key")
        if resolution == "unresolved":
            if k is not None:
                raise F.HarnessError("%s: unresolved item must have null key" % iid)
        else:
            fields = F.KINDS[kind]["fields"]
            if not isinstance(k, dict) or set(k) != set(fields):
                raise F.HarnessError("%s: adjudicated key must contain exactly %r" % (iid, fields))
            for field in fields:
                err = F.label_error(kind, k[field], field)
                if err:
                    raise F.HarnessError("%s: %s %s" % (iid, field, err))
        decisions[iid] = copy.deepcopy(row)
    missing = sorted(set(expected) - seen)
    if missing:
        raise F.HarnessError("adjudication is incomplete; missing %r" % missing)
    out = copy.deepcopy(key)
    out["schema"] = ADJUDICATED_KEY_SCHEMA
    out["base_key_sha256"] = F.canon_sha(key)
    out["adjudicator"] = copy.deepcopy(who)
    out["adjudication_sha256"] = F.canon_sha(adj)
    out["semantics"] = copy.deepcopy(out["semantics"])
    out["semantics"]["statement"] = (
        "Fresh4 keys are model-labelled and model-adjudicated from a frozen guide, not clinician-verified. "
        "Agreed and adjudicated strata are reported separately; neither is clinical truth or evidence of clinical effectiveness."
    )
    n_adj = 0
    for it in out["items"]:
        d = decisions.get(it["id"])
        if d is None:
            continue
        it["adjudication"] = {"rationale": d["rationale"], "uncertainty": d["uncertainty"]}
        if d["resolution"] == "adjudicated":
            it["status"] = "adjudicated"
            it["key"] = copy.deepcopy(d["key"])
            n_adj += 1
        else:
            it["status"] = "unadjudicated"
            it["key"] = None
    total = len(out["items"])
    unresolved = total - key["counts"]["agreed"] - n_adj
    out["counts"] = dict(key["counts"], adjudicated=n_adj, unresolved=unresolved,
                         scoreable=key["counts"]["agreed"] + n_adj)
    return out


def _score_subset(key, raw, statuses):
    k = copy.deepcopy(key)
    k["items"] = [it for it in k["items"] if it["status"] in statuses]
    ids = {it["id"] for it in k["items"]}
    r = copy.deepcopy(raw)
    r["items"] = [it for it in r["items"] if it["id"] in ids]
    for it in k["items"]:
        if it["status"] == "adjudicated":
            it["status"] = "agreed"
    return F.compute_scores(k, r)


def _composite(label):
    out = {}
    if not isinstance(label, str):
        return out
    for part in label.split(";"):
        if "=" in part:
            d, v = part.split("=", 1)
            out[d] = v
    return out


def direction(kind, key_label, reader_label):
    """Closing means an asserted resolution not supported by the key; all other wrong answers hold."""
    if kind == "ldct-workup":
        return "closing" if reader_label in ("wk_treatment", "wk_benign") and reader_label != key_label else "holding"
    if kind == "ifhas-followup":
        kd, rd = _composite(key_label), _composite(reader_label)
        if key_label in ("dna", "indeterminate"):
            return "closing" if any(v in ("managed", "not_confirmed") for v in rd.values()) else "holding"
        for d, v in rd.items():
            if v in ("managed", "not_confirmed") and kd.get(d) in ("pending", None):
                return "closing"
    return "holding"


def _wilson(k, n):
    if not n:
        return [None, None, None]
    z = 1.959963984540054
    p = k / n
    den = 1 + z * z / n
    centre = (p + z * z / (2 * n)) / den
    half = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / den
    return [round(p, 6), round(max(0, centre - half), 6), round(min(1, centre + half), 6)]


def compute_fresh4_scores(key, raw):
    F._validate_raw(key, raw)
    pooled = _score_subset(key, raw, ("agreed", "adjudicated", "unadjudicated"))
    by_origin = {
        "agreed": _score_subset(key, raw, ("agreed",)),
        "adjudicated": _score_subset(key, raw, ("adjudicated",)),
    }
    keyed = {it["id"]: it for it in key["items"] if it["status"] in ("agreed", "adjudicated")}
    rows = {it["id"]: it for it in raw["items"]}
    wrong = []
    for kind, s in pooled["agreed"]["by_kind"].items():
        for iid in s["agreed_but_wrong_ids"]:
            wrong.append({"id": iid, "kind": kind, "key_origin": keyed[iid]["status"],
                          "key": keyed[iid]["key"]["label"], "reader_label": rows[iid]["label"],
                          "direction": direction(kind, keyed[iid]["key"]["label"], rows[iid]["label"])})
    wrong_direction = {"closing": sum(x["direction"] == "closing" for x in wrong),
                       "holding": sum(x["direction"] == "holding" for x in wrong),
                       "total": len(wrong), "items": wrong}
    analysis = {}
    for kind, s in sorted(pooled["agreed"]["by_kind"].items()):
        n = s["n"]
        analysis[kind] = {"n": n,
                          "reader_a_correct_wilson95": _wilson(s["reader_a_correct"], n),
                          "reader_b_correct_wilson95": _wilson(s["reader_b_correct"], n),
                          "readers_agree_wilson95": _wilson(s["agree"], n)}
    return {"schema": SCORE_SCHEMA, "semantics": key["semantics"], "counts": key["counts"],
            "by_key_origin": by_origin, "pooled": pooled, "wrong_direction": wrong_direction,
            "analysis_wilson95": analysis, "models": raw.get("models", {}), "raw_items": len(raw["items"])}


def reader_gates(score, expected_items=100):
    if score is None:
        return [{"id": "R1_complete_raw_output", "status": "not_run", "expected_items": expected_items},
                {"id": "R2_zero_closing_direction", "status": "not_run", "numerator": None,
                 "denominator": None, "threshold": 0}]
    n = score.get("pooled", {}).get("agreed", {}).get("totals", {}).get("n", 0)
    closing = score.get("wrong_direction", {}).get("closing")
    return [
        {"id": "R1_complete_raw_output", "status": "pass" if score.get("raw_items") == expected_items else "fail",
         "observed_items": score.get("raw_items"), "expected_items": expected_items},
        {"id": "R2_zero_closing_direction", "status": "pass" if closing == 0 else "fail",
         "numerator": closing, "denominator": n, "threshold": 0,
         "clopper_pearson_note": "Zero events do not establish a zero clinical risk; denominator is pooled scoreable items."},
    ]


FALSE_CLOSURE_METRICS = [
    "crc_false_closures", "cervical_false_closures", "lung_false_closures", "dr_false_closures",
    "dr_referral_false_closures", "breast_false_closures", "breast_recall_false_closures",
    "ldct_false_closures", "ldct_action_false_closures", "ifhas_false_closures", "ifhas_followup_false_closures",
]
ZERO_METRICS = {
    "I2": ["wrong_patient_released", "released_without_granted_consent"],
    "I3": ["duplicate_or_lost_outputs", "ingest_lost_or_duplicated", "final_state_mismatch_loops"],
}


def _zero_gate(gid, noise, safety, metrics):
    failures, numerator, denominator = [], 0, 0
    for metric in metrics:
        value = safety.get(metric)
        if not isinstance(value, list) or len(value) < 2 or value[0] is None:
            failures.append({"metric": metric, "numerator": None, "denominator": None, "error": "missing"})
            continue
        numerator += value[0]
        if value[1] is not None:
            denominator += value[1]
        if value[0] != 0:
            failures.append({"metric": metric, "numerator": value[0], "denominator": value[1]})
    return {"id": gid, "noise": noise, "status": "pass" if not failures else "fail",
            "numerator": numerator, "denominator": denominator, "threshold": 0, "failures": failures}


def integrated_gates(summary):
    if summary is None:
        return [{"id": "I0_all_10_lanes_complete", "status": "not_run", "observed_lanes": 0, "expected_lanes": 10}]
    by_noise = summary.get("by_noise", {})
    observed = sum(by_noise.get(n, {}).get("seeds", 0) for n in ("v1", "v2"))
    gates = [{"id": "I0_all_10_lanes_complete", "status": "pass" if observed == 10 else "fail",
              "observed_lanes": observed, "expected_lanes": 10, "seeds": INTEGRATED_SEEDS}]
    for noise in ("v1", "v2"):
        safety = by_noise.get(noise, {}).get("safety", {})
        gates.append(_zero_gate("I1_%s_zero_listed_false_closures" % noise, noise, safety, FALSE_CLOSURE_METRICS))
        for prefix, name in (("I2", "zero_wrong_patient_or_unconsented_releases"),
                             ("I3", "zero_duplicate_loss_or_state_mismatch")):
            g = _zero_gate("%s_%s_%s" % (prefix, noise, name), noise, safety, ZERO_METRICS[prefix])
            gates.append(g)
        forged = _zero_gate("I4_%s_zero_forged_with_effect" % noise, noise, safety, ["forged_with_effect"])
        tamper = safety.get("tamper_detected")
        alarms = safety.get("tamper_false_alarms")
        if tamper != [85, 85, None] or not alarms or alarms[0] != 0:
            forged["status"] = "fail"
            forged["failures"].append({"metric": "tamper_detection_or_false_alarm", "observed": [tamper, alarms],
                                       "expected": {"detected": [85, 85], "false_alarms": 0}})
        gates.append(forged)
    return gates


def check_fresh4_freeze(repo, manifest, pre_score=True):
    with fresh4_environment():
        errs = F.check_freeze(repo, manifest, paths=P4, pre_score=pre_score)
    rc, cc, lc = manifest["reader_code_commit"], manifest["corpus_commit"], manifest["labels_commit"]
    for path in PROTOCOL_PROTECTED:
        if not F._exists_at(repo, rc, path):
            errs.append("protocol protected file missing at reader_code_commit: " + path)
        for changed in F._changed(repo, rc, [path]) or []:
            errs.append("%s changed since reader_code_commit" % changed)
    for path in CORPUS_AUX:
        if not F._exists_at(repo, cc, path):
            errs.append(path + " missing at corpus_commit")
        for changed in F._changed(repo, cc, [path]) or []:
            errs.append("%s changed since corpus_commit" % changed)
    for path in LABEL_AUX:
        if F._exists_at(repo, cc, path):
            errs.append(path + " present before labels")
        if not F._exists_at(repo, lc, path):
            errs.append(path + " missing at labels_commit")
        for changed in F._changed(repo, lc, [path]) or []:
            errs.append("%s changed since labels_commit" % changed)
    for line in (F._git(repo, "status", "--porcelain", "--untracked-files=all") or "").splitlines():
        path = line[3:].strip().strip('"')
        if path.startswith("scripts/fresh4_eval/"):
            errs.append("uncommitted change under watched path: " + path)
    return errs


def render_score(score, gates):
    t = score["pooled"]["agreed"]["totals"]
    lines = ["# Fresh4 frozen reader scores", "", "**" + score["semantics"]["statement"] + "**", "",
             "Status: built; simulated synthetic text; cached local Gemma reader; no clinical-effectiveness claim.", "",
             "Key origins: %d direct agreements + %d adjudicated = %d scoreable; %d unresolved." % (
                 score["counts"]["agreed"], score["counts"]["adjudicated"], score["counts"]["scoreable"],
                 score["counts"]["unresolved"]), "",
             "| stratum | scoreable n | A correct | B correct | B unavailable | readers agree | agreed but wrong |",
             "|---|---:|---:|---:|---:|---:|---:|"]
    for name, block in list(score["by_key_origin"].items()) + [("pooled", score["pooled"])]:
        x = block["agreed"]["totals"]
        lines.append("| %s | %d | %d | %d | %d | %d | %d |" % (
            name, x.get("n", 0), x.get("reader_a_correct", 0), x.get("reader_b_correct", 0),
            x.get("reader_b_unavailable", 0), x.get("agree", 0), x.get("agreed_but_wrong", 0)))
    lines += ["", "Pooled per-kind denominators and Wilson 95% intervals are in score.json/analysis_wilson95.", "",
              "Wrong agreed answers: %d closing-direction and %d holding-direction." % (
                  score["wrong_direction"]["closing"], score["wrong_direction"]["holding"]), "",
              "| gate | status | numerator | denominator |", "|---|---|---:|---:|"]
    for g in gates:
        lines.append("| %s | %s | %s | %s |" % (g["id"], g["status"], g.get("numerator"), g.get("denominator")))
    lines += ["", "Unresolved items are not scored for accuracy. No gate or reader was changed after seeing output.", ""]
    return "\n".join(lines)


def run_fresh4_score(repo, manifest):
    errs = check_fresh4_freeze(repo, manifest, pre_score=True)
    if errs:
        raise F.HarnessError("freeze check failed; reader not run:\n  " + "\n  ".join(errs))
    key, _ = F.read_json(os.path.join(repo, ADJUDICATED_KEY))
    cmd = ["go", "test", "-count=1", "-run", "^TestFresh2Reader$", "."]
    env = {"FRESH2_CORPUS": os.path.join(repo, P4["corpus"]),
           "FRESH2_CACHE_DIR": os.path.join(repo, ROOT, "reader-cache")}
    raw = F.run_reader(cmd, os.path.join(repo, "pipeline"), env)
    score = compute_fresh4_scores(key, raw)
    score["frozen"] = dict(manifest, head=(F._git(repo, "rev-parse", "HEAD") or "").strip(),
                           adjudicated_key_sha256=F.canon_sha(key))
    gates = reader_gates(score)
    rd = os.path.join(repo, P4["results_dir"])
    os.mkdir(rd)
    F.write_new(os.path.join(rd, "reader_raw.json"), F.dumps(raw))
    F.write_new(os.path.join(rd, "score.json"), F.dumps(score))
    F.write_new(os.path.join(rd, "reader_gates.json"), F.dumps({"schema": "fresh4-reader-gates/v1", "gates": gates}))
    F.write_new(os.path.join(rd, "SCORE.md"), render_score(score, gates))
    return score, gates


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--repo", default=F.REPO)
    sub = ap.add_subparsers(dest="cmd", required=True)
    sub.add_parser("validate-corpus")
    sub.add_parser("blind-input")
    sub.add_parser("ingest")
    sub.add_parser("adjudication-input")
    sub.add_parser("apply-adjudication")
    p = sub.add_parser("export-inputs"); p.add_argument("outdir")
    p = sub.add_parser("manifest"); p.add_argument("--reader-code-commit", required=True)
    p = sub.add_parser("check-freeze"); p.add_argument("--after-score", action="store_true")
    sub.add_parser("score")
    p = sub.add_parser("integrated-gates"); p.add_argument("summary"); p.add_argument("--out", required=True)
    a = ap.parse_args(argv)
    repo = os.path.abspath(a.repo)
    path = lambda rel: os.path.join(repo, rel)
    try:
        if a.cmd == "validate-corpus":
            c = F._load_corpus(path(P4["corpus"]))
            errs = validate_fresh4_corpus(c, F.load_known_texts(repo, paths=P4))
            print("\n".join(errs) if errs else "fresh4 corpus OK: 100 items, sha256 " + F.canon_sha(c))
            return 1 if errs else 0
        if a.cmd == "blind-input":
            c = F._load_corpus(path(P4["corpus"]))
            blind = make_blind_input(c)
            F.write_new(path(BLIND_INPUT), F.dumps(blind))
            m = {"schema": "fresh4-labeller-input-manifest/v1", "canonical_corpus_path": P4["corpus"],
                 "canonical_corpus_sha256": F.canon_sha(c), "labeller_input_path": BLIND_INPUT,
                 "labeller_input_sha256": sha_file(path(BLIND_INPUT)), "item_count": len(blind["items"]),
                 "guide_path": "scripts/fresh2_eval/LABELLING-GUIDE.md", "guide_sha256": F.guide_sha256(),
                 "excluded": ["provenance", "author files", "coverage mapping", "keys", "labels", "reader code", "scores"],
                 "declaration": "Blind labellers receive only the frozen guide and metadata-free labeller-input.json."}
            F.write_new(path(BLIND_MANIFEST), F.dumps(m))
            print("wrote key-free blind input and manifest")
            return 0
        if a.cmd == "ingest":
            with fresh4_environment():
                c = F._load_corpus(path(P4["corpus"]))
                (la, ba), (lb, bb) = (F.read_json(path(p)) for p in P4["labels"])
                key, agreement = F.build_key(c, la, lb, ba, bb)
            F.write_new(path(P4["key"]), F.dumps(key))
            F.write_new(path(P4["agreement"]), F.dumps(agreement))
            print(F.dumps({"counts": key["counts"], "overall": agreement["overall"]}), end="")
            return 0
        if a.cmd == "adjudication-input":
            c = F._load_corpus(path(P4["corpus"]))
            key, _ = F.read_json(path(P4["key"]))
            d = make_disagreement_input(c, key)
            F.write_new(path(ADJUDICATION_INPUT), F.dumps(d))
            print("wrote %d disagreements" % len(d["items"]))
            return 0
        if a.cmd == "apply-adjudication":
            key, _ = F.read_json(path(P4["key"]))
            adj, _ = F.read_json(path(ADJUDICATION))
            out = apply_adjudication(key, adj)
            F.write_new(path(ADJUDICATED_KEY), F.dumps(out))
            post = {"schema": "fresh4-post-adjudication-agreement/v1", "counts": out["counts"],
                    "adjudicator": out["adjudicator"], "uncertainty": {
                        level: sum(it.get("adjudication", {}).get("uncertainty") == level for it in out["items"])
                        for level in ("low", "moderate", "high")},
                    "note": "Direct agreement is measured before adjudication. Adjudicated items remain a separate scoring stratum."}
            F.write_new(path(POST_AGREEMENT), F.dumps(post))
            print(F.dumps(out["counts"]), end="")
            return 0
        if a.cmd == "export-inputs":
            c = F._load_corpus(path(P4["corpus"]))
            F.export_reader_inputs(c, path(a.outdir))
            print("wrote key-free reader cache inputs")
            return 0
        if a.cmd == "manifest":
            with fresh4_environment():
                m = F.build_manifest(repo, a.reader_code_commit, paths=P4)
            F.write_new(path(P4["manifest"]), F.dumps(m))
            print(F.dumps(m), end="")
            return 0
        if a.cmd == "check-freeze":
            m, _ = F.read_json(path(P4["manifest"]))
            errs = check_fresh4_freeze(repo, m, pre_score=not a.after_score)
            print("\n".join(errs) if errs else "fresh4 freeze OK")
            return 1 if errs else 0
        if a.cmd == "score":
            m, _ = F.read_json(path(P4["manifest"]))
            score, gates = run_fresh4_score(repo, m)
            print(F.dumps({"counts": score["counts"], "wrong_direction": score["wrong_direction"], "gates": gates}), end="")
            return 0
        if a.cmd == "integrated-gates":
            summary, _ = F.read_json(a.summary)
            gates = integrated_gates(summary)
            F.write_new(a.out, F.dumps({"schema": "fresh4-integrated-gates/v1", "gates": gates,
                                        "r1agent10_validation": "not run; its separate validation seeds remain untouched"}))
            print(F.dumps(gates), end="")
            return 0 if all(g["status"] == "pass" for g in gates) else 1
    except (F.HarnessError, OSError, ValueError) as e:
        print(str(e), file=sys.stderr)
        return 1
    return 2


if __name__ == "__main__":
    sys.exit(main())
