# Fifth loop type: lung cancer screening with low-dose CT and Lung-RADS follow-up (design)

Status: built on this branch, 1 Nov 2026; evaluation is synthetic/simulated only. Results go in `pipeline/RESULTS.md` (written by the coordinator).

**Claim tested** (as for lung nodules, DR and breast): one engine closes every follow-up loop. A new loop type is a rule table, two report readers and a generator, not a new pipeline. The LDCT loop runs through the same mock Malaffi → NATS → prep → identity linker → closure → reviewer → audit path as the other programmes, in the same integrated run.

**Programme id:** `lung_ldct`. **Rule id:** `acr2022lungrads`.

## 0. How this differs from the existing `lung_nodule` loop

| | `lung_nodule` (built) | `lung_ldct` (this design) |
|---|---|---|
| Who | any adult ≥35 with a nodule found by chance on a CT done for another reason | an enrolled, eligible person in the DoH lung cancer screening programme (55–75, smoking history) |
| Opening event | an incidental-finding CT report at an imaging centre | a screening LDCT at a DoH-designated lung cancer screening centre |
| Rule | Fleischner Society 2017 (size, type, count, risk → window) | ACR Lung-RADS v2022 category → interval, as required by the DoH specification (8.4.5, 8.5) |
| Scope note | Fleischner 2017 excludes lung cancer screening (MacMahon et al., Radiology 2017;284:228–243); `fleischner.go` says so | Lung-RADS is the screening system; an incidental-nodule CT is never routed here |
| Providers | `rad-imaging`, `mafraq-rad` | `lcs-centre`, `thx-hosp` (new, separate) |
| Report kinds | `lung-finding`, `lung-ct`, PET/CT, tissue | `ldct-screen`, `ldct-workup` |

The two programmes share the CT vocabulary but not a rule, a provider, a report kind or a label. A person in the LDCT cohort has no other loop (generator), so no report can be claimed by both.

## 1. Sources (verified) and what we assume

**Programme document (Abu Dhabi).** DoH, *Lung Cancer Screening Service Specifications*, PH/NCD/LCSC/SR/0.9, version 0.9, approved and effective 20 Dec 2018, Cancer Prevention and Control Section. Public at `https://www.doh.gov.ae/-/media/Feature/Resources/Standards/Lung-Cancer-Screening-Service-Specifications_Publish.ashx` (read 1 Nov 2026; not copied into the repo). Clauses used:
- 6.1.1: eligible candidates aged 55–75 with a smoking history (30 pack-years, or 20 pack-years plus one risk factor, or 20 years of shisha/dokha/medwakh); 6.3: exclusions; 10: screening stops at 76.
- 7.4–7.6: centre informs the candidate of results; negative → next screening appointment scheduled (7.5.1); positive → assessed and discussed by a multidisciplinary team (MDT) before referral to a treatment facility (7.6.1), then referral (7.6.2).
- 8.2: centre protocols may follow ACR / NCCN or equivalent updates; 8.3: every screening LDCT is double read by two radiologists; 8.4.5 and 8.5: the report carries a Lung-RADS final assessment (Appendix 8 reproduces Lung-RADS v1.0, 2014).
- 11.2: screening outcomes reported to DoH within 2 weeks of the screening date.
- Appendix 4 KPIs: abnormal recall rate <25% acceptable, <20% desirable; lung cancer detection rate 0.5–1%; retention rate >50% / >70%; diagnostic interval (LDCT to result) ≥90% within 7 working days; others not used here.
- Appendix 7: MDT composition (radiology, pulmonology, thoracic surgery, oncology, pathology).

**Rule (international).** ACR Committee on Lung-RADS, *Lung-RADS v2022* (American College of Radiology, November 2022), described in Christensen J, Prosper AE, Wu CC, et al., *ACR Lung-RADS v2022: Assessment Categories and Management Recommendations*, J Am Coll Radiol 2024;21(3):473–488, doi:10.1016/j.jacr.2023.09.009, PMID 37820837 (also Chest 2024;165(3):738–753).
- The ACR table and the JACR full text could not be fetched from this environment (access blocked). The category intervals below were checked against three sources that agree: the PubMed record above (citation, v2022 changes: stepped management, infectious findings, airway nodules, atypical cysts), the DoH Appendix 8 (Lung-RADS v1.0: same intervals for 1, 2, 3, 4A, 4B), and secondary summaries of the v2022 table (Radiopaedia "Lung-RADS"; RadioGraphics 2023 update, doi:10.1148/rg.230037, title only).

| Lung-RADS v2022 | Meaning | ACR management |
|---|---|---|
| 0 | incomplete (prior CT comparison needed, part of the lungs not evaluable, or findings suggesting infection/inflammation) | comparison with prior CT, additional imaging, and/or 1–3 month LDCT |
| 1 | negative | continue annual screening, LDCT in 12 months |
| 2 | benign appearance or behaviour | continue annual screening, LDCT in 12 months |
| 3 | probably benign | 6-month LDCT |
| 4A | suspicious | 3-month LDCT; PET/CT may be used if a solid component ≥8 mm |
| 4B | very suspicious | diagnostic chest CT with or without contrast, PET/CT and/or tissue sampling, and/or referral for clinical evaluation |
| 4X | category 3 or 4 with additional features that raise suspicion | as 4B |
| S | modifier: significant other finding | as appropriate to the finding |

v2022 notes used: follow-up intervals run from the date of the current exam; stepped management (a stable or smaller category 3 nodule is reported as 2 at follow-up, a stable 4A as 3); category 3 or 4A findings that resolve, and 4B findings proven benign, are reclassified by the most concerning remaining finding.

**Wasl assumptions (no outside source; disclosed):**
- A1. Clock for category 0: 91 days (the 3-month upper end of "1–3 month LDCT"). A later LDCT report or addendum on the same registration with a category decides the loop from its own day (as breast technical repeat).
- A2. Grace for scheduled LDCTs: due day + 30 days. Category 3: due 183 days (6 months), overdue after 213. Category 4A: due 91 days (3 months), overdue after 121. Fleischner work in this repo uses windows; Lung-RADS gives one time point, so we add one month of grace.
- A3. Clock for 4B/4X: a documented work-up outcome (MDT decision) within 30 days. Neither ACR nor DoH sets a time; 30 days is our choice. Urgency 4B/4X `urgent`, 4A `suspicious`, 0/3 `routine`; urgency changes wording only.
- A4. A 3/4A loop closes when the follow-up LDCT is read (any category except 0). The follow-up's category is recorded with its next step. A follow-up that itself needs action (3, 4A, 4B, 4X) raises the escalation `followup_needs_tracking`: the centre opens a new tracking request for that scan (the generator does), which becomes its own loop.
- A5. A work-up outcome "referred for treatment" needs malignant tissue and a documented MDT discussion in the same document (DoH 7.6.1: MDT before referral). Otherwise the label is indeterminate and goes to a human.
- A6. Category order for the overall assessment when a report lists several findings: 1 < 2 < 3 < 4A < 4B < 4X (Lung-RADS assigns the exam the category of its most concerning finding). A stated overall/final category wins. The S modifier is read and dropped: other findings belong to the incidental-findings work, not this loop.
- A7. A structured report (synoptic template) carries its Lung-RADS category as a coded value. No standard code system for Lung-RADS categories was verified, so the generator uses a placeholder system `urn:wsi:lung-rads` with text `Lung-RADS 4A`. The coded value is the second reader and must equal reader A's reading of the summary text (DR and breast pattern); it is never used alone. **Agreement is not independent full-report validation:** it shows only that the code matches reader A's reading of the summary, not that either is clinically correct; agreeing coded/text paths can both be wrong.
- A8 (post-hoc review round 1). For LR3/LR4A, the closing follow-up must be an independent LDCT report, not an addendum (`basedOn` / `focus` parent identity is carried into the scan fact), and must be dated at least 1 day after the deciding scan. The one-day minimum rejects a same-day duplicate/reread; it is not an ACR clinical interval. Addenda still complete LR0 under A1. An ineligible addendum/reread does not block a later independent follow-up.
- A9 (post-hoc review round 1). `wk_benign` requires the annual-screening outcome plus benign tissue or a documented MDT decision in the same document. PET-supported benign reclassification therefore needs the MDT field in this model; PET-only and bare "return to annual screening" text hold as indeterminate. Treatment, surveillance and annual phrases must be affirmed, not negated. This deliberately changed text-label behaviour after the original results were seen.

## 2. Rule: `acr2022lungrads` (`ldctrule.go`)

A pure function from a screening LDCT label to an action and clocks in days from the scan date.

| Label | Action | Due day | Deadline (overdue after) | Urgency |
|---|---|---|---|---|
| LR1, LR2 | `annual`: continue annual screening; next LDCT due at +365 (recorded, not tracked) | 365 | none | – |
| LR0 | `complete_ldct`: comparison / additional imaging / 1–3 month LDCT | 91 | 91 | routine |
| LR3 | `followup_ldct` | 183 | 213 | routine |
| LR4A | `followup_ldct` | 91 | 121 | suspicious |
| LR4B, LR4X | `workup`: diagnostic CT, PET/CT, tissue, clinical evaluation; MDT outcome | 30 | 30 | urgent |
| anything else | unknown to the rule: hold for a human, never close | | | |

## 3. States (`ldct.go`), the same gates as the other programmes

| Condition | State | Provenance |
|---|---|---|
| Screening report: readers disagree, one unavailable, or coded value ≠ text | open, evidence review item | `hold_decision` |
| Category agreed, LR1/LR2 | `no_recall_needed` (annual screening recorded) | evidence → recommendation → closure_decision |
| LR0 | open, completion clock 91 days; a later categorised report replaces it | `hold_decision` |
| LR3/LR4A, no later LDCT read | open; `overdue` after the deadline | `hold_decision` |
| LR3/LR4A, later independent LDCT (not an addendum or same-day reread) read with a category ≠ 0 | `closed`, outcome `followup_done`, follow-up category and its next step recorded | … → closure_decision |
| LR4B/LR4X, work-up document on an untrusted link | open, link review item | `hold_decision` |
| LR4B/LR4X, readers agree: treatment referral after MDT with malignant tissue; annual screening with benign tissue or MDT reclassification; MDT surveillance plan | `closed` with outcome `wk_treatment`, `wk_benign` or `wk_surveillance` and `within_deadline` | … → closure_decision |
| LR4B/LR4X, readers agree: attended but tissue/PET awaited or MDT pending | stays open | `hold_decision` |
| LR4B/LR4X, readers agree: did not attend | `no_show`; rebook outreach | `hold_decision` |

- Clinician sign-off on an evidence item stands in for reader agreement, as everywhere. An unread earlier document blocks a decision on a later one. A document dated before the deciding scan does not count.
- Follow-up LDCTs for 3/4A are done at the screening centre, so they sit on the scan's own registration (no identity link needed). Work-up documents for 4B/4X come from a thoracic diagnostic unit at another hospital and must cross the identity linker (calibrated p ≥ τ, EID or human).
- Escalations: `overdue_ldct_followup` / `overdue_ldct_workup`, `rebook_after_dna`, `urgent_ldct_workup_open` (4B/4X open), `followup_needs_tracking` (A4), `surveillance_after_workup` (closed with an MDT surveillance plan).
- Not checked: radiology–pathology concordance and staging (MDT); the S finding; smoking cessation (DoH 9); eligibility (DoH 6, done by the centre).

## 4. Readers and corpus

**Fields only** (both readers): screening report: overall/stated Lung-RADS category, per-finding categories, the worst finding; work-up document: attended, MDT discussed, tissue (malignant / benign / pending / none), outcome (treatment / annual / surveillance / pending). One shared rule turns fields into the label; disagreement, a missing answer, an indeterminate label or an ungrounded quote goes to review.

- Reader A: regex/rules (`ldctread.go`).
- Reader B: local Gemma 3 4B (Q4_K_M via the shared `llama-server`), JSON-schema constrained, verbatim-quote grounded, cached once per unique text into `models/ldct_gemma_cache.json` (`wsi7 ldctcache`). No model calls during runs.
- Coded path (A7): coded category = second reader, must equal reader A.

**Corpus:** `scripts/ldct_corpus.py` (seed 31), written by this work: 50 screening reports and 45 work-up documents with a labelled key. Traps: a prior exam's category; several findings with the worst deciding; a stated final category; S modifier ("Lung-RADS 2S"); 4A vs 4B vs 4X; word-only categories; negation ("no suspicious nodule"); comparison pending (category 0); infectious findings with 1–3 month LDCT; work-up with biopsy awaited; MDT pending; benign biopsy; did not attend; declined biopsy; treatment without MDT wording.

**Not a fair test.** There is no held-out corpus for this loop: authoring one with another model family was not approved for this task. Reader accuracy is measured only on the dev corpus the readers were written against, and the generator reuses dev text, so every reader and integrated-run number for this loop is optimistic.

**Built (dev corpus: 50 LDCT reports, 45 work-up documents; `pipeline/results/ldct-readers/`):**
- Reader A (regex) was iterated on this corpus until it read every text as keyed (50/50, 45/45; one fix: "referred from lung cancer screening" was being read as a cancer diagnosis).
- Reader B prompt v1: LDCT reports 49/50, work-up 36/45 (it read "interval CT in 3 months" as no decision). Prompt v2 (one revision on dev: a per-nodule category is not the overall category; a planned interval CT is surveillance; MDT only when named): reports 48/50, work-up 41/45. v2 is frozen.
- With v2, both readers agree on 48/50 reports and 41/45 work-up documents, 0 agreed-but-wrong. The 3 "treatment referral without MDT wording" traps are never agreed (reader A says indeterminate; Gemma says treatment), so they go to a human.
- Coded path: the generator's structured summaries are read by reader A as their coded category for every category (unit test, 140 texts). Coded/text agreement is not independent full-report validation and can be agreeing-but-wrong (A7).

**Post-hoc review round 1 (after the original integrated results):** A8/A9 and the wrong-kind hold/consent regressions were added. No prompt or cache revision and no model calls were needed. Replaying the cached reader-B answers through the changed shared rule (`pipeline/results/ldct-readers/v2_review1_dev.json`) leaves report results at A 50/50, B 48/50, agreement 48/50. Work-up results fall from A 45/45, B 41/45, agreement 41/45 to **A 43/45, B 39/45, agreement 39/45**, still 0 agreed-but-wrong. The two additional holds are lw10/lw16: their original keys remain `wk_benign`, but "diagnostic CT resolved; return to annual screening" has neither MDT nor benign-tissue evidence in the extracted fields, so both readers now return non-agreeing `indeterminate`. The original labels and v1/v2 score files were not rewritten. New synthetic negation/bare-annual regression cases are tests, not additions to the scored dev corpus.

## 5. Generator (`wsi7 gen -ldct N`, `genldct.go`)

**Providers:** `lcs-centre`, a DoH-designated lung cancer screening centre (screening LDCT reports, follow-up LDCTs, tracking ServiceRequests); `thx-hosp`, a thoracic diagnostic unit and lung MDT at a regional hospital (work-up documents; registered under a name variant with the CRC linkage noise model; other persons' documents are distractors).

**Cohort:** N persons aged 55–75 (DoH 6.1.1; smoking eligibility is assumed to have been checked by the centre and is not modelled), any sex, from the background pool with no other loop. Own RNG stream (seed + `ldctSeedOffset`), emitted after every other programme, so `-ldct 0` leaves every other feed and every other loop's truth byte-identical (`genldct_test.go`).

**Capacity (checked against the free person pool, seed 2026 unless stated):** each run needs N index persons + N/8 distractor persons aged 55–75 with no other loop (child loops reuse the index person).

| Cohort | Free persons 55–75 | Largest N |
|---|---|---|
| Demo (6,500 persons, 1,500 CRC, 600 cervical) | 1,151 | about 1,020 |
| Demo + 800 lung + 1,200 DR + 160 breast | 415 | about 365 |
| Frozen evaluation (8,871 held-out persons, 2,040 CRC, 816 cervical, 1,088 lung, 1,200 DR, 70 breast) | 891 (seed 37: 870) | about 770 |

The LDCT cohort is not sex-restricted, so it competes less for persons than breast does.

**Simulated parameters (assumptions):**
- Category at the index scan: LR0 2%, LR1 35%, LR2 50%, LR3 7%, LR4A 3.5%, LR4B 2%, LR4X 0.5% (recall 3/4A/4B/4X = 13%, under the DoH Appendix 4 ceiling of 25%).
- Coded category on 60% of scans (structured template).
- LR0: completed by an addendum after comparison with a prior CT (1–14 days) 40%, a 1–3 month LDCT 40%, never 20%; the completion category is drawn from the non-0 mix.
- LR3/LR4A follow-up LDCT done within the deadline 55%, late 20%, never 25% (assumption; published Lung-RADS 3–4 adherence is lower, see `docs/ROADMAP.md`). Follow-up category for LR3: 2 80%, 1 5%, 4A 10%, 4B 5%; for LR4A: 3 55%, 2 20%, 4B 20%, 4X 5%. A follow-up with category 3/4A/4B/4X gets its own tracking request (child loop, A4).
- LR4B/LR4X work-up: outcome documented within 30 days 60%, late 25%, never 15%; first visit DNA 8%; 60% of first documents are "tissue / PET awaited, for MDT" with the MDT outcome 7–14 days later. Outcome 4B: treatment 35%, benign 40%, surveillance 25%; 4X: 55/20/25.
- Index scans fall on days −300…90 of the [0, 120] observation window, so 6-month clocks can elapse inside the window.

**Outreach:** loop-level intents (phone then letter, in the screening centre's name): `ldct_followup_due` (deadline within 14 days), `ldct_followup_overdue`, `ldct_workup_rebook` (DNA), `ldct_workup_overdue`. Same consent hold; contact permission recorded by the screening centre (4% deny, own RNG in `augment_cohort.py`, seed 1010).

## 6. Metrics (`ldctmetrics.go` → `report7.json` `programmes_live.lung_ldct`)

- Closure P/R (closed + no_recall_needed), false closures with attribution; action-loop closure P/R (0/3/4A/4B/4X).
- Triage accuracy (action, deadline, urgency), overdue-flag accuracy, state accuracy, outcome accuracy on closed action loops.
- DoH Appendix 4, truth vs Wasl-measured: abnormal recall rate (index scans with 3/4A/4B/4X; acceptable <25%); lung cancer detection rate (work-up loops closed as referred for treatment per screen; DoH 0.5–1%). Wasl KPI: action loops with the recommended step documented within the clock.
- Reader agreement and agreed-but-wrong counts end to end (`two_reader`).

**Pass gates:** 0 LDCT false closures; no new wrong-patient messages or messages without consent; every other programme's outcome metrics identical with LDCT on and off on the same seed. Reader accuracy is reported but, with no held-out set, is not a gate.

**Matched-run disclosure (seed 2026; 800 lung, 1,200 DR, 160 breast; LDCT 300 vs 0):** the complete existing-programme objects in `programmes_live` and `programmes_no_review`, plus CRC/cervical closure and drop-off, CRC KPI and existing-reader objects, were exactly equal. The shared identity-candidate review queue was not identical: existing-programme review items were 1,585 with LDCT vs 1,581 without (+4 net: cervical +1, CRC −1, DR +2, lung +2; breast 0). Queue diffs included four added cross-programme link reviews and a changed pair on an existing evidence item. Wrong-signoff counts and all programme outcome/state metrics were unchanged. This small review-load effect is disclosed rather than claiming reviewer-workload parity.

**Post-hoc review-round-1 integrated rerun (same seed and byte-identical 300-index/303-loop cohort):** with review, LDCT resolved and action closure P/R stayed 1.000/1.000, with 0/278 and 0/21 false closures; state, triage and overdue accuracy stayed 1.000. Work-up reader agreement fell from 44/51 to 41/51 instances and LDCT review items rose from 3 to 5; the reviewer resolved the new holds without a false closure. Without review, resolved recall fell 0.993 → 0.986, action recall 0.905 → 0.810, state accuracy 0.993 → 0.987 and overdue accuracy 0.997 → 0.990; this is the expected cost of holding lw10/lw16 rather than auto-closing them, not a hidden metric tune. All false-closure, wrong-patient-release, released-without-granted-consent, opt-out-timing, duplicate/lost-output and forgery-effect counts stayed 0. CRC, incidental lung, DR and breast live/no-review objects stayed exactly equal to both earlier on/off runs, and all 11 non-LDCT feed files stayed byte-identical to LDCT-off. Cervical was the exception: one additional simulated wrong-link sign-off left one more truth-closed loop open (live recall 0.967 → 0.962; no-review object unchanged). Thus exact cross-programme live-outcome parity is not claimed for this rerun; the shared simulated-reviewer sequence/load effect is disclosed.

`final_aggregate.py` adds `ldct_*` metrics and the safety line `ldct_false_closures`; `WASL_LDCT` (default 0) in `demo.sh` and `final_eval.sh`.

## 7. API

`GET /api/cases?programme=lung_ldct`; each summary carries an `ldct` object (scan report, label, coded flag, rule row, due/deadline days, follow-up report and category, work-up document and outcome, within_deadline, next step). `POST /api/cases/{id}/review` accepts LDCT evidence labels (`LR0`…`LR4X`, `wk_*`) as a clinician evidence sign-off, like the other programmes.

## 8. Out of scope

- Real LDCT images or text; an image reader; Lung-RADS size measurement (the readers read the radiologist's category, they do not recompute it).
- Eligibility, consent to screening, smoking cessation, the annual rescreen beyond the 120-day window, retention rate, staging and treatment timeliness.
- The S modifier finding and the "C" prior-cancer modifier of earlier versions.
