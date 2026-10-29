#!/usr/bin/env python3
"""
Deterministic generator of 300 synthetic Abu Dhabi colonoscopy reports.
Sixth independent author (Gemini 4 Argon, Google).
Random seed: 6067. Standard library only.
"""

import hashlib
import json
import os
import random

SEED = 6067

FACILITIES = [
    ("Sheikh Shakhbout Medical City (SSMC)", "Al Mafraq, Abu Dhabi", "SEHA / Mayo Clinic JV"),
    ("Cleveland Clinic Abu Dhabi (CCAD)", "Al Maryah Island, Abu Dhabi", "M42 Health"),
    ("Tawam Hospital Endoscopy Unit", "Al Ain, Abu Dhabi Emirate", "SEHA"),
    ("Sheikh Khalifa Medical City (SKMC)", "Al Karamah, Abu Dhabi", "SEHA"),
    ("Burjeel Medical City (BMC) Day Surgery", "Mohamed Bin Zayed City, Abu Dhabi", "Burjeel Holdings"),
    ("Mediclinic Airport Road Hospital", "Airport Road, Abu Dhabi", "Mediclinic Middle East"),
    ("NMC Royal Hospital GI Suite", "Khalifa City A, Abu Dhabi", "NMC Healthcare"),
    ("Danat Al Emarat GI & Endoscopy Center", "Abu Dhabi Gate City", "M42 Health"),
    ("Al Rahba Hospital Endoscopy Unit", "Al Rahba, Abu Dhabi", "SEHA"),
    ("Corniche Hospital Endoscopy Service", "Al Bateen, Abu Dhabi", "SEHA"),
]

ENDOSCOPISTS = [
    ("Dr. Tariq Al-Mansoori, MBBS, FRCP (UK)", "GD-10482"),
    ("Dr. Fatima Al-Zaabi, MD, FACG", "GD-19384"),
    ("Dr. Rajeshwar Nair, MBBS, MD, DM (Gastro)", "GD-22810"),
    ("Dr. Alistair MacLeod, MBChB, MRCP, FEBGH", "GD-11029"),
    ("Dr. Hessa Al-Dhaheri, MBBS, Arab Board (IM)", "GD-30192"),
    ("Dr. Walid El-Shamy, MBBCh, MSc, PhD", "GD-18442"),
    ("Dr.Siobhan O'Connor, MB BCh BAO, FRCPI", "GD-27411"),
    ("Dr. Khalaf Al-Mazrouei, MD, FRCPC", "GD-14920"),
    ("Dr. Priya Sundaram, MBBS, DNB (Gastro)", "GD-29104"),
    ("Dr. Ziad Khoury, MD, American Board (GI)", "GD-16553"),
]

NURSES = [
    "RN Mary Joy Villanueva", "RN Aisha Al-Hosani", "RN Biju Thomas",
    "RN Laila Al-Kindi", "RN Grace Pascual", "RN Sawsan Darwish",
    "RN Reynaldo Santos", "RN Noura Al-blooshi"
]

INDICATIONS = [
    "DoH Opportunistic Colorectal Cancer Screening (FIT positive)",
    "IFOBT positive on routine HAAD/DoH preventive screening",
    "Average-risk screening colonoscopy (age > 40 per DoH guidelines)",
    "Surveillance: personal history of colonic adenomas",
    "Evaluation of chronic iron deficiency anaemia",
    "Unexplained altered bowel habit and intermittent haematochezia",
    "Chronic watery diarrhoea ?microscopic colitis",
    "Post-polypectomy surveillance (index procedure 3 years ago)",
    "Abdominal pain and bloating, rule out organic colonic pathology",
    "Family history of first-degree relative with CRC at age 48",
]

ARABIC_SNIPPETS = {
    "header": [
        "تقرير تنظير القولون - دائرة الصحة أبوظبي",
        "وحدة المناظير والجراحة اليومية - تقرير طبي",
        "نتيجة فحص المنظار السفلي (القولون)",
    ],
    "caecum_yes": [
        "تم الوصول إلى الأعور بنجاح وتصوير الصمام اللفائفي الأعوري.",
        "اكتمل الفحص حتى الأعور (Caecum reached).",
        "تم إدخال المنظار حتى نهاية القولون (الأعور) دون مضاعفات.",
    ],
    "caecum_no": [
        "لم يتم الوصول إلى الأعور بسبب التواء شديد في القولون السيني.",
        "فحص غير مكتمل - توقف المنظار عند القولون المستعرض.",
        "تعذر إكمال التنظير للأعور نظراً لعدم تحمل المريض.",
    ],
    "prep_adeq": [
        "تحضير القولون كان جيداً جداً ومناسباً للفحص الدقيق.",
        "نظافة القولون: مقبولة / جيدة (تسمح برؤية الغشاء المخاطي).",
        "التحضير ممتاز في جميع أجزاء القولون.",
    ],
    "prep_inadeq": [
        "تحضير القولون سيء للغاية وتوجد بقايا برازية صلبة تمنع الرؤية.",
        "نظافة القولون غير كافية (Inadequate preparation) - يوصى بإعادة الفحص.",
        "تحضير رديء لم يسمح بفحص الغشاء المخاطي بشكل آمن.",
    ],
    "no_tissue": [
        "لم يتم أخذ أي خزعات أو استئصال لحميات خلال هذا الإجراء.",
        "غشاء مخاطي سليم، لا توجد زوائد لحمية، لم تؤخذ عينات.",
    ],
    "tissue": [
        "تم استئصال زائدة لحمية وإرسالها للمختبر (قسم الأنسجة).",
        "تم أخذ خزعات متعددة من القولون وإرسالها للفحص المجهري.",
    ]
}

def spell(text, mode):
    """Apply UK or US medical spelling."""
    pairs = [
        ("caecum", "cecum"),
        ("Caecum", "Cecum"),
        ("CAECUM", "CECUM"),
        ("caecal", "cecal"),
        ("Caecal", "Cecal"),
        ("CAECAL", "CECAL"),
        ("oesophagus", "esophagus"),
        ("haemorrhoids", "hemorrhoids"),
        ("Haemorrhoids", "Hemorrhoids"),
        ("anaemia", "anemia"),
        ("diarrhoea", "diarrhea"),
        ("faecal", "fecal"),
        ("Faecal", "Fecal"),
        ("diverticulosis", "diverticulosis"),
        ("visualised", "visualized"),
        ("Visualised", "Visualized"),
        ("recognised", "recognized"),
        ("manoeuvre", "maneuver"),
        ("colour", "color"),
        ("pediatric", "paediatric"),
    ]
    if mode == "us":
        for uk, us in pairs:
            if uk == "pediatric":
                continue
            text = text.replace(uk, us)
    else:
        for uk, us in pairs:
            if uk == "pediatric":
                text = text.replace("pediatric", "paediatric")
            else:
                text = text.replace(us, uk)
    return text

def apply_ocr_noise(text, rng):
    """Inject realistic scanner/OCR noise into a report without destroying human readability."""
    replacements = [
        ("caecum", "c4ecum"), ("cecum", "cecurri"), ("colonoscopy", "co1onoscopy"),
        ("Colonoscopy", "CoIonoscopy"), ("mucosa", "rnucosa"), ("biopsy", "bi0psy"),
        ("polyp", "po1yp"), ("Preparation", "Preparat1on"), ("Boston", "Bost0n"),
        ("adequate", "adeq uate"), ("inadequate", "inadeq uate"), ("normal", "norrnal"),
        ("ileocaecal", "i1eocaecal"), ("ileocecal", "i1eocecal"), ("valve", "va1ve"),
        ("histology", "histo1ogy"), ("snare", "sn~re"), ("forceps", "f0rceps"),
        ("Withdrawal", "Withdrawa1"), ("Endoscopist", "End0scopist"),
    ]
    lines = text.split("\n")
    noisy_lines = []
    # Add scan header
    scan_hdrs = [
        "[SCANNED DOCUMENT - TIF->PDF OCR ENGINE v4.12 - SEHA ARCHIVE]",
        "--- FAX/SCAN IMPORT : QUALITY=MEDIUM : PAGE 1 OF 1 ---",
        "*** OCR TEXT LAYER (AUTO-EXTRACTED FROM IMAGESCAN_0092.TIF) ***",
    ]
    noisy_lines.append(rng.choice(scan_hdrs))
    for line in lines:
        if rng.random() < 0.45:
            for orig, noisy in replacements:
                if orig in line and rng.random() < 0.5:
                    line = line.replace(orig, noisy, 1)
        if rng.random() < 0.12 and len(line) > 20:
            pos = rng.randint(5, len(line) - 5)
            line = line[:pos] + rng.choice(["~", "|", "^", " . "]) + line[pos:]
        noisy_lines.append(line)
    return "\n".join(noisy_lines)


def build_specs():
    """Deterministically plan all 300 report specifications to satisfy exact distributions and minimums."""
    rng = random.Random(SEED)
    specs = []

    # Target truth distribution: 120 adequate (40%), 90 awaiting_histology (30%), 90 inadequate (30%)
    for i in range(120):
        # Adequate: reality_caecum=True, reality_prep=True, and either no tissue or histology is back
        if i < 72:
            specs.append({
                "truth_target": "adequate",
                "r_caecum": True, "r_prep": True, "r_tissue": False, "r_histo": False
            })
        else:
            specs.append({
                "truth_target": "adequate",
                "r_caecum": True, "r_prep": True, "r_tissue": True, "r_histo": True
            })

    for i in range(90):
        # Awaiting histology: reality_caecum=True, reality_prep=True, r_tissue=True, r_histo=False
        specs.append({
            "truth_target": "awaiting_histology",
            "r_caecum": True, "r_prep": True, "r_tissue": True, "r_histo": False
        })

    for i in range(90):
        # Inadequate: r_caecum=False or r_prep=False
        if i < 38:
            r_c, r_p = False, True
        elif i < 76:
            r_c, r_p = True, False
        else:
            r_c, r_p = False, False
        r_t = (i % 3 == 0)
        r_h = (r_t and (i % 6 == 0))
        specs.append({
            "truth_target": "inadequate",
            "r_caecum": r_c, "r_prep": r_p, "r_tissue": r_t, "r_histo": r_h
        })

    rng.shuffle(specs)

    # Assign basic properties and defaults
    layouts = ["seha_table", "ccad_narrative", "day_surgery_compact", "bilingual_form", "discharge_letter", "provation_Audit", "pathology_bundle"]
    for idx, s in enumerate(specs):
        s["idx"] = idx + 1
        s["uid"] = f"s{idx+1:03d}"
        s["layout"] = layouts[idx % len(layouts)]
        s["spelling"] = "uk" if idx % 2 == 0 else "us"
        s["arabic"] = False
        s["ocr"] = False
        s["prep_style"] = "verbal"  # verbal, bbps, ungraded
        s["caecum_stated"] = True
        s["prep_stated"] = True
        s["tissue_subtype"] = "none" if not s["r_tissue"] else ("polyp" if idx % 2 == 0 else "biopsy")
        s["hard_tags"] = []
        s["extra_tags"] = []
        s["addendum_type"] = None

    # Ensure >= 30 non-polyp biopsies among r_tissue=True
    tissue_indices = [i for i, s in enumerate(specs) if s["r_tissue"]]
    for i in tissue_indices[:35]:
        specs[i]["tissue_subtype"] = "biopsy"
    for i in tissue_indices[35:]:
        specs[i]["tissue_subtype"] = "polyp"

    # Assign unstated extent (>= 28) and ungraded prep (>= 42)
    # Some on adequate/awaiting_histology (hiding a fine procedure), some on inadequate (hidden:undocumented_failure)
    adeq_or_await = [i for i, s in enumerate(specs) if s["truth_target"] in ("adequate", "awaiting_histology")]
    inadeq_c_false = [i for i, s in enumerate(specs) if not s["r_caecum"] and s["r_prep"]]
    inadeq_p_false = [i for i, s in enumerate(specs) if s["r_caecum"] and not s["r_prep"]]

    # 22 unstated caecum on fine procedures + 6 on failed caecum = 28 unstated extent
    for i in adeq_or_await[:22]:
        specs[i]["caecum_stated"] = False
    for i in inadeq_c_false[:6]:
        specs[i]["caecum_stated"] = False
        specs[i]["extra_tags"].append("hidden:undocumented_failure")

    # 34 ungraded prep on fine procedures + 8 on failed prep = 42 ungraded prep
    for i in adeq_or_await[22:56]:
        specs[i]["prep_stated"] = False
        specs[i]["prep_style"] = "ungraded"
    for i in inadeq_p_false[:8]:
        specs[i]["prep_stated"] = False
        specs[i]["prep_style"] = "ungraded"
        specs[i]["extra_tags"].append("hidden:undocumented_failure")

    # Assign BBPS to >= 40 stated prep cases
    stated_prep_indices = [i for i, s in enumerate(specs) if s["prep_stated"]]
    for i in stated_prep_indices[:45]:
        specs[i]["prep_style"] = "bbps"

    # Now assign Hard Case Families across the remaining/compatible reports
    # Keep track of used indices for mutually exclusive structural overrides
    used_for_hard = set()

    def pick_candidates(cond_fn, count):
        cands = [i for i, s in enumerate(specs) if i not in used_for_hard and cond_fn(s)]
        chosen = cands[:count]
        for c in chosen:
            used_for_hard.add(c)
        return chosen

    # 1. hard:bbps_subscore_trap (Total >= 6, e.g. 3+3+1=7 or 3+2+1=6, where one segment is 1 -> inadequate!)
    cands = pick_candidates(lambda s: s["prep_stated"] and not s["r_prep"], 10)
    for c in cands:
        specs[c]["prep_style"] = "bbps"
        specs[c]["hard_tags"].append("hard:bbps_subscore_trap")

    # 2. hard:post_wash_salvage (Initially poor/brown effluent, washed to adequate)
    cands = pick_candidates(lambda s: s["prep_stated"] and s["r_prep"], 10)
    for c in cands:
        specs[c]["hard_tags"].append("hard:post_wash_salvage")

    # 3. hard:addendum_override (>= 12 reports where addendum flips a key field)
    # 4a: histology arrives in addendum (r_tissue=True, r_histo=True)
    cands1 = pick_candidates(lambda s: s["r_tissue"] and s["r_histo"] and s["caecum_stated"] and s["prep_stated"], 5)
    for c in cands1:
        specs[c]["addendum_type"] = "histo_arrived"
        specs[c]["hard_tags"].append("hard:addendum_override")
    # 4b: caecum corrected from yes to no in addendum (r_caecum=False, caecum_stated=True)
    cands2 = pick_candidates(lambda s: not s["r_caecum"] and s["caecum_stated"], 4)
    for c in cands2:
        specs[c]["addendum_type"] = "caecum_retracted"
        specs[c]["hard_tags"].append("hard:addendum_override")
    # 4c: Diminutive polyp biopsied/snared noted in nursing/addendum after main body said none
    cands3 = pick_candidates(lambda s: s["r_tissue"] and not s["r_histo"] and s["caecum_stated"] and s["prep_stated"], 3)
    for c in cands3:
        specs[c]["addendum_type"] = "tissue_added"
        specs[c]["hard_tags"].append("hard:addendum_override")

    # 4. hard:prior_exam_distractor (>= 14 reports with prior exam history/histology/failure)
    cands = pick_candidates(lambda s: s["caecum_stated"] and s["prep_stated"], 14)
    for c in cands:
        specs[c]["hard_tags"].append("hard:prior_exam_distractor")

    # 5. hard:negated_tissue (>= 24 reports with explicit negated tissue statements when r_tissue=False)
    cands = pick_candidates(lambda s: not s["r_tissue"], 24)
    for c in cands:
        specs[c]["hard_tags"].append("hard:negated_tissue")

    # 6. hard:distant_landmark_illusion (Visualised caecum from distance / transillumination, not intubated -> r_caecum=False)
    cands = pick_candidates(lambda s: not s["r_caecum"] and s["caecum_stated"], 8)
    for c in cands:
        specs[c]["hard_tags"].append("hard:distant_landmark_illusion")

    # 7. hard:icv_lip_only_failure (Reached ICV lip or appendiceal orifice glimpsed, cap did not enter caecal pole -> r_caecum=False)
    cands = pick_candidates(lambda s: not s["r_caecum"] and s["caecum_stated"], 8)
    for c in cands:
        specs[c]["hard_tags"].append("hard:icv_lip_only_failure")

    # 8. hard:ileocaecal_resection_anastomosis (Prior right hemicolectomy; reaching ileocolic anastomosis counts as complete extent -> r_caecum=True)
    cands = pick_candidates(lambda s: s["r_caecum"] and s["caecum_stated"], 8)
    for c in cands:
        specs[c]["hard_tags"].append("hard:ileocaecal_resection_anastomosis")

    # 9. hard:resect_and_discard_optical (Optical diagnosis only or polyp lost/not retrieved? No, if tissue is NOT taken because a hyperplastic-appearing lesion or submucosal lipoma was only optically inspected / pillow-signed WITHOUT biopsy/resection -> r_tissue=False)
    cands = pick_candidates(lambda s: not s["r_tissue"], 9)
    for c in cands:
        specs[c]["hard_tags"].append("hard:optical_lesion_not_sampled")

    # 10. hard:polyp_seen_unresected_anticoag (True polyp seen, patient on clopidogrel/apixaban or poor prep, so NOT resected or biopsied today -> r_tissue=False)
    cands = pick_candidates(lambda s: not s["r_tissue"], 9)
    for c in cands:
        specs[c]["hard_tags"].append("hard:polyp_seen_unresected_anticoag")

    # 11. hard:fecal_water_aspirated_trap (Mentions semi-solid stool/fecal fluid, naive reader thinks inadequate, but explicitly suctioned to BBPS 2/2/2 or good prep -> r_prep=True)
    cands = pick_candidates(lambda s: s["r_prep"] and s["prep_stated"], 8)
    for c in cands:
        specs[c]["hard_tags"].append("hard:fecal_water_aspirated_trap")

    # 12. hard:histo_requisition_only_not_result (Mentions pathology bottle numbers, jar labels, and clinically expected histology 'likely tubular adenoma, sent to histopathology', when r_histo=False)
    cands = pick_candidates(lambda s: s["r_tissue"] and not s["r_histo"], 10)
    for c in cands:
        specs[c]["hard_tags"].append("hard:histo_requisition_only_not_result")

    # 13. hard:therapeutic_clip_or_apc_no_tissue (APC of angiodysplasia or prophylactic clipping of diverticular bleed without tissue sampling -> r_tissue=False)
    cands = pick_candidates(lambda s: not s["r_tissue"], 8)
    for c in cands:
        specs[c]["hard_tags"].append("hard:therapeutic_clip_or_apc_no_tissue")

    # 14. hard:failed_ti_but_caecum_reached (Failed terminal ileum intubation after multiple attempts, naive reader sees "failed intubation" and marks incomplete, but caecum WAS reached -> r_caecum=True)
    cands = pick_candidates(lambda s: s["r_caecum"] and s["caecum_stated"], 10)
    for c in cands:
        specs[c]["hard_tags"].append("hard:failed_ti_but_caecum_reached")

    # Assign Arabic (>= 34 reports) and OCR noise (>= 28 reports)
    for i in range(34):
        target_idx = (i * 7 + 3) % 300
        specs[target_idx]["arabic"] = True
        if specs[target_idx]["layout"] != "bilingual_form" and i % 2 == 0:
            specs[target_idx]["layout"] = "bilingual_form"

    for i in range(28):
        target_idx = (i * 9 + 5) % 300
        specs[target_idx]["ocr"] = True

    return specs


def render_extent_section(spec, rng):
    """Generate clinical prose for extent of examination."""
    tags = spec["hard_tags"]
    c_stated = spec["caecum_stated"]
    r_c = spec["r_caecum"]
    add_type = spec["addendum_type"]

    if not c_stated:
        lines = [
            "The colonoscope was introduced through the anus and advanced proximally under direct vision. Withdrawal was carried out carefully inspecting the folds.",
            "Scope inserted per rectum and advanced without undue difficulty. Examination of the colonic lumen was performed during slow withdrawal.",
            "Instrument advanced smoothly through the colon. Retroflexion performed in the rectum prior to scope removal.",
            "Colonoscopy performed under conscious sedation. The colonic mucosa was surveyed carefully on withdrawal.",
        ]
        return rng.choice(lines)

    if add_type == "caecum_retracted":
        # Main body claims caecum reached; addendum will retract it to hepatic flexure
        return "The colonoscope was advanced to what initially appeared to be the caecal pole with visualisation of a fold resembling the ileocaecal valve."

    if "hard:distant_landmark_illusion" in tags:
        opts = [
            "The scope was advanced to the mid-ascending colon. From this vantage point, the caecal pole and appendiceal orifice were visualised at a distance, though severe looping prevented advancing the tip into the caecum itself.",
            "Right colon entered; although right iliac fossa transillumination was seen and the ileocaecal valve was glimpsed from afar, fixed angulation precluded physical intubation of the caecum.",
            "Advanced past the hepatic flexure; the caecal caput was seen distally across a fluid-filled segment, but the colonoscope tip could not be manoeuvred into the caecum due to a redundant transverse loop.",
        ]
        return rng.choice(opts)

    if "hard:icv_lip_only_failure" in tags:
        opts = [
            "The instrument tip reached the distal lip of the ileocaecal valve, but due to a tight alpha loop and patient discomfort, the scope could not be advanced past the valve into the caecal pole; the appendiceal orifice was not reached.",
            "Advanced to the level of the ileocaecal valve orifice; however, the caecal base/caput could not be intubated or fully inspected despite position change and abdominal pressure.",
            "Colonoscope reached the inferior lip of the ileocaecal valve only. Complete caecal intubation (medial wall and appendiceal orifice) was not achieved.",
        ]
        return rng.choice(opts)

    if "hard:ileocaecal_resection_anastomosis" in tags:
        opts = [
            "Note: Patient has a history of right hemicolectomy (native caecum absent). The colonoscope was successfully advanced to the end-to-side ileocolic anastomosis, which was patent and healthy, completing full colonic evaluation.",
            "Surgical anatomy: previous ileocaecal resection. The scope was advanced to the surgical ileocolonic anastomosis and neo-terminal ileum, representing the maximal anatomical extent.",
            "Status post right hemicolectomy; the native caecum and ileocaecal valve are surgically absent. Complete examination achieved to the ileocolic anastomosis at 65 cm.",
        ]
        return rng.choice(opts)

    if "hard:failed_ti_but_caecum_reached" in tags:
        opts = [
            "The caecum was reached and confirmed by the appendiceal orifice, tri-radiate fold, and ileocaecal valve. Multiple attempts at intubation of the terminal ileum failed due to a sharply angulated valve.",
            "Successful caecal intubation (photodocumented appendiceal orifice and caecal strap fold). Attempted terminal ileal intubation was unsuccessful despite butylscopolamine and position change.",
            "Scope advanced to the caecal pole (landmarks: ICV and appendiceal orifice clearly visualised). Intubation of the terminal ileum was not achieved (failed TI cannulation).",
        ]
        return rng.choice(opts)

    if r_c:
        opts = [
            "The colonoscope was advanced to the caecum, confirmed by identification of the appendiceal orifice, ileocaecal valve, and tri-radiate fold. Withdrawal time from caecum was 8 minutes.",
            "Extent of exam: Caecum reached. Landmarks photographed: ileocaecal valve, appendiceal orifice, and terminal ileum (intubated to 10 cm, normal villous mucosa).",
            "Complete colonoscopy to the caecal pole. The ileocaecal valve and appendiceal orifice were clearly visualised and photodocumented.",
            "The instrument was passed smoothly to the caecum. All three caecal landmarks (ICV, appendiceal orifice, tri-radiate fold) were positively identified.",
        ]
        return rng.choice(opts)
    else:
        opts = [
            "The procedure was abandoned at the splenic flexure due to fixed sigmoid diverticular angulation and patient discomfort. Caecum was not reached.",
            "Incomplete colonoscopy: scope could not be passed beyond a redundant, looping transverse colon despite stiffening wire and left lateral/supine position changes.",
            "Extent achieved: Descending colon only. Marked looping and fixation in the pelvis prevented further proximal advancement.",
            "The colonoscope was advanced only to the hepatic flexure; further advancement into the ascending colon and caecum was impossible due to severe paradoxical looping.",
        ]
        return rng.choice(opts)


def render_prep_section(spec, rng):
    """Generate clinical prose for bowel preparation."""
    tags = spec["hard_tags"]
    p_stated = spec["prep_stated"]
    p_style = spec["prep_style"]
    r_p = spec["r_prep"]

    if not p_stated or p_style == "ungraded":
        opts = [
            "Pre-procedure preparation: Split-dose polyethylene glycol (Moviprep) taken per patient the evening prior and morning of procedure.",
            "Bowel purge regimen: 4L PEG-ELS administered according to standard day-surgery protocol.",
            "Preparation agent: Sodium picosulfate / magnesium citrate (Picoprep) x2 sachets taken as instructed.",
            "Patient completed standard oral bowel cleansing protocol prior to arrival at the endoscopy unit.",
        ]
        return rng.choice(opts)

    if "hard:bbps_subscore_trap" in tags:
        trap_scores = [
            ("Boston Bowel Preparation Scale (BBPS): Right colon = 1, Transverse colon = 3, Left colon = 3 (Total BBPS = 7/9). Thick adherent stool in the ascending colon could not be cleared.", 1, 3, 3),
            ("BBPS Score: 6/9 (Right=2, Transverse=3, Left=1). Note: Left colon/sigmoid segment scored 1 due to semisolid faecal residue obscuring mucosal views.", 2, 3, 1),
            ("Bowel Prep (Boston Scale): RC 1 / TC 2 / LC 3 -> Total score 6. Because the right colon segment was 1/3 with unremovable particulate matter, mucosal inspection there was compromised.", 1, 2, 3),
            ("BBPS 3 + 3 + 1 = 7/9 (Right 3, Transverse 3, Rectosigmoid 1). Despite high total score, the rectosigmoid had persistent solid stool (segment score 1).", 3, 3, 1),
        ]
        return rng.choice(trap_scores)[0]

    if "hard:post_wash_salvage" in tags:
        opts = [
            "On insertion, poorly digested turbid brown fluid was encountered in the right and transverse colon; however, following vigorous irrigation with the water-jet pump and complete suctioning, mucosal visualisation was adequate throughout (post-wash BBPS: Right 2, Transverse 2, Left 3; Total 7/9).",
            "Initial views were obscured by murky effluent (initially appearing poor), but after extensive washing and aspiration during withdrawal, bowel preparation was upgraded to good/adequate in all segments.",
            "Moderate semi-liquid residue noted upon entry; all segments were thoroughly lavaged and suctioned clear, achieving adequate mucosal inspection (post-cleansing BBPS 2/2/2 = 6/9).",
        ]
        return rng.choice(opts)

    if "hard:fecal_water_aspirated_trap" in tags:
        opts = [
            "Some dark faecal-stained water was present in the dependent folds of the sigmoid and caecum, which was easily aspirated. Overall bowel preparation was good/adequate with >95% mucosa clearly visualised.",
            "Scattered pools of bile-stained liquid effluent were suctioned without difficulty; no adherent solid stool. Bowel preparation rated as fair/adequate for polyp detection.",
            "Residual turbid fluid in the transverse colon was aspirated completely, revealing clean underlying mucosa. Preparation quality: Good (BBPS 2-3-2 = 7/9).",
        ]
        return rng.choice(opts)

    if p_style == "bbps":
        if r_p:
            adeq_bbps = [
                "Boston Bowel Preparation Scale (BBPS): Right 3, Transverse 3, Left 3 (Total = 9/9 - Excellent).",
                "Bowel cleansing assessed via BBPS: R=2, T=3, L=2 (Total score 7/9, adequate).",
                "BBPS: 2 / 2 / 2 (Total 6/9). All three colonic segments achieved a score of at least 2 after washing.",
                "Preparation quality: BBPS 8/9 (Right colon 2, Transverse colon 3, Left colon 3).",
                "Boston score: LC=3, TC=2, RC=3 (Aggregate 8/9).",
            ]
            return rng.choice(adeq_bbps)
        else:
            inadeq_bbps = [
                "Boston Bowel Preparation Scale (BBPS): Right 1, Transverse 1, Left 2 (Total = 4/9 - Inadequate).",
                "BBPS score: R=0, T=1, L=2 (Total 3/9). Solid faecal matter precluded safe or reliable mucosal inspection.",
                "Preparation quality (Boston Scale): 1 / 2 / 1 (Total 4/9). Inadequate bowel cleansing.",
                "BBPS: Right colon = 1, Transverse = 2, Left = 2 (Total 5/9 - inadequate due to right colon segment < 2).",
            ]
            return rng.choice(inadeq_bbps)
    else:
        # Verbal prep
        if r_p:
            verbal_adeq = [
                "Bowel preparation quality was good throughout the colon, allowing thorough mucosal inspection.",
                "Preparation: Excellent. Clean mucosa visualised in all segments.",
                "Bowel cleansing: Fair / adequate after minor suctioning of clear fluid.",
                "Mucosal visibility was adequate (rated good on institutional scale).",
            ]
            return rng.choice(verbal_adeq)
        else:
            verbal_inadeq = [
                "Bowel preparation was poor, with semi-solid adherent stool coating the mucosa that could not be washed off.",
                "Preparation quality: Inadequate. High risk of missed lesions; repeat colonoscopy with extended split-dose prep advised.",
                "Unsatisfactory / poor bowel cleansing throughout the right and transverse colon.",
                "Bowel prep: Poor (thick particulate residue obstructing views).",
            ]
            return rng.choice(verbal_inadeq)


def render_findings_and_tissue(spec, rng):
    """Generate clinical prose for mucosal findings, polypectomy/biopsy, and pathology."""
    tags = spec["hard_tags"]
    r_t = spec["r_tissue"]
    r_h = spec["r_histo"]
    sub = spec["tissue_subtype"]
    add_type = spec["addendum_type"]

    findings = []

    # Prior exam distractor if tagged
    if "hard:prior_exam_distractor" in tags:
        priors = [
            "Clinical background: Prior colonoscopy in 2022 was incomplete (stopped at sigmoid due to poor prep, BBPS 3/9) and showed a 12 mm tubular adenoma with low-grade dysplasia in the rectum (histology ref H22-9910).",
            "History note: Previous colonoscopy 3 years ago at an outside facility had inadequate bowel preparation and piecemeal polypectomy of a sessile serrated lesion (pathology confirmed SSL without dysplasia).",
            "Comparison: Index exam in 2021 was aborted at the splenic flexure; pathology from that prior exam showed hyperplastic rectal polyps and focal active colitis.",
            "Prior records reviewed: In 2023 the patient had an incomplete colonoscopy (caecum not reached) with cold snare polypectomy of two sigmoid tubular adenomas.",
        ]
        findings.append(rng.choice(priors))

    if add_type == "tissue_added":
        # Main text claims no tissue taken; addendum will correct it
        findings.append("Colonic mucosa appeared grossly unremarkable from the proximal extent to the rectum. No masses or ulcerations seen. Initial dictation noted no tissue specimens collected.")
        return " ".join(findings)

    if not r_t:
        if "hard:optical_lesion_not_sampled" in tags:
            opts = [
                "In the ascending colon, a 10 mm smooth yellowish submucosal bulge consistent with a classic benign lipoma (positive pillow sign on closed-forceps probing) was seen; no biopsy or polypectomy was performed.",
                "Two diminutive (2 mm) pale rectosigmoid mucosal elevations with Kudo Type I pit pattern on NBI were optically diagnosed as benign lymphoid aggregates / mucosal tags; no tissue was resected or biopsied.",
                "A small 4 mm submucosal phlebectasia was noted in the sigmoid colon. Assessed with virtual chromoendoscopy (NBI); no biopsy or resection was taken.",
            ]
            findings.append(rng.choice(opts))
        elif "hard:polyp_seen_unresected_anticoag" in tags:
            opts = [
                "A 14 mm pedunculated polyp (Paris Ip) was visualised in the sigmoid colon. Because the patient is currently taking uninterrupted clopidogrel and rivaroxaban, polypectomy and biopsy were deferred; no tissue was taken today. Rescheduled for therapeutic colonoscopy off anticoagulation.",
                "In the transverse colon, an 8 mm sessile polyp was identified. However, as the patient had not held apixaban prior to this diagnostic exam, the polyp was photographed and left in situ without biopsy or snare resection.",
                "A 12 mm flat lesion (Paris IIa) was seen in the ascending colon; deferred for EMR by the advanced endoscopy service. No tissue sampling or polypectomy was performed during this session.",
            ]
            findings.append(rng.choice(opts))
        elif "hard:therapeutic_clip_or_apc_no_tissue" in tags:
            opts = [
                "Two non-bleeding caecal angiodysplasias (AVMs) were identified and ablated successfully using Argon Plasma Coagulation (APC, 1.0 L/min, 30W). No polyps seen; no biopsies or tissue specimens taken.",
                "Diverticulosis in the sigmoid colon with a visible vessel in a diverticular neck treated prophylactically with two endoclips. No mucosal biopsy or polypectomy performed.",
                "Radiation proctopathy changes noted in the distal rectum and treated with focal APC therapy. No tissue samples or biopsies were obtained.",
            ]
            findings.append(rng.choice(opts))
        elif "hard:negated_tissue" in tags:
            opts = [
                "Normal vascular pattern and mucosal architecture throughout. No polyps, masses, ulcers, or diverticula identified. Biopsies: None. Polypectomy: None.",
                "No adenomatous or serrated lesions detected on slow withdrawal with NBI. No tissue was removed or sent for histopathological analysis.",
                "Entire examined colon was macroscopically normal. Zero polyps encountered; no cold snare, hot snare, or forceps biopsy performed.",
                "Small non-bleeding internal haemorrhoids noted on retroflexion. Otherwise normal colonoscopy; no biopsies taken and no polypectomies performed.",
            ]
            findings.append(rng.choice(opts))
        else:
            opts = [
                "The colonic mucosa was normal throughout. Scattered uncomplicated left-sided diverticula noted. No polyps seen.",
                "Grossly normal examination of the colon and rectum. Grade I internal haemorrhoids on rectal retroflexion.",
                "Unremarkable mucosal surfaces throughout all inspected segments. No neoplastic lesions identified.",
            ]
            findings.append(rng.choice(opts))
        return " ".join(findings)

    # r_t is True (either polyp or biopsy)
    if sub == "polyp":
        polyp_descs = [
            "A 6 mm sessile polyp (Paris Is) in the sigmoid colon was completely resected using a cold snare and retrieved into specimen Jar A.",
            "Two polyps identified: a 4 mm diminutive polyp in the ascending colon removed by cold biopsy forceps, and an 8 mm sessile polyp in the transverse colon resected en bloc by cold snare. Both retrieved.",
            "In the descending colon, a 9 mm pedunculated polyp was removed via hot snare polypectomy following adrenaline submucosal lift; specimen retrieved for pathology.",
            "A 5 mm flat lesion (Paris IIa) in the hepatic flexure was resected completely with a cold snare and sent to the histopathology laboratory.",
        ]
        findings.append(rng.choice(polyp_descs))
    else:
        biopsy_descs = [
            "No discrete polyps or masses were seen. To evaluate chronic watery diarrhoea, random serial step biopsies (x4 right colon, x4 left colon) were obtained with cold forceps to rule out microscopic colitis.",
            "No polyps identified. Mild patchy erythema and loss of vascular pattern noted in the rectosigmoid; targeted mucosal biopsies (Jar 1) were taken with cold biopsy forceps.",
            "Macroscopically normal colonic mucosa without polyps. Random colonic biopsies taken from the ascending, transverse, and descending colon per chronic diarrhoea protocol.",
            "Aphthous erosions noted near the ileocaecal valve without polyps; cold forceps biopsies obtained for histopathological correlation.",
        ]
        findings.append(rng.choice(biopsy_descs))

    # Handle histology status
    if add_type == "histo_arrived":
        findings.append("Tissue specimen dispatched to SEHA Central Anatomic Pathology Laboratory (status at time of initial sign-off: pending).")
    elif r_h:
        if sub == "polyp":
            histo_results = [
                "HISTOPATHOLOGY RESULT (Integrated Lab Feed - Verified): Sigmoid colon polyp, cold snare: Tubular adenoma with low-grade dysplasia; resection margins clear.",
                "PATHOLOGY REPORT ATTACHED: Specimen A (colon polyp): Tubulovillous adenoma, negative for high-grade dysplasia. Specimen B: Hyperplastic polyp.",
                "FINAL HISTOLOGY DIAGNOSIS: Sessile serrated lesion (SSL) without cytological dysplasia, completely excised.",
                "MICROSCOPIC DESCRIPTION & DIAGNOSIS: Colonic polyp biopsy/snare shows benign tubular adenoma (LGD). Surveillance interval: 3-5 years.",
            ]
        else:
            histo_results = [
                "HISTOPATHOLOGY RESULT (Verified): Random colonic biopsies: Colonic mucosa with thickened subepithelial collagen band (>10 um) and intraepithelial lymphocytosis, diagnostic of collagenous colitis.",
                "PATHOLOGY REPORT ATTACHED: Random right and left colon biopsies show normal crypt architecture without active inflammation, granulomas, or microscopic colitis.",
                "FINAL HISTOLOGY DIAGNOSIS: Rectosigmoid mucosal biopsy: Mild chronic non-specific colitis; negative for dysplasia or CMV.",
                "MICROSCOPIC DIAGNOSIS: Colon, random biopsies: Lymphocytic colitis (>20 IELs per 100 surface epithelial cells). Negative for malignancy.",
            ]
        findings.append(rng.choice(histo_results))
    else:
        if "hard:histo_requisition_only_not_result" in tags:
            req_traps = [
                "PATHOLOGY REQUISITION FORM GENERATED: Container #1 (10% neutral buffered formalin) barcode ADH-88412-P. Endoscopic impression entered on requisition: 'Likely benign tubular adenoma vs hyperplastic polyp - rule out dysplasia'. Specimen in transit to histopathology; final report awaited.",
                "Lab Order Summary: Surgical Pathology Level IV (CPT 88305) ordered. Provisional clinical diagnosis on specimen label: 'Suspected collagenous colitis / adenoma'. Awaiting histological processing.",
                "Specimen logged into Cerner PathNet (Accession pending). Clinical differential noted on path slip: tubular adenoma vs sessile serrated polyp. Patient to be contacted once pathology is reported.",
            ]
            findings.append(rng.choice(req_traps))
        else:
            pending_notes = [
                "Specimens placed in formalin and sent to histopathology; results pending.",
                "Tissue submitted to pathology laboratory. Follow up in GI clinic with histopathology results in 2 weeks.",
                "Pathology: Pending microscopic examination. Surveillance plan to be finalised upon receipt of histology.",
                "Specimen retrieved and dispatched to laboratory; awaiting final pathology report.",
            ]
            findings.append(rng.choice(pending_notes))

    return " ".join(findings)


def render_addendum(spec, rng):
    """Render an addendum if spec['addendum_type'] is set."""
    add_type = spec["addendum_type"]
    if not add_type:
        return ""
    if add_type == "histo_arrived":
        if spec["tissue_subtype"] == "polyp":
            return (
                "\n\n=== ADDENDUM / SUPPLEMENTARY PATHOLOGY LINK (Signed 4 days post-procedure) ===\n"
                "Histopathology result received and reviewed: Colonic polyp (cold snare) shows benign tubular adenoma "
                "with low-grade dysplasia, completely excised. Surveillance colonoscopy recommended in 5 years."
            )
        else:
            return (
                "\n\n=== ADDENDUM / SUPPLEMENTARY PATHOLOGY LINK (Signed 5 days post-procedure) ===\n"
                "Histopathology result received and reviewed: Random colonic biopsies show normal colonic mucosa "
                "with no evidence of microscopic (collagenous or lymphocytic) colitis or dysplasia."
            )
    elif add_type == "caecum_retracted":
        return (
            "\n\n=== ADDENDUM / CORRECTION TO PROCEDURE REPORT (Entered same day 16:40) ===\n"
            "Correction to Extent of Examination: Upon review of endoscopic photo-documentation and fluoroscopic/magnetic "
            "scope imager configuration with the senior consultant, the fold previously thought to be the ileocaecal valve "
            "was in fact a prominent hepatic flexure fold. The caecum was NOT reached (examination limited to hepatic flexure). "
            "Patient scheduled for CT colonography."
        )
    elif add_type == "tissue_added":
        return (
            "\n\n=== ADDENDUM / NURSING SPECIMEN RECONCILIATION (Entered same day 15:15) ===\n"
            "Correction to Procedure Findings: During withdrawal in the rectum, a 4 mm diminutive sessile polyp was "
            "in fact removed with cold biopsy forceps and placed in Formalin Jar #1 (omitted from initial template dictation). "
            "Specimen has been sent to histopathology; results are currently pending."
        )
    return ""


def render_arabic_block(spec, rng):
    """Render realistic Arabic clinical documentation matching the report's key fields."""
    if not spec["arabic"]:
        return ""
    lines = [rng.choice(ARABIC_SNIPPETS["header"])]
    if spec["caecum_stated"] and spec["addendum_type"] != "caecum_retracted":
        if spec["r_caecum"]:
            lines.append("- مدى الفحص: " + rng.choice(ARABIC_SNIPPETS["caecum_yes"]))
        else:
            lines.append("- مدى الفحص: " + rng.choice(ARABIC_SNIPPETS["caecum_no"]))
    if spec["prep_stated"]:
        if spec["r_prep"]:
            lines.append("- تحضير القولون: " + rng.choice(ARABIC_SNIPPETS["prep_adeq"]))
        else:
            lines.append("- تحضير القولون: " + rng.choice(ARABIC_SNIPPETS["prep_inadeq"]))
    if spec["r_tissue"]:
        lines.append("- العينات: " + rng.choice(ARABIC_SNIPPETS["tissue"]))
    else:
        lines.append("- العينات: " + rng.choice(ARABIC_SNIPPETS["no_tissue"]))
    return "\n".join(lines)


def format_report_text(spec, rng):
    """Assemble the complete report using one of 7 distinct layouts."""
    fac_name, fac_loc, fac_net = rng.choice(FACILITIES)
    doc_name, doc_lic = rng.choice(ENDOSCOPISTS)
    nurse_name = rng.choice(NURSES)
    indication = rng.choice(INDICATIONS)

    mrn = f"ADH-{rng.randint(100000, 999999)}-{spec['idx']:03d}"
    emirates_id = f"784-{rng.randint(1945, 1985)}-{rng.randint(1000000, 9999999)}-{rng.randint(1, 9)}"
    age = rng.randint(41, 79)
    sex = rng.choice(["Male", "Female"])
    proc_date = f"2026-{rng.randint(1, 10):02d}-{rng.randint(1, 28):02d}"
    sedation = rng.choice([
        "Midazolam 3 mg IV + Fentanyl 50 mcg IV (Conscious Sedation)",
        "Propofol TCI (Monitored Anaesthesia Care by Anaesthetist)",
        "Midazolam 2.5 mg IV + Pethidine 25 mg IV",
        "Entonox (Nitrous Oxide) on demand - unsedated protocol",
    ])
    scope_model = rng.choice(["Olympus CF-HQ190L", "Olympus CF-EZ1500DL", "Pentax EC38-i10L", "Fujifilm EC-760ZP-V/L"])

    extent_txt = render_extent_section(spec, rng)
    prep_txt = render_prep_section(spec, rng)
    findings_txt = render_findings_and_tissue(spec, rng)
    addendum_txt = render_addendum(spec, rng)
    arabic_txt = render_arabic_block(spec, rng)

    layout = spec["layout"]

    if layout == "seha_table":
        body = (
            f"========================================================================\n"
            f"{fac_name.upper()} | {fac_net}\n"
            f"Location: {fac_loc} | DoH Facility Endoscopy Record\n"
            f"========================================================================\n"
            f"Patient MRN   : {mrn:<18} Emirates ID : {emirates_id}\n"
            f"Age / Sex     : {age}Y / {sex:<12} Exam Date   : {proc_date}\n"
            f"Endoscopist   : {doc_name} (DoH Lic: {doc_lic})\n"
            f"Assisting RN  : {nurse_name:<18} Scope ID    : {scope_model}\n"
            f"------------------------------------------------------------------------\n"
            f"INDICATION    : {indication}\n"
            f"SEDATION      : {sedation}\n"
            f"------------------------------------------------------------------------\n"
            f"BOWEL PREPARATION:\n  {prep_txt}\n\n"
            f"EXTENT OF EXAMINATION:\n  {extent_txt}\n\n"
            f"FINDINGS, INTERVENTIONS & PATHOLOGY:\n  {findings_txt}\n"
        )
        if arabic_txt:
            body += f"------------------------------------------------------------------------\n{arabic_txt}\n"
        body += "========================================================================" + addendum_txt

    elif layout == "ccad_narrative":
        body = (
            f"{fac_name} - Digestive Disease Institute\n"
            f"{fac_loc}\n\n"
            f"OPERATIVE ENDOSCOPY NARRATIVE (Ref: {mrn} / Date: {proc_date})\n"
            f"Patient: {age}-year-old {sex.lower()} (EID: {emirates_id})\n"
            f"Attending Physician: {doc_name} [License {doc_lic}]\n\n"
            f"Clinical Indication: The patient was referred for {indication.lower()}. Informed consent was obtained "
            f"and a time-out was completed with {nurse_name}.\n\n"
            f"Procedure Description: Under {sedation}, a digital rectal exam was unremarkable. Using the {scope_model} "
            f"high-definition colonoscope: {extent_txt} {prep_txt}\n\n"
            f"Mucosal Evaluation and Specimens: {findings_txt}\n\n"
            f"Complications: None immediate. Patient discharged from recovery in stable condition."
        )
        if arabic_txt:
            body += f"\n\n[Arabic Summary / ملخص التقرير]\n{arabic_txt}"
        body += addendum_txt

    elif layout == "day_surgery_compact":
        body = (
            f"[{fac_name} - Day Surgery Endoscopy Log #{spec['uid'].upper()}]\n"
            f"MRN: {mrn} | Age/Sex: {age}/{sex[0]} | Date: {proc_date} | MD: {doc_name}\n"
            f"Ind: {indication} | Meds: {sedation}\n"
            f"* Prep: {prep_txt}\n"
            f"* Extent: {extent_txt}\n"
            f"* Findings/Specimens: {findings_txt}"
        )
        if arabic_txt:
            body += f"\n* AR: {arabic_txt.replace(chr(10), ' | ')}"
        body += addendum_txt

    elif layout == "bilingual_form":
        ar_block = arabic_txt if arabic_txt else (
            "تقرير تنظير القولون الموحد - أبوظبي\n"
            "تم إجراء الفحص تحت التهدئة الوريدية وفق البروتوكول المعتمد."
        )
        body = (
            f"DEPARTMENT OF HEALTH - ABU DHABI (DoH) BILINGUAL ENDOSCOPY RECORD\n"
            f"Facility: {fac_name} ({fac_loc})\n"
            f"MRN: {mrn} | EID: {emirates_id} | Date: {proc_date} | Age/Gender: {age}/{sex}\n"
            f"Consultant: {doc_name} ({doc_lic}) | Nurse: {nurse_name}\n"
            f"-------------------------------------------------------------------\n"
            f"{ar_block}\n"
            f"-------------------------------------------------------------------\n"
            f"1. Indication: {indication}\n"
            f"2. Sedation & Instrument: {sedation}; {scope_model}\n"
            f"3. Bowel Cleansing Assessment: {prep_txt}\n"
            f"4. Anatomical Extent Reached: {extent_txt}\n"
            f"5. Endoscopic Findings & Histopathology: {findings_txt}"
        ) + addendum_txt

    elif layout == "discharge_letter":
        body = (
            f"To: Referring Primary Care / DoH Screening Physician\n"
            f"From: {doc_name}, {fac_name} ({fac_loc})\n"
            f"Date of Procedure: {proc_date} | Patient MRN: {mrn} ({age}yo {sex})\n\n"
            f"Dear Colleague,\n"
            f"Thank you for referring this patient for colonoscopy ({indication}).\n\n"
            f"Procedure Summary:\n"
            f"- Sedation & Scope: {sedation} ({scope_model}, assisted by {nurse_name}).\n"
            f"- Bowel Preparation: {prep_txt}\n"
            f"- Extent of Exam: {extent_txt}\n"
            f"- Findings & Tissue Sampling: {findings_txt}\n\n"
            f"Yours sincerely,\n{doc_name} (DoH Lic: {doc_lic})"
        )
        if arabic_txt:
            body += f"\n\n{arabic_txt}"
        body += addendum_txt

    elif layout == "provation_Audit":
        body = (
            f"PROVATION MD / GI QUALITY AUDIT EXPORT - {fac_name.upper()}\n"
            f"Record ID: {mrn} | EID: {emirates_id} | Service Date: {proc_date}\n"
            f"Provider: {doc_name} ({doc_lic}) | RN: {nurse_name} | Scope: {scope_model}\n"
            f"Patient Demographics: {age} yr {sex}\n"
            f"  [INDICATION]       {indication}\n"
            f"  [SEDATION]         {sedation}\n"
            f"  [PREP QUALITY]     {prep_txt}\n"
            f"  [CAECAL EXTENT]    {extent_txt}\n"
            f"  [MUCOSA & TISSUE]  {findings_txt}"
        )
        if arabic_txt:
            body += f"\n  [ARABIC NOTES]\n{arabic_txt}"
        body += addendum_txt

    else:  # pathology_bundle
        body = (
            f"*** INTEGRATED ENDOSCOPY & ANATOMIC PATHOLOGY SUMMARY ***\n"
            f"Institution : {fac_name} ({fac_loc})\n"
            f"Patient ID  : {mrn} (EID {emirates_id}) | {age}Y {sex} | Date: {proc_date}\n"
            f"Endoscopist : {doc_name} [{doc_lic}] | Nurse: {nurse_name}\n\n"
            f"SECTION A - ENDOSCOPIC PROCEDURE\n"
            f"Indication: {indication}\n"
            f"Instrument & Sedation: {scope_model}; {sedation}\n"
            f"Bowel Preparation: {prep_txt}\n"
            f"Extent of Examination: {extent_txt}\n\n"
            f"SECTION B - FINDINGS, SPECIMENS & HISTOPATHOLOGY STATUS\n"
            f"{findings_txt}"
        )
        if arabic_txt:
            body += f"\n\nSECTION C - ARABIC DOCUMENTATION\n{arabic_txt}"
        body += addendum_txt

    # Apply UK/US spelling
    body = spell(body, spec["spelling"])

    # Apply OCR noise if flagged
    if spec["ocr"]:
        body = apply_ocr_noise(body, rng)

    return body


def compute_keys_and_tags(spec):
    """Compute truth, key_fields, key_state, key_reason, and tags strictly from spec."""
    r_c = spec["r_caecum"]
    r_p = spec["r_prep"]
    r_t = spec["r_tissue"]
    r_h = spec["r_histo"]

    # 1. truth from hidden reality
    if (not r_c) or (not r_p):
        truth = "inadequate"
    elif r_t and (not r_h):
        truth = "awaiting_histology"
    else:
        truth = "adequate"
    assert truth == spec["truth_target"], f"Truth mismatch: {truth} != {spec['truth_target']}"

    # 2. key_fields from what the text supports (after any addendum correction)
    if not spec["caecum_stated"]:
        kf_caecum = "not_stated"
    else:
        kf_caecum = "yes" if r_c else "no"

    if not spec["prep_stated"]:
        kf_prep = "not_stated"
    else:
        kf_prep = "adequate" if r_p else "inadequate"

    kf_polyps = "yes" if r_t else "no"
    kf_tissue = kf_polyps
    kf_histo = "yes" if (r_t and r_h) else "no"

    key_fields = {
        "caecum": kf_caecum,
        "prep": kf_prep,
        "polyps": kf_polyps,
        "tissue_taken": kf_tissue,
        "histology": kf_histo,
    }

    # 3. key_state / key_reason applied in exact order to key_fields
    if kf_caecum == "no":
        key_state, key_reason = "not_closed", "incomplete_exam"
    elif kf_prep == "inadequate":
        key_state, key_reason = "not_closed", "inadequate_prep"
    elif kf_caecum == "not_stated" or kf_prep == "not_stated":
        key_state, key_reason = "needs_human", "missing_documentation"
    elif kf_tissue == "yes" and kf_histo == "no":
        key_state, key_reason = "not_closed", "awaiting_histology"
    else:
        key_state, key_reason = "closed", "criteria_met"

    # 4. Assemble tags
    tags = [
        f"fmt:{spec['layout']}",
        f"spelling:{spec['spelling']}",
        f"prep:{spec['prep_style']}",
        f"tissue:{spec['tissue_subtype']}",
    ]
    if not spec["caecum_stated"]:
        tags.append("extent:not_stated")
    else:
        tags.append("extent:stated")

    if spec["arabic"]:
        tags.append("lang:arabic")
    if spec["ocr"]:
        tags.append("noise:ocr")

    for t in spec["extra_tags"]:
        if t not in tags:
            tags.append(t)
    for t in spec["hard_tags"]:
        if t not in tags:
            tags.append(t)

    return truth, key_fields, key_state, key_reason, tags


def main():
    rng = random.Random(SEED + 101)
    specs = build_specs()
    reports = []
    seen_texts = set()

    for spec in specs:
        text = format_report_text(spec, rng)
        assert text not in seen_texts, f"Duplicate report text generated for {spec['uid']}"
        seen_texts.add(text)

        sha = hashlib.sha256(text.encode("utf-8")).hexdigest()
        truth, key_fields, key_state, key_reason, tags = compute_keys_and_tags(spec)

        reports.append({
            "uid": spec["uid"],
            "sha": sha,
            "text": text,
            "truth": truth,
            "key_fields": key_fields,
            "key_state": key_state,
            "key_reason": key_reason,
            "tags": tags,
        })

    payload = {
        "source": "Sixth independent author (Gemini 4 Argon, Google) - deterministic stdlib generator gen_heldout6.py, random seed 6067, 300 synthetic Abu Dhabi colonoscopy reports, 2026-10-29",
        "reports": reports,
    }

    out_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "reports_heldout6.json")
    with open(out_path, "w", encoding="utf-8") as f:
        json.dump(payload, f, ensure_ascii=False, indent=2)


if __name__ == "__main__":
    main()
