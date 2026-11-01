#!/usr/bin/env python3
"""Fresh held-out LDCT corpus (screen reports + work-up notes) with explicit answer keys.

Hand-authored text, written without opening scripts/ldct_corpus.py and never scored before the corpus was frozen in
git. Standard library only; deterministic. Writes screen_reports.json and workup_notes.json into the directory given
as argv[1] (default data/ldct/heldout under the repo root).

Key conventions (the Lung-RADS v2022 labels the pipeline uses):
  screen label: the exam's overall category as a radiologist would assign it (the most concerning finding; an
  explicitly stated final category is the same thing). S modifier ignored. LR0 LR1 LR2 LR3 LR4A LR4B LR4X.
  work-up label: wk_dna (did not attend) | wk_incomplete (work-up not done, tissue pending, or outcome pending) |
  wk_treatment (referred for cancer treatment AND malignant tissue AND MDT discussed) | wk_benign (return to annual
  screening, tissue not malignant, with benign tissue or an MDT decision) | wk_surveillance (interval imaging,
  tissue not malignant) | indeterminate (internally contradictory or too little to decide: a human must read it).
"""
import json, os, sys

S = []
W = []


def s(label, tags, text):
    S.append((label, tags, text))


def w(label, tags, text):
    W.append((label, tags, text))


# ---- screening reports: LR1 ----
s("LR1", ["normal"], "LUNG CANCER SCREENING - LOW DOSE CT\nTechnique: non-contrast LDCT, 1 mm reconstructions.\nFindings: Lungs are clear. Airways patent. No pulmonary nodule is seen. Mediastinum unremarkable.\nImpression: Lung-RADS 1. Next screen in 12 months.")
s("LR1", ["normal", "negated"], "Screening LDCT thorax. No nodules, no masses, no consolidation, no effusion. No suspicious finding. Final category: Lung-RADS 1 - negative.")
s("LR1", ["normal", "modifier"], "Baseline low-dose CT. Mild centrilobular emphysema in the upper lobes. No nodule. Mild coronary artery calcification noted - for GP review.\nLung-RADS: 1S.\nAnnual LDCT.")
s("LR1", ["normal", "distractor"], "LDCT chest (annual round 3). Compared with the 2025 study, which was reported as Lung-RADS 2 because of a new 4 mm solid nodule, that nodule is no longer visible; nothing new. Overall assessment this year: Lung-RADS category 1.")
s("LR1", ["normal", "format"], "LDCT SCREEN | Result: LR-1 | Nodules: none | Recommendation: routine 12-month LDCT.")
s("LR1", ["normal", "negated", "distractor"], "Low dose chest CT, screening programme. There is no pulmonary nodule and no feature to suggest Lung-RADS 3, 4A, 4B or 4X. Lungs and pleura are normal. Overall: negative screen (Lung-RADS 1).")
s("LR1", ["normal", "negated", "distractor"], "LDCT chest screening. Impression: the study is NOT Lung-RADS 3 or 4 - there is no significant finding. A completely calcified granuloma (3 mm) in the left lower lobe is a benign calcification. Lung-RADS 1. See you next year.")
# ---- LR2 ----
s("LR2", ["normal"], "LDCT lung screening. Solid nodule right lower lobe, 3.5 mm mean diameter, unchanged since the prior round. Perifissural nodule left, 4 mm, smooth, triangular. Both benign appearance.\nImpression: Lung-RADS 2 (benign appearance or behaviour). Continue annual screening.")
s("LR2", ["normal"], "Screening CT chest (low dose). Calcified granuloma in the right upper lobe. Solid nodule left lower lobe 4.5 mm, smooth. No other nodule.\nFinal: LungRADS 2.")
s("LR2", ["boundary"], "LDCT screening, baseline. Solid nodule left upper lobe measuring 5.6 mm mean diameter (below the 6 mm threshold for category 3 on a baseline scan). No suspicious features.\nLung-RADS category 2.\nRepeat at 12 months.")
s("LR2", ["distractor"], "LDCT. 5 mm ground-glass nodule right middle lobe, unchanged compared with the previous year (non-solid nodules under 30 mm are category 2). Conclusion: Lung-RADS 2. Annual screening.")
s("LR2", ["negated"], "Low-dose CT for lung cancer screening: small subpleural nodule 3 mm, stable. There is no suspicious nodule, no spiculated mass and no lymphadenopathy. Lung-RADS: 2S (moderate emphysema). Next LDCT in 12 months.")
s("LR2", ["format"], "LDCT screening result - category: 2 (Lung-RADS v2022). Findings: scattered solid nodules all under 6 mm, smooth. Action: annual.")
s("LR2", ["distractor"], "Annual LDCT. A 2022 study had Lung-RADS 4A for a nodule that has since resolved completely. Today: a stable 3 mm solid nodule in the lingula and linear scarring; nothing else. Lung-RADS 2.")
s("LR2", ["distractor"], "Low-dose CT lung screening. Comparison: 2024 - Lung-RADS 4B; subsequent PET and biopsy showed organising pneumonia (benign) and the area has now resolved. Today: 4 mm solid nodule right apex, new but under 6 mm. Overall Lung-RADS 2. (Do not apply the 2024 category.)")
s("LR2", ["boundary"], "Annual LDCT. A new solid nodule 3.8 mm mean diameter right lower lobe (new nodules below 4 mm are category 2). Otherwise normal lungs. Lung-RADS 2.")
s("LR2", ["words"], "Low dose CT lung cancer screening. Solid nodule 4 mm, benign appearance. Final assessment: Lung-RADS two (benign). Annual screening to continue.")
# ---- LR0 ----
s("LR0", ["boundary"], "LDCT screening. Examination is incomplete: respiratory motion artefact in the lower zones limits assessment; additional imaging is needed. Lung-RADS 0. Recall for a repeat limited LDCT.")
s("LR0", ["boundary", "distractor"], "Low-dose CT chest, baseline screening. Several nodules cannot be characterised without the prior CT from an outside hospital, which has been requested. Category: Lung-RADS 0 (comparison with prior imaging pending). Result will be finalised on receipt.")
s("LR0", ["negated"], "Screening LDCT. Part of the left lower lobe is obscured by dense atelectasis, so a nodule cannot be excluded there. Not a negative screen. Lung-RADS 0 - repeat LDCT in 1-3 months.")
s("LR0", ["format"], "LDCT SCREEN | Result: category 0 | Reason: infectious/inflammatory process suspected, treat and repeat LDCT 1-3 months | Not reportable as negative.")
s("LR0", ["distractor"], "LDCT screening. Technically limited by patient motion. The radiologist notes that if the repeat shows no nodule, this would be Lung-RADS 1, but until then the category is 0 (incomplete).")
# ---- LR3 ----
s("LR3", ["boundary"], "Baseline LDCT. Solid nodule right upper lobe, mean diameter 7.2 mm, smooth margins. Probably benign.\nImpression: Lung-RADS 3. Short-interval LDCT in 6 months.")
s("LR3", ["boundary"], "LDCT screening. Part-solid nodule left lower lobe, total diameter 8 mm, solid component 4 mm (under 6 mm). Overall Lung-RADS category 3, six month LDCT advised.")
s("LR3", ["distractor"], "Annual LDCT. The referral form queries Lung-RADS 4 because of a family history of lung cancer, which does not change the category. Imaging: 7 mm solid nodule left apex, newly seen on this round (new nodules of 4 mm to under 8 mm are category 3). A separate 2 mm calcified granuloma is benign. Final: Lung-RADS 3.")
s("LR3", ["format"], "LDCT SCREEN - LR 3 - probably benign - repeat LDCT 6 mo.")
s("LR3", ["negated", "boundary"], "Low-dose lung screening CT. 30 mm non-solid ground-glass nodule right upper lobe, new on this scan. No solid component, no spiculation, no lymphadenopathy. Lung-RADS 3 (non-solid nodule of 30 mm or more). Recommend LDCT at 6 months.")
# ---- LR4A / 4B / 4X ----
s("LR4A", ["boundary"], "Annual LDCT. Solid nodule right middle lobe now 9 mm, previously 5 mm: growth of 4 mm. Suspicious. Lung-RADS 4A. Diagnostic CT or 3-month LDCT; PET/CT may be considered for 8 mm or larger solid component.")
s("LR4A", ["boundary"], "Baseline LDCT. Solid nodule left lower lobe, mean diameter 9.5 mm. Impression: Lung-RADS 4A - recommend LDCT in 3 months, PET/CT if solid component 8 mm or more.")
s("LR4A", ["distractor"], "Screening LDCT. Subsolid nodule right upper lobe 14 mm with 7 mm solid component. A separate 4 mm solid nodule is category 2. The overall exam takes the more concerning finding: Lung-RADS 4A.")
s("LR4A", ["format"], "LDCT SCREEN RESULT | Lung-RADS 4A | Suspicious | Follow-up LDCT in 3 months.")
s("LR4A", ["negated"], "Low dose CT thorax. New solid nodule 8.5 mm right lower lobe (new nodules of 8 mm or more and under 15 mm are category 4A). No lymphadenopathy, no spiculation, no features of 4X. Final category: 4A.")
s("LR4A", ["boundary", "words"], "Screening low-dose CT. Solid nodule right lung, 10 mm. Final assessment given in words: category four A, suspicious. Three-month follow-up LDCT recommended.")
s("LR4B", ["boundary"], "Baseline LDCT. Solid nodule left upper lobe 17 mm, spiculated. Very suspicious. Lung-RADS 4B. Refer for chest physician review, tissue sampling and/or PET/CT.")
s("LR4B", ["boundary"], "Annual screening CT. Solid nodule right lower lobe grown from 10 mm to 16 mm. Lung-RADS category 4B. Urgent referral to the lung multidisciplinary team.")
s("LR4B", ["format"], "LDCT SCREEN | category 4B | Solid 15 mm+ | Action: diagnostic work-up within 30 days.")
s("LR4B", ["distractor"], "Low dose CT. Part-solid nodule 22 mm, solid component 10 mm. Prior LDCT a year ago was Lung-RADS 2 (no nodule at that site). This represents a new, very suspicious lesion. Overall: Lung-RADS 4B.")
s("LR4B", ["words"], "Annual LDCT. Large solid nodule left lower lobe 28 mm. Lung-RADS category four B - very suspicious. Immediate referral.")
s("LR4X", ["boundary"], "Screening LDCT. Spiculated 12 mm solid nodule right upper lobe with ipsilateral enlarged hilar lymph nodes, 18 mm short axis. Category 4 with additional suspicious features: Lung-RADS 4X. Urgent multidisciplinary team referral.")
s("LR4X", ["format"], "LDCT SCREEN | 4X | nodule with additional features suspicious for lung cancer (lymphadenopathy, pleural thickening) | refer urgently.")
s("LR4X", ["distractor"], "Annual LDCT. Previously Lung-RADS 3 (7 mm nodule). The nodule is now 11 mm, with a new pleural-based nodule and an enlarged subcarinal node (16 mm). Overall assessment: Lung-RADS category 4X.")

# ---- work-up notes ----
w("wk_dna", ["dna"], "Lung health check clinic - diagnostic work-up after LDCT (Lung-RADS 4B). The patient did not attend her CT-guided biopsy appointment on the 12th and could not be reached by telephone. Letter sent. Rebooking pending.")
w("wk_dna", ["dna", "distractor"], "Thoracic assessment clinic. Patient failed to attend today's consultation. Note: she attended the pre-assessment spirometry last week (normal), but no work-up of the nodule has taken place and no MDT discussion yet.")
w("wk_dna", ["dna"], "Outcome of invitation for PET/CT following abnormal screening LDCT: DNA x2. Referrer informed.")
w("wk_dna", ["dna", "format"], "WORK-UP RESULT | attended: no (DNA) | biopsy: none | outcome: none.")
w("wk_incomplete", ["pending"], "Bronchoscopy with EBUS-TBNA performed today for a 16 mm right upper lobe nodule (Lung-RADS 4B). Samples sent to cytology. Result awaited. Case to be discussed at lung MDT once the result is available.")
w("wk_incomplete", ["declined"], "Respiratory clinic review after LDCT category 4A. PET/CT arranged but the patient declined the scan and any biopsy for now, wishes to think. No investigation was done. To return in 4 weeks.")
w("wk_incomplete", ["pending", "distractor"], "PET/CT today: FDG-avid nodule SUVmax 7.4. A CT-guided core biopsy was taken; histology not yet reported. The previous screening round in 2025 was Lung-RADS 2, no relevance. Awaiting pathology before any decision.")
w("wk_incomplete", ["not_done"], "Appointment for diagnostic CT thorax cancelled because the scanner was out of service; postponed to next week. No investigation performed. She attended for the booking.")
w("wk_incomplete", ["pending", "negated"], "MDT outcome not yet available: case not discussed at the lung MDT; biopsy not yet performed. No decision has been made about treatment or return to screening.")
w("wk_incomplete", ["pending"], "Lung MDT meeting list: patient listed for discussion next Thursday following biopsy on the 4th. Histology: malignant cells identified, pending immunohistochemistry typing; final report awaited. Plan: for discussion.")
w("wk_incomplete", ["pending", "boundary"], "She attended. CT-guided biopsy done and the cytology preliminary report suggests possible carcinoma but the final report is not yet available. Lung MDT has not met. Outcome pending.")
w("wk_incomplete", ["format"], "WORK-UP RESULT | attended yes | investigation: not done (patient unwell) | biopsy: none | outcome: pending.")
w("wk_treatment", ["treatment"], "Lung MDT outcome. Histology from CT-guided biopsy: adenocarcinoma. PET/CT: localised, no nodal disease. Discussed at the thoracic multidisciplinary meeting - recommendation: lobectomy. Referred to thoracic surgery.")
w("wk_treatment", ["treatment"], "Lung cancer MDT (discussed on the 9th). EBUS-TBNA cytology positive for non-small cell lung carcinoma. Stage IIIA. Plan: concurrent chemoradiotherapy. Referral to clinical oncology made today.")
w("wk_treatment", ["treatment", "distractor"], "Attended thoracic clinic. Seen last year (2025) for a benign granuloma, which is unrelated. Current nodule: biopsy shows squamous cell carcinoma. MDT decision documented: stereotactic body radiotherapy (SABR) for an early-stage tumour. Referred to oncology.")
w("wk_treatment", ["treatment"], "Thoracic MDT: 4X lesion left upper lobe, core biopsy malignant (small cell carcinoma, limited stage). MDT recommends chemoradiotherapy; referral to medical oncology sent.")
w("wk_treatment", ["treatment", "format"], "MDT OUTCOME | attended: yes | biopsy: adenocarcinoma (malignant) | discussed at lung MDT: yes | plan: surgery, referral to thoracic surgery done.")
w("wk_treatment", ["treatment", "boundary"], "MDT note: previously (2024) a benign lesion was resected on the other side. This episode: biopsy of the right nodule is malignant, small cell. MDT decision: referral to oncology for systemic therapy.")
w("wk_benign", ["benign"], "Diagnostic work-up complete. PET/CT: low uptake. CT-guided biopsy: granulomatous inflammation, no malignancy. Discussed at lung MDT: benign. Patient to return to the annual lung screening programme.")
w("wk_benign", ["benign", "negated"], "Bronchoscopy and biopsy of right lower lobe lesion. Histology shows organising pneumonia; there was no evidence of malignancy or carcinoma. MDT: no further action; return to annual LDCT screening.")
w("wk_benign", ["benign", "distractor"], "Lung MDT outcome (nodule 4A). The family history of cancer was noted but is not relevant to this decision. Tissue: benign hamartoma on biopsy, no malignant cells. Decision: back to the annual screening round.")
w("wk_benign", ["benign", "mdt-only"], "Respiratory MDT discussion: after interval LDCT and PET showing no uptake, the lesion has resolved on imaging. No biopsy was required. MDT decision: benign inflammatory change; return to annual screening.")
w("wk_benign", ["benign", "format"], "WORK-UP RESULT | attended yes | biopsy: benign | MDT: discussed - benign | outcome: return to annual LDCT.")
w("wk_benign", ["benign", "boundary"], "Attended. Biopsy: non-diagnostic sample; repeat biopsy not needed after MDT review of PET (no uptake). MDT decision: benign; return to annual screening.")
w("wk_surveillance", ["surveillance"], "Respiratory clinic. PET/CT shows mild uptake (SUV 2.1) in the 9 mm nodule; biopsy not feasible given position, benign by MDT consensus. Plan: surveillance CT in 3 months.")
w("wk_surveillance", ["surveillance", "negated"], "Percutaneous biopsy: benign lung tissue, no malignancy. Lung MDT agreed to keep under surveillance. Interval CT in six months before any return to the screening programme.")
w("wk_surveillance", ["surveillance"], "Thoracic clinic review. Nodule assessed with diagnostic CT; the MDT recommended repeat CT in 3 months to confirm stability.")
w("wk_surveillance", ["format"], "WORK-UP RESULT | attended yes | investigation: diagnostic CT done | tissue: none | outcome: repeat CT 3 months.")
w("indeterminate", ["contradiction"], "MDT summary: biopsy shows adenocarcinoma. Decision: return to annual screening with LDCT in 12 months.")
w("indeterminate", ["contradiction"], "Clinic note: referred to thoracic surgery for lobectomy. Biopsy: benign. No MDT mentioned.")
w("indeterminate", ["no-outcome"], "Short letter: the patient attended the thoracic clinic today. Examination unremarkable. Thank you for your referral.")


def dump(path, source, rows, prefix):
    out = {"source": source, "reports": []}
    for i, (label, tags, text) in enumerate(rows, 1):
        out["reports"].append({"id": "%s%02d" % (prefix, i), "text": text, "key": {"label": label}, "tags": sorted(tags)})
    assert len({r["text"] for r in out["reports"]}) == len(rows)
    with open(path, "w", encoding="utf-8") as f:
        json.dump(out, f, indent=1, ensure_ascii=False)
        f.write("\n")


if __name__ == "__main__":
    d = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "data", "ldct", "heldout")
    os.makedirs(d, exist_ok=True)
    dump(os.path.join(d, "screen_reports.json"), "Held-out LDCT screening reports; hand-authored, frozen in git before any reader was run; synthetic", S, "hs")
    dump(os.path.join(d, "workup_notes.json"), "Held-out LDCT work-up notes; hand-authored, frozen in git before any reader was run; synthetic", W, "hw")
    print(len(S), len(W))
