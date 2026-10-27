# FINAL BRIEF: Wasl (وصل), closing screening loops across Abu Dhabi providers

**Challenge:** #5, Closing the Screening-to-Care Loop. **Submission due 21 Nov 2026** (team registered).

**Documents:**

| Document | Use |
|---|---|
| This brief | Single source of truth for status and plan. |
| `docs/PITCH.md` | Short pitch. |
| `docs/WRITEUP.md` | Full sourced write-up (≈9.6k words; condense after the build). |
| `docs/ROADMAP.md` | Roadmap design. |

Measured results live in `pipeline/RESULTS.md` (stages 0–7, LTT, image reader, lung nodules), `pipeline/results/final/SUMMARY.md` (frozen evaluation), `booking/RESULTS.md`, `image/RESULTS.md`, `reader/RESULTS-gemma4b.md` and `docs/research/v7-gate-offline-results.md`.

**Status (28 Oct):**
- **Backend:** all 8 handoff stages are built and pushed (one-command demo, v7 gate, live 4B reader, consent hold, HL7 v2, API, booking, frozen 5-seed eval). The six roadmap depth items are built too: Learn-then-Test gate, image reader, lung-nodule loop, direct-to-procedure and prep-risk levers. The encoder retrain on real name pairs is finishing.
- **Left:** human double-labelling of the 43 reports (both team members), the Next.js frontend (Kai), the demo video, and condensing the write-up.
- **Judging (23 Oct, before the build):** 5 simulated judge personas over many blind rounds; latest mean **47.2/60**, with about 9–11 points per judge attributed to build work that is now done.

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
  | Wasl | **32.0%** |

- **Why it matters: targeting.** Under EID-only tracking:
  - 62% of real completions look lost;
  - about 1 in 4 patients on the chase list are already done.

  Reminders and navigators get spent on the wrong patients. Wasl points the standard levers at the patients who are actually lost.

## What Wasl is

### Built (one Go codebase; every item below ran in an integrated NATS run with chaos testing)
- **Streaming backbone:** NATS JetStream, exactly-once decisions. Two ingest paths: a FHIR R4 Subscription from a mock Malaffi, and **HL7 v2.5.1 over MLLP** (the production-shaped tier).
- **Self-trained Arabic↔Latin name encoder** (2.3 MB, Go), deterministic HNSW matching, a LightGBM scorer, stratified calibration, union-find with human merge/split.
- **Safety gate (v7strict):** a loop auto-closes only when the patient link is trusted *and* two evidence readers agree field by field (DoH rules plus Gemma; the local Gemma 3 4B runs live). Messages also need a second identifier (exact DOB) or a human confirmation. Everything else goes to double human review.
- **Learn-then-Test certification** of the closure tier on a separate calibration split (`-policy v7ltt`).
- **Hold-only image reader:** caecal-landmark and bowel-prep photos can hold a loop or route it to a one-click endoscopist grade, never close it.
- **Consent hold:** no message is released until contact permission is granted; opt-outs cancel queued messages.
- **Closure rule from the DoH CRC spec:** caecum documented, prep graded, histology back if tissue was taken. Named end-states; `suspected_malignancy` never waits for histology.
- **Second loop type:** incidental lung nodules (Fleischner 2017) on the same engine.
- **API for the frontend:** materialised run, RBAC, append-only hash-chained review ledger, three deterministic demo fixtures.
- **Patient layer:** owner-named notices, teach-back, consultation routing, and the rights to contest, opt out and get an explanation.
- **Governance:** hash-chained provenance, tamper detectors, RBAC, a 12-item hazard log, model cards, and a DoH RAI control map (self-classified High risk).

### Simulated
- Booking engine as decision support (constraint-safe, overbooking, rebooking), with per-case suggestions.
- Network pooling.
- Queue levers: direct-to-procedure (DoH App 9 checklist) and prep-risk targeting (Dik 2015).
- Cervical programme.

### Designed (roadmap)
- **Phase 3, more loop types:** breast, lung LDCT, diabetic retinopathy, IFHAS.
- **Second route:** on-premise deployment for a provider network.
- Multi-period booking lookahead (the current optimiser ties greedy).

## Evidence (synthetic cohorts with real Wikidata names; real US report texts)

| What | Result |
|---|---|
| Integrated baseline run: 1,500 CRC + 600 cervical loops, noisy reviewer, process + NATS kills | CRC precision 1.000, recall 0.961, **0/571 false closures**. 0 duplicates or losses. Tamper 17/17 detected. Forged events 0/24 with effect. 0 wrong-patient sends. |
| **Frozen final evaluation**: 5 seeds × 2 noise models; v2 is an independent corruption model never used for training or tuning | Held-out v2 noise: CRC P 1.000, R 0.864 (0.855–0.872), **0/2,473 false closures** (95% UB 0.15%), 0/3,720 wrong-patient messages, 0/2,652 sent without consent, KPI error 0.038 vs EID-only 0.201. Dev noise: R 0.954. |
| Learn-then-Test gate, 12,600 calibration loops (cal split, v2 noise) | τ_close 0.98 certified with 0 false closures and 0 wrong-patient messages: P(both rates ≤ 0.1%) ≥ 95% for loops like the calibration set. |
| Consent hold | 0 messages released without granted permission (stage 3: 0/619 released; 373 held until the grant). |
| HL7 v2 over MLLP, with connection drops and duplicate re-sends | 30,668/30,668 resources delivered exactly once; 888 duplicate re-sends absorbed; CRC 0/574 false closures. |
| Local Gemma 3 4B reader, live in the integrated run | CRC P 1.000, R 0.955, 0/567 false closures; histology question 43/43. |
| v7 gate offline sweep (5 seeds × 3 reviewer error rates) | Double review: **0/8,422 false closures**. Single review gives 21, so double review is mandatory. |
| Image reader (HyperKvasir, grouped held-out split) | Caecum AUC 0.998; prep AUC 0.991 (specificity 0.83, so "adequate" only routes to a human). Integrated: 0/571 false closures, 0 upgrades. |
| Lung-nodule loop, 800 loops | P 1.000, R 0.993, 0/297 false closures, recommendation accuracy 1.000. |
| Encoder vs off-the-shelf | R@10 0.932 vs 0.604 (best single) and 0.928 (four combined). The gain over the combination is deployability (2.3 MB vs 236 MB), not accuracy. |
| Booking simulator | 94.5% KPI at 1.5 slots per FIT+. The optimiser ties greedy even when capacity is scarce, so we make no optimisation claim. Below 1 slot per FIT+, the queue collapses under any policy. |
| Queue levers (simulated) | At 1.2 slots per FIT+: direct-to-procedure plus prep targeting +13 pp ≤31-day completion (93% of draws better). Prep targeting frees about 88 repeat colonoscopies per 1,000. No lever helps below 1.0 slot. |
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

## Plan to 21 Nov

| Item | Owner | Status |
|---|---|---|
| Backend stages 0–7 and the six depth items | Anas (backend) | Done |
| Encoder retrain on real name pairs | Anas | Finishing |
| Re-run the frozen 5-seed evaluation on the final code | Anas | After the encoder lands |
| Next.js dashboard and coordinator worklist on `docs/API.md` | Kai (frontend) | In progress |
| Human double-labelling of the 43 reports; report κ (`docs/labelling/`) | Both | Not started |
| Record the 3-minute demo video | Both | After the frontend |
| Condense the write-up into the submission document | Both | After the final evaluation |

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
- The 43 report texts carry an LLM-produced answer key until the human double-labelling is done. The lung CT reports are synthetic.
- The Learn-then-Test guarantee covers loops like the synthetic calibration set, not real Abu Dhabi data; a pilot must recalibrate on locally adjudicated loops (≈3.7k).
- The image reader was trained on Norwegian video frames; UAE stills need local recalibration. Its weights inherit ImageNet's non-commercial terms.
- Queue-lever effect sizes are borrowed from published trials; App 9 fields are synthetic.
- Female-list access gap (18 pp vs the 5 pp limit) is not yet re-tested on the current booking engine. The review-load ratio across name origins now passes (1.14 vs 1.25 on held-out noise).
- Non-English notices are machine-drafted and need professional translation before the pilot.

## Top risks
1. **"It's measurement, not care."** Answer: targeting (1 in 4 on the chase list is wrong), queue control, network pooling.
2. **Synthetic data.** Answer: real names and report texts; frozen eval; we never claim outcomes.
3. **Malaffi is HL7 v2, not FHIR.** Answer: the HL7 v2 MLLP adapter is built and tested under drops and duplicates.
4. **"Your thresholds were tuned on your test data."** Answer: frozen evaluation on an independent noise model, and a Learn-then-Test certificate on a separate calibration split.
