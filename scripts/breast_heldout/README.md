# Dataset card: Synthetic Abu Dhabi breast screening and assessment texts

## Authorship and provenance

Author: independent author (GPT-6.1 Sol, OpenAI)

Version date: 2026-11-01  
Generator: `gen_breast_heldout.py`  
Random seed: `9241`

All documents are invented. They describe fictional Abu Dhabi screening units,
diagnostic breast assessment units and breast MDTs. No patient records, real
provider records or external data were used.

## Generation and reproducibility

Run:

    python3 gen_breast_heldout.py

The generator writes these files beside the script, overwriting existing copies:

- `screen_reports.json`: 40 screening mammogram reports, `hb01`–`hb40`.
- `assess_notes.json`: 50 screening-recall assessment/MDT documents,
  `ha01`–`ha50`.

Python 3 and its standard library are the only dependencies. Generation uses no
network, model calls, clock or input-file reads. Case allocations and clinical
facts are explicit; a locally seeded pseudorandom generator selects limited
wording and fictional unit-name variations.

Each output is a JSON object with `source` and `reports`. Each report contains
`id`, `text`, `key` and `tags`. UTF-8 encoding, fixed indentation, sorted tags,
fixed record order and a final LF make repeated runs byte-identical in the same
Python environment. No absolute paths are embedded.

All 90 texts are unique. Each file uses six layouts. Both UK and US spelling
occur. Arabic is provided as bilingual headers and, in selected screening
reports, breast-side names. OCR artifacts are deliberately mild and do not
obscure the clinical facts.

Internal assertions check IDs, uniqueness, schema, label derivation, exact label
and tag counts, coding rules and hard-case coverage.

## Intended use

Suitable for testing structured extraction, episode attribution, laterality,
negation scope and rule-based classification of synthetic breast documents.

Not suitable for clinical decisions, estimating Abu Dhabi disease prevalence,
validating Arabic clinical fluency, or demonstrating performance on real clinical
records. “Heldout” is a naming convention, not a claim of independence from any
particular training corpus.

## Label distribution

### Screening

| Label | Count | Percentage | Coded |
|---|---:|---:|---:|
| B1 | 10 | 25% | 5 |
| B2 | 8 | 20% | 4 |
| B0 | 8 | 20% | 4 |
| B3 | 2 | 5% | 1 |
| B4 | 6 | 15% | 3 |
| B5 | 3 | 7.5% | 3 |
| TR | 3 | 7.5% | 0 |
| **Total** | **40** | **100%** | **20** |

### Assessment/MDT

| Label | Count | Percentage |
|---|---:|---:|
| return_routine | 13 | 26% |
| early_recall | 5 | 10% |
| to_treatment | 10 | 20% |
| incomplete | 15 | 30% |
| dna | 7 | 14% |
| **Total** | **50** | **100%** |

## Keying decisions

### Screening

- `b_right` and `b_left` describe the current breast-specific assessments.
- Categories follow the requested ACR BI-RADS fifth-edition interpretation.
  Subcategories 4A, 4B and 4C become `"4"`. Category 6 is not used.
- Without technical inadequacy, aggregation uses
  `1 < 2 < 3 < 0 < 4 < 5`.
- If a breast is `TR`, an opposite-breast category of 0, 3, 4 or 5 determines
  the label. Otherwise the report label is `TR`.
- `overall` records only an expressly stated current overall category. It is
  `null` when none is assigned. A superseding amendment overrides a withdrawn
  preliminary overall category.
- Twenty reports have short coded synopses; the remaining twenty use uncoded
  narrative reports. CPT II mappings are B0→3340F, B1→3341F, B2→3342F,
  B3→3343F, B4→3344F and B5→3345F. A `TR` label is never coded.
- Technical inadequacy of one breast does not prevent coding an overall
  abnormal assessment driven by the other breast.
- Previous-screen categories are historical context, never current labels.

### Assessment/MDT

- Keys describe only the current screening-recall episode.
- `attended` means attendance at the assessment appointment, not an
  administrative telephone contact.
- `assessment="done"` includes clinical breast examination alone, even when
  diagnostic imaging or final management remains outstanding.
- `assessment="not_done"` records that no assessment occurred. For DNA cases,
  it is also used as the only available no-assessment value; `attended=false`
  takes classification precedence. It does not imply attendance.
- `biopsy` concerns current-episode needle/core pathology. Historical benign
  biopsies do not populate this field.
- `pending` biopsy means tissue was obtained and pathology is awaited.
  `none` means no current biopsy result is reported.
- A benign result does not imply routine screening when imaging/pathology are
  discordant. A malignant result does not imply that treatment referral has
  already been decided.
- DCIS is malignant for the biopsy field, including when invasion is absent.
- Classification precedence is: nonattendance → `dna`; otherwise no assessment,
  pending biopsy or pending outcome → `incomplete`; otherwise use the specified
  biopsy/outcome combinations.
- No case combines routine/early recall with malignant biopsy, or treatment
  with a nonmalignant biopsy.

## Tag counts

Tags are multilabel; columns must not be summed to obtain document counts.

### Screening tags

| Tag | Count |
|---|---:|
| language:arabic | 8 |
| scan:ocr | 4 |
| hard:prior_category | 8 |
| hard:zero_over_three | 2 |
| hard:technical_override | 4 |
| hard:technical_not_incomplete | 3 |
| hard:subcategory_collapse | 7 |
| hard:benign_not_negative | 8 |
| hard:negative_scope | 9 |
| hard:corrected_assessment | 2 |

Thirty of forty screening reports (75%) carry at least one `hard:*` tag.

### Assessment/MDT tags

| Tag | Count |
|---|---:|
| language:arabic | 9 |
| scan:ocr | 5 |
| hard:historical_biopsy | 5 |
| hard:imaging_not_biopsy | 10 |
| hard:malignant_in_situ | 3 |
| hard:pending_histology | 5 |
| hard:declined_assessment | 3 |
| hard:assessment_without_resolution | 3 |
| hard:benign_discordance | 2 |
| hard:malignant_not_yet_treatment | 2 |
| hard:nonattendance_contact | 4 |

Thirty-four of fifty assessment documents (68%) carry at least one `hard:*` tag.

## Hard-case families and examples

| Family | Example | Reading distinction |
|---|---|---|
| prior_category | hb01 | Historical bilateral BI-RADS 2 does not override today's negative assessment. |
| zero_over_three | hb20 | Left incomplete imaging outranks right probably benign findings. |
| technical_override | hb22 | Right technical inadequacy does not override left BI-RADS 0: label B0. |
| technical_not_incomplete | hb39 | One technically inadequate breast plus a benign opposite breast gives TR, not B0 or B2. |
| subcategory_collapse | hb31 | Right BI-RADS 4C normalizes to category 4 and outranks left category 0. |
| benign_not_negative | hb11 | Benign vascular calcification remains category 2 despite no suspicious lesion. |
| negative_scope | hb35 | No suspicious calcification does not negate a highly suspicious mass. |
| corrected_assessment | hb24 | A withdrawn preliminary overall category 2 is superseded by final category 0. |
| historical_biopsy | ha29 | A benign 2021 biopsy does not replace the awaited current core result. |
| imaging_not_biopsy | ha07 | Completed diagnostic imaging can establish routine return without any biopsy. |
| malignant_in_situ | ha19 | DCIS without invasion is still malignant pathology and can lead to treatment referral. |
| pending_histology | ha30 | Biopsy acquisition is not the same as an available histological diagnosis. |
| declined_assessment | ha33 | Administrative attendance with declined procedures is incomplete, not DNA or completed assessment. |
| assessment_without_resolution | ha36 | Clinical examination counts as assessment done, although management remains pending. |
| benign_discordance | ha39 | Benign but discordant pathology does not justify routine discharge. |
| malignant_not_yet_treatment | ha41 | Confirmed malignancy with a deferred management decision remains incomplete. |
| nonattendance_contact | ha44 | Appointment-only telephone contact does not count as attendance or clinical assessment. |

## Coverage notes

- 17 documents contain Arabic phrases.
- Nine contain deliberate scan/OCR noise.
- Eight screening reports explicitly distinguish earlier-screen categories
  from today's assessment.
- All ten B1 reports explicitly negate suspicious findings; other categories
  also contain scoped negative statements.
- Seven screening reports include 4A, 4B or 4C.
- Five assessment documents mention an earlier-year biopsy.
- Five describe a current biopsy with pathology awaited.
- Three describe attendance without assessment because the woman declined or
  could not remain.

## Limitations

The texts are template-generated and intentionally balanced rather than
population-representative. Shared phrasing, explicit episode boundaries and
clear amendments may make extraction easier than in authentic records. OCR
noise is limited; Arabic content is brief rather than full Arabic reporting.
The codes implement the requested mapping, not a billing-compliance standard.
This dataset has not undergone independent clinical validation.
