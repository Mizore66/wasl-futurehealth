#!/usr/bin/env python3
"""gen_heldout8.py: deterministic, synthetic colonoscopy evaluation corpus."""
import hashlib
import json
import random
from collections import Counter
from pathlib import Path

SEED = 8081
SOURCE = (
    "Eighth independent author (GPT-6.1 Sol, OpenAI) - deterministic stdlib "
    "generator gen_heldout8.py, random seed 8081, 300 synthetic Abu Dhabi "
    "colonoscopy reports, 2026-10-29"
)

FAMILIES = (
    "amended_extent",
    "historical_results",
    "conditional_future_sampling",
    "preprinted_orders",
    "ileum_not_completion",
    "withdrawal_not_extent",
    "lavage_not_final_grade",
    "lesion_not_specimen",
    "biopsy_without_polypectomy",
    "scoped_tissue_negation",
    "jar_count_not_lesion_count",
    "pathology_workflow_status",
    "landmark_completion",
    "separate_upper_endoscopy",
    "referral_not_performance",
    "quality_word_scope",
    "external_specimen",
    "radiology_not_extent",
    "staged_treatment",
    "other_site_histology",
)

LAYOUTS = (
    "structured", "letter", "theatre_log",
    "narrative", "discharge", "worksheet",
)


def classify(fields):
    if fields["caecum"] == "no":
        return "not_closed", "incomplete_exam"
    if fields["prep"] == "inadequate":
        return "not_closed", "inadequate_prep"
    if fields["caecum"] == "not_stated" or fields["prep"] == "not_stated":
        return "needs_human", "missing_documentation"
    if fields["tissue_taken"] == "yes" and fields["histology"] == "no":
        return "not_closed", "awaiting_histology"
    return "closed", "criteria_met"


def reality_truth(caecum, prep, tissue, result):
    if not caecum or not prep:
        return "inadequate"
    if tissue and not result:
        return "awaiting_histology"
    return "adequate"


def family_note(family, pick, tissue, result, cword):
    """Contextual traps; these never replace the definitive procedure sections."""
    notes = {
        0: (
            "The signed correction at the end supersedes the insertion entry.",
            "Insertion was transcribed incorrectly; use the consultant's final correction.",
            "An extent-field amendment is appended after the original procedure record.",
        ),
        1: (
            "History only: a 2022 colonoscopy yielded a tubular adenoma; that result is not today's pathology.",
            "Previous examination: poor cleansing and a hyperplastic polyp in 2021. Neither entry describes this examination.",
            "Imported history records a prior incomplete colonoscopy with benign biopsy results. Current findings follow separately.",
        ),
        2: (
            "If watery diarrhoea recurs, consider repeat colonoscopy with mapping biopsies; this is a future contingency.",
            "Further sampling may be arranged after review if symptoms persist; the proposal is not a record of sampling today.",
            "Safety-net plan: repeat examination and biopsy only if the clinical course warrants it.",
        ),
        3: (
            "The preprinted order set contains 'polypectomy / biopsy'; those unchecked options are not performed-procedure entries.",
            "Booking form offered biopsy and snare treatment as possible interventions; the actual tissue record below controls.",
            "Consent includes possible biopsy and polypectomy. Permission alone does not document either intervention.",
        ),
        4: (
            "Terminal ileum not intubated; ileal entry was not required for this colonic examination.",
            "No ileoscopy was undertaken. The colonic extent is recorded independently.",
            "The ileal valve was not traversed; do not equate absent ileoscopy with an incomplete colon examination.",
        ),
        5: (
            "The detailed withdrawal description starts in the sigmoid; it is not the maximum insertion entry.",
            "Withdrawal observations were dictated from the rectosigmoid backwards in the note; insertion extent is a separate field.",
            "A withdrawal photograph is labelled 'descending colon'; that label does not identify the furthest point reached.",
        ),
        6: (
            "Irrigation and suction were used. Any preparation assessment below is the final assessment after cleaning.",
            "Residual fluid was suctioned and adherent material washed. The cleaning manoeuvre itself is not a prep grade.",
            "Water-jet lavage preceded the recorded final bowel-preparation assessment; washing alone does not establish adequacy.",
        ),
        7: (
            "An additional diminutive rectal prominence was left untouched; observing it did not generate a specimen.",
            "A separate tiny distal lesion was neither biopsied nor removed and has no pathology result.",
            "One small anal-verge lesion was inspected only, with no attempt at sampling that lesion.",
        ),
        8: (
            "The samples are non-polyp mucosal biopsies; there was no polypectomy.",
            "Only flat mucosa was sampled for microscopic assessment, not a polyp.",
            "Cold forceps were used for mucosal mapping, with no polyp excision.",
        ),
        9: (
            "No biopsy of the rectum was taken. This negative statement concerns the rectum only.",
            "The terminal ileum was not sampled; that statement does not describe sampling elsewhere.",
            "No tissue was taken from the anal canal; see the procedure-wide tissue record separately.",
        ),
        10: (
            "The specimen counter counts containers, not the number of lesions seen.",
            "Several fragments can share one pot; pot count is not a count of polyps.",
            "The specimen field is a jar inventory, not a lesion inventory.",
        ),
        11: (
            (
                "The old tracker entry 'pending' predates the current result reproduced below."
                if result else
                "Laboratory registration, if present, is an administrative event, not a diagnostic histology result."
            ),
            (
                "A result is now reproduced below; the earlier dispatch status is obsolete."
                if result else
                "A courier or accession entry does not establish that microscopic reporting is complete."
            ),
            (
                "Ignore the stale 'awaiting report' workflow flag: the current tissue diagnosis is included."
                if result else
                "Grossing and accession labels alone do not constitute a histological diagnosis."
            ),
        ),
        12: (
            "The insertion entry uses anatomical landmarks rather than a tick-box completion claim.",
            "Completion is to be read from the documented insertion landmarks, not the procedure title.",
            "The named landmarks in the extent entry determine whether the proximal endpoint was reached.",
        ),
        13: (
            "Separate same-day gastroscopy: gastric biopsies were taken. Those are not colonoscopy specimens.",
            "The upper-endoscopy note records duodenal sampling; that procedure has a separate specimen stream.",
            "Combined booking included OGD with antral biopsies. Only the colonoscopy tissue entry below applies here.",
        ),
        14: (
            "Referral request: 'biopsy suspected colitis'. A requested intervention is not proof it was performed.",
            "The referral asked for possible polypectomy; the actual intervention is documented separately.",
            "The booking indication includes 'for biopsy if indicated', not a completed biopsy event.",
        ),
        15: (
            "Sedation was adequate despite poor initial tolerance; these adjectives do not grade the bowel preparation.",
            "Recovery observations were good and initial tolerance was poor. Neither is a cleansing assessment.",
            "The nursing phrase 'adequate sedation' and the tolerance score refer to sedation, not bowel cleansing.",
        ),
        16: (
            "An outside specimen from a different attendance was handed to the courier today; it was not obtained at this colonoscopy.",
            "Reception logged an external biopsy pot from an earlier procedure. Its arrival is not sampling during this examination.",
            "The transfer sheet lists tissue collected at another unit last week; today's colonoscopy has its own tissue entry.",
        ),
        17: (
            f"Referral CT mentions a {cword} lesion; a radiological location does not prove endoscopic insertion to it.",
            f"The phrase '{cword} thickening' belongs to the preprocedure CT summary, not the insertion record.",
            f"Imaging had queried disease at the {cword}; scope extent must be read from this procedure's entry.",
        ),
        18: (
            "A possible staged mucosal resection was discussed for a later attendance; no future intervention is counted today.",
            "Advanced resection referral is a plan, not evidence that an EMR occurred during this examination.",
            "Any therapeutic return visit will be documented separately; today's intervention list is the operative record.",
        ),
        19: (
            "A copied pathology line says 'benign skin tag' from a dermatology specimen; it is not this colonoscopy's result.",
            "The chart also contains benign breast-core histology from another episode; that result is unrelated to colonic tissue.",
            "An old gallbladder pathology result is visible in the shared chart. It does not close the colonoscopy tissue episode.",
        ),
    }
    return pick(notes[family])


def make_report(i, rng):
    pick = rng.choice
    uid = f"v{i + 1:03d}"
    family = i % 20
    uk = i % 2 == 0
    cword = "caecum" if uk else "cecum"
    procedure_word = "anaesthesia" if uk else "anesthesia"

    # Private reality. It is deliberately not exported as another answer field.
    if i < 120:
        caecum, prep = True, True
        tissue = i % 3 != 0
        result = tissue
    elif i < 210:
        caecum, prep, tissue, result = True, True, True, False
    else:
        caecum = i % 3 != 0
        prep = i % 3 == 0
        tissue = i % 2 == 0
        result = tissue and i % 4 == 0

    # This family always represents actual non-polyp sampling.
    if family == 8:
        tissue = True
        result = i < 120 or (i >= 210 and i % 4 == 0)

    extent_missing = i % 10 in (0, 1) and family != 0
    prep_missing = i % 6 == 1
    bbps = not prep_missing and i % 5 in (0, 1)
    biopsy_only = tissue and (i % 3 == 1 or family == 8)

    extent_key = "not_stated" if extent_missing else ("yes" if caecum else "no")
    prep_key = "not_stated" if prep_missing else (
        "adequate" if prep else "inadequate"
    )
    yn_tissue = "yes" if tissue else "no"
    fields = {
        "caecum": extent_key,
        "prep": prep_key,
        # Historical name: this means tissue acquisition, not visible polyps.
        "polyps": yn_tissue,
        "tissue_taken": yn_tissue,
        "histology": "yes" if tissue and result else "no",
    }

    def extent_sentence(reached):
        if reached:
            if family == 12:
                return pick((
                    "Appendiceal orifice and ileocaecal valve both identified; proximal pole photographed."
                    if uk else
                    "Appendiceal orifice and ileocecal valve both identified; proximal pole photographed.",
                    "The appendiceal opening and both lips of the ileocaecal valve were documented at the proximal endpoint."
                    if uk else
                    "The appendiceal opening and both lips of the ileocecal valve were documented at the proximal endpoint.",
                    "Insertion ended at the blind proximal pole with the appendiceal orifice and ileocaecal valve in view."
                    if uk else
                    "Insertion ended at the blind proximal pole with the appendiceal orifice and ileocecal valve in view.",
                ))
            return pick((
                f"Scope advanced to the {cword}; appendiceal orifice and valve photographed.",
                f"{cword.capitalize()} reached and identified by both standard landmarks.",
                f"Complete insertion to the {cword}, confirmed by the valve and appendiceal opening.",
                f"The examination reached the {cword}; landmark images were saved.",
            ))
        return pick((
            f"Maximum insertion: hepatic flexure. The {cword} was not reached because of fixed looping.",
            f"Stopped in the transverse colon owing to intolerance; no {cword} intubation.",
            f"Insertion abandoned at the splenic flexure; incomplete examination, {cword} not reached.",
            f"Only as far as the descending colon examined; the {cword} was not reached.",
        ))

    correction = ""
    if family == 0:
        extent_text = "Original insertion entry: " + extent_sentence(not caecum)
        correction = (
            "SIGNED FINAL ADDENDUM — consultant correction of insertion field: "
            + extent_sentence(caecum)
            + " This replaces the original insertion entry; all other entries stand."
        )
    elif extent_missing:
        extent_text = pick((
            "Insertion endpoint: [field left blank].",
            "Extent field was not completed in the transferred procedure record.",
            "Maximum insertion: not recorded.",
        ))
    else:
        extent_text = extent_sentence(caecum)

    if prep_missing:
        prep_text = pick((
            "Preparation: split-dose PEG was prescribed; no cleansing grade recorded.",
            "Patient reports finishing the bowel preparation. Endoscopic quality assessment is blank.",
            "Bowel-prep agent: PEG solution. Quality of cleansing: not documented.",
            "Preparation instructions were followed by patient report; no procedural prep rating supplied.",
        ))
        prep_tag = "prep:ungraded"
    elif bbps:
        scores = pick(
            ((2, 2, 2), (3, 2, 3), (2, 3, 2), (3, 3, 3))
            if prep else
            ((1, 2, 3), (3, 1, 2), (2, 3, 1))
        )
        r, t, l = scores
        total = sum(scores)
        prep_text = pick((
            f"Boston Bowel Preparation Scale after washing: right {r}, transverse {t}, left {l}; total {total}/9.",
            f"BBPS (R/T/L) = {r}/{t}/{l}; aggregate {total}. Final post-lavage scores.",
            f"Final Boston score: R={r}; T={t}; L={l} (sum {total} of 9).",
            f"Post-cleaning BBPS: left {l}, right {r}, transverse {t}; total={total}.",
        ))
        prep_tag = "prep:bbps"
    else:
        if prep:
            grade = pick(("excellent", "good", "fair", "adequate"))
        else:
            grade = pick(("poor", "inadequate"))
        prep_text = pick((
            f"Final bowel preparation: {grade}.",
            f"Cleansing after irrigation was graded {grade}.",
            f"Endoscopist's bowel-preparation assessment: {grade}.",
            f"The final quality of bowel cleansing was {grade}.",
        ))
        prep_tag = "prep:verbal"

    if not tissue:
        tissue_text = pick((
            "No biopsy or polypectomy was performed; no tissue obtained.",
            "Inspection only. Neither mucosal sampling nor polyp removal took place.",
            "Specimens: none. No biopsies taken and no polyps resected.",
            "There was no tissue acquisition: no biopsy, snare excision or other removal.",
        ))
        tissue_text += " " + pick((
            "No current colonoscopy histology is expected.",
            "There is no tissue result for this examination.",
            "No colonoscopy specimen was submitted to pathology.",
        ))
        tissue_tag = "tissue:none"
    else:
        if biopsy_only:
            tissue_text = pick((
                "Non-polyp biopsies taken from ascending and descending colon to assess microscopic colitis; two labelled pots.",
                "Flat ascending-colon mucosa sampled with cold forceps; no polyp removed.",
                "Random right- and left-colon mucosal biopsies obtained for diarrhoea assessment, without polypectomy.",
                "Cold mucosal biopsies from the descending colon were submitted; this was not polyp excision.",
            ))
            diagnosis = pick((
                "Colonic mucosa with no active inflammation or dysplasia.",
                "Mild nonspecific chronic mucosal inflammation; no dysplasia.",
                "No diagnostic features of microscopic colitis in the sampled mucosa.",
            ))
            tissue_tag = "tissue:biopsy_only"
        else:
            tissue_text = pick((
                "A 4 mm sigmoid polyp was removed by cold snare and retrieved for histology.",
                "Cold-snare excision of a 5 mm descending-colon polyp; specimen recovered and sent.",
                "One diminutive sigmoid polyp was excised with cold forceps and submitted to pathology.",
                "A small rectosigmoid polyp was resected and retrieved; tissue sent in one pot.",
            ))
            diagnosis = pick((
                "Hyperplastic polyp, negative for dysplasia.",
                "Tubular adenoma with low-grade dysplasia; no invasive carcinoma.",
                "Sessile serrated lesion without dysplasia.",
            ))
            tissue_tag = "tissue:polypectomy"
        if result:
            tissue_text += " " + pick((
                "Current-procedure histopathology report, signed and reproduced here: ",
                "Result for the tissue obtained at this examination: ",
                "Matched pathology for this colonoscopy specimen is now available: ",
            )) + diagnosis
        else:
            tissue_text += " " + pick((
                "Histology from this procedure is pending; no result is available.",
                "Current specimens have been sent; the diagnostic pathology report has not returned.",
                "Await the histological diagnosis for today's tissue; no result is reproduced or available yet.",
            ))

    note = family_note(family, pick, tissue, result, cword)
    indication = pick((
        "positive faecal immunochemical test" if uk else "positive fecal immunochemical test",
        "intermittent rectal bleeding",
        "persistent change in bowel habit",
        "surveillance after previous adenoma",
        "iron-deficiency anaemia" if uk else "iron-deficiency anemia",
        "chronic non-bloody diarrhoea" if uk else "chronic non-bloody diarrhea",
        "assessment of unexplained lower abdominal symptoms",
        "family history of colorectal cancer",
    ))
    unit = pick((
        "Al Safa Day Surgery Endoscopy Unit, Abu Dhabi",
        "Palm Crescent Hospital Endoscopy, Abu Dhabi",
        "Capital Harbour Day Surgery, Abu Dhabi",
        "Oasis Gate Hospital Digestive Unit, Abu Dhabi",
    ))
    age = 24 + rng.randrange(60)
    header = (
        f"{unit}\nSynthetic encounter {uid.upper()} | age {age} | "
        f"procedure date 2026-10-{1 + i % 28:02d}"
    )
    technique = pick((
        f"Consent confirmed; IV sedation under the {procedure_word} team. Monitoring maintained.",
        f"Time-out completed. Monitored {procedure_word} care; left lateral position.",
        f"Identity and consent checked. Sedation administered with {procedure_word} support.",
    ))
    observation = pick((
        "No active bleeding was seen in the inspected segments.",
        "No immediate procedural complication was recorded.",
        "Patient transferred to recovery in stable condition.",
    ))
    arabic = i % 8 == 0
    scan = i % 9 == 0
    tail = observation
    if arabic:
        tail += "\n" + pick((
            "المريض مستقر بعد الإجراء. تم شرح تعليمات الخروج للمرافق.",
            "تعليمات الخروج: الراحة اليوم وعدم القيادة بعد المهدئ.",
            "تمت مراجعة تعليمات السلامة مع المريض والمرافق في أبوظبي.",
        ))
    if scan:
        header = pick((
            "SCANN ED COPY | Page l / l | barcode: ||| ||\n",
            "OCR transfer: patlent identlfier checked | footer reproduced\n",
            "[scan margin: speckle . . .]  Endoscopy / Endosc0py\n",
        )) + header

    layout_index = (i % 6 + i // 30) % 6
    layout = LAYOUTS[layout_index]
    if layout == "structured":
        text = (
            f"{header}\nCOLONOSCOPY RECORD\nIndication: {indication}\n"
            f"{technique}\nContext: {note}\nExtent: {extent_text}\n"
            f"Preparation: {prep_text}\nTissue / results: {tissue_text}\n{tail}"
        )
    elif layout == "letter":
        text = (
            f"{header}\nDear referring colleague,\n"
            f"Colonoscopy was undertaken for {indication}. {technique}\n"
            f"Relevant context: {note}\n{prep_text}\n{extent_text}\n"
            f"{tissue_text}\n{tail}\nSigned: duty endoscopist."
        )
    elif layout == "theatre_log":
        text = (
            f"{header}\nDAY-SURGERY PROCEDURE LOG\n"
            f"Reason | {indication}\nSafety | {technique}\n"
            f"Background/qualifiers | {note}\nBowel cleansing | {prep_text}\n"
            f"Insertion entry | {extent_text}\nIntervention and laboratory | "
            f"{tissue_text}\nRecovery | {tail}"
        )
    elif layout == "narrative":
        text = (
            f"{header}\nConsultant procedure narrative\n"
            f"Indication was {indication}. {note}\n{technique} "
            f"{extent_text} {prep_text}\n{tissue_text}\n{tail}"
        )
    elif layout == "discharge":
        text = (
            f"{header}\nENDOSCOPY DISCHARGE SUMMARY\n"
            f"Procedure: colonoscopy for {indication}.\n{technique}\n"
            f"Procedure outcome:\n  {extent_text}\n  {prep_text}\n"
            f"Sampling and result status:\n  {tissue_text}\n"
            f"Clinical context and plans:\n  {note}\n{tail}"
        )
    else:
        text = (
            f"{header}\nENDOSCOPY WORKSHEET — signed record\n"
            f"A. Indication: {indication}\nB. Technique: {technique}\n"
            f"C. Insertion: {extent_text}\nD. Cleansing: {prep_text}\n"
            f"E. Tissue/results: {tissue_text}\nF. Context: {note}\n"
            f"G. Recovery: {tail}"
        )
    if correction:
        text += "\n\n" + correction
    text += "\n"

    tags = [
        f"fmt:{layout}",
        "spelling:uk" if uk else "spelling:us",
        prep_tag,
        tissue_tag,
        f"hard:{FAMILIES[family]}",
    ]
    if extent_missing:
        tags.append("doc:extent_unstated")
    if arabic:
        tags.append("lang:arabic")
    if scan:
        tags.append("scan:ocr_noise")
    if bbps and not prep:
        tags.append("hard:bbps_segment_minimum")
    if (extent_missing and not caecum) or (prep_missing and not prep):
        tags.append("hidden:undocumented_failure")

    state, reason = classify(fields)
    report = {
        "uid": uid,
        "sha": hashlib.sha256(text.encode("utf-8")).hexdigest(),
        "text": text,
        "truth": reality_truth(caecum, prep, tissue, result),
        "key_fields": fields,
        "key_state": state,
        "key_reason": reason,
        "tags": tags,
    }
    assert fields["polyps"] == fields["tissue_taken"]
    assert not result or tissue
    assert report["truth"] == (
        "inadequate" if i >= 210 else
        "awaiting_histology" if i >= 120 else "adequate"
    )
    return report


def build():
    rng = random.Random(SEED)
    reports = [make_report(i, rng) for i in range(300)]
    assert len({r["text"] for r in reports}) == 300
    assert [r["uid"] for r in reports] == [
        f"v{i:03d}" for i in range(1, 301)
    ]
    tags = Counter(t for r in reports for t in r["tags"])
    expected_tags = {
        **{f"fmt:{name}": 50 for name in LAYOUTS},
        "spelling:uk": 150,
        "spelling:us": 150,
        "prep:ungraded": 50,
        "prep:bbps": 100,
        "prep:verbal": 150,
        "tissue:none": 83,
        "tissue:biopsy_only": 95,
        "tissue:polypectomy": 122,
        **{f"hard:{name}": 15 for name in FAMILIES},
        "hard:bbps_segment_minimum": 18,
        "doc:extent_unstated": 45,
        "lang:arabic": 38,
        "scan:ocr_noise": 34,
        "hidden:undocumented_failure": 20,
    }
    assert dict(tags) == expected_tags, (dict(tags), expected_tags)
    assert Counter(r["truth"] for r in reports) == {
        "adequate": 120, "awaiting_histology": 90, "inadequate": 90
    }
    assert Counter(r["key_state"] for r in reports) == {
        "closed": 86, "needs_human": 79, "not_closed": 135
    }
    assert Counter(r["key_reason"] for r in reports) == {
        "criteria_met": 86,
        "missing_documentation": 79,
        "incomplete_exam": 25,
        "inadequate_prep": 45,
        "awaiting_histology": 65,
    }
    for report in reports:
        assert classify(report["key_fields"]) == (
            report["key_state"], report["key_reason"]
        )
        assert report["sha"] == hashlib.sha256(
            report["text"].encode("utf-8")
        ).hexdigest()
    return {"source": SOURCE, "reports": reports}


def main():
    payload = json.dumps(
        build(), ensure_ascii=False, sort_keys=True, indent=2
    ) + "\n"
    destination = Path(__file__).resolve().parent / "reports_heldout8.json"
    # Binary writing fixes line endings as well as encoding on every platform.
    destination.write_bytes(payload.encode("utf-8"))


if __name__ == "__main__":
    main()
