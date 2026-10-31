# Dataset Card: Abu Dhabi Synthetic Diabetic Retinopathy Screening & Eye-Clinic Benchmark

**Author:** independent author (Gemini 4 Argon, Google)  
**Generator:** `gen_dr_heldout.py` (Python 3 standard library only, deterministic seed `9091`)  
**Files Produced:** `screen_reports.json` (40 primary-care screening reports) and `clinic_notes.json` (50 hospital eye-clinic notes after referral)

---

## 1. How It Works

Running `python3 gen_dr_heldout.py` deterministically generates two JSON files in the same directory as the script:
- `screen_reports.json`: 40 synthetic diabetic eye screening reports (`hs01`–`hs40`) modelled on Abu Dhabi Ambulatory Healthcare Services (SEHA AHS) and Imperial College London Diabetes Centre (ICLDC) screening clinics using the English NHS Diabetic Eye Screening Programme (DESP) per-eye grading scheme.
- `clinic_notes.json`: 50 synthetic hospital ophthalmology / medical-retina outpatient notes (`hc01`–`hc50`) from tertiary referral centres (e.g. SSMC, CCAD, SKMC, Tawam) following a screening referral.

The script uses only Python's standard library (`json`, `os`, `random`) with a fixed seed (`9091`), makes no network or clock calls, and performs internal verification (`verify_dataset`) to ensure every record's `label` strictly satisfies the formal grading rules and all tag quotas before writing byte-identical UTF-8 JSON outputs.

---

## 2. Label Rules & Keying Decisions

### 2.1 Screening Reports (`screen_reports.json`)
Each report key contains `{"r_right", "r_left", "m_right", "m_left", "urgent", "label", "coded"}`:
- **Per-eye R grade**: `"0"` (none), `"1"` (background: microaneurysms, dot/blot haemorrhages, exudates away from macula), `"2"` (pre-proliferative: venous beading/loops, IRMA, multiple deep blot haemorrhages), `"3A"` (active proliferative: NVD/NVE, pre-retinal or vitreous haemorrhage), `"3S"` (stable treated proliferative: old PRP laser scars, regressed/fibrosed vessels, no active new vessels), `"U"` (ungradable).
- **Per-eye M grade**: `"1"` (maculopathy: exudate within 1 disc diameter (DD) of the fovea, group of macular exudates, retinal thickening, or any microaneurysm/haemorrhage within 1 DD of the fovea accompanied by visual acuity 6/12 or worse), `"0"` (no maculopathy), or `""` when the eye is `"U"`.
- **Patient-level `label` derivation**:
  1. If either eye is `"U"`: if the fellow eye is referable on its own (`R` in `{"2", "3S", "3A"}` or `M == "1"`), the patient-level grade is derived from that referable eye alone (e.g., `R2M0` + `U` $\rightarrow$ `R2M0`); otherwise `label = "U"` (e.g., `R1M0` + `U` $\rightarrow$ `U`, `R0M0` + `U` $\rightarrow$ `U`).
  2. Otherwise, `R` is the worse eye (`3A > 3S > 2 > 1 > 0`) and `M` is `"1"` if either eye is `"1"`, else `"0"`, yielding `"R" + R + "M" + M`.
  3. If `"urgent": true` (the grader explicitly requests an urgent referral) and worst `R != "3A"`, `"/urgent"` is appended (e.g. `R2M1/urgent`). When worst `R == "3A"`, the label remains `R3AM0` or `R3AM1` without `"/urgent"`.
- **Coded vs Free-text (`coded`)**: 16 of 40 reports (40.0%) originate from sites that transmit a structured per-eye grade (`"coded": label`); the remaining 24 reports have `"coded": null` and consist of free-text narrative findings.

### 2.2 Hospital Eye-Clinic Notes (`clinic_notes.json`)
Each note key contains `{"attended", "fundus", "outcome", "label"}`:
- `"attended": false` $\rightarrow$ `fundus = null`, `outcome = "none"`, `label = "dna"`.
- `"fundus": "no_view"` (patient attended and dilation/exam was attempted, but dense cataract, corneal opacity, or vitreous haemorrhage prevented seeing the retina) $\rightarrow$ `label = "not_assessable"`.
- `"fundus": "not_examined"` (patient attended and often had visual acuity or intraocular pressure measured, but the retina was not examined due to refusal to dilate/driving, acute angle-closure risk, infective conjunctivitis, vasovagal episode, or clinic timing) $\rightarrow$ `label = "not_examined"`.
- `"fundus": "examined"` $\rightarrow$ `label = "examined_" + outcome`:
  - `"examined_treated"`: intravitreal anti-VEGF/steroid injection or laser (PRP / focal) administered, initiated, or booked today.
  - `"examined_discharged"`: retina examined and patient discharged back to the annual diabetic eye screening programme.
  - `"examined_followup"`: retina examined and kept under hospital observation/review/imaging with no treatment given or booked today.

---

## 3. Dataset Counts

### 3.1 Screening Reports (`screen_reports.json` — 40 reports)

| Category / Label | Count | Share |
|---|---:|---:|
| `R0M0` | 12 | 30.0% |
| `R1M0` | 8 | 20.0% |
| `R1M1` (includes 1 unilateral `U` + `R1M1`) | 5 | 12.5% |
| `R2M0` (includes 1 unilateral `R2M0` + `U`) | 3 | 7.5% |
| `R2M1` (non-urgent) | 2 | 5.0% |
| `R3SM0` | 2 | 5.0% |
| `R3SM1` | 1 | 2.5% |
| `R2M1/urgent` (urgent flag, non-3A) | 1 | 2.5% |
| `R3AM1` (urgent flag, active PDR) | 1 | 2.5% |
| `U` (2 bilateral `U`, 1 `U`+`R0M0`, 1 `R1M0`+`U`) | 4 | 10.0% |
| **Total** | **40** | **100.0%** |

*(Summary shares: `R0M0` = 30.0%, `R1M0` = 20.0%, non-urgent referable = 32.5%, urgent (`R3AM1` + `R2M1/urgent`) = 5.0% [total referable = 37.5%], `U` = 10.0%; coded = 16/40 [40.0%]).*

### 3.2 Eye-Clinic Notes (`clinic_notes.json` — 50 notes)

| Label | Count | Share |
|---|---:|---:|
| `examined_treated` | 10 | 20.0% |
| `examined_followup` | 10 | 20.0% |
| `examined_discharged` | 10 | 20.0% |
| `not_examined` | 8 | 16.0% |
| `not_assessable` | 5 | 10.0% |
| `dna` | 7 | 14.0% |
| **Total** | **50** | **100.0%** |

### 3.3 Tag Counts Across Both Files

| Tag | `screen_reports.json` | `clinic_notes.json` | Total |
|---|---:|---:|---:|
| `coded` | 16 | 0 | 16 |
| `spelling:uk` | 28 | 32 | 60 |
| `spelling:us` | 12 | 18 | 30 |
| `arabic` | 5 | 6 | 11 |
| `ocr_noise` | 4 | 4 | 8 |
| `eye:right_left` | 17 | 20 | 37 |
| `eye:od_os` | 10 | 16 | 26 |
| `eye:re_le` | 13 | 14 | 27 |
| `eye:arabic` | 3 | 2 | 5 |
| `prev_grade_diff` | 6 | 0 | 6 |
| `negated_finding` | 8 | 0 | 8 |
| `negated_treatment` | 0 | 6 | 6 |
| `va_no_retinal_exam` | 0 | 8 | 8 |
| `hard:non_dr_pathology` | 2 | 0 | 2 |
| `hard:foveal_ma_good_va` | 2 | 0 | 2 |
| `hard:distant_exudate` | 1 | 0 | 1 |
| `hard:va_driven_m1` | 1 | 0 | 1 |
| `hard:stable_prp_3s` | 3 | 0 | 3 |
| `hard:ungradable_asymmetry` | 4 | 0 | 4 |
| `hard:historical_tx_only` | 0 | 3 | 3 |
| `hard:patient_declined_tx` | 0 | 3 | 3 |
| `hard:false_positive_referral` | 0 | 4 | 4 |
| `hard:anterior_seg_only` | 0 | 3 | 3 |
| `hard:dna_with_referral_text` | 0 | 3 | 3 |
| `layout:0` .. `layout:5` | 6–7 each (6 layouts) | 8–9 each (6 layouts) | 90 |
| **Any `hard:*` tag** | **13 (32.5%)** | **16 (32.0%)** | **29** |

---

## 4. Hard-Case Families (`hard:*`)

We designed **11 clinician-realistic hard-case families** (6 in screening, 5 in eye-clinic notes) where a naive keyword or regex matcher fails unless it models clinical context, anatomical thresholds, visual acuity rules, and temporal/attendance status:

1. **`hard:foveal_ma_good_va`** *(Screening — 2 reports: `hs16`, `hs17`)*  
   Under NHS DESP rules, a microaneurysm or haemorrhage within 1 DD of the fovea only triggers `M1` if accompanied by visual acuity of 6/12 (20/40) or worse. A naive parser seeing `"within 1 DD of the fovea"` predicts `M1`.  
   *Example (`hs16`):* `"VA: Right eye 6/6 (corrected)... two microaneurysms and a tiny dot haemorrhage located 0.5 disc diameters temporal to the foveal centre, with no exudates and no retinal thickening; because visual acuity is 6/6 (better than 6/12), M1 criteria are not met."` $\rightarrow$ `R1M0`.

2. **`hard:va_driven_m1`** *(Screening — 1 report: `hs22`)*  
   Conversely, when there are no exudates and no retinal thickening, a parafoveal microaneurysm/haemorrhage within 1 DD *does* become `M1` when visual acuity is reduced to $\le$ 6/12.  
   *Example (`hs22`):* `"VA: RE 6/18 (pinhole 6/12)... shows no hard exudates and no obvious stereo thickening, but there are three microaneurysms and a small blot haemorrhage situated 0.6 disc diameters from the centre of the fovea; combined with reduced visual acuity of 6/12, this meets NHS M1 maculopathy criteria."` $\rightarrow$ `R1M1`.

3. **`hard:distant_exudate`** *(Screening — 1 report: `hs18`)*  
   Hard exudates are a classic keyword trigger for `M1`, but isolated exudates $>1$ DD away from the fovea that do not form a macular group are graded as background retinopathy (`R1M0`).  
   *Example (`hs18`):* `"OS shows a few microaneurysms and two small, isolated hard exudates located 2.5 disc diameters superior to the fovea (well outside the 1 DD macular zone and not forming a macular group)."` $\rightarrow$ `R1M0`.

4. **`hard:non_dr_pathology`** *(Screening — 2 reports: `hs06`, `hs07`)*  
   Reports describing macular drusen (dry AMD), choroidal naevus, or hypertensive AV nipping contain macular and vascular keywords but explicitly state zero diabetic lesions (`R0M0`).  
   *Example (`hs06`):* `"Fundus photos show intermediate hard and soft macular drusen within 1 DD of the fovea in both eyes with mild RPE mottling consistent with dry age-related macular degeneration (AMD). However, there are zero microaneurysms, no retinal hemorrhages, no hard lipid exudates, and no diabetic macular edema..."` $\rightarrow$ `R0M0`.

5. **`hard:stable_prp_3s`** *(Screening — 3 reports: `hs28`, `hs29`, `hs30`)*  
   Mentions of proliferative disease and fibrosed vascular fronds at the disc can trigger `R3A` in a naive reader, whereas quiescent regressed vessels with pigmented panretinal laser scars represent `R3S`.  
   *Example (`hs28`):* `"RE shows extensive, well-pigmented panretinal photocoagulation (PRP) laser scars in 360 degrees with a fibrosed, completely regressed vascular frond at the disc; no active new vessels or haemorrhage..."` $\rightarrow$ `R3SM0`.

6. **`hard:ungradable_asymmetry`** *(Screening — 4 reports: `hs31`, `hs32`, `hs38`, `hs39`)*  
   When one eye is `"U"`, the patient label depends on whether the fellow eye is referable on its own (`R2`/`R3A`/`R3S` or `M1` overrides `U`; `R0M0` or `R1M0` stays `U`).  
   *Example (`hs39`):* `"RE is well visualised and shows only mild background retinopathy (three peripheral microaneurysms, no maculopathy... R1M0). LE is ungradable (U) due to severe asteroid hyalosis... Because the gradable RE is not referable on its own, overall report grade is U."` $\rightarrow$ `U`.

7. **`hard:historical_tx_only`** *(Clinic — 3 notes: `hc13`, `hc16`, `hc23`)*  
   Notes that document prior panretinal laser or past intravitreal anti-VEGF courses contain strong treatment keywords (`"Ranibizumab"`, `"Eylea"`, `"PRP laser"`), but today's outcome is observation (`examined_followup`) or discharge (`examined_discharged`).  
   *Example (`hc13`):* `"Past ocular history: RE underwent 3x intravitreal Ranibizumab injections in 2022 and full PRP laser in 2021. Today's dilated fundus examination shows quiescent, well-pigmented PRP scars... No active disease; no laser or injection needed today. Review in clinic in 6 months."` $\rightarrow$ `examined_followup`.

8. **`hard:patient_declined_tx`** *(Clinic — 3 notes: `hc15`, `hc19`, `hc20`)*  
   The clinician offers or recommends an intravitreal anti-VEGF injection or panretinal laser photocoagulation, but the patient declines treatment today and opts for observation only (`examined_followup`).  
   *Example (`hc15`):* `"We discussed intravitreal anti-VEGF injection for the right eye, but the patient declined any injection or laser treatment today (nothing given or booked) and requested conservative observation... Follow-up in 8 weeks for repeat OCT."` $\rightarrow$ `examined_followup`.

9. **`hard:false_positive_referral`** *(Clinic — 4 notes: `hc21`, `hc22`, `hc26`, `hc30`)*  
   The note quotes the incoming screening referral for suspected `R1M1`, `R2M0` (IRMA), or `R3AM0` (NVE), but slit-lamp biomicroscopy and OCT prove the lesion is a benign mimic (drusen, congenital loop, Gunn's dot, epiretinal membrane) and the patient is discharged (`examined_discharged`).  
   *Example (`hc30`):* `"S: Referred for query NVE right eye (R3AM0 on screening referral). O: Dilated 90D and contact lens fundus examination right and left eyes: the suspected neovascular lesion in the right eye is a benign epiretinal membrane with mild retinal vessel straightening, not neovascularisation... A/P: Discharged to annual diabetic eye screening."` $\rightarrow$ `examined_discharged`.

10. **`hard:anterior_seg_only`** *(Clinic — 3 notes: `hc31`, `hc32`, `hc35`)*  
    Slit-lamp examination and gonioscopy *are* performed on the anterior segment (and YAG laser iridotomy is even booked for narrow angles in `hc32`), but pupil dilation is withheld or refused so the **fundus** is not examined (`not_examined`).  
    *Example (`hc32`):* `"Anterior segment slit-lamp exam and gonioscopy revealed occludable narrow drainage angles... Pharmacological pupil dilation was withheld today due to acute angle-closure risk, so the retina/fundus was not examined. Booked bilateral YAG laser peripheral iridotomy first..."` $\rightarrow$ `not_examined`.

11. **`hard:dna_with_referral_text`** *(Clinic — 3 notes: `hc44`, `hc45`, `hc46`)*  
    The administrative DNA note quotes the incoming screening referral (including `"active NVD"`, `"panretinal laser photocoagulation and anti-VEGF injection"`), tempting a naive keyword reader into predicting `examined_treated` instead of `dna`.  
    *Example (`hc44`):* `"ADMINISTRATIVE ENTRY - DID NOT ATTEND (DNA). Referral text on file from DESP screening stated: 'Right eye R3AM1 with active NVD and macular exudates... urgent hospital assessment for panretinal laser photocoagulation and anti-VEGF injection.' Outcome today: Patient failed to attend..."` $\rightarrow$ `dna`.
