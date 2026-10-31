#!/usr/bin/env python3
"""Generate two deterministic, wholly synthetic Abu Dhabi breast datasets."""

import json
import random
from collections import Counter
from pathlib import Path

SEED = 9241
SOURCE = (
    "Independent author (GPT-6.1 Sol, OpenAI) - deterministic stdlib generator "
    "gen_breast_heldout.py, random seed 9241, synthetic breast screening texts, "
    "2026-11-01"
)
CODES = {
    "B0": "3340F", "B1": "3341F", "B2": "3342F",
    "B3": "3343F", "B4": "3344F", "B5": "3345F",
}
ORDER = {"1": 0, "2": 1, "3": 2, "0": 3, "4": 4, "5": 5}

# Explicit case allocation prevents random sampling from changing prevalence.
SCREEN_PAIRS = (
    [("1", "1")] * 10
    + [
        ("2", "1"), ("1", "2"), ("2", "2"), ("2", "1"),
        ("1", "2"), ("2", "2"), ("2", "1"), ("1", "2"),
    ]
    + [
        ("0", "1"), ("3", "0"), ("0", "2"), ("TR", "0"),
        ("0", "1"), ("2", "0"), ("0", "3"), ("0", "TR"),
    ]
    + [("3", "1"), ("2", "3")]
    + [
        ("4", "1"), ("2", "4"), ("4", "0"),
        ("TR", "4"), ("4", "3"), ("1", "4"),
    ]
    + [("5", "2"), ("4", "5"), ("5", "TR")]
    + [("TR", "1"), ("2", "TR"), ("TR", "TR")]
)
CODED_IDS = set(range(1, 38, 2)) | {36}
SCREEN_ARABIC = {1, 6, 11, 16, 21, 26, 31, 36}
SCREEN_OCR = {4, 14, 24, 34}
PRIOR_SCREEN = {1, 4, 7, 10, 13, 16, 19, 22}
SUBCATEGORIES = {
    29: "4A", 30: "4B", 31: "4C", 32: "4A",
    33: "4B", 34: "4C", 36: "4C",
}
ASSESS_ARABIC = {1, 7, 13, 19, 25, 31, 37, 43, 49}
ASSESS_OCR = {5, 15, 25, 35, 45}
HISTORICAL_BIOPSY = {2, 14, 29, 44, 46}


def screening_label(right, left):
    if "TR" in (right, left):
        other = left if right == "TR" else right
        return "B" + other if other in {"0", "3", "4", "5"} else "TR"
    return "B" + max((right, left), key=ORDER.__getitem__)


def assessment_label(key):
    if not key["attended"]:
        return "dna"
    if (
        key["assessment"] == "not_done"
        or key["biopsy"] == "pending"
        or key["outcome"] == "pending"
    ):
        return "incomplete"
    if key["outcome"] == "treatment" and key["biopsy"] == "malignant":
        return "to_treatment"
    if key["outcome"] == "routine" and key["biopsy"] != "malignant":
        return "return_routine"
    if key["outcome"] == "early_recall" and key["biopsy"] != "malignant":
        return "early_recall"
    raise ValueError("Unsupported assessment combination")


def breast_description(category, number, short, rng):
    """Descriptions do not rely on overall-category inference."""
    if category == "TR":
        reason = rng.choice([
            "motion obscures detail",
            "posterior tissue excluded by positioning; pectoral muscle not shown",
            "positioning excludes required breast tissue",
        ])
        return (
            "Technically inadequate: " + reason
            + "; repeat acquisition required. No BI-RADS category assigned "
              "to this breast."
        )

    subcategory = SUBCATEGORIES.get(number, "4A") if category == "4" else category
    if short:
        descriptions = {
            "0": "Incomplete; focal asymmetry needs additional diagnostic views",
            "1": "Negative; no suspicious mass or malignant calcification",
            "2": "Benign vascular calcification; no suspicious lesion",
            "3": "Probably benign circumscribed oval mass; six-month imaging",
            "4": "Suspicious grouped calcifications; no suspicious mass; biopsy advised",
            "5": (
                "Highly suggestive irregular spiculated mass; "
                "no suspicious calcification; biopsy advised"
            ),
        }
    else:
        descriptions = {
            "0": (
                "A focal asymmetry cannot be resolved on these screening views. "
                "Additional diagnostic mammographic views and targeted ultrasound "
                "are required; this breast remains incomplete"
            ),
            "1": rng.choice([
                "No suspicious mass, architectural distortion or malignant calcification",
                "No suspicious mass or malignant calcification; no focal abnormality",
            ]),
            "2": (
                "Coarse vascular calcification has a benign appearance. "
                "No suspicious mass or malignant calcification; the benign "
                "finding is retained in today's assessment"
            ),
            "3": (
                "An oval circumscribed mass has probably benign morphology. "
                "Short-interval imaging at six months is advised"
            ),
            "4": (
                "Grouped pleomorphic calcifications are suspicious and require "
                "stereotactic core sampling. No suspicious mass is seen; this "
                "does not remove the concern about the calcifications"
            ),
            "5": (
                "An irregular spiculated mass is highly suggestive of malignancy; "
                "urgent tissue diagnosis is advised. No suspicious calcification "
                "is seen, but the mass remains highly concerning"
            ),
        }
    return descriptions[category] + "; BI-RADS " + subcategory + "."


def screening_layout(number, header, body, comparison, final, signoff):
    parts = {
        "header": header,
        "body": body,
        "comparison": comparison or "Comparison: no earlier category quoted.",
        "final": final,
        "signoff": signoff,
    }
    layouts = [
        "{header}\nFINDINGS\n{body}\n{comparison}\n{final}\n{signoff}",
        "{header}\n{comparison}\nBREAST-BY-BREAST RECORD:\n{body}\n{final}\n{signoff}",
        "{header}\n--- screening worksheet ---\n{body}\n"
        "Comparison field: {comparison}\nDisposition field: {final}\n{signoff}",
        "{header}\nDear screening team,\n{comparison}\n{body}\n"
        "{final}\n{signoff}",
        "{header}\n[Current examination]\n{body}\n"
        "[Earlier examination]\n{comparison}\n[Conclusion]\n{final}\n{signoff}",
        "{header}\nMAMMOGRAPHY / two-view digital acquisition\n"
        "{comparison}\n{body}\nSUMMARY: {final}\n{signoff}",
    ]
    return layouts[(number - 1) % len(layouts)].format(**parts)


def make_screening(rng):
    reports = []
    for number, (right, left) in enumerate(SCREEN_PAIRS, 1):
        label = screening_label(right, left)
        coded = CODES[label] if number in CODED_IDS else None
        overall = (
            label[1:]
            if label != "TR" and (
                coded is not None or number % 4 == 0 or number in {24, 30}
            )
            else None
        )
        tags = set()
        header = (
            f"Abu Dhabi | Synthetic Breast Screening Unit "
            f"{rng.choice(['Palm', 'Dune', 'Pearl'])} | HB{number:02d}\n"
            "Screening mammogram | 01 Nov 2026"
        )
        if number in SCREEN_ARABIC:
            header += "\nوحدة فحص الثدي - أبوظبي"
            tags.add("language:arabic")
        if number in SCREEN_OCR:
            header += "\nScann ed copy | D1GITAL | Acces sion verified"
            tags.add("scan:ocr")

        ordinary_names = [
            ("Right breast", "Left breast"), ("RT", "LT"), ("R", "L")
        ]
        names = (
            ("الثدي الأيمن", "الثدي الأيسر")
            if number in {11, 21, 31}
            else ordinary_names[(number - 1) % 3]
        )
        body = "\n".join(
            name + ": " + breast_description(category, number, coded is not None, rng)
            for name, category in zip(names, (right, left))
        )

        comparison = ""
        if number in PRIOR_SCREEN:
            previous = "2" if label == "B1" else "1"
            comparison = (
                f"Previous screen, 2023: bilateral BI-RADS {previous}. "
                "That historical assessment is not today's assessment."
            )
            tags.add("hard:prior_category")
        if number in {20, 25}:
            tags.add("hard:zero_over_three")
        if number in {22, 26, 32, 37}:
            tags.add("hard:technical_override")
        if number in {38, 39, 40}:
            tags.add("hard:technical_not_incomplete")
        if number in SUBCATEGORIES:
            tags.add("hard:subcategory_collapse")
        if 11 <= number <= 18:
            tags.add("hard:benign_not_negative")
        if 29 <= number <= 37:
            tags.add("hard:negative_scope")

        if number in {24, 30}:
            withdrawn = "2" if number == 24 else "0"
            final = (
                f"Superseding amendment: preliminary overall BI-RADS {withdrawn} "
                f"was entered in error and is withdrawn. "
                f"Final overall assessment: BI-RADS {overall}."
            )
            tags.add("hard:corrected_assessment")
        elif overall is not None:
            final = f"Final overall assessment: BI-RADS {overall}."
        elif label == "TR":
            final = (
                "Technical repeat required for the inadequate breast image set(s); "
                "no overall BI-RADS assessment is assigned."
            )
        else:
            final = (
                "Management follows the breast-specific findings above; "
                "no overall category is stated."
            )
        if coded is not None:
            final += f" CPT II final-assessment code: {coded}."

        signoff = (
            "Authorised by screening radiologist; recall centre notified."
            if number % 2
            else "Authorized by screening radiologist; recall center notified."
        )
        text = screening_layout(number, header, body, comparison, final, signoff)
        reports.append({
            "id": f"hb{number:02d}",
            "text": text,
            "key": {
                "b_right": right,
                "b_left": left,
                "overall": overall,
                "label": label,
                "coded": coded,
            },
            "tags": sorted(tags),
        })
    return reports


def assessment_case(number):
    """Return episode-level facts and matching current-episode narrative."""
    attended, assessment, biopsy, outcome = True, "done", "none", "routine"
    tags = set()
    attendance = "She attended the recalled-screen assessment appointment."

    if number <= 13:
        if number <= 6:
            biopsy = "benign"
            evaluation = (
                "Additional mammographic views and targeted ultrasound were "
                "performed. This episode's image-guided core biopsy shows "
                "benign fibroadenomatoid change, concordant with the imaging."
            )
        else:
            evaluation = (
                "Diagnostic mammographic views and targeted ultrasound were "
                "performed. The recalled asymmetry resolves as summation "
                "of normal tissue. No biopsy was performed in this episode."
            )
            tags.add("hard:imaging_not_biopsy")
        plan = "Assessment complete: return to routine breast screening."
    elif number <= 18:
        outcome = "early_recall"
        if number <= 15:
            biopsy = "benign"
            evaluation = (
                "Targeted ultrasound and core biopsy were performed. "
                "This episode's core shows benign stromal fibrosis, "
                "concordant with imaging."
            )
        else:
            evaluation = (
                "Diagnostic views and targeted ultrasound were performed. "
                "A probably benign oval mass remains. No biopsy was performed "
                "in this episode."
            )
            tags.add("hard:imaging_not_biopsy")
        plan = (
            "Decision: short-interval imaging in six months, not discharge "
            "to routine screening and not referral for cancer treatment."
        )
    elif number <= 28:
        biopsy, outcome = "malignant", "treatment"
        if number in {19, 21, 23}:
            diagnosis = (
                "ductal carcinoma in situ (DCIS), with no invasive carcinoma "
                "identified in the cores"
            )
            tags.add("hard:malignant_in_situ")
        else:
            diagnosis = "invasive carcinoma of no special type"
        evaluation = (
            "Diagnostic imaging and image-guided core biopsy were completed. "
            f"This episode's core biopsy confirms {diagnosis}."
        )
        plan = (
            "MDT decision: refer to the breast cancer surgical service for "
            "definitive treatment planning. Referral has been issued; this "
            "is not a surveillance-only disposition."
        )
    elif number in {29, 30, 31, 32, 43}:
        biopsy, outcome = "pending", "pending"
        evaluation = (
            "Diagnostic imaging was performed and a core biopsy was taken "
            "during this episode. Histology has not been issued; the result "
            "is awaited."
        )
        plan = "No final management decision; review once current histology is available."
        tags.add("hard:pending_histology")
    elif number in {33, 34, 35}:
        assessment, outcome = "not_done", "pending"
        explanations = {
            33: "She declined all assessment procedures after administrative check-in.",
            34: "She declined assessment because she wished to reconsider consent.",
            35: (
                "She could not remain because of an urgent family commitment "
                "and left before assessment started."
            ),
        }
        evaluation = (
            explanations[number]
            + " No clinical breast examination, additional imaging, ultrasound "
              "or biopsy took place. The encounter was administrative only."
        )
        plan = "Rebooking offered; assessment and management decision remain outstanding."
        tags.add("hard:declined_assessment")
    elif number in {36, 37, 38}:
        outcome = "pending"
        evaluation = (
            "Clinical breast examination was performed. Targeted imaging could "
            "not be completed at this visit and remains outstanding. "
            "No biopsy result is reported for this episode."
        )
        plan = (
            "Return to finish diagnostic imaging; no final screening or "
            "treatment disposition has been made."
        )
        tags.add("hard:assessment_without_resolution")
    elif number in {39, 40}:
        biopsy, outcome = "benign", "pending"
        evaluation = (
            "Diagnostic imaging and core sampling were completed. This "
            "episode's core biopsy is benign, but the result is radiologically "
            "discordant with the persistent suspicious distortion."
        )
        plan = (
            "Repeat sampling is requested. Management remains undecided; "
            "the benign result does not authorize routine screening."
        )
        tags.add("hard:benign_discordance")
    elif number in {41, 42}:
        biopsy, outcome = "malignant", "pending"
        evaluation = (
            "Further imaging and core biopsy were performed. This episode's "
            "core confirms invasive breast carcinoma."
        )
        plan = (
            "MDT management discussion has been deferred pending completion "
            "of staging and patient consultation. No treatment referral or "
            "final management decision has yet been issued."
        )
        tags.add("hard:malignant_not_yet_treatment")
    else:
        attended, assessment, biopsy, outcome = False, "not_done", "none", "pending"
        attendance = (
            "She did not attend the scheduled in-person recalled-screen "
            "assessment appointment."
        )
        if number <= 47:
            evaluation = (
                "Administrative telephone contact confirmed that she had not "
                "attended. The call concerned appointment dates only; no "
                "clinical assessment, imaging or biopsy occurred."
            )
            tags.add("hard:nonattendance_contact")
        else:
            evaluation = (
                "Attendance register checked: no attendance and no clinical "
                "assessment, further imaging or biopsy in this episode."
            )
        plan = "Recall remains open; a new appointment is offered. No decision yet."

    history = "Referral: recalled following a screening mammogram."
    if number in HISTORICAL_BIOPSY:
        history += (
            " Historical record only: a 2021 core biopsy was benign. "
            "That earlier biopsy is not a result from the current episode."
        )
        tags.add("hard:historical_biopsy")

    key = {
        "attended": attended,
        "assessment": assessment,
        "biopsy": biopsy,
        "outcome": outcome,
    }
    key["label"] = assessment_label(key)
    return key, history, attendance, evaluation, plan, tags


def assessment_layout(number, header, history, attendance, evaluation, plan, footer):
    parts = dict(
        header=header, history=history, attendance=attendance,
        evaluation=evaluation, plan=plan, footer=footer,
    )
    layouts = [
        "{header}\nREFERRAL / HISTORY: {history}\nATTENDANCE: {attendance}\n"
        "ASSESSMENT: {evaluation}\nPLAN: {plan}\n{footer}",
        "{header}\nDear referring screening unit,\n{history}\n"
        "{attendance} {evaluation}\nOur decision: {plan}\n{footer}",
        "{header}\nMDT episode summary\nBackground | {history}\n"
        "Visit | {attendance}\nEvidence | {evaluation}\n"
        "Disposition | {plan}\n{footer}",
        "{header}\nS: {history}\nO: {attendance}\n"
        "A: {evaluation}\nP: {plan}\n{footer}",
        "{header}\nRecalled-screen pathway checklist\n"
        "[Referral] {history}\n[Visit] {attendance}\n"
        "[Work-up] {evaluation}\n[Next step] {plan}\n{footer}",
        "{header}\nClinical chronology\n1. {history}\n2. {attendance}\n"
        "3. {evaluation}\n4. {plan}\n{footer}",
    ]
    return layouts[(number - 1) % len(layouts)].format(**parts)


def make_assessments(rng):
    reports = []
    for number in range(1, 51):
        key, history, attendance, evaluation, plan, tags = assessment_case(number)
        unit = (
            "Diagnostic Breast Assessment Unit"
            if number % 3 else "Breast MDT"
        )
        header = (
            f"Abu Dhabi | Synthetic {unit} "
            f"{rng.choice(['Palm', 'Dune', 'Pearl'])} | HA{number:02d}\n"
            "Current screening-recall episode | 01 Nov 2026"
        )
        if number in ASSESS_ARABIC:
            header += "\nوحدة تقييم الثدي - أبوظبي"
            tags.add("language:arabic")
        if number in ASSESS_OCR:
            header += "\nScann ed clinic c0py | rec all episode"
            tags.add("scan:ocr")
        footer = (
            "Authorised entry; breast assessment centre."
            if number % 2
            else "Authorized entry; breast assessment center."
        )
        text = assessment_layout(
            number, header, history, attendance, evaluation, plan, footer
        )
        reports.append({
            "id": f"ha{number:02d}",
            "text": text,
            "key": key,
            "tags": sorted(tags),
        })
    return reports


SCREEN_TAG_COUNTS = {
    "language:arabic": 8,
    "scan:ocr": 4,
    "hard:prior_category": 8,
    "hard:zero_over_three": 2,
    "hard:technical_override": 4,
    "hard:technical_not_incomplete": 3,
    "hard:subcategory_collapse": 7,
    "hard:benign_not_negative": 8,
    "hard:negative_scope": 9,
    "hard:corrected_assessment": 2,
}
ASSESS_TAG_COUNTS = {
    "language:arabic": 9,
    "scan:ocr": 5,
    "hard:historical_biopsy": 5,
    "hard:imaging_not_biopsy": 10,
    "hard:malignant_in_situ": 3,
    "hard:pending_histology": 5,
    "hard:declined_assessment": 3,
    "hard:assessment_without_resolution": 3,
    "hard:benign_discordance": 2,
    "hard:malignant_not_yet_treatment": 2,
    "hard:nonattendance_contact": 4,
}


def tag_counts(reports):
    return Counter(tag for report in reports for tag in report["tags"])


def validate(screening, assessments):
    assert len(screening) == 40 and len(assessments) == 50
    assert [r["id"] for r in screening] == [
        f"hb{i:02d}" for i in range(1, 41)
    ]
    assert [r["id"] for r in assessments] == [
        f"ha{i:02d}" for i in range(1, 51)
    ]
    all_reports = screening + assessments
    assert len({r["text"] for r in all_reports}) == 90
    assert all(set(r) == {"id", "text", "key", "tags"} for r in all_reports)
    assert all(r["tags"] == sorted(set(r["tags"])) for r in all_reports)

    assert Counter(r["key"]["label"] for r in screening) == {
        "B1": 10, "B2": 8, "B0": 8, "B3": 2,
        "B4": 6, "B5": 3, "TR": 3,
    }
    assert Counter(r["key"]["label"] for r in assessments) == {
        "return_routine": 13, "early_recall": 5, "to_treatment": 10,
        "incomplete": 15, "dna": 7,
    }
    assert tag_counts(screening) == SCREEN_TAG_COUNTS
    assert tag_counts(assessments) == ASSESS_TAG_COUNTS

    for report in screening:
        key = report["key"]
        assert set(key) == {"b_right", "b_left", "overall", "label", "coded"}
        assert key["label"] == screening_label(key["b_right"], key["b_left"])
        assert key["overall"] is None or (
            key["label"] != "TR" and key["overall"] == key["label"][1:]
        )
        assert key["coded"] is None or key["coded"] == CODES[key["label"]]
        if key["label"] == "TR":
            assert key["coded"] is None
    assert sum(r["key"]["coded"] is not None for r in screening) == 20

    for report in assessments:
        key = report["key"]
        assert set(key) == {
            "attended", "assessment", "biopsy", "outcome", "label"
        }
        assert type(key["attended"]) is bool
        assert key["assessment"] in {"done", "not_done"}
        assert key["biopsy"] in {"malignant", "benign", "pending", "none"}
        assert key["outcome"] in {"routine", "early_recall", "treatment", "pending"}
        assert key["label"] == assessment_label(key)
        if key["outcome"] == "treatment":
            assert key["biopsy"] == "malignant"
        if key["outcome"] in {"routine", "early_recall"}:
            assert key["biopsy"] != "malignant"

    for reports, expected_hard in ((screening, 30), (assessments, 34)):
        hard_count = sum(
            any(tag.startswith("hard:") for tag in r["tags"]) for r in reports
        )
        assert hard_count == expected_hard
        assert hard_count * 10 >= len(reports) * 3


def write_dataset(path, reports):
    payload = {"source": SOURCE, "reports": reports}
    data = json.dumps(payload, ensure_ascii=False, indent=2) + "\n"
    path.write_bytes(data.encode("utf-8"))


def main():
    # No input files, network access, wall clock, or environment-based sampling.
    rng = random.Random(SEED)
    screening = make_screening(rng)
    assessments = make_assessments(rng)
    validate(screening, assessments)
    directory = Path(__file__).absolute().parent
    write_dataset(directory / "screen_reports.json", screening)
    write_dataset(directory / "assess_notes.json", assessments)


if __name__ == "__main__":
    main()
