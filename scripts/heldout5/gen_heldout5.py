#!/usr/bin/env python3
"""Generate a deterministic, wholly synthetic colonoscopy evaluation corpus.

Author: independent author (GPT-6.1 Sol, OpenAI)
Only Python's standard library is used. No inputs, network, or clock are read.
The hidden reality in each Case is deliberately not exported into the reports.
"""

import hashlib
import json
import random
from collections import Counter
from dataclasses import dataclass
from pathlib import Path


SEED = 5051
DATASET_DATE = "2026-10-29"  # Fixed publication date; never read from a clock.
AUTHOR = "independent author (GPT-6.1 Sol, OpenAI)"


HARD_FAMILIES = {
    "normal_mucosa_sampled": "Normal visual mucosa still has random mucosal biopsies.",
    "ileum_optional": "Failed terminal ileal intubation does not undo documented caecal landmarks.",
    "lesion_left_in_situ": "A seen polyp is deliberately left untouched, so lesion presence is not tissue acquisition.",
    "future_sampling": "A future biopsy or resection recommendation is not sampling during this procedure.",
    "post_lavage_grade": "The final post-wash quality assessment supersedes the dirty insertion view.",
    "outside_current_histology": "A current-procedure pathology result imported from another laboratory counts when linked by accession.",
    "regimen_not_quality": "Completing a preparation regimen says nothing about its observed cleansing adequacy.",
    "metrics_not_extent": "Withdrawal time and inspection photographs alone do not establish an insertion endpoint.",
    "superseding_correction": "An explicitly superseding correction controls over an earlier signed or provisional statement.",
    "histology_addendum": "A later tissue diagnosis closes an earlier pending histology entry for the same accession.",
    "resection_without_retrieval": "A polypectomy counts as tissue taken even when the specimen is lost and cannot yield histology.",
    "prior_episode": "Prior completion, preparation grades, and tissue diagnoses do not describe this procedure.",
    "receipt_not_result": "Laboratory receipt and gross processing are not a microscopic histology result.",
    "other_site_histology": "Same-day gastric histology does not supply a result for the colon biopsies.",
    "fragments_not_procedures": "Several tissue fragments and containers still belong to one current sampling episode.",
    "segment_over_total": "A Boston total of seven fails adequacy when one segment is scored one.",
    "signed_result_unreleased": "A signed microscopic diagnosis counts even while the patient portal release is pending.",
    "failure_priority": "Documented incomplete extent takes priority over poor preparation and pending histology.",
    "extent_over_distance": "A long insertion distance and looping do not substitute for reaching the caecum.",
}


@dataclass(frozen=True)
class Case:
    name: str
    # Independent, latent procedure facts: reached, adequate, tissue, result back.
    reality: tuple
    # Independently specified documentary facts: extent, prep, tissue, result.
    fields: tuple
    expected_key: tuple
    prep_style: str
    tissue_style: str
    hard: tuple = ()
    extra: tuple = ()


CASES = (
    Case("routine", (True, True, False, False), ("yes", "adequate", "no", "no"), ("closed", "criteria_met"), "verbal", "none"),
    Case("normal_biopsy_result", (True, True, True, True), ("yes", "adequate", "yes", "yes"), ("closed", "criteria_met"), "boston", "biopsy", ("normal_mucosa_sampled",)),
    Case("ileum_failed", (True, True, False, False), ("yes", "adequate", "no", "no"), ("closed", "criteria_met"), "verbal", "none", ("ileum_optional",)),
    Case("polyp_untouched", (True, True, False, False), ("yes", "adequate", "no", "no"), ("closed", "criteria_met"), "verbal", "none", ("lesion_left_in_situ",)),
    Case("future_resection", (True, True, False, False), ("yes", "adequate", "no", "no"), ("closed", "criteria_met"), "verbal", "none", ("future_sampling", "lesion_left_in_situ")),
    Case("washed_clean", (True, True, False, False), ("yes", "adequate", "no", "no"), ("closed", "criteria_met"), "verbal", "none", ("post_lavage_grade",)),
    Case("external_result", (True, True, True, True), ("yes", "adequate", "yes", "yes"), ("closed", "criteria_met"), "boston", "polypectomy", ("outside_current_histology",)),
    Case("prep_missing_fine", (True, True, False, False), ("yes", "not_stated", "no", "no"), ("needs_human", "missing_documentation"), "ungraded", "none", ("regimen_not_quality",)),
    Case("extent_missing_fine", (True, True, False, False), ("not_stated", "adequate", "no", "no"), ("needs_human", "missing_documentation"), "verbal", "none", ("metrics_not_extent",)),
    Case("extent_corrected_complete", (True, True, False, False), ("yes", "adequate", "no", "no"), ("closed", "criteria_met"), "verbal", "none", ("superseding_correction",), ("doc:later_correction",)),
    Case("histology_added", (True, True, True, True), ("yes", "adequate", "yes", "yes"), ("closed", "criteria_met"), "boston", "polypectomy", ("histology_addendum",), ("doc:later_correction",)),
    Case("both_missing_fine", (True, True, False, False), ("not_stated", "not_stated", "no", "no"), ("needs_human", "missing_documentation"), "ungraded", "none", ("regimen_not_quality", "metrics_not_extent")),
    Case("normal_biopsy_pending", (True, True, True, False), ("yes", "adequate", "yes", "no"), ("not_closed", "awaiting_histology"), "verbal", "biopsy", ("normal_mucosa_sampled",)),
    Case("lost_polyp", (True, True, True, False), ("yes", "adequate", "yes", "no"), ("not_closed", "awaiting_histology"), "verbal", "polypectomy", ("resection_without_retrieval",)),
    Case("old_histology", (True, True, True, False), ("yes", "adequate", "yes", "no"), ("not_closed", "awaiting_histology"), "boston", "polypectomy", ("prior_episode",), ("doc:prior_procedure",)),
    Case("lab_received", (True, True, True, False), ("yes", "adequate", "yes", "no"), ("not_closed", "awaiting_histology"), "verbal", "polypectomy", ("receipt_not_result",)),
    Case("gastric_result", (True, True, True, False), ("yes", "adequate", "yes", "no"), ("not_closed", "awaiting_histology"), "verbal", "biopsy", ("other_site_histology",)),
    Case("biopsy_correction", (True, True, True, False), ("yes", "adequate", "yes", "no"), ("not_closed", "awaiting_histology"), "verbal", "biopsy", ("superseding_correction", "normal_mucosa_sampled"), ("doc:later_correction",)),
    Case("fragments", (True, True, True, False), ("yes", "adequate", "yes", "no"), ("not_closed", "awaiting_histology"), "boston", "polypectomy", ("fragments_not_procedures",)),
    Case("prep_missing_pending", (True, True, True, False), ("yes", "not_stated", "yes", "no"), ("needs_human", "missing_documentation"), "ungraded", "biopsy", ("regimen_not_quality",)),
    Case("extent_missing_pending", (True, True, True, False), ("not_stated", "adequate", "yes", "no"), ("needs_human", "missing_documentation"), "verbal", "polypectomy", ("metrics_not_extent",)),
    Case("looping_incomplete", (False, True, False, False), ("no", "adequate", "no", "no"), ("not_closed", "incomplete_exam"), "verbal", "none", ("extent_over_distance",)),
    Case("boston_bad_segment", (True, False, False, False), ("yes", "inadequate", "no", "no"), ("not_closed", "inadequate_prep"), "boston", "none", ("segment_over_total",)),
    Case("prep_correction_poor", (True, False, False, False), ("yes", "inadequate", "no", "no"), ("not_closed", "inadequate_prep"), "verbal", "none", ("superseding_correction",), ("doc:later_correction",)),
    Case("incomplete_prep_missing", (False, True, False, False), ("no", "not_stated", "no", "no"), ("not_closed", "incomplete_exam"), "ungraded", "none", ("regimen_not_quality",)),
    Case("hidden_bad_prep", (True, False, False, False), ("yes", "not_stated", "no", "no"), ("needs_human", "missing_documentation"), "ungraded", "none", ("regimen_not_quality",), ("hidden:undocumented_failure",)),
    Case("hidden_incomplete", (False, True, False, False), ("not_stated", "adequate", "no", "no"), ("needs_human", "missing_documentation"), "verbal", "none", ("metrics_not_extent",), ("hidden:undocumented_failure",)),
    Case("several_failures", (False, False, True, False), ("no", "inadequate", "yes", "no"), ("not_closed", "incomplete_exam"), "verbal", "biopsy", ("failure_priority",)),
    Case("result_but_poor_prep", (True, False, True, True), ("yes", "inadequate", "yes", "yes"), ("not_closed", "inadequate_prep"), "verbal", "polypectomy", ("signed_result_unreleased",)),
    Case("extent_corrected_incomplete", (False, True, False, False), ("no", "adequate", "no", "no"), ("not_closed", "incomplete_exam"), "verbal", "none", ("superseding_correction",), ("doc:later_correction",)),
)


LAYOUTS = ("narrative", "structured", "letter", "day_surgery", "dictated", "checklist", "sectioned", "transfer_summary")
UNITS = (
    "Al Mina Day Surgery Centre, Abu Dhabi",
    "Al Reem Hospital Endoscopy Unit, Abu Dhabi",
    "Khalifa City Ambulatory Endoscopy, Abu Dhabi",
    "Madinat Zayed Hospital Endoscopy Service, Abu Dhabi",
    "Al Bateen Day Surgery Unit, Abu Dhabi",
    "Bani Yas Hospital Digestive Unit, Abu Dhabi",
)


def latent_truth(reality):
    reached, clean, tissue, result = reality
    if not reached or not clean:
        return "inadequate"
    if tissue and not result:
        return "awaiting_histology"
    return "adequate"


def documentary_key(fields):
    extent, prep, tissue, histology = fields
    if extent == "no":
        return "not_closed", "incomplete_exam"
    if prep == "inadequate":
        return "not_closed", "inadequate_prep"
    if "not_stated" in (extent, prep):
        return "needs_human", "missing_documentation"
    if tissue == "yes" and histology == "no":
        return "not_closed", "awaiting_histology"
    return "closed", "criteria_met"


def boston_text(rng, inadequate):
    scores = [3, 3, 1] if inadequate else rng.choice(([2, 2, 2], [2, 3, 2], [3, 2, 3], [3, 3, 3]))
    r, t, l = scores
    total = sum(scores)
    formats = (
        f"BBPS after washing: right {r}, transverse {t}, left {l}; total {total}/9.",
        f"Boston Bowel Preparation Scale (final): R/T/L = {r}/{t}/{l} (sum {total}).",
        f"Final Boston scores: right colon={r}; transverse colon={t}; left colon={l}. Overall: {total} of 9.",
        f"BBPS, post-lavage: total {total}; segment order RIGHT | TRANSVERSE | LEFT: {r} | {t} | {l}.",
        f"Boston preparation assessment, after suction: L={l}, T={t}, R={r}; aggregate {total}/9.",
        f"Bowel cleansing (Boston): {r}-{t}-{l}, listed proximal to distal; total = {total}.",
    )
    return rng.choice(formats)


def clinical_blocks(case, rng, us, accession):
    cecum = "cecum" if us else "caecum"
    cecal = "cecal" if us else "caecal"
    extent, prep, tissue, histology = case.fields
    size = rng.choice((3, 4, 5, 6, 7, 8))
    site = rng.choice(("sigmoid", "descending", "transverse", "ascending"))
    complete = rng.choice((
        f"The {cecum} was reached; the appendiceal orifice and ileocecal valve were photographed." if us else f"The {cecum} was reached; the appendiceal orifice and ileocaecal valve were photographed.",
        f"Extent: {cecum}, identified by the appendiceal orifice and {cecal} pole with valve view.",
        f"Insertion completed to the {cecum}. Both defining landmarks were identified and images saved.",
    ))
    if extent == "not_stated":
        extent_text = rng.choice((
            "Withdrawal inspection time 9 minutes; segment photographs filed in the image archive.",
            "Endoscope passed under direct vision; mucosa inspected during withdrawal for 8 minutes.",
            "Procedure images stored. Withdrawal inspection lasted 10 minutes excluding interventions.",
        ))
    elif extent == "no":
        extent_text = f"Stopped at the splenic flexure because of fixed angulation; the {cecum} was not reached."
    else:
        extent_text = complete
    if case.name == "ileum_failed":
        extent_text += " The terminal ileum could not be intubated despite two gentle attempts."
    if case.name == "looping_incomplete":
        extent_text = f"Insertion length 140 cm with marked sigmoid looping. Maximum extent was transverse colon; {cecum} not reached. Procedure stopped for discomfort."
    if case.name == "several_failures":
        extent_text = f"Digital rectal examination normal. Scope stopped in sigmoid colon at 35 cm; {cecum} not reached."
    if case.name == "extent_corrected_complete":
        extent_text = "Original signed extent entry: hepatic flexure only; examination incomplete."
    if case.name == "extent_corrected_incomplete":
        extent_text = f"Original signed extent entry: {cecum} reached."

    if case.prep_style == "boston":
        prep_text = boston_text(rng, prep == "inadequate")
    elif case.prep_style == "ungraded":
        prep_text = rng.choice((
            "Preparation regimen: split-dose polyethylene glycol; patient reports completing both doses.",
            "Bowel preparation medication: PEG solution, split dosing. Last dose taken before admission.",
            "Preparation history: prescribed two-dose PEG regimen completed; no vomiting reported.",
        ))
    else:
        grade = rng.choice(("poor", "inadequate")) if prep == "inadequate" else rng.choice(("excellent", "good", "fair", "adequate"))
        prep_text = f"Final bowel preparation grade: {grade}."
    if case.name == "polyp_untouched" or case.name == "lost_polyp":
        prep_text = "Final bowel preparation grade: fair."
    if case.name == "washed_clean":
        prep_text = "On insertion, preparation was poor with liquid stool. After extensive lavage and suction, the final preparation grade was good throughout."
    if case.name == "prep_correction_poor":
        prep_text = "Original signed preparation entry: good."

    negative = rng.choice((
        "No biopsy or polypectomy was performed; no tissue was taken.",
        "Specimens: none. Neither biopsies nor polyp removal were undertaken.",
        "No tissue obtained: no mucosal biopsies and no polypectomy during this procedure.",
    ))
    if case.tissue_style == "none":
        finding_text = "No focal mucosal lesion in the examined bowel. " + negative
    elif case.tissue_style == "biopsy":
        finding_text = f"No polyps seen. Random right and left colon mucosal biopsies obtained for diarrhoea; containers A and B, accession {accession}."
        if case.name in ("normal_biopsy_result", "normal_biopsy_pending"):
            finding_text = "Mucosa appeared normal throughout the colon. " + finding_text
        if us:
            finding_text = finding_text.replace("diarrhoea", "diarrhea")
    else:
        finding_text = f"A {size} mm sessile {site} polyp was removed by cold snare and retrieved for histology, accession {accession}."
    if case.name in ("polyp_untouched", "future_resection"):
        finding_text = f"A 22 mm flat sigmoid polyp was seen and left in situ. " + negative
    if case.name == "future_resection":
        finding_text += " Plan: repeat therapeutic colonoscopy for resection, with biopsies only if resection is deferred."
    if case.name == "lost_polyp":
        finding_text = f"A {size} mm {site} polyp was excised completely by cold snare. The detached specimen was lost in suction and not retrieved. No material could be sent to pathology."
    if case.name == "biopsy_correction":
        finding_text = "Normal visual mucosa, no polyps. Original signed specimen entry: none; no biopsies taken."
    if case.name == "several_failures":
        finding_text = f"Erythematous sigmoid mucosa biopsied in the examined segment, accession {accession}. No polypectomy."
    if case.name == "fragments":
        finding_text = f"One {size + 10} mm sigmoid polyp removed piecemeal with cold snare. Three retrieved fragments divided between containers A and B; both belong to this procedure, accession {accession}."

    if tissue == "no":
        path_text = "No current tissue histology requested."
    elif histology == "yes" and case.tissue_style == "biopsy":
        path_text = f"Current colon pathology, {accession}, final signed result: colonic mucosa without active inflammation or microscopic colitis."
    elif histology == "yes":
        path_text = f"Current polyp histology, {accession}, final signed result: tubular adenoma with low-grade dysplasia."
    else:
        path_text = f"Histology for current colon specimens ({accession}) is pending; no result available."
    if case.name == "lost_polyp":
        path_text = "No histology result: resected polyp not retrieved and no specimen submitted."
    if case.name == "external_result":
        path_text = f"External laboratory attachment, explicitly matched to this colonoscopy and accession {accession}: FINAL microscopic diagnosis, tubular adenoma with low-grade dysplasia. Result imported into this record."
    if case.name == "histology_added":
        path_text = f"Initial pathology status for {accession}: pending."
    if case.name == "old_histology":
        path_text = f"Historical surveillance record (2024 colonoscopy): sigmoid tubular adenoma, low-grade dysplasia. This is not the current specimen. Current accession {accession}: histology awaited."
    if case.name == "lab_received":
        path_text = f"Laboratory tracking {accession}: RECEIVED; gross description 'one tan fragment'. Microscopy not yet performed; final diagnosis pending."
    if case.name == "gastric_result":
        path_text = f"Same-day upper endoscopy, separate gastric accession G-{accession}: chronic inactive gastritis on final biopsy result. Colon accession {accession} remains pending; no colon diagnosis is available."
    if case.name == "result_but_poor_prep":
        path_text += " Patient portal release is pending; the diagnostic report itself has already been signed."

    addendum = ""
    if case.name == "extent_corrected_complete":
        addendum = f"Later signed correction after image review: the hepatic-flexure entry was erroneous. The {cecum} WAS reached, with appendiceal orifice and valve documented. This replaces the earlier extent entry."
    elif case.name == "extent_corrected_incomplete":
        addendum = f"Later signed correction after image review: the image labelled '{cecum}' shows hepatic flexure. Actual endpoint was hepatic flexure; the {cecum} was NOT reached. This replaces the original extent entry."
    elif case.name == "prep_correction_poor":
        addendum = "Later signed quality correction: 'good' was a selection error. Final preparation was poor, with adherent stool obscuring mucosa despite washing. This replaces the original preparation entry."
    elif case.name == "biopsy_correction":
        addendum = f"Later signed specimen correction: the 'none/no biopsies' entry is withdrawn. Right and left colon mucosal biopsies WERE taken during this procedure, accession {accession}; histology remains pending."
    elif case.name == "histology_added":
        addendum = f"Later pathology addendum, {accession}, final signed report: hyperplastic polyp, no dysplasia. This result supersedes the earlier pending status for this procedure."
    return extent_text, prep_text, finding_text, path_text, addendum


def render(layout, meta, blocks, history, arabic):
    extent, prep, findings, path, addendum = blocks
    unit, visit, patient, indication, sedation, spelling = meta
    consent = "Consent confirmed; recovery observations stable."
    # A spelling cue appears even when the body has no dialect-specific word.
    admin = f"{spelling} endoscopy record | Visit {visit} | {patient}"
    if layout == "narrative":
        text = f"{unit}\nCOLONOSCOPY REPORT\n{admin}\nIndication: {indication}. {sedation}\n{history}\n{extent} {prep}\n{findings}\n{path}\n{consent}"
    elif layout == "structured":
        text = f"{unit}\nProcedure: colonoscopy\n{admin}\nINDICATION / {indication}\nSEDATION / {sedation}\nHISTORY / {history or 'No relevant prior endoscopy supplied.'}\nEXTENT / {extent}\nPREPARATION / {prep}\nFINDINGS AND INTERVENTIONS / {findings}\nPATHOLOGY / {path}\n{consent}"
    elif layout == "letter":
        text = f"{unit}\nDear referring colleague,\nRe: {admin}\nWe performed colonoscopy for {indication}. {sedation}\n{history}\n{prep} {extent}\n{findings}\n{path}\n{consent}\nSigned: attending endoscopist, Endoscopy Service."
    elif layout == "day_surgery":
        text = f"{unit}\nDAY SURGERY PROCEDURE AND DISCHARGE NOTE\n{admin}\nProcedure today: colonoscopy for {indication}.\n{history}\nAnaesthesia/sedation record: {sedation}\nIntra-procedure record: {extent}\n{prep}\nInterventions: {findings}\nSpecimen follow-through: {path}\nDischarge: {consent} Written post-procedure advice supplied."
    elif layout == "dictated":
        text = f"{unit}\nDICTATED COLONOSCOPY NOTE\n{admin}\nReason for examination, {indication}. {sedation}\n{history}\nPreparation, {prep}\nInsertion and inspection, {extent}\nMucosal findings and action, {findings}\nLaboratory update, {path}\nEnd of dictation. {consent}"
    elif layout == "checklist":
        text = f"{unit}\nCOLONOSCOPY CARE RECORD\n{admin}\n[x] Consent checked\n[x] Indication: {indication}\n[x] Sedation record: {sedation}\nBackground: {history or 'Referral history reviewed.'}\nInsertion record -> {extent}\nCleansing record -> {prep}\nTissue record -> {findings}\nResults record -> {path}\n[x] Recovery stable; advice sheet issued."
    elif layout == "sectioned":
        text = f"{unit}\nCOLONOSCOPY / PROCEDURE RECORD\n{admin}\n1. Referral\n{indication}. {history}\n2. Medication\n{sedation}\n3. Technical examination\n{extent}\n{prep}\n4. Observation / treatment\n{findings}\n5. Histopathology record\n{path}\n6. Recovery\n{consent}"
    else:
        text = f"{unit}\nENDOSCOPY HANDOVER TO REFERRER\n{admin}\nToday's procedure: colonoscopy\nClinical question: {indication}\nBackground supplied: {history or 'No prior procedure details supplied.'}\nMedication: {sedation}\nTechnical summary: {prep} {extent}\nProcedure actions: {findings}\nLaboratory record: {path}\nRecovery handover: {consent}"
    if addendum:
        text += "\n\nADDENDUM / CORRECTION — later entry\n" + addendum
    if spelling == "Authorized":
        text = text.replace("Anaesthesia/sedation", "Anesthesia/sedation")
    if arabic:
        # These lines only repeat the final documentary facts, never hidden facts.
        text += "\n\nملاحظة باللغة العربية تخص فحص اليوم:\nتم شرح الإجراء وتعليمات ما بعد الفحص للمريض."
        if blocks[0] and arabic[0] != "not_stated":
            text += "\n" + ("تم الوصول إلى الأعور." if arabic[0] == "yes" else "لم يتم الوصول إلى الأعور.")
        if arabic[1] != "not_stated":
            text += "\n" + ("تحضير الأمعاء كافٍ للفحص." if arabic[1] == "adequate" else "تحضير الأمعاء غير كافٍ للفحص.")
    return text.strip() + "\n"


def generate():
    rng = random.Random(SEED)
    order = list(range(len(CASES))) * 10
    rng.shuffle(order)
    arabic_indices = set(rng.sample(range(300), 60))
    ocr_indices = set(rng.sample(range(300), 50))
    prior_indices = set(rng.sample(range(300), 60))
    us_indices = set(rng.sample(range(300), 150))
    reports = []
    provenance = []
    for i, case_index in enumerate(order):
        case = CASES[case_index]
        uid = f"r{i + 1:03d}"
        accession = f"C26-H5-{i + 1:03d}"
        us = i in us_indices
        layout = LAYOUTS[i % len(LAYOUTS)]
        age = rng.randint(24, 83)
        sex = rng.choice(("female", "male"))
        visit = f"H5-{i + 1:03d}, 2026-09-{rng.randint(1, 28):02d}"
        patient = f"Synthetic patient {i + 1:03d}, {age} years, {sex}"
        indication = rng.choice(("positive faecal immunochemical test", "iron deficiency anaemia", "altered bowel habit", "intermittent rectal bleeding", "screening", "chronic watery diarrhoea"))
        if case.tissue_style == "biopsy":
            indication = "chronic watery diarrhoea"
        if us:
            indication = indication.replace("faecal", "fecal").replace("diarrhoea", "diarrhea").replace("anaemia", "anemia")
        sedation = rng.choice(("IV midazolam and fentanyl; supplemental oxygen given.", "Propofol sedation under anaesthetic supervision.", "Unsedated examination at patient request; observations monitored."))
        if us:
            sedation = sedation.replace("anaesthetic", "anesthetic")
        spelling = "Authorized" if us else "Authorised"
        blocks = clinical_blocks(case, rng, us, accession)
        prior = i in prior_indices or case.name == "old_histology"
        history = ""
        if prior:
            history = rng.choice((
                "Prior procedure, 2024: complete colonoscopy with good preparation and benign polyp histology. Those findings are historical.",
                "Previous colonoscopy in 2024 stopped in sigmoid colon with poor preparation; prior biopsies showed no dysplasia. The current examination is recorded below.",
                "Historical 2024 endoscopy: polypectomy with tubular adenoma on histology; that accession is separate from today's procedure.",
            ))
        meta = (rng.choice(UNITS), visit, patient, indication, sedation, spelling)
        text = render(layout, meta, blocks, history, case.fields if i in arabic_indices else None)
        if i in ocr_indices:
            # Fixed administrative OCR artefacts preserve every clinical numeral
            # and qualifier; no machine needs to guess a corrupted Boston score.
            text = text.replace("COLONOSCOPY", "C0L0N0SCOPY").replace("Consent", "C0nsent")
            text = "SCANNED C0PY | batch H5 | page 1 of 1\n" + text.replace(" | Visit", "  |  Visit")
            text += "[scan footer: verif1ed / end of rec0rd]\n"
        extent, prep, tissue, histology = case.fields
        state, reason = documentary_key(case.fields)
        tags = {
            f"fmt:{layout}", f"spelling:{'US' if us else 'UK'}",
            f"prep:{case.prep_style}", f"tissue:{case.tissue_style}",
            f"extent:{extent}",
        }
        tags.update("hard:" + family for family in case.hard)
        tags.update(case.extra)
        if case.tissue_style == "none":
            tags.add("doc:negated_tissue")
        if prior:
            tags.update(("doc:prior_procedure", "hard:prior_episode"))
        if i in arabic_indices:
            tags.add("lang:arabic")
        if i in ocr_indices:
            tags.add("noise:ocr")
        reports.append({
            "uid": uid,
            "sha": hashlib.sha256(text.encode("utf-8")).hexdigest(),
            "text": text,
            "truth": latent_truth(case.reality),
            "key_fields": {"caecum": extent, "prep": prep, "polyps": tissue, "tissue_taken": tissue, "histology": histology},
            "key_state": state,
            "key_reason": reason,
            "tags": sorted(tags),
        })
        provenance.append(case)
    validate(reports, provenance)
    return {
        "source": "Fifth independent author (GPT-6.1 Sol, OpenAI) - deterministic stdlib generator gen_heldout5.py, random seed 5051, 300 synthetic Abu Dhabi colonoscopy reports, " + DATASET_DATE,
        "reports": reports,
    }


def validate(reports, provenance):
    assert len(reports) == 300
    assert len({r["text"] for r in reports}) == 300
    assert len({r["sha"] for r in reports}) == 300
    tags = Counter(t for r in reports for t in r["tags"])
    truth_counts = Counter(r["truth"] for r in reports)
    assert truth_counts == {"adequate": 120, "awaiting_histology": 90, "inadequate": 90}
    for i, (r, case) in enumerate(zip(reports, provenance), 1):
        assert r["uid"] == f"r{i:03d}"
        assert r["sha"] == hashlib.sha256(r["text"].encode("utf-8")).hexdigest()
        assert r["truth"] == latent_truth(case.reality)
        f = r["key_fields"]
        fields = (f["caecum"], f["prep"], f["tissue_taken"], f["histology"])
        assert fields == case.fields
        reached, clean, taken, result_back = case.reality
        if f["caecum"] != "not_stated":
            assert (f["caecum"] == "yes") == reached
        if f["prep"] != "not_stated":
            assert (f["prep"] == "adequate") == clean
        assert (f["tissue_taken"] == "yes") == taken
        assert (f["histology"] == "yes") == (taken and result_back)
        assert (r["key_state"], r["key_reason"]) == documentary_key(fields) == case.expected_key
        assert f["polyps"] == f["tissue_taken"]
        assert f["caecum"] in {"yes", "no", "not_stated"}
        assert f["prep"] in {"adequate", "inadequate", "not_stated"}
        assert f["tissue_taken"] in {"yes", "no"} and f["histology"] in {"yes", "no"}
        assert f["histology"] != "yes" or f["tissue_taken"] == "yes"
        assert case.reality[2] or not case.reality[3]
        assert all(sum(t.startswith(prefix) for t in r["tags"]) == 1 for prefix in ("fmt:", "spelling:", "prep:", "tissue:"))
        assert all(t[5:] in HARD_FAMILIES for t in r["tags"] if t.startswith("hard:"))
        hidden_failure = (not case.reality[0] and f["caecum"] == "not_stated") or (not case.reality[1] and f["prep"] == "not_stated")
        assert ("hidden:undocumented_failure" in r["tags"]) == hidden_failure
        assert ("lang:arabic" in r["tags"]) == any("\u0600" <= ch <= "\u06ff" for ch in r["text"])
        assert ("noise:ocr" in r["tags"]) == r["text"].startswith("SCANNED C0PY")
        assert ("doc:later_correction" in r["tags"]) == ("ADDENDUM / CORRECTION" in r["text"])
    requirements = {
        "lang:arabic": 30, "noise:ocr": 25, "prep:ungraded": 40,
        "extent:not_stated": 25, "prep:boston": 30, "tissue:biopsy": 25,
        "doc:negated_tissue": 20, "doc:later_correction": 10, "doc:prior_procedure": 10,
    }
    assert all(tags[tag] >= minimum for tag, minimum in requirements.items())
    assert sum(t.startswith("fmt:") for t in tags) >= 6
    assert tags["spelling:UK"] > 0 and tags["spelling:US"] > 0
    assert sum(t.startswith("hard:") for t in tags) >= 10
    assert sum(any(t.startswith("hard:") for t in r["tags"]) for r in reports) >= 105


def main():
    dataset = generate()
    encoded = (json.dumps(dataset, ensure_ascii=False, indent=2) + "\n").encode("utf-8")
    destination = Path(__file__).resolve().with_name("reports_heldout5.json")
    destination.write_bytes(encoded)
    print("Wrote reports_heldout5.json: 300 reports")
    print("Truth: " + json.dumps(dict(sorted(Counter(r["truth"] for r in dataset["reports"]).items()))))
    print("JSON SHA-256: " + hashlib.sha256(encoded).hexdigest())


if __name__ == "__main__":
    main()
