#!/usr/bin/env python3
"""
Deterministic generator of synthetic breast screening and diagnostic assessment
reports from Abu Dhabi (SEHA, CCAD, Mediclinic, NMC, Burjeel, Mubadala Health).
Standard library only. Seed 9377.
"""

import json
import os
import random

SEED = 9377
SOURCE_STR = (
    "Independent author (Gemini 4 Argon, Google) - deterministic stdlib "
    "generator gen_breast_heldout.py, random seed 9377, synthetic breast "
    "screening texts, 2026-11-01"
)

CPT_MAP = {
    "B0": "3340F",
    "B1": "3341F",
    "B2": "3342F",
    "B3": "3343F",
    "B4": "3344F",
    "B5": "3345F",
}

ACR_RANK = {"1": 1, "2": 2, "3": 3, "0": 4, "4": 5, "5": 6}


def compute_screen_label(b_right: str, b_left: str) -> str:
    """Compute screening label according to exact specification."""
    if b_right == "TR" or b_left == "TR":
        other = b_left if b_right == "TR" else b_right
        if other in ("0", "3", "4", "5"):
            return "B" + other
        return "TR"
    r_rank = ACR_RANK[b_right]
    l_rank = ACR_RANK[b_left]
    worse = b_right if r_rank >= l_rank else b_left
    return "B" + worse


def compute_assess_label(
    attended: bool, assessment: str, biopsy: str, outcome: str
) -> str:
    """Compute assessment/MDT label according to exact specification."""
    if not attended:
        return "dna"
    if assessment == "not_done" or biopsy == "pending" or outcome == "pending":
        return "incomplete"
    if outcome == "treatment" and biopsy == "malignant":
        return "to_treatment"
    if outcome == "routine" and biopsy != "malignant":
        return "return_routine"
    if outcome == "early_recall" and biopsy != "malignant":
        return "early_recall"
    raise ValueError(
        f"Disallowed assessment key combination: attended={attended}, "
        f"assessment={assessment}, biopsy={biopsy}, outcome={outcome}"
    )


def apply_ocr_noise(text: str, rng: random.Random) -> str:
    """Apply deterministic OCR / fax scan artifacts to selected tokens."""
    replacements = [
        ("mammogram", "mamm0gram"),
        ("Mammography", "Mamm0graphy"),
        ("calcifications", "ca1cifications"),
        ("microcalcifications", "micr0calcifications"),
        ("fibroglandular", "fibr0glandular"),
        ("architectural", "architectura1"),
        ("BI-RADS", "B1-RADS"),
        ("bilateral", "bi1ateral"),
        ("ultrasound", "u1trasound"),
        ("Assessment", "Assessrnent"),
        ("Screening", "Screen1ng"),
        (" parenchymal ", " parenchyrna1 "),
    ]
    out = text
    applied = 0
    for old, new in replacements:
        if old in out and (rng.random() < 0.65 or applied < 2):
            out = out.replace(old, new, 1)
            applied += 1
    header_noise = rng.choice(
        [
            "[FAX RCVD +971-2-819-4002 P.01/01 QUALITY: DEGRADED ~~~~]\n",
            "*** SCANNED DOCUMENT - OCR CONFIDENCE 84.2% - DO NOT DISCARD ***\n",
            "|| TIF_SCAN_009482_ABUDHABI_PACS || ~~ l1l1 ~~~\n",
        ]
    )
    return header_noise + out


def build_screening_reports(rng: random.Random) -> list:
    """Generate 40 unique screening mammogram reports from Abu Dhabi units."""
    specs = [
        # --- 20 CODED SYNOPTIC REPORTS (hb01 .. hb20) ---
        {
            "id": "hb01",
            "b_right": "1",
            "b_left": "1",
            "overall": "1",
            "coded": "3341F",
            "layout": "synoptic_seha",
            "tags": ["coded_synoptic", "layout:synoptic_seha", "spelling:UK", "negation"],
            "body": (
                "COMPOSITION: ACR b (scattered areas of fibroglandular density).\n"
                "COMPARISON: Screening mammogram dated 14/05/2024 (then graded BI-RADS 2 for a right simple cyst, now resolved).\n"
                "RIGHT BREAST: Normal parenchymal architecture. No suspicious mass, no architectural distortion, and no malignant-type microcalcifications.\n"
                "LEFT BREAST: Normal fibroglandular tissue. No discrete mass or suspicious calcification.\n"
                " AXILLAE: Unremarkable bilaterally.\n"
                "LATERALITY SCORES: Right = BI-RADS 1 | Left = BI-RADS 1.\n"
                "OVERALL FINAL ASSESSMENT: BI-RADS 1 (Negative).\n"
                "CPT-II CODE: 3341F | ROUTINE RECALL: 24 months."
            ),
            "extra_tags": ["prior_diff_cat"],
        },
        {
            "id": "hb02",
            "b_right": "1",
            "b_left": "1",
            "overall": "1",
            "coded": "3341F",
            "layout": "synoptic_doh_bilingual",
            "tags": ["coded_synoptic", "layout:synoptic_doh_bilingual", "arabic", "spelling:US"],
            "body": (
                "برنامج الفحص المبكر لسرطان الثدي - دائرة الصحة أبوظبي\n"
                "DOH ABU DHABI BREAST CANCER SCREENING REGISTRY - SYNOPTIC EXTRACT\n"
                "Breast Density: ACR A (almost entirely fatty).\n"
                "Right Breast (الثدي الأيمن): Negative. No masses, calcifications, or skin thickening. Category: 1.\n"
                "Left Breast (الثدي الأيسر): Negative. Symmetric fatty stroma. Category: 1.\n"
                "Final Combined Category: BI-RADS 1 (Negative / سليم).\n"
                "Billing / Quality Code: 3341F."
            ),
            "extra_tags": [],
        },
        {
            "id": "hb03",
            "b_right": "1",
            "b_left": "1",
            "overall": "1",
            "coded": "3341F",
            "layout": "synoptic_ccad",
            "tags": ["coded_synoptic", "layout:synoptic_ccad", "spelling:US", "hard:family_hx_distractor"],
            "body": (
                "CLEVELAND CLINIC ABU DHABI - IMAGING INSTITUTE SYNOPTIC SCREEN\n"
                "Clinical History: Asymptomatic 48yo female. Strong family history: mother had invasive ductal carcinoma (BI-RADS 5 at diagnosis in 2019) and sister had DCIS.\n"
                "Parenchymal Density: ACR C (heterogeneously dense).\n"
                "RT Breast: No mass, distortion, or suspicious calcifications. [BI-RADS 1]\n"
                "LT Breast: No mass, distortion, or suspicious calcifications. [BI-RADS 1]\n"
                "Overall Assessment: BI-RADS 1 (Negative) | CPT-II: 3341F."
            ),
            "extra_tags": ["negation"],
        },
        {
            "id": "hb04",
            "b_right": "1",
            "b_left": "1",
            "overall": "1",
            "coded": "3341F",
            "layout": "synoptic_mediclinic",
            "tags": ["coded_synoptic", "layout:synoptic_mediclinic", "spelling:UK"],
            "body": (
                "MEDICLINIC AIRPORT ROAD HOSPITAL - BREAST SCREENING SUMMARY\n"
                "Views: Bilateral CC and MLO digital mammography with DBT.\n"
                "Density: ACR B.\n"
                "R Breast: Unremarkable involuting parenchyma. BI-RADS 1.\n"
                "L Breast: Unremarkable involuting parenchyma. BI-RADS 1.\n"
                "Overall Summary Assessment: Category 1 (Negative).\n"
                "Registry Performance Code: 3341F."
            ),
            "extra_tags": [],
        },
        {
            "id": "hb05",
            "b_right": "1",
            "b_left": "1",
            "overall": "1",
            "coded": "3341F",
            "layout": "synoptic_seha",
            "tags": ["coded_synoptic", "layout:synoptic_seha", "spelling:UK", "ocr_noise"],
            "body": (
                "SEHA MOBILE MAMMOGRAPHY VAN #03 (AL DHAFRA REGION)\n"
                "Density: ACR B (Scattered fibroglandular tissue).\n"
                "Right (RT): Negative mammogram. No suspicious mass or microcalcifications. Score: 1.\n"
                "Left (LT): Negative mammogram. Normal Cooper ligaments and skin line. Score: 1.\n"
                "Overall BI-RADS Assessment: 1 | Quality Code: 3341F."
            ),
            "extra_tags": ["negation"],
        },
        {
            "id": "hb06",
            "b_right": "2",
            "b_left": "1",
            "overall": "2",
            "coded": "3342F",
            "layout": "synoptic_ccad",
            "tags": ["coded_synoptic", "layout:synoptic_ccad", "spelling:US", "prior_diff_cat"],
            "body": (
                "CCAD BREAST IMAGING - STRUCTURED SCREENING REPORT\n"
                "Comparison: Prior screen 2023 was scored BI-RADS 0 (recalled for right retroareolar asymmetry, proven benign intramammary lymph node on spot views).\n"
                "RT Breast: Stable 6 mm circumscribed reniform mass with fatty hilum in UOQ, typical intramammary lymph node. Coarse popcorn-like calcifying fibroadenoma centrally. Category: 2.\n"
                "LT Breast: No focal abnormality. Category: 1.\n"
                "Overall Assessment: BI-RADS 2 (Benign) | CPT-II: 3342F."
            ),
            "extra_tags": [],
        },
        {
            "id": "hb07",
            "b_right": "1",
            "b_left": "2",
            "overall": "2",
            "coded": "3342F",
            "layout": "synoptic_doh_bilingual",
            "tags": ["coded_synoptic", "layout:synoptic_doh_bilingual", "arabic", "spelling:UK"],
            "body": (
                "تقرير فحص الثدي الشعاعي - مدينة الشيخ شخبوط الطبية (SSMC)\n"
                "Parenchymal Pattern: ACR B.\n"
                "Right Breast (الثدي الأيمن): Normal appearances, no suspicious features. BI-RADS 1.\n"
                "Left Breast (الثدي الأيسر): Benign vascular calcifications and secretory rod-like calcifications bilaterally but prominent on the left. BI-RADS 2.\n"
                "Overall Assessment (التقييم النهائي): BI-RADS 2 (Benign / تغيرات حميدة).\n"
                "CPT-II Code: 3342F."
            ),
            "extra_tags": ["negation"],
        },
        {
            "id": "hb08",
            "b_right": "2",
            "b_left": "2",
            "overall": "2",
            "coded": "3342F",
            "layout": "synoptic_mediclinic",
            "tags": ["coded_synoptic", "layout:synoptic_mediclinic", "spelling:UK", "hard:postop_scar_mimic"],
            "body": (
                "MEDICLINIC AL NOOR HOSPITAL - STRUCTURED MAMMOGRAPHY\n"
                "History: Bilateral reduction mammoplasty in 2020.\n"
                "R Breast: Architectural distortion and fat-fluid oil cysts in the inferior pole consistent with surgical reduction scars; stable since 2022. No suspicious mass. BI-RADS 2.\n"
                "L Breast: Symmetric post-reduction parenchymal distortion and coarse dystrophic suture calcifications. BI-RADS 2.\n"
                "Overall Final Assessment: BI-RADS 2 (Benign) | CPT-II: 3342F."
            ),
            "extra_tags": ["negation"],
        },
        {
            "id": "hb09",
            "b_right": "2",
            "b_left": "1",
            "overall": "2",
            "coded": "3342F",
            "layout": "synoptic_seha",
            "tags": ["coded_synoptic", "layout:synoptic_seha", "spelling:US"],
            "body": (
                "TAWAM HOSPITAL BREAST CARE CENTER - AL AIN (SEHA)\n"
                "Breast Composition: ACR A.\n"
                "Right: Single 11 mm radiolucent encapsulated mass with thin rim calcification (oil cyst / lipoma) at 9 o'clock. BI-RADS 2.\n"
                "Left: No mass, architectural distortion, or microcalcifications. BI-RADS 1.\n"
                "Overall Category: BI-RADS 2 | CPT-II: 3342F."
            ),
            "extra_tags": ["negation"],
        },
        {
            "id": "hb10",
            "b_right": "0",
            "b_left": "1",
            "overall": "0",
            "coded": "3340F",
            "layout": "synoptic_seha",
            "tags": ["coded_synoptic", "layout:synoptic_seha", "spelling:UK", "prior_diff_cat"],
            "body": (
                "CORNICHE / SSMC SCREENING PROGRAMME - SYNOPTIC REPORT\n"
                "Comparison: Prior screening mammogram 2024 was BI-RADS 1 bilaterally.\n"
                "Right Breast: New 9 mm focal asymmetry in the upper outer quadrant seen on both CC and MLO projections; requires spot compression and targeted ultrasound. BI-RADS 0.\n"
                "Left Breast: Unremarkable parenchyma, no suspicious abnormality. BI-RADS 1.\n"
                "Overall Final Assessment: BI-RADS 0 (Incomplete - Recall to Assessment Unit).\n"
                "CPT-II Code: 3340F."
            ),
            "extra_tags": [],
        },
        {
            "id": "hb11",
            "b_right": "2",
            "b_left": "0",
            "overall": "0",
            "coded": "3340F",
            "layout": "synoptic_ccad",
            "tags": ["coded_synoptic", "layout:synoptic_ccad", "spelling:US"],
            "body": (
                "CCAD SCREENING MAMMOGRAPHY - CODED SUMMARY\n"
                "Breast Density: ACR C.\n"
                "RT Breast: Stable calcified fibroadenoma in lower inner quadrant. Category: 2.\n"
                "LT Breast: Indeterminate group of amorphous calcifications in the retroareolar region; magnification views required. Category: 0.\n"
                "Overall Assessment: BI-RADS 0 (Incomplete) | CPT-II: 3340F."
            ),
            "extra_tags": [],
        },
        {
            "id": "hb12",
            "b_right": "0",
            "b_left": "3",
            "overall": "0",
            "coded": "3340F",
            "layout": "synoptic_mediclinic",
            "tags": ["coded_synoptic", "layout:synoptic_mediclinic", "spelling:UK", "hard:zero_outranks_three"],
            "body": (
                "MEDICLINIC AIRPORT ROAD - DUAL-READ SYNOPTIC SCREEN\n"
                "R Breast: Developing density in the upper inner quadrant not clearly characterised on standard projections; needs further imaging evaluation and prior films from Dubai. BI-RADS 0.\n"
                "L Breast: Circumscribed 7 mm non-calcified oval mass in the lower outer quadrant, unchanged from 6-month follow-up film brought by patient, remains probably benign. BI-RADS 3.\n"
                "Overall Assessment: BI-RADS 0 (Incomplete - Category 0 takes precedence over contralateral Category 3 per ACR hierarchy).\n"
                "CPT-II Code: 3340F."
            ),
            "extra_tags": [],
        },
        {
            "id": "hb13",
            "b_right": "0",
            "b_left": "0",
            "overall": "0",
            "coded": "3340F",
            "layout": "synoptic_doh_bilingual",
            "tags": ["coded_synoptic", "layout:synoptic_doh_bilingual", "arabic", "spelling:US"],
            "body": (
                "مركز الفحص الشامل - أبوظبي (SEHA AHS)\n"
                "Breast Density: ACR D (extremely dense / كثافة عالية جداً).\n"
                "Right Breast (الثدي الأيمن): Partially obscured oval mass in central breast; ultrasound needed. BI-RADS 0.\n"
                "Left Breast (الثدي الأيسر): Questionable architectural distortion in UOQ on MLO view only; spot compression needed. BI-RADS 0.\n"
                "Overall Assessment: BI-RADS 0 (Incomplete - غـير مكتمل) | CPT-II: 3340F."
            ),
            "extra_tags": [],
        },
        {
            "id": "hb14",
            "b_right": "3",
            "b_left": "1",
            "overall": "3",
            "coded": "3343F",
            "layout": "synoptic_ccad",
            "tags": ["coded_synoptic", "layout:synoptic_ccad", "spelling:US"],
            "body": (
                "CCAD BREAST IMAGING - SYNOPTIC REPORT (DIAGNOSTIC-ASSISTED SCREEN)\n"
                "Density: ACR B.\n"
                "RT Breast: Solitary circumscribed 8 mm oval mass at 12 o'clock demonstrated on screening DBT with immediate supplementary ultrasound showing a complicated cyst / fibroadenoma (<2% malignancy risk). BI-RADS 3.\n"
                "LT Breast: Negative. No mass or calcifications. BI-RADS 1.\n"
                "Overall Assessment: BI-RADS 3 (Probably Benign - 6-month short-interval follow-up) | CPT-II: 3343F."
            ),
            "extra_tags": ["negation"],
        },
        {
            "id": "hb15",
            "b_right": "4",
            "b_left": "1",
            "overall": "4",
            "coded": "3344F",
            "layout": "synoptic_seha",
            "tags": ["coded_synoptic", "layout:synoptic_seha", "spelling:UK", "subcat_4"],
            "body": (
                "SSMC BREAST SCREENING UNIT - CODED REPORT\n"
                "Composition: ACR B.\n"
                "Right Breast: 12 mm irregular mass with microlobulated margins at 10 o'clock posterior depth. Graded BI-RADS 4B (moderate suspicion for malignancy).\n"
                "Left Breast: Normal breast parenchyma. No suspicious features. Graded BI-RADS 1.\n"
                "Overall Final Assessment: BI-RADS 4 (Suspicious - Urgent referral to One-Stop Assessment Clinic).\n"
                "CPT-II Code: 3344F."
            ),
            "extra_tags": ["negation"],
        },
        {
            "id": "hb16",
            "b_right": "2",
            "b_left": "4",
            "overall": "4",
            "coded": "3344F",
            "layout": "synoptic_mediclinic",
            "tags": ["coded_synoptic", "layout:synoptic_mediclinic", "spelling:UK", "subcat_4", "prior_diff_cat"],
            "body": (
                "MEDICLINIC AL JOWHARA HOSPITAL (AL AIN) - SYNOPTIC SCREEN\n"
                "Comparison: Prior mammogram 2023 was BI-RADS 2 bilaterally.\n"
                "R Breast: Unchanged benign coarse calcification. BI-RADS 2.\n"
                "L Breast: New 7 mm cluster of fine pleomorphic microcalcifications in the upper outer quadrant. Subcategorised as BI-RADS 4A/4B -> recorded as BI-RADS 4.\n"
                "Overall Assessment: BI-RADS 4 (Suspicious Abnormality) | CPT-II: 3344F."
            ),
            "extra_tags": [],
        },
        {
            "id": "hb17",
            "b_right": "4",
            "b_left": "3",
            "overall": "4",
            "coded": "3344F",
            "layout": "synoptic_ccad",
            "tags": ["coded_synoptic", "layout:synoptic_ccad", "spelling:US", "subcat_4"],
            "body": (
                "CLEVELAND CLINIC ABU DHABI - SYNOPTIC BREAST SCREEN\n"
                "RT Breast: Architectural distortion with associated fine linear branching calcifications spanning 14 mm at 2 o'clock. BI-RADS 4C (high suspicion).\n"
                "LT Breast: Stable circumscribed oval mass at 6 o'clock under short-interval surveillance. BI-RADS 3.\n"
                "Overall Assessment: BI-RADS 4 | CPT-II: 3344F."
            ),
            "extra_tags": [],
        },
        {
            "id": "hb18",
            "b_right": "TR",
            "b_left": "4",
            "overall": "4",
            "coded": "3344F",
            "layout": "synoptic_seha",
            "tags": ["coded_synoptic", "layout:synoptic_seha", "spelling:UK", "hard:tr_with_actionable_contra"],
            "body": (
                "TAWAM MOBILE SCREENING UNIT - SYNOPTIC REPORT\n"
                "Right Breast: Technically inadequate examination (TR) - severe motion blur and frozen shoulder prevented inclusion of posterior retromammary space and pectoral muscle; no BI-RADS category assigned to right side.\n"
                "Left Breast: Clearly visualised 16 mm irregular mass with indistinct margins and tether in the upper outer quadrant. BI-RADS 4.\n"
                "Overall Final Assessment: BI-RADS 4 (Actionable contralateral category supersedes unilateral technical repeat).\n"
                "CPT-II Code: 3344F."
            ),
            "extra_tags": [],
        },
        {
            "id": "hb19",
            "b_right": "5",
            "b_left": "1",
            "overall": "5",
            "coded": "3345F",
            "layout": "synoptic_doh_bilingual",
            "tags": ["coded_synoptic", "layout:synoptic_doh_bilingual", "arabic", "spelling:UK"],
            "body": (
                "مدينة الشيخ شخبوط الطبية - قسم تصوير الثدي (SSMC)\n"
                "Right Breast (الثدي الأيمن): 28 mm dense spiculated mass in the upper outer quadrant with overlying skin retraction, nipple inversion, and pleomorphic intraductal calcifications. BI-RADS 5.\n"
                "Left Breast (الثدي الأيسر): Normal fibroglandular tissue, no mass or calcification. BI-RADS 1.\n"
                "Overall Final Assessment (التقييم النهائي): BI-RADS 5 (Highly Suggestive of Malignancy / اشتباه عالٍ بالورم الخبيث).\n"
                "CPT-II Code: 3345F."
            ),
            "extra_tags": ["negation"],
        },
        {
            "id": "hb20",
            "b_right": "2",
            "b_left": "5",
            "overall": "5",
            "coded": "3345F",
            "layout": "synoptic_ccad",
            "tags": ["coded_synoptic", "layout:synoptic_ccad", "spelling:US", "prior_diff_cat"],
            "body": (
                "CCAD IMAGING INSTITUTE - SYNOPTIC SCREENING MAMMOGRAM\n"
                "Comparison: Screening mammogram 3 years ago in Al Ain was BI-RADS 1.\n"
                "RT Breast: Scattered benign punctate calcifications. BI-RADS 2.\n"
                "LT Breast: 32 mm classic spiculated mass at 1 o'clock with segmental fine linear casting calcifications and enlarged pathological left axillary nodes. BI-RADS 5.\n"
                "Overall Assessment: BI-RADS 5 (Highly Suggestive of Malignancy) | CPT-II: 3345F."
            ),
            "extra_tags": [],
        },
        # --- 20 FREE-TEXT RADIOLOGIST REPORTS (hb21 .. hb40, coded=None) ---
        {
            "id": "hb21",
            "b_right": "1",
            "b_left": "1",
            "overall": "1",
            "coded": None,
            "layout": "narrative_nmc",
            "tags": ["free_text", "layout:narrative_nmc", "spelling:UK", "negation"],
            "body": (
                "NMC SPECIALTY HOSPITAL ABU DHABI - DEPARTMENT OF RADIOLOGY\n"
                "EXAMINATION: Bilateral Screening Digital Mammography.\n"
                "CLINICAL INDICATION: Routine biennial screening in a 52-year-old asymptomatic woman.\n"
                "FINDINGS:\n"
                "Both breasts show heterogeneously dense fibroglandular parenchyma (ACR composition C), which may obscure small masses. "
                "In the right breast, there is no evidence of a dominant mass, architectural distortion, or suspicious cluster of microcalcifications. "
                "In the left breast, the parenchymal pattern is symmetrical with no suspicious mass, no malignant calcification, and no skin or nipple retraction. "
                "Both axillae demonstrate normal fatty-hilum lymph nodes.\n"
                "IMPRESSION: Normal bilateral screening mammogram. BI-RADS 1 for both right and left breasts (Overall BI-RADS 1)."
            ),
            "extra_tags": [],
        },
        {
            "id": "hb22",
            "b_right": "1",
            "b_left": "1",
            "overall": None,
            "coded": None,
            "layout": "letter_burjeel",
            "tags": ["free_text", "layout:letter_burjeel", "spelling:UK", "hard:no_numeric_birads"],
            "body": (
                "BURJEEL MEDICAL CITY (MOHAMMED BIN ZAYED CITY, ABU DHABI)\n"
                "Consultant Breast Radiologist Narrative Report\n\n"
                "Dear Referring Colleague,\n"
                "Thank you for referring this 55-year-old lady for her routine screening mammogram. Standard craniocaudal and mediolateral oblique projections were obtained.\n"
                "The breast parenchyma is almost entirely fatty bilaterally. On careful review of the right breast, I see no mass lesion, no parenchymal deformity, and no suspicious calcifications. "
                "Similarly, the left breast is entirely unremarkable with normal skin thickness, normal retroareolar ducts, and clear axillary tails.\n"
                "Conclusion: Entirely negative bilateral screening examination. She has been discharged back to routine two-yearly screening."
            ),
            "extra_tags": ["negation"],
        },
        {
            "id": "hb23",
            "b_right": "1",
            "b_left": "1",
            "overall": "1",
            "coded": None,
            "layout": "narrative_seha",
            "tags": ["free_text", "layout:narrative_seha", "spelling:UK", "prior_diff_cat", "hard:resolved_prior_suspicion"],
            "body": (
                "SEHA - AL CORNICHE DIAGNOSTIC & SCREENING CENTRE\n"
                "Bilateral Screening Mammography with Tomosynthesis\n\n"
                "Comparison is made with the 2023 screening and assessment episode where the right breast was initially categorised as BI-RADS 4A for a cluster of calcifications, "
                "which underwent vacuum-assisted excision confirming benign columnar cell change with no residual calcifications.\n"
                "Today's examination shows scattered fibroglandular densities bilaterally. At the right 10 o'clock biopsy site, a tiny titanium marker clip is noted without any residual calcifications, mass, or architectural distortion; given complete resolution of the lesion and absence of any visible tissue abnormality, the right breast is graded BI-RADS 1 today. "
                "The left breast shows normal fibroglandular stroma with no suspicious mass or calcification (BI-RADS 1).\n"
                "Overall Impression: Negative bilateral screening mammogram, BI-RADS 1."
            ),
            "extra_tags": ["negation"],
        },
        {
            "id": "hb24",
            "b_right": "1",
            "b_left": "1",
            "overall": None,
            "coded": None,
            "layout": "pacs_dump",
            "tags": ["free_text", "layout:pacs_dump", "spelling:US", "ocr_noise"],
            "body": (
                "PACS_EXPORT // FACILITY: MUBADALA HEALTH DUBAI-ABUDHABI NETWORK - CAPITAL HEALTH SCREENING\n"
                "MODALITY: MG // STUDY: BILAT MAMMO SCREENING\n"
                "TEXT:\n"
                "Bilateral CC and MLO views performed. Scattered fibroglandular densities are present in both breasts.\n"
                "RT breast: BI-RADS 1. No focal mass, no architectural distortion, no malignant-appearing calcifications.\n"
                "LT breast: BI-RADS 1. Unremarkable parenchyma and skin envelope. Axillae clear.\n"
                "Recommendation: Routine biennial mammography."
            ),
            "extra_tags": ["negation"],
        },
        {
            "id": "hb25",
            "b_right": "1",
            "b_left": "1",
            "overall": "1",
            "coded": None,
            "layout": "narrative_nmc",
            "tags": ["free_text", "layout:narrative_nmc", "spelling:UK", "arabic"],
            "body": (
                "مستشفى إن إم سي رويال - مدينة خليفة (NMC ROYAL HOSPITAL KHALIFA CITY)\n"
                "Screening Mammography Narrative Report\n"
                "The breasts show scattered areas of fibroglandular density (ACR B). "
                "In the right breast (الثدي الأيمن), there is no evidence of a mass, parenchymal distortion, or suspicious microcalcifications (BI-RADS 1). "
                "In the left breast (الثدي الأيسر), appearances are likewise normal with no suspicious lesion identified (BI-RADS 1).\n"
                "Final Overall Impression: BI-RADS 1 (Negative / فحص طبيعي)."
            ),
            "extra_tags": ["negation"],
        },
        {
            "id": "hb26",
            "b_right": "2",
            "b_left": "1",
            "overall": "2",
            "coded": None,
            "layout": "narrative_seha",
            "tags": ["free_text", "layout:narrative_seha", "spelling:UK"],
            "body": (
                "MADINAT ZAYED HOSPITAL (AL DHAFRA) - RADIOLOGY DEPARTMENT\n"
                "Bilateral Screening Mammography\n\n"
                "The breast tissue is heterogeneously dense. In the right breast upper outer quadrant, there is a well-circumscribed 14 mm oval mass containing coarse popcorn calcifications, "
                "typical of an involuting hyalinised fibroadenoma (Right BI-RADS 2). No suspicious mass or malignant calcification is seen on the right. "
                "The left breast demonstrates normal fibroglandular architecture without any benign or suspicious focal lesion (Left BI-RADS 1).\n"
                "Overall Impression: Benign mammographic findings, overall BI-RADS 2. Routine recall in 2 years."
            ),
            "extra_tags": ["negation"],
        },
        {
            "id": "hb27",
            "b_right": "1",
            "b_left": "2",
            "overall": None,
            "coded": None,
            "layout": "letter_burjeel",
            "tags": ["free_text", "layout:letter_burjeel", "spelling:UK", "hard:no_numeric_birads"],
            "body": (
                "BURJEEL HOSPITAL - AL NAJDA STREET, ABU DHABI\n"
                "Breast Screening Narrative Report\n\n"
                "Bilateral two-view digital mammography was performed and compared with films from 2022 and 2024.\n"
                "The right breast is completely normal with fatty involution and no focal findings whatsoever.\n"
                "In the left breast, there are multiple bilateral-type but strictly left-sided large rod-like secretory calcifications radiating towards the nipple, as well as two simple radiolucent oil cysts measuring 5 mm and 7 mm in the lower inner quadrant. These are classic benign entities unchanged over 4 years.\n"
                "Summary: Normal right breast and benign left breast findings. Return to routine biennial screening."
            ),
            "extra_tags": [],
        },
        {
            "id": "hb28",
            "b_right": "2",
            "b_left": "2",
            "overall": "2",
            "coded": None,
            "layout": "pacs_dump",
            "tags": ["free_text", "layout:pacs_dump", "spelling:US", "ocr_noise"],
            "body": (
                "PACS_EXPORT // DANAT AL EMARAT HOSPITAL FOR WOMEN & CHILDREN - ABU DHABI\n"
                "STUDY: BILATERAL SCREENING MAMMOGRAM\n"
                "History: Bilateral subpectoral silicone breast implants placed in 2018. Eklund push-back views obtained.\n"
                "Findings: Both silicone implants appear intact with no extracapsular silicone granulomas. "
                "Right breast native tissue shows benign vascular calcifications (BI-RADS 2). "
                "Left breast native tissue shows a stable 9 mm calcified hamartoma (fibroadenolipoma) with classic 'breast-within-a-breast' appearance (BI-RADS 2). "
                "No suspicious mass or malignant microcalcifications on either side.\n"
                "Overall Assessment: BI-RADS 2 (Benign)."
            ),
            "extra_tags": ["negation"],
        },
        {
            "id": "hb29",
            "b_right": "0",
            "b_left": "1",
            "overall": "0",
            "coded": None,
            "layout": "narrative_seha",
            "tags": ["free_text", "layout:narrative_seha", "spelling:UK"],
            "body": (
                "SHEIKH KHALIFA MEDICAL CITY (SKMC) - ABU DHABI\n"
                "Screening Mammogram Report\n\n"
                "Bilateral digital mammography shows scattered fibroglandular tissue.\n"
                "Right breast (RT): There is a 10 mm focal asymmetry in the retroareolar region seen best on the CC projection, without clearly benign features. Additional spot compression views and targeted ultrasound are required to exclude an underlying mass (Right BI-RADS 0).\n"
                "Left breast (LT): Normal breast parenchyma with no focal mass, distortion, or suspicious calcifications (Left BI-RADS 1).\n"
                "Overall Assessment: BI-RADS 0 (Incomplete - Recall to SKMC Breast Assessment Unit)."
            ),
            "extra_tags": ["negation"],
        },
        {
            "id": "hb30",
            "b_right": "1",
            "b_left": "0",
            "overall": None,
            "coded": None,
            "layout": "letter_burjeel",
            "tags": ["free_text", "layout:letter_burjeel", "spelling:UK", "hard:no_numeric_birads"],
            "body": (
                "BURJEEL DAY SURGERY CENTER - AL REEM ISLAND, ABU DHABI\n"
                "Screening Mammography Report\n\n"
                "The right breast parenchyma is unremarkable with no mass, architectural tether, or calcification.\n"
                "In the left breast upper outer quadrant, there is a small cluster of indistinct microcalcifications whose morphology cannot be reliably characterised on standard screening projections alone. "
                "Magnification views and comparison with outside prior films from Amman are needed before a definitive assessment can be rendered.\n"
                "Action: Incomplete examination; patient recalled to the diagnostic breast unit for further imaging evaluation."
            ),
            "extra_tags": ["negation"],
        },
        {
            "id": "hb31",
            "b_right": "2",
            "b_left": "0",
            "overall": "0",
            "coded": None,
            "layout": "narrative_nmc",
            "tags": ["free_text", "layout:narrative_nmc", "spelling:US", "ocr_noise"],
            "body": (
                "NMC PROVITAE CENTER - ABU DHABI\n"
                "Bilateral Screening Mammography\n"
                "Right breast: Benign scattered punctate and dermal calcifications, unchanged. Rated BI-RADS 2.\n"
                "Left breast: New 11 mm rounded density at 3 o'clock mid-depth, partially obscured by adjacent fibroglandular tissue. Needs targeted ultrasound to differentiate cyst from solid mass. Rated BI-RADS 0.\n"
                "Overall Final Impression: BI-RADS 0 (Incomplete - Needs additional imaging evaluation)."
            ),
            "extra_tags": [],
        },
        {
            "id": "hb32",
            "b_right": "0",
            "b_left": "2",
            "overall": "0",
            "coded": None,
            "layout": "pacs_dump",
            "tags": ["free_text", "layout:pacs_dump", "spelling:UK", "prior_diff_cat"],
            "body": (
                "PACS_EXPORT // SEHA AL WATHBA SCREENING CLINIC\n"
                "Comparison: 2023 screening mammogram was graded BI-RADS 1 bilaterally.\n"
                "R breast: Subtle architectural distortion in the lower outer quadrant on MLO tomosynthesis slice 24/55; requires diagnostic workup with spot views and ultrasound (R = BI-RADS 0).\n"
                "L breast: Benign intramammary lymph node in axillary tail (L = BI-RADS 2).\n"
                "Overall Assessment: BI-RADS 0."
            ),
            "extra_tags": [],
        },
        {
            "id": "hb33",
            "b_right": "1",
            "b_left": "3",
            "overall": "3",
            "coded": None,
            "layout": "narrative_nmc",
            "tags": ["free_text", "layout:narrative_nmc", "spelling:US"],
            "body": (
                "HARLEY STREET MEDICAL CENTRE - MARINA VILLAGE, ABU DHABI\n"
                "Bilateral Screening Mammogram (with immediate same-visit ultrasound check of known lesion)\n\n"
                "Right breast (R): Negative mammogram. No mass or calcifications (BI-RADS 1).\n"
                "Left breast (L): Solitary group of round and punctate calcifications in the central breast, together with a 6 mm circumscribed oval non-calcified mass at 4 o'clock showing <2% likelihood of malignancy. Graded BI-RADS 3.\n"
                "Overall Impression: BI-RADS 3 (Probably Benign - 6-month unilateral left follow-up recommended)."
            ),
            "extra_tags": ["negation"],
        },
        {
            "id": "hb34",
            "b_right": "4",
            "b_left": "2",
            "overall": "4",
            "coded": None,
            "layout": "narrative_seha",
            "tags": ["free_text", "layout:narrative_seha", "spelling:UK", "subcat_4"],
            "body": (
                "SSMC / MAFRAQ BREAST IMAGING CENTRE - ABU DHABI\n"
                "Bilateral Screening Digital Mammography\n\n"
                "Right breast: In the upper inner quadrant at 1 o'clock, there is a 9 mm irregular mass with indistinct margins and associated amorphous microcalcifications. This is suspicious for malignancy and subcategorised as BI-RADS 4A.\n"
                "Left breast: Benign coarse vascular calcifications and a simple oil cyst. BI-RADS 2.\n"
                "Overall Final Assessment: BI-RADS 4 (Suspicious abnormality - referred to One-Stop Breast Clinic for ultrasound-guided core biopsy)."
            ),
            "extra_tags": [],
        },
        {
            "id": "hb35",
            "b_right": "1",
            "b_left": "4",
            "overall": None,
            "coded": None,
            "layout": "letter_burjeel",
            "tags": ["free_text", "layout:letter_burjeel", "spelling:UK", "subcat_4"],
            "body": (
                "BURJEEL MEDICAL CITY - ONCOLOGY & BREAST IMAGING CENTRE\n"
                "Screening Mammography Narrative Report\n\n"
                "The right breast shows normal fibroglandular tissue (BI-RADS 1) with no mass or suspicious calcifications.\n"
                "In the left breast at the 2 o'clock position, 5 cm from the nipple, there is a 15 mm lobulated hyperdense mass with microlobulated and focal angulated margins, accompanied by a 10 mm cluster of fine pleomorphic calcifications (Left BI-RADS 4C).\n"
                "Action: Urgent recall to the multidisciplinary breast assessment clinic for tissue diagnosis."
            ),
            "extra_tags": ["negation"],
        },
        {
            "id": "hb36",
            "b_right": "5",
            "b_left": "2",
            "overall": "5",
            "coded": None,
            "layout": "narrative_seha",
            "tags": ["free_text", "layout:narrative_seha", "spelling:UK", "arabic"],
            "body": (
                "مستشفى توام - العين (TAWAM HOSPITAL BREAST IMAGING)\n"
                "Bilateral Screening Mammography Report\n\n"
                "Right Breast (الثدي الأيمن): There is a 24 mm high-density spiculated mass in the upper outer quadrant with marked architectural tethering, overlying skin thickening, and linear branching ductal calcifications extending towards the nipple. Enlarged cortical-thickened right axillary lymph nodes are also present. Right breast = BI-RADS 5.\n"
                "Left Breast (الثدي الأيسر): Stable calcified fibroadenoma in the lower inner quadrant. Left breast = BI-RADS 2.\n"
                "Overall Assessment: BI-RADS 5 (Highly suggestive of malignancy - تحويل عاجل لعيادة تشخيص الأورام)."
            ),
            "extra_tags": [],
        },
        {
            "id": "hb37",
            "b_right": "TR",
            "b_left": "1",
            "overall": None,
            "coded": None,
            "layout": "pacs_dump",
            "tags": ["free_text", "layout:pacs_dump", "spelling:UK", "hard:tr_overrides_benign"],
            "body": (
                "PACS_EXPORT // SEHA AL GHARBIA (LIWA) MOBILE SCREENING UNIT\n"
                "STUDY: BILATERAL SCREENING MAMMOGRAM\n"
                "Right breast (R): Technically inadequate examination (TR). The pectoral muscle is not visualised on the MLO view, the inframammary fold is cut off, and there is prominent skin-fold and motion artefact obscuring the posterior third of the gland. No BI-RADS category can be issued for the right breast.\n"
                "Left breast (L): Well-positioned views showing normal fatty-involuted breast tissue with no mass or calcification (BI-RADS 1).\n"
                "Disposition: Technical repeat (TR) required for the right breast."
            ),
            "extra_tags": ["negation"],
        },
        {
            "id": "hb38",
            "b_right": "2",
            "b_left": "TR",
            "overall": None,
            "coded": None,
            "layout": "narrative_nmc",
            "tags": ["free_text", "layout:narrative_nmc", "spelling:US", "hard:tr_overrides_benign", "ocr_noise"],
            "body": (
                "NMC ROYAL WOMEN'S HOSPITAL - ABU DHABI\n"
                "Screening Mammography Report\n"
                "Right breast (RT): Good positioning. Scattered fibroglandular tissue with a stable 8 mm benign calcified oil cyst in the central zone. Graded BI-RADS 2.\n"
                "Left breast (LT): Technical Repeat (TR) required. Severe patient motion blur during exposure and inadequate compression resulted in non-diagnostic images of the left breast; posterior tissue and retromammary space are excluded. No BI-RADS score assigned to left breast.\n"
                "Conclusion: Recall for technical repeat of the left mammogram (TR)."
            ),
            "extra_tags": [],
        },
        {
            "id": "hb39",
            "b_right": "TR",
            "b_left": "TR",
            "overall": None,
            "coded": None,
            "layout": "letter_burjeel",
            "tags": ["free_text", "layout:letter_burjeel", "spelling:UK"],
            "body": (
                "BURJEEL OASIS MEDICAL CENTRE - BANIYAS, ABU DHABI\n"
                "Screening Mammogram Technical Review\n\n"
                "Bilateral CC and MLO views were attempted on the mobile unit. Unfortunately, due to a calibration fault on the compression paddle combined with marked patient tremor, "
                "both the right breast (TR) and the left breast (TR) images are technically inadequate with severe motion unsharpness, droop of the nipple-areolar complex, and exclusion of the pectoral muscles bilaterally. "
                "No diagnostic BI-RADS category can be assigned to either breast.\n"
                "Action: Technical repeat (TR) of bilateral mammography arranged at the main hospital unit."
            ),
            "extra_tags": [],
        },
        {
            "id": "hb40",
            "b_right": "0",
            "b_left": "TR",
            "overall": "0",
            "coded": None,
            "layout": "narrative_seha",
            "tags": ["free_text", "layout:narrative_seha", "spelling:UK", "hard:tr_with_actionable_contra"],
            "body": (
                "TAWAM HOSPITAL SCREENING PROGRAMME - AL AIN\n"
                "Bilateral Screening Mammography\n\n"
                "Right breast (RT): Despite mild under-compression, a 13 mm focal asymmetry with possible architectural distortion is seen in the upper outer quadrant requiring diagnostic spot views and ultrasound (Right = BI-RADS 0).\n"
                "Left breast (LT): Technically inadequate images (TR) due to severe motion blur and failure to include the posterior pectoral margin; no category assigned to the left breast.\n"
                "Overall Assessment: BI-RADS 0 (Patient recalled to the diagnostic assessment clinic where both the right breast workup and repeat left mammographic projections will be performed)."
            ),
            "extra_tags": [],
        },
    ]

    reports = []
    for sp in specs:
        label = compute_screen_label(sp["b_right"], sp["b_left"])
        if sp["overall"] is not None:
            assert sp["overall"] == label[1:], f"Mismatch in {sp['id']}"
        if sp["coded"] is not None:
            assert label != "TR", f"TR cannot be coded in {sp['id']}"
            assert sp["coded"] == CPT_MAP[label], f"Bad CPT in {sp['id']}"

        tags = list(sp["tags"]) + list(sp["extra_tags"])
        text = sp["body"]
        if "ocr_noise" in tags:
            text = apply_ocr_noise(text, rng)

        reports.append(
            {
                "id": sp["id"],
                "text": text,
                "key": {
                    "b_right": sp["b_right"],
                    "b_left": sp["b_left"],
                    "overall": sp["overall"],
                    "label": label,
                    "coded": sp["coded"],
                },
                "tags": tags,
            }
        )
    return reports


def build_assessment_notes(rng: random.Random) -> list:
    """Generate 50 unique diagnostic breast assessment / MDT documents."""
    specs = [
        # --- 13 RETURN_ROUTINE (ha01 .. ha13) ---
        {
            "id": "ha01",
            "attended": True,
            "assessment": "done",
            "biopsy": "none",
            "outcome": "routine",
            "layout": "onestop_clinic",
            "tags": ["layout:onestop_clinic", "spelling:UK"],
            "body": (
                "SSMC ONE-STOP BREAST ASSESSMENT CLINIC - ABU DHABI\n"
                "Attendance: Attended recall clinic on schedule.\n"
                "Reason for Recall: Right UOQ focal asymmetry on screening mammogram (BI-RADS 0).\n"
                "Assessment Performed: Clinical breast examination (normal, P1), right spot compression craniocaudal and MLO views, and targeted high-frequency right breast ultrasound.\n"
                "Findings: The focal asymmetry disperses completely on spot compression, representing summation of normal fibroglandular tissue. Ultrasound of the right UOQ shows normal breast parenchyma with no solid or cystic mass. No biopsy required.\n"
                "Outcome / Decision: Discharged back to routine biennial screening (Return to Routine Screening)."
            ),
        },
        {
            "id": "ha02",
            "attended": True,
            "assessment": "done",
            "biopsy": "none",
            "outcome": "routine",
            "layout": "us_diagnostic_report",
            "tags": ["layout:us_diagnostic_report", "spelling:US"],
            "body": (
                "CLEVELAND CLINIC ABU DHABI - DIAGNOSTIC BREAST ASSESSMENT REPORT\n"
                "Patient Status: Attended appointment.\n"
                "Indication: Recall from screening for a 10 mm circumscribed mass in the left breast.\n"
                "Diagnostic Workup: Targeted left breast ultrasound performed.\n"
                "Findings: At the 3 o'clock position, 4 cm from the nipple, corresponding to the mammographic density, there is a classic anechoic simple cyst measuring 10 x 7 mm with imperceptible walls and posterior acoustic enhancement (BI-RADS 2). No needle aspiration or core biopsy indicated.\n"
                "Disposition: Benign finding. Return to routine 2-year screening mammography."
            ),
        },
        {
            "id": "ha03",
            "attended": True,
            "assessment": "done",
            "biopsy": "none",
            "outcome": "routine",
            "layout": "Arabic_bilingual_summary",
            "tags": ["layout:Arabic_bilingual_summary", "arabic", "spelling:UK"],
            "body": (
                "عيادة تقييم الثدي التخصصية - مستشفى توام (TAWAM BREAST ASSESSMENT UNIT)\n"
                "حالة الحضور: حضرت المريضة (Attended).\n"
                "Reason for Recall: Left retroareolar questionable microcalcifications.\n"
                "Assessment: True lateral and magnification views of the left breast (الثدي الأيسر) plus targeted ultrasound.\n"
                "Findings: Magnification views demonstrate classic tea-cup (milk of calcium) layering within microcysts on the 90-degree lateral projection. Ultrasound confirms tiny clustered microcysts. Benign BI-RADS 2; no biopsy performed.\n"
                "Decision (القرار الطبي): Return to routine screening (العودة للفحص الدوري الروتيني كل سنتين)."
            ),
        },
        {
            "id": "ha04",
            "attended": True,
            "assessment": "done",
            "biopsy": "none",
            "outcome": "routine",
            "layout": "onestop_clinic",
            "tags": ["layout:onestop_clinic", "spelling:UK", "prior_year_biopsy", "hard:historical_biopsy_distractor"],
            "body": (
                "SKMC DIAGNOSTIC BREAST CLINIC - ABU DHABI\n"
                "Attendance: Attended.\n"
                "Clinical & Past History: Note that in 2021 she underwent an ultrasound-guided core biopsy of the right breast which showed benign sclerosing adenosis (B2), and her mother was treated for breast cancer in 2017.\n"
                "Today's Recall Assessment: Recalled for a new density in the left upper inner quadrant. Spot compression tomosynthesis and targeted left breast ultrasound were performed today.\n"
                "Findings: The left UIQ density corresponds to a typical 6 mm reniform intramammary lymph node with a preserved echogenic hilum. No biopsy was performed during this episode.\n"
                "Outcome: Reassured and discharged to routine biennial screening."
            ),
        },
        {
            "id": "ha05",
            "attended": True,
            "assessment": "done",
            "biopsy": "none",
            "outcome": "routine",
            "layout": "clinic_letter",
            "tags": ["layout:clinic_letter", "spelling:UK"],
            "body": (
                "MEDICLINIC AIRPORT ROAD HOSPITAL - BREAST ASSESSMENT LETTER\n"
                "Dear Dr Al Mansoori,\n"
                "Your patient attended our recall assessment clinic today following an incomplete screening mammogram (right breast technical/asymmetry query). "
                "We performed repeat coned compression mammography and targeted right breast ultrasound, alongside clinical palpation which was normal. "
                "All imaging confirmed normal overlapping fibroglandular stroma with no discrete mass lesion and no microcalcifications. No biopsy was needed.\n"
                "She has been returned to the routine 24-month screening programme."
            ),
        },
        {
            "id": "ha06",
            "attended": True,
            "assessment": "done",
            "biopsy": "none",
            "outcome": "routine",
            "layout": "us_diagnostic_report",
            "tags": ["layout:us_diagnostic_report", "spelling:US", "ocr_noise"],
            "body": (
                "NMC SPECIALTY HOSPITAL ABU DHABI - DIAGNOSTIC BREAST WORKUP\n"
                "Attendance: Patient attended scheduled diagnostic slot.\n"
                "Procedures Performed: Bilateral magnification views and targeted ultrasound of the right lower inner quadrant.\n"
                "Findings: The calcifications seen on screening are confirmed on tangential views to lie entirely within dermal sebaceous glands (benign skin calcifications, BI-RADS 2). Ultrasound is negative for any mass. Biopsy: None.\n"
                "Management Outcome: Return to routine screening."
            ),
        },
        {
            "id": "ha07",
            "attended": True,
            "assessment": "done",
            "biopsy": "benign",
            "outcome": "routine",
            "layout": "mdt_minutes",
            "tags": ["layout:mdt_minutes", "spelling:UK"],
            "body": (
                "SSMC MULTIDISCIPLINARY TEAM (MDT) BREAST MEETING MINUTES\n"
                "Patient Attendance at Assessment: Attended Triple Assessment on 12/10/2026.\n"
                "Assessment Summary: Recalled for 14 mm lobulated right breast mass at 9 o'clock. Ultrasound-guided 14G core needle biopsy performed.\n"
                "Histopathology Result (This Episode): Three cores of breast tissue showing a benign fibroadenoma with pericanalicular growth pattern; no atypia, no in-situ or invasive malignancy (B2 benign, concordant with imaging).\n"
                "MDT Outcome: Concordant benign biopsy. Discharge patient back to routine biennial screening."
            ),
        },
        {
            "id": "ha08",
            "attended": True,
            "assessment": "done",
            "biopsy": "benign",
            "outcome": "routine",
            "layout": "mdt_minutes",
            "tags": ["layout:mdt_minutes", "spelling:UK", "hard:mimic_benign_pathology"],
            "body": (
                "TAWAM HOSPITAL BREAST MDT RECORD - AL AIN\n"
                "Episode Attendance: Attended diagnostic assessment and stereotactic biopsy.\n"
                "Imaging: Recalled for a 9 mm cluster of amorphous microcalcifications in the left upper outer quadrant (scored M4a).\n"
                "Pathology (Left 9G Vacuum-Assisted Core Biopsy): Sections show florid sclerosing adenosis and apocrine metaplasia with abundant luminal microcalcifications; immunohistochemistry confirms intact p63/calponin myoepithelial cell layer throughout. Whilst sclerosing adenosis can radiologically and histologically mimic invasive carcinoma, there is no atypia, no DCIS, and no malignancy (B2 Benign).\n"
                "MDT Decision: Radiology-pathology concordance confirmed. Return to routine screening."
            ),
        },
        {
            "id": "ha09",
            "attended": True,
            "assessment": "done",
            "biopsy": "benign",
            "outcome": "routine",
            "layout": "onestop_clinic",
            "tags": ["layout:onestop_clinic", "spelling:US"],
            "body": (
                "CCAD BREAST ASSESSMENT & PATHOLOGY ADDENDUM\n"
                "Attendance: Attended diagnostic clinic.\n"
                "Workup Done: Diagnostic mammography, ultrasound, and ultrasound-guided core biopsy of a 12 mm irregular hypoechoic area in the right lower outer quadrant (history of seat-belt trauma 4 months ago).\n"
                "Biopsy Result: Benign fat necrosis with lipid-laden macrophages, foreign-body giant cells, and haemosiderin; negative for atypia or malignancy.\n"
                "Final Outcome: Benign concordant result. Return to routine screening."
            ),
        },
        {
            "id": "ha10",
            "attended": True,
            "assessment": "done",
            "biopsy": "benign",
            "outcome": "routine",
            "layout": "mdt_minutes",
            "tags": ["layout:mdt_minutes", "spelling:UK", "prior_year_biopsy"],
            "body": (
                "BURJEEL MEDICAL CITY - BREAST MDT SUMMARY\n"
                "Background: Previous contralateral right breast biopsy in 2020 was a benign papilloma.\n"
                "Current Recall Episode: Attended assessment for a new 8 mm cluster of microcalcifications in the left central breast. Stereotactic core biopsy performed during this episode yielded representative calcifications on specimen radiograph.\n"
                "Histopathology (Current Episode): Benign fibrocystic change with microcysts, stromal fibrosis, and calcium oxalate crystals (B2 Benign).\n"
                "MDT Outcome: Return to routine two-yearly screening."
            ),
        },
        {
            "id": "ha11",
            "attended": True,
            "assessment": "done",
            "biopsy": "benign",
            "outcome": "routine",
            "layout": "clinic_letter",
            "tags": ["layout:clinic_letter", "spelling:UK", "arabic"],
            "body": (
                "مدينة الشيخ خليفة الطبية (SKMC) - عيادة جراحة وأمراض الثدي\n"
                "Attendance: Attended assessment and results clinic.\n"
                "Dear Doctor, This 51-year-old lady attended after recall for a 15 mm circumscribed mass in the right breast (الثدي الأيمن). "
                "Triple assessment including ultrasound-guided core needle biopsy was completed. Histology confirmed a benign tubular adenoma / hamartoma (نتيجة الخزعة حميدة تماماً - B2). "
                "The MDT has discharged her back to routine screening (العودة للفحص الروتيني)."
            ),
        },
        {
            "id": "ha12",
            "attended": True,
            "assessment": "done",
            "biopsy": "benign",
            "outcome": "routine",
            "layout": "mdt_minutes",
            "tags": ["layout:mdt_minutes", "spelling:UK", "hard:mimic_benign_pathology"],
            "body": (
                "SSMC BREAST MDT OUTCOME SHEET\n"
                "Attendance: Attended assessment unit.\n"
                "Workup: Recalled for architectural distortion in the right upper outer quadrant. Tomosynthesis-guided vacuum-assisted excision biopsy (VAE, 7G) performed.\n"
                "Pathology (This Episode): Completely excised 6 mm radial scar (benign complex sclerosing lesion) with fibroelastotic core; although radial scars mimic spiculated carcinoma on mammography, comprehensive sampling shows no epithelial atypia, no LCIS/DCIS, and no invasive malignancy (B2/B3 fully excised benign lesion).\n"
                "MDT Decision: No surgery required; return to routine screening."
            ),
        },
        {
            "id": "ha13",
            "attended": True,
            "assessment": "done",
            "biopsy": "none",
            "outcome": "routine",
            "layout": "us_diagnostic_report",
            "tags": ["layout:us_diagnostic_report", "spelling:US"],
            "body": (
                "DANAT AL EMARAT HOSPITAL - DIAGNOSTIC BREAST IMAGING\n"
                "Attendance: Attended.\n"
                "Assessment: Recalled for bilateral asymmetric densities. Extended CC views, rolled views, and bilateral whole-breast ultrasound performed.\n"
                "Findings: Bilateral ectopic accessory axillary breast tissue with normal sonographic echotexture; no focal solid mass or suspicious feature. Biopsy: Not indicated (none).\n"
                "Outcome: Return to routine biennial screening."
            ),
        },
        # --- 5 EARLY_RECALL (ha14 .. ha18) ---
        {
            "id": "ha14",
            "attended": True,
            "assessment": "done",
            "biopsy": "none",
            "outcome": "early_recall",
            "layout": "onestop_clinic",
            "tags": ["layout:onestop_clinic", "spelling:UK"],
            "body": (
                "TAWAM HOSPITAL BREAST ASSESSMENT CLINIC - AL AIN\n"
                "Attendance: Attended.\n"
                "Assessment Performed: Spot compression views and targeted ultrasound of the left breast for a 7 mm oval mass recalled from screening.\n"
                "Findings: Ultrasound shows a 7 x 4 mm well-circumscribed oval hypoechoic mass oriented parallel to the skin at 4 o'clock, consistent with a probable fibroadenoma or complicated cyst with homogeneous low-level internal echoes (BI-RADS 3). After discussion, no biopsy was performed today.\n"
                "Outcome: Short-interval early recall in 6 months for repeat left breast ultrasound and mammogram."
            ),
        },
        {
            "id": "ha15",
            "attended": True,
            "assessment": "done",
            "biopsy": "none",
            "outcome": "early_recall",
            "layout": "us_diagnostic_report",
            "tags": ["layout:us_diagnostic_report", "spelling:US"],
            "body": (
                "CCAD DIAGNOSTIC BREAST CENTER\n"
                "Attendance: Attended diagnostic evaluation.\n"
                "Workup: Magnification views and ultrasound of the right breast for a solitary group of punctate calcifications.\n"
                "Findings: Tight cluster of round/punctate calcifications without associated mass or distortion (BI-RADS 3, <2% risk of malignancy). Patient is on dual antiplatelet therapy following recent coronary stenting; team elected imaging surveillance rather than biopsy.\n"
                "Outcome: Early recall at 6 months for unilateral right magnification mammography."
            ),
        },
        {
            "id": "ha16",
            "attended": True,
            "assessment": "done",
            "biopsy": "benign",
            "outcome": "early_recall",
            "layout": "mdt_minutes",
            "tags": ["layout:mdt_minutes", "spelling:UK"],
            "body": (
                "SSMC BREAST MDT MEETING MINUTES\n"
                "Attendance: Attended assessment and core biopsy.\n"
                "Assessment: Recalled for a retroareolar intraductal mass on the right. Ultrasound-guided 14G core biopsy performed.\n"
                "Biopsy Result (This Episode): Benign intraductal papilloma without epithelial atypia (B3 lesion of uncertain potential but benign histology, no malignancy).\n"
                "MDT Outcome: Patient declined vacuum excision; MDT agreed on early recall (short-interval 6-month clinical and ultrasound follow-up)."
            ),
        },
        {
            "id": "ha17",
            "attended": True,
            "assessment": "done",
            "biopsy": "benign",
            "outcome": "early_recall",
            "layout": "mdt_minutes",
            "tags": ["layout:mdt_minutes", "spelling:UK", "ocr_noise"],
            "body": (
                "SKMC BREAST MDT RECORD - ABU DHABI\n"
                "Attendance: Attended diagnostic clinic.\n"
                "Workup: Ultrasound-guided core needle biopsy of an 11 mm complex solid-cystic lesion in the left upper outer quadrant.\n"
                "Histopathology: Benign fibrocystic changes with papillary apocrine hyperplasia, no atypia and no malignancy (benign biopsy). However, because the lesion was only partially sampled due to wall collapse, the MDT recommends short-interval imaging.\n"
                "MDT Outcome: Early recall in 6 months for repeat targeted ultrasound."
            ),
        },
        {
            "id": "ha18",
            "attended": True,
            "assessment": "done",
            "biopsy": "benign",
            "outcome": "early_recall",
            "layout": "clinic_letter",
            "tags": ["layout:clinic_letter", "spelling:US"],
            "body": (
                "MEDICLINIC AL NOOR HOSPITAL - BREAST CLINIC FOLLOW-UP PLAN\n"
                "Attendance: Attended.\n"
                "Summary: This 46-year-old woman underwent diagnostic mammography, ultrasound, and core biopsy of a 9 mm right breast mass at 12 o'clock. "
                "Pathology returned benign pseudoangiomatous stromal hyperplasia (PASH) with no malignancy. Given the cellular stroma, the breast MDT advised a 6-month short-interval early recall ultrasound to confirm dimensional stability."
            ),
        },
        # --- 10 TO_TREATMENT (ha19 .. ha28) ---
        {
            "id": "ha19",
            "attended": True,
            "assessment": "done",
            "biopsy": "malignant",
            "outcome": "treatment",
            "layout": "mdt_minutes",
            "tags": ["layout:mdt_minutes", "spelling:UK"],
            "body": (
                "SSMC MULTIDISCIPLINARY BREAST ONCOLOGY BOARD - ABU DHABI\n"
                "Attendance: Attended One-Stop Triple Assessment Clinic.\n"
                "Assessment Performed: Bilateral diagnostic mammography, bilateral breast and axillary ultrasound, and ultrasound-guided 14G core biopsies of the right UOQ 18 mm spiculated mass and a cortical-thickened right axillary node.\n"
                "Biopsy Result (This Episode): Right breast core biopsy = Invasive ductal carcinoma (NST), Grade 2, ER positive (Allred 8/8), PR positive (7/8), HER2 negative (IHC 1+), Ki-67 18% (B5b Malignant). Right axillary node = metastatic carcinoma.\n"
                "MDT Outcome: Referred to breast surgical oncology for right wide local excision and axillary clearance (To Treatment)."
            ),
        },
        {
            "id": "ha20",
            "attended": True,
            "assessment": "done",
            "biopsy": "malignant",
            "outcome": "treatment",
            "layout": "mdt_minutes",
            "tags": ["layout:mdt_minutes", "spelling:US", "arabic"],
            "body": (
                "قرار اللجنة الطبية متعددة التخصصات لأورام الثدي - كليفلاند كلينك أبوظبي (CCAD MDT)\n"
                "Attendance: Attended (حضرت المريضة).\n"
                "Diagnostic Workup: Stereotactic 9G vacuum-assisted core biopsy of a 22 mm segmental cluster of fine linear branching calcifications in the left breast (الثدي الأيسر).\n"
                "Pathology Result: High-grade Ductal Carcinoma In Situ (DCIS, B5a Malignant) with comedonecrosis, ER positive.\n"
                "MDT Outcome (الخطة العلاجية): Referred to breast surgery for wire-localized left lumpectomy and radiotherapy planning (Treatment)."
            ),
        },
        {
            "id": "ha21",
            "attended": True,
            "assessment": "done",
            "biopsy": "malignant",
            "outcome": "treatment",
            "layout": "mdt_minutes",
            "tags": ["layout:mdt_minutes", "spelling:UK", "hard:dcis_is_malignant"],
            "body": (
                "TAWAM HOSPITAL BREAST MDT - AL AIN\n"
                "Attendance: Attended assessment clinic.\n"
                "Imaging & Biopsy: Recalled for 14 mm pleomorphic calcifications in the right central breast; ultrasound showed no mass. Stereotactic core biopsy performed.\n"
                "Histopathology (This Episode): There is no invasive carcinoma identified in any of the 12 cores; however, cores contain intermediate-to-high nuclear grade Ductal Carcinoma In Situ (DCIS, solid and cribriform patterns, B5a malignant non-invasive breast cancer).\n"
                "MDT Outcome: Referred to surgical oncology for wide local excision of right DCIS (Treatment)."
            ),
        },
        {
            "id": "ha22",
            "attended": True,
            "assessment": "done",
            "biopsy": "malignant",
            "outcome": "treatment",
            "layout": "mdt_minutes",
            "tags": ["layout:mdt_minutes", "spelling:UK", "hard:discordant_repeat_biopsy"],
            "body": (
                "SKMC BREAST MDT MINUTES - ABU DHABI\n"
                "Attendance: Attended both initial and repeat assessment visits.\n"
                "Assessment & Biopsy Course: Recalled for a 16 mm spiculated mass at left 2 o'clock (BI-RADS 5). Initial freehand clinic core pass last week sampled only perilesional adipose tissue (non-representative B1), which was deemed discordant at radiology review. Repeat ultrasound-guided 14G core biopsy performed 4 days ago directly through the central nidus.\n"
                "Final Pathology (This Episode): Invasive lobular carcinoma, classical type, Grade 2, E-cadherin negative, ER 8/8, PR 6/8, HER2 negative (B5b Malignant).\n"
                "MDT Outcome: Referred for breast MRI and surgical oncology management (Treatment)."
            ),
        },
        {
            "id": "ha23",
            "attended": True,
            "assessment": "done",
            "biopsy": "malignant",
            "outcome": "treatment",
            "layout": "clinic_letter",
            "tags": ["layout:clinic_letter", "spelling:UK"],
            "body": (
                "BURJEEL MEDICAL CITY - ONCOLOGY & BREAST SURGERY CLINIC\n"
                "Attendance: Attended.\n"
                "Dear Colleague, Following recall from screening for a 26 mm left breast mass, this 58-year-old lady underwent full diagnostic imaging and ultrasound-guided core needle biopsy in our unit. "
                "Histopathology confirmed Grade 3 triple-negative invasive ductal carcinoma (ER 0, PR 0, HER2 0, Ki-67 75% - Malignant). "
                "Following MDT discussion today, she has been referred directly to medical oncology to commence neoadjuvant chemo-immunotherapy prior to surgery (Treatment)."
            ),
        },
        {
            "id": "ha24",
            "attended": True,
            "assessment": "done",
            "biopsy": "malignant",
            "outcome": "treatment",
            "layout": "mdt_minutes",
            "tags": ["layout:mdt_minutes", "spelling:US", "prior_year_biopsy"],
            "body": (
                "CCAD BREAST TUMOR BOARD SUMMARY\n"
                "Past History: Prior right breast biopsy in 2019 showed benign usual ductal hyperplasia.\n"
                "Current Recall Episode: Attended diagnostic workup for a new 11 mm spiculated mass in the left lower inner quadrant. Ultrasound-guided core biopsy completed.\n"
                "Pathology (Current Episode): Invasive tubular carcinoma, Grade 1, ER positive, PR positive, HER2 negative (Malignant).\n"
                "Tumor Board Outcome: Referred to breast surgery for left seed-localized partial mastectomy and sentinel lymph node biopsy (Treatment)."
            ),
        },
        {
            "id": "ha25",
            "attended": True,
            "assessment": "done",
            "biopsy": "malignant",
            "outcome": "treatment",
            "layout": "onestop_clinic",
            "tags": ["layout:onestop_clinic", "spelling:UK", "ocr_noise"],
            "body": (
                "TAWAM HOSPITAL ONE-STOP BREAST CLINIC & MDT DISPOSITION\n"
                "Attendance: Attended.\n"
                "Workup: Diagnostic mammogram, ultrasound, and core biopsy of right 12 o'clock 21 mm irregular mass.\n"
                "Biopsy Result: Invasive micropapillary carcinoma of the breast, Grade 2, HER2-amplified (IHC 3+), ER positive (B5b Malignant).\n"
                "Outcome: Referred to medical and surgical oncology for anti-HER2 neoadjuvant therapy and definitive surgery (Treatment)."
            ),
        },
        {
            "id": "ha26",
            "attended": True,
            "assessment": "done",
            "biopsy": "malignant",
            "outcome": "treatment",
            "layout": "mdt_minutes",
            "tags": ["layout:mdt_minutes", "spelling:UK"],
            "body": (
                "MEDICLINIC AIRPORT ROAD - MULTIDISCIPLINARY BREAST CANCER TEAM\n"
                "Attendance: Attended diagnostic assessment.\n"
                "Assessment: Bilateral contrast-enhanced spectral mammography, targeted ultrasound, and core needle biopsy of a 19 mm right retroareolar mass.\n"
                "Histopathology: Invasive mucinous (colloid) carcinoma, Grade 1, strongly ER/PR positive, HER2 negative (B5b Malignant).\n"
                "MDT Decision: Proceed to surgical treatment (right central wide local excision and sentinel node biopsy)."
            ),
        },
        {
            "id": "ha27",
            "attended": True,
            "assessment": "done",
            "biopsy": "malignant",
            "outcome": "treatment",
            "layout": "clinic_letter",
            "tags": ["layout:clinic_letter", "spelling:US"],
            "body": (
                "NMC ROYAL HOSPITAL KHALIFA CITY - BREAST ONCOLOGY REFERRAL\n"
                "Attendance: Attended recall workup.\n"
                "Summary: Patient recalled for architectural distortion in the right upper outer quadrant. Tomosynthesis spot views, ultrasound, and 14G core biopsy were performed. "
                "Pathology shows invasive ductal carcinoma with lobular features (Grade 2, malignant). "
                "Outcome: Referred to the breast oncology surgical team for definitive treatment."
            ),
        },
        {
            "id": "ha28",
            "attended": True,
            "assessment": "done",
            "biopsy": "malignant",
            "outcome": "treatment",
            "layout": "mdt_minutes",
            "tags": ["layout:mdt_minutes", "spelling:UK", "hard:dcis_is_malignant"],
            "body": (
                "SSMC BREAST MDT RECORD - ABU DHABI\n"
                "Attendance: Attended triple assessment.\n"
                "Workup: Recalled for microcalcifications at left 11 o'clock. Stereotactic vacuum core biopsy performed.\n"
                "Pathology: Encapsulated papillary carcinoma with surrounding cribriform Ductal Carcinoma In Situ (DCIS), no frank stromal invasion seen, classified as B5a in-situ malignancy (malignant biopsy).\n"
                "MDT Outcome: Referred to breast surgery for therapeutic wide local excision (Treatment)."
            ),
        },
        # --- 15 INCOMPLETE (ha29 .. ha43) ---
        {
            "id": "ha29",
            "attended": True,
            "assessment": "done",
            "biopsy": "pending",
            "outcome": "pending",
            "layout": "onestop_clinic",
            "tags": ["layout:onestop_clinic", "spelling:UK", "biopsy_awaited"],
            "body": (
                "SSMC ONE-STOP BREAST ASSESSMENT CLINIC - ABU DHABI\n"
                "Attendance: Attended clinic today.\n"
                "Assessment Performed: Clinical breast exam (15 mm firm mass right UOQ, E3), spot magnification mammography, and targeted right breast ultrasound (irregular antiparallel hypoechoic mass, U4). "
                "Ultrasound-guided 14-gauge core needle biopsy (4 passes) performed under local anaesthesia and marker clip deployed.\n"
                "Biopsy Result: Specimen sent to histopathology; result awaited (Pending).\n"
                "Outcome: Pending MDT discussion next Tuesday once histology and receptor profile are available."
            ),
        },
        {
            "id": "ha30",
            "attended": True,
            "assessment": "done",
            "biopsy": "pending",
            "outcome": "pending",
            "layout": "us_diagnostic_report",
            "tags": ["layout:us_diagnostic_report", "spelling:US", "biopsy_awaited"],
            "body": (
                "CCAD DIAGNOSTIC BREAST INTERVENTION NOTE\n"
                "Attendance: Attended.\n"
                "Procedure: Tomosynthesis-guided stereotactic 9G vacuum-assisted core biopsy of left upper outer quadrant pleomorphic microcalcifications. Specimen radiograph confirmed >15 calcifications in cores.\n"
                "Pathology Status: Biopsy taken today; formal pathology result is pending.\n"
                "Disposition / Outcome: Pending pathology review at next multidisciplinary tumor board."
            ),
        },
        {
            "id": "ha31",
            "attended": True,
            "assessment": "done",
            "biopsy": "pending",
            "outcome": "pending",
            "layout": "Arabic_bilingual_summary",
            "tags": ["layout:Arabic_bilingual_summary", "arabic", "spelling:UK", "biopsy_awaited"],
            "body": (
                "عيادة تقييم الثدي - مستشفى توام (TAWAM BREAST ASSESSMENT UNIT)\n"
                "حالة الحضور: حضرت المريضة (Attended).\n"
                "Assessment: Targeted ultrasound of the left breast (الثدي الأيسر) showed a 13 mm microlobulated solid mass at 2 o'clock. Ultrasound-guided core biopsy was taken today.\n"
                "Biopsy Status: Pending (بانتظار نتيجة الخزعة المخبرية).\n"
                "Outcome: Pending MDT decision next week (القرار النهائي قيد الانتظار لحين صدور تقرير الأنسجة)."
            ),
        },
        {
            "id": "ha32",
            "attended": True,
            "assessment": "done",
            "biopsy": "pending",
            "outcome": "pending",
            "layout": "onestop_clinic",
            "tags": ["layout:onestop_clinic", "spelling:UK", "biopsy_awaited", "prior_year_biopsy", "hard:historical_biopsy_distractor"],
            "body": (
                "SKMC BREAST ASSESSMENT CLINIC - ABU DHABI\n"
                "Attendance: Attended.\n"
                "Past History: Had a right breast core biopsy in 2022 which showed benign fibroadenoma (discharged to routine screening at that time).\n"
                "Today's Episode: Recalled for a new 11 mm indistinct mass in the contralateral left breast at 6 o'clock. Diagnostic mammogram, ultrasound, and ultrasound-guided 14G core biopsy of the left breast lesion were performed today.\n"
                "Current Biopsy Result: Formal histopathology is awaited from the lab (Pending).\n"
                "Outcome: Pending MDT review once current biopsy result returns."
            ),
        },
        {
            "id": "ha33",
            "attended": True,
            "assessment": "done",
            "biopsy": "pending",
            "outcome": "pending",
            "layout": "clinic_letter",
            "tags": ["layout:clinic_letter", "spelling:UK", "biopsy_awaited"],
            "body": (
                "BURJEEL MEDICAL CITY - BREAST ASSESSMENT PROGRESS NOTE\n"
                "Attendance: Attended recall clinic.\n"
                "We evaluated this 54-year-old lady recalled for architectural distortion in the right breast. Diagnostic tomosynthesis and ultrasound confirmed a 10 mm spiculated parenchymal distortion. "
                "An ultrasound-guided core biopsy was performed today without complication; histology results are currently awaited. "
                "Management outcome is pending next week's breast MDT meeting."
            ),
        },
        {
            "id": "ha34",
            "attended": True,
            "assessment": "done",
            "biopsy": "pending",
            "outcome": "pending",
            "layout": "us_diagnostic_report",
            "tags": ["layout:us_diagnostic_report", "spelling:US", "biopsy_awaited", "ocr_noise"],
            "body": (
                "MEDICLINIC AIRPORT ROAD - INTERVENTIONAL BREAST REPORT\n"
                "Attendance: Attended.\n"
                "Assessment: Diagnostic ultrasound of right axilla and right breast tail; 14G core needle biopsy taken from a 12 mm suspicious mass at 10 o'clock.\n"
                "Biopsy Result: Pending pathology read.\n"
                "Outcome: Pending final pathology and MDT disposition."
            ),
        },
        {
            "id": "ha35",
            "attended": True,
            "assessment": "not_done",
            "biopsy": "none",
            "outcome": "pending",
            "layout": "onestop_clinic",
            "tags": ["layout:onestop_clinic", "spelling:UK", "declined_or_unable", "hard:attended_but_no_workup"],
            "body": (
                "SSMC BREAST ASSESSMENT CLINIC - NURSING & CLINICIAN NOTE\n"
                "Attendance: Patient attended the reception desk and was checked into the clinic room.\n"
                "Assessment Status: Not done. On pre-procedure check, the patient reported an acute severe panic attack and claustrophobia and declined both the supplementary compression mammographic views and the breast ultrasound/clinical exam today. She requested to return next week with her daughter.\n"
                "Biopsy: None.\n"
                "Outcome: Pending rescheduled diagnostic assessment next week."
            ),
        },
        {
            "id": "ha36",
            "attended": True,
            "assessment": "not_done",
            "biopsy": "none",
            "outcome": "pending",
            "layout": "clinic_letter",
            "tags": ["layout:clinic_letter", "spelling:UK", "declined_or_unable", "hard:attended_but_no_workup"],
            "body": (
                "TAWAM HOSPITAL BREAST ASSESSMENT UNIT - AL AIN\n"
                "Attendance: Attended clinic appointment.\n"
                "Assessment: Not done today. Immediately prior to entering the mammography suite, the patient experienced a vasovagal syncopal episode with blood pressure 85/50 mmHg and had to be transferred to the emergency observation bay before any clinical breast exam, diagnostic mammogram, or ultrasound could be performed.\n"
                "Biopsy: None.\n"
                "Outcome: Pending completion of recall assessment once medically stabilised."
            ),
        },
        {
            "id": "ha37",
            "attended": True,
            "assessment": "not_done",
            "biopsy": "none",
            "outcome": "pending",
            "layout": "onestop_clinic",
            "tags": ["layout:onestop_clinic", "spelling:US", "declined_or_unable", "hard:attended_but_no_workup"],
            "body": (
                "CCAD DIAGNOSTIC BREAST CLINIC ENCOUNTER NOTE\n"
                "Attendance: Patient arrived and checked in at the Breast Imaging reception.\n"
                "Assessment Performed: Not done. The stereotactic/DBT mammography unit suffered an unexpected X-ray tube coolant fault and the patient could not wait for the backup room due to urgent childcare commitments; she declined ultrasound or clinical examination today and asked to rebook the entire assessment together.\n"
                "Biopsy: None.\n"
                "Outcome: Pending rebooked assessment appointment."
            ),
        },
        {
            "id": "ha38",
            "attended": True,
            "assessment": "not_done",
            "biopsy": "none",
            "outcome": "pending",
            "layout": "Arabic_bilingual_summary",
            "tags": ["layout:Arabic_bilingual_summary", "arabic", "spelling:UK", "declined_or_unable"],
            "body": (
                "عيادة تقييم الثدي - مدينة الشيخ خليفة الطبية (SKMC)\n"
                "Attendance: Attended clinic (حضرت المريضة إلى العيادة).\n"
                "Assessment: Not done (لم يتم إجراء أي فحص إضافي). The patient attended to discuss her recall letter but declined any further mammographic views, ultrasound, or clinical examination today as she wishes to obtain a second opinion abroad first.\n"
                "Biopsy: None.\n"
                "Outcome: Pending (معلق - تم إبلاغ المريضة بضرورة استكمال التقييم)."
            ),
        },
        {
            "id": "ha39",
            "attended": True,
            "assessment": "done",
            "biopsy": "none",
            "outcome": "pending",
            "layout": "us_diagnostic_report",
            "tags": ["layout:us_diagnostic_report", "spelling:US"],
            "body": (
                "CCAD DIAGNOSTIC BREAST IMAGING REPORT\n"
                "Attendance: Attended.\n"
                "Assessment Performed: Magnification views and targeted ultrasound of the left breast for a 10 mm cluster of fine pleomorphic calcifications (BI-RADS 4B). Stereotactic vacuum biopsy could not be performed today because the patient is taking therapeutic warfarin (INR 3.1).\n"
                "Biopsy: None performed today (scheduled for next Monday after 5-day warfarin hold).\n"
                "Outcome: Pending stereotactic biopsy."
            ),
        },
        {
            "id": "ha40",
            "attended": True,
            "assessment": "done",
            "biopsy": "none",
            "outcome": "pending",
            "layout": "onestop_clinic",
            "tags": ["layout:onestop_clinic", "spelling:UK"],
            "body": (
                "SSMC BREAST ASSESSMENT CLINIC - ABU DHABI\n"
                "Attendance: Attended.\n"
                "Assessment Performed: Clinical examination, spot compression views, and bilateral breast ultrasound for subtle right retroareolar distortion in extremely dense breasts.\n"
                "Findings: Ultrasound is equivocal with shadowy retroareolar fibrosis. No biopsy performed today; contrast-enhanced breast MRI has been requested to clarify whether a true enhancing mass is present.\n"
                "Outcome: Pending contrast-enhanced breast MRI and subsequent MDT review."
            ),
        },
        {
            "id": "ha41",
            "attended": True,
            "assessment": "done",
            "biopsy": "benign",
            "outcome": "pending",
            "layout": "mdt_minutes",
            "tags": ["layout:mdt_minutes", "spelling:UK"],
            "body": (
                "TAWAM HOSPITAL BREAST MDT MINUTES - AL AIN\n"
                "Attendance: Attended diagnostic clinic and core biopsy.\n"
                "Assessment & Biopsy: Recalled for a 12 mm cluster of linear calcifications in the right UOQ. Stereotactic 14G core biopsy performed; histology shows benign columnar cell change with flat epithelial atypia (B3 lesion, benign biopsy with no malignancy).\n"
                "MDT Discussion: Because of limited sampling on 14G cores, a final management decision is deferred.\n"
                "Outcome: Pending diagnostic 7G vacuum-assisted excision (VAE) before deciding between routine recall vs surgical excision."
            ),
        },
        {
            "id": "ha42",
            "attended": True,
            "assessment": "done",
            "biopsy": "benign",
            "outcome": "pending",
            "layout": "mdt_minutes",
            "tags": ["layout:mdt_minutes", "spelling:UK"],
            "body": (
                "SKMC MULTIDISCIPLINARY BREAST MEETING\n"
                "Attendance: Attended.\n"
                "Assessment & Biopsy: Recalled for a high-suspicion 17 mm spiculated mass in the left breast (BI-RADS 4C). Ultrasound-guided core biopsy returned benign normal fibrous breast stroma (B1/B2 benign tissue, no malignancy).\n"
                "MDT Discussion: Severe radiologic-pathologic discordance; the biopsy needle likely skimmed anterior to the lesion.\n"
                "Outcome: Pending urgent repeat stereotactic/ultrasound vacuum biopsy; no final outcome decision yet."
            ),
        },
        {
            "id": "ha43",
            "attended": True,
            "assessment": "done",
            "biopsy": "malignant",
            "outcome": "pending",
            "layout": "mdt_minutes",
            "tags": ["layout:mdt_minutes", "spelling:US"],
            "body": (
                "CCAD BREAST MULTIDISCIPLINARY PRE-MDT NOTE\n"
                "Attendance: Attended diagnostic assessment and core biopsy.\n"
                "Workup: Ultrasound and core needle biopsy of right 9 o'clock 15 mm mass performed.\n"
                "Pathology Result: Preliminary pathology addendum signed out this morning confirms invasive ductal carcinoma (Malignant), with HER2 IHC equivocal (2+) and FISH amplification pending.\n"
                "Outcome: Pending formal MDT board review on Thursday and completion of staging CT/bone scan before finalizing treatment plan."
            ),
        },
        # --- 7 DNA (ha44 .. ha50) ---
        {
            "id": "ha44",
            "attended": False,
            "assessment": "not_done",
            "biopsy": "none",
            "outcome": "pending",
            "layout": "onestop_clinic",
            "tags": ["layout:onestop_clinic", "spelling:UK"],
            "body": (
                "SSMC BREAST ASSESSMENT CLINIC - DID NOT ATTEND (DNA) LOG\n"
                "Scheduled Slot: 28/10/2026 09:30 AM.\n"
                "Attendance: Did Not Attend (DNA). Two SMS reminders and one telephone call from the breast care nurse went unanswered.\n"
                "Assessment: Not done.\n"
                "Biopsy: None.\n"
                "Outcome: Pending re-contact; second recall letter dispatched by registered post and copy sent to referring family physician."
            ),
        },
        {
            "id": "ha45",
            "attended": False,
            "assessment": "not_done",
            "biopsy": "none",
            "outcome": "pending",
            "layout": "Arabic_bilingual_summary",
            "tags": ["layout:Arabic_bilingual_summary", "arabic", "spelling:UK"],
            "body": (
                "إشعار عدم حضور موعد تقييم الثدي - مستشفى توام (TAWAM BREAST UNIT)\n"
                "حالة الحضور: لم تحضر المريضة الموعد المحدد (Did Not Attend / DNA).\n"
                "Reason for Recall: Right breast BI-RADS 0 asymmetry on screening mammogram.\n"
                "Assessment: Not done (لم يتم إجراء الفحص).\n"
                "Biopsy: None.\n"
                "Outcome: Pending (تمت إعادة جدولة الموعد وإرسال رسالة نصية للتذكير)."
            ),
        },
        {
            "id": "ha46",
            "attended": False,
            "assessment": "not_done",
            "biopsy": "none",
            "outcome": "pending",
            "layout": "clinic_letter",
            "tags": ["layout:clinic_letter", "spelling:UK", "hard:scheduled_plan_on_dna"],
            "body": (
                "SKMC BREAST CARE CENTRE - NON-ATTENDANCE NOTIFICATION\n"
                "Dear Dr Al Dhaheri,\n"
                "Re: Recall from Screening Mammography (Left UOQ pleomorphic calcifications, BI-RADS 4B).\n"
                "Your patient was booked into our One-Stop Assessment Clinic today for left magnification views, targeted breast ultrasound, and stereotactic core needle biopsy with potential surgical oncology referral if malignant.\n"
                "However, she Did Not Attend (DNA) her appointment today and when contacted by phone stated she had travelled to Cairo for a family emergency. "
                "No assessment or biopsy took place. Outcome remains pending her return to Abu Dhabi."
            ),
        },
        {
            "id": "ha47",
            "attended": False,
            "assessment": "not_done",
            "biopsy": "none",
            "outcome": "pending",
            "layout": "us_diagnostic_report",
            "tags": ["layout:us_diagnostic_report", "spelling:US"],
            "body": (
                "CCAD BREAST IMAGING - NO-SHOW / DNA RECORD\n"
                "Appointment Status: Patient did not attend (No-Show / DNA).\n"
                "Scheduled Study: Diagnostic mammogram and ultrasound for screening recall (right focal asymmetry).\n"
                "Assessment Performed: Not done.\n"
                "Biopsy: None.\n"
                "Disposition: Pending reschedule by patient access coordinator."
            ),
        },
        {
            "id": "ha48",
            "attended": False,
            "assessment": "not_done",
            "biopsy": "none",
            "outcome": "pending",
            "layout": "clinic_letter",
            "tags": ["layout:clinic_letter", "spelling:UK", "hard:scheduled_plan_on_dna"],
            "body": (
                "MEDICLINIC AIRPORT ROAD - BREAST SCREENING RECALL ADMINISTRATION\n"
                "Attendance Status: Did Not Attend (DNA).\n"
                "Clinical Note: This 61-year-old woman was recalled after screening showed a 20 mm spiculated mass in the right breast (BI-RADS 5). "
                "Our protocol booked her for triple assessment, ultrasound-guided core biopsy, and fast-track MDT oncology discussion. "
                "Unfortunately, she failed to attend her slot this morning. Breast care navigator is calling her next of kin urgently; no assessment or biopsy has been performed and outcome is pending."
            ),
        },
        {
            "id": "ha49",
            "attended": False,
            "assessment": "not_done",
            "biopsy": "none",
            "outcome": "pending",
            "layout": "onestop_clinic",
            "tags": ["layout:onestop_clinic", "spelling:UK", "ocr_noise"],
            "body": (
                "MADINAT ZAYED HOSPITAL (SEHA AL DHAFRA) - BREAST ASSESSMENT UNIT\n"
                "Attendance: DNA (Did not attend appointment).\n"
                "Notes: Patient unable to travel from Ghayathi due to transport issue. No diagnostic imaging, clinical exam, or biopsy performed.\n"
                "Outcome: Pending new appointment next week."
            ),
        },
        {
            "id": "ha50",
            "attended": False,
            "assessment": "not_done",
            "biopsy": "none",
            "outcome": "pending",
            "layout": "clinic_letter",
            "tags": ["layout:clinic_letter", "spelling:US"],
            "body": (
                "BURJEEL MEDICAL CITY - BREAST CLINIC NON-ATTENDANCE MEMO\n"
                "Patient did not attend (DNA) her scheduled recall assessment appointment on 30/10/2026 for evaluation of a left breast mass seen on screening mammography. "
                "Assessment: Not done. Biopsy: None. Outcome: Pending patient rebooking."
            ),
        },
    ]

    notes = []
    for sp in specs:
        label = compute_assess_label(
            sp["attended"], sp["assessment"], sp["biopsy"], sp["outcome"]
        )
        tags = list(sp["tags"])
        text = sp["body"]
        if "ocr_noise" in tags:
            text = apply_ocr_noise(text, rng)

        notes.append(
            {
                "id": sp["id"],
                "text": text,
                "key": {
                    "attended": sp["attended"],
                    "assessment": sp["assessment"],
                    "biopsy": sp["biopsy"],
                    "outcome": sp["outcome"],
                    "label": label,
                },
                "tags": tags,
            }
        )
    return notes


def verify_constraints(screen_reports: list, assess_notes: list) -> None:
    """Self-check all schema, uniqueness, label, and minimum count rules."""
    assert len(screen_reports) == 40, f"Expected 40 screen, got {len(screen_reports)}"
    assert len(assess_notes) == 50, f"Expected 50 assess, got {len(assess_notes)}"

    all_texts = [r["text"] for r in screen_reports] + [r["text"] for r in assess_notes]
    assert len(set(all_texts)) == 90, "All 90 texts must be strictly unique"

    # Check IDs
    assert [r["id"] for r in screen_reports] == [f"hb{i:02d}" for i in range(1, 41)]
    assert [r["id"] for r in assess_notes] == [f"ha{i:02d}" for i in range(1, 51)]

    # Check hard tag percentage (>= 30% in each file)
    s_hard = sum(
        1 for r in screen_reports if any(t.startswith("hard:") for t in r["tags"])
    )
    a_hard = sum(
        1 for r in assess_notes if any(t.startswith("hard:") for t in r["tags"])
    )
    assert s_hard >= 12, f"Need >=12 hard screen reports, got {s_hard}"
    assert a_hard >= 15, f"Need >=15 hard assess notes, got {a_hard}"

    # Distinct hard families across dataset
    hard_fams = {
        t
        for r in (screen_reports + assess_notes)
        for t in r["tags"]
        if t.startswith("hard:")
    }
    assert len(hard_fams) >= 8, f"Need >=8 hard families, got {len(hard_fams)}"


def main() -> None:
    rng = random.Random(SEED)
    screen_reports = build_screening_reports(rng)
    assess_notes = build_assessment_notes(rng)

    # Ensure >=30% of each file has a hard:* tag by tagging additional hard cases
    # where clinical reasoning is required.
    extra_screen_hard = {
        "hb01": "hard:prior_category_contrast",
        "hb06": "hard:prior_category_contrast",
        "hb10": "hard:prior_category_contrast",
        "hb16": "hard:prior_category_contrast",
        "hb20": "hard:prior_category_contrast",
        "hb32": "hard:prior_category_contrast",
    }
    for r in screen_reports:
        if r["id"] in extra_screen_hard and extra_screen_hard[r["id"]] not in r["tags"]:
            r["tags"].append(extra_screen_hard[r["id"]])

    extra_assess_hard = {
        "ha10": "hard:historical_biopsy_distractor",
        "ha24": "hard:historical_biopsy_distractor",
        "ha38": "hard:attended_but_no_workup",
        "ha41": "hard:b3_uncertain_potential_deferred",
        "ha42": "hard:discordant_repeat_biopsy",
    }
    for r in assess_notes:
        if r["id"] in extra_assess_hard and extra_assess_hard[r["id"]] not in r["tags"]:
            r["tags"].append(extra_assess_hard[r["id"]])

    verify_constraints(screen_reports, assess_notes)

    script_dir = os.path.dirname(os.path.abspath(__file__))
    screen_path = os.path.join(script_dir, "screen_reports.json")
    assess_path = os.path.join(script_dir, "assess_notes.json")

    screen_payload = {"source": SOURCE_STR, "reports": screen_reports}
    assess_payload = {"source": SOURCE_STR, "reports": assess_notes}

    with open(screen_path, "w", encoding="utf-8") as f:
        json.dump(screen_payload, f, indent=2, ensure_ascii=False)
        f.write("\n")

    with open(assess_path, "w", encoding="utf-8") as f:
        json.dump(assess_payload, f, indent=2, ensure_ascii=False)
        f.write("\n")


if __name__ == "__main__":
    main()
