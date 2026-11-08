# Abu Dhabi Synthetic Breast Screening & Diagnostic Assessment Dataset Card

## Overview
- **Author**: independent author (Gemini 4 Argon, Google)
- **Generator**: `gen_breast_heldout.py` (Python 3 standard library only, deterministic with random seed `9377`, no external I/O or clock dependencies).
- **Files Produced**:
  - `screen_reports.json`: 40 synthetic bilateral screening mammogram reports (`hb01`–`hb40`) modelled on Abu Dhabi screening units (SEHA/SSMC, Tawam, SKMC, Cleveland Clinic Abu Dhabi, Mediclinic, NMC, Burjeel, Danat Al Emarat, and Al Dhafra mobile vans).
  - `assess_notes.json`: 50 synthetic diagnostic breast assessment unit and multidisciplinary team (MDT) documents (`ha01`–`ha50`) for women recalled from screening.

Running `python3 gen_breast_heldout.py` in its directory deterministically generates byte-identical `screen_reports.json` and `assess_notes.json`.

---

## How It Works & Keying Decisions

### 1. Screening Mammogram Reports (`screen_reports.json`)
Each report contains per-breast ACR BI-RADS 5th edition categories (`b_right`, `b_left` in `{"0", "1", "2", "3", "4", "5", "TR"}`; category 6 is never used), the explicitly stated overall assessment (`overall` or `null`), the derived `label`, and `coded` (CPT-II code or `null`).
- **Hierarchy & Technical Repeat (`TR`) Rule**:
  - If either breast is `"TR"` (technically inadequate due to positioning, motion blur, or excluded pectoral muscle): if the contralateral breast has an actionable recall/surveillance category in `{"0", "3", "4", "5"}`, `label = "B" + other_category` (e.g., `TR` + `4` $\rightarrow$ `B4`, `0` + `TR` $\rightarrow$ `B0`); otherwise (`1`, `2`, or `TR` on the other side), `label = "TR"`.
  - When neither breast is `"TR"`, `label = "B" + max(b_right, b_left)` using the strict ACR urgency ranking `1 < 2 < 3 < 0 < 4 < 5` (note that `"0"` outranks `"3"`).
  - Whenever `overall` is non-null in the text, it matches the category of `label`.
- **Coded vs. Free-Text Split**:
  - Exactly 20 reports (`hb01`–`hb20`, 50%) are structured synoptic summaries carrying CPT-II codes (`B0`: `3340F`, `B1`: `3341F`, `B2`: `3342F`, `B3`: `3343F`, `B4`: `3344F`, `B5`: `3345F`).
  - Exactly 20 reports (`hb21`–`hb40`, 50%) are free-text radiologist narratives with `coded = null` (some omit numeric BI-RADS scores or an overall score). `"TR"` labels are never coded.
- **Subcategories**: Reports mentioning `4A`, `4B`, or `4C` key that breast as `"4"` and label as `"B4"`.

### 2. Diagnostic Assessment & MDT Documents (`assess_notes.json`)
Each document records follow-up of a recalled woman with `attended` (`bool`), `assessment` (`"done"` | `"not_done"`), `biopsy` (`"malignant"` | `"benign"` | `"pending"` | `"none"`), `outcome` (`"routine"` | `"early_recall"` | `"treatment"` | `"pending"`), and `label`:
- `attended == false` $\rightarrow$ `"dna"` (always paired with `assessment="not_done"`, `biopsy="none"`, `outcome="pending"`).
- Else if `assessment == "not_done"` or `biopsy == "pending"` or `outcome == "pending"` $\rightarrow$ `"incomplete"`.
- Else if `outcome == "treatment"` and `biopsy == "malignant"` $\rightarrow$ `"to_treatment"`.
- Else if `outcome == "routine"` and `biopsy != "malignant"` $\rightarrow$ `"return_routine"`.
- Else if `outcome == "early_recall"` and `biopsy != "malignant"` $\rightarrow$ `"early_recall"`.
- **Episode Scoping**: Prior-year biopsies mentioned as clinical background do not populate `biopsy`; only needle/core/vacuum biopsies performed in the current recall episode are keyed. Both invasive carcinoma (`B5b`) and ductal carcinoma in situ (`DCIS`, `B5a`) are keyed as `biopsy = "malignant"`.

---

## Counts per Label and per Tag

### `screen_reports.json` Label Distribution (40 reports)
| Label | Count | Percentage | Target |
| :--- | ---: | ---: | ---: |
| `B1` | 10 | 25.0% | ~25.0% |
| `B2` | 8 | 20.0% | ~20.0% |
| `B0` | 8 | 20.0% | ~20.0% |
| `B3` | 2 | 5.0% | ~5.0% |
| `B4` | 6 | 15.0% | ~15.0% |
| `B5` | 3 | 7.5% | ~7.5% |
| `TR` | 3 | 7.5% | ~7.5% |
| **Total** | **40** | **100.0%** | **100.0%** |

### `assess_notes.json` Label Distribution (50 documents)
| Label | Count | Percentage | Target |
| :--- | ---: | ---: | ---: |
| `return_routine` | 13 | 26.0% | ~25.0% |
| `early_recall` | 5 | 10.0% | ~10.0% |
| `to_treatment` | 10 | 20.0% | ~20.0% |
| `incomplete` | 15 | 30.0% | ~30.0% |
| `dna` | 7 | 14.0% | ~15.0% |
| **Total** | **50** | **100.0%** | **100.0%** |

### Tag Counts Across Both Files
| Tag | `screen_reports.json` | `assess_notes.json` | Total |
| :--- | ---: | ---: | ---: |
| `coded_synoptic` | 20 | 0 | 20 |
| `free_text` | 20 | 0 | 20 |
| `layout:synoptic_seha` | 6 | 0 | 6 |
| `layout:synoptic_ccad` | 6 | 0 | 6 |
| `layout:synoptic_doh_bilingual` | 4 | 0 | 4 |
| `layout:synoptic_mediclinic` | 4 | 0 | 4 |
| `layout:narrative_seha` | 6 | 0 | 6 |
| `layout:narrative_nmc` | 5 | 0 | 5 |
| `layout:letter_burjeel` | 5 | 0 | 5 |
| `layout:pacs_dump` | 4 | 0 | 4 |
| `layout:onestop_clinic` | 0 | 12 | 12 |
| `layout:mdt_minutes` | 0 | 16 | 16 |
| `layout:us_diagnostic_report` | 0 | 9 | 9 |
| `layout:clinic_letter` | 0 | 9 | 9 |
| `layout:Arabic_bilingual_summary` | 0 | 4 | 4 |
| `spelling:UK` | 26 | 35 | 61 |
| `spelling:US` | 14 | 15 | 29 |
| `arabic` | 6 | 5 | 11 |
| `ocr_noise` | 5 | 4 | 9 |
| `negation` | 19 | 0 | 19 |
| `prior_diff_cat` | 7 | 0 | 7 |
| `subcat_4` (`4A`/`4B`/`4C`) | 5 | 0 | 5 |
| `prior_year_biopsy` | 0 | 4 | 4 |
| `biopsy_awaited` | 0 | 6 | 6 |
| `declined_or_unable` | 0 | 4 | 4 |
| **Any `hard:*` tag** | **15 (37.5%)** | **16 (32.0%)** | **31** |

---

## Hard-Case Families (`hard:<family>`)

Thirteen distinct hard-case families are included across the two files (37.5% of `screen_reports.json` and 32.0% of `assess_notes.json`):

1. **`hard:zero_outranks_three`** (ACR `0` > `3` precedence)
   - *Clinical Trap*: A naive numerical sorter treats BI-RADS 3 as higher than BI-RADS 0, whereas ACR hierarchy ranks `0` (incomplete workup needed now) above `3` (short-interval surveillance).
   - *Example (`hb12`)*: Right breast has a developing density (`BI-RADS 0`); left breast has a stable circumscribed oval mass (`BI-RADS 3`). Keyed `b_right="0"`, `b_left="3"`, `label="B0"`.

2. **`hard:tr_with_actionable_contra`** (Unilateral `TR` paired with contralateral `0`/`3`/`4`/`5`)
   - *Clinical Trap*: A keyword matcher seeing `"Technically inadequate examination (TR)"` labels the whole report `"TR"`, missing an actionable mass on the other side that mandates immediate assessment.
   - *Example (`hb18`)*: Right breast is `"TR"` due to frozen shoulder and motion blur; left breast shows a 16 mm irregular mass (`BI-RADS 4`). Keyed `b_right="TR"`, `b_left="4"`, `label="B4"`.

3. **`hard:tr_overrides_benign`** (Unilateral `TR` paired with contralateral `1` or `2`)
   - *Clinical Trap*: A reader sees `"BI-RADS 2"` on one breast and labels the report `B2`, overlooking that the contralateral breast was non-diagnostic (`TR`) and requires a technical repeat.
   - *Example (`hb38`)*: Right breast shows a benign calcified oil cyst (`BI-RADS 2`); left breast requires a technical repeat (`TR`) due to severe motion blur. Keyed `b_right="2"`, `b_left="TR"`, `label="TR"`.

4. **`hard:resolved_prior_suspicion`** (Historical BI-RADS 4/excision site now completely negative)
   - *Clinical Trap*: Mention of a prior `"BI-RADS 4A"` and biopsy clip triggers a false-positive `B4` or `B2` read, whereas complete resolution of calcifications with no tissue abnormality is scored `BI-RADS 1`.
   - *Example (`hb23`)*: Prior 2023 screen was `BI-RADS 4A` (benign columnar cell change on vacuum excision); today's mammogram is completely normal (`BI-RADS 1` bilaterally).

5. **`hard:prior_category_contrast`** (Prior screen had a different BI-RADS category from today's)
   - *Clinical Trap*: Naive extractors latch onto the first BI-RADS category mentioned in the comparison sentence rather than today's assessment.
   - *Example (`hb06`)*: Comparison states the 2023 screen was `"BI-RADS 0"`, while today's exam shows a stable right intramammary lymph node (`BI-RADS 2`) and normal left breast (`BI-RADS 1`) $\rightarrow$ `label="B2"`.

6. **`hard:postop_scar_mimic`** (Post-reduction mammoplasty architectural distortion & oil cysts)
   - *Clinical Trap*: Terms like `"architectural distortion"` and `"calcifications"` typically signal BI-RADS 4, but stable bilateral post-reduction scars and suture calcifications are benign (`BI-RADS 2`).
   - *Example (`hb08`)*: Bilateral reduction mammoplasty scars with architectural distortion and oil cysts stable since 2022 $\rightarrow$ `b_right="2"`, `b_left="2"`, `label="B2"`.

7. **`hard:family_hx_distractor`** (Explicit BI-RADS 5 / cancer in a first-degree relative)
   - *Clinical Trap*: Clinical history notes `"mother had invasive ductal carcinoma (BI-RADS 5 at diagnosis)"`, tempting a keyword reader into `B5` on a normal `B1` screen.
   - *Example (`hb03`)*: Strong family history with mother's `BI-RADS 5` diagnosis cited in history; patient's own bilateral mammogram is `BI-RADS 1`.

8. **`hard:no_numeric_birads`** (Prose-only radiologist narrative without numeric BI-RADS strings)
   - *Clinical Trap*: Regex extractors searching for `BI-RADS [0-5]` fail when a consultant radiologist writes a purely narrative letter describing normal/benign/incomplete findings.
   - *Example (`hb27`)*: Narrative letter describes a completely normal right breast and classic benign rod-like secretory calcifications and oil cysts on the left, returning the woman to routine screening (`b_right="1"`, `b_left="2"`, `overall=null`, `label="B2"`).

9. **`hard:mimic_benign_pathology`** (Sclerosing adenosis / radial scar explicitly contrasted with invasive cancer)
   - *Clinical Trap*: Pathology notes explain that florid sclerosing adenosis or a radial scar `"can radiologically and histologically mimic invasive carcinoma"`, fooling surface-level cancer detectors despite a concordant benign (`B2`) conclusion.
   - *Example (`ha08`)*: Left stereotactic biopsy shows florid sclerosing adenosis with intact p63/calponin myoepithelial layer and no malignancy $\rightarrow$ `biopsy="benign"`, `outcome="routine"`, `label="return_routine"`.

10. **`hard:dcis_is_malignant`** (Pure Ductal Carcinoma In Situ with `"no invasive carcinoma"`)
    - *Clinical Trap*: Pathology states `"There is no invasive carcinoma identified"`, which a naive negation rule misclassifies as benign, whereas `DCIS` (`B5a`) is an in-situ malignancy requiring surgical treatment.
    - *Example (`ha21`)*: Right stereotactic biopsy shows no invasion but high-grade DCIS (`B5a`) referred to surgical oncology $\rightarrow$ `biopsy="malignant"`, `outcome="treatment"`, `label="to_treatment"`.

11. **`hard:discordant_repeat_biopsy`** (Initial discordant benign/non-representative pass vs. repeat biopsy)
    - *Clinical Trap*: When a note narrates both an initial benign/normal pass (`B1`/`B2`) and a repeat biopsy (either proven malignant on repeat or awaiting repeat), a naive reader picks the first biopsy mentioned.
    - *Example (`ha22`)*: Initial pass sampled only perilesional fat (`B1`), deemed discordant; repeat ultrasound-guided core biopsy confirmed Grade 2 invasive lobular carcinoma (`B5b`) referred for surgery $\rightarrow$ `biopsy="malignant"`, `outcome="treatment"`, `label="to_treatment"`.

12. **`hard:historical_biopsy_distractor`** (Prior-year biopsy in clinical background vs. current episode)
    - *Clinical Trap*: A prior-year biopsy result (`"2021 right breast core biopsy showed benign sclerosing adenosis"`) is mistaken for this episode's biopsy when no biopsy was performed today (or when today's biopsy is still pending).
    - *Example (`ha04`)*: Mentions 2021 right core biopsy (`B2`); today's left breast recall resolved on spot views and ultrasound as an intramammary node with no biopsy taken this episode (`biopsy="none"`, `outcome="routine"`, `label="return_routine"`).

13. **`hard:attended_but_no_workup` & `hard:scheduled_plan_on_dna`** (Attendance vs. assessment completion decouplings)
    - *Clinical Trap*: In `hard:attended_but_no_workup` (`ha35`–`ha38`), the woman physically checks into the clinic (`attended=true`) but faints, panics, or declines imaging (`assessment="not_done"`, `label="incomplete"`). Conversely, in `hard:scheduled_plan_on_dna` (`ha46`, `ha48`), the letter details the planned ultrasound, core biopsy, and oncology referral before stating she Did Not Attend (`attended=false`, `assessment="not_done"`, `label="dna"`).
