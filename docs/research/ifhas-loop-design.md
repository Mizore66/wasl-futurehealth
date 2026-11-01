# Sixth loop type: IFHAS abnormal cardiometabolic results to documented follow-up (design)

Status: design, 1 Nov 2026. Built on this branch; results go in `pipeline/RESULTS.md`.

**Claim tested** (same as lung, DR and breast): one engine closes every follow-up loop. A new loop type is a rule table, readers and a generator, not a new pipeline. The IFHAS loop runs through the same mock Malaffi → NATS → prep → identity linker → closure → reviewer → audit path as the other programmes, in the same integrated run.

**What IFHAS is.** DoH Abu Dhabi's *Standard for the Provision of the Periodic Comprehensive Screening Program for Adults – IFHAS*, DoH/SD/ADPHC-CHS/IFHAS/V10/2025 (July 2025; text in `sources/doh/ifhas.txt`; [DoH](https://www.doh.gov.ae/-/media/EE758C3B77314BC394F2479AEF803AA2.ashx)). Owner: ADPHC. Eligibility: Thiqa members aged 18 and over (4.5.1–4.5.2); ADPHC describes the programme as Thiqa cardholders aged 18 to 75 at more than 150 facilities ([ADPHC, 21 Feb 2024](https://www.adphc.gov.ae/en/Media-Center/News/more-than-150-healthcare-facilities-provide-ifhas-programme-services)); the 2025 expansion added reproductive health, memory, anxiety and CBC screening ([Abu Dhabi Media Office, 1 Aug 2025](https://www.mediaoffice.abudhabi/en/health/abu-dhabi-public-health-centre-expands-comprehensive-screening-programme-ifhas-to-advance-community-health-and-preventive-care/)). Neither public page describes how abnormal results are followed up; the standard does (below).

**Scope of this loop.** The cardiometabolic part of the IFHAS "Major Package" (Appendix 7: every 3 years for ages 18–39, 40–64 and 65–75): blood pressure, BMI, random blood sugar, HbA1c and the lipid panel (Appendix 6). One loop per IFHAS screening visit. Its own cancer screens (breast, cervical, colorectal, lung) are the other loop types; the remaining IFHAS items (creatinine, CBC, vision, hearing, PHQ-9/GAD-7, MoCA, AMH, dental, DEXA, AAA) are out of scope (section 7).

## 1. What the standard says about follow-up

| Clause | Text (abridged) | Used for |
|---|---|---|
| 4.9.1 | schedule follow-up visits to give results "in the case of abnormal results, appropriate counselling and referrals"; the ordering physician communicates results, "especially if they do not show up for follow up" | the loop exists; a did-not-attend state |
| 4.9.2.1 | results communicated and documented at the follow-up visit "within 10 days of the initial visit" | the urgent clock (10 days) and the first part of the routine clock |
| 4.9.2.2–4.9.2.3 | normal: discharged to routine screening; abnormal: participant physically present | `no_followup_needed`; follow-up notes are visit documents |
| 4.9.3 | referrals "shall be made if IFHAS abnormal results noted (Appendix 8)" to licensed health professionals | the follow-up document comes from another provider, reached through the identity linker |
| 4.9.4 | providers must "receive timely and complete feedback on referrals" | what Wasl automates |
| 4.11.4.1 | the clerk "should tag the participant for IFHAS to allow tracking through Malaffi" | the tracking ServiceRequest |
| Appendix 8 | referral pathway: random blood sugar and HbA1c → GP, family physician, internal medicine, endocrinologist; lipid panel → GP, family physician, internal medicine, cardiologist; blood pressure → GP, family physician, internal medicine, cardiologist; weight → GP, family physician, internal medicine, endocrinologist | one receiving "follow-up clinic" (family / internal medicine) in the generator |
| Appendix 9 | KPIs per package: participation (>85%), abnormal findings rate, and "Referral of participants with Abnormal Findings … within 15 working days of releasing the results" | the second part of the routine clock; the KPIs Wasl reports |

**What the standard does not publish:** the thresholds that make a result "abnormal", and any deadline for the receiving clinic to see the participant. Both are filled below from cited guidelines or labelled as Wasl assumptions.

## 2. Rule: `doh2025ifhas` (`ifhasrule.go`)

A pure function from the coded results of one screening visit to one cell per domain, then to an action, an urgency and a deadline in days from the screening visit.

| Domain | Input (LOINC, UCUM) | Cell | Threshold | Source |
|---|---|---|---|---|
| glycaemia | HbA1c 4548-4 (%); random plasma glucose 2345-7 (mg/dL) or 14749-6 (mmol/L) | S severe | HbA1c > 10% or glucose ≥ 300 mg/dL (16.7 mmol/L) | ADA *Standards of Care 2025*, Section 9, rec. 9.23 ("very high": consider insulin) |
| | | D diabetes range | HbA1c ≥ 6.5% or glucose ≥ 200 mg/dL (11.1 mmol/L) | ADA 2025 Section 2, Table 2.1; diagnosis "requires two abnormal results" without unequivocal hyperglycaemia (rec. 2.1b), so the loop asks for confirmation |
| | | P prediabetes | HbA1c 5.7–6.4% | ADA 2025 Table 2.2 |
| | | N | otherwise | |
| blood pressure | systolic 8480-6, diastolic 8462-4 (mm[Hg]), office reading | S severe | ≥ 180 and/or ≥ 110 | ESH 2023 grade 3 ([ERA synopsis, Table 1](https://pmc.ncbi.nlm.nih.gov/articles/PMC11139525/)) |
| | | H hypertension range | ≥ 140 and/or ≥ 90 | ESH 2023: hypertension is defined on *repeated* office readings, so one IFHAS reading needs confirmation |
| | | N | otherwise | |
| lipids | LDL-C 13457-7 / 18262-6 / 2089-1 (mg/dL) or 22748-8 (mmol/L); triglyceride 2571-8 (mg/dL) or 14927-8 (mmol/L) | H | LDL-C ≥ 190 mg/dL (4.9 mmol/L) or triglyceride ≥ 500 mg/dL (5.6 mmol/L) | 2018 AHA/ACC multisociety cholesterol guideline (Grundy et al., [JACC 2019;73:e285](https://www.jacc.org/doi/10.1016/j.jacc.2018.11.003)): severe primary hypercholesterolaemia; severe hypertriglyceridaemia is defined on a *fasting* sample, so a random IFHAS sample needs a fasting repeat |
| | | N | otherwise | |
| weight | BMI 39156-5 (kg/m2) | O obesity | ≥ 30 | WHO adult definition ([fact sheet](https://www.who.int/news-room/fact-sheets/detail/obesity-and-overweight)) |
| | | N | otherwise | |

LOINC codes and names were checked against the NLM Clinical Tables LOINC service (1 Nov 2026): 4548-4 *Hemoglobin A1c/Hemoglobin.total in Blood*; 2345-7 *Glucose [Mass/volume] in Serum or Plasma*; 14749-6 *Glucose [Moles/volume] in Serum or Plasma*; 13457-7 *Cholesterol in LDL [Mass/volume] in Serum or Plasma by calculation*; 18262-6 *… by Direct assay*; 2089-1 *Cholesterol in LDL [Mass/volume] in Serum or Plasma*; 22748-8 *Cholesterol in LDL [Moles/volume] in Serum or Plasma*; 2571-8 *Triglyceride [Mass/volume] in Serum or Plasma*; 14927-8 *Triglyceride [Moles/volume] in Serum or Plasma*; 8480-6 *Systolic blood pressure*; 8462-4 *Diastolic blood pressure*; 39156-5 *Body mass index (BMI) [Ratio]*. Total cholesterol (2093-3) and HDL (2085-9) are printed on the panel but do not decide anything. Unit conversions: glucose mg/dL = mmol/L × 18.016; cholesterol × 38.67; triglyceride × 88.57.

**Label:** `bp=<cell>;glyc=<cell>;lipid=<cell>;weight=<cell>` (keys sorted). A domain that is missing or unreadable is `X`, and any `X` makes the label unknown to the rule: the loop holds for a human (IFHAS 4.11.4.3: an incomplete package cannot be reported).

**Decision:**

| Cells | Action | Urgency | Deadline (days from the screening visit) |
|---|---|---|---|
| no abnormal domain (N everywhere, or glycaemia P) | `routine`: no follow-up loop; rescreen in 3 years (Appendix 7); P gets lifestyle advice and a yearly HbA1c (recorded only) | – | 1095 (recorded, no overdue clock) |
| any of glycaemia D, BP H, lipids H, weight O; none severe | `follow_up` | routine | **59** = 10 (4.9.2.1 results visit) + 21 (Appendix 9: referral within 15 working days of releasing the results, × 7/5) + 28 (**Wasl assumption**: first visit at the receiving clinic within 4 weeks of referral) |
| glycaemia S or BP S | `follow_up` | urgent | **10** (4.9.2.1: the results visit). **Wasl assumption**: a severe result is confirmed or managed at that visit. For comparison only, NICE NG136 rec. 1.5.1 asks for a repeat clinic reading within 7 days at 180/120 without target-organ damage |

**Our own choices (no outside source, disclosed):**
- The receiving clinic's 28 days (no IFHAS, DoH or ADPHC deadline was found), and the urgent loop's 10 days.
- Prediabetes alone opens no loop: it is counselled at the IFHAS results visit. Random glucose 140–199 mg/dL is not an abnormal cell (no diagnostic meaning on a random sample).
- One loop per visit with one deadline per domain. The loop is overdue as soon as one abnormal domain is unresolved past its own clock (urgent domains 10 days, the rest 59).
- Severe hypertriglyceridaemia (TG ≥ 1000 mg/dL), LDL 160–189, total cholesterol and HDL do not add cells.

## 3. States (`ifhas.go`), same gates as the other programmes

The tracking ServiceRequest is opened by the screening facility for every IFHAS visit (4.11.4.1 tagging). The screening results DiagnosticReport sits on the same registration. Follow-up documents come from the receiving clinic (Appendix 8), reached through the identity linker.

| Condition | State | Provenance |
|---|---|---|
| Screening results not received, or the two screening readers disagree | open, evidence review item | `hold_decision` |
| Label unknown to the rule (any domain X) | open, evidence review item | `hold_decision` |
| No abnormal domain | `no_followup_needed` (3-year rescreen recorded) | evidence → recommendation → closure_decision |
| Abnormal, no follow-up document linked | open; `overdue` once a domain's clock has passed | `hold_decision` |
| Follow-up document on an untrusted link (p_cal < τ) | open, link review item | `hold_decision` |
| Document linked, readers disagree (or indeterminate) | open, evidence review item | `hold_decision` |
| Readers agree: did not attend | `no_show`; rebook outreach; a later document decides | `hold_decision` |
| Readers agree: some abnormal domains still pending (repeat test ordered, result awaited, confirmed without a plan, declined) | stays open | `hold_decision` |
| Every abnormal domain resolved by agreed documents: `managed` (abnormality confirmed and a management plan started: medicine, lifestyle programme, dietitian or weight clinic, specialist referral) or `not_confirmed` (repeat normal: routine screening) | `closed`, per-domain outcomes and `within_deadline` recorded | … → closure_decision |

- A follow-up document can cover several domains; each domain keeps the status of the latest agreed document that mentions it (a later "pending" re-opens a domain). Domains that were not abnormal on the screen are ignored.
- An unread earlier document blocks a decision on a later one. A document dated before the screen does not count. Clinician sign-off on an evidence item stands in for reader agreement, as for the other programmes.
- Escalations: `overdue_ifhas_followup`, `urgent_ifhas_result_open` (open loop with a severe cell), `rebook_after_dna`.

## 4. Readers

**Screening results (deterministic, no language model).** The facility sends one DiagnosticReport per visit: the printed results panel as text, and the coded results as contained FHIR Observations (LOINC + UCUM quantity). Reader A is a regex reader over the printed panel; reader B is the coded Observations, with unit conversion and plausibility ranges (HbA1c 3–20%, glucose 20–1,000 mg/dL, systolic 60–300, diastolic 30–200, LDL 10–500 mg/dL, TG 10–5,000 mg/dL, BMI 10–90). Both produce the 4-cell label; they must agree, as for the coded BI-RADS and DR grades. A coded value under the wrong unit (a glucose in mmol/L coded as mg/dL, an HbA1c in mmol/mol coded as %) fails the plausibility check, so the coded reader is unavailable and the screen goes to a human. Earlier results printed on the panel ("previous HbA1c …") are ignored by reader A.

**Follow-up documents (free text).** Reader A is regex/rules; reader B is local Gemma 3 4B (Q4_K_M via the shared `llama-server`), JSON-schema constrained, quote-grounded, cached once per unique text into `models/ifhas_gemma_cache.json` (`wsi7 ifhascache`); no model calls during runs. Both extract: attendance, and per domain one of `managed | not_confirmed | pending | not_mentioned`. One shared rule makes the label `dna`, or `dom=status;…` over the mentioned domains, or `indeterminate` (nothing mentioned, a quote not found verbatim). Disagreement or indeterminate → review item.

**Dev corpus** (`scripts/ifhas_corpus.py`, synthetic, written for this work, deterministic seed): follow-up notes for every domain and status, two- and three-domain notes, did-not-attend notes, and traps: last year's values, prediabetes on repeat ("not diabetes" + lifestyle advice is `not_confirmed`), white-coat readings on home monitoring, negation ("no hypertension"), repeat test ordered or declined, confirmed but "plan to follow", a fasting repeat for triglycerides, mmol/mol HbA1c. The corpus is written by the same author as the readers and is **not a fair test**: reader accuracy on it is dev-only. No held-out corpus by another model family is part of this branch (no paid model calls).

## 5. Generator and cohort (`wsi7 gen -ifhas N`, `genifhas.go`)

**Providers:**
- `ifh-screen`: IFHAS screening facility (`clinic`). Sends the tracking ServiceRequest and the results DiagnosticReport.
- `ifu-clinic`: family / internal medicine follow-up clinic (the Appendix 8 referral destination; `hospital` in the engine's provider kinds, i.e. downstream). The person is registered under a name variant with the CRC linkage noise model. Other persons' follow-up notes are distractors (N/8).

**Cohort:** background persons aged 18–75 with no other loop, any sex. The synthetic person pool does not model Thiqa membership, so every such person is treated as eligible (assumption). Own RNG stream (seed + `ifhasSeedOffset`), emitted after every other programme, so `-ifhas 0` leaves every other feed byte-identical (test).

**Result mix (simulated, not data):** glycaemia P 15%, D 6%, S 1%; BP H 15%, S 1%; lipids H 5%; obesity 20%; drawn independently. About 40% of visits have at least one abnormal domain, which is above the WRITEUP scenario of 10–20% referred; the scenario is about referrals, the simulation about loops.

**Coded results:** every visit; 3% of visits carry one unit-coding error (simulated) that sends the screen to review. 10% of panels print an earlier result as well.

**Follow-up (simulated):** documented within the clock 55%, late 25%, never 20%. First visit missed (DNA) 10%, rebooked 7–21 days later. Confirmation: at the first attended visit a D, H or lipid-H domain is `pending` (repeat ordered) 50% of the time, resolved 7–21 days later; S and obesity resolve at the first visit. Final status `managed` vs `not_confirmed`: glycaemia D 75/25, S 95/5; BP H 60/40 (white-coat), S 90/10; lipids 70/30; obesity 97/3. One document covers every domain discussed at a visit when the corpus has that combination, else one document per domain on the same day.

**Timing:** visits fall on days −120…90 of the [0, 120] observation window.

**Outreach:** loop-level intents as for DR and breast (phone then letter, in the screening facility's name): `ifhas_followup_due` (deadline within 7 days), `ifhas_overdue_followup`, `ifhas_rebook` (did not attend). Same consent hold; consent recorded by the screening facility (4% deny, own RNG in `augment_cohort.py`).

**Capacity:** measured in the results (section 8 of the results paragraph): how many IFHAS loops fit next to CRC, cervical, lung, DR and breast on the demo person pool.

## 6. Metrics (`ifhasmetrics.go`, `report7.json` → `programmes_live.ifhas`)

- Closure P/R (closed + no_followup_needed) and follow-up-loop closure P/R; false closures with attribution (wrong-patient document vs truth state).
- Triage accuracy (action, urgency, deadline, abnormal domain set), overdue-flag accuracy, state accuracy, per-domain outcome accuracy on closed loops.
- IFHAS Appendix 9, truth vs Wasl-measured: abnormal findings rate per visit (and per domain); follow-up documented within the clock among loops whose clock elapsed (the nearest measurable form of the 15-working-day referral KPI: Wasl sees the receiving clinic's document, not the referral letter).
- Two-reader agreement end to end for screens (coded path) and follow-up notes, with agreed-but-wrong counts.

**Pass gates:** 0 IFHAS false closures; no new wrong-patient messages or messages without consent; other programmes' metrics identical with IFHAS on and off on the same seed.

`final_aggregate.py` adds `ifhas_*` metrics and the safety line `ifhas_false_closures`; `WASL_IFHAS` (default 0) in `demo.sh` and `final_eval.sh`.

## 7. Out of scope

- Creatinine/eGFR, CBC, vision, hearing, PHQ-9/GAD-7, MoCA, reproductive health, dental, DEXA and AAA results (Appendix 6–8) and the IFHAS cancer screens (covered by the other loop types).
- Known diagnoses from the IFHAS questionnaire (Appendix 1): a person already treated for hypertension or diabetes would be followed by their own physician; the generator draws screening results without a history.
- The 10-day results visit as its own tracked step, the referral letter itself, and the 3-year rescreen (beyond the 120-day window; recorded on the loop).
- Real IFHAS data or real clinic notes.
