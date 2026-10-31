# Synthetic Abu Dhabi diabetic-eye screening and referral notes

## Dataset card

**Author:** independent author (gpt-6.1-sol)  
**Version date:** 2026-10-31  
**Nature:** entirely synthetic; no patient records or identifying patient data.

### Purpose and scope

A small held-out-style challenge set for extracting current diabetic retinopathy
screening grades and hospital referral outcomes from clinical text. Settings are
Abu Dhabi primary-care diabetic eye screening and hospital eye clinics receiving
screening referrals.

This is a rule-based benchmark, not clinical guidance or a representative sample
of Abu Dhabi practice. The screening grades follow the specified English NHS
diabetic eye screening scheme.

## Generation and reproducibility

Save the Python block as `gen_dr_heldout.py`, then run:

    python3 gen_dr_heldout.py

It writes `screen_reports.json` and `clinic_notes.json` beside the script.
Existing files with those names are overwritten.

The generator uses Python 3 and the standard library only. It does not read
files, use a clock, access the network, or call an LLM. Fixed clinical
specifications determine the cases. A local random generator seeded with
`9137` selects exactly 16 screening records for coded-site presentation.
Layouts, spelling, bilingual content and OCR decoration use fixed schedules.

Serialization uses UTF-8, literal Unicode, fixed indentation and explicit LF
newlines. Repeated runs produce byte-identical files. Validation assertions
check labels, counts, IDs, uniqueness and several coverage requirements.

## Files and schema

Each file contains:

- `source`: the specified provenance string.
- `reports`: an ordered array of records.

Each record has `id`, `text`, `key` and `tags`. IDs are `hs01`–`hs40` for
screening and `hc01`–`hc50` for clinic notes. All 90 texts are unique.

Screening keys contain per-eye R and M grades, an explicit-request `urgent`
flag, the combined `label`, and `coded`. The latter equals the label for coded
sites and is JSON null otherwise.

Clinic keys contain `attended`, `fundus`, `outcome` and `label`.
For non-attendance, `fundus` is null. For non-examination, no view and
non-attendance, `outcome` is `none`.

## Counts per label

### Screening: 40 reports

| Label | Count |
|---|---:|
| R0M0 | 12 |
| R1M0 | 8 |
| R2M0 | 3 |
| R2M1 | 2 |
| R3AM0 | 2 |
| R3AM1 | 1 |
| R3SM0 | 2 |
| R3SM1 | 1 |
| R0M1 | 1 |
| R1M1 | 2 |
| U | 4 |
| R1M1/urgent | 1 |
| R2M0/urgent | 1 |

There are 14 non-urgent referable reports and two additional urgent referable
reports. Thus the mutually exclusive presentation buckets are 30% R0M0,
20% R1M0, 35% non-urgent referable, 10% U and 5% urgent.

### Clinic: 50 notes

| Label | Count |
|---|---:|
| examined_treated | 10 |
| examined_followup | 10 |
| examined_discharged | 10 |
| not_examined | 8 |
| not_assessable | 5 |
| dna | 7 |

Examined notes constitute 60%, with equal treatment, follow-up and discharge
shares. The remaining shares are 16%, 10% and 14%, respectively.

## Counts per tag

Counts are numbers of records carrying each tag. Tags are multi-label and
therefore do not sum to file size. A zero means the tag is absent from that file.

| Tag | Screening | Clinic |
|---|---:|---:|
| layout:L1 | 7 | 9 |
| layout:L2 | 7 | 9 |
| layout:L3 | 7 | 8 |
| layout:L4 | 7 | 8 |
| layout:L5 | 6 | 8 |
| layout:L6 | 6 | 8 |
| spelling:uk | 20 | 25 |
| spelling:us | 20 | 25 |
| format:coded | 16 | 0 |
| format:free_text | 24 | 0 |
| lang:arabic | 5 | 5 |
| noise:ocr | 4 | 4 |
| context:prior_screen | 7 | 0 |
| hard:history_vs_today | 7 | 3 |
| hard:negation | 3 | 5 |
| hard:location_and_acuity | 3 | 0 |
| hard:laterality | 3 | 0 |
| hard:ungradable_override | 6 | 0 |
| hard:activity_vs_scars | 3 | 0 |
| hard:mimic_or_comorbidity | 3 | 3 |
| hard:workflow_vs_examination | 0 | 15 |
| hard:planned_is_treated | 0 | 3 |
| hard:disposition_semantics | 0 | 4 |

At least one `hard:*` tag appears on 25/40 screening reports (62.5%) and
33/50 clinic notes (66%).

The six layouts are a report, headed assessment, correspondence, encounter
extract, service/current-encounter record, and bracketed clinical entry.

## Hard-case families and examples

| Family | Reading challenge | Example |
|---|---|---|
| history_vs_today | Separate prior grades or treatments from current findings. | `hs29`: previous R3A before laser, currently stable R3S. |
| negation | Scope absence statements without suppressing positive findings elsewhere. | `hs27`: no disc new vessels, but active new vessels elsewhere. |
| location_and_acuity | Apply location and acuity criteria rather than matching “macula.” | `hs14`: near-foveal microaneurysm with 6/9 acuity and no other qualifying finding remains M0. |
| laterality | Combine severity and maculopathy across different eyes. | `hs24`: right R2 and left M1 combine to R2M1. |
| ungradable_override | An ungradable eye does not erase an independently referable fellow eye. | `hs23`: right R2 with ungradable left eye yields R2M0. |
| activity_vs_scars | Distinguish stable treated proliferation from active new vessels. | `hs30`: left laser scars and fibrosed vessels remain R3S despite an ungradable right eye. |
| mimic_or_comorbidity | Do not equate every lesion, procedure or investigation with diabetic retinal treatment or a fundus view. | `hc40`: B-scan through dense bilateral vitreous haemorrhage does not make the fundus assessable. |
| workflow_vs_examination | Distinguish attendance, acuity checks, templates and actual retinal assessment. | `hc31`: vision and pressures recorded during attendance, but retinae not examined. |
| planned_is_treated | Apply the requested convention that booked retinal treatment counts as treated. | `hc04`: PRP booked for next week yields examined_treated. |
| disposition_semantics | Identify the destination of discharge, not merely the word. | `hc16`: discharge from the injection chair with retained hospital follow-up is examined_followup. |

## Keying decisions

### Screening

- Current findings, not historical results or referral suspicions, determine
  grades.
- R severity order is `3A > 3S > 2 > 1 > 0`.
- Maculopathy may come from either eye independently of the worse R grade.
- For an ungradable eye, M is the empty string. If the fellow eye independently
  has R2, R3A, R3S or M1, only that eye determines the combined grade; otherwise
  the combined label is U.
- The two urgent cases contain explicit grader requests. An urgent flag adds
  `/urgent` except for R3A. R3A findings alone do not set this explicit-request
  flag.
- `hs14` and `hs33` contrast the acuity-dependent near-foveal lesion criterion:
  6/9 without another qualifying finding is M0; 6/12 is M1.
- `hs32` illustrates R0M1: diabetic retinal thickening without visible
  photographic retinopathy lesions.
- Coded-site records state both per-eye grades and the combined code.
  Free-text records describe the eye findings without supplying a combined
  coded result.

### Clinic

- Non-attendance takes precedence over template or referral content.
- Attempted assessment with an obscured retinal view is `not_assessable`.
  Dilation not done or a vision-only visit is `not_examined`.
- All examined cases explicitly document bilateral retinal examination.
  All no-view cases explicitly prevent assessment of both retinae, avoiding
  an unspecified rule for mixed bilateral examination status.
- Retinal laser, PRP, anti-VEGF or intravitreal steroid given, started or
  booked counts as treated.
- Historical therapy, discussion alone, deferred unbooked injections and
  cataract surgery do not count as current retinal treatment.
- Return to screening is discharged. Continued hospital review, observation
  or imaging is followup.

## Coverage and limitations

- Ten texts contain Arabic phrases, including bilingual headers and Arabic
  eye names.
- Eight contain scan/OCR noise. Noise is deliberately restricted to
  administrative decoration rather than corrupting the diagnostic evidence.
- Both UK and US spelling appear in each file.
- Eye wording includes right/left, OD/OS, RE/LE and Arabic names.
- Seven screening reports explicitly contrast a different previous screening
  result with today's assessment.
- Many screening reports negate findings, including absence of new vessels
  and maculopathy.
- Five clinic notes explicitly negate, decline or defer current retinal
  treatment; all eight not-examined notes record vision without retinal
  examination.

The examples are intentionally compact and templated. They do not model the
full linguistic diversity, epidemiology or workflow of real clinics. Labels
and tags are supplied for evaluation; hide them from systems being tested.
Synthetic provenance does not make this a substitute for clinical validation
or an independently collected external test set.
