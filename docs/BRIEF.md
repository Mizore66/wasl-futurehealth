# FINAL BRIEF: Wasl (وصل), closing screening loops across Abu Dhabi providers

**Challenge:** #5, Closing the Screening-to-Care Loop. **Submission due 21 Nov 2026** (team registered).

**Documents:**

| Document | Use |
|---|---|
| This brief | Single source of truth for status and plan. |
| `docs/PITCH.md` | Short pitch. |
| `docs/WRITEUP.md` | Full sourced write-up (≈9.6k words; condense after the build). |
| `docs/ROADMAP.md` | Roadmap design. |

Measured results live in `pipeline/RESULTS.md`, `pipeline/v7-gate/RESULTS.md`, `booking/RESULTS.md` and `reader/RESULTS-gemma4b.md`.

**Status (23 Oct):**
- **Judging:** we stress-tested the write-up against 5 simulated judge personas (clinician, health system, technical, hackathon veteran, responsible AI) over many blind rounds. Latest mean is **47.2/60** on the official criteria.
- **What's left:** about 9–11 points per judge are build work. About 1–2.5 points are inherent limits.
- **Writing:** further writing gains are spent.

## The problem in plain words
- **The patient.** A woman has a positive stool test (FIT+) at her clinic. Her colonoscopy, if it happens, may be at another hospital, under a Latin spelling of her name, with no Emirates ID. Her clinic never sees it.
- **Local baseline.** In Abu Dhabi only **30.5%** of FIT+ patients had a colonoscopy, after a median **72 days**, against a 31-day standard (SEHA, BMC Cancer 2021).
- **Owner and KPI:**
  - Owner: the ADPHC CRC Screening Programme.
  - KPI: colonoscopy within 31 days of referral, target >90% (CRC spec 5.5.4.2, App 2).
- **The KPI is mis-measured.** In our merged simulation:

  | Tracker | Within 31 days |
  |---|---|
  | EID-only tracking | **12.4%** |
  | Truth | **33.0%** |
  | Wasl | **31.9%** |

- **Why it matters: targeting.** Under EID-only tracking:
  - 62% of real completions look lost;
  - about 1 in 4 patients on the chase list are already done.

  Reminders and navigators get spent on the wrong patients. Wasl points the standard levers at the patients who are actually lost.

## What Wasl is

### Built (merged Go codebase and runs)
- **Streaming backbone:** NATS JetStream with 13 processes and exactly-once decisions. Ingest is a FHIR R4 Subscription from a mock Malaffi.
- **Self-trained Arabic↔Latin name encoder** (2.3 MB, Go), with deterministic HNSW matching, a LightGBM scorer, and union-find with human merge/split.
- **Safety gate:** a loop auto-closes only when the patient link is trusted *and* two evidence readers agree. The readers are DoH rules plus Gemma; the local Gemma 3 4B reader is verified. Everything else goes to double human review.
- **Closure rule from the DoH CRC spec:**
  - caecum documented;
  - prep graded;
  - histology back if tissue was taken.
- **Named end-states:** normal, surveillance, MDT, specialist, repeat, declined. `suspected_malignancy` never waits for histology.
- **Patient layer:** owner-named notices, teach-back, consultation routing, and the rights to contest, opt out and get an explanation.
- **Governance:** hash-chained provenance, RBAC, a 12-item hazard log, model cards, and a DoH RAI control map (self-classified High risk).

### Simulated
- Booking engine (constraint-safe, with overbooking and rebooking).
- Network pooling.
- Cervical programme.

### Designed (roadmap)
- **Phase 2, queue control.** Raises completion instead of only measuring it:
  - direct-to-procedure booking;
  - prep-failure prevention;
  - network pooling.
- **Phase 3, one engine for every follow-up loop:** breast, lung LDCT, incidental nodules, diabetic retinopathy, IFHAS.
- **Depth:**
  - image reader (blind-first grading);
  - Learn-then-Test gate.
- **Second route:** on-premise deployment for a provider network.

## Evidence (synthetic cohorts with real Wikidata names; real US report texts)

| What | Result |
|---|---|
| Merged run: 1,500 CRC + 600 cervical loops, noisy reviewer, chaos testing | CRC precision 1.000, recall 0.960, **0/570 false closures**. 0/15,769 duplicates or losses. Tamper 17/17 detected. Forged events 24/24 denied. 0 wrong-patient sends. |
| v7 gate under the strict closure rule (ungraded prep stays open; 5 seeds × 3 reviewer error rates) | Double review: **0/8,422 false closures**, recall 0.94–0.97. No-review recall 0.273. 27 reviewer-h per 1k loops. Single review gives 21 false closures, so double review is mandatory. |
| Encoder vs off-the-shelf | R@10 0.932 vs 0.604 (best single) and 0.928 (four combined). The gain over the combination is deployability (2.3 MB vs 236 MB), not accuracy. |
| Local Gemma 3 4B hybrid reader | 40/43 agreement with the 26B; 0 wrong closures; 7.5 s per report on a laptop. |
| Booking simulator | 94.5% KPI at 1.5 slots per FIT+. The optimiser ties greedy *even when capacity is scarce* (tested), so we make no optimisation claim. Below 1 slot per FIT+, the queue collapses under any policy. |
| Network pooling, capacity misallocated across regions | ≤31 days for all FIT+ rises from 22.2% (no cross-site booking) to **42.1%** (pooled). No gain when capacity is short overall. |
| Projection from published trial effects | Day-120 completion 22.5% → 40.7% with automated levers only (+18 pp); the base case uses +13 pp. |

## Constraints (state them up front)
1. **Capacity.**
   - When Wasl succeeds, the queue fills.
   - Wasl uses misplaced capacity across a group's sites, creates capacity through prep and direct booking (roadmap), and quantifies the shortfall for DoH.
   - Below about 1 slot per FIT+ network-wide, no software helps.
2. **Who pays.**
   - Not Wasl. Providers bill the colonoscopies; the payer (Thiqa, or the insurer) funds them.
   - This is mandated care (5.5.4.2), but it is new spend of ≈AED 3M a year at the base case, partly offset by ≈AED 0.55M in savings.
   - System test: **≈AED 43k–77k per life-year**, under the WHO-CHOICE threshold (≈AED 185k).
   - Wasl's price is a placeholder pending a separate pricing study.
3. **Regulated buyer** (true of every entry). See the staged pilot plan below.

## Path to pilot (gated)

| Months | Step | Gate to proceed |
|---|---|---|
| 0–3 | Unpaid shadow pilot: 1 PHC → 1 endoscopy unit, via the Future Health pathway. DDGO assessment in parallel. | 0 false closures; recall ≥0.95 vs the coordinator's list |
| 4–9 | Live outreach, stepped-wedge across 6–8 PHCs (power 0.81–0.94) | Lower bound of the uplift >0; 0 audited false closures |
| 9–12 | Contract. Primary: ADHDS vendor agreement (precedents: Orion, VitalHub, Digi7; DoH–ADHDS–Philips AI PoC). Fallback: DoH limited tender. Second door: on-premise for a provider network (HIE 4.3.13.2.1.3). | Contract signed |
| 12+ | Cervical, then breast, then other loop types | Fixed cost (AED 0.54–1.39M year 1) falls toward ≈AED 5–28 per loop |

## Build plan to 21 Nov (2 devs; code mostly exists in `pipeline/`, `booking/`, `reader/`)

| Week | Dev A (Go/systems) | Dev B (ML/front end) |
|---|---|---|
| 24–30 Oct | **Merge** the v7 gate, booking and local 4B reader into one pipeline. Code the consent hold. | **Frozen 5-seed eval** with an independent noise generator. Hand-label the 43 reports between you and report κ. |
| 31 Oct–6 Nov | HL7 v2 MLLP adapter against a mock. Create-if-absent replay ledger. | Queue-lever arms in the booking sim (direct-to-procedure, prep-failure). Learn-then-Test gate on a held-out calibration cohort. |
| 7–13 Nov | Second loop type (lung nodules / Fleischner) on the same engine. Failover test. | Image reader on HyperKvasir (cecum + BBPS), hold-only wiring. Next.js dashboard and coordinator worklist. |
| 14–21 Nov | One-command demo, deterministic seed. Record the demo video. | Condense the write-up into the submission document, with real numbers from the build. **Submit by 21 Nov.** |

**If time is short, cut in this order:**
1. image reader;
2. second loop type;
3. Learn-then-Test.

**Never cut:** the merge, the frozen eval, the consent hold, the HL7 adapter.

## 3-minute demo
1. **0:00–0:30:** the patient story. The dashboard shows 12% under EID-only tracking vs 33% truth. "Here's the evidence behind your KPI, and who to chase."
2. **0:30–1:30:** one patient, live:
   - Arabic name at FIT+;
   - notice, then teach-back;
   - booked;
   - the report arrives as a Latin name with no EID;
   - linked; the readers agree; evidenced closure.
3. **1:30–2:15:** kill a node; 0 lost events. A reader disagreement is held; a human signs; the chain verifies.
4. **2:15–3:00:** network pooling (22% → 42%), the gated pilot plan, the honest limits.

## Honest limits (say them out loud)
- Patients, reviewers, noise and capacity are simulated. No clinician labels.
- The 43 report texts are LLM-labelled and reused. They will be replaced by a synthetic App 14 corpus.
- Thresholds were chosen after seeing eval errors. The frozen eval and the Learn-then-Test gate fix this.
- Two fairness thresholds currently fail:
  - review-load ratio 1.26 vs the 1.25 limit;
  - female-list access gap 18 pp vs the 5 pp limit.

  The remedies are designed.
- 16 messages went out before contact-consent status was known; the hold is being coded.
- Non-English notices are machine-drafted and need professional translation before the pilot.

## Top risks
1. **"It's measurement, not care."** Answer: targeting (1 in 4 on the chase list is wrong), queue control, network pooling.
2. **Synthetic data.** Answer: real names and report texts; frozen eval; we never claim outcomes.
3. **Malaffi is HL7 v2, not FHIR.** Answer: the HL7 adapter is the primary production tier.
4. **Build slips.** Answer: follow the cut order above.
