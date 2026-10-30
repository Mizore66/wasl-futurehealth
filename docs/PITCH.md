# Wasl (وصل): evidence behind every screening follow-up across Abu Dhabi providers

**Challenge #5: Closing the Screening-to-Care Loop.** The full submission write-up, with every source and assumption, is `docs/WRITEUP.md`.

**The patient.**
- A woman has a positive stool test (FIT+) at her clinic.
- If her colonoscopy happens, it may be at another hospital, under a Latin spelling of her name and with no Emirates ID. Her clinic never sees it.
- In Abu Dhabi only 30.5% of FIT+ patients had a colonoscopy, after a median 72 days against a 31-day standard (SEHA 2021).

**The headline: KPI assurance for the ADPHC CRC programme.** Share of referrals reaching colonoscopy within 31 days, in our merged simulation:

| Tracker | Result |
|---|---|
| Emirates-ID-only tracking | 12.4% |
| Truth | 33.0% |
| Wasl | 32.0% |

Wasl shows the evidence behind the programme's number, case by case. It is a DoH public-health and quality-assurance use (HIE Policy 4.3.12.1).

**Why the standard levers need Wasl:** reminders and navigation only work if you chase the right patients. With Emirates-ID-only tracking, 62% of real completions look lost, and about 1 in 4 patients on the chase list are already done. Wasl gets the measured rate within about 1 point of truth, so the levers land on the patients who are actually lost.

**What it is (built and run):**
- **Streaming backbone:** Go + NATS JetStream. Ingests FHIR R4 and **HL7 v2 over MLLP**. Across 10 frozen-evaluation runs with process and broker kills: 0 of 201,285 outputs duplicated or lost.
- **Our own Arabic↔Latin name encoder:** 2.3 MB, in-process. It matches four off-the-shelf models combined (R@10 0.932 vs 0.928) at about 1/100 of the size.
- **Matching:** deterministic HNSW (1M records, R@10 0.999) plus a LightGBM scorer.
- **Safety gate:** a loop auto-closes only when the patient link is trusted *and* two evidence readers (DoH rules plus a local Gemma 3 4B) agree. Everything else goes to double human review. The closure threshold is **certified by Learn-then-Test** on a separate calibration split, not hand-tuned.
- **Photos as a third, hold-only reader:** caecal-landmark and bowel-prep photos can hold a loop or send it to a one-click endoscopist grade, never close it.
- **Consent hold:** no message goes out until contact permission is granted.
- **Closure rule from the DoH CRC standard:**
  - Caecum documented, prep graded and, if tissue was taken, histology back.
  - End-states with owners: normal, surveillance, MDT, specialist, repeat, declined.
  - Suspected cancer never waits for histology.
- **Patient layer:** owner-named notices, teach-back, consultation routing, and the rights to contest, opt out and get an explanation.
- **Governance:** hash-chained provenance (tamper 17/17 detected), RBAC, a 12-item hazard log, model cards, and a DoH RAI control map (self-classified High risk).
- **One engine, more loops:** incidental lung nodules (Fleischner 2017) already run on the same engine: 0/297 false closures in an 800-loop run.

**Measured safety (simulated cohorts, honest limits):**
- **Frozen evaluation** on a corruption model the system never trained or tuned on (5 seeds): CRC precision 1.000, recall 0.704, **0/2,063 false closures** (95% upper bound 0.18%), plus 0/1,463 on 800 lung-nodule loops per run; 0/11,682 wrong-patient messages; 0/7,539 messages sent without consent; review load across name origins max/min 1.13 pooled over seeds. Recall is lower than before because reports that never grade the bowel prep now wait for the unit's grade instead of being guessed, and the first run of this evaluation found 4 false closures from one bug, which we fixed and disclosed. An earlier run found one wrong-patient closure; we traced it to a name-only link at our confidence cap, raised the closure tier above the cap, and re-ran: zero.
- **Report readers on unseen text:** we tried to break our own readers with seven fresh 300-report corpora written by four AI model families (Claude, GPT, Gemini, Kimi), each frozen before the readers saw it. Three found a way to fool both readers at once: about 1 in 35 of the 26B's automatic closures on those corpora was wrong (6 of 211; 10 of 137 for the in-UAE 4B). Each was fixed by a general rule, and the last two fresh corpora, including one against the exact version we ship, passed with 0. Because real reports will hold new traps, the pilot starts in shadow mode: a clinician confirms every automatic closure until 300 in a row are correct, then audits a random 10%.
- **Learn-then-Test:** the 0.99 closure tier is certified on 12,600 calibration loops: P(false-closure and wrong-patient rates ≤ 0.1%) ≥ 95% for loops like the calibration set.
- Gate under the strict DoH closure rule (5 seeds × 3 reviewer error rates): 0/8,422 false closures with double review. Single review gives 21, so double review is mandatory.

**Deployment and money:**
- **Hosting:** UAE-sovereign (Core42 / Azure UAE North), keys held in-country, egress denied.
- **Malaffi interface:** public docs show HL7 v2, so the HL7 v2 MLLP adapter is the primary production tier. It is built: 30,668/30,668 resources delivered exactly once through connection drops and duplicate re-sends.
- **Year-1 cost:** AED 0.54–1.39M, about AED 112–290 per closed loop at 12.6k FIT+/yr. Wasl's own price is a placeholder pending a separate pricing study.
- **Who pays for the extra colonoscopies:** not Wasl. Providers bill them and the payer (Thiqa or the insurer) funds them. It is mandated care (CRC 5.5.4.2), but new spend: ≈AED 3M a year at the base case, partly offset by ≈AED 0.55M in savings.
- **Value (base case: 5.7k FIT+/yr, +13 pp completion; automated levers alone project +18 pp):** ~18 extra cancers found a year, ≈AED 550k in treatment avoided, ≈3 deaths averted a year. That is ≈AED 43k–77k per life-year **all-in** (Wasl plus the extra colonoscopies, net of savings), under the WHO-CHOICE threshold (≈AED 185k). So the system-level case is health gained per dirham.
- **Gated path to pilot:**
  - 0–3 mo: shadow pilot (1 PHC → 1 unit) plus the DDGO assessment. Gate: 0 false closures; recall ≥0.95.
  - 4–9 mo: stepped-wedge across 6–8 PHCs (power 0.81–0.94). Gate: uplift lower bound >0.
  - 9–12 mo: ADHDS vendor contract (precedents: Orion, VitalHub, Digi7, the Philips AI PoC); fallback: DoH limited tender.
  - 12 mo+: cervical, breast, then other loop types.

**Queue control (simulated decision support, built):** how Wasl raises completion, not just measures it.
- Direct-to-procedure booking with a nurse doing the App 9 checklist. It targets the 36.3% of non-completers lost at the GI consult.
- Prep-risk targeting (Dik 2015 score), the only lever that creates capacity: about 88 repeat colonoscopies avoided per 1,000.
- Together at 1.2 slots per FIT+: **+13 pp** ≤31-day completion in simulation. Below 1.0 slot, no lever helps.
- Network pooling across a group's sites: when slots are in the wrong places, ≤31-day completion goes from 22% to 42%. No gain when capacity is short overall.

**Roadmap (designed, not built):**
- **Phase 3, more loop types:** incidental imaging, diabetic retinopathy, IFHAS and breast. That is ≈50–110k loops a year at ≈AED 5–28 per loop.
- **Second route to market:** an on-premise deployment for a provider network tracking its own patients.
- **Tested and dropped:** we tested whether booking optimisation beats greedy when capacity is scarce. It doesn't, so we make no claim.

**Capacity:** when Wasl succeeds, the queue fills. Wasl uses misplaced slots across a group's sites, creates slots (prep, direct booking), and shows DoH the remaining shortfall. Below about 1 slot per FIT+ network-wide, no software helps.

**Demo (3 min):**
1. The undercount (12% vs 33%).
2. An Arabic-named patient's report arrives under a Latin name with no Emirates ID. It links, the readers agree, the loop closes.
3. A node kill: 0 lost events.
4. A reader disagreement is held, a human signs, and the chain verifies.
5. Network pooling (22% → 42%) and the gated pilot plan.

**Limits:**
- The data is simulated, with no clinician labels, as expected for a hackathon. The 43 report texts carry an LLM-produced key until our human double-labelling is done.
- The Learn-then-Test guarantee covers synthetic loops like the calibration set; a pilot must recalibrate on locally adjudicated loops.
- The image reader was trained on Norwegian video frames; UAE photos need local recalibration.
- Queue-lever effect sizes are borrowed from published trials.
- Measurement alone doesn't raise attendance; the gain comes from the patient levers and from capacity.
