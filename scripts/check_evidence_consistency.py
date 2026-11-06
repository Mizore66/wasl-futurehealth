#!/usr/bin/env python3
"""Deterministic submission-evidence consistency checker (stdlib only).

Checks that submission-facing documents agree with the authoritative data
(pipeline/RESULTS.md final seven-programme section, docs/labelling/agreement.json,
data/evidence/reports43.json) and do not turn built/simulated/designed claims
into clinical or human-validated claims. It changes nothing and computes no
new metrics.

Usage: python3 scripts/check_evidence_consistency.py [repo_root]
Exit status 0 when consistent, 1 when findings exist.
"""
import json
import re
import sys
from collections import namedtuple
from pathlib import Path

Finding = namedtuple("Finding", "rule file line msg")

DOCS = ["README.md"] + ["docs/" + n for n in (
    "BACKEND-HANDOFF.md", "BRIEF.md", "WRITEUP.md", "PITCH.md",
    "DEMO-RECORDING-GUIDE.md", "SUBMISSION-DRAFT.md")] + ["docs/labelling/README.md"]
RESULTS = "pipeline/RESULTS.md"
FINAL_HEADING = "### Frozen seven-programme evaluation at `075abc5`"

# Older five-programme run (5496563) figures, superseded by the 075abc5 run.
STALE = ["0.782", "3,025", "14,451", "9,602", "12.9 h"]
SUPERSEDE_MARK = re.compile(r"supersed|historical", re.I)
NEGATION = re.compile(r"\b(not|no|never|without|nor|neither|cannot)\b|n't", re.I)
OVERCLAIM = re.compile(
    r"clinically validated|clinician[- ]validated|human[- ]validated|"
    r"independent(ly)? human[- ]label|fully validated|"
    r"validated (default[- ]model )?(upgrade|agent)|"
    r"clinician endorsement|procurement agreement|buyer commitment|"
    r"live Malaffi integration|deployed (in|at) ", re.I)
NOT_INDEPENDENT = re.compile(r"not independent human[- ](double[- ]label|label)", re.I)
ZERO_124 = re.compile(r"0/124(?![,\d])")
KAPPA_ONE =re.compile(r"(κ|kappa)[^.\n]{0,20}?\b1(\.0+)?\b", re.I)


def _read(root, rel):
    p = Path(root) / rel
    return p.read_text(encoding="utf-8") if p.exists() else None


def _lines(root, rel):
    t = _read(root, rel)
    return [] if t is None else t.splitlines()


def final_figures(root):
    """Authoritative held-out v2 figures parsed from the final RESULTS section."""
    text = _read(root, RESULTS) or ""
    i = text.find(FINAL_HEADING)
    if i < 0:
        return None
    sec = text[i:]
    j = sec.find("\n## ", 10)
    sec = sec[:j] if j > 0 else sec
    out = {}
    m = re.search(r"^\| CRC \| [\d.]+ \| ([\d.]+) \|", sec, re.M)
    out["recall"] = m and m.group(1)
    m = re.search(r"CRC / cervical / lung / DR / breast false closures \|[^|]*\| 0/([\d,]+);", sec)
    out["crc_fc"] = m and m.group(1)
    m = re.search(r"Wrong-patient messages / without granted consent \|[^|]*\| 0/([\d,]+); 0/([\d,]+) \|", sec)
    out["wrong_patient"], out["no_consent"] = (m.group(1), m.group(2)) if m else (None, None)
    m = re.search(r"reviewer time was [\d.]+ and ([\d.]+) h/1k", sec)
    out["hours"] = m and m.group(1)
    return out


def check(root):
    f = []

    def add(rule, file, line, msg):
        f.append(Finding(rule, file, line, msg))

    # --- current final figures ---------------------------------------------
    fig = final_figures(root)
    if not fig or not all(fig.values()):
        add("current-figures", RESULTS, 0, f"cannot parse final seven-programme figures: {fig}")
    else:
        for rel in ("README.md", "docs/SUBMISSION-DRAFT.md", "docs/BRIEF.md", "docs/PITCH.md"):
            t = _read(root, rel)
            if t is None:
                add("current-figures", rel, 0, "file missing")
                continue
            for k, tok in fig.items():
                if tok not in t:
                    add("current-figures", rel, 0, f"missing final {k} = {tok}")

    # --- stale final-looking figures ---------------------------------------
    for rel in DOCS:
        for n, line in enumerate(_lines(root, rel), 1):
            hit = [s for s in STALE if s in line]
            if hit and not SUPERSEDE_MARK.search(line):
                add("stale-figure", rel, n, f"older-run figure(s) {hit} not marked superseded")

    # --- kappa data and wording --------------------------------------------
    try:
        ag = json.loads(_read(root, "docs/labelling/agreement.json"))
        if ag.get("n_reports") != 43:
            add("kappa-data", "docs/labelling/agreement.json", 0, "n_reports != 43")
        for name in ("caecum", "prep", "polyps", "histology", "state"):
            fld = ag["fields"].get(name, {})
            if fld.get("kappa_ab") != 1.0 or fld.get("agree_ab") != 1.0 or fld.get("disagreements"):
                add("kappa-data", "docs/labelling/agreement.json", 0,
                    f"{name}: kappa_ab/agree_ab not 1.0 or disagreements remain")
        if "not independent human double-labelling" not in ag.get("method", ""):
            add("kappa-data", "docs/labelling/agreement.json", 0, "method lacks not-independent wording")
        rep = json.loads(_read(root, "data/evidence/reports43.json"))
        v = rep["verification"]
        if v.get("status") != "clinically_verified" or v.get("human_double_labelled") is not False:
            add("kappa-data", "data/evidence/reports43.json", 0, "verification metadata inconsistent")
        if len(rep["reports"]) != 43:
            add("kappa-data", "data/evidence/reports43.json", 0, "report count != 43")
    except (TypeError, KeyError, ValueError) as e:
        add("kappa-data", "docs/labelling/agreement.json", 0, f"unreadable authoritative data: {e!r}")

    for rel in DOCS + [RESULTS]:
        for n, line in enumerate(_lines(root, rel), 1):
            if KAPPA_ONE.search(line):
                if not NOT_INDEPENDENT.search(line):
                    add("kappa-wording", rel, n, "κ value without 'not independent human double-labelling'")
                if "model-assisted" not in line.lower() and "model passes" not in line.lower() \
                        and "model-assisted" not in "".join(_lines(root, rel)[max(0, n - 3):n]).lower():
                    add("kappa-wording", rel, n, "κ value without 'model-assisted' nearby")

    # --- 19/20, 0 unsafe, fair-prep fix ------------------------------------
    for rel in ("docs/SUBMISSION-DRAFT.md", RESULTS):
        t = (_read(root, rel) or "").replace("*", "")
        if "20 automatic decisions, 19 correct and 0 unsafe" not in t:
            add("accuracy-claim", rel, 0, "missing '20 automatic decisions, 19 correct and 0 unsafe'")
        if "inadequate_prep" not in t or "fair" not in t:
            add("accuracy-claim", rel, 0, "missing fair -> inadequate_prep fix")
    for rel in DOCS:
        for n, line in enumerate(_lines(root, rel), 1):
            if "automatic decisions" in line and "19 correct" not in line:
                add("accuracy-claim", rel, n, "automatic-decision count is not 19 correct of 20")

    # --- stale pending-labelling statements --------------------------------
    stale_lab = re.compile(
        r"until our human double-labelling|Double-label the 43|LLM-produced key|"
        r"labelling (is )?(pending|in progress|not yet)|human double-labelling is (pending|done)|"
        r"no clinician labels", re.I)
    for rel in DOCS:
        for n, line in enumerate(_lines(root, rel), 1):
            low = line.lower()
            if stale_lab.search(line) and "held-out" not in low and "heldout" not in low:
                add("stale-labelling", rel, n, "pending/absent-labelling statement is stale for the 43 reports")

    # --- LDCT/IFHAS and 0/124 caveats ----------------------------------------
    for rel in DOCS + [RESULTS]:
        for n, line in enumerate(_lines(root, rel), 1):
            if ZERO_124.search(line) and "agreed-but-wrong" not in line:
                add("ldct-ifhas-caveat", rel, n, "0/124 not described as agreed-but-wrong")
        t = _read(root, rel) or ""
        if ZERO_124.search(t) and not re.search(r"non-blind|not blind", t, re.I):
            add("ldct-ifhas-caveat", rel, 0, "0/124 cited without non-blind caveat")
    t = _read(root, "docs/BRIEF.md") or ""
    if "non-blind" not in t or "reuse development texts" not in t:
        add("ldct-ifhas-caveat", "docs/BRIEF.md", 0,
            "needs non-blind and generators-reuse-development-texts caveats")
    for rel in ("docs/SUBMISSION-DRAFT.md", "README.md"):
        if not re.search(r"not independent (reader-)?generalisation", _read(root, rel) or ""):
            add("ldct-ifhas-caveat", rel, 0, "LDCT/IFHAS 'not independent generalisation' caveat missing")

    # --- r1agent10 wording -----------------------------------------------------
    for rel in ("docs/DEMO-RECORDING-GUIDE.md", "docs/SUBMISSION-DRAFT.md", "docs/BACKEND-HANDOFF.md", RESULTS):
        flat = (_read(root, rel) or "").replace("*", "")
        if "r1agent10" not in flat:
            add("agent-wording", rel, 0, "r1agent10 not mentioned")
            continue
        if not re.search(r"five fewer closures|five closures below|five below|lost five closures", flat):
            add("agent-wording", rel, 0, "r1agent10 without the five-fewer-closures-than-unguarded trade-off")
        if not re.search(r"validation (did not run|was not run)", flat, re.I):
            add("agent-wording", rel, 0, "r1agent10 without 'validation did not run'")

    # --- status labels and overclaims ----------------------------------------------
    t = _read(root, "docs/SUBMISSION-DRAFT.md") or ""
    for w in ("**Built**", "**Simulated**", "**Designed**"):
        if w not in t:
            add("status-labels", "docs/SUBMISSION-DRAFT.md", 0, f"missing status definition {w}")
    if "No clinical deployment or patient-outcome improvement is claimed" not in t:
        add("status-labels", "docs/SUBMISSION-DRAFT.md", 0, "missing no-clinical-deployment disclaimer")
    for rel in DOCS:
        for n, line in enumerate(_lines(root, rel), 1):
            if OVERCLAIM.search(line) and not NEGATION.search(line):
                add("overclaim", rel, n, "clinical/validated/deployed claim without negation")
    return f


def main(argv):
    root = Path(argv[1]) if len(argv) > 1 else Path(__file__).resolve().parent.parent
    found = check(root)
    for x in found:
        print(f"[{x.rule}] {x.file}:{x.line}: {x.msg}")
    print(f"{len(found)} finding(s)")
    return 1 if found else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
