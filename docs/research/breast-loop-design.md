# Fourth loop type: breast screening recall to assessment outcome (design)

Status: design, 1 Nov 2026. Built on this branch; results go in `pipeline/RESULTS.md`.

**Claim tested** (same as lung and DR): one engine closes every follow-up loop. A new loop type is a rule table, two report readers and a generator, not a new pipeline. The breast loop runs through the same mock Malaffi → NATS → prep → identity linker → closure → reviewer → audit path as the other programmes, in the same integrated run.

**Scope.** What `docs/WRITEUP.md` specifies (Breast row of "Same engine, other programmes"), limited to what the engine already supports: one tracking ServiceRequest per screening mammogram, a screening report on the screen's own registration, and assessment-unit documents at another provider reached through the identity linker.

## 1. Rule: `doh2025breast` (`breastrule.go`)

A pure function from the overall BI-RADS final assessment of a screening mammogram to an action and a deadline in days from the screening date.

| Screening final assessment | Action, deadline |
|---|---|
| BI-RADS 1 (negative), 2 (benign) | no recall: return to routine screening (every 2 years) |
| BI-RADS 0 (needs additional imaging) | recall to assessment; outcome within 14 days |
| BI-RADS 3 (probably benign) | recall to assessment; outcome within 14 days |
| BI-RADS 4 (suspicious) | recall to assessment with needle biopsy; outcome within 21 days |
| BI-RADS 5 (highly suggestive of malignancy) | as 4, marked urgent |
| Technically inadequate (no category) | repeat mammogram within 21 days |
| BI-RADS 6, or anything else | unknown to the rule: hold for a human, never close |

Sources:
- **Standard:** DoH Abu Dhabi, *Standard for Breast Cancer Screening & Diagnosis*, DoH/ST/BCSD/ADPHC/V2/2025 (July 2025), text in `sources/doh/breast.txt`.
  - 3.7.4 and Appendix 6: final assessment reported as BI-RADS categories (ACR BI-RADS Atlas, 5th edition, 2013), with CPT II codes 3340F–3350F.
  - 3.8.2: BI-RADS 1/2 are discharged to routine screening; Appendix 2: mammography every two years from age 40 (average risk).
  - 3.8.3: abnormal screens are referred to a diagnostic breast assessment unit.
  - 3.9.2: triple assessment (imaging, clinical examination, image-guided needle biopsy if indicated); 3.9.8: results considered by an MDT.
  - 3.9.9–3.9.10: early recall only after assessment, never as a substitute for it; early recall rate recorded and audited.
  - 3.9.11: histologically confirmed cancer referred to treatment within 10 working days of diagnosis.
  - Appendix 3 KPI 11: screening to offered assessment ≤5 working days; assessment to results ≤5 working days; needle biopsy to result ≤5 working days. KPI 3 technical repeat rate, KPI 4 abnormal recall rate, KPI 5 early recall rate.
- **Clocks:** working days are counted as calendar days × 7/5, as for the other programmes (15 WD = 21 days). Recall: 5 WD to an offered assessment + 5 WD to results = 10 WD = **14 days** from the screening date. Suspicious or malignant (4, 5): + 5 WD for the needle biopsy result = 15 WD = **21 days**.
- **BI-RADS hierarchy** (ACR 5th edition): when the two breasts differ, the overall assessment is the more worrying one, in the order 1 < 2 < 3 < 0 < 4 < 5.
- **Our own choices (disclosed, no outside source):**
  - BI-RADS 3 on a *screening* report is treated as a recall (14 days). ACR 5th edition reserves category 3 for diagnostic work-up, and DoH 3.9.9 forbids early recall without assessment, so the assessment unit decides.
  - The technical repeat gets a 21-day clock (the 15-WD result clock of 3.7.1, since the screen has no result yet). A later gradable mammogram on the same registration decides the loop from its own day; as for DR "U", one breast inadequate does not hide a recall category in the other breast.
  - Urgency: 5 is `urgent`, 4 `suspicious`, 0 and 3 `routine`. It changes escalation wording only, not the clock.
  - BI-RADS 4A/4B/4C are read as 4.

## 2. States (`breast.go`), same gates as the other programmes

| Condition | State | Provenance |
|---|---|---|
| Free-text screening report, readers disagree or one is unavailable | open, evidence review item | `hold_decision` |
| Category agreed (or coded = text), 1 or 2 | `no_recall_needed` (routine screening recorded) | evidence → recommendation → closure_decision |
| Technically inadequate | open, repeat-mammogram clock (21 days); a later gradable screen replaces it | `hold_decision` |
| Recall, no assessment document linked | open; `overdue` after the deadline | `hold_decision` |
| Assessment document on an untrusted link (p_cal < τ) | open, link review item | `hold_decision` |
| Document linked, readers disagree | open, evidence review item | `hold_decision` |
| Readers agree: assessment documented with an outcome: return to routine, early recall, or cancer referred to treatment | `closed`, outcome and `within_deadline` recorded | … → closure_decision |
| Readers agree: attended, but assessment incomplete or the outcome pending (biopsy awaited, MDT not yet held) | stays open; a later outcome document closes it | `hold_decision` |
| Readers agree: did not attend | `no_show`; rebook outreach; a later outcome closes it | `hold_decision` |

- The three WRITEUP end-states are recorded as the outcome of a `closed` loop: `return_routine`, `early_recall` (escalation `early_recall_audit`, 3.9.10) and `to_treatment` (= WRITEUP `closed_to_treatment`; the 10-WD treatment-referral due day is recorded, not tracked). One `closed` state keeps every downstream consumer (API, pilot audit, governance) unchanged.
- An outcome `to_treatment` needs a malignant needle-biopsy result in the same document (3.9.11: histologically confirmed). Otherwise the label is indeterminate and goes to a human.
- Clinician sign-off on an evidence item stands in for reader agreement, as for the other programmes. An unread earlier assessment document blocks a decision on a later one. A document dated before the deciding screen does not count.
- Not checked by the engine: radiology–pathology concordance (3.9.8) stays with the MDT.

## 3. Readers and corpora

**Input forms:**
- Screening units that use a synoptic report (3.7.5) send the final assessment as a CPT II code (3340F–3345F, Appendix 6) with a short summary. The code is the second reader and must equal reader A's reading of the summary (the DR coded-grade pattern); it is never used alone.
- Other units send a radiologist's free-text report, read by regex and Gemma.
- The assessment / MDT document is always free text.

**Readers:**
- Reader A is regex/rules. Reader B is local Gemma 3 4B (Q4_K_M via `llama-server`), JSON-schema constrained, verbatim-quote grounded, cached once per unique text into `models/breast_gemma_cache.json` (`wsi7 breastcache`); no model calls during runs.
- Both extract fields only:
  - Screening: BI-RADS per breast (0–6, or technically inadequate), any stated overall final assessment.
  - Assessment: attended (yes/no), assessment done, needle biopsy (malignant / benign / pending / none), outcome (routine / early recall / treatment / pending).
- One shared rule turns fields into the label. Disagreement, a missing answer, an indeterminate label or a quote not found verbatim → review item.

**Corpora (synthetic, disclosed as such):**
- **Dev** (`scripts/breast_corpus.py`, seed 29): about 40 screening reports and 50 assessment documents with a labelled key, used to write the regex and the prompt.
- **Held-out**: about the same size, written by another model family through the gateway used for the earlier held-out sets (`scripts/breast_heldout/`), committed before any reader runs and scored once after both readers are frozen.
- **Traps:** a previous screen's category in the text; the worse breast decides (hierarchy 0 above 3); negation ("no suspicious mass", "no malignant calcifications"); 4A/4B/4C; "recall" wording without a category; technical inadequacy in one breast only; biopsy taken but result awaited; benign biopsy then early recall; "discussed at MDT" without an outcome; did not attend; patient declined biopsy.

**Pass gate:** 0 agreed-but-wrong labels on held-out text. The result is reported whatever it is.

## 4. Generator and cohort (`wsi7 gen -breast N`, `genbreast.go`)

**Providers:**
- `bsu-screen`: breast screening unit (mammography). Sends the screening report and the tracking ServiceRequest.
- `bau-hosp`: diagnostic breast assessment unit at a regional hospital. Sends assessment and MDT outcome documents. The woman is registered under a name variant with the CRC linkage noise model. Other women's assessment documents are distractors.

**Cohort:** women aged 40–75 (Appendix 2: average risk from 40; the upper bound is our choice) from the background pool with no other loop. Own RNG stream (seed + `breastSeedOffset`), emitted after every other programme, so `-breast 0` leaves every other feed byte-identical. A test pins this.

**Category mix (simulated):**
- Technically inadequate 2% (under the KPI 3 ceiling of 3%).
- Gradable: BI-RADS 1 62%, 2 28%, recall 10% (0: 7.5%, 3: 0.5%, 4: 1.6%, 5: 0.4%). The recall rate sits inside the 5–10% WRITEUP band and under the KPI 4 initial-screen ceiling of 15%.
- Coded final assessment: 70% of screens (simulated); never for a technical repeat.

**Assessment (simulated):** outcome documented within the deadline 60%, late 25%, never 15%. A first visit is missed (DNA) 8% of the time, then rebooked. For 4/5, 70% of first visits are "biopsy taken, results awaited, for MDT" followed by an MDT outcome 5–9 days later; for 0/3, 15%. Outcomes by category follow Appendix 6 cancer likelihoods: 0: routine 87%, early recall 5%, treatment 8%; 3: 78/20/2; 4: 55/8/37; 5: 12/3/85. Technical repeat: a repeat mammogram within 21 days in 75% of cases.

**Timing:** screens fall on days −120…90 of the [0, 120] observation window.

**Outreach:** loop-level intents as for DR (phone then letter, in the screening unit's name): `breast_assessment_due` (recall, no outcome yet, deadline within 7 days), `breast_overdue_recall`, `breast_rebook` (did not attend), `breast_repeat_mammogram` (technical repeat). Same consent hold; consent recorded by the screening unit (4% deny, own RNG in `augment_cohort.py`).

## 5. Metrics

`breastmetrics.go` writes into `report7.json` → `programmes_live.breast`:
- Closure P/R (closed + no_recall_needed) and recall-loop closure P/R; false closures with attribution.
- Triage accuracy (action, deadline, urgency), overdue-flag accuracy, state accuracy, outcome accuracy on closed recall loops.
- DoH Appendix 3, truth vs Wasl-measured: KPI 11 assessment outcome within the clock; KPI 4 abnormal recall rate; KPI 5 early recall rate.
- Reader agreement and agreed-but-wrong counts end to end.

**Pass gates:** 0 breast false closures; 0 agreed-but-wrong held-out labels (reported either way); no new wrong-patient messages or messages without consent; other programmes' no-review metrics identical with breast on and off on the same seed.

`final_aggregate.py` adds `breast_*` metrics and the safety line `breast_false_closures`; `WASL_BREAST` (default 0) in `demo.sh` and `final_eval.sh`.

## 6. Out of scope

- Real mammograms, an image reader, real assessment-unit text.
- The treatment-referral clock as its own loop (3.9.11), radiology–pathology concordance (3.9.8).
- Routine re-invitation and the 6-month early-recall mammogram (beyond the 120-day window; recorded on the loop).
- Increased-risk schedules (Appendix 2, MRI), symptomatic referrals, male breast.
