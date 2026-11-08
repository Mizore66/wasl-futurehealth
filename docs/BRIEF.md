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

**Status (2 Nov):**
- **Backend:** all 8 handoff stages and all 20 agents2 items are built and integrated locally; nothing has been pushed. This includes the one-command demo, v7 gate, live 4B reader, consent hold, HL7 v2, API, booking, frozen 5-seed eval, Learn-then-Test gate, image reader, five additional loop types, deployment and adversarial/scale tests. The encoder retrain is built and merged. X10/B8 hardening and fresh scale/B8 validation are complete; the final-code ten-lane frozen evaluation is complete at decision code `075abc5`. The seven-programme API samples have been refreshed.
- **Left:** the Next.js frontend (Kai) and the demo video. The 43 reports now have clinically verified model-assisted labels (κ = 1.000 after clinician adjudication), not independent human double-labelling. The submission draft and recording guide are prepared.
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
  | Wasl | **31.9%** |

  Seed-2026 simulated run (Wasl 1.1 points from truth). Frozen five-seed evaluation (`pipeline/results/final/SUMMARY.md`): Wasl gap about 1 point under v1 noise, 3.1 points under independent v2 noise; exact values vary by run.

- **Why it matters: targeting.** Under EID-only tracking:
  - 62% of real completions look lost;
  - about 1 in 4 patients on the chase list are already done.

  Reminders and navigators get spent on the wrong patients. Wasl points the standard levers at the patients who are actually lost.

## What Wasl is

### Built (one Go codebase; every item below ran in an integrated NATS run with chaos testing)
- **Streaming backbone:** NATS JetStream, exactly-once decisions. Two ingest paths: a FHIR R4 Subscription from a mock Malaffi, and **HL7 v2.5.1 over MLLP** (the production-shaped tier).
- **Self-trained Arabic↔Latin name encoder** (2.3 MB, Go), deterministic HNSW matching, a LightGBM scorer, stratified calibration, union-find with human merge/split.
- **Safety gate (v7strict, closure tier 0.99, above the 0.989 cap on name-only links):** a loop auto-closes only when the patient link is trusted *and* two evidence readers agree field by field (DoH rules plus Gemma; the local Gemma 3 4B runs live). Messages also need a second identifier (an exact DOB, or a matching Emirates ID with agreeing name, sex and DOB) or a human confirmation. Everything else goes to double human review.
- **Learn-then-Test certification** of the closure tier on a separate calibration split (`-policy v7ltt`).
- **Hold-only image reader:** caecal-landmark and bowel-prep photos can hold a loop or route it to a one-click endoscopist grade, never close it.
- **Consent hold:** no message is released until contact permission is granted; opt-outs cancel queued messages.
- **Closure rule from the DoH CRC spec:** caecum documented, prep graded, histology back if tissue was taken. Named end-states; `suspected_malignancy` never waits for histology.
- **Seven loop types:** incidental lung nodules (Fleischner 2017), diabetic retinopathy screening (DoH 2024 referral criteria, NHS DESP deadlines), breast screening recall (DoH 2025, BI-RADS), lung cancer screening LDCT (ACR 2022 Lung-RADS) and IFHAS abnormal-result follow-up (DoH 2025) on the same engine as CRC and cervical.
- **API for the frontend:** materialised run, RBAC, append-only hash-chained review ledger, three deterministic demo fixtures.
- **Patient layer:** owner-named notices, teach-back, consultation routing, and the rights to contest, opt out and get an explanation.
- **Governance:** hash-chained provenance, tamper detectors, RBAC, a 12-item hazard log, model cards, and a DoH RAI control map (self-classified High risk).

### Simulated
- Booking engine as decision support (constraint-safe, overbooking, rebooking), with per-case suggestions.
- Network pooling.
- Queue levers: direct-to-procedure (DoH App 9 checklist) and prep-risk targeting (Dik 2015).
- Cervical programme.

### Designed (roadmap)
- **Phase 3, additional loop types:** the seven current programmes share the engine, but every new clinical rule set still needs local validation.
- **Second route:** on-premise deployment for a provider network.
- Multi-period booking lookahead (the current optimiser ties greedy).

## Evidence (synthetic cohorts with real Wikidata names; real US report texts)

| What | Result |
|---|---|
| Integrated baseline run: 1,500 CRC + 600 cervical loops, noisy reviewer, process + NATS kills | CRC precision 1.000, recall 0.948, **0/563 false closures**. 0 duplicates or losses. Tamper 17/17 detected. Forged events 0/24 with effect. 0 wrong-patient sends. |
| **Frozen final evaluation**: 5 seeds × 2 noise models; v2 is an independent corruption model never used for training or tuning; 6,314 index loops per run across seven programme types from every held-out person | Held-out v2 noise (code `075abc5`): CRC P 1.000, R 0.785 (0.772–0.798), **0/3,035 false closures**; cervical 0/1,303, lung 0/1,907 (surveillance CT 0/32), DR 0/5,627 (referral loops 0/233, referral recall 0.894), breast 0/338 (recall loops 0/23), LDCT 0/1,413 (action loops 0/121, recall 0.981), IFHAS 0/3,392 (follow-up loops 0/1,047, recall 0.906). Wrong-patient messages 0/16,486, sent without consent 0/11,650; KPI error 0.031 vs EID-only 0.207; reviewer time 11.6 h per 1k loops across the changed programme mix; hand-off packets 300/300 CRC, 21/21 lung, 35/35 DR. Programme-standardised origin ratio 1.07 (p = 0.185); age-band 1.07, sex 1.03. LDCT/IFHAS generators reuse development texts, so their reader results are not independent generalisation evidence. Disclosed post-hoc changes: τ 0.98 → 0.99 after one wrong-patient closure; the day-15 prep decision no longer skips pending histology (4 false closures in the first run at `b53ce89`); surname-variant and Emirates ID identity rules (about 10% less reviewer time, safety unchanged); hand-off packet fixes and two generator truth corrections after a trace of the misses; held-out noise model fix; MTSamples biopsy key correction; short surnames no longer corroborate alone (after 1 wrong-patient message in a run); second-identifier answers reused across a patient's loops; larger cohorts from every held-out person; reader v7.2 ignores upper-GI tissue in combined EGD + colonoscopy reports; a loop messages the patient only about its own programme's events; the diabetic retinopathy loop (readers tuned on one held-out corpus, fair-tested on a second); one link question per registration pair (also removed the held-out sex gap); the origin ratio standardised by programme (dev noise shows a possible CRC gap, 1.08–1.10, p = 0.01–0.06, in identity questions; weak evidence after about 16 subgroup tests); a confirming review before a human 'same' merges records that contradict on sex or birth date (after the Learn-then-Test re-run found one such double-reviewer-error merge); the same confirming review for second-identifier answers; a rejected intake that cites the referral raises a link question (4 missed packets recovered); the contradiction check narrowed to sex only (a trace found the birth-date arm held 675 true matches and caught no wrong merge); the breast loop (readers fair-tested on a held-out corpus by another model family). |
| **Clean-review sensitivity** (post-hoc; same five held-out v2 seeds and frozen settings, except both simulated reviewer-error rates set to zero) | CRC P 1.000, **R 0.859 (0.835–0.883)**, **0/3,322 false closures**, versus R 0.785 with reviewer-error stress (**+7.4 pp**). Wrong-patient messages 0/16,534; sent without consent 0/11,610. This makes simulated reviewers agree with simulated truth; it is not measured clinician performance and does not replace the frozen stress result. See `pipeline/results/clean-review-sensitivity/`. |
| **Post-hoc scale and wrong-EID hardening** | Fresh normal and x10 scale runs measured zero unexpected linker restarts, wrong-patient releases, false closures, consent violations, duplicate outputs and replay mismatches after the historical failures were fixed; all historical failures remain disclosed. On fresh B8 seeds 17/29 with 2% wrong EIDs, guard-off produced 19 false closures and guard-on produced 0; CRC recall 0.7535 → 0.7420, review 31.9205 → 33.012 items/100 loops. Guard remains default-off; residual linkage contamination remains. See `pipeline/results/scale/` and `pipeline/results/b8-hardening/`. Container runtime was verified only with Podman 5.8.2 using the Docker Compose v5.1.4 provider; Docker Engine and a restarted/retained-volume deployment remain unverified. Static deployment checks passed. |
| **Post-hoc CRC recall-loss analysis** (B8 seeds 17/29; read-only) | Wrong-EID guard-on had 394/1,527 misses: first barriers were identity/linking 49.5%, simulated reviewer error 26.6%, timing/ungraded prep 11.4%, and model/rule/other 12.4%. No link/no review was the largest category (146); ungraded prep appeared in 241 misses. After excluding 13 guard-off closures using another person's report, net right-patient guard recall cost was 0.26 pp. The split proposal uses new non-evaluation seeds, disjoint people, new text and new noise with locked validation; no training or tuning was implemented. See `pipeline/results/recall-analysis/`. |
| **Report readers on unseen text**: 7 more 300-report corpora by 4 model families (Claude, GPT-6.1 Sol, Gemini 4 Argon, Kimi K3.1 Pro), each frozen before any reader ran and each the fair test of the previous fix | 3 of 7 fair tests found an unsafe phrasing class: 6 of 211 automatic closures by the 26B were wrong (2.8%, UB 6.1%; 6/2,100 reports), 10 of 137 with the 4B. Each class fixed by a generic rule; reader v7.2 has 0 unsafe on all corpora; the last two fair tests (v7 on corpus 7, shipped v7.1 on corpus 8) passed with both readers. Pilot: shadow mode (clinician confirms every auto-closure) until 300 in a row are correct, then a 10% random audit; any error returns that class to shadow mode. |
| Learn-then-Test gate, 12,600 calibration loops (cal split, v2 noise) | τ_close 0.99 certified with 0 false closures and 0 wrong-patient messages (re-run at `0f6a532`): P(both rates ≤ 0.1%) ≥ 95% for loops like the calibration set. |
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
| Encoder retrain on real name pairs | Anas | Done |
| Re-run the frozen 5-seed evaluation on the final code | Anas | Done at decision code `075abc5` |
| Next.js dashboard and coordinator worklist on `docs/API.md` | Kai (frontend) | In progress |
| Clinically verified model-assisted labelling of the 43 reports; report κ (`docs/labelling/`) | Both + clinician | Complete: κ = 1.000; not independent human double-labelling |
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
- Patients, reviewers, noise and capacity are simulated. No independent human-labelled evaluation is claimed.
- The 43 report texts have clinically verified model-assisted labels (κ = 1.000 after clinician adjudication), not independent human double-labelling. The lung CT reports are synthetic. Fresh LDCT/IFHAS held-out text checks are non-blind, single-author synthetic evidence without clinician labels; their integrated generators still reuse development texts. The earlier fresh check found 0/124 agreed-but-wrong answers; that is not a zero-error claim. For the current agreed-but-wrong counts, see the fresh2 and fresh3 bullets below. Unsafe single-reader errors remain and 56% of IFHAS notes routed to review.
- Fresh2 model-labelled check (built, simulated text, not clinician-verified, not clinical effectiveness): two model families agreed on 113/120 items, 7 unadjudicated. Of 85 agreed items read by Gemma 3 4B, 4 were agreed-but-wrong: 1 closing-direction (f2i-031, key `bp=pending`, both readers `managed`) and 3 holding-direction (f2l-002, f2l-021, f2l-029). Reader B was unavailable on 13 items (11 IFHAS follow-up quote-grounding failures, 2 IFHAS screenings with an implausible coded value); unavailable is not a correct answer. IFHAS screening is coded results, not a Gemma reader, and is reported separately (6/28 panel regex, 28/28 coded; the guide wants a random glucose but 27/30 panels say fasting). Authoring and labelling ran in sibling-branch workspaces, so the chronology rests on commit order and the declared input and prompt hashes; it is not independently verified, and file times are not relied on. See `data/fresh2/results/ANALYSIS.md`.
- Fresh3 (the fair test after reader hardening; built, simulated text, model-labelled keys, not clinician-verified, not clinical effectiveness): two model families agreed on 93/100 items, 7 unadjudicated. 3 agreed-but-wrong answers, all LDCT work-up and all holding-direction (f3l-008, f3l-019, f3l-048; none closing), so the new LDCT safeguards did not fix that kind (3 before, 3 after, different items). Matching definitions only: IFHAS follow-up reader B unavailable 10 to 1, IFHAS follow-up agreed-but-wrong 1 to 0. Reader B was unavailable on 2 items; IFHAS screening is coded results, not Gemma. No overall accuracy improvement and no "0 unsafe" claim. Fresh2 limits above still apply. See `data/fresh3/results/ANALYSIS.md`.
- The Learn-then-Test guarantee covers loops like the synthetic calibration set (whose CRC loops are 51.9% women vs 69.6% in the eval cohorts, because the calibration split has only 779 women), not real Abu Dhabi data; a pilot must recalibrate on locally adjudicated loops (≈3.7k).
- The image reader was trained on Norwegian video frames; UAE stills need local recalibration. Its weights inherit ImageNet's non-commercial terms.
- Queue-lever effect sizes are borrowed from published trials; App 9 fields are synthetic.
- Female-list access gap: female-list protection in the booking engine cuts it from 18.5 to 12.6 pp at the assumed 15–30% female-list share. Getting under the 5 pp limit also needs about 30% (median 4.5 pp) to 40% (0.6 pp) of lists to be female lists. Wasl reports that staffing shortfall but can't fix it. The review-load ratio across name origins passes: 1.21 (95% CI 1.09–1.34) per seed and 1.08 pooled vs the 1.25 limit on held-out noise, and the pooled gap is no longer significant (p = 0.11). Two of five single seeds are above 1.25, which is how max/min over four unequal groups behaves. Placeholder birth dates (25% of Arab vs 6% of Western persons) still send more messages to a human on purpose. It costs reviewer time, not accuracy.
- Non-English notices are machine-drafted and need professional translation before the pilot.

## Top risks
1. **"It's measurement, not care."** Answer: targeting (1 in 4 on the chase list is wrong), queue control, network pooling.
2. **Synthetic data.** Answer: real names and report texts; frozen eval; we never claim outcomes.
3. **Malaffi is HL7 v2, not FHIR.** Answer: the HL7 v2 MLLP adapter is built and tested under drops and duplicates.
4. **"Your thresholds were tuned on your test data."** Answer: frozen evaluation on an independent noise model, and a Learn-then-Test certificate on a separate calibration split.
