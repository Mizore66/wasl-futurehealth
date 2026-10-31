# Third loop type: diabetic retinopathy screening to eye-clinic exam (design)

Status: approved design, 31 Oct 2026. Built on branch `dr-loop`; results go in `pipeline/RESULTS.md`.

**Claim tested** (same as the lung loop): one engine closes every follow-up loop. A new loop type is a rule table, two report readers and a generator, not a new pipeline. The DR loop runs through the same mock Malaffi → NATS → prep → identity linker → closure → reviewer → audit path as CRC, cervical and lung, in the same integrated run and the same frozen evaluation.

## 1. Rule: `doh2024dr` (`drrule.go`)

A pure function from the worst-eye screening grade to an action and a deadline in days from the screening date.

| Screening grade (worst eye) | Referable | Action, deadline |
|---|---|---|
| R3A (active proliferative), or any grade the grader marks urgent | yes | refer, eye-clinic exam within 42 days |
| R2 (moderate/severe non-proliferative) or M1 (maculopathy), any R | yes | refer, exam within 91 days |
| R3S (treated proliferative, stable) | yes | refer, exam within 91 days |
| R1M0 (mild) | no | rescreen in 12 months |
| R0M0 (none) | no | rescreen in 12 months |
| U (ungradable) | no referral yet | repeat photo within 28 days |

Sources:
- **Who is referable:** DoH Abu Dhabi, *Standard for Diagnosis and Management of Diabetes Mellitus Type 1 and 2*, DOH/ST/SDMDMT/V1/2024 (Sept 2024).
  - Clause 3.13.85: refer any macular oedema, moderate or worse non-proliferative disease, and any proliferative disease.
  - Clause 3.13.81 uses the R-grade wording and sets annual repeat exams.
- **Deadlines:** NHS Diabetic Eye Screening Programme Pathway Standards (2018): R3A seen in the hospital eye service within 6 weeks; R2/M1 within 13 weeks. DoH sets no deadline in weeks, so these are borrowed and disclosed as such.
- **Our own choices (disclosed):**
  - The 28-day repeat photo for ungradable images has no outside source.
  - R3S is treated as routine.
  - The DoH option of a 1–2 year interval is not used, because HbA1c is not in the data.
- **Rescreens:** the 12-month rescreen is recorded on the loop. No rescreen loop is opened, because the window is far beyond the 120-day observation window.

## 2. States (`dr.go`), same gates as lung

| Condition | State | Provenance |
|---|---|---|
| Free-text screening report, readers disagree or one is unavailable | open, evidence review item | `hold_decision` |
| Grade agreed (or coded), not referable | `no_referral_needed` (rescreen date recorded) | evidence → recommendation → closure_decision |
| Grade U | open, repeat-photo clock (28 days); a later gradable photo replaces it | `hold_decision` |
| Referable, no eye-clinic exam linked | open; `overdue` after the deadline | `hold_decision` |
| Eye-clinic note on an untrusted link (p_cal < τ) | open, link review item | `hold_decision` |
| Note linked, readers disagree | open, evidence review item | `hold_decision` |
| Note linked, readers agree: retina examined + outcome (treated, follow-up booked, discharged to screening) | `closed`, `within_deadline` recorded | … → closure_decision |
| Readers agree: retina not examined (vision check only) | stays open; this note cannot close the loop | `hold_decision` |
| Readers agree: not assessable (e.g. dense cataract) | `not_assessable`, clinician decides | `hold_decision` |
| Hospital appointment status no-show | open, `no_show`; rebook outreach | `hold_decision` |

Clinician sign-off on an evidence item stands in for reader agreement, as for CRC and lung.

Measure: JAWDA 5.1.4 (DoH standard §5), the share of patients with diabetes who had a retinal or dilated eye exam. It is reported as truth vs Wasl-measured, like lung appropriate follow-up.

## 3. Readers and corpora

**Input forms:**
- Some screening sites send the grade as a code (grading software or an approved AI device, DoH 3.13.82). It is used directly.
- Other sites send a grader's free-text report.
- The eye-clinic note is always free text.

**Readers:**
- Reader A is regex/rules. Reader B is local Gemma 3 4B (Q4_K_M via `llama-server`), JSON-schema constrained with verbatim-quote grounding.
- Both extract fields only:
  - Screening: R and M grade per eye, urgent flag, ungradable.
  - Clinic: retina examined (yes/no), outcome, not assessable, did not attend.
- One shared rule turns fields into the label. Gemma runs once per unique text into `models/dr_gemma_cache.json`; no model calls happen during runs.
- Disagreement, a missing answer, or a quote not found verbatim in the text → review item.

**Corpora (synthetic, disclosed as such):**
- **Dev** (`scripts/dr_corpus.py`, seed 19): about 40 screening reports and 50 clinic notes, with a labelled key. Used to write the regex and the prompt.
- **Held-out**: about the same size, written by another model family through the gateway used for CRC corpora 5–8. It is committed before any reader runs and scored once, after both readers are frozen.
- **Traps:**
  - Worst eye decides ("R1 right, R2 left").
  - Negation ("no new vessels", "no maculopathy").
  - R3S vs R3A.
  - Cataract or small pupil → U.
  - Vision check only, no fundus exam.
  - Did not attend.
  - Discharged back to screening.
  - Anti-VEGF or laser given.

**Pass gate:** 0 agreed-but-wrong labels on held-out text. The result is reported whatever it is.

## 4. Generator and cohort (`wsi7 gen -dr N`, `gendr.go`)

**Providers:**
- `phc-retina`: primary-care diabetes clinic with a retinal camera. Sends the screening report and, for referable grades, a referral ServiceRequest to ophthalmology.
- `eye-hosp`: ophthalmology department at a regional hospital. Sends the clinic note and appointment status. The patient is registered under a name variant with the CRC linkage noise model (script, typos, birth-date noise, 62/26/12% Emirates ID capture). Other diabetics' clinic notes are distractors.

**Cohort:** N = 1,200 screens per run (checked against the free person pool). Persons come only from the background pool and never overlap the lung loops. The generator has its own RNG stream (seed + `drSeedOffset`) and emits after every other programme, so `-dr 0` leaves CRC, cervical and lung byte-identical. A test pins this.

**Grade mix (simulated):**
- Ungradable: 5%.
- Gradable: R0 81%, R1 10%, R2/M1 6%, R3A 2.5%, R3S 0.5%. Any retinopathy is 19%, matching the Al-Ain survey (Al-Maskari & El-Sadig, *BMC Ophthalmology* 2007, 7:11: 19%, 95% CI 15.1–23.5%, proliferative 3.8%).
- Coded grade: 60% of screening sites (simulated).

**Attendance for referable grades (simulated):** within the deadline 50%, late 25%, never 25%. 10% no-show first, then rebook. Ungradable: a repeat photo within 28 days in 70% of cases.

**Timing:** screens fall on days −120…90 in the [0, 120] observation window.

**Outreach:** v2 templates (EN/AR) with the automation disclosure: `dr_clinic_due` and the existing `rebook_after_noshow`. Same consent hold and second-identifier guard. A DR loop messages only about its own programme's events.

## 5. Metrics and evaluation

`drmetrics.go` writes into `report7.json` → `programmes_live.diabetic_retinopathy`:
- Closure P/R.
- False closures with a Clopper–Pearson 95% upper bound.
- Triage accuracy (referable / urgent / routine / not referable / ungradable).
- Overdue-flag accuracy.
- State accuracy.
- JAWDA 5.1.4 truth vs measured.
- Review items per 100 loops.
- Reader agreement and agreed-but-wrong counts.

**Pass gates** (set before the run):
- 0 DR false closures.
- 0 agreed-but-wrong reader labels on held-out text.
- No new wrong-patient messages or messages without consent.
- No-change check: CRC, cervical and lung metrics match a run without DR on the same seed, or every difference is explained before merge.

**Frozen evaluation:** `final_eval.sh` gains `WASL_DR` (default 1,200), 5 seeds × noise v1/v2. `final_aggregate.py` adds a DR section. The origin, age-band and sex review-load checks include DR loops.

## 6. Out of scope

- Real retinal photographs or an image reader.
- Real eye-clinic text.
- Rescreen loops.
- Pregnancy schedules (DoH 3.13.84).
- Treatment-quality checks.
