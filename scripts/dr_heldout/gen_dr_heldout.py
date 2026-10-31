#!/usr/bin/env python3
"""
Deterministic generator of synthetic diabetic retinopathy screening reports
and post-referral hospital eye-clinic notes from Abu Dhabi.

Author: independent author (Gemini 4 Argon, Google)
Random seed: 9091
"""

import json
import os
import random

SEED = 9091
SOURCE_STR = (
    "Independent author (Gemini 4 Argon, Google) - deterministic stdlib generator "
    "gen_dr_heldout.py, random seed 9091, synthetic diabetic retinopathy texts, 2026-10-31"
)

R_RANK = {"0": 0, "1": 1, "2": 2, "3S": 3, "3A": 4}
REFERABLE_R = {"2", "3S", "3A"}


def compute_screening_label(r_right, r_left, m_right, m_left, urgent):
    """Apply the exact NHS DESP / task specification label rules."""
    assert (r_right == "U") == (m_right == ""), f"Invalid right U/M: {r_right}, {m_right}"
    assert (r_left == "U") == (m_left == ""), f"Invalid left U/M: {r_left}, {m_left}"

    def is_referable(r, m):
        if r == "U":
            return False
        return (r in REFERABLE_R) or (m == "1")

    if r_right == "U" or r_left == "U":
        if r_right == "U" and r_left == "U":
            return "U"
        other_r = r_left if r_right == "U" else r_right
        other_m = m_left if r_right == "U" else m_right
        if is_referable(other_r, other_m):
            lbl = f"R{other_r}M{other_m}"
            if urgent and other_r != "3A":
                lbl += "/urgent"
            return lbl
        return "U"

    worse_r = r_right if R_RANK[r_right] >= R_RANK[r_left] else r_left
    worse_m = "1" if (m_right == "1" or m_left == "1") else "0"
    lbl = f"R{worse_r}M{worse_m}"
    if urgent and worse_r != "3A":
        lbl += "/urgent"
    return lbl


def compute_clinic_label(attended, fundus, outcome):
    """Apply the exact eye-clinic label rules."""
    if not attended:
        assert fundus is None and outcome == "none"
        return "dna"
    if fundus == "no_view":
        return "not_assessable"
    if fundus == "not_examined":
        return "not_examined"
    if fundus == "examined":
        assert outcome in ("treated", "followup", "discharged")
        return f"examined_{outcome}"
    raise ValueError(f"Invalid clinic key state: {attended}, {fundus}, {outcome}")


def apply_ocr_noise(text, rng):
    """Deterministically inject realistic scan/OCR artefacts into selected tokens."""
    replacements = [
        ("Retinopathy", "Retin0pathy"),
        ("retinopathy", "ret1nopathy"),
        ("Maculopathy", "Macu1opathy"),
        ("maculopathy", "macul0pathy"),
        ("haemorrhages", "haem0rrhages"),
        ("hemorrhages", "hem0rrhages"),
        ("microaneurysms", "m1croaneurysms"),
        ("Screening", "Screen1ng"),
        ("screening", "screenlng"),
        ("Ophthalmology", "0phthalmology"),
        ("Fundus", "Fundu5"),
        ("fundus", "fundu5"),
        ("Assessment", "Assessrnent"),
        ("Clinic", "Cl1nic"),
        ("Patient", "Pat1ent"),
        ("Referral", "Referra1"),
        ("Right", "R1ght"),
        ("Left", "Lelt"),
    ]
    out = text
    count = 0
    for old, new in replacements:
        if old in out and rng.random() < 0.65:
            out = out.replace(old, new, 1)
            count += 1
    if count == 0:
        out = out.replace("e", "3", 1).replace("i", "1", 1)
    return out + "\n[Scanned document - OCR confidence 84% ~ |l1]"


def format_screening_layout(layout_id, site, date_str, mrn, body, coded_label):
    """Format screening report in one of 6 distinct layouts."""
    if layout_id == 0:
        hdr = f"=== SEHA AHS PRIMARY CARE DIABETIC EYE SCREENING ===\nCentre: {site} | Date: {date_str} | MRN: {mrn}"
        code_line = f"\n[SYSTEM CODED GRADE: {coded_label}]" if coded_label else ""
        return f"{hdr}\n{body}{code_line}"
    elif layout_id == 1:
        hdr = f"MUBADALA HEALTH / IMPERIAL COLLEGE LONDON DIABETES CENTRE ({site})\nRetinal Photography Report ({date_str}) - File #{mrn}"
        code_line = f"\nSummary Code: {coded_label}" if coded_label else ""
        return f"{hdr}\n--------------------------------------------------\n{body}{code_line}"
    elif layout_id == 2:
        code_line = f" | AutoGrade={coded_label}" if coded_label else " | AutoGrade=NONE"
        return f"<{date_str}> [{site}] Pt:{mrn}{code_line}\n{body}"
    elif layout_id == 3:
        hdr = f"ببرنامج فحص اعتلال الشبكية السكري - أبوظبي (Abu Dhabi DESP)\nClinic: {site} | Date: {date_str} | ID: {mrn}"
        code_line = f"\nFinal DESP Code: {coded_label}" if coded_label else ""
        return f"{hdr}\n{body}{code_line}"
    elif layout_id == 4:
        code_line = f"CODED OUTCOME: {coded_label}\n" if coded_label else ""
        return f"REPORTING PORTAL EXPORT // {site.upper()} // {date_str}\nPATIENT ID: {mrn}\n{code_line}FINDINGS:\n{body}"
    else:
        code_line = f" (Grade: {coded_label})" if coded_label else ""
        return f"Dear GP,\nRe: Patient {mrn} screened at {site} on {date_str}{code_line}.\n{body}\nYours sincerely,\nAccredited Retinal Grader"


def format_clinic_layout(layout_id, hospital, date_str, mrn, body):
    """Format hospital eye-clinic note in one of 6 distinct layouts."""
    if layout_id == 0:
        return (
            f"SHEIKH SHAKHBOUT MEDICAL CITY (SSMC) - OPHTHALMOLOGY DEPT\n"
            f"Clinic: {hospital} | Date: {date_str} | MRN: {mrn}\n"
            f"--------------------------------------------------------\n{body}"
        )
    elif layout_id == 1:
        return (
            f"CLEVELAND CLINIC ABU DHABI - EYE INSTITUTE\n"
            f"Encounter Date: {date_str} | Patient MRN: {mrn} | Location: {hospital}\n"
            f"{body}"
        )
    elif layout_id == 2:
        return (
            f"عيادة العيون التخصصية - صحة أبوظبي (SEHA Ophthalmology)\n"
            f"[{hospital} - {date_str} - File: {mrn}]\n{body}"
        )
    elif layout_id == 3:
        return (
            f"MEDICAL RETINA CONSULTATION NOTE\n"
            f"Facility: {hospital} // Date: {date_str} // Pt: {mrn}\n"
            f"========================================\n{body}"
        )
    elif layout_id == 4:
        return f"Outpatient Letter ({date_str}) - {hospital} (MRN {mrn})\nDear Referrer,\n{body}\nRegards, Medical Retina Team"
    else:
        return f"EHR SOAP NOTE | {hospital} | {date_str} | {mrn}\n{body}"


def build_screening_reports(rng):
    sites = [
        "Al Bateen Healthcare Center",
        "Madinat Khalifa Healthcare Center",
        "Baniyas Primary Care Clinic",
        "Al Zafaranah Diagnostic & Screening Center",
        "ICLDC Abu Dhabi Main Branch",
        "ICLDC Al Ain Branch",
        "Al Yahar Ambulatory Center",
        "Al Muwaiji Primary Screening Unit",
    ]

    # 40 screening specs: 13 carry hard:* tags (32.5%)
    # (id, r_right, r_left, m_right, m_left, urgent, is_coded, layout_id, tags, body_text)
    specs = [
        # --- R0M0 (12 reports: hs01..hs12) ---
        (
            "hs01", "0", "0", "0", "0", False, True, 0,
            ["coded", "spelling:uk", "eye:right_left", "negated_finding"],
            "Structured Grade Summary: Right eye R0 M0, Left eye R0 M0. Dual-field digital colour photographs show normal optic discs and normal macular reflexes bilaterally. No microaneurysms, no haemorrhages, and no exudates seen. Routine annual recall.",
        ),
        (
            "hs02", "0", "0", "0", "0", False, True, 1,
            ["coded", "spelling:us", "eye:od_os"],
            "Coded Grading Output: OD = R0 M0 | OS = R0 M0. Color fundus photography demonstrates clear media and healthy retinal vasculature in both eyes. No diabetic retinopathy or maculopathy identified. Repeat screening in 12 months.",
        ),
        (
            "hs03", "0", "0", "0", "0", False, True, 3,
            ["coded", "arabic", "eye:re_le", "eye:arabic"],
            "نتيجة الفحص: طبيعي في العينين (Normal bilateral exam).\nStructured Per-Eye Grade: RE (العين اليمنى): R0 M0; LE (العين اليسرى): R0 M0. Maculae flat and dry. Return to annual call/recall.",
        ),
        (
            "hs04", "0", "0", "0", "0", False, True, 4,
            ["coded", "spelling:uk", "eye:right_left", "prev_grade_diff"],
            "Per-eye coded assessment: Right eye R0 M0, Left eye R0 M0. Note: Previous screening 12 months ago was graded R1M0 (single tiny dot haemorrhage right temporal arcade), which has completely resolved today with improved HbA1c. Annual recall.",
        ),
        (
            "hs05", "0", "0", "0", "0", False, False, 2,
            ["spelling:uk", "eye:re_le", "negated_finding"],
            "VA RE 6/6, LE 6/6. Mydriatic two-field images captured. RE: normal disc, normal vessels, no microaneurysms, no maculopathy. LE: normal fundus appearance, no haemorrhages or exudates, no new vessels. Impression: no diabetic retinopathy in either eye.",
        ),
        (
            "hs06", "0", "0", "0", "0", False, False, 5,
            ["spelling:us", "eye:od_os", "hard:non_dr_pathology"],
            "VA OD 20/25, OS 20/20. Fundus photos show intermediate hard and soft macular drusen within 1 DD of the fovea in both eyes with mild RPE mottling consistent with dry age-related macular degeneration (AMD). However, there are zero microaneurysms, no retinal hemorrhages, no hard lipid exudates, and no diabetic macular edema in OD or OS. No diabetic retinopathy.",
        ),
        (
            "hs07", "0", "0", "0", "0", False, False, 0,
            ["spelling:uk", "eye:right_left", "hard:non_dr_pathology"],
            "Right eye and Left eye retinal photographs reviewed. Prominent copper wiring and arteriovenous nipping noted along the superotemporal arcade in the right eye (Grade 2 hypertensive retinopathy), plus a benign choroidal naevus inferonasally in the left eye. No diabetic microaneurysms, no blot haemorrhages, and no exudates near the fovea in either eye. Routine diabetic recall.",
        ),
        (
            "hs08", "0", "0", "0", "0", False, False, 1,
            ["spelling:uk", "eye:re_le", "prev_grade_diff", "negated_finding"],
            "Comparison made with prior screening from last year (which showed mild background changes R1M0 in the left eye). Today's photographs of RE and LE show complete turnover and resolution of the previous microaneurysms. No active microaneurysms, no haemorrhages, and no exudates in either eye today.",
        ),
        (
            "hs09", "0", "0", "0", "0", False, False, 2,
            ["spelling:us", "eye:od_os", "ocr_noise"],
            "Digital Retinal Screening examination. OD: normal optic nerve, flat macula, no diabetic retinopathy or edema. OS: clear media, normal caliber vessels, no microaneurysms or hemorrhages seen. Recall 1 year.",
        ),
        (
            "hs10", "0", "0", "0", "0", False, False, 3,
            ["arabic", "spelling:uk", "eye:right_left", "eye:arabic"],
            "تقرير فحص الشبكية: لا يوجد اعتلال شبكي سكري.\nRight eye (العين اليمنى): normal fundus, clear foveal reflex. Left eye (العين اليسرى): healthy disc and vessels, no diabetic changes detected.",
        ),
        (
            "hs11", "0", "0", "0", "0", False, False, 4,
            ["spelling:uk", "eye:re_le", "negated_finding"],
            "Good quality 45-degree photographs obtained after tropicamide 1%. RE shows healthy neuroretinal rim and spotless posterior pole. LE shows normal retinal background; specifically no microaneurysms, no venous beading, no IRMA, and no maculopathy. Routine 12-month screening.",
        ),
        (
            "hs12", "0", "0", "0", "0", False, False, 5,
            ["spelling:us", "eye:right_left"],
            "Both the right eye and the left eye demonstrate normal retinal architecture on today's non-mydriatic camera capture. Foveal avascular zones appear intact without lipid or hemorrhage. Will invite back in one year.",
        ),

        # --- R1M0 (8 reports: hs13..hs20) ---
        (
            "hs13", "1", "0", "0", "0", False, True, 0,
            ["coded", "spelling:uk", "eye:right_left"],
            "Structured Grade: Right eye = R1 M0, Left eye = R0 M0. Right eye shows three isolated microaneurysms and one small dot haemorrhage nasal to the disc, well away from the macula. Left eye has no retinopathy. Annual screening recall.",
        ),
        (
            "hs14", "0", "1", "0", "0", False, True, 1,
            ["coded", "spelling:us", "eye:od_os"],
            "Coded Per-Eye Summary: OD R0 M0; OS R1 M0. OS demonstrates mild background diabetic retinopathy with scattered microaneurysms superior to the vascular arcade; macula is free of exudates or edema. OD is unremarkable.",
        ),
        (
            "hs15", "1", "1", "0", "0", False, True, 2,
            ["coded", "spelling:uk", "eye:re_le", "prev_grade_diff"],
            "Per-eye grades: RE R1 M0, LE R1 M0 (Progressed from R0M0 at 2025 screening). Bilateral mild background retinopathy with occasional dot haemorrhages and microaneurysms outside the macular grid. No maculopathy in either eye.",
        ),
        (
            "hs16", "1", "0", "0", "0", False, False, 3,
            ["arabic", "spelling:uk", "eye:right_left", "hard:foveal_ma_good_va"],
            "فحص الشبكية السكري - درجة بسيطة في العين اليمنى.\nVA: Right eye 6/6 (corrected), Left eye 6/6. Right eye shows two microaneurysms and a tiny dot haemorrhage located 0.5 disc diameters temporal to the foveal centre, with no exudates and no retinal thickening; because visual acuity is 6/6 (better than 6/12), M1 criteria are not met. Left eye is completely normal. Annual recall.",
        ),
        (
            "hs17", "1", "1", "0", "0", False, False, 4,
            ["spelling:uk", "eye:re_le", "hard:foveal_ma_good_va", "negated_finding"],
            "Visual acuity: RE 6/5, LE 6/9. Fundus grading: RE has a single microaneurysm within 1 DD of the fovea and three dot haemorrhages nasally, but no hard exudates, no retinal thickening, and vision is 6/5 so no referable maculopathy. LE shows two peripheral microaneurysms superiorly, no maculopathy, no new vessels. Background retinopathy only.",
        ),
        (
            "hs18", "0", "1", "0", "0", False, False, 5,
            ["spelling:us", "eye:od_os", "hard:distant_exudate"],
            "VA OD 20/20, OS 20/25. OD fundus is completely clear without lesions. OS shows a few microaneurysms and two small, isolated hard exudates located 2.5 disc diameters superior to the fovea (well outside the 1 DD macular zone and not forming a macular group). No macular thickening. Mild background retinopathy OS.",
        ),
        (
            "hs19", "1", "1", "0", "0", False, False, 0,
            ["spelling:uk", "eye:right_left", "ocr_noise", "negated_finding"],
            "Right eye: occasional microaneurysms and a single flame haemorrhage in the nasal periphery; macula is clear with no exudates. Left eye: four microaneurysms temporal to the macula (>2 DD from fovea), no venous beading, no IRMA, and no maculopathy. Background diabetic retinopathy bilaterally.",
        ),
        (
            "hs20", "1", "0", "0", "0", False, False, 1,
            ["spelling:uk", "eye:re_le", "prev_grade_diff"],
            "Previous grade 1 year ago was R0M0. Today RE shows early background changes with 4 microaneurysms along the inferotemporal arcade and a tiny blot haemorrhage; macula remains dry and clear. LE remains free of diabetic retinopathy. Continue annual screening.",
        ),

        # --- Referable non-urgent (14 reports: hs21..hs34) ---
        (
            "hs21", "1", "1", "1", "0", False, True, 0,
            ["coded", "spelling:uk", "eye:right_left"],
            "Structured Grade: Right eye R1 M1, Left eye R1 M0. Right eye demonstrates background microaneurysms plus a distinct ring of hard exudates encroaching to within 0.5 disc diameters of the fovea. Left eye shows a few nasal microaneurysms only. Routine referral to hospital eye services for right maculopathy.",
        ),
        (
            "hs22", "1", "0", "1", "0", False, False, 2,
            ["spelling:uk", "eye:re_le", "hard:va_driven_m1"],
            "VA: RE 6/18 (pinhole 6/12), LE 6/6. RE fundus shows no hard exudates and no obvious stereo thickening, but there are three microaneurysms and a small blot haemorrhage situated 0.6 disc diameters from the centre of the fovea; combined with reduced visual acuity of 6/12, this meets NHS M1 maculopathy criteria. LE is completely normal (R0M0). Refer to medical retina clinic.",
        ),
        (
            "hs23", "1", "1", "0", "1", False, False, 3,
            ["arabic", "spelling:us", "eye:od_os", "prev_grade_diff"],
            "تحويل لعيادة الشبكية بسبب اعتلال اللطخة الصفراء في العين اليسرى.\nPrior screening was R1M0 OU. Today OD shows mild background microaneurysms without macular involvement (R1M0). OS shows mild background retinopathy plus a cluster of hard lipid exudates and focal retinal thickening within 1 DD of the fovea (R1M1). Routine ophthalmology referral.",
        ),
        (
            "hs24", "2", "1", "0", "0", False, True, 4,
            ["coded", "spelling:uk", "eye:right_left", "negated_finding"],
            "Coded Grade: Right eye R2 M0, Left eye R1 M0. Right eye shows pre-proliferative diabetic retinopathy with multiple deep dark blot haemorrhages in all four quadrants and venous beading superotemporally; no new vessels and no maculopathy. Left eye has mild background microaneurysms only. Refer to hospital eye clinic.",
        ),
        (
            "hs25", "1", "2", "0", "0", False, False, 5,
            ["spelling:uk", "eye:re_le", "prev_grade_diff", "negated_finding"],
            "Upgraded from R1M0 last year. RE shows stable background microaneurysms and intact macula. LE now demonstrates clear intraretinal microvascular abnormalities (IRMA) nasal to the disc and venous reduplication/beading along the inferior arcade, without new vessels and without macular exudates. Routine referral for left pre-proliferative DR.",
        ),
        (
            "hs26", "2", "1", "1", "0", False, True, 1,
            ["coded", "spelling:us", "eye:od_os"],
            "System Coded Grade: OD = R2 M1, OS = R1 M0. OD exhibits venous loops, multiple deep blot hemorrhages, and circinate macular exudates within 0.5 DD of the foveal center. OS shows mild background microaneurysms with a dry macula. Refer to retina specialist.",
        ),
        (
            "hs27", "2", "2", "1", "1", False, False, 0,
            ["spelling:uk", "eye:right_left", "ocr_noise"],
            "Right eye: multiple deep blot haemorrhages, prominent IRMA, and group of macular exudates with retinal thickening at the fovea. Left eye: venous beading in two quadrants, deep haemorrhages, and exudates within 1 DD of the fovea. Bilateral pre-proliferative retinopathy with bilateral maculopathy. Routine HES referral.",
        ),
        (
            "hs28", "3S", "1", "0", "0", False, True, 2,
            ["coded", "spelling:uk", "eye:re_le", "hard:stable_prp_3s", "negated_finding"],
            "Coded Assessment: RE R3S M0, LE R1 M0. RE shows extensive, well-pigmented panretinal photocoagulation (PRP) laser scars in 360 degrees with a fibrosed, completely regressed vascular frond at the disc; no active new vessels or haemorrhage, macula dry. LE shows a few background microaneurysms only.",
        ),
        (
            "hs29", "1", "3S", "0", "0", False, False, 3,
            ["arabic", "spelling:uk", "eye:right_left", "hard:stable_prp_3s"],
            "آثار ليزر قديمة ومستقرة في الشبكية اليسرى.\nRight eye has two small background microaneurysms outside the arcade and a normal macula (R1M0). Left eye demonstrates old confluent peripheral PRP laser scars from prior treatment of proliferative disease; vessels are quiescent with no active neovascularisation and no macular oedema (R3SM0).",
        ),
        (
            "hs30", "3S", "3S", "0", "1", False, True, 4,
            ["coded", "spelling:us", "eye:od_os", "hard:stable_prp_3s"],
            "Structured Grade: OD R3S M0, OS R3S M1. Both eyes show quiescent, regressed proliferative retinopathy with full panretinal laser scars and no active neovascularization. However, OS now shows a new group of lipid exudates within 1 DD of the fovea. Refer back to medical retina.",
        ),
        (
            "hs31", "2", "U", "0", "", False, True, 5,
            ["coded", "spelling:uk", "eye:right_left", "hard:ungradable_asymmetry"],
            "Per-Eye Coded Grade: Right eye = R2 M0, Left eye = U. Right eye has clear media showing venous beading and multiple deep blot haemorrhages (pre-proliferative, macula clear). Left eye is completely ungradable due to a dense mature white cataract obscuring the fundus. Because the right eye has referable retinopathy on its own, overall grade is R2M0.",
        ),
        (
            "hs32", "U", "1", "", "1", False, False, 0,
            ["spelling:uk", "eye:re_le", "hard:ungradable_asymmetry"],
            "RE cannot be graded today (Grade U) owing to dense posterior subcapsular cataract and poor pupil dilation preventing any view of the disc or macula. LE has clear media and shows background microaneurysms together with hard exudates 0.4 DD from the fovea (R1M1). As LE is referable for maculopathy, overall report is graded from LE as R1M1.",
        ),
        (
            "hs33", "0", "1", "0", "1", False, True, 1,
            ["coded", "spelling:uk", "eye:right_left", "prev_grade_diff"],
            "Structured Grade: Right eye R0 M0, Left eye R1 M1 (Progressed from R1M0 at previous screening). Right eye remains normal. Left eye has developed a cluster of hard exudates and focal thickening temporal to and within 1 DD of the fovea.",
        ),
        (
            "hs34", "1", "1", "1", "1", False, False, 2,
            ["spelling:us", "eye:od_os"],
            "VA OD 20/30, OS 20/40. Bilateral mild background retinopathy with microaneurysms and dot hemorrhages. In addition, both OD and OS show rings of hard exudates within 1 disc diameter of the center of the fovea. Bilateral diabetic maculopathy; routine referral placed.",
        ),

        # --- Urgent / R3A (2 reports: hs35..hs36) ---
        (
            "hs35", "3A", "2", "1", "0", True, True, 3,
            ["coded", "arabic", "spelling:uk", "eye:right_left", "eye:arabic"],
            "تحويل عاجل جداً - نزيف وأوعية دموية جديدة نشطة في العين اليمنى.\nCoded Grade: Right eye (العين اليمنى) = R3A M1, Left eye (العين اليسرى) = R2 M0. Right eye shows active fine new vessels at the disc (NVD), a boat-shaped pre-retinal subhyaloid haemorrhage, and macular exudates within 1 DD of the fovea. Left eye shows venous beading and IRMA without maculopathy. URGENT referral requested to hospital eye service within 2 weeks.",
        ),
        (
            "hs36", "2", "1", "1", "0", True, False, 4,
            ["spelling:uk", "eye:re_le", "ocr_noise"],
            "RE shows severe pre-proliferative changes (R2) with extensive deep dark blot haemorrhages in all quadrants, marked venous loops, and heavy circinate exudates threatening the foveal centre (M1) with drop in vision to 6/18. LE shows mild background microaneurysms only (R1M0). Although no frank new vessels are seen yet, grader requests an URGENT referral to medical retina due to severe ischaemic R2 features and imminent threat to central vision.",
        ),

        # --- Ungradable U (4 reports: hs37..hs40) ---
        (
            "hs37", "U", "U", "", "", False, True, 0,
            ["coded", "spelling:uk", "eye:right_left"],
            "Structured Grade: Right eye = U, Left eye = U. Bilateral dense nuclear sclerotic and cortical cataracts completely obscure the posterior pole in both eyes. Neither optic disc nor macula can be graded. Refer to slit-lamp biomicroscopy / cataract clinic.",
        ),
        (
            "hs38", "U", "0", "", "0", False, True, 1,
            ["coded", "spelling:us", "eye:od_os", "hard:ungradable_asymmetry"],
            "Coded Output: OD = U, OS = R0 M0. OD image is ungradable due to central corneal scarring and small pupil (<2 mm). OS is clearly visualized and shows a completely normal retina with no diabetic retinopathy (R0M0). Because the fellow gradable eye is non-referable, overall screening outcome is U for technical/slit-lamp repeat.",
        ),
        (
            "hs39", "1", "U", "0", "", False, False, 5,
            ["spelling:uk", "eye:re_le", "hard:ungradable_asymmetry", "negated_finding"],
            "RE is well visualised and shows only mild background retinopathy (three peripheral microaneurysms, no maculopathy, no pre-proliferative features: R1M0). LE is ungradable (U) due to severe asteroid hyalosis and media opacity hiding more than 50% of the macular and disc fields. Because the gradable RE is not referable on its own, overall report grade is U.",
        ),
        (
            "hs40", "U", "U", "", "", False, False, 2,
            ["spelling:uk", "eye:right_left"],
            "Attempted non-mydriatic and mydriatic photography. Both right eye and left eye photographs are ungradable due to poor patient fixation, severe photophobia, and blink artefact across the macula in all frames. Refer to slit-lamp biomicroscopy clinic.",
        ),
    ]

    reports = []
    for idx, (rep_id, rr, rl, mr, ml, urg, is_coded, layout_id, tags, body) in enumerate(specs):
        lbl = compute_screening_label(rr, rl, mr, ml, urg)
        coded_val = lbl if is_coded else None
        site = sites[idx % len(sites)]
        date_str = f"2026-0{(idx % 9) + 1}-{(idx % 27) + 1:02d}"
        mrn = f"AD-SCR-{9100 + idx}"

        full_text = format_screening_layout(layout_id, site, date_str, mrn, body, coded_val)
        if "ocr_noise" in tags:
            full_text = apply_ocr_noise(full_text, rng)

        reports.append({
            "id": rep_id,
            "text": full_text,
            "key": {
                "r_right": rr,
                "r_left": rl,
                "m_right": mr,
                "m_left": ml,
                "urgent": urg,
                "label": lbl,
                "coded": coded_val,
            },
            "tags": tags + [f"layout:{layout_id}"],
        })

    return reports


def build_clinic_notes(rng):
    hospitals = [
        "SSMC Medical Retina Unit",
        "Cleveland Clinic Abu Dhabi Eye Institute",
        "Tawam Hospital Ophthalmology Clinic",
        "Sheikh Khalifa Medical City (SKMC) Eye Clinic",
        "Al Rahba Hospital Eye Outpatient Dept",
        "Corniche / SEHA Specialized Eye Center",
    ]

    # 50 clinic specs: 16 carry hard:* tags (32.0%)
    # (id, attended, fundus, outcome, layout_id, tags, body_text)
    specs = [
        # --- examined_treated (10 notes: hc01..hc10) ---
        (
            "hc01", True, "examined", "treated", 0,
            ["spelling:uk", "eye:right_left"],
            "Referred from DESP with R3AM1 right eye. VA: Right eye 6/18, Left eye 6/9. Slit-lamp dilated fundus examination after tropicamide/phenylephrine: Right eye confirms active neovascularisation at the disc (NVD) and centre-involving diabetic macular oedema (OCT CST 435 um). Left eye shows moderate NPDR with dry macula. Plan: Intravitreal Aflibercept (Eylea) 2 mg/0.05 mL injection administered to the right eye today under aseptic protocol, and Session 1 panretinal photocoagulation (PRP) laser booked for next week.",
        ),
        (
            "hc02", True, "examined", "treated", 1,
            ["spelling:us", "eye:od_os"],
            "Post-screening referral for bilateral macular edema. VA OD 20/40, OS 20/50. Dilated biomicroscopy and OCT macula performed: OD shows moderate NPDR with central cystoid macular edema (CST 390 um); OS shows severe NPDR with subfoveal fluid and lipid exudates (CST 460 um). Treatment: Bilateral intravitreal Faricimab (Vabysmo) loading course initiated today (OS injected today, OD scheduled in 7 days).",
        ),
        (
            "hc03", True, "examined", "treated", 2,
            ["arabic", "spelling:uk", "eye:re_le", "eye:arabic"],
            "تم فحص قاع العين وإعطاء حقنة داخل الجسم الزجاجي للعين اليمنى.\nVA RE (العين اليمنى) 6/24, LE (العين اليسرى) 6/6. Dilated 90D fundus exam: RE shows clinically significant macular oedema with circinate exudates threatening the fovea; LE shows mild background DR only. Intravitreal Ranibizumab (Lucentis) 0.5 mg given to RE today without complication.",
        ),
        (
            "hc04", True, "examined", "treated", 3,
            ["spelling:uk", "eye:right_left"],
            "Referred with active proliferative DR left eye. VA Right 6/9, Left 6/12. Dilated fundus exam: Right eye has quiescent old laser scars (stable). Left eye demonstrates active new vessels elsewhere (NVE) along the superotemporal arcade with a small preretinal blot haemorrhage; macula is flat. Action: Urgent fill-in panretinal photocoagulation (PRP) laser performed to the left eye today (1,100 burns, Pascal pattern).",
        ),
        (
            "hc05", True, "examined", "treated", 4,
            ["spelling:uk", "eye:re_le"],
            "Thank you for referring this pseudophakic patient with persistent left diabetic macular oedema. On dilated examination today, RE shows mild NPDR (M0) and LE shows chronic cystoid macular thickening (OCT 480 um) refractory to prior anti-VEGF. We implanted an intravitreal Dexamethasone 700 mcg (Ozurdex) slow-release steroid implant into the LE today.",
        ),
        (
            "hc06", True, "examined", "treated", 5,
            ["spelling:us", "eye:od_os", "ocr_noise"],
            "S: Referred from screening for R2M1 OD. O: VA OD 20/30, OS 20/20. Dilated fundus exam: OD has non-center-involving focal macular edema from a leaking microaneurysm cluster 600 microns superotemporal to fovea; OS has mild NPDR. A/P: Focal/grid Navilas macular laser photocoagulation performed to OD today.",
        ),
        (
            "hc07", True, "examined", "treated", 0,
            ["spelling:uk", "eye:right_left"],
            "Patient attended following screening referral. Dilated slit-lamp examination shows florid NVD and NVE in the right eye and severe pre-proliferative changes in the left eye. Consent obtained and bilateral PRP laser treatment booked (right eye urgent slot tomorrow morning, left eye next week).",
        ),
        (
            "hc08", True, "examined", "treated", 1,
            ["spelling:us", "eye:od_os"],
            "VA OD 20/60, OS 20/25. Dilated retinal exam and OCT confirm center-involving diabetic macular edema OD (CST 510 um) with moderate NPDR; OS has mild NPDR without edema. Patient consented and booked for course of 3 monthly intravitreal Bevacizumab injections OD starting in Thursday's injection list.",
        ),
        (
            "hc09", True, "examined", "treated", 2,
            ["arabic", "spelling:uk", "eye:re_le"],
            "خطة العلاج: جلسة ليزر للشبكية اليسرى اليوم.\nDilated funduscopy: RE shows mild background retinopathy (no treatment required). LE shows new vessels at the disc (NVD) with clear media. Completed first session of panretinal photocoagulation (PRP) laser to LE today (850 burns).",
        ),
        (
            "hc10", True, "examined", "treated", 3,
            ["spelling:uk", "eye:right_left"],
            "Attended medical retina clinic. Dilated fundus exam reveals right eye high-risk proliferative diabetic retinopathy with preretinal haemorrhage inferiorly and centre-involving macular oedema. Administered intravitreal Aflibercept to the right eye today and booked PRP laser in 2 weeks.",
        ),

        # --- examined_followup (10 notes: hc11..hc20) ---
        (
            "hc11", True, "examined", "followup", 4,
            ["spelling:uk", "eye:right_left", "negated_treatment"],
            "Referred from screening with R2M0 right eye. VA Right 6/6, Left 6/6. Dilated slit-lamp biomicroscopy confirms moderate-to-severe non-proliferative DR in the right eye (venous beading and blot haemorrhages) and mild NPDR in the left eye. OCT maculae are completely flat and dry bilaterally. No neovascularisation seen; PRP laser not needed and anti-VEGF injection not indicated at this stage. Plan: Close observation in Medical Retina clinic in 4 months with widefield OCT-A.",
        ),
        (
            "hc12", True, "examined", "followup", 5,
            ["spelling:us", "eye:od_os", "negated_treatment"],
            "S: Screening referral for M1 OS. O: VA OD 20/20, OS 20/20. Dilated fundus exam and spectral-domain OCT: OD mild NPDR. OS shows a small cluster of hard exudates 450 microns temporal to the fovea with a normal central subfield thickness (255 um) and no intraretinal cysts. A/P: Good vision and non-center-involving lipid without thickening; intravitreal injection not required today and laser deferred. Follow up in 4 months with repeat OCT.",
        ),
        (
            "hc13", True, "examined", "followup", 0,
            ["spelling:uk", "eye:re_le", "negated_treatment", "hard:historical_tx_only"],
            "VA RE 6/9, LE 6/9. Past ocular history: RE underwent 3x intravitreal Ranibizumab injections in 2022 and full PRP laser in 2021. Today's dilated fundus examination shows quiescent, well-pigmented PRP scars in RE with regressed vessels and a completely dry macula on OCT (CST 248 um); LE shows moderate NPDR with no macular oedema. No active disease; no laser or injection needed today. Review in clinic in 6 months.",
        ),
        (
            "hc14", True, "examined", "followup", 1,
            ["spelling:us", "eye:od_os", "negated_treatment"],
            "Consult for bilateral pre-proliferative DR. VA 20/25 OU. Dilated exam with 78D lens shows IRMA and deep blot hemorrhages in both OD and OS, but no NVD/NVE and no clinically significant macular edema on OCT. Panretinal laser photocoagulation is not indicated right now. Will order ultra-widefield fluorescein angiography and review in 3 months.",
        ),
        (
            "hc15", True, "examined", "followup", 2,
            ["arabic", "spelling:uk", "eye:right_left", "hard:patient_declined_tx"],
            "المريض رفض أخذ الحقنة اليوم وفضل المتابعة فقط.\nVA Right 6/15, Left 6/9. Dilated fundus exam and OCT show right eye diabetic macular oedema (CST 375 um) and left eye mild NPDR. We discussed intravitreal anti-VEGF injection for the right eye, but the patient declined any injection or laser treatment today (nothing given or booked) and requested conservative observation while optimising blood pressure and HbA1c. Follow-up in 8 weeks for repeat OCT.",
        ),
        (
            "hc16", True, "examined", "followup", 3,
            ["spelling:uk", "eye:re_le", "hard:historical_tx_only", "negated_treatment"],
            "Patient attended for post-screening review. History of focal macular laser to LE three years ago (2023) and intravitreal Eylea course completed 18 months ago. Today's dilated slit-lamp exam shows old pigmented macular laser scars in LE, flat retinas bilaterally, and moderate NPDR in RE. OCT shows no fluid. No further injections or laser required now; routine clinic follow-up in 6 months.",
        ),
        (
            "hc17", True, "examined", "followup", 4,
            ["spelling:uk", "eye:right_left", "ocr_noise"],
            "Referred as R2M1 left eye. On dilated examination today, right eye has mild background retinopathy and left eye has moderate NPDR with a single parafoveal microaneurysm and trace extrafoveal thickening (VA 6/6). No treatment needed today. Will review in hospital eye clinic in 4 months.",
        ),
        (
            "hc18", True, "examined", "followup", 5,
            ["spelling:us", "eye:od_os"],
            "S: 1st trimester pregnant patient referred with R2M0 OU. O: VA 20/20 OU. Dilated fundus exam confirms moderate NPDR in both OD and OS with cotton wool spots and blot hemorrhages; maculae are dry on OCT. A/P: No treatment needed now, but given pregnancy risk of progression, keep under close hospital surveillance with follow-up in 8 weeks.",
        ),
        (
            "hc19", True, "examined", "followup", 0,
            ["spelling:uk", "eye:re_le", "hard:patient_declined_tx"],
            "Dilated fundus examination performed. RE shows severe NPDR (4-2-1 rule met by haemorrhages) without NVD/NVE; LE shows moderate NPDR. OCT macula normal in both eyes. We offered early prophylactic panretinal laser photocoagulation to RE, but the patient declined laser treatment today in favour of watchful waiting; arranged 3-month clinic review with widefield imaging.",
        ),
        (
            "hc20", True, "examined", "followup", 1,
            ["spelling:us", "eye:right_left", "hard:patient_declined_tx"],
            "VA Right 20/30, Left 20/25. Dilated biomicroscopy shows bilateral moderate NPDR and mild parafoveal thickening in the right eye (OCT CST 335 um). Intravitreal anti-VEGF injection was discussed as an option, but the patient declined any injection today and elected close observation while starting fenofibrate; return to retina clinic in 3 months.",
        ),

        # --- examined_discharged (10 notes: hc21..hc30) ---
        (
            "hc21", True, "examined", "discharged", 2,
            ["arabic", "spelling:uk", "eye:right_left", "negated_treatment", "hard:false_positive_referral"],
            "لا يحتاج لتدخل علاجي - يعاد لبرنامج الفحص السنوي.\nReferred from screening as query R1M1 right eye (suspected macular exudate). VA Right 6/6, Left 6/6. Dilated 90D fundus examination and macular OCT today show a small benign hard drusen temporal to the right fovea (mistaken for lipid exudate on 2D screening photo) and only two peripheral microaneurysms (R1M0); left eye is normal (R0M0). No laser or injection required. Discharged back to the annual diabetic eye screening programme.",
        ),
        (
            "hc22", True, "examined", "discharged", 3,
            ["spelling:us", "eye:od_os", "hard:false_positive_referral"],
            "Referred for suspected IRMA OS (R2M0). VA 20/20 OU. Careful dilated slit-lamp biomicroscopy and OCT angiography confirm a congenital retinal vascular tortuosity loop OS without true IRMA, venous beading, or neovascularization. Mild background microaneurysms only OU. Patient discharged back to primary-care diabetic eye screening recall.",
        ),
        (
            "hc23", True, "examined", "discharged", 4,
            ["spelling:uk", "eye:re_le", "hard:historical_tx_only"],
            "Referred with R3SM0 in RE. Patient had panretinal photocoagulation laser to RE in 2018. Today's dilated fundus examination shows completely quiet, stable pigmented PRP scars in RE with no active new vessels for over 7 years, and no retinopathy in LE; OCT maculae are dry. Discharged from hospital eye service back to annual DESP digital surveillance.",
        ),
        (
            "hc24", True, "examined", "discharged", 5,
            ["spelling:uk", "eye:right_left", "ocr_noise"],
            "S: Screening grade U due to small pupils. O: After pharmacological mydriasis with tropicamide 1% and phenylephrine 2.5%, clear view of both fundi achieved on slit-lamp biomicroscopy. Right eye and left eye show completely healthy discs, vessels, and maculae (R0M0 bilaterally). A/P: Discharged back to annual diabetic eye screening programme.",
        ),
        (
            "hc25", True, "examined", "discharged", 0,
            ["spelling:us", "eye:od_os"],
            "Referred for single flame hemorrhage near the disc OD. VA 20/20 OU. Dilated fundus exam today (6 weeks post-screening) shows complete resolution of the tiny nerve fiber layer hemorrhage OD; no microaneurysms, no macular edema, and normal OCT in both OD and OS. Discharged to routine annual screening.",
        ),
        (
            "hc26", True, "examined", "discharged", 1,
            ["spelling:uk", "eye:re_le", "hard:false_positive_referral"],
            "VA RE 6/5, LE 6/6. Referred with possible macular exudate LE (R1M1). Dilated biomicroscopy and OCT show a tiny Gunn's dot / internal limiting membrane sheen reflection in LE with normal retinal thickness (240 um) and zero exudates or microaneurysms in either eye. Discharged back to community diabetic eye screening.",
        ),
        (
            "hc27", True, "examined", "discharged", 2,
            ["arabic", "spelling:uk", "eye:right_left"],
            "تم فحص الشبكية بالعدسة المكبرة والنتيجة مطمئنة.\nReferred following ungradable screening photos due to blink artefact. Today on dilated slit-lamp exam, both right eye and left eye are clearly visualised and show mild background retinopathy only (a few peripheral microaneurysms, dry maculae). Returned/discharged to the annual DESP recall list.",
        ),
        (
            "hc28", True, "examined", "discharged", 3,
            ["spelling:us", "eye:od_os"],
            "Consultation for query maculopathy OD. VA OD 20/20, OS 20/20. Dilated fundus exam and macular cube OCT reveal a solitary microaneurysm 900 microns from the fovea OD with normal foveal contour, no exudates, and 20/20 vision; OS is free of retinopathy. Does not require hospital follow-up; discharged back to screening program.",
        ),
        (
            "hc29", True, "examined", "discharged", 4,
            ["spelling:uk", "eye:re_le"],
            "We examined this patient's retinas after dilation today. Both RE and LE show only very mild background diabetic changes (R1M0) with normal OCT profiles and no pre-proliferative or macular disease. We have discharged the patient back to the primary care screening programme for annual photography.",
        ),
        (
            "hc30", True, "examined", "discharged", 5,
            ["spelling:uk", "eye:right_left", "hard:false_positive_referral"],
            "S: Referred for query NVE right eye (R3AM0 on screening referral). O: Dilated 90D and contact lens fundus examination right and left eyes: the suspected neovascular lesion in the right eye is a benign epiretinal membrane with mild retinal vessel straightening, not neovascularisation. No diabetic maculopathy. A/P: Discharged to annual diabetic eye screening.",
        ),

        # --- not_examined (8 notes: hc31..hc38) ---
        (
            "hc31", True, "not_examined", "followup", 0,
            ["spelling:uk", "eye:right_left", "va_no_retinal_exam", "hard:anterior_seg_only"],
            "Patient attended clinic. Nursing triage recorded VA: Right eye 6/9, Left eye 6/9; IOP: Right 16 mmHg, Left 15 mmHg. Slit-lamp examination of the anterior segment showed clear corneas and quiet anterior chambers, but patient had driven himself to the hospital and refused dilating drops today as he had to drive back to Al Ain immediately. Undilated view through 2 mm pupils was not attempted for grading; dilated fundus exam deferred. Rebooked for dilated retinal examination in 2 weeks.",
        ),
        (
            "hc32", True, "not_examined", "followup", 1,
            ["spelling:us", "eye:od_os", "va_no_retinal_exam", "hard:anterior_seg_only"],
            "VA OD 20/25, OS 20/30; IOP 24 mmHg OD, 25 mmHg OS. Anterior segment slit-lamp exam and gonioscopy revealed occludable narrow drainage angles (Shaffer Grade 1) in both eyes. Pharmacological pupil dilation was withheld today due to acute angle-closure risk, so the retina/fundus was not examined. Booked bilateral YAG laser peripheral iridotomy first, followed by dilated fundus exam.",
        ),
        (
            "hc33", True, "not_examined", "followup", 2,
            ["arabic", "spelling:uk", "eye:re_le", "va_no_retinal_exam"],
            "تم قياس النظر وضغط العين فقط ولم يتم فحص قاع العين اليوم.\nVA RE 6/12, LE 6/9; IOP RE 14, LE 14. Tropicamide drops were instilled at 10:15, but the patient developed vasovagal dizziness and nausea in the waiting room before seeing the ophthalmologist and asked to go home before any fundus or OCT examination could be performed. Rescheduled clinic appointment in 3 weeks.",
        ),
        (
            "hc34", True, "not_examined", "followup", 3,
            ["spelling:uk", "eye:right_left", "va_no_retinal_exam"],
            "Triage completed: Visual acuity Right eye 6/6, Left eye 6/12; Goldmann IOP 17/18 mmHg; autorefraction recorded. Due to an emergency theatre call-out and a 2-hour clinic delay, the patient had to leave for dialysis before dilation or retinal examination took place. Fundus not examined today; priority re-booking arranged.",
        ),
        (
            "hc35", True, "not_examined", "followup", 4,
            ["spelling:us", "eye:od_os", "va_no_retinal_exam", "hard:anterior_seg_only"],
            "Patient presented for screening referral, but complained of acute red sticky right eye for 2 days. VA OD 20/30, OS 20/20. Anterior segment exam showed florid acute adenoviral follicular conjunctivitis OD with contagious discharge. To prevent cross-infection and corneal toxicity, contact/dilated fundus examination and OCT imaging were deferred today. Prescribed lubricants and rebooked for retinal exam in 3 weeks.",
        ),
        (
            "hc36", True, "not_examined", "followup", 5,
            ["spelling:uk", "eye:re_le", "va_no_retinal_exam"],
            "S: Attended for DR referral. O: VA RE 6/9, LE 6/9; IOP 13 mmHg OU. Patient arrived 90 minutes late at 16:45 just as clinic was closing; optometrist checked visual acuity and pressures, but there was insufficient time for pharmacological mydriasis and dilated fundus examination. A/P: Retina not examined; rebooked for next Monday morning.",
        ),
        (
            "hc37", True, "not_examined", "followup", 0,
            ["spelling:uk", "eye:right_left", "va_no_retinal_exam"],
            "Visual acuity measured (Right 6/18, Left 6/12) and intraocular pressure checked (15/16 mmHg). Patient is severely needle- and drop-phobic and declined mydriatic drops and slit-lamp bright light examination today; only external eye inspection performed, fundus not examined. Will rebook in dedicated widefield non-mydriatic camera slot.",
        ),
        (
            "hc38", True, "not_examined", "followup", 1,
            ["spelling:us", "eye:od_os", "va_no_retinal_exam"],
            "Checked in at Eye Institute: VA OD 20/20, OS 20/25; blood glucose 19.4 mmol/L; BP 195/112 mmHg. Because of symptomatic hypertensive urgency, patient was transferred directly from ophthalmic nursing triage to the Emergency Department before pupil dilation or fundus examination was performed.",
        ),

        # --- not_assessable / no_view (5 notes: hc39..hc43) ---
        (
            "hc39", True, "no_view", "followup", 2,
            ["arabic", "spelling:uk", "eye:right_left", "eye:arabic", "ocr_noise"],
            "تعذر رؤية الشبكية بسبب عتامة شديدة في العدسة والجسم الزجاجي.\nVA Right eye (العين اليمنى) Hand Movements, Left eye (العين اليسرى) Counting Fingers. Pupils dilated fully to 8 mm, but on slit-lamp biomicroscopy and indirect ophthalmoscopy there is NO VIEW of the fundus in either eye: Right eye has a dense diffuse vitreous haemorrhage obscuring the disc and retina; Left eye has a mature brunescent white cataract preventing any red reflex. B-scan ultrasound shows flat retinas bilaterally. Booked B-scan review and cataract/vitreoretinal assessment.",
        ),
        (
            "hc40", True, "no_view", "followup", 3,
            ["spelling:us", "eye:od_os"],
            "Referred for ungradable screening photos. VA OD 20/200, OS 20/100. After full pharmacological dilation, attempted 90D and 20D indirect ophthalmoscopy and OCT: dense bilateral nuclear and posterior subcapsular cataracts completely block any view of the posterior pole in both OD and OS (fundus not visualizable). Referred to anterior segment list for bilateral phacoemulsification + IOL so the retina can be assessed post-operatively.",
        ),
        (
            "hc41", True, "no_view", "followup", 4,
            ["spelling:uk", "eye:re_le"],
            "Patient attended following screening Grade U. On examination today, RE has dense central corneal leukoma and band keratopathy, and LE has dense old vitreous haemorrhage with 360-degree posterior synechiae. Despite maximal mydriatic drops, the retina could not be seen in either eye on slit-lamp or indirect ophthalmoscopy. B-scan ultrasonography performed (no tractional detachment detected).",
        ),
        (
            "hc42", True, "no_view", "followup", 5,
            ["spelling:uk", "eye:right_left"],
            "S: Sudden drop in vision right eye, longstanding poor vision left eye. O: VA Right CF, Left HM. Dilated fundus exam attempted: zero view of the right fundus due to dense fresh vitreous haemorrhage, and no view of the left fundus due to dense mature cataract and fibrotic posterior capsule. A/P: Retina not assessable clinically; B-scan flat, urgent VR referral.",
        ),
        (
            "hc43", True, "no_view", "followup", 0,
            ["spelling:us", "eye:od_os"],
            "Attempted dilated retinal exam and OCT today. Both OD and OS have severe corneal edema from decompensated Fuchs endothelial dystrophy plus dense 4+ nuclear sclerosis; neither optic nerve nor macula can be visualized in either eye. B-scan ultrasound shows attached retina OU. Follow up in cornea/cataract clinic.",
        ),

        # --- dna (7 notes: hc44..hc50) ---
        (
            "hc44", False, None, "none", 1,
            ["spelling:uk", "eye:right_left", "hard:dna_with_referral_text"],
            "ADMINISTRATIVE ENTRY - DID NOT ATTEND (DNA).\nReferral text on file from DESP screening stated: 'Right eye R3AM1 with active NVD and macular exudates, Left eye R2M0; urgent hospital assessment for panretinal laser photocoagulation and anti-VEGF injection.'\nOutcome today: Patient failed to attend their scheduled 09:30 appointment and did not answer three telephone calls. Because the referral indicates high-risk R3A disease, a further urgent appointment slot has been sent by SMS and registered post.",
        ),
        (
            "hc45", False, None, "none", 2,
            ["arabic", "spelling:uk", "eye:re_le", "hard:dna_with_referral_text"],
            "لم يحضر المريض للموعد المحدد في عيادة الشبكية (Did Not Attend).\nNote: Referred from primary care screening with RE R1M1 (macular oedema) and LE R1M0 for OCT and possible intravitreal injection. Patient did not check in at reception today. Second appointment offered in 3 weeks.",
        ),
        (
            "hc46", False, None, "none", 3,
            ["spelling:us", "eye:od_os", "hard:dna_with_referral_text"],
            "NO SHOW / DNA NOTE.\nReason for appointment: Screening referral for OD R2M1, OS R2M0 (pre-proliferative diabetic retinopathy and macular thickening). Patient did not attend today's Medical Retina clinic session. Nurse coordinator contacted next of kin; patient is currently traveling outside the UAE. Will re-schedule upon return.",
        ),
        (
            "hc47", False, None, "none", 4,
            ["spelling:uk", "eye:right_left"],
            "The patient did not attend (DNA) their outpatient ophthalmology appointment today following referral from the diabetic eye screening programme (right eye and left eye background/macular changes). No clinical examination took place. A re-booking letter has been generated.",
        ),
        (
            "hc48", False, None, "none", 5,
            ["spelling:us", "eye:od_os"],
            "S/O: Patient was booked in the 11:00 slot for evaluation of OD and OS diabetic retinopathy referral, but was a No-Show / Did Not Attend. A/P: Chart closed as DNA; automated recall SMS triggered.",
        ),
        (
            "hc49", False, None, "none", 0,
            ["spelling:uk", "eye:re_le"],
            "DNA (Did Not Attend). Patient booked for post-screening biomicroscopy of RE and LE. Called twice in waiting area with no response. Referrer notified and one further appointment allocated.",
        ),
        (
            "hc50", False, None, "none", 1,
            ["spelling:uk", "eye:right_left"],
            "Appointment status: Did Not Attend (DNA). Patient was referred for assessment of right and left eye diabetic retinopathy, but telephoned on the morning of clinic to state they had no transport and could not attend. Rescheduled.",
        ),
    ]

    notes = []
    for idx, (note_id, att, fun, outc, layout_id, tags, body) in enumerate(specs):
        lbl = compute_clinic_label(att, fun, outc)
        hosp = hospitals[idx % len(hospitals)]
        date_str = f"2026-0{(idx % 9) + 1}-{(idx % 27) + 1:02d}"
        mrn = f"AD-CLN-{8200 + idx}"

        full_text = format_clinic_layout(layout_id, hosp, date_str, mrn, body)
        if "ocr_noise" in tags:
            full_text = apply_ocr_noise(full_text, rng)

        notes.append({
            "id": note_id,
            "text": full_text,
            "key": {
                "attended": att,
                "fundus": fun,
                "outcome": outc,
                "label": lbl,
            },
            "tags": tags + [f"layout:{layout_id}"],
        })

    return notes


def verify_dataset(screen_reports, clinic_notes):
    """Self-check all constraints before writing files."""
    assert len(screen_reports) == 40, f"Expected 40 screening reports, got {len(screen_reports)}"
    assert len(clinic_notes) == 50, f"Expected 50 clinic notes, got {len(clinic_notes)}"

    all_texts = [r["text"] for r in screen_reports] + [r["text"] for r in clinic_notes]
    assert len(set(all_texts)) == 90, "All 90 texts must be unique"

    # Check IDs
    assert [r["id"] for r in screen_reports] == [f"hs{i:02d}" for i in range(1, 41)]
    assert [r["id"] for r in clinic_notes] == [f"hc{i:02d}" for i in range(1, 51)]

    # Verify labels match rules
    for r in screen_reports:
        k = r["key"]
        expected = compute_screening_label(k["r_right"], k["r_left"], k["m_right"], k["m_left"], k["urgent"])
        assert k["label"] == expected, f"Mismatch on {r['id']}: {k['label']} != {expected}"
        if k["coded"] is not None:
            assert k["coded"] == k["label"]

    for r in clinic_notes:
        k = r["key"]
        expected = compute_clinic_label(k["attended"], k["fundus"], k["outcome"])
        assert k["label"] == expected, f"Mismatch on {r['id']}: {k['label']} != {expected}"

    # Check hard tag percentage (>= 30% in each file: >= 12 in screening, >= 15 in clinic)
    s_hard = sum(1 for r in screen_reports if any(t.startswith("hard:") for t in r["tags"]))
    c_hard = sum(1 for r in clinic_notes if any(t.startswith("hard:") for t in r["tags"]))
    assert s_hard >= 12, f"Expected >= 12 hard screening reports, got {s_hard}"
    assert c_hard >= 15, f"Expected >= 15 hard clinic notes, got {c_hard}"


def main():
    rng = random.Random(SEED)
    screen_reports = build_screening_reports(rng)
    clinic_notes = build_clinic_notes(rng)
    verify_dataset(screen_reports, clinic_notes)

    out_dir = os.path.dirname(os.path.abspath(__file__))
    screen_path = os.path.join(out_dir, "screen_reports.json")
    clinic_path = os.path.join(out_dir, "clinic_notes.json")

    screen_payload = {
        "source": SOURCE_STR,
        "reports": screen_reports,
    }
    clinic_payload = {
        "source": SOURCE_STR,
        "reports": clinic_notes,
    }

    with open(screen_path, "w", encoding="utf-8", newline="\n") as f:
        json.dump(screen_payload, f, ensure_ascii=False, indent=2)
        f.write("\n")

    with open(clinic_path, "w", encoding="utf-8", newline="\n") as f:
        json.dump(clinic_payload, f, ensure_ascii=False, indent=2)
        f.write("\n")


if __name__ == "__main__":
    main()
