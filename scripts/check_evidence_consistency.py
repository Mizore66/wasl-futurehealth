#!/usr/bin/env python3
"""Deterministic submission-evidence consistency checker (stdlib only).

Checks that submission-facing documents agree with the authoritative data
(pipeline/RESULTS.md final seven-programme section, docs/labelling/agreement.json,
data/evidence/reports43.json) and do not turn built/simulated/designed claims
into clinical or human-validated claims. It also rejects tracked text files
that contain a developer's absolute home-directory path. It changes nothing
and computes no new metrics.

Usage: python3 scripts/check_evidence_consistency.py [repo_root]
Exit status 0 when consistent, 1 when findings exist.
"""
import datetime
import hashlib
import json
import re
import subprocess
import sys
from collections import namedtuple
from pathlib import Path

Finding = namedtuple("Finding", "rule file line msg")

DOCS = ["README.md"] + ["docs/" + n for n in (
    "BACKEND-HANDOFF.md", "BRIEF.md", "WRITEUP.md", "PITCH.md",
    "DEMO-RECORDING-GUIDE.md", "SUBMISSION-DRAFT.md")] + ["docs/labelling/README.md"]
RESULTS = "pipeline/RESULTS.md"
ANNOTATION_SIDECAR = "pipeline/results/final/annotation-provenance.json"
FROZEN_MANIFEST = "pipeline/results/final/manifest.json"
FINAL_HEADING = "### Frozen seven-programme evaluation at `075abc5`"

# Older-run (5496563 and intermediate) figures, superseded by the 075abc5 run.
# Matched with numeric boundaries, so "13,025" or "0.7821" do not count.
STALE = ["0.782", "0.842", "0.843", "3,025", "3,147", "3,156", "14,451", "9,602",
         "1,903", "5,625", "1,403", "1,404", "2,025", "2,032", "5,621", "5,636", "12.9 h"]
SUPERSEDE_MARK = re.compile(r"supersed|historical", re.I)
# Exempt from current-claim checks: other experiments or explicitly historical text.
OTHER_RUN = re.compile(r"supersed|historical|sensitivity|clean-review|post-hoc|baseline|"
                       r"earlier|intermediate|dev(elopment)? (noise|v1)|no review", re.I)
NEGATION = re.compile(r"\b(not|no|never|without|nor|neither|cannot|none)\b|n't", re.I)
OVERCLAIM = re.compile(
    r"clinically validated|clinician[- ]validated|human[- ]validated|"
    r"independent(ly)? human[- ]label|fully validated|"
    r"validated (default[- ]model )?(upgrade|agent)|"
    r"clinician endorsement|procurement agreement|buyer commitment|"
    r"live Malaffi integration|deployed (in|at) ", re.I)
NOT_INDEPENDENT = re.compile(r"not independent human[- ](double[- ]label|label)", re.I)
ZERO_124 = re.compile(r"0/124(?![,\d])")
KAPPA_ONE = re.compile(r"(κ|kappa)[^.\n]{0,20}?\b1(\.0+)?\b", re.I)
FAIR_PREP = re.compile(r"\bfair\b[^.]{0,80}inadequate_prep")
STALE_LAB = re.compile(
    r"until our human double-labelling|Double-label the 43|LLM-produced key|"
    r"labelling (is )?(pending|in progress|not yet)|human double-labelling is (pending|done)|"
    r"double-label\w*[^.]{0,60}\b(pending|in progress|not yet|still to)|"
    r"no clinician labels", re.I)
HELDOUT_CTX = re.compile(r"held-?out|fresh|LDCT|IFHAS|cohorts?", re.I)
SENTENCE = re.compile(r"(?<=[.!?])\s+(?=[A-Z0-9`*(\[])")
NEG_WINDOW = 40  # characters of same-clause text before/after a match searched for negation
FIG_FILES = ("README.md", "docs/SUBMISSION-DRAFT.md", "docs/BRIEF.md", "docs/PITCH.md")
WRITEUP = "docs/WRITEUP.md"


def _num(tok):
    """Regex for a number token with numeric boundaries."""
    return r"(?<![\d,.])" + re.escape(tok) + r"(?![\d]|,\d|\.\d)"


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
    m = re.search(r"CRC / cervical / lung / DR / breast false closures \|[^|]*\| "
                  r"0/([\d,]+); 0/([\d,]+); 0/([\d,]+); 0/([\d,]+); 0/([\d,]+) \|", sec)
    for k, i in zip(("crc_fc", "cervical_fc", "lung_fc", "dr_fc", "breast_fc"), range(1, 6)):
        out[k] = m and m.group(i)
    m = re.search(r"Wrong-patient messages / without granted consent \|[^|]*\| 0/([\d,]+); 0/([\d,]+) \|", sec)
    out["wrong_patient"], out["no_consent"] = (m.group(1), m.group(2)) if m else (None, None)
    m = re.search(r"reviewer time was [\d.]+ and ([\d.]+) h/1k", sec)
    out["hours"] = m and m.group(1)
    return out


CLAUSE_BREAK = re.compile(r"[.;:]|\b(but|however|although|whereas|yet)\b", re.I)


def negated(line, pos):
    """Heuristic: a negation word earlier in the same clause (up to 120 chars back, not past
    a sentence end, semicolon, colon or contrast word). Commas do not break a clause so that
    "Do not claim A, B or C" is covered; it cannot understand arbitrary grammar."""
    before = line[max(0, pos - 120):pos]
    last = None
    for m in CLAUSE_BREAK.finditer(before):
        last = m
    if last:
        before = before[last.end():]
    return NEGATION.search(before) is not None


def has_num(text, tok):
    return re.search(_num(tok), text) is not None


def sentences(line):
    return SENTENCE.split(line)


def historic_row(line):
    """A table row whose label (first 250 chars) marks it superseded or historical."""
    return line.lstrip().startswith("|") and SUPERSEDE_MARK.search(line[:250]) is not None


def claim_patterns(fig):
    """(key, regex, expected) for headline claims; a captured value must equal expected."""
    return [
        ("recall", r"CRC (?:P|precision) 1\.000, (?:R|recall) ([\d.]*\d)", fig["recall"]),
        ("crc_fc", r"0/([\d,]*\d)\s+(?:CRC )?false closures", fig["crc_fc"]),
        ("wrong_patient", r"0/([\d,]*\d)\s+wrong-patient messages|[Ww]rong-patient messages\s+0/([\d,]*\d)", fig["wrong_patient"]),
        ("no_consent", r"0/([\d,]*\d)\s+(?:messages )?(?:sent )?(?:releases )?without (?:granted )?consent", fig["no_consent"]),
        ("hours", r"([\d.]*\d) h (?:per|/) ?1(?:,000|k)", fig["hours"]),
    ]


def check_claims(root, rel, fig, add):
    """Every headline claim must carry the current value, unless it is explicitly another run."""
    for n, line in enumerate(_lines(root, rel), 1):
        if re.search(r"sensitivity|clean-review|post-hoc|baseline|no review|separate|live-reader|stress run", line, re.I) or historic_row(line):
            continue
        if not re.search(r"frozen|075abc5|held-out (v2|noise)", line, re.I):
            continue  # only headline lines about the frozen final evaluation
        for key, pat, want in claim_patterns(fig):
            for m in re.finditer(pat, line):
                got = next(g for g in m.groups() if g)
                if key == "crc_fc":  # only counts attached to the CRC programme
                    pre = line[max(0, m.start() - 80):m.start()]
                    k = pre.rfind("CRC")
                    if k < 0 and "CRC" not in m.group(0):
                        continue
                    if k >= 0 and re.search(r"lung|cervical|\bDR\b|breast|retinopathy|LDCT|IFHAS", pre[k:]):
                        continue
                local = line[max(0, m.start() - 100):m.end() + 60]
                if got != want and not SUPERSEDE_MARK.search(local):
                    add("current-figures", rel, n, f"{key} claim {got} != current {want}")


def _timestamp(value, field):
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"{field} must be a valid ISO 8601 timestamp")
    text = value.strip()
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"
    try:
        parsed = datetime.datetime.fromisoformat(text)
    except ValueError as e:
        raise ValueError(f"{field} must be a valid ISO 8601 timestamp") from e
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=datetime.timezone.utc)
    return parsed


def check_annotation_provenance(root, add):
    """Validate the post-hoc sidecar without changing or reinterpreting frozen results."""
    rel = ANNOTATION_SIDECAR
    try:
        raw = _read(root, rel)
        if raw is None:
            add("annotation-provenance", rel, 0, "post-hoc annotation sidecar missing")
            return
        sidecar = json.loads(raw)
        if not isinstance(sidecar, dict):
            raise ValueError("sidecar must be an object")
        if sidecar.get("record_type") != "post_hoc_annotation_provenance" or sidecar.get("post_hoc") is not True:
            add("annotation-provenance", rel, 0, "sidecar must be explicitly post-hoc")
        if sidecar.get("annotations_completed_after_frozen_run") is not True:
            add("annotation-provenance", rel, 0, "later annotation must be placed after the frozen run")
        if sidecar.get("evaluation_rerun") is not False or sidecar.get("frozen_results_modified") is not False:
            add("annotation-provenance", rel, 0, "sidecar must not claim a rerun or frozen-result modification")
        timing = sidecar.get("timing_note", "")
        for wording in ("post-hoc", "after the frozen evaluation",
                        "does not place later annotation before the frozen run"):
            if wording not in timing:
                add("annotation-provenance", rel, 0, f"timing note lacks '{wording}'")

        frozen_ref = sidecar["frozen_run"]
        completed_at = _timestamp(sidecar.get("annotation_completed_at"), "annotation_completed_at")
        frozen_at = _timestamp(frozen_ref.get("frozen_at"), "frozen_run.frozen_at")
        if completed_at <= frozen_at:
            add("annotation-provenance", rel, 0,
                "annotation_completed_at must be later than frozen_run.frozen_at")
        if frozen_ref.get("manifest") != "manifest.json":
            add("annotation-provenance", rel, 0, "sidecar must target the adjacent historical manifest.json")
        manifest_path = Path(root) / FROZEN_MANIFEST
        manifest_bytes = manifest_path.read_bytes()
        manifest = json.loads(manifest_bytes)
        if frozen_ref.get("manifest_sha256") != hashlib.sha256(manifest_bytes).hexdigest():
            add("annotation-provenance", rel, 0, "historical manifest hash differs from the frozen sidecar target")
        for key in ("code_revision", "frozen_at"):
            if frozen_ref.get(key) != manifest.get(key):
                add("annotation-provenance", rel, 0, f"sidecar frozen-run {key} does not match manifest")

        annotation = sidecar["annotation"]
        if not isinstance(annotation, dict):
            raise ValueError("annotation must be an object")
        if annotation.get("limitation") != "not independent human double-labelling":
            add("annotation-provenance", rel, 0, "exact not-independent-human-double-labelling limitation missing")
        reports = json.loads(_read(root, "data/evidence/reports43.json"))
        if annotation.get("status") != reports["verification"]["status"]:
            add("annotation-provenance", rel, 0, "annotation status differs from reports43.json verification")
        sheets = ["docs/labelling/labels_Anas_completed.csv", "docs/labelling/labels_Kai_completed.csv"]
        if annotation.get("completed_sheets") != sheets:
            add("annotation-provenance", rel, 0, "sidecar must reference both completed labelling sheets")
        expected = {
            "agreement": "docs/labelling/agreement.json",
            "adjudications": "docs/labelling/adjudications.csv",
        }
        for key, path in expected.items():
            if annotation.get(key) != path:
                add("annotation-provenance", rel, 0, f"sidecar {key} path must be {path}")
        for path in sheets + list(expected.values()):
            if not (Path(root) / path).is_file():
                add("annotation-provenance", rel, 0, f"referenced annotation evidence missing: {path}")
    except (AttributeError, KeyError, OSError, TypeError, ValueError) as e:
        add("annotation-provenance", rel, 0, f"unreadable or malformed annotation provenance: {e!r}")


def check(root):
    f = []

    def add(rule, file, line, msg):
        f.append(Finding(rule, file, line, msg))

    # --- required files ----------------------------------------------------
    for rel in DOCS + [RESULTS]:
        if _read(root, rel) is None:
            add("missing-file", rel, 0, "required file missing")

    # --- current final figures ---------------------------------------------
    fig = final_figures(root)
    if not fig or not all(fig.values()):
        add("current-figures", RESULTS, 0, f"cannot parse final seven-programme figures: {fig}")
    else:
        for rel in FIG_FILES:
            t = _read(root, rel)
            if t is None:
                continue
            for k, tok in fig.items():
                if k == "cervical_fc" and rel == "docs/PITCH.md":
                    continue  # PITCH does not quote cervical
                if not has_num(t, tok):
                    add("current-figures", rel, 0, f"missing final {k} = {tok}")
        for rel in DOCS:
            check_claims(root, rel, fig, add)
        # WRITEUP needs a current (non-superseded) 075abc5 row carrying the final figures.
        rows = [l for l in _lines(root, WRITEUP) if "075abc5" in l and not historic_row(l)
                and l.lstrip().startswith("|")]
        need = (fig["crc_fc"], fig["wrong_patient"], fig["no_consent"], fig["hours"], fig["recall"])
        if not any(all(has_num(l, x) for x in need) for l in rows):
            add("current-figures", WRITEUP, 0, "no current 075abc5 row with the final figures")

    # --- stale final-looking figures ---------------------------------------
    for rel in DOCS:
        for n, line in enumerate(_lines(root, rel), 1):
            if historic_row(line):
                continue
            for s in sentences(line):
                hit = [x for x in STALE if has_num(s, x)]
                if hit and not SUPERSEDE_MARK.search(s):
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
        if not FAIR_PREP.search(t):
            add("accuracy-claim", rel, 0, "missing fair -> inadequate_prep fix")
    for rel in DOCS:
        for n, line in enumerate(_lines(root, rel), 1):
            if "automatic decisions" in line and "19 correct" not in line:
                add("accuracy-claim", rel, n, "automatic-decision count is not 19 correct of 20")

    # --- stale pending-labelling statements --------------------------------
    for rel in DOCS:
        for n, line in enumerate(_lines(root, rel), 1):
            for s in sentences(line):
                if STALE_LAB.search(s) and not (HELDOUT_CTX.search(s) and not re.search(r"\b43\b|MTSamples", s)):
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
            for m in OVERCLAIM.finditer(line):
                if not negated(line, m.start()):
                    add("overclaim", rel, n, "clinical/validated/deployed claim without negation")
    check_annotation_provenance(root, add)
    check_kpi_evidence(root, add)
    check_home_paths(root, add)
    return f


KPI_BAD_WASL = re.compile(r"\b32\.0\s*%")
KPI_WITHIN_ONE = re.compile(r"within (?:about |roughly |~)?(?:one|1)(?:\.0)? (?:percentage[- ])?(?:point|pp)\b", re.I)
KPI_QUALIFIER = re.compile(r"seed[- ]2026|v1|noise|five[- ]seed|frozen", re.I)
KPI_VALUES = ("12.4%", "33.0%", "31.9%")


def check_kpi_evidence(root, add):
    """Seed-2026 KPI values are 12.4 / 33.0 / 31.9 (1.1-point gap); the five-seed frozen gap is
    about 1 point (v1) and 3.1 points (v2). 32.0% and unqualified 'within one point' must not return."""
    for rel in DOCS:
        for n, line in enumerate(_lines(root, rel), 1):
            if KPI_BAD_WASL.search(line):
                add("kpi-evidence", rel, n, "32.0% is not the seed-2026 Wasl value (31.9%)")
            for m in KPI_WITHIN_ONE.finditer(line):
                if not KPI_QUALIFIER.search(line[max(0, m.start() - 150):m.end() + 150]):
                    add("kpi-evidence", rel, n, "unqualified 'within one point' claim; name seed 2026 or the v1/v2 five-seed result")
    for rel in KPI_DOCS:
        t = _read(root, rel) or ""
        for v in KPI_VALUES:
            if v not in t:
                add("kpi-evidence", rel, 0, f"missing seed-2026 KPI value {v}")
        if not KPI_SEED_GAP_NEAR.search(t):
            add("kpi-evidence", rel, 0, "missing seed-2026 1.1-point gap (seed 2026 with '1.1 points' within 300 characters)")
        if not KPI_V1_GAP.search(t):
            add("kpi-evidence", rel, 0, "missing five-seed v1 'about 1 point under v1' wording")
        if not KPI_V2_GAP.search(t):
            add("kpi-evidence", rel, 0, "missing five-seed v2 '3.1 points under independent v2' wording")


# A developer's absolute home-directory path (macOS /Users/<name>/ or Linux /home/<name>/) must not be
# committed; write repo-relative paths or the <repo>/... and <home>/... placeholders instead. Generic CI and
# container homes are allowed. The name needs at least two characters, so test fixtures such as /home/u stay legal.
HOME_PATH = re.compile(r"/(?:Users|home)/(?!(?:runner|user|node)/)[a-z][a-z0-9_.-]+/")
# Byte-frozen fresh4 evaluation records. Their SHA-256 values are bound into the frozen labels commit
# (data/fresh4/FREEZE.json) and the PROTOCOL_PROTECTED templates in scripts/fresh4_eval/fresh4.py, so they
# record exactly what each labeller/adjudicator Agent was sent and cannot be rewritten in place.
HOME_PATH_FROZEN = frozenset((
    "data/fresh4/adjudication_prompt.md",
    "data/fresh4/labels/labeller_a_prompt.md",
    "data/fresh4/labels/labeller_b_failed_attempt_1_prompt.md",
    "data/fresh4/labels/labeller_b_prompt.md",
    "scripts/fresh4_eval/ADJUDICATOR-PROMPT-TEMPLATE.md",
    "scripts/fresh4_eval/AUTHOR-PROMPT.md",
    "scripts/fresh4_eval/LABELLER-PROMPT-TEMPLATE.md",
))


def tracked_files(root):
    """Tracked paths under root, or [] when root is not a Git checkout (e.g. a test fixture)."""
    try:
        out = subprocess.run(["git", "ls-files", "-z"], cwd=root, capture_output=True, check=True).stdout
    except (OSError, subprocess.CalledProcessError):
        return []
    return [p for p in out.decode("utf-8", "surrogateescape").split("\0") if p]


def check_home_paths(root, add, rels=None):
    for rel in tracked_files(root) if rels is None else rels:
        if rel in HOME_PATH_FROZEN:
            continue
        p = Path(root) / rel
        try:
            if not p.is_file() or p.is_symlink():
                continue
            data = p.read_bytes()
        except OSError:
            continue
        if b"\0" in data[:8192]:
            continue  # binary
        for n, line in enumerate(data.decode("utf-8", "replace").splitlines(), 1):
            if HOME_PATH.search(line):
                add("home-path", rel, n, "absolute developer home path; use a repo-relative path or <repo>/ or <home>/")


KPI_DOCS = ("README.md", "docs/SUBMISSION-DRAFT.md", "docs/PITCH.md", "docs/WRITEUP.md",
            "docs/BRIEF.md", "docs/DEMO-RECORDING-GUIDE.md")
# A seed-2026 mention must sit within 300 characters of the 1.1-point gap (either order).
KPI_SEED_GAP_NEAR = re.compile(r"seed[- ]2026(?:(?!\n).){0,300}?(?<![\d.])1\.1[- ]points?\b"
                               r"|(?<![\d.])1\.1[- ]points?\b(?:(?!\n).){0,300}?seed[- ]2026", re.I)
KPI_V1_GAP = re.compile(r"about 1 point under v1\b")
KPI_V2_GAP = re.compile(r"(?<![\d.])3\.1 (?:points )?under (?:independent )?v2\b")


def main(argv):
    root = Path(argv[1]) if len(argv) > 1 else Path(__file__).resolve().parent.parent
    found = check(root)
    for x in found:
        print(f"[{x.rule}] {x.file}:{x.line}: {x.msg}")
    print(f"{len(found)} finding(s)")
    return 1 if found else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
