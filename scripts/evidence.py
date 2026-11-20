#!/usr/bin/env python3
"""Evidence pack generator (stdlib only).

Reads the frozen held-out evaluation (pipeline/results/final/summary.json, its manifest and the per-seed
report7.json files), the deployed demo run (docs/api-samples/dashboard.json) and, when present, the public
benchmark summary (pipeline/results/benchmarks/summary.json). Writes:

  web/data/evidence.json   the shared contract read by the web /evidence page
  docs/EVIDENCE.md         the same data as plain tables
  docs/evidence/*.svg      charts of the findings
  README.md                the claim ledger between the evidence:ledger markers (if the markers exist)

It computes no new evaluation: every number is a value from those files, an arithmetic transform of one
(1 - x, x * 1000, mean and t-interval over seeds exactly as scripts/final_aggregate.py does), or a cited
parameter listed in PARAMETERS. scripts/test_evidence.py checks that.

Usage: python3 scripts/evidence.py [--check]   (--check: exit 1 if any output would change)
"""
import glob
import json
import math
import os
import sys
from statistics import mean, stdev

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
FINAL = "pipeline/results/final"
SUMMARY = FINAL + "/summary.json"
DEMO = "docs/api-samples/dashboard.json"
BENCH = "pipeline/results/benchmarks/summary.json"
OUT_JSON = "web/data/evidence.json"
OUT_MD = "docs/EVIDENCE.md"
OUT_SVG = "docs/evidence"
T95 = {1: 12.706, 2: 4.303, 3: 3.182, 4: 2.776, 5: 2.571, 6: 2.447, 7: 2.365, 8: 2.306, 9: 2.262}  # final_aggregate.py
MODELLED = "modelled, conditional on navigation acting on the corrected list"

# Programme closure metrics in summary.json: (label, precision key, recall key, safety key)
PROGRAMMES = [
    ("Colorectal (CRC)", "crc_precision", "crc_recall", "crc_false_closures"),
    ("Cervical", "cervical_precision", "cervical_recall", "cervical_false_closures"),
    ("Incidental lung nodule", "lung_precision", "lung_recall", "lung_false_closures"),
    ("Diabetic retinopathy", "dr_precision", "dr_recall", "dr_false_closures"),
    ("Breast recall", "breast_precision", "breast_recall", "breast_false_closures"),
    ("Lung screening (LDCT)", "ldct_precision", "ldct_recall", "ldct_false_closures"),
    ("IFHAS abnormal result", "ifhas_precision", "ifhas_recall", "ifhas_false_closures"),
]
# Safety rows after the programme false closures, in table order: (label, summary.json safety key)
SAFETY_EXTRA = [
    ("DR referral-loop false closures", "dr_referral_false_closures"),
    ("Breast recall-loop false closures", "breast_recall_false_closures"),
    ("LDCT action-loop false closures", "ldct_action_false_closures"),
    ("IFHAS follow-up-loop false closures", "ifhas_followup_false_closures"),
    ("Lung surveillance-CT false closures", "lung_surveillance_ct_false_closures"),
    ("Rejected-referral hand-off false closures", "handoff_false_closures"),
    ("Wrong-patient messages released", "wrong_patient_released"),
    ("Messages released without granted consent", "released_without_granted_consent"),
    ("Duplicate or lost pipeline outputs (process and broker kills)", "duplicate_or_lost_outputs"),
    ("Forged events with any effect", "forged_with_effect"),
    ("Tamper attacks not detected", None),  # tamper_detected: [detected, attacks]; events = attacks - detected
]

# Cited parameters (from the evidence-plan report, Documents/reports/Proving hackathon impact and accuracy.md).
SEHA_URL = "https://pmc.ncbi.nlm.nih.gov/articles/PMC8343888/"
SELBY_URL = "https://doi.org/10.1177/0969141320904977"
PARAMETERS = [
    {"name": "FIT+ patients with a follow-up colonoscopy, SEHA ambulatory clinics 2015-16", "value": "30.5%", "scope": "UAE",
     "source": "Abu Dhabi SEHA cohort, BMC Cancer 2021", "url": SEHA_URL},
    {"name": "31-day standard: colonoscopy within 31 days of referral after FIT+", "value": "31 days", "scope": "UAE",
     "source": "DoH colorectal cancer screening specification, Appendix 2", "url": "https://www.doh.gov.ae/-/media/980F1D8087214B6D9E630B457B85E4B9.ashx"},
    {"name": "Navigation uplift in colonoscopy completion (low)", "value": "3 pp", "scope": "INTL",
     "source": "Selby et al. 2021, J Med Screen: about 3 points per extra intervention", "url": SELBY_URL},
    {"name": "Navigation uplift in colonoscopy completion (high)", "value": "11 pp", "scope": "INTL",
     "source": "Selby et al. 2021, J Med Screen: programmes with navigators, 6-month completion", "url": SELBY_URL},
    {"name": "CRC found per completed colonoscopy after FIT+ (low)", "value": "2.5%", "scope": "UAE",
     "source": "SEHA cohort, 11 cancers in 436 colonoscopies (team's arithmetic)", "url": SEHA_URL},
    {"name": "CRC found per completed colonoscopy after FIT+ (high)", "value": "5.5%", "scope": "INTL",
     "source": "Korean national programme", "url": "https://pmc.ncbi.nlm.nih.gov/articles/PMC5832343/"},
    {"name": "Positive FITs per year in the illustrative scenarios", "value": "1,000-10,000", "scope": "ASSUMED",
     "source": "Illustrative. SEHA had about 685 positives a year in one network (team's annualisation); ADPHC reports screenings up more than 280% in 2025",
     "url": "https://menafn.com/1111766961/Abu-Dhabi-Records-More-Than-280-Increase-in-Cancer-Screenings-and-Strengthens-Adoption-of-Preventive-Health-Behaviours"},
    {"name": "Navigator minutes per chase contact", "value": "5-20 min", "scope": "ASSUMED",
     "source": "No Abu Dhabi figure exists; varied as an assumption", "url": ""},
    {"name": "Malaffi unique patient profiles vs emirate population", "value": "14 million vs 4.14 million", "scope": "UAE",
     "source": "Malaffi progress report; SCAD 2024 population. Suggests identity fragmentation (inference, not a duplicate rate)",
     "url": "https://www.malaffi.ae/malaffi-progress-report/"},
]
UPLIFT_PP = (3, 11)       # parameters 3 and 4
PPV = (0.025, 0.055)      # parameters 5 and 6
POSITIVES = (1000, 10000)  # parameter 7
CHASE_MIN = (5, 20)       # parameter 8

CLAIMS = [
    ("C1", "Emirates-ID-only matching under-counts 31-day colonoscopy completion; Wasl raises the measured rate toward the true rate",
     "simulated", "pipeline/results/final/SUMMARY.md (kpi_eid_only, kpi_wasl, kpi_truth)", "make verify SEED=<n>; full run: scripts/final_eval.sh"),
    ("C2", "No false closures in any of the seven programmes in the frozen held-out evaluation (for example CRC 0/3,035 under v2 noise, 95% upper bound 0.121%)",
     "simulated", "docs/EVIDENCE.md#safety; pipeline/results/final/summary.json", "make verify SEED=<n>"),
    ("C3", "Thresholds, error definitions and metric code were frozen at 075abc5ad04a on 2026-11-02T04:33Z before the held-out runs",
     "built", "pipeline/results/final/manifest.json; scripts/final_eval.sh; scripts/final_manifest.py", "git show 075abc5ad04a --stat; python3 -m json.tool pipeline/results/final/manifest.json"),
    ("C4", "Links records across Arabic and Latin spellings with a self-trained name encoder, HNSW candidates and a calibrated scorer",
     "built", "encoder/RESULTS.md (held-out Wikidata eval split); pipeline/linker.go; pipeline/ann.go", "cd pipeline && go test ./..."),
    ("C5", "A loop closes only when DoH rules and a model reader agree on the report; anything uncertain goes to human review",
     "built", "pipeline/v7.go; pipeline/evidence.go; pipeline/RESULTS.md", "scripts/demo.sh test"),
    ("C6", "Ingests FHIR R4 and HL7 v2.5.1 over MLLP into one event model",
     "built", "pipeline/hl7.go; pipeline/hl7_test.go", "cd pipeline && go test -run MLLP ./... && go test -run HL7 ./..."),
    ("C7", "Exactly-once processing under process and broker kills: 0 duplicate or lost outputs in all frozen runs",
     "simulated", "docs/EVIDENCE.md#safety (duplicate_or_lost_outputs)", "make verify SEED=<n>"),
    ("C8", "Hash-chained provenance detects tampering: 85/85 attacks detected, 0 false alarms per noise model",
     "simulated", "pipeline/results/final/*/tamper.json", "scripts/demo.sh tamper"),
    ("C9", "No message is released without granted contact consent (0/11,650 under v2 noise)",
     "simulated", "docs/EVIDENCE.md#safety", "make verify SEED=<n>"),
    ("C10", "Seven loop types run on the same engine (CRC, cervical, lung nodule, DR, breast, LDCT, IFHAS)",
     "built", "pipeline/results/final/SUMMARY.md; docs/BACKEND-HANDOFF.md (Seven loop types)", "WASL_LDCT=300 WASL_IFHAS=800 scripts/demo.sh final"),
    ("C11", "Review load by name origin is measured: pooled max/min 1.05 under v1 (p = 0.451) and 1.12 under v2 (p = 0.007); 1.07 under v2 after standardising by programme (p = 0.185)",
     "simulated", "docs/EVIDENCE.md#worst-seed-and-worst-stratum", "make verify SEED=<n>"),
    ("C12", "Hosted demo with a staff worklist, case evidence and audit trail on synthetic data",
     "built", "https://wasl-topaz-mu.vercel.app (staff area /app); docs/LIVE.md", "Open the web app, then /app, then any case and its audit trail"),
    ("C13", "Queue control (direct-to-procedure plus prep targeting) adds about +13 pp 31-day completion at 1.2 slots per FIT+",
     "simulated", "booking/RESULTS.md", "scripts/demo.sh booking"),
    ("C14", "Extra colonoscopies and cancers found if navigation acts on the corrected list (" + MODELLED + ")",
     "designed", "docs/EVIDENCE.md#modelled-benefit", "python3 scripts/evidence.py"),
]

CHECKLIST = [
    ("TRIPOD+AI 18e", "Data availability", "Synthetic cohorts are regenerated from seeds; persons from Wikidata (CC0)", "scripts/final_eval.sh; data/persons3.json; pipeline/results/final/manifest.json (hashes)"),
    ("TRIPOD+AI 18f", "Code availability", "All code in this repository", "pipeline/; scripts/"),
    ("TRIPOD+AI 22", "Model availability", "Model artefacts and their SHA-256 hashes", "models/; pipeline/results/final/manifest.json"),
    ("TRIPOD+AI participants, outcome ascertainment", "Real participants and outcomes", "Not applicable: synthetic only. No real patients", "pipeline/results/final/manifest.json (limitations)"),
    ("DECIDE-AI 6", "Significant errors defined in advance and all observed errors listed", "False closure defined in code before the frozen run; every safety count listed", "docs/EVIDENCE.md#error-definitions; #safety"),
    ("DECIDE-AI human factors, clinical use", "Users and setting in live use", "Not applicable: synthetic only; reviewers are simulated", "docs/BRIEF.md (pilot plan)"),
    ("NeurIPS 7", "Error bars, their method and the source of variation", "95% t-intervals over 5 seeds (generator variance); Clopper-Pearson for zero-event counts", "scripts/final_aggregate.py; docs/EVIDENCE.md"),
    ("NeurIPS 4-5", "Reproducibility; open data and code", "One command per result", "docs/EVIDENCE.md (claim ledger); make verify"),
]


def read(p):
    with open(p, encoding="utf-8") as f:
        return f.read()


def rel(p):
    return os.path.join(ROOT, p)


def load(p):
    with open(rel(p), encoding="utf-8") as f:
        return json.load(f)


def ci(xs):
    """Mean and 95% t-interval over seeds, rounded as scripts/final_aggregate.py does."""
    m, s = mean(xs), stdev(xs)
    h = T95.get(len(xs) - 1, 2.0) * s / math.sqrt(len(xs))
    return [round(m, 4), round(m - h, 4), round(m + h, 4)]


def r(x, n=4):
    return None if x is None else round(x, n)


def per_seed(noise):
    """Per-seed values read from pipeline/results/final/<noise>-s<seed>/report7.json."""
    out = []
    for p in sorted(glob.glob(rel(f"{FINAL}/{noise}-s*/report7.json"))):
        with open(p, encoding="utf-8") as f:
            rep = json.load(f)
        k = rep["crc_kpi_31d"]
        out.append({
            "seed": int(os.path.basename(os.path.dirname(p)).split("-s")[1]),
            "pair_precision": rep["linkage"]["pair_precision"], "pair_recall": rep["linkage"]["pair_recall"],
            "false_merge_pairs": rep["linkage"]["false_merge_pairs"],
            "crc_recall": rep["closure"]["crc"]["wasl_live_with_review"]["recall"],
            "eid": k["eid_only"]["kpi_all_referrals"], "wasl": k["wasl_live"]["kpi_all_referrals"], "truth": k["truth"]["kpi_all_referrals"],
        })
    return out


def per_1000(eid, wasl, truth, tag):
    """The measured table per 1,000 positive FITs, from three 31-day completion rates (fractions)."""
    rows = [
        ("Completions seen within 31 days", 1000 * eid, 1000 * wasl, 1000 * truth, "per 1,000 positive FITs"),
        ("Share of true completions seen", 100 * eid / truth, 100 * wasl / truth, 100.0, "%"),
        ("Patients wrongly flagged 'not followed up'", 1000 * (truth - eid), 1000 * (truth - wasl), 0.0, "per 1,000 positive FITs"),
        ("Worklist size (patients to chase)", 1000 * (1 - eid), 1000 * (1 - wasl), 1000 * (1 - truth), "per 1,000 positive FITs"),
        ("Worklist precision (share of listed patients truly not done)", 100 * (1 - truth) / (1 - eid), 100 * (1 - truth) / (1 - wasl), None, "%"),
    ]
    return [{"row": f"{name} ({tag})", "eid_only": r(a, 1), "wasl": r(b, 1), "truth": r(c, 1), "unit": u} for name, a, b, c, u in rows]


def build():
    S = load(SUMMARY)
    man = S["manifest"]
    demo = load(DEMO)["metrics"]["crc_kpi_31d"]
    bench = load(BENCH) if os.path.exists(rel(BENCH)) else []
    N = S["by_noise"]
    seeds = {n: per_seed(n) for n in ("v1", "v2")}

    # ---- impact
    imp = []
    for n in ("v2", "v1"):  # v2 (independent noise) first: the headline
        m = N[n]["metrics"]
        imp += per_1000(m["kpi_eid_only"][0], m["kpi_wasl"][0], m["kpi_truth"][0], f"frozen {n} five-seed mean")
    imp += per_1000(demo["eid_only"]["kpi_all_referrals"], demo["wasl_live"]["kpi_all_referrals"], demo["truth"]["kpi_all_referrals"], "deployed demo run")
    scen = [{"name": "worst", "uplift_pp": 0, "ppv": PPV[0], "extra_colonoscopies_per_1000": 0, "cancers_found_per_1000": 0}]
    for name, up, ppv in (("base", UPLIFT_PP[0], PPV[0]), ("best", UPLIFT_PP[1], PPV[1])):
        scen.append({"name": name, "uplift_pp": up, "ppv": ppv, "extra_colonoscopies_per_1000": up * 10,
                     "cancers_found_per_1000": r(up * 10 * ppv, 2)})
    base = UPLIFT_PP[0] * 10 * PPV[0]  # cancers found per 1,000 positives at the base case

    def c1k(up, ppv):
        return r(up * 10 * ppv, 2)
    tornado = [
        {"input": "Navigation uplift (INTL)", "low": "3 pp", "high": "11 pp",
         "out_low": c1k(UPLIFT_PP[0], PPV[0]), "out_high": c1k(UPLIFT_PP[1], PPV[0])},
        {"input": "CRC found per colonoscopy (UAE low, INTL high)", "low": "2.5%", "high": "5.5%",
         "out_low": c1k(UPLIFT_PP[0], PPV[0]), "out_high": c1k(UPLIFT_PP[0], PPV[1])},
    ]
    impact = {
        "per_1000": imp,
        "calibration": {"label": "Abu Dhabi FIT+ patients with a follow-up colonoscopy (SEHA ambulatory clinics, 2015-16)", "value": 0.305,
                        "source": "BMC Cancer 2021", "url": SEHA_URL,
                        "caveat": "No fixed 31-day window (median 72 days), and only colonoscopies inside SEHA's own network were counted, so it may "
                                  "undercount completions done elsewhere. The simulated truth sitting near it is a plausibility check, not a validation."},
        "modelled": {"label": "Modelled benefit, " + MODELLED + ". Worst case is the measurement benefit alone. Tornado output: cancers found per 1,000 positive FITs, "
                              f"base case {round(base, 2)} (3 pp uplift, 2.5%). Per year = per 1,000 x positive FITs a year / 1,000 (1,000-10,000 a year, ASSUMED).",
                     "scenarios": scen, "tornado": tornado, "parameters": PARAMETERS},
    }

    # ---- accuracy
    definitions = [
        {"term": "False closure (primary safety error)", "meaning": "A loop Wasl's final live decision marks closed while the simulator's hidden truth says it is not closed. "
         "Counted per programme on loops Wasl closed (report7 closure.<programme>.wasl_live_with_review.false_closures; pipeline/report7.go closureStatsDecisions, "
         "pipeline/analyze.go closureStatsProg), pooled over seeds, with a Clopper-Pearson 95% upper bound (scripts/final_aggregate.py). The frozen run counts false "
         "outcomes only; the later evidence-ownership check (#193) is not part of it. The patient drops off the worklist, so this is the dangerous error."},
        {"term": "False open (missed closure)", "meaning": "A loop the truth says is closed but Wasl leaves open (closure 'missed'). Rate = 1 - closure recall. "
         "It creates extra review or chase work; the patient stays visible."},
        {"term": "Wrong-patient merge", "meaning": "Two registrations of different people placed in one linker cluster (report7 linkage.false_merge_pairs; pipeline/audit.go "
         "linkMetrics). Pair false-match rate = 1 - pair precision. Its clinical consequences are measured separately as false closures and as "
         "wrong-patient messages released (an outreach message whose triggering record belongs to someone other than the loop's patient; pipeline/sweep.go govMetrics)."},
        {"term": "Missed match", "meaning": "Two registrations of the same person left in different clusters. Pair missed-match rate = 1 - pair recall over all true same-person pairs."},
        {"term": "Closure false-match / missed-match rate", "meaning": "The same two rates at the level of each programme's loops: 1 - closure precision and 1 - closure recall, "
         "with the evaluation's 95% t-intervals over five seeds."},
        {"term": "Review band", "meaning": "Items routed to a human reviewer instead of an automatic decision, per 100 loops, and simulated reviewer hours per 1,000 loops."},
        {"term": "Pre-registration", "meaning": f"Code revision {man['code_revision'][:12]} with thresholds, policy {man['policy']}, model and data hashes and seeds "
         f"{man['seeds']} recorded in pipeline/results/final/manifest.json, frozen {man['frozen_at'][:16]}Z before the held-out runs. "
         "v2 noise was never used for training, calibration or rule design; the v7 rules were designed on earlier v1-noise seeds."},
    ]
    linkage, safety, worst = [], [], []
    for n in ("v1", "v2"):
        m = N[n]["metrics"]
        for label, pk, rk, _ in PROGRAMMES:
            p, rc = m[pk], m[rk]
            linkage.append({"noise": n, "metric": f"{label}: closure false-match rate (1 - precision)", "value": r(1 - p[0]), "low": r(1 - p[2]), "high": r(1 - p[1])})
            linkage.append({"noise": n, "metric": f"{label}: closure missed-match rate (1 - recall)", "value": r(1 - rc[0]), "low": r(1 - rc[2]), "high": r(1 - rc[1])})
        pp, pr = ci([s["pair_precision"] for s in seeds[n]]), ci([s["pair_recall"] for s in seeds[n]])
        linkage.append({"noise": n, "metric": "Linkage pairs: false-match rate (1 - pair precision)", "value": r(1 - pp[0]), "low": r(1 - pp[2]), "high": r(1 - pp[1])})
        linkage.append({"noise": n, "metric": "Linkage pairs: missed-match rate (1 - pair recall)", "value": r(1 - pr[0]), "low": r(1 - pr[2]), "high": r(1 - pr[1])})
        for k, label in (("review_items_per_100_loops", "Review band: review items per 100 loops"), ("reviewer_h_per_1k", "Review band: reviewer hours per 1,000 loops")):
            v = m[k]
            linkage.append({"noise": n, "metric": label, "value": v[0], "low": v[1], "high": v[2]})
        f1 = m["linkage_pair_f1"]
        linkage.append({"noise": n, "metric": "Linkage pair F1 (not comparable across linkers)", "value": f1[0], "low": f1[1], "high": f1[2]})

        sp = N[n]["safety_pooled"]
        for label, _, _, sk in PROGRAMMES:
            ev, den, ub = sp[sk]
            safety.append({"metric": f"[{n}] {label} false closures", "events": ev, "denominator": den, "upper_95": ub})
        for label, sk in SAFETY_EXTRA:
            if sk is None:
                det, att, _ = sp["tamper_detected"]
                safety.append({"metric": f"[{n}] {label}", "events": att - det, "denominator": att, "upper_95": None})
                continue
            ev, den, ub = sp[sk]
            safety.append({"metric": f"[{n}] {label}", "events": ev, "denominator": den, "upper_95": ub})
        safety.append({"metric": f"[{n}] Wrong-patient merge pairs in linker clusters (not zero; see definitions)",
                       "events": sum(s["false_merge_pairs"] for s in seeds[n]), "denominator": None, "upper_95": None})

        sd = seeds[n]
        lo = min(sd, key=lambda s: s["crc_recall"]); hi = max(sd, key=lambda s: s["crc_recall"])
        worst.append({"label": f"[{n}] CRC closure recall, worst and best seed", "value": f"{lo['crc_recall']} (seed {lo['seed']}) to {hi['crc_recall']} (seed {hi['seed']})"})
        lo = min(sd, key=lambda s: s["pair_recall"]); hi = max(sd, key=lambda s: s["pair_recall"])
        worst.append({"label": f"[{n}] Linkage pair recall, worst and best seed", "value": f"{lo['pair_recall']} (seed {lo['seed']}) to {hi['pair_recall']} (seed {hi['seed']})"})
        g = max(sd, key=lambda s: abs(s["truth"] - s["wasl"]))
        worst.append({"label": f"[{n}] Largest Wasl 31-day KPI gap to truth, by seed", "value": f"{round(abs(g['truth'] - g['wasl']) * 100, 1)} points (seed {g['seed']}: Wasl {g['wasl']}, truth {g['truth']}, EID-only {g['eid']})"})
        sub = N[n]["crc_by_age_band_and_sex"]
        for dim, name in (("age_band", "age band"), ("sex", "sex")):
            rec = sub[dim]["crc_recall"]
            w = min(rec, key=rec.get)
            worst.append({"label": f"[{n}] CRC recall by {name}, worst stratum", "value": f"{w}: {rec[w]} (range {min(rec.values())} to {max(rec.values())}); false closures 0 in every stratum"})
        ro = N[n]["review_load_by_origin"]
        pi = ro["pooled_items_per_100"]
        w = max(pi, key=pi.get)
        worst.append({"label": f"[{n}] Review load by name origin, heaviest group", "value": f"{w}: {pi[w]} items per 100 loops (max/min {ro['pooled_ratio']}, p = {ro['pooled_p_value']}; by programme {ro['standardised']['ratio']}, p = {ro['standardised']['p_value']})"})

    gen = {"revision": man["code_revision"], "frozen_at": man["frozen_at"], "source": SUMMARY, "generated_by": "scripts/evidence.py"}
    claims = [{"id": i, "claim": c, "status": s, "evidence": e, "reproduce": rp} for i, c, s, e, rp in CLAIMS]
    data = {"generated": gen, "impact": impact,
            "accuracy": {"definitions": definitions, "linkage": linkage, "safety": safety, "worst": worst},
            "benchmarks": bench, "claims": claims}
    return data, seeds


# ---------------------------------------------------------------- SVG (stdlib)
INK, INK2, MUTED, GRID, SURF = "#0b0b0b", "#52514e", "#8a8984", "#e4e3df", "#fcfcfb"
C_WASL, C_EID, C_TRUTH = "#2a78d6", "#eb6834", "#1baf7a"  # categorical slots 1-3 (validated all-pairs)


def esc(s):
    return str(s).replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;").replace('"', "&quot;")


def svg(w, h, body, title, desc):
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {w} {h}" width="{w}" height="{h}" role="img" '
            f'font-family="system-ui, -apple-system, Segoe UI, sans-serif">\n<title>{esc(title)}</title><desc>{esc(desc)}</desc>\n'
            f'<rect width="{w}" height="{h}" fill="{SURF}"/>\n'
            f'<text x="16" y="26" font-size="15" font-weight="600" fill="{INK}">{esc(title)}</text>\n'
            f'<text x="16" y="46" font-size="12" fill="{INK2}">{esc(desc)}</text>\n' + body + "</svg>\n")


def txt(x, y, s, size=12, fill=INK2, anchor="start", weight="400"):
    return f'<text x="{x:.1f}" y="{y:.1f}" font-size="{size}" fill="{fill}" text-anchor="{anchor}" font-weight="{weight}">{esc(s)}</text>\n'


def bar(x, y, w, h, fill, tip):
    w = max(w, 0.5)
    return f'<rect x="{x:.1f}" y="{y:.1f}" width="{w:.1f}" height="{h:.1f}" rx="2" fill="{fill}"><title>{esc(tip)}</title></rect>\n'


def legend(x, y, items):
    out = ""
    for label, col in items:
        out += f'<rect x="{x}" y="{y - 9}" width="10" height="10" rx="2" fill="{col}"/>' + txt(x + 15, y, label, 12, INK2)
        x += 22 + 7 * len(label)
    return out


def chart_impact(data):
    rows = [x for x in data["impact"]["per_1000"] if x["row"].startswith("Completions seen")]
    W, L, R = 760, 200, 60
    y, body = 92, legend(L, 70, [("Emirates ID only", C_EID), ("Wasl", C_WASL), ("Simulated truth", C_TRUTH)])
    scale = (W - L - R) / 400
    for row in rows:
        tag = row["row"].split("(")[-1].rstrip(")")
        body += txt(16, y + 24, tag, 12, INK)
        for key, col, lab in (("eid_only", C_EID, "EID only"), ("wasl", C_WASL, "Wasl"), ("truth", C_TRUTH, "Truth")):
            v = row[key]
            body += bar(L, y, v * scale, 14, col, f"{tag}, {lab}: {v} per 1,000") + txt(L + v * scale + 6, y + 11, f"{v:g}", 11, INK)
            y += 17
        y += 16
    body += f'<line x1="{L}" y1="86" x2="{L}" y2="{y - 12}" stroke="{MUTED}"/>\n'
    return svg(W, y + 8, body, "31-day completions seen per 1,000 positive FITs",
               "Emirates-ID-only matching sees about a third of true completions; Wasl sees most of them. Synthetic cohorts.")


def chart_missed(data):
    rows = [x for x in data["accuracy"]["linkage"] if "missed-match" in x["metric"]]
    names = []
    for x in rows:
        nm = x["metric"].split(":")[0]
        if nm not in names:
            names.append(nm)
    W, L, R = 760, 220, 40
    xmax = 0.25
    sc = (W - L - R) / xmax
    y0 = 90
    body = legend(L, 70, [("v1 noise (training-time)", C_WASL), ("v2 noise (independent)", C_EID)])
    for t in (0, 0.05, 0.10, 0.15, 0.20, 0.25):
        x = L + t * sc
        body += f'<line x1="{x:.1f}" y1="{y0 - 6}" x2="{x:.1f}" y2="{y0 + 28 * len(names)}" stroke="{GRID}"/>' + txt(x, y0 + 28 * len(names) + 16, f"{t * 100:.0f}%", 11, MUTED, "middle")
    for i, nm in enumerate(names):
        y = y0 + 28 * i + 10
        body += txt(16, y + 4, nm, 12, INK)
        for j, (noise, col) in enumerate((("v1", C_WASL), ("v2", C_EID))):
            x = next(z for z in rows if z["noise"] == noise and z["metric"].startswith(nm))
            yy = y - 4 + 8 * j
            lo, hi = max(x["low"], 0), min(x["high"], xmax)
            body += f'<line x1="{L + lo * sc:.1f}" y1="{yy}" x2="{L + hi * sc:.1f}" y2="{yy}" stroke="{col}" stroke-width="2"/>'
            body += (f'<circle cx="{L + x["value"] * sc:.1f}" cy="{yy}" r="4" fill="{col}" stroke="{SURF}" stroke-width="2">'
                     f'<title>{esc(nm)}, {noise}: {x["value"] * 100:.2f}% (95% {x["low"] * 100:.2f} to {x["high"] * 100:.2f})</title></circle>\n')
    h = y0 + 28 * len(names) + 30
    return svg(W, h, body, "Missed-match rate (1 - recall) with 95% intervals",
               "Missed closures stay on the worklist. Closure false-match rate is 0 in every programme; linkage pairs are not 0 (see table).")


def chart_safety(data):
    rows = [x for x in data["accuracy"]["safety"] if x["metric"].startswith("[v2]") and x["upper_95"] is not None]
    W, L, R = 760, 330, 120
    xmax = max(x["upper_95"] for x in rows)
    sc = (W - L - R) / xmax
    y, body = 70, ""
    for x in rows:
        nm = x["metric"][5:]
        body += txt(16, y + 11, nm, 12, INK)
        body += bar(L, y, x["upper_95"] * sc, 13, C_WASL, f"{nm}: {x['events']}/{x['denominator']}, upper bound {x['upper_95'] * 100:.3f}%")
        body += txt(L + x["upper_95"] * sc + 6, y + 11, f"{x['events']}/{x['denominator']:,} (< {x['upper_95'] * 100:.2f}%)", 11, INK)
        y += 20
    return svg(W, y + 10, body, "Zero-event safety counts, v2 noise: 95% upper bound on the true rate",
               "Bars show the Clopper-Pearson upper bound; every observed count is 0. Small denominators give wide bounds.")


def chart_seeds(seeds):
    W, L, R = 760, 120, 40
    sc = (W - L - R) / 0.25
    body = legend(L, 70, [("Emirates ID only", C_EID), ("Wasl", C_WASL)])
    y = 96
    for t in (0, 0.05, 0.10, 0.15, 0.20, 0.25):
        x = L + t * sc
        body += f'<line x1="{x:.1f}" y1="{y - 8}" x2="{x:.1f}" y2="{y + 2 * 50}" stroke="{GRID}"/>' + txt(x, y + 2 * 50 + 16, f"{t * 100:.0f} pts", 11, MUTED, "middle")
    for i, n in enumerate(("v1", "v2")):
        yy = y + 50 * i + 12
        body += txt(16, yy + 4, f"{n} noise", 12, INK)
        for j, s in enumerate(seeds[n]):
            for key, col, off in (("eid", C_EID, -10), ("wasl", C_WASL, 10)):
                g = abs(s["truth"] - s[key])
                body += (f'<circle cx="{L + g * sc:.1f}" cy="{yy + off + 3 * j - 6}" r="4" fill="{col}" stroke="{SURF}" stroke-width="2">'
                         f'<title>{n} seed {s["seed"]}, {key}: {g * 100:.1f} points from truth</title></circle>\n')
    return svg(W, y + 2 * 50 + 26, body, "Gap to the true 31-day rate, every held-out seed",
               "One dot per seed. The worst Wasl seed is still far closer to truth than the best EID-only seed.")


def chart_tornado(data):
    t = data["impact"]["modelled"]["tornado"]
    W, L, R = 760, 350, 50
    xmax = max(x["out_high"] for x in t)
    sc = (W - L - R) / xmax
    base = t[0]["out_low"]
    y, body = 70, ""
    for x in sorted(t, key=lambda z: z["out_high"] - z["out_low"], reverse=True):
        body += txt(16, y + 12, f"{x['input']}: {x['low']} to {x['high']}", 12, INK)
        body += bar(L + x["out_low"] * sc, y, (x["out_high"] - x["out_low"]) * sc, 16, C_WASL, f"{x['input']}: {x['out_low']} to {x['out_high']} cancers per 1,000")
        body += txt(L + x["out_high"] * sc + 6, y + 12, f"{x['out_high']:g}", 11, INK)
        y += 26
    body += f'<line x1="{L + base * sc:.1f}" y1="62" x2="{L + base * sc:.1f}" y2="{y}" stroke="{INK}" stroke-dasharray="3 3"/>' + txt(L + base * sc, y + 14, f"base {base:g}", 11, INK2, "middle")
    return svg(W, y + 24, body, "Modelled cancers found per 1,000 positive FITs: which input matters most",
               "Modelled, conditional on navigation acting on the corrected list. Not a Wasl result. Others held at base.")


# ---------------------------------------------------------------- Markdown
def pct(x, n=2):
    return f"{x * 100:.{n}f}%"


def ledger_md(claims):
    out = ["| # | Claim | Status | Evidence | Reproduce |", "|---|---|---|---|---|"]
    for c in claims:
        cell = [x.replace("|", "\\|") for x in (c["claim"], c["evidence"], c["reproduce"])]
        out.append(f"| {c['id']} | {cell[0]} | {c['status'].capitalize()} | {cell[1]} | `{cell[2]}` |")
    return "\n".join(out)


def render_md(data, seeds):
    g, imp, acc = data["generated"], data["impact"], data["accuracy"]
    L = [f"# Evidence: what Wasl measured, how, and how to check it",
         "",
         f"Generated by `scripts/evidence.py` from `{g['source']}` (code revision `{g['revision'][:12]}`, frozen {g['frozen_at'][:16]}Z). "
         "Do not edit by hand; run `python3 scripts/evidence.py`. Machine-readable copy: `web/data/evidence.json`. All patients are synthetic.",
         "",
         "## Verify it yourself",
         "",
         "- `make verify` (runs `scripts/verify.sh`, about 80 s): checks the frozen results against committed SHA-256 hashes, recomputes every aggregate from the per-seed results, runs the named safety tests, prints PASS or FAIL per claim.",
         "- `make verify SEED=<any integer>` (about 5 minutes; needs `bin/nats-server`): reruns the frozen code end to end on a fresh held-out cohort nobody has seen, reduced to 600 CRC + 240 cervical loops, and prints the accuracy and safety table.",
         "- `python3 scripts/evidence.py --check && python3 -m unittest scripts/test_evidence.py`: every number below traces to the frozen summary or a cited parameter.",
         "",
         "## Error definitions",
         "",
         "These are the definitions the frozen evaluation already used. They were fixed in code (`pipeline/report7.go`, `pipeline/analyze.go`, "
         "`pipeline/audit.go`, `scripts/final_aggregate.py`) and recorded with thresholds, model hashes and seeds in "
         f"`pipeline/results/final/manifest.json` at revision `{g['revision'][:12]}`, frozen {g['frozen_at'][:16]}Z, before the held-out runs "
         "(`scripts/final_eval.sh` refuses to run on a dirty tree). No new pre-registration is claimed. The labels on the 43 report texts were "
         "completed after the freeze; `pipeline/results/final/annotation-provenance.json` records that.",
         "",
         "| Term | Meaning |", "|---|---|"]
    L += [f"| {d['term']} | {d['meaning']} |" for d in acc["definitions"]]
    L += ["", "No published standard defines an acceptable false-closure rate for screening follow-up. Wasl's closure tier (0.99) and "
          "double human review are the team's own policy.", "",
          "## Accuracy", "",
          "False-match rate is 1 - precision, missed-match rate is 1 - recall (Harron et al. 2017). Means over five held-out seeds with 95% "
          "t-intervals; the interval reflects generator variance between seeds. F1 comes last because it is not comparable across linkers.",
          "", "![Missed-match rate by programme](evidence/missed_match.svg)", "",
          "| Metric | v1 noise (training-time) | v2 noise (independent) |", "|---|---|---|"]
    for x in acc["linkage"]:
        if x["noise"] != "v1":
            continue
        y = next(z for z in acc["linkage"] if z["noise"] == "v2" and z["metric"] == x["metric"])
        if "Review band" in x["metric"] or "F1" in x["metric"]:
            f = lambda z: f"{z['value']:g} ({z['low']:g} to {z['high']:g})"
        else:
            f = lambda z: f"{pct(z['value'])} ({pct(z['low'])} to {pct(z['high'])})"
        L.append(f"| {x['metric']} | {f(x)} | {f(y)} |")
    L += ["", "The closure false-match rate is 0 with a zero-width interval because every seed had precision 1.000; read it with the "
          "safety table's upper bounds, not as a certainty. t-intervals near 0 can dip below 0 (breast, LDCT); they are shown as the "
          "evaluation computed them. The linker does make wrong-patient merges (about 1% of linked pairs); the "
          "closure gate, a second identifier and human review stop them from closing loops or messaging the wrong person in these runs.", "",
          "### Safety", "",
          "False closures first. Pooled over five seeds; the upper bound is the two-sided Clopper-Pearson 95% bound for the true rate when 0 events were seen.",
          "", "![Zero-event safety upper bounds](evidence/safety_upper.svg)", "",
          "| Safety count | Events | Denominator | 95% upper bound |", "|---|---|---|---|"]
    for x in acc["safety"]:
        den = "–" if x["denominator"] is None else f"{x['denominator']:,}"
        ub = "–" if x["upper_95"] is None else pct(x["upper_95"], 3)
        L.append(f"| {x['metric']} | {x['events']} | {den} | {ub} |")
    L += ["", "### Worst seed and worst stratum", "",
          "![Gap to truth per seed](evidence/seed_gap.svg)", "", "| | |", "|---|---|"]
    L += [f"| {w['label']} | {w['value']} |" for w in acc["worst"]]
    L += ["", "Name-origin recall is not reported by the frozen evaluation; review load by origin is. With about 16 subgroup tests per "
          "evaluation, no highlighted p-value passes a Bonferroni cut (about 0.003).", ""]

    L += ["## Impact: measured", "",
          "Wasl changes what the programme can see, not what patients do. Per 1,000 positive FITs, from the 31-day completion rates. "
          "It assumes Wasl's detected completions are true completions, which the 0 false closures above support.",
          "", "![Completions seen per 1,000 positive FITs](evidence/impact_per_1000.svg)", "",
          "| Per 1,000 positive FITs | Emirates ID only | Wasl | Simulated truth | Unit |", "|---|---|---|---|---|"]
    for x in imp["per_1000"]:
        f = lambda v: "–" if v is None else f"{v:g}"
        L.append(f"| {x['row']} | {f(x['eid_only'])} | {f(x['wasl'])} | {f(x['truth'])} | {x['unit']} |")
    v2 = [x for x in imp["per_1000"] if "frozen v2" in x["row"]]
    flag = v2[2]
    avoided = flag["eid_only"] - flag["wasl"]
    L += ["", f"Plain reading (v2, the independent noise model): EID-only matching wrongly flags about {flag['eid_only']:.0f} of every 1,000 "
          f"positives as not followed up; Wasl about {flag['wasl']:.0f}. Wasl raises the measured rate toward the true rate. The real rate "
          "moves only if someone acts on the list.", "",
          f"Chase time spent on patients who were already done falls by {avoided:.0f} contacts per 1,000 positives; at 5 to 20 minutes per "
          f"chase (ASSUMED) that is {avoided * CHASE_MIN[0] / 60:.0f} to {avoided * CHASE_MIN[1] / 60:.0f} navigator hours per 1,000 positives.", "",
          "### Calibration point", "", f"{imp['calibration']['label']}: **{pct(imp['calibration']['value'], 1)}** "
          f"([{imp['calibration']['source']}]({imp['calibration']['url']})). The simulated truth is "
          f"{v2[0]['truth'] / 10:.1f}% (v2 mean) and {imp['per_1000'][-5]['truth'] / 10:.1f}% (deployed run); EID-only measurement is "
          f"{v2[0]['eid_only'] / 10:.1f}%. Caveats: {imp['calibration']['caveat']}", "",
          "## Modelled benefit", "",
          f"**{MODELLED.capitalize()}.** This is not a Wasl result. NICE ESF / ISPOR budget-impact shape: a range of scenarios and a tornado, not a point estimate.", "",
          "| Scenario | Navigation uplift | CRC per colonoscopy | Extra colonoscopies per 1,000 positives | Cancers found per 1,000 positives |", "|---|---|---|---|---|"]
    for s in imp["modelled"]["scenarios"]:
        label = "worst (measurement benefit only)" if s["name"] == "worst" else s["name"]
        L.append(f"| {label} | {s['uplift_pp']} pp | {pct(s['ppv'], 1) if s['name'] != 'worst' else '–'} | {s['extra_colonoscopies_per_1000']:g} | {s['cancers_found_per_1000']:g} |")
    L += ["", "![Tornado](evidence/tornado.svg)", "",
          "| Input (others at base) | Low | High | Cancers per 1,000 at low | at high |", "|---|---|---|---|---|"]
    L += [f"| {t['input']} | {t['low']} | {t['high']} | {t['out_low']:g} | {t['out_high']:g} |" for t in imp["modelled"]["tornado"]]
    sc_ = imp["modelled"]["scenarios"]
    L += ["", f"Scale: cancers a year = per 1,000 x positive FITs a year / 1,000. At 1,000 to 10,000 positives a year (ASSUMED) the base "
          f"case gives {sc_[1]['cancers_found_per_1000'] * POSITIVES[0] / 1000:g} to {sc_[1]['cancers_found_per_1000'] * POSITIVES[1] / 1000:g} and the best case "
          f"{sc_[2]['cancers_found_per_1000'] * POSITIVES[0] / 1000:g} to {sc_[2]['cancers_found_per_1000'] * POSITIVES[1] / 1000:g} cancers found a year."]
    L += ["", "Delay harm is used qualitatively only: longer waits after a positive FIT are associated with more advanced disease in observational "
          "studies. No lives-saved figure is claimed.", "",
          "### Parameter provenance", "", "| Parameter | Value | Scope | Source |", "|---|---|---|---|"]
    for p in imp["modelled"]["parameters"]:
        src = f"[{p['source']}]({p['url']})" if p["url"] else p["source"]
        L.append(f"| {p['name']} | {p['value']} | {p['scope']} | {src} |")
    L += ["", "UAE = published Abu Dhabi or UAE figure; INTL = borrowed from another country; ASSUMED = no source; used only as a stated range.", "",
          "## Public benchmarks", ""]
    if data["benchmarks"]:
        L += ["| Benchmark | Dataset | Metric | Wasl | Baseline | Note |", "|---|---|---|---|---|---|"]
        for b in data["benchmarks"]:
            base = "–" if b.get("baseline") is None else f"{b['baseline']:g} ({b.get('baseline_source') or ''})"
            L.append(f"| {b['name']} | [{b['dataset']}]({b['url']}) | {b['metric']} | {b['wasl']:g} | {base} | {b['note']} |")
    else:
        L.append("Not yet available: `pipeline/results/benchmarks/summary.json` is absent from this build.")
    L += ["", "## Claim ledger", "", ledger_md(data["claims"]), "",
          "## Reporting-checklist map", "", "| Item | Asks for | Wasl | Where |", "|---|---|---|---|"]
    L += [f"| {a} | {b} | {c} | {d} |" for a, b, c, d in CHECKLIST]
    L += ["", "**NICE Evidence Standards Framework tier:** Tier A (system efficiency): Wasl tracks and escalates follow-up and does not diagnose. "
          "If framed as part of the screening programme it would sit in Tier C ([NICE ESF section B](https://www.nice.org.uk/corporate/ecd7/chapter/section-b-classification-of-digital-health-technologies)). "
          "The evidence plan for a pilot is time-to-closure and the measured-vs-true gap at two DoH providers.", "",
          "## What Wasl does not claim", "",
          "- It does not raise the follow-up rate by itself; it raises the measured rate toward the true rate.",
          "- No real patient data, real Malaffi feed, clinician users or patient outcomes. Reviewers and capacity are simulated.",
          "- No measured clinical benefit: extra colonoscopies and cancers found are modelled and conditional on navigation acting on the list.",
          "- Zero observed false closures is not a zero rate; the upper bounds above are the claim.",
          "- The cohort generator, noise and matcher were written by the team. v2 noise and the held-out persons reduce that circularity; they do not remove it.",
          ""]
    return "\n".join(L)


LEDGER_START, LEDGER_END = "<!-- evidence:ledger:start -->", "<!-- evidence:ledger:end -->"


def outputs():
    data, seeds = build()
    files = {OUT_JSON: json.dumps(data, indent=1, ensure_ascii=False) + "\n", OUT_MD: render_md(data, seeds)}
    files[OUT_SVG + "/impact_per_1000.svg"] = chart_impact(data)
    files[OUT_SVG + "/missed_match.svg"] = chart_missed(data)
    files[OUT_SVG + "/safety_upper.svg"] = chart_safety(data)
    files[OUT_SVG + "/seed_gap.svg"] = chart_seeds(seeds)
    files[OUT_SVG + "/tornado.svg"] = chart_tornado(data)
    readme = read(rel("README.md"))
    if LEDGER_START in readme and LEDGER_END in readme:
        a, rest = readme.split(LEDGER_START, 1)
        _, b = rest.split(LEDGER_END, 1)
        files["README.md"] = a + LEDGER_START + "\n" + ledger_md(data["claims"]) + "\n" + LEDGER_END + b
    return files


def main(argv):
    check = "--check" in argv
    stale = []
    for p, content in outputs().items():
        old = read(rel(p)) if os.path.exists(rel(p)) else None
        if old == content:
            continue
        if check:
            stale.append(p)
            continue
        os.makedirs(os.path.dirname(rel(p)) or ".", exist_ok=True)
        with open(rel(p), "w", encoding="utf-8") as f:
            f.write(content)
        print("wrote", p)
    if stale:
        print("stale (run python3 scripts/evidence.py):", ", ".join(stale))
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
