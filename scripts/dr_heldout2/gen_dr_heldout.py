#!/usr/bin/env python3
"""gen_dr_heldout.py: deterministic synthetic diabetic-eye screening data."""
import json
import random
from collections import Counter
from pathlib import Path

SEED = 9137
SOURCE = (
    "Independent author (gpt-6.1-sol) - deterministic stdlib generator "
    "gen_dr_heldout.py, random seed 9137, synthetic diabetic retinopathy "
    "texts, 2026-10-31"
)

# Hard-family aliases keep the specification tables readable.
FAMILIES = {
    "H": "history_vs_today",
    "N": "negation",
    "L": "location_and_acuity",
    "B": "laterality",
    "U": "ungradable_override",
    "A": "activity_vs_scars",
    "C": "mimic_or_comorbidity",
    "W": "workflow_vs_examination",
    "P": "planned_is_treated",
    "D": "disposition_semantics",
}

# Fields: right R, left R, right M, left M, explicit urgent request,
# hard-family aliases, contextual statement. Row order determines IDs.
SCREEN = [
    ("0", "0", "0", "0", False, "H",
     "Previous screening: R1M0. Today's photographs show none of those lesions."),
    ("0", "0", "0", "0", False, "C",
     "Small peripheral choroidal naevus on the left is incidental, not diabetic disease."),
    ("0", "0", "0", "0", False, "N",
     "Referral query said 'possible new vessels'; grading finds no new vessels in either eye."),
    ("0", "0", "0", "0", False, "",
     "Routine annual photography; image quality adequate bilaterally."),
    ("0", "0", "0", "0", False, "",
     "Both macular fields and disc fields are satisfactory."),
    ("0", "0", "0", "0", False, "",
     "Repeat images after a blink are clear in both eyes."),
    ("0", "0", "0", "0", False, "",
     "Grading completed on the current bilateral screening image set."),
    ("0", "0", "0", "0", False, "",
     "Routine diabetic screening encounter; no referral requested."),
    ("0", "0", "0", "0", False, "",
     "Visual acuity is 6/6 in each eye; photographs are adequate."),
    ("0", "0", "0", "0", False, "",
     "No diabetic maculopathy on either current screening image."),
    ("0", "0", "0", "0", False, "",
     "Annual recall to the primary-care diabetic eye screening service."),
    ("0", "0", "0", "0", False, "",
     "Disc and macular photographs independently checked and accepted."),
    ("1", "0", "0", "0", False, "H",
     "Previous screening: R0M0. A few right-eye microaneurysms are new today."),
    ("1", "0", "0", "0", False, "L",
     "The right-eye acuity is 6/9: the isolated near-foveal microaneurysm "
     "does not meet the acuity-dependent maculopathy criterion."),
    ("1", "1", "0", "0", False, "L",
     "Sparse hard exudates lie outside the macula and more than one disc "
     "diameter from either fovea; no macular group of exudates."),
    ("0", "1", "0", "0", False, "B",
     "The right eye is normal; background changes belong to the left eye only."),
    ("1", "0", "0", "0", False, "C",
     "A dark streak crossing the left photograph is a camera artefact, "
     "not a vitreous haemorrhage; the repeat image is clear."),
    ("1", "1", "0", "0", False, "",
     "Background lesions only; no pre-proliferative features."),
    ("1", "0", "0", "0", False, "",
     "No venous beading, IRMA or new vessels in either eye."),
    ("0", "1", "0", "0", False, "",
     "Left peripheral microaneurysms only; both maculae are spared."),
    ("2", "1", "0", "0", False, "H",
     "Previous screening: R1M0. Right-eye venous beading is present today."),
    ("2", "0", "0", "0", False, "N",
     "Right IRMA is definite despite absence of venous beading; "
     "there are no new vessels."),
    ("2", "U", "0", "", False, "U",
     "The right eye independently warrants referral; left image failure "
     "does not erase the right-eye grade."),
    ("2", "1", "0", "1", False, "B",
     "The pre-proliferative changes are on the right, while qualifying "
     "macular exudates are on the left."),
    ("2", "2", "1", "0", False, "",
     "Bilateral pre-proliferative disease with right maculopathy."),
    ("3A", "1", "0", "0", False, "H",
     "Previous screening: R1M0. Active right proliferative disease is now visible."),
    ("3A", "0", "0", "0", False, "N",
     "No disc new vessels, but active new vessels elsewhere are visible on the right."),
    ("3A", "1", "0", "1", False, "B",
     "Active proliferative disease is right-sided; maculopathy is left-sided."),
    ("3S", "1", "0", "0", False, "HA",
     "Previous screening: R3AM0 before laser. Today the right vessels are "
     "fibrosed with no active new vessels."),
    ("U", "3S", "", "0", False, "UA",
     "The left eye has stable treated proliferative disease and independently "
     "warrants referral; the right eye cannot be graded."),
    ("3S", "1", "1", "0", False, "A",
     "Stable proliferative status does not exclude coexisting right maculopathy."),
    ("0", "0", "1", "0", False, "C",
     "Right OCT confirms focal retinal thickening consistent with diabetic "
     "macular oedema, despite no visible retinopathy lesions on photographs."),
    ("1", "0", "1", "0", False, "L",
     "Right acuity is 6/12 with a haemorrhage within one disc diameter of "
     "the fovea: this meets the acuity-dependent maculopathy criterion."),
    ("1", "1", "0", "1", False, "H",
     "Previous screening: R0M0. Today there is background disease and "
     "a qualifying left macular group of exudates."),
    ("U", "0", "", "0", False, "U",
     "Right photographs remain obscured by cataract. The normal left eye "
     "is not independently referable."),
    ("0", "U", "0", "", False, "U",
     "Left images could not be obtained adequately. The right eye is normal."),
    ("U", "U", "", "", False, "UH",
     "Previous screening: R0M0. Neither eye can be graded from today's "
     "blurred images; this is not evidence of a normal current screen."),
    ("U", "1", "", "0", False, "U",
     "The left eye has background disease only without maculopathy; "
     "the ungradable right eye determines the combined ungradable result."),
    ("1", "0", "1", "0", True, "",
     "Marked recent right visual deterioration accompanies the macular finding."),
    ("2", "1", "0", "0", True, "",
     "Rapid progression of right pre-proliferative changes prompts escalation."),
]

R_FINDINGS = {
    "0": "No microaneurysms, haemorrhages, venous abnormalities or new vessels.",
    "1": "A few microaneurysms and dot haemorrhages away from the macula.",
    "2": "Venous beading, IRMA and multiple deep blot haemorrhages.",
    "3A": "Active new vessels elsewhere with a small peripheral pre-retinal haemorrhage.",
    "3S": "Panretinal laser scars and fibrosed vessels; no active new vessels "
          "or fresh pre-retinal haemorrhage.",
}
M_FINDINGS = {
    "0": "No macular exudates or retinal thickening; no microaneurysm or "
         "haemorrhage within one disc diameter of the foveal centre.",
    "1": "A group of hard exudates within the macula, including exudate "
         "within one disc diameter of the foveal centre.",
}

# Whole-eye overrides avoid contradicting the acuity-dependent examples.
EYE_OVERRIDES = {
    (14, 0): "One microaneurysm within one disc diameter of the foveal centre; "
             "acuity 6/9. No exudates or retinal thickening and no other lesions.",
    (22, 0): "Definite IRMA and multiple deep blot haemorrhages outside the "
             "macula, without venous beading or new vessels. No maculopathy.",
    (32, 0): "No visible microaneurysms, haemorrhages, exudates or new vessels. "
             "OCT shows focal retinal thickening at the fovea consistent with "
             "diabetic macular oedema.",
    (33, 0): "One dot haemorrhage within one disc diameter of the foveal centre; "
             "acuity 6/12. No exudates, retinal thickening, venous beading or new vessels.",
}

# Fields: attended, fundus status, outcome, hard-family aliases, narrative.
# Treatments are retinal laser/PRP or intravitreal therapy, not cataract surgery.
CLINIC = [
    (True, "examined", "treated", "",
     "Dilated fundus examination confirms right active proliferative disease. "
     "Right PRP performed today; left fundus has background changes."),
    (True, "examined", "treated", "",
     "Both retinae examined. Centre-involving right diabetic macular oedema "
     "confirmed; intravitreal aflibercept given today."),
    (True, "examined", "treated", "",
     "Bilateral fundus examination completed. Persistent left diabetic "
     "macular oedema treated with intravitreal steroid implant today."),
    (True, "examined", "treated", "P",
     "Both fundi examined; right disc new vessels confirmed. PRP booked "
     "for next week, not performed during this visit."),
    (True, "examined", "treated", "P",
     "Dilated examination of both retinae shows bilateral macular oedema. "
     "Anti-VEGF loading course started by booking the first injection."),
    (True, "examined", "treated", "",
     "Fundus examined in each eye. Focal retinal laser to left leaking "
     "microaneurysms completed today."),
    (True, "examined", "treated", "",
     "Both fundi reviewed after dilation. Right proliferative disease remains "
     "active; additional PRP delivered."),
    (True, "examined", "treated", "",
     "Both retinae examined and OCT reviewed. Bevacizumab injected into "
     "the left eye for diabetic macular oedema."),
    (True, "examined", "treated", "P",
     "Dilated fundus examination confirms right diabetic macular oedema. "
     "Aflibercept injection booked; patient has not yet received the dose."),
    (True, "examined", "treated", "",
     "Retinal examination completed bilaterally. Left new vessels elsewhere "
     "treated with scatter retinal laser today."),
    (True, "examined", "followup", "H",
     "Old PRP was performed three years ago. Both fundi examined today: "
     "stable scars, no active new vessels. Observe with review in six months."),
    (True, "examined", "followup", "N",
     "Both fundi examined: moderate background retinopathy without oedema. "
     "Laser not needed; review in six months."),
    (True, "examined", "followup", "N",
     "Dilated examination of both retinae and OCT show mild non-centre "
     "oedema. Anti-VEGF is not indicated; repeat OCT in four months."),
    (True, "examined", "followup", "N",
     "Both fundi examined. Injection discussed but explicitly deferred, "
     "not started or booked. Observe borderline right macular thickening."),
    (True, "examined", "followup", "C",
     "Cataract surgery was performed last month. Both retinae are examined "
     "today: mild diabetic changes only. No retinal treatment; hospital review "
     "with OCT in six months."),
    (True, "examined", "followup", "D",
     "Both fundi examined. Discharged from the injection chair today, "
     "not from medical retina: no injection given or booked. Retain hospital "
     "follow-up for OCT in eight weeks."),
    (True, "examined", "followup", "N",
     "Bilateral retinal examination finds stable background disease. "
     "PRP not required and no injection planned; review in nine months."),
    (True, "examined", "followup", "H",
     "Referral cites last year's macular oedema. Both retinae examined "
     "today; oedema has resolved without current treatment. Review in six months."),
    (True, "examined", "followup", "N",
     "Both fundi examined after dilation. No laser today or planned; "
     "non-centre macular exudates will be observed with repeat imaging."),
    (True, "examined", "followup", "",
     "Both retinae examined. Stable pre-proliferative changes without "
     "maculopathy; repeat dilated review in three months."),
    (True, "examined", "discharged", "D",
     "Both fundi examined: no diabetic retinopathy. Discharged from the "
     "hospital eye clinic back to the primary-care screening programme."),
    (True, "examined", "discharged", "D",
     "Bilateral dilated examination shows background retinopathy only. "
     "Return to annual diabetic eye screening; no hospital review arranged."),
    (True, "examined", "discharged", "H",
     "An injection was given elsewhere two years ago, not today. Both "
     "fundi examined now: no active disease or maculopathy. Discharge "
     "back to the screening programme."),
    (True, "examined", "discharged", "",
     "Retinal examination completed bilaterally. No referable diabetic "
     "disease; returned to routine primary-care diabetic eye screening."),
    (True, "examined", "discharged", "",
     "Both fundi examined: sparse peripheral microaneurysms, no maculopathy. "
     "Discharge to annual screening."),
    (True, "examined", "discharged", "",
     "Dilated fundus examination is normal in each eye. Hospital episode "
     "closed and screening programme asked to resume recall."),
    (True, "examined", "discharged", "",
     "Both retinae examined; the photographic referral concern is not "
     "confirmed. Return to the diabetic eye screening service."),
    (True, "examined", "discharged", "",
     "Bilateral retinal examination shows mild background changes only. "
     "No specialist surveillance needed; discharge to screening."),
    (True, "examined", "discharged", "",
     "Both fundi assessed after dilation, without referable diabetic "
     "findings. Annual primary-care retinal screening to continue."),
    (True, "examined", "discharged", "D",
     "Both retinae examined. Discharged specifically to the diabetic "
     "screening programme, not to another hospital clinic; no hospital follow-up."),
    (True, "not_examined", "none", "W",
     "Patient attended. Vision RE 6/9, LE 6/9 and pressures recorded only. "
     "No dilation and no retinal examination; rebook full assessment."),
    (True, "not_examined", "none", "W",
     "Patient attended for triage. Vision OD 20/30, OS 20/25. Left before "
     "dilation; neither retina examined."),
    (True, "not_examined", "none", "W",
     "Seen in clinic; vision right 6/12, left 6/9. Dilation declined "
     "because the patient must drive. Fundus not examined, rather than obscured."),
    (True, "not_examined", "none", "W",
     "Attendance confirmed. Vision RE 6/6, LE 6/6. Pressure-only pathway "
     "completed; retinal examination deliberately postponed."),
    (True, "not_examined", "none", "W",
     "Patient seen, vision 6/18 right and 6/12 left. Drops not administered "
     "because the visit ended early; no fundus examination attempted."),
    (True, "not_examined", "none", "W",
     "Vision OD 20/40, OS 20/30 recorded at an attended nursing visit. "
     "No clinician retinal examination and no fundus imaging today."),
    (True, "not_examined", "none", "W",
     "Patient attended; vision RE 6/9, LE 6/12. Only counselling and "
     "medication reconciliation completed. Retinae not examined."),
    (True, "not_examined", "none", "W",
     "Vision right 6/24, left 6/18 measured. Patient was seen but "
     "dilation was not done; neither retina assessed. Full visit rebooked."),
    (True, "no_view", "none", "C",
     "Patient attended and dilation was attempted. Dense bilateral "
     "cataracts prevent any retinal view; neither fundus assessable. "
     "Cataract-service referral made, with no retinal treatment booked."),
    (True, "no_view", "none", "C",
     "Patient attended. Dense vitreous haemorrhage in both eyes prevents "
     "visualisation of either retina despite dilation. B-scan shows attached "
     "retinae but does not provide a fundus view; specialist review arranged."),
    (True, "no_view", "none", "",
     "Attended and dilated. Severe bilateral corneal opacity prevents "
     "a view of either retina; fundus cannot be assessed."),
    (True, "no_view", "none", "",
     "Patient seen. Dense bilateral cataracts obstruct all fundus views "
     "despite dilation and attempted imaging. Retinae not assessable."),
    (True, "no_view", "none", "",
     "Attended referral visit. Bilateral dense media opacity prevents "
     "retinal assessment after dilation; arrange anterior-segment review."),
    (False, None, "none", "W",
     "Did not attend the screening-referral appointment. Template carried "
     "forward: 'dilate, fundus examination, OCT'; none was performed."),
    (False, None, "none", "W",
     "No-show for the hospital retinal appointment after screening referral. "
     "An appointment letter is not an attended examination; re-invite."),
    (False, None, "none", "W",
     "Patient absent from the booked screening-referral clinic. Telephone "
     "message left; no examination or treatment took place."),
    (False, None, "none", "W",
     "Did not attend. The referral requests laser assessment, but no "
     "patient review, treatment decision or treatment booking occurred."),
    (False, None, "none", "W",
     "Screening referral accepted; patient failed to attend the first "
     "hospital visit. Old visual acuities in the referral are not today's measurements."),
    (False, None, "none", "W",
     "Patient did not attend the rescheduled retinal clinic. Electronic "
     "check-in was entered in error and corrected; no clinical encounter."),
    (False, None, "none", "W",
     "Absent from screening-referral appointment. Administrative call "
     "answered by a relative only; patient not seen or examined."),
]

SCREEN_ARABIC = {2, 8, 14, 23, 35}
CLINIC_ARABIC = {3, 12, 22, 33, 47}
SCREEN_OCR = {6, 17, 28, 38}
CLINIC_OCR = {8, 19, 31, 44}
PRIOR_SCREEN = {1, 13, 21, 26, 29, 34, 37}

EYE_NAMES = [
    ("Right eye", "Left eye"),
    ("OD", "OS"),
    ("RE", "LE"),
    ("Right", "Left"),
    ("العين اليمنى", "العين اليسرى"),
    ("Right / OD", "Left / OS"),
]


def screening_label(key):
    """Apply the requested combination rule, including unilateral U."""
    eyes = [
        (key["r_right"], key["m_right"]),
        (key["r_left"], key["m_left"]),
    ]
    if any(r == "U" for r, _ in eyes):
        eyes = [
            (r, m) for r, m in eyes
            if r != "U" and (r in {"2", "3A", "3S"} or m == "1")
        ]
        if not eyes:
            return "U"
    rank = {"0": 0, "1": 1, "2": 2, "3S": 3, "3A": 4}
    r = max((r for r, _ in eyes), key=rank.__getitem__)
    m = "1" if any(m == "1" for _, m in eyes) else "0"
    label = "R" + r + "M" + m
    if key["urgent"] and r != "3A":
        label += "/urgent"
    return label


def clinic_label(key):
    if not key["attended"]:
        return "dna"
    if key["fundus"] == "no_view":
        return "not_assessable"
    if key["fundus"] == "not_examined":
        return "not_examined"
    assert key["fundus"] == "examined"
    suffix = key["outcome"]
    if suffix not in {"treated", "discharged"}:
        suffix = "followup"
    return "examined_" + suffix


def style_text(text, us):
    if not us:
        return text
    replacements = (
        ("haemorrhage", "hemorrhage"),
        ("oedema", "edema"),
        ("centre", "center"),
        ("programme", "program"),
        ("visualisation", "visualization"),
        ("counselling", "counseling"),
        ("naevus", "nevus"),
        ("artefact", "artifact"),
    )
    for old, new in replacements:
        text = text.replace(old, new)
    return text


def wrap(i, kind, body, arabic, ocr):
    """Six layouts, with cosmetic OCR noise outside diagnostic evidence."""
    layout = (i - 1) % 6 + 1
    service = (
        "Abu Dhabi primary-care diabetic eye screening centre"
        if kind == "screen"
        else "Abu Dhabi hospital eye centre / screening-referral clinic"
    )
    # Guarantees that each spelling tag has an actual visible counterpart.
    spelling_line = "Documentation centre: Abu Dhabi."
    if layout == 1:
        text = service + "\nReport\n" + body + "\n" + spelling_line
    elif layout == 2:
        text = service + "\nASSESSMENT:\n" + body + "\nADMIN: " + spelling_line
    elif layout == 3:
        text = "Clinical correspondence | " + service + "\nDear colleague,\n"
        text += body + "\nRegards, eye service. " + spelling_line
    elif layout == 4:
        text = service + "\n--- encounter extract ---\n"
        text += body + "\n--- end ---\n" + spelling_line
    elif layout == 5:
        text = "SERVICE: " + service + "\nCURRENT ENCOUNTER > "
        text += body + "\nRECORD > " + spelling_line
    else:
        text = service + "\n[Clinical entry]\n" + body
        text += "\n[Administration]\n" + spelling_line
    if arabic:
        text = "فحص العين لمرضى السكري | أبوظبي\n" + text
    if ocr:
        text = "Scann3d c0py | pag e 1 / 1 | clinic sta mp: [illegible]\n" + text
    return style_text(text, us=(i % 2 == 0))


def tags_for(i, aliases, arabic, ocr):
    tags = [
        "layout:L" + str((i - 1) % 6 + 1),
        "spelling:us" if i % 2 == 0 else "spelling:uk",
    ]
    tags.extend("hard:" + FAMILIES[a] for a in aliases)
    if arabic:
        tags.append("lang:arabic")
    if ocr:
        tags.append("noise:ocr")
    return tags


def eye_description(i, side, r, m):
    if (i, side) in EYE_OVERRIDES:
        return EYE_OVERRIDES[(i, side)]
    if r == "U":
        return "Images inadequate: this eye could not be graded; macular grade unavailable."
    return R_FINDINGS[r] + " " + M_FINDINGS[m]


def make_screen(rng):
    # Exactly 16/40 coded sites, chosen reproducibly without affecting labels.
    coded_ids = set(rng.sample(range(1, 41), 16))
    reports = []
    for i, (rr, rl, mr, ml, urgent, aliases, extra) in enumerate(SCREEN, 1):
        key = {
            "r_right": rr, "r_left": rl, "m_right": mr, "m_left": ml,
            "urgent": urgent,
        }
        key["label"] = screening_label(key)
        coded = i in coded_ids
        key["coded"] = key["label"] if coded else None
        arabic = i in SCREEN_ARABIC
        ocr = i in SCREEN_OCR
        # Arabic eye names occur only in records carrying the Arabic tag.
        names = EYE_NAMES[4] if arabic else EYE_NAMES[(i - 1) % 4]
        parts = []
        if coded:
            parts.append("Current screening grading summary; combined code: "
                         + key["label"] + ".")
        else:
            parts.append("Grader's report on today's screening images.")
        for side, (r, m) in enumerate(((rr, mr), (rl, ml))):
            grade = ""
            if coded:
                grade = 'R=' + r + '; M=' + (m if m else '""') + ". "
            parts.append(names[side] + ": " + grade + eye_description(i, side, r, m))
        parts.append(extra)
        if urgent:
            parts.append("Grader explicitly requests an urgent hospital eye referral.")
        body = "\n".join(parts)
        tags = tags_for(i, aliases, arabic, ocr)
        tags.append("format:coded" if coded else "format:free_text")
        if i in PRIOR_SCREEN:
            tags.append("context:prior_screen")
        reports.append({
            "id": f"hs{i:02d}",
            "text": wrap(i, "screen", body, arabic, ocr),
            "key": key,
            "tags": tags,
        })
    return reports


def make_clinic():
    reports = []
    for i, (attended, fundus, outcome, aliases, narrative) in enumerate(CLINIC, 1):
        key = {"attended": attended, "fundus": fundus, "outcome": outcome}
        key["label"] = clinic_label(key)
        arabic = i in CLINIC_ARABIC
        ocr = i in CLINIC_OCR
        body = (
            "Hospital appointment following referral from Abu Dhabi "
            "primary-care diabetic eye screening.\n" + narrative
        )
        if arabic:
            body += "\nالعين اليمنى / العين اليسرى: see current encounter above."
        reports.append({
            "id": f"hc{i:02d}",
            "text": wrap(i, "clinic", body, arabic, ocr),
            "key": key,
            "tags": tags_for(i, aliases, arabic, ocr),
        })
    return reports


def validate(screen, clinic):
    assert len(screen) == 40 and len(clinic) == 50
    assert len({r["text"] for r in screen + clinic}) == 90
    assert [r["id"] for r in screen] == [f"hs{i:02d}" for i in range(1, 41)]
    assert [r["id"] for r in clinic] == [f"hc{i:02d}" for i in range(1, 51)]
    expected_screen = {
        "R0M0": 12, "R1M0": 8, "R2M0": 3, "R2M1": 2,
        "R3AM0": 2, "R3AM1": 1, "R3SM0": 2, "R3SM1": 1,
        "R0M1": 1, "R1M1": 2, "U": 4,
        "R1M1/urgent": 1, "R2M0/urgent": 1,
    }
    expected_clinic = {
        "examined_treated": 10, "examined_followup": 10,
        "examined_discharged": 10, "not_examined": 8,
        "not_assessable": 5, "dna": 7,
    }
    assert Counter(r["key"]["label"] for r in screen) == expected_screen
    assert Counter(r["key"]["label"] for r in clinic) == expected_clinic
    for report in screen:
        k = report["key"]
        assert k["label"] == screening_label(k)
        assert k["coded"] is None or k["coded"] == k["label"]
        for side in ("right", "left"):
            r, m = k["r_" + side], k["m_" + side]
            assert r in {"0", "1", "2", "3A", "3S", "U"}
            assert m in ({""} if r == "U" else {"0", "1"})
    for report in clinic:
        k = report["key"]
        assert k["label"] == clinic_label(k)
        assert k["fundus"] in {"examined", "not_examined", "no_view", None}
        assert k["outcome"] in {"treated", "followup", "discharged", "none"}
        assert k["attended"] or (k["fundus"] is None and k["outcome"] == "none")
    for records, hard_count in ((screen, 25), (clinic, 33)):
        assert sum(any(t.startswith("hard:") for t in r["tags"])
                   for r in records) == hard_count
        assert len({t for r in records for t in r["tags"]
                    if t.startswith("layout:")}) == 6
    assert sum(r["key"]["coded"] is not None for r in screen) == 16
    assert sum("context:prior_screen" in r["tags"] for r in screen) == 7
    assert sum("lang:arabic" in r["tags"] for r in screen + clinic) == 10
    assert sum("noise:ocr" in r["tags"] for r in screen + clinic) == 8


def main():
    rng = random.Random(SEED)
    screen = make_screen(rng)
    clinic = make_clinic()
    validate(screen, clinic)
    destination = Path(__file__).resolve().parent
    for name, reports in (
        ("screen_reports.json", screen),
        ("clinic_notes.json", clinic),
    ):
        payload = {"source": SOURCE, "reports": reports}
        # Explicit bytes avoid platform-dependent newline conversion.
        data = json.dumps(payload, ensure_ascii=False, indent=2) + "\n"
        (destination / name).write_bytes(data.encode("utf-8"))


if __name__ == "__main__":
    main()
