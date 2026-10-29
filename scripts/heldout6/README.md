# Dataset Card: Synthetic Abu Dhabi Colonoscopy Corpus (`reports_heldout6.json`)

**Author:** independent author (Gemini 4 Argon, Google)  
**Generator:** `gen_heldout6.py` (Python 3 standard library only, deterministic seed `6067`)  
**Output file:** `reports_heldout6.json` (300 unique synthetic colonoscopy reports with clinical ground-truth and text-supported answer keys)

---

## 1. How It Works

`gen_heldout6.py` deterministically generates 300 synthetic colonoscopy reports modelled after real-world documentation across Abu Dhabi day-surgery centres and hospital endoscopy suites (e.g., SSMC, CCAD, SKMC, Tawam, Burjeel Medical City, Mediclinic Airport Road, NMC Royal, Danat Al Emarat, Al Rahba, Corniche Hospital) operating under Department of Health – Abu Dhabi (DoH) colorectal cancer screening and clinical guidelines.

Each report is constructed from two distinct layers:
1. **Hidden Reality (`truth`):** Whether the caecum (or surgical ileocolic anastomosis) was physically reached (`r_caecum`), whether post-washing mucosal preparation was adequate (`r_prep`), whether any tissue was sampled via polypectomy or biopsy during this exam (`r_tissue`), and whether the histopathology result for that tissue has returned (`r_histo`).
2. **Text-Supported Clinical Reading (`key_fields`, `key_state`, `key_reason`):** What a careful clinician reading the complete report text (including any addenda/corrections) can substantiate, evaluated strictly via the ordered decision cascade:
   - `caecum == "no"` $\rightarrow$ `not_closed` / `incomplete_exam`
   - `prep == "inadequate"` $\rightarrow$ `not_closed` / `inadequate_prep`
   - `caecum == "not_stated"` or `prep == "not_stated"` $\rightarrow$ `needs_human` / `missing_documentation`
   - `tissue_taken == "yes"` and `histology == "no"` $\rightarrow$ `not_closed` / `awaiting_histology`
   - Otherwise $\rightarrow$ `closed` / `criteria_met`

Running `python3 gen_heldout6.py` produces byte-identical `reports_heldout6.json` across runs without network, clock, or external file dependencies.

---

## 2. Corpus Distribution & Tag Counts

### Ground-Truth (`truth`) Distribution
| `truth` | Count | Percentage |
| :--- | ---: | ---: |
| `adequate` | 120 | 40.0% |
| `awaiting_histology` | 90 | 30.0% |
| `inadequate` | 90 | 30.0% |
| **Total** | **300** | **100.0%** |

### Answer Key State (`key_state` / `key_reason`) Distribution
| `key_state` | `key_reason` | Count |
| :--- | :--- | ---: |
| `closed` | `criteria_met` | 94 |
| `not_closed` | `awaiting_histology` | 70 |
| `not_closed` | `incomplete_exam` | 46 |
| `not_closed` | `inadequate_prep` | 34 |
| `needs_human` | `missing_documentation` | 56 |

### Structural, Language, Noise, Prep & Tissue Tags
| Category | Tag | Count | Requirement |
| :--- | :--- | ---: | :--- |
| **Layouts (7 distinct)** | `fmt:seha_table` | 42 | $\ge 6$ layouts |
| | `fmt:ccad_narrative` | 41 | |
| | `fmt:day_surgery_compact` | 38 | |
| | `fmt:bilingual_form` | 55 | |
| | `fmt:discharge_letter` | 42 | |
| | `fmt:provation_Audit` | 41 | |
| | `fmt:pathology_bundle` | 41 | |
| **Spelling** | `spelling:uk` | 150 | UK & US |
| | `spelling:us` | 150 | |
| **Preparation** | `prep:verbal` | 203 | |
| | `prep:bbps` | 55 | $\ge 30$ Boston scores |
| | `prep:ungraded` | 42 | $\ge 40$ ungraded prep |
| **Extent** | `extent:stated` | 272 | |
| | `extent:not_stated` | 28 | $\ge 25$ unstated extent |
| **Tissue Subtype** | `tissue:none` | 132 | |
| | `tissue:polyp` | 133 | |
| | `tissue:biopsy` | 35 | $\ge 25$ non-polyp biopsies |
| **Bilingual / Noise** | `lang:arabic` | 34 | $\ge 30$ Arabic text |
| | `noise:ocr` | 28 | $\ge 25$ scan/OCR noise |
| **Hidden Failures** | `hidden:undocumented_failure` | 14 | Unstated extent/prep hiding real failure |

---

## 3. Hard-Case Families (116 Reports / 38.7% of Corpus)

Every report containing a nuanced clinical reading challenge carries a `hard:*` tag. In total, **116 of 300 reports (38.7%)** carry at least one `hard:*` tag across 12 distinct families (the required core families plus 10 clinician-focused hard-case families):

| Tag | Count | One-Line Description |
| :--- | ---: | :--- |
| `hard:negated_tissue` | 24 | Explicit multi-phrased negation of biopsies and polypectomies (`polyps: "no"`, `tissue_taken: "no"`). |
| `hard:prior_exam_distractor` | 14 | References prior incomplete colonoscopies, prior poor BBPS scores, or prior adenoma histology that must not be attributed to today's exam. |
| `hard:addendum_override` | 12 | Late addendum or nursing/photo reconciliation overrides the main body (histology arrives, caecal extent retracted to hepatic flexure, or omitted rectal polyp biopsy added). |
| `hard:bbps_subscore_trap` | 10 | Total BBPS score is $\ge 6$ (e.g., `3+3+1=7/9` or `2+3+1=6/9`), but one segment score is `< 2`, rendering preparation `inadequate`. |
| `hard:post_wash_salvage` | 10 | Initial insertion describes murky/poor effluent, but vigorous irrigation and suctioning achieve `adequate` post-wash mucosal inspection. |
| `hard:histo_requisition_only_not_result` | 10 | Contains pathology jar barcodes, CPT codes, and provisional clinical impressions on the requisition slip ("likely tubular adenoma"), while actual histology is still pending (`histology: "no"`). |
| `hard:failed_ti_but_caecum_reached` | 10 | Terminal ileal (TI) intubation failed after multiple attempts ("failed intubation"), but the caecum itself was fully reached and landmarks photographed (`caecum: "yes"`). |
| `hard:optical_lesion_not_sampled` | 9 | Submucosal lipoma (positive pillow sign) or diminutive Kudo Type I lesion optically characterised with NBI and left in situ without biopsy or resection (`tissue_taken: "no"`). |
| `hard:polyp_seen_unresected_anticoag` | 9 | True polyp (8–14 mm) identified and photographed, but neither biopsied nor resected today due to uninterrupted anticoagulation/antiplatelets (`polyps: "no"`, `tissue_taken: "no"`). |
| `hard:distant_landmark_illusion` | 8 | Caecal pole or ICV glimpsed from afar / transilluminated from the mid-ascending colon, but looping prevented physical intubation of the caecum (`caecum: "no"`). |
| `hard:icv_lip_only_failure` | 8 | Scope tip reached only the inferior lip of the ileocaecal valve without intubating the caecal caput or visualising the appendiceal orifice (`caecum: "no"`). |
| `hard:ileocaecal_resection_anastomosis` | 8 | Patient has a prior right hemicolectomy (native caecum absent); reaching the surgical ileocolic anastomosis represents complete anatomical extent (`caecum: "yes"`). |
| `hard:fecal_water_aspirated_trap` | 8 | Mentions dark faecal-stained water or turbid fluid in dependent folds, which was completely aspirated to yield `adequate` visualisation. |
| `hard:therapeutic_clip_or_apc_no_tissue` | 8 | Endoscopic therapy performed (APC of caecal angiodysplasias or clipping of diverticular vessel) without any tissue biopsy or polypectomy (`tissue_taken: "no"`). |

*(Note: Some reports combine `hard:addendum_override` or `hard:prior_exam_distractor` with other tags; 116 unique reports have $\ge 1$ `hard:*` tag.)*

---

## 4. Keying Decisions a Reviewer Should Check

1. **`polyps` / `tissue_taken` equivalence (`hard:polyp_seen_unresected_anticoag`, `tissue:biopsy`):** Per the specification, `polyps` in `key_fields` is defined as `(= tissue taken this procedure)` and shares the exact value of `tissue_taken`. Consequently, random biopsies for microscopic colitis with zero macroscopic polyps are keyed as `polyps: "yes", tissue_taken: "yes"`, whereas a macroscopic polyp left untouched in situ because the patient is on clopidogrel/apixaban is keyed as `polyps: "no", tissue_taken: "no"` (since no tissue was taken during this procedure, so no histology is awaited).
2. **Surgical Anastomosis as Complete Extent (`hard:ileocaecal_resection_anastomosis`):** In patients status-post right hemicolectomy / ileocaecal resection, native caecal landmarks do not exist; intubating the ileocolonic surgical anastomosis completes the colonic examination and is keyed as `caecum: "yes"`.
3. **ILEOCAECAL Valve Lip vs Caecal Pole (`hard:icv_lip_only_failure` vs `hard:failed_ti_but_caecum_reached`):** Reaching only the distal lip of the ICV without entering the caecal pole is an incomplete colonoscopy (`caecum: "no"`). Conversely, reaching the caecal pole and identifying the appendiceal orifice while failing to intubate the terminal ileum is a complete colonoscopy (`caecum: "yes"`).
4. **Addendum Supremacy (`hard:addendum_override`):** When a same-day consultant image review retracts caecal intubation to the hepatic flexure, `caecum` is keyed `"no"`. When a pathology addendum appends the final microscopic diagnosis days later, `histology` is keyed `"yes"`. When nursing specimen reconciliation adds a diminutive rectal polyp biopsy omitted from the main body, `tissue_taken` is keyed `"yes"` and `histology` is `"no"`.
