"""Queue levers (SIMULATED decision support): direct-to-procedure pathway + prep-failure risk.

Both levers come from docs/research/queue-levers.md and docs/ROADMAP.md L1. Nothing here books, prescribes or messages
anyone: the engine only *suggests* a pathway and a prep intervention; a nurse completes the App 9 checklist and a
clinician orders any split/extended regimen.

1. Synthetic DoH App 9 pre-colonoscopy fields per case (deterministic, seeded by case id). The pilot reads these from
   Malaffi (medications, problem list) plus one nurse call; here every value is SYNTHETIC. Prevalences are tagged:
   [A] = assumption for an Abu Dhabi FIT+ population aged 40-75 (no local source found).
2. Pathway rule (DoH CRC spec 5.5.1 + App 9: no GI consult is required, only a pre-colonoscopy assessment):
   ASA >= 3, anticoagulation/antiplatelet needing a peri-procedural plan, or an implanted cardiac device -> clinic
   (GI consult) pathway; everyone else -> direct-to-procedure via the nurse checklist.
3. Prep-failure risk: the Dik 2015 score (Dik et al., Gastrointest Endosc 2015;81(3):665-672, PMID 25600879), with
   published points: ASA>=3 2, tricyclic antidepressants 3, opioids 1, diabetes 1, chronic constipation 2, prior
   abdominal/pelvic surgery 1, current hospitalisation 1, history of inadequate prep 1 (range 0-12). Cut-off >=2:
   sensitivity 66%, specificity 79%, PPV 29%, NPV 95% (>=3: sens 43%, spec 90%). AUC 0.72 development, 0.77
   validation, 0.62-0.74 external. LIMITATION: derived on 1,996 split-dose colonoscopies at 4 Dutch centres
   (inadequate = total BBPS < 6), not a FIT+ cohort and not a UAE population; local recalibration is needed.
   Secondary reference (not used for scoring): Gimeno-Garcia 2017 (Endoscopy, PMID 28282690): comorbidity (diabetes,
   stroke, cirrhosis or CKD) 4, TCA 1.705, constipation 1.225, abdominal/pelvic surgery 0.606; high risk if >1.225.
"""
from __future__ import annotations
import hashlib
import math

SCHEMA = "wasl-levers/1"

# ---- synthetic App 9 prevalences (all [A] unless noted) ----
PREV = {
    "anticoagulant": 0.07,          # [A] warfarin/DOAC/P2Y12 needing a peri-procedural plan
    "diabetes": 0.20,               # [A] above the adult UAE prevalence (~16%, IDF Atlas 2021) because FIT+ is 40-75
    "insulin_given_diabetes": 0.25, # [A]
    "device": 0.015,                # [A] pacemaker / ICD
    "prior_abdominal_surgery": 0.15,  # [A]
    "tricyclic": 0.02,              # [A]
    "opioid": 0.04,                 # [A]
    "constipation": 0.14,           # [A] near the pooled global chronic-constipation prevalence (~14%)
    "prior_colonoscopy": 0.10,      # [A]
    "prior_inadequate_given_prior": 0.20,  # [A]
    "hospitalised": 0.0,            # outpatient FIT+ screening referrals: nobody is an inpatient at referral
}
# P(ASA >= 3): [A] higher with insulin-treated diabetes or anticoagulation
ASA3_BASE, ASA3_DIAB, ASA3_INSULIN_OR_AC = 0.06, 0.15, 0.35

# ---- Dik 2015 points (published; PMID 25600879) ----
POINTS = {"asa_ge_3": 2, "tricyclic": 3, "opioid": 1, "diabetes": 1, "constipation": 2,
          "prior_abdominal_surgery": 1, "hospitalised": 1, "prior_inadequate_prep": 1}
HIGH_RISK_MIN = 2   # Dik 2015 cut-off >= 2 (sens 66%, spec 79%, NPV 95%)
SCORE_REF = "Dik et al. GIE 2015;81(3):665-672 (PMID 25600879); cut-off >=2"

INTERVENTION = {
    "low": {"code": "sms_prep_sequence", "text": "SMS prep-instruction sequence (D-7, D-3, D-1) in the patient's language",
            "evidence": "SMS education: adequate prep 81.7% vs 75.7% (RR 1.10, 7 RCTs; IJCD 2022)"},
    "high": {"code": "nurse_call_extended_regimen",
             "text": "Nurse prep call + split/extended regimen and low-residue diet, if the clinician orders it",
             "evidence": "model-targeted extra PEG for high-risk patients: 94% vs 49% adequate (DLD 2023, one trial)"},
}


def _u(key: str, salt: str) -> float:
    return int(hashlib.sha256(f"levers|{salt}|{key}".encode()).hexdigest()[:12], 16) / 16 ** 12


def app9_fields(case_id: str) -> dict:
    """Deterministic synthetic App 9 + Dik predictor fields for one case."""
    f = {}
    f["anticoagulant"] = _u(case_id, "ac") < PREV["anticoagulant"]
    f["diabetes"] = _u(case_id, "dm") < PREV["diabetes"]
    f["insulin"] = f["diabetes"] and _u(case_id, "ins") < PREV["insulin_given_diabetes"]
    f["device"] = _u(case_id, "dev") < PREV["device"]
    f["prior_abdominal_surgery"] = _u(case_id, "surg") < PREV["prior_abdominal_surgery"]
    f["tricyclic"] = _u(case_id, "tca") < PREV["tricyclic"]
    f["opioid"] = _u(case_id, "opi") < PREV["opioid"]
    f["constipation"] = _u(case_id, "con") < PREV["constipation"]
    prior = _u(case_id, "pc") < PREV["prior_colonoscopy"]
    f["prior_colonoscopy"] = prior
    f["prior_inadequate_prep"] = prior and _u(case_id, "pip") < PREV["prior_inadequate_given_prior"]
    f["hospitalised"] = _u(case_id, "hosp") < PREV["hospitalised"]
    p3 = ASA3_INSULIN_OR_AC if (f["insulin"] or f["anticoagulant"]) else ASA3_DIAB if f["diabetes"] else ASA3_BASE
    u = _u(case_id, "asa")
    f["asa_class"] = 4 if u < p3 * 0.08 else 3 if u < p3 else 2 if u < p3 + (1 - p3) * 0.6 else 1
    return f


def pathway(f: dict) -> dict:
    """Direct-to-procedure unless App 9 needs a clinician (spec 5.5.1 / App 9)."""
    reasons = []
    if f["asa_class"] >= 3:
        reasons.append(f"ASA {f['asa_class']}")
    if f["anticoagulant"]:
        reasons.append("anticoagulation/antiplatelet: peri-procedural plan")
    if f["device"]:
        reasons.append("implanted cardiac device")
    notes = []
    if f["insulin"]:
        notes.append("insulin: dose-adjustment instructions in the nurse checklist")
    if not reasons:
        return {"route": "direct", "reasons": ["App 9 nurse checklist clear"], "checklist_notes": notes}
    return {"route": "clinic", "reasons": reasons, "checklist_notes": notes}


def prep_score(f: dict) -> tuple[int, list[str]]:
    feats = {"prior_inadequate_prep": f["prior_inadequate_prep"], "tricyclic": f["tricyclic"], "opioid": f["opioid"],
             "diabetes": f["diabetes"], "constipation": f["constipation"], "asa_ge_3": f["asa_class"] >= 3,
             "prior_abdominal_surgery": f["prior_abdominal_surgery"], "hospitalised": f.get("hospitalised", False)}
    on = [k for k, v in feats.items() if v]
    return sum(POINTS[k] for k in on), on


def prep_risk(f: dict) -> dict:
    s, factors = prep_score(f)
    band = "high" if s >= HIGH_RISK_MIN else "low"
    return {"score": s, "band": band, "factors": factors, "weights": SCORE_REF,
            "limitation": "derived on Dutch split-dose colonoscopies, not FIT+ / UAE; recalibrate locally",
            "suggested_intervention": INTERVENTION[band], "requires_clinician_order": True}


def baseline_inadequate_p(score: int, intercept: float, beta: float = math.log(1.6)) -> float:
    """Per-patient P(inadequate prep) without any lever: logit = intercept + beta*score.
    beta = ln 1.6 per Dik point is an ASSUMPTION (the paper reports discrimination, not a per-point odds ratio)."""
    return 1 / (1 + math.exp(-(intercept + beta * score)))


def fit_intercept(scores, mean_target: float, beta: float = math.log(1.6)) -> float:
    """Intercept so the population mean P(inadequate) equals mean_target."""
    lo, hi = -8.0, 4.0
    for _ in range(60):
        mid = (lo + hi) / 2
        m = sum(baseline_inadequate_p(s, mid, beta) for s in scores) / max(1, len(scores))
        lo, hi = (mid, hi) if m < mean_target else (lo, mid)
    return (lo + hi) / 2


def assess(case_id: str) -> dict:
    f = app9_fields(case_id)
    return {"app9": dict(f, data_mode="synthetic"), "pathway": pathway(f), "prep_risk": prep_risk(f)}
