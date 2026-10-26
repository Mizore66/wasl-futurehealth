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
| Wasl | 31.9% |

Wasl shows the evidence behind the programme's number, case by case. It is a DoH public-health and quality-assurance use (HIE Policy 4.3.12.1).

**Why the standard levers need Wasl:** reminders and navigation only work if you chase the right patients. With Emirates-ID-only tracking, 62% of real completions look lost, and about 1 in 4 patients on the chase list are already done. Wasl gets the measured rate within about 1 point of truth, so the levers land on the patients who are actually lost.

**What it is (built and run):**
- **Streaming backbone:** Go + NATS JetStream, 13 processes. 0 of 15,769 events duplicated or lost across process and broker kills.
- **Our own Arabic↔Latin name encoder:** 2.3 MB, in-process. It matches four off-the-shelf models combined (R@10 0.932 vs 0.928) at about 1/100 of the size.
- **Matching:** deterministic HNSW (1M records, R@10 0.999) plus a LightGBM scorer.
- **Safety gate:** a loop auto-closes only when the patient link is trusted *and* two evidence readers (DoH rules plus a local Gemma 3 4B) agree. Everything else goes to double human review.
- **Closure rule from the DoH CRC standard:**
  - Caecum documented, prep graded and, if tissue was taken, histology back.
  - End-states with owners: normal, surveillance, MDT, specialist, repeat, declined.
  - Suspected cancer never waits for histology.
- **Patient layer:** owner-named notices, teach-back, consultation routing, and the rights to contest, opt out and get an explanation.
- **Governance:** hash-chained provenance (tamper 17/17 detected), RBAC, a 12-item hazard log, model cards, and a DoH RAI control map (self-classified High risk).

**Measured safety (simulated cohorts, honest limits):**
- Merged run: CRC precision 1.000, recall 0.960, 0/570 false closures; 0 wrong-patient messages released.
- Gate under the strict DoH closure rule (5 seeds × 3 reviewer error rates): 0/8,422 false closures with double review. Single review gives 21, so double review is mandatory.

**Deployment and money:**
- **Hosting:** UAE-sovereign (Core42 / Azure UAE North), keys held in-country, egress denied.
- **Malaffi interface:** public docs show HL7 v2, so the HL7 adapter is the primary production tier.
- **Year-1 cost:** AED 0.54–1.39M, about AED 112–290 per closed loop at 12.6k FIT+/yr. Wasl's own price is a placeholder pending a separate pricing study.
- **Who pays for the extra colonoscopies:** not Wasl. Providers bill them and the payer (Thiqa or the insurer) funds them. It is mandated care (CRC 5.5.4.2), but new spend: ≈AED 3M a year at the base case, partly offset by ≈AED 0.55M in savings.
- **Value (base case: 5.7k FIT+/yr, +13 pp completion; automated levers alone project +18 pp):** ~18 extra cancers found a year, ≈AED 550k in treatment avoided, ≈3 deaths averted a year. That is ≈AED 43k–77k per life-year **all-in** (Wasl plus the extra colonoscopies, net of savings), under the WHO-CHOICE threshold (≈AED 185k). So the system-level case is health gained per dirham.
- **Gated path to pilot:**
  - 0–3 mo: shadow pilot (1 PHC → 1 unit) plus the DDGO assessment. Gate: 0 false closures; recall ≥0.95.
  - 4–9 mo: stepped-wedge across 6–8 PHCs (power 0.81–0.94). Gate: uplift lower bound >0.
  - 9–12 mo: ADHDS vendor contract (precedents: Orion, VitalHub, Digi7, the Philips AI PoC); fallback: DoH limited tender.
  - 12 mo+: cervical, breast, then other loop types.

**Roadmap (designed, not built):**
- **Phase 2, queue control.** This is how Wasl raises completion, not just measures it.
  - Direct-to-procedure booking with a nurse doing the App 9 checklist. It targets the 36.3% of non-completers lost at the GI consult.
  - Prep-failure prevention, the only lever that creates capacity: +62–197 slots per 1,000 exams.
  - Network pooling across a group's sites. Simulated: when slots are in the wrong places, ≤31-day completion goes from 22% to 42%. No gain when capacity is short overall.
- **Phase 3, one engine for every follow-up loop:** incidental imaging, diabetic retinopathy, IFHAS, breast and lung. That is ≈50–110k loops a year at ≈AED 5–28 per loop.
- **Depth:**
  - an image reader that the endoscopist sees only after grading blind;
  - a Learn-then-Test gate that guarantees false closures ≤ α.
- **Second route to market:** an on-premise deployment for a provider network tracking its own patients.
- **Tested and dropped:** we tested whether booking optimisation beats greedy when capacity is scarce. It doesn't, so we make no claim.

**Capacity:** when Wasl succeeds, the queue fills. Wasl uses misplaced slots across a group's sites, creates slots (prep, direct booking), and shows DoH the remaining shortfall. Below about 1 slot per FIT+ network-wide, no software helps.

**Demo (3 min):**
1. The undercount (12% vs 33%).
2. An Arabic-named patient's report arrives under a Latin name with no Emirates ID. It links, the readers agree, the loop closes.
3. A node kill: 0 lost events.
4. A reader disagreement is held, a human signs, and the chain verifies.
5. Network pooling (22% → 42%) and the gated pilot plan.

**Limits (what still needs building):**
- The v7 gate, booking engine and local reader aren't merged into one run yet; the merged run is a single seed.
- Frozen 5-seed evaluation not run.
- HL7 adapter not built.
- Consent hold not coded.
- Fairness remedies and the correlated-reviewer sweep not evaluated.
- The data is simulated, with no clinician labels, as expected for a hackathon.
- Measurement alone doesn't raise attendance; the gain comes from the patient levers and from capacity.
