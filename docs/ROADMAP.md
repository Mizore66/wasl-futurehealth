# Wasl v2: breaking the "limits of the idea"

Written 2026-10-23. The research is in `queue-levers.md`, `scope-routes.md` and `tech-depth.md`.

**What v1 was:** an evidence-backed tracker. **What v2 adds:** it also controls the queue, runs one engine for every follow-up loop, and has more than one buyer. Each section names the limit it breaks, the move, what kill-testing found, and what the build involves.

---

## L1. "Measurement doesn't raise completion; capacity binds" → Wasl manages the queue

Verdicts from kill-testing:

| Lever | Verdict | Evidence | How v2 does it |
|---|---|---|---|
| **Direct-to-procedure** | KEEP | 36.3% of Abu Dhabi FIT+ non-completers were "referred to a gastroenterologist but did not attend" (SEHA, PMC8343888 T5), the largest single local leak. The DoH spec needs only the referral form (5.5.1) and the App 9 pre-colonoscopy assessment, not a GI consult. Straight-to-test cut time to first test from 22 to 13 days (Banerjea 2017), and direct access was non-inferior (62.7% vs 57.1%). | A nurse completes the App 9 checklist (anticoagulants, diabetes and insulin, devices, prior GI surgery, ASA class) from Malaffi data plus one call. ASA ≥3 or anticoagulation routes to a clinic; everyone else is booked straight into a colonoscopy slot. Navigation is part of the same flow, because unnavigated direct access was *slower* in one US system (HR 0.59). |
| **Prep-failure prevention** | KEEP: the only lever that *adds* capacity | Per 1,000 adequate exams, lowering inadequate prep from 15% to 10% frees 62 slots; from 20% to 5%, 197. SMS raises adequate prep (RR 1.10, 7 RCTs) and cuts no-shows (RR 0.74). Model-targeted extra prep for high-risk patients: 94% vs 49% adequate. The Dik score has AUC 0.77 internally and 0.62–0.69 externally. DoH App 9.5 caps inadequate prep at 10%. | Predict prep risk from Malaffi data: diabetes, opioids or tricyclics, constipation, prior inadequate prep, ASA. Low-risk patients get an SMS sequence; high-risk patients get a nurse call plus a split or extended regimen, with the clinician's order. Slots recovered and per-unit inadequate-prep rate become dashboard KPIs. |
| **Pooled regional queue** | KEEP where capacity is uneven | Ontario's central positive-FIT booking cut the mean wait from 99 to 44 days and raised the share seen within 56 days from 43% to 92% (confounded by a test switch). A systematic review found 5–47 days gained in mandatory models. | One emirate-level queue fills any unit's free slot within region and preference constraints. DoH spec 5.4.1 already encourages an online booking system. This needs per-unit slot feeds, which are the pilot's data dependency and are buyer-held, so out of scope for us. |
| f-Hb triage | WEAK (tie-breaker only) | Cancer PPV rises from 14 to 90 per 1,000 between f-Hb 20–39 and ≥180 µg/g. But no screening programme schedules by f-Hb, and stage harm only appears after more than 10 months of delay. | Used only to order patients *within* the 31-day window. The spec requires labs to record the haemoglobin value (4.13.3). |

**What changes in the impact claim:**
- v1 said "the patient levers project +5.3 pp at ≤31 days".
- v2 adds the queue levers: removing the consult step, recovering prep-failure slots and pooling. Together these raise both the 31-day KPI and total completion.
- The booking simulator already models booking-at-acceptance. v2 adds direct-to-procedure (removes the consult leak) and recovered capacity (+6–20% slots) as simulated arms. The build will report the honest number.

## L2. "Thin net cash; costs more than manual tracing at low volume" → one engine for every loop

Published follow-up failure rates are high across loop types:
- incidental lung nodules: 38% get appropriate follow-up;
- adrenal incidentalomas: 34% imaged;
- abnormal labs: 6.8–62% not followed up;
- diabetic-retinopathy referral: 55–69% attended;
- Lung-RADS 3–4 adherence: 27.6%.

**Rough emirate volume [A]:**

| Loop | Loops a year |
|---|---|
| Incidental imaging | 10–30k |
| Diabetic retinopathy | 6–12k |
| IFHAS abnormal results | 30–60k (weakest estimate) |
| Breast recall | 2–10k |
| Lung LDCT | <0.5k |
| CRC + cervical | 2–6k |
| **Total** | **≈50–110k** (25–60× CRC alone) |

**Per-loop cost:** AED 0.44–1.39M ÷ 50–110k ≈ **AED 4–28 per loop**, below manual tracing (AED 10–91).

**Engine fit:** the same linkage, gate, two-reader evidence and owner/SLA machinery. Each new loop needs only:
- a closure rule (from its DoH standard or ACR follow-up guidance);
- an evidence schema;
- clocks.

**Order of rollout:**
1. CRC and cervical (built).
2. Breast (specified).
3. Lung LDCT. A DoH programme exists, and Lung-RADS gives explicit intervals.
4. Incidental lung nodules: Fleischner intervals, and the highest volume.
5. Diabetic retinopathy: SEHA already runs AI reading at 11 centres.
6. IFHAS abnormal results.

**Competition:** no incidental-finding tracker (PowerScribe Follow-up Manager, Eon, Rad AI Continuity) has a public UAE deployment. Philips AI Manager, embedded in Abu Dhabi radiology through DoH/ADHDS, *detects* findings but doesn't track follow-up. So Wasl is the downstream closure layer for Philips' detections: a partner, not a competitor.

## L3. "One gatekeeper (ADHDS), 6–9 months, no startup precedent" → three doors

1. **ADHDS/ADPHC module (primary, programme-wide):** as in v1.
2. **Provider network, on-premise:**
   - SEHA/PureHealth (14 hospitals, 70+ clinics, 3.5M+ patients) or M42 tracking its *own* patients counts as "related Healthcare Operations" under HIE 4.3.13.2.1.3, which includes "quality and improvement assessments".
   - Constraints:
     - no cloud storage (4.3.13.2.2.9);
     - no copying ADHIE data into its own system (4.3.13.2.2.11);
     - no third-party access (4.3.13.2.2.4–5).
   - So this door means Wasl deployed **on-premise as the network's processor**, holding pointers and decisions, not copies.
   - It needs no Malaffi change order, and the network's own EMR feeds suffice for intra-network loops.
3. **Payer signal (supporting, not a buyer yet):**
   - Thiqa claims carry mandatory CPT codes, so a colonoscopy claim is *independent evidence the procedure happened*. It reopens or escalates loops, but never closes them, because claims carry no caecum, prep or histology data.
   - DoH's Pay-for-Quality standard (June 2025) covers Thiqa facilities and TPAs, which is a future incentive hook.

**Other emirates:** NABIDH (Dubai) and Riayati (federal) are expansion markets with their own gatekeepers. No loop-tracking product was found on either.

## L4. "Encoder ties the ensemble; optimiser ties greedy; reader is just extraction" → depth where it pays

1. **Image evidence reader (third reader; can hold a loop, never close one):**
   - DoH App 11 already requires caecal-landmark photos.
   - Train on HyperKvasir (CC BY 4.0): cecum 1,009, retroflex 391, bbps-0-1 646, bbps-2-3 1,148 images; ≈625 MB per class from OSF. Ileum has only 9 images, so it is unusable.
   - Published results: ICV AUC 0.98, AO 0.95; BBPS ≈92%.
   - **Job:** turn "prep ungraded" into "photos suggest adequate → one-click endoscopist grade", and flag missing caecal photos.
   - This **recovers part of the 0.766 → 0.273 auto-closure drop without breaking the DoH rule**, because a human still grades.
   - Domain shift (Norwegian video frames vs UAE stills) means local recalibration in the pilot.
   - This makes Wasl multimodal: text + images + structured data.
2. **Learn-then-Test gate (replaces hand-set thresholds):**
   - Gives a finite-sample guarantee: P(false-closure rate ≤ α) ≥ 1−δ. It runs separately for wrong-patient and false-adequate risks, with Bonferroni over δ.
   - At α = 0.001, δ = 0.05 it needs 2,995 held-out calibration loops with 0 errors, or 4,742 with 1.
   - It answers "rules were chosen after seeing eval errors" with a formal guarantee.
   - No published application to record linkage was found, so this is a novel contribution.
3. **Optimiser: run, and still tied. The claim is dropped.**
   - **Experiment:** capacity sweep on the existing simulator, 40 draws at 0.8 / 1.0 / 1.2 slots per FIT+, plus a scarce-female-list variant (10% of lists).
   - **Result:** the daily ILP beats greedy by ≤0.5 pp on ≤31-day completion at every capacity level (e.g. 1.0 slot per FIT+: 13.7% vs 13.1%, winning 23 of 40 draws; with scarce female lists, 15.7% vs 13.3%, winning 29 of 40). The intervals all cross 0.
   - **Why:** the engine solves each day's batch myopically. Published 12–16% gains come from *multi-period lookahead* (two-stage stochastic or ADP), which we don't have.
   - **v2 decision:**
     - Stop claiming optimisation as depth. Booking is "constraint-safe".
     - The depth budget moves to the image reader and the LTT gate.
     - Lookahead is a later research item, not a pitch claim.
   - **Second finding:** below 1.0 slot per FIT+ the queue collapses (≤31-day completion ≈0–14%) whatever the booking policy. Only capacity-creating levers move that regime: prep-failure prevention and removing the consult visit.
4. **Encoder upgrade:**
   - Retrain on *real* Arabic↔Latin person pairs: JRC-Names (~19.5k Arabic persons) and ANETAC (79,924 pairs), with self-mined hard negatives (same-surname collisions). Evaluate on held-out real variants.
   - Fixes "trained on its own noise model" and keeps the 2.3 MB Go footprint.
   - Licences need checking before use.

## L5. "Patient uplift borrowed from US trials"
This can't be fixed by design. The stepped-wedge pilot (6–8 PHCs, power 0.81–0.94) measures it locally. v2 makes this the pilot's primary endpoint.

---

## What stays a limit after v2
- Capacity still caps the absolute ceiling. v2 *creates* some slots (prep) and *reallocates* others (pooling), but DoH funds endoscopy lists.
- Every door still runs through a regulated buyer; there is no consumer route.
- All volumes for wider loops are estimates [A] until registry queries.

## Build impact (for the plan)

| Item | Effort [A] | Order |
|---|---|---|
| Merge v7 gate + booking + 4B reader; frozen 5-seed eval | 1 week | 1 |
| Direct-to-procedure + prep-risk arms in the booking sim | 2–3 days | 2 |
| Learn-then-Test gate on a held-out synthetic calibration cohort | 2 days | 3 |
| Image reader: HyperKvasir cecum + BBPS classifier, held-out eval, hold-only wiring | 3–4 days | 4 |
| Second loop type: lung-nodule (Fleischner) rules + evidence schema, to prove "one engine" | 2–3 days | 5 |
| HL7 v2 adapter, consent hold, App 14 corpus, failover test | 1 week | 6 |
| Encoder retrain on real pairs (after licence check) | 2 days | 7 |
