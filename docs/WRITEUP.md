## Wasl (وصل): evidence behind every screening follow-up across Abu Dhabi providers (Closing the Screening-to-Care Loop)

> **The product:** evidence-backed CRC follow-up tracking for the ADPHC programme. It is built, and it is what we pitch and demo. One buyer (ADPHC), one KPI (31-day completion), one engine.
> **The roadmap** (designed, clearly fenced below): Phase 2 is queue control, which raises completion. Phase 3 puts the same engine to work on the programme's other follow-up loops, which spreads the fixed cost. Every item is marked built, simulated or designed.

**The patient.** A woman has a positive stool test (FIT+) at her clinic. If the colonoscopy happens, it may be at another hospital, under a Latin spelling of her name and with no Emirates ID (EID). The clinic never sees it. In one Abu Dhabi study only 30.5% of FIT+ patients had a colonoscopy, after a median 72 days against a 31-day standard (SEHA, BMC Cancer 2021).

**KPI assurance (headline).** In our merged simulation, counting all referrals:

| Tracker | Reaching colonoscopy ≤31 days from referral (CRC 5.5.4.2 and the App 2 KPI) |
|---|---|
| EID-only tracking | **12.4%** |
| Truth | **33.0%** |
| Wasl | **32.0%** |

- If you count only patients seen at colonoscopy, EID-only is close (65.5% vs 63.7%).
- Either way the true rate is far below the >90% target (CRC spec, Appendix 2).
- Wasl shows the ADPHC CRC programme the evidence behind its number, case by case. It is framed as a DoH public-health and quality use: the HIE policy (4.3.13.2) limits *facilities* to treatment and operations uses.
- **Why the programme needs it:** the 31-day KPI (CRC 5.5.4.2; App 2 KPI table: "between referral after positive screening FIT test and… colonoscopy") has no public Abu Dhabi measurement since 2015–16. DoH's referral-tracking platform is still described as future, and Malaffi identifies care gaps but not evidenced closure.

**Why the levers need Wasl: you can only chase the patients you know are lost.**
- Reminders, navigation and automated referral are standard, proven levers. Their weak point is *targeting*: a clinic can only chase patients it believes haven't had their colonoscopy.
- With EID-only tracking, 20.6 of every 100 referrals had their colonoscopy on time but look lost, because it happened at another provider under a Latin spelling with no Emirates ID. That is **62% of real completions invisible**, and **about 1 in 4 patients on the "chase" list (23.5%) already done**.
- So nurse calls and reminders get spent on patients who are fine, and the genuinely lost patients get the same diluted attention.
- Wasl's linkage gets the measured rate within about 1 point of truth (32.0% vs 33.0%), so the levers land on the patients who actually need them. It also says *why* each one is stuck (never notified, not understood, referral rejected, not booked), so each gets the right lever, not a generic reminder.
- The levers are commodity; knowing exactly who to point them at, across providers, is what Wasl adds.

**Constraints, stated up front (all numbers are sourced below):**

| Constraint | How Wasl handles it |
|---|---|
| **Endoscopy capacity: success fills the queue** | If Wasl finds and books every lost patient, demand for endoscopy rises. Where slots are short, more patients just means a longer queue. Wasl responds on three fronts:<br>(1) **Uses misplaced capacity (roadmap; simulated):** it books across a provider group's sites. In simulation this recovers misallocated capacity fully (22% → 42% within 31 days when the slots are in the wrong places).<br>(2) **Creates capacity (roadmap, designed):** it prevents failed preps, which frees +62–197 slots per 1,000 exams, and books straight to procedure, which recovers the consult step where 36.3% of non-completers are lost.<br>(3) **Shows the remaining shortfall per unit,** as a number for DoH capacity planning.<br>Below about 1 slot per FIT+ across the whole network, no software helps, and Wasl's report shows exactly that. |
| **Who pays for the extra colonoscopies?** | **Not Wasl.** Providers perform and bill them. The **payer** (Thiqa, i.e. government via Daman, or the patient's insurer) funds them. They are care the programme already mandates: the DoH CRC spec requires every FIT+ referral to get a colonoscopy within 31 days (5.5.4.2, target >90%), and about two-thirds currently don't happen. Delivering them does raise the payer's spend, by ≈AED 3M a year at the base case (≈740 extra colonoscopies at ≈AED 4k), partly offset by ≈AED 0.55M in earlier-stage treatment savings. So the system-level test is health gained per dirham: ≈AED 43k–77k per life-year all-in, under the WHO-CHOICE threshold (≈AED 185k). Wasl's own price is a separate, placeholder question. |
| **Every health-system entry runs through a regulated buyer** (true of any entry in this challenge) | A staged plan with a gate at each step: **(0–3 mo)** unpaid shadow pilot, 1 PHC → 1 endoscopy unit, via the Future Health pathway; DDGO assessment in parallel; exit criteria 0 false closures and recall ≥0.95 vs the coordinator's list. **(4–9 mo)** live outreach, stepped-wedge across 6–8 PHCs (power 0.81–0.94); go/no-go = uplift's lower bound >0 and 0 audited false closures. **(9–12 mo)** contract: ADHDS vendor agreement (precedents Orion, VitalHub, the UAE SME Digi7; DoH–ADHDS–Philips AI PoC) or, fallback, a direct DoH limited tender. Second door: on-premise for a provider network tracking its own patients (HIE 4.3.13.2.1.3). **(12 mo+)** cervical, then breast, then other loop types on the same engine, spreading the fixed cost (AED 0.54–1.39M year 1) toward ≈AED 5–28 per loop. |

**System (one Go codebase; every component below ran in an integrated NATS run with chaos testing):**
- **Streaming:** NATS JetStream (3 nodes), FHIR R4 Subscription ingest from a mock Malaffi server, and HL7 v2.5.1 over MLLP.
- **Real Malaffi interface:** public documents describe HL7 v2 only: real-time ADT feeds, the HIE Policy's "HL7 2.x/3.0", and LOINC codes in OBX segments. No public source documents FHIR. Ingestion is therefore tiered behind one idempotency key, `source|type|id|version`, sent as the NATS message ID:
  - (a) HL7 v2 ADT/ORM/ORU/SIU/MDM over MLLP, ACKed only after JetStream persists the message. This is the expected production tier, **built**: 30,668/30,668 resources delivered exactly once through 2 connection drops and 888 duplicate re-sends; CRC 0/574 false closures. Corrected results become new versions.
  - (b) FHIR R4 Subscription. Built and run against the mock: 20,906 versions processed exactly once despite 392 drops, 568 duplicates and 682 reorders.
  - (c) FHIR polling on `_lastUpdated`, with a watermark that advances only after the ack.
  - (d) Nightly bulk reconciliation.

  JetStream dedupes within 1 hour; older replays are caught by a create-if-absent ledger (designed).
- **Our own model:** an Arabic↔Latin character transformer in Go; deterministic HNSW blocking; LightGBM scorer; union-find with human merge/split.
- **Gate:** auto-close only if the patient link is trusted AND both evidence readers agree; otherwise human review.
  - **v7strict (canonical, built):** DOB/sex/surname-stratified recalibration; closure tier 0.99 (above the 0.989 cap on uncorroborated links); exact DOB match (or a human confirmation) before messaging on EID-chain or model-only links; per-field two-reader consensus; ungraded prep never closes.
  - Readers: regex/DoH rules and open-weights Gemma (cervical uses a coded field).
  - **Third reader, images (built; can hold a loop, never close one):**
    - DoH App 11 already requires caecal-landmark photos. A classifier trained on HyperKvasir (CC BY 4.0) uses its cecum (1,009 images), retroflex (391) and BBPS 0–1 / 2–3 (646 / 1,148) classes.
    - Our held-out results (near-duplicate-grouped split): caecum AUC 0.998, prep AUC 0.991 (specificity 0.83). In the integrated run: 0/571 false closures and 0 labels upgraded. Weights published with the dataset authors' permission.
    - **Job:** for "prep ungraded" reports, ask the endoscopist to grade prep **blind** from the stored photos first. Only then is the image reader's suggestion shown. If the two disagree, the case goes to a second reader.
    - The agreement rate between the endoscopist and the reader is audited monthly, to detect automation bias.
    - The reader also flags missing caecal photos.
    - This recovers part of the 0.766 → 0.273 auto-closure drop without breaking the DoH rule, because a human still grades.
    - Domain shift (Norwegian video frames vs UAE stills) means local recalibration in the pilot.
  - **Learn-then-Test gate (built; replaces the hand-set thresholds, `-policy v7ltt`):**
    - It gives a finite-sample guarantee that P(false-closure rate ≤ α) ≥ 1−δ, run separately for wrong-patient and false-adequate risks.
    - On 12,600 calibration loops from a separate person split (held-out noise), the 0.99 tier certifies at α = 0.001, δ = 0.05 with 0 false closures and 0 wrong-patient messages. The guarantee covers loops like the calibration set, not real data.
    - This directly answers "rules were chosen after seeing eval errors". We found no published application to record linkage.
  - **Closure rule, per the CRC standard:** a loop closes only with caecum documented (App 11.2.2), prep graded Excellent or Adequate (App 9.4–9.5, App 14), and, **if tissue was taken**, histology back. A no-polyp, no-biopsy report with those two fields closes as `closed_normal` on the report alone. An ungraded prep stays open as `done_quality_unverified`, and Wasl queries the endoscopy-unit lead, who has 5 working days to reply [A]. With no reply, it escalates to the programme coordinator. If the grade still isn't documented by day 15, a clinician decides between `closed_normal` with a documentation flag and `repeat_required`. This replaces our earlier v7 "ungraded prep can close" assumption.
  - **End-states (CRC 5.5.3, App 5).** A loop closes only into a named state with an owner and a next date:

    | End-state | Condition | Next step |
    |---|---|---|
    | `closed_normal` | No polyps, or non-neoplastic histology | Re-invite at the programme interval (5.5.3.1) |
    | `closed_surveillance` | Adenoma, or a sessile serrated lesion | Surveillance colonoscopy at 5 y/none, 3 y or 1 y by count and size. Adenomas follow DoH App 5. Serrated lesions follow BSG 2020 by the same count and size rules, because App 5 doesn't cover them [A]. Wasl opens the future loop. |
    | `closed_to_gastro` | Significant non-neoplastic finding (e.g. colitis) | Referral to gastroenterology with an owner. The loop stays visible as `awaiting_gastro` until gastroenterology accepts the referral or a first appointment is documented, and only then closes. |
    | `closed_to_mdt` | Cancer | CRC core-team MDT; the treatment clock (<31 working days, 5.5.3.4) starts as a new loop |
    | `closed_to_specialist` | Increased- or high-risk features | Specialised centre (5.1.2–5.1.3) |
    | `closed_declined` / `closed_unfit` / `closed_transferred_out` / `closed_deceased` | Informed refusal documented by the treating physician (CRC 4.16.2), with a FIT re-offer per 5.2.1.1 where relevant; or clinician-documented unfitness; or care moved outside Abu Dhabi or the UAE; or death | Human closure only, never automatic. A reason code is required, and these closures are reported separately so they can't inflate the KPI. |
    | `repeat_required` (open) | Prep inadequate, or caecum not reached (App 9, App 11 §2) | Repeat colonoscopy is booked by the endoscopy unit, with a new 31-day clock from the inadequate procedure. ESGE 2019 (strong recommendation) and USMSTF set an *outer* limit of a repeat within 1 year for screening and surveillance. A FIT+ colonoscopy is diagnostic, so we keep the programme's 31-day standard as a configurable default. If the repeat is also incomplete, the unit is asked for an alternative test (e.g. CT colonography), with clinician choice. It stays the same loop. |
    | `awaiting_histology` (open) | Tissue taken, histology not yet reported | Clock: 10 calendar days from the procedure, aligned with the RCPath turnaround benchmark of 90% within 10 days (the DoH CRC spec sets no pathology clock) [A]. A breach escalates to the lab lead, and then to the coordinator at day 15. |
    | `suspected_malignancy` (open, never auto-closes) | Endoscopist's impression says suspicious, mass or stricture, or an urgent flag is set | Same-day MDT coordinator alert with a pre-filled referral. The 5.5.3.4 clock runs from the endoscopy date, not from histology. Histology then moves it to `closed_to_mdt` or back to the table above, with clinician sign-off. |

    Wasl reads the interval from the report; a clinician confirms it. A mismatch between histology and interval goes to review.
  - Double review on merges and on "adequate" verdicts. Correlated-error mitigations (designed):
    - different roles for the two reads (coordinator for links, clinician for evidence);
    - a blinded second read;
    - reviewer-pair rotation;
    - a 2% weekly senior audit sample to estimate correlated error.
- **Patient layer:**
  - Template notices that name the case owner.
    - **Languages:** English source text now. Arabic is machine-drafted, as are the other languages, and every non-English template needs professional translation before the pilot. Urdu, Hindi, Malayalam, Bengali and Tagalog drafts are attached alongside it (Appendix A), matching the UAE's largest non-Arab groups (India ~38%, Pakistan ~17%, Bangladesh ~7%, Philippines ~7%; secondary UAE-wide estimates, since SCAD publishes no Abu Dhabi nationality breakdown).
    - The new languages will be professionally translated and back-translated before the pilot.
    - The teach-back parser has only been tested on EN/MSA/Gulf/Arabizi; replies in other languages will route to an interpreter callback until validated.
    - **Messaging channel:** SMS is sent through an in-UAE licensed aggregator (TDRA-registered sender ID), with message logs kept in-UAE. WhatsApp is used only for non-clinical prompts such as "please call your coordinator". No result or clinical content goes over WhatsApp. A contest or explanation request needs one-time-code identity verification.
  - **AI disclosure:** every notice states that a computer system matches records and reads reports, and that a person checks uncertain cases (DoH RAI 3.1.6.1.1–2).
  - **Patient-to-clinician consultation:**
    - A patient with a question replies or calls. The message is routed by topic: prep, sedation or results go to the endoscopy-unit nurse; whether to proceed, risk or refusal goes to the referring physician.
    - Response SLA: 1 working day for nurse questions and 2 for physician questions [A]. A breach escalates to the coordinator.
    - The outcome is logged as a `consultation` event on the loop: answered, rebooked, declined (leading to `closed_declined` with the physician's documentation), or escalated.
    - Clinician-to-clinician: the structured e-consult in the rejected-referral hand-off (below).
  - **Patient rights:**
    - **Explanation:** on request, and only after one-time-code verification to the phone number on record, Wasl renders the patient's provenance chain in plain language: which record matched, which report closed the loop, and the end-state.
    - **Contest:** replying "this isn't right" re-opens the loop as `contested`. A different human reviews it within 5 working days, and auto-closure never overrides a contest.
    - **Opt-out (RAI 3.1.6.1.3):** the patient can opt out of automated outreach and auto-closure. The loop then runs fully manually, and the clinical follow-up continues.
    - **Retention:** opting out stops processing, not retention; records are kept ≥25 y.
  - Teach-back and e-consult parsed by Gemma (cached in the merged run).
  - Outreach held on unconfirmed links.
  - SLA escalation per programme, using official clocks where they exist: result to patient within 15 working days (CRC 5.5.2, breast 3.7.1) or 21 days (cervical 10.6.2); referral → colonoscopy ≤31 days, measured from the referral date as in 5.5.4.2 and the App 2 KPI. App 4 item 7 phrases the same target "from a positive FIT test", so Wasl also reports the FIT+→referral lag; that lag has its own 95% KPI; colposcopy by grade (CX App 5). The acknowledgement and referral-reply deadlines are **our own configurable defaults**; no standard sets them.
- **Impact chain: one lever per drop-off point** (merged run, 1,500 CRC loops):
  - never notified (153): owner-named notice plus SLA alarm;
  - not understood (260): teach-back, nurse callback;
  - referral rejected or unanswered (123): hand-off below;
  - no-show (71): booking with overbooking and auto-rebook;
  - unlinked (168 → 0 after review): linkage.

  Wasl names each lost patient's drop-off point: 96.6% of CRC loops get the same state as the simulator's hidden truth, across 10 states (cervical 98.8%). The worst confusion is "referral unanswered" vs "not booked" (38 vs 8 true).
  - **Patient-side metrics (planned):**
    - teach-back first-pass rate (*understood* on first reply ÷ notified), per language;
    - nurse callback within 2 working days for misunderstood replies (0/50 missed escalations on the frozen set);
    - SURE 4-item decisional-certainty screen (Légaré 2010). We found no Arabic-validated version, so before use the plan is forward/back translation plus cognitive interviews with ~10 patients per language (ISPOR translation guidance). Until then it is reported as exploratory. For reassurance specifically (the brief's word), the primary measure is the anxiety subscale of the HADS (HADS-A), which has published Arabic validations. We pair it with the exploratory 6-item short-form STAI (Marteau & Bekker 1992), for which Arabic versions of the full STAI exist; the short form's Arabic validity is unconfirmed, so it is also exploratory;
    - share of normal results communicated within 15 working days. The measure it moves directly is **days until a stalled loop is noticed**: today that only happens through manual audit; with Wasl it happens on the day the deadline passes. Not yet measured; it falls out of the event log.
- **Rejected-referral hand-off (designed):**
  - The sender keeps ownership and sends a structured packet: result, quoted rejection reason, preferences.
  - The receiving unit must accept, accept with conditions, redirect or query by a deadline.
  - A redirect creates a new owned referral; silence escalates to the coordinator.
- **Booking engine (separate, simulated):** constraint-safe booking with overbooking and auto-rebooking. Female-endoscopist, Ramadan and region constraints were honoured on 996/996 bookings. The value comes from booking at all, plus overbooking and rebooking. It reports both the KPI and the all-FIT+ rate, because dropping no-shows games the KPI.
  - **We tested whether optimisation beats greedy when capacity is scarce, and it doesn't.** Capacity sweep at 0.8–1.2 slots per FIT+, 40 draws, plus a 10%-female-list variant: the daily ILP beats greedy by ≤0.5 pp at every level (best case 15.7% vs 13.3%, winning 29/40 draws). Every interval crosses 0.
  - The published 12–16% gains need multi-period lookahead, which we don't do. So we make no optimisation claim.
  - **Below 1.0 slot per FIT+, ≤31-day completion collapses to 0–14% under *any* booking policy.** Only levers that create capacity help there (see "Queue control" below).
- **Fairness remedies (designed, not yet evaluated):**
  - reserve female-endoscopist slots (the simulator sweep shows ~91% at 40% of lists);
  - per-origin gate calibration to equalise review load.
- **Dashboard:** reported vs evidenced closure per provider; drop-off reasons (never notified, not understood, referral rejected, no-show, inadequate, unlinked); review load by name origin; small-cell suppression.
- **Governance:**
  - RBAC and consent in code;
  - hash-chained provenance, with the daily root co-signed into an in-UAE WORM store that is independent of the pipeline (optional external publication);
  - Legal mapping **verified against official texts**:
    - Law 2/2019 Art. 13: in-UAE storage and processing, copies included (ADHICS DP 1.4);
    - Art. 20: retention ≥25 y;
    - PDPL Art. 2(2)(e): excludes health data covered by its own law;
    - breach reporting: ADHICS DP 1.7 form within 72 h, HIE 6.1.2.27 to DoH and Malaffi within 5 working days, patients within 60 days;
    - DoH RAI Standard, self-classified High risk.

    Open item: PDPL Executive Regulations are not yet issued.
  - **DoH RAI control map (25 clauses).**
    - Evidence: human-in-the-loop, robustness, traceability, RBAC, attribute use.
    - Designed: monitoring, explanations, owners.
    - Written in this document: risk register (hazard log H1–H12 below, 3.3.1.1–2), sustainability note (3.1.8), data-protection note (3.1.4.4), model cards (3.2.4–5).
    - Data licence gap: the 43 MTSamples evaluation reports. mtsamples.com permits "print, share, or link… for educational purposes" with credit and is silent on other uses. The Kaggle CC0 label is the uploader's own. They were used for evaluation only and will be replaced by the synthetic App 14 corpus.
    - Remaining gaps, all deployment-stage: the DDGO conformity assessment before and after deployment (3.3.1.2.2.2, the only clause specific to High-risk systems), MFA and encryption at rest (build), clinician co-design and an AI-literacy programme (pilot stage).
  - **Hazard log / risk register (RAI 3.3.1.1–3.3.1.2): 12 hazards, each rated, with controls and a named owner role. Reviewed quarterly and after any audited false closure.**
    - H1, wrong-patient closure: p≥0.99 gate, DOB guard, double review. *Measured:* 0/8,422 false closures; 0 wrong-patient sends.
    - H2, false "adequate": two readers, and an ungraded prep stays open. *Measured:* 0.
    - H3, missed surveillance interval: clinician confirms.
    - H4, cancer delayed until histology: the `suspected_malignancy` state.
    - H5, contest ignored: a contest blocks auto-closure.
    - H6, outreach sent before consent: **open gap**.
    - H7, correlated reviewer error: ρ sweep planned.
    - H8, unequal burden or access: monitored; the 79% vs 97% gap is open.
    - H9, data leaving the UAE: local 4B, egress denied.
    - H10, silent message loss: *measured* 0/15,769.
    - H11, tampering: *measured* 17/17 detected.
    - H12, drift on new report templates: disagreement alarm.
  - **Legal basis for the DoH use:**
    - HIE Policy 4.3.12.1 lets DoH use Malaffi data "for public health activities authorized by Applicable Laws", including "reducing morbidity and mortality" (.3) and "quality improvement and quality assurance" (.6).
    - Law 2/2019 Art. 16(3) and (5) allow disclosure without written consent for public-health preventive procedures and at the Health Authority's request for monitoring.
    - Patient outreach stays under the referring facility's treatment relationship (4.3.13.2.1.3).
    - **Two separate consent questions:**
      - **Processing the data** (linkage and evidence): no patient consent is needed, because the lawful basis is DoH public-health and QA use (HIE 4.3.12.1, Art. 16).
      - **Contacting the patient** (SMS, WhatsApp or calls): this needs the patient's recorded contact-channel preference and opt-out status from the referring facility, held as a FHIR `Consent` resource or an HL7 PD1/CON segment.
      - Wasl holds outreach until that status is known, and never contacts a patient who has opted out. The 16 early messages broke this second rule (contact status not yet known), not the first.
  - **Data protection impact (summary).** Each purpose gets only the fields it needs:

    | Purpose | Fields |
    |---|---|
    | Linkage | Names (both scripts), DOB, sex, EID if present, MRN |
    | Evidence | Report text, read in memory; only the extracted fields and quotes are stored |
    | Outreach | Contact details, held by the referring facility, never copied into Wasl |
    | Monitoring | Inferred origin and sex, aggregate only |
    | Image reader (roadmap) | Procedure photos already in the record, read in memory; only the suggested grade and the landmark flag are stored. Basis: the treating unit's own quality documentation (App 11) |
    | Prep-risk score (roadmap) | Medications, diabetes, ASA, prior prep grade. Basis: treatment (4.3.13.2.1.3); used only to tailor the patient's own prep support |
    | Direct-to-procedure booking (roadmap) | The App 9 checklist fields. Basis: treatment; a nurse completes the checklist and the clinician signs |

    - **Lawful basis:** HIE Policy 4.3.12.1 and Law 2/2019 Art. 16 (below).
    - **Retention:**
      - The **clinical record** (the reports and referrals) stays with the treating facilities and Malaffi, which hold the Law 2/2019 Art. 20 retention duty (≥25 y).
      - Wasl keeps **pointers, decisions and the provenance chain**, plus the **linkage store**. "Pointers, not copies" applies to clinical content (reports, results). For identity it can't apply: linkage needs normalised names in both scripts, DOB, sex, EID-if-present, and the encoder embeddings in the HNSW index.
      - These identity fields and embeddings **are personal data**. Lawful basis: HIE 4.3.12.1, the DoH public-health/QA use. They are encrypted under the customer-held key, accessible only to the linker service and reviewers, and kept as long as the loop is open, plus the audit period. Embeddings are rebuilt, not archived, when a patient record is merged, split or erased by the source system. That is retained for the life of the loop plus the audit period, aligned to the record because closure decisions are part of the care record.
      - Derived scores (prep risk, image suggestions, inferred origin) are dropped 2 years after loop closure, unless an audit or contest is open.
    - **Access:** RBAC, with every access hash-logged.
    - **Residual risk:** linkage inherently needs identifiers, so de-identification isn't possible (RAI 3.1.4.4.3, accepted with controls).
  - **Fairness acceptance thresholds** (checked before go-live and monthly [A]):
    - review-load ratio between name-origin groups ≤1.25 (measured 1.14, 95% CI 1.07–1.21 on held-out noise. The gap came from placeholder 1 Jan / 1 Jul birth dates, which are commoner for some origins and can't corroborate a link alone. A placeholder DOB now corroborates only on a full name match);
    - access gap in ≤31-day booking between female-list and other patients ≤5 pp (measured 18 pp: fails, so slot reservation is required);
    - false-closure rate: 0 in every group.
  - **Clinical safety officer:** a named ADPHC clinician (role [A]). Any change to closure rules, intervals or reader models goes through change control: hazard-log impact, regression on the frozen sets, and CSO sign-off per release.
  - **Sovereignty:**
    - **Merkle anchor:** a daily root hash only (32 bytes, no patient data), written to a WORM store inside the UAE and co-signed by the ADHDS CISO and an ADPHC officer. Publishing it externally would be safe, because it holds no patient data, but it isn't needed.
    - **US CLOUD Act exposure on Azure/Core42:** data is encrypted with keys held in a DoH/ADHDS-controlled HSM that Microsoft can't access (Core42 external key management). A compelled-disclosure order to the cloud provider would therefore yield only ciphertext **for data at rest and in transit**.
      - **Data in use** (VM memory during processing) is the residual exposure. It is mitigated by Azure confidential VMs (AMD SEV-SNP or Intel TDX memory encryption) for the reader and decision services, and recorded in the hazard log under H9.
    - The residual risk is metadata and service telemetry, kept in-UAE per ADHICS CS 1.2(3) and covered in the hazard log (H9).
  - **Roles:**
    - DoH/ADPHC is the data **controller**.
    - ADHDS, as Malaffi operator, is the **processor**.
    - Wasl is a **sub-processor** under ADHDS. It is bound by ADHICS third-party controls TP 1 (third-party security policy) and TP 2.1–2.3 (security requirements in the agreement, service monitoring, change control), with access only inside the ADHDS boundary.
  - **Why infer name origin rather than use recorded nationality:**
    - The fairness risk is in *linkage difficulty*, which depends on a name's script and transliteration patterns, not on citizenship. For example, an Arab-named Indian national and a South Asian-named Emirati fail differently.
    - Recorded nationality is used where present, for access-equity reporting such as the female-list gap.
    - Inferred origin is used only to stratify linkage review load and error rates, where nationality would hide the effect.
    - This is the minimum attribute that answers the question (RAI 3.2.2). It is never shown per patient.
  - **Model licences:**
    - Gemma 3 is used under the Gemma Terms of Use and Prohibited Use Policy, which allow commercial deployment with use restrictions and pass-through of those terms. The restrictions bar generating medical advice without human oversight; Wasl's reader only extracts fields and every closure has a human-review path.
    - LightGBM is MIT; the encoder is our own.
  - **Name origin (data-protection note):** it is inferred only for aggregate fairness monitoring and is never a scorer input; the scorer uses script, sex and DOB. It is access-restricted, reported with small-cell suppression, and dropped if monitoring ends (RAI 3.2.2).
  - **Sustainability (RAI 3.1.8):** the reader uses ~1.4–10 kWh a year (4.5–33 GPU-h); the always-on CPUs use ~4.4 MWh a year [A]. Energy is kept down by the 4B reader instead of the 26B, regex-first routing, caching by report hash, and modular services.
  - **Model cards:** one each for the encoder, pair scorer, evidence readers, teach-back parser and no-show predictor. Each covers intended and out-of-scope use, data, metrics, subgroup results, oversight point, owner and retraining trigger. Examples:
    - pair scorer: retrain on any audited false merge at p≥0.99;
    - readers: retrain on any false "adequate" or a new report template.

    The no-show predictor is a hand-set logistic model (SEHA base rate 0.174, prior no-show OR 3.8), not a trained model.
  - **Retention vs chain:** one hash chain per loop (4 entries, ~2 KB), plus a daily Merkle root co-signed into an in-UAE WORM store outside the pipeline's write path.
    - Corrections are appended `amendment` entries that point at the superseded hash; nothing is deleted.
    - Measured footprint: ~37 KB per loop, all logs included. That is 12 GB over 25 years at 12.6k loops/yr, or 35 GB with 3 in-UAE copies.
    - After 2 years, records move to in-UAE WORM storage.

**Evidence (real runs; Wikidata names, CC0, some Arabic forms LLM-written; cohorts, noise, patients and reviewer simulated):**

| Area | Result |
|---|---|
| Integrated baseline run (v7strict), one seed: 1,500 CRC + 600 cervical; 4% independent reviewer error; 3 process kills + 1 NATS kill | CRC P 1.000, R 0.948, 0/563 false closures. Cervical 0/216. No review: CRC R 0.167. 26.3 reviewer-h/1k. 0 duplicates or losses. Tamper 17/17. Forged 0/24 with effect. 0 wrong-patient sends. 0 messages without granted consent. |
| **Frozen final evaluation**, 5 seeds × 2 noise models; v2 = an independent corruption model never used for training or tuning | Held-out v2: CRC P 1.000, R 0.864 (0.859–0.870), **0/2,475 false closures** (UB 0.15%), 0/3,686 wrong-patient messages, 0/2,650 messages without consent, KPI error 0.038 vs EID-only 0.201, review-load ratio across origins 1.14 (1.07–1.21). Dev v1: R 0.947, 0/2,728. At τ 0.98 a run had found 1/2,757 (a name-only link at the 0.989 cap); τ is now 0.99, a disclosed post-hoc change. |
| Lung-nodule loop (Fleischner 2017), 800 loops on the same engine | P 1.000, R 0.993, 0/297 false closures; recommendation accuracy 1.000. Synthetic CT report corpus. |
| Rule baselines | EID-only: P 0.834, R 0.364. Any procedure ≤120 d: P 0.661, R 0.431. |
| v7 gate under the **current closure rule** (ungraded prep stays open; offline, one cohort, 5 seeds × 3 reviewer error rates) | Double review: 0/8,422 false closures (upper bound 0.04%); recall 0.973 / 0.958 / 0.943 at 2 / 4 / 6% error. No-review recall 0.273; 27.0 reviewer-h per 1k loops. Single review: 21 false closures, so double review is mandatory. |
| *Same gate, old rule (ungraded prep closes); superseded* | No-review recall 0.766; 0/8,553; 16.1 h/1k. 20 of the 43 MTSamples reports never grade prep, so this cohort is a worst case. The DoH report template (App 14) has a prep-quality field. Rules were chosen after seeing eval errors. |
| v7 calibration | 0/5,142 wrong at p≥0.99 (was 20). Calibration error .025 → .051, by design: Clopper–Pearson lower bounds and a cap at 0.989 for uncorroborated pairs. Over-confidence at the gate means a wrong-patient closure; under-confidence elsewhere costs only review time. |
| Encoder vs off-the-shelf | Recall@10 0.932 vs 0.604 (best single) and 0.928 (all four combined). F1 .954 [.950–.958] vs .946 [.942–.951] for the combination. Against the combination, the gain is **deployability, not accuracy**: 2.3 MB in-process Go vs 236 MB across four models plus ONNX/Python, deterministic, and CPU-only. Stacking ours with ICU + Double Metaphone gives R@10 .962. Trained on the eval's noise model. **Upgrade in progress:** retraining on *real* Arabic↔Latin person pairs (Wikidata variants and NETransliteration, both CC0; JRC-Names and ANETAC rejected on licence) with self-mined same-surname hard negatives, tested on held-out real variants. |
| Scale (synthetic 1M) | HNSW recall@10 0.999; p50 0.42 ms (p99 ≈8 ms under load). |
| Local inference (43 reports) | Gemma 3 4B hybrid reader, on a laptop with no proxy: 40/43 agreement with the 26B; 0 wrong; 7.5 s/report. Live in the integrated run: CRC P 1.000, R 0.955, 0/567 false closures. |
| Booking (calibrated to SEHA; capacity assumed) | Over completed colonoscopies: 94.5% at ~1.5 ring-fenced slots per FIT+/yr (status quo 23%; first-available booking 90%, so most of the gain is from booking at all plus capacity). Over all FIT+: 52% (status quo 6%). Optimiser ≈ greedy. Female-endoscopist patients: 79% vs 97%. |
| Parsers (one non-clinician labeller) | E-consult 100/100 vs keyword 43 (tuned 85). Teach-back 98/100, 0/50 missed escalations. |
| Report text | 43 US MTSamples (6 not colonoscopies), LLM-labelled key, reused 17–44×. Readers agree on 41/43; 0/455 agreed labels wrong. |

**Frozen-evaluation protocol (to run before submission):**
1. Independent noise generator, used only for testing.
2. Freeze all thresholds and checkpoints (git hash).
3. 5 fresh seeds on unseen persons.
4. Report means with 95% intervals.
5. Pre-committed revert-on-miss.
6. Correlated-reviewer sensitivity at ρ = 0–1.

**Path to pilot:**
- Delivered as a Malaffi module through ADHDS, inside UAE hosting. ADPHC funds it as KPI assurance; providers pay nothing extra.
- **Deployment (UAE-sovereign):**
  - Core42 Sovereign Cloud (Azure plus external HSM with customer-held keys), with Azure UAE North (3 zones) as the reference.
  - NATS: 3 nodes, one per zone, R3 streams; async mirror in UAE Central.
  - **DR targets [A]:**
    - Zone loss: RPO 0 for acknowledged messages and RTO ≈ seconds. JetStream R3 is Raft-replicated and acknowledges only after a quorum write, so it survives losing one replica (NATS docs, "Replication and R3"). Our own kill tests: 0/15,769 lost.
    - Region loss: RPO ≤ 5 min (the mirror lag, an alarmed threshold) and RTO ≤ 4 h in regex-only mode, so nothing auto-closes [A].
    - **Failover runbook:**
      1. Declare region loss. A human decides; there is no automatic promotion, so split-brain is avoided.
      2. Fence the old primary: revoke its credentials and block its routes.
      3. Promote the UAE Central mirror streams to writable (edit the stream to remove the mirror config).
      4. Restart consumers from their last processed sequence. Duplicates are absorbed by the idempotency ledger.
      5. Re-point the HL7/FHIR ingest endpoints.
      6. Reconcile against the Malaffi nightly export.

      Failback is the same in reverse, with replay by idempotency key. Each step is scripted, which is how we reach the ≤4 h target. It hasn't been tested yet.
    - After failback, the stream is reconciled by idempotency key and the nightly Malaffi export; decisions are replayed deterministically from the log.
  - Gemma 3 4B reader: 2.2k–15.6k reports/yr is 4.5–33 GPU-hours/yr. Two quarter-GPU VMs in separate zones; optional 26B on a half-GPU VM.
  - DR in UAE Central is regex-only (no GPU SKU), so nothing auto-closes during DR.
  - Keys sit in a DoH/ADHDS-held HSM (ADHICS CS 1.2(5,7,8)). Logs, ledger, backups, DR and telemetry all stay in the UAE (Law 2/2019 Art. 13; ADHICS CS 1.2(2), DP 1.4). Reader-subnet egress is denied.
- **Operations:**
  - **SLOs:** ingest→decision p95 <60 s; 99.5% monthly availability; 0 lost messages, reconciled nightly; <5% of review items older than 2 working days; notice-SLA breaches alarmed within 1 h.
  - **Upgrades:** rolling, one zone at a time. The decision service runs blue-green: shadow-decide and compare before switching.
  - **Model rollback:** every decision records its model and rule hash. Rollback repoints to the pinned checkpoint and replays affected loops; HNSW is deterministic, so replays are byte-identical. An audited false closure freezes auto-closure.
  - **Runbooks:** node loss; reader outage (regex-only mode, nothing auto-closes); HSM loss; Malaffi feed gap (reconcile from the nightly export).
- **GPU supply:** Core42's AI Cloud publicly lists H100s in the UAE from $2.50/h, inside its Regulated Tech Environment. Our sizing needs a fraction of one GPU.
- **Prototype residency:** the 26B reader ran through a third-party proxy and received only public MTSamples text and synthetic data, never patient data. Production uses the local 4B, which agrees on 40/43 reports.
- **Year-1 cost (AED; [A] = assumed):**

  | Item | Range |
  |---|---|
  | Hosting (Azure UAE North list, 1-yr reserved). Customer-held external keys (Core42 EKM / Managed HSM, ≈$28k/yr) are the **floor in every tier**, because the CLOUD Act protection depends on them. The low figure below therefore understates hosting by about AED 0.1M; the totals are corrected. A cheaper Key Vault-only tier would give up that protection, and we don't offer it. Low: 3× D4s_v5 for NATS ($3.8k) + 2× D8s_v5 app and storage (≈$5.6k) + 2× quarter-GPU NC RTX PRO 6000 ($18.1k) High adds Managed HSM ($28k), the 26B tier ($19.6k) and +30% for DR and monitoring. The GPUs are about two-thirds of the low line and exist for zone redundancy, not load (≤33 GPU-h/yr). A CPU-only 4B reader would cut ≈AED 67k [A, CPU speed not measured]. | 101k–309k |
  | Malaffi/ADHDS integration: 28–54 person-weeks for 4 interfaces (ORU/ADT, report text, Task write-back, SSO). That is above vendor quotes of $10–25k per interface and below the $50k per interface in Walker (Health Affairs 2005). Elapsed time is 6–9 months: Orion took 6 months from contract to a live Malaffi core, and onboarding takes "up to several months" | 168k–540k |
  | DDGO conformity + ADHICS dossier and external test [A] | 88k–290k |
  | Operations, 0.25–0.5 FTE [A] | 66k–156k |
  | Review labour: 27 h per 1k loops; coordinator ≈AED 90/h; sessional gastroenterologist ≈AED 450/h for the 12.8 h evidence share. CRC only, 1.8k–12.6k loops. Three programmes at 0.45 FTE ≈ AED 0.12M [A]. | 14k–96k |
  | **Total** | **≈0.54M–1.39M** (EKM included at the low end) |

  - Per closed loop in year 1: AED 112–290 at 12.6k FIT+/yr, or 803–2,079 at 1.8k. Year 2 halves that.
  - For CRC *alone* at the 1.8k volume floor, Wasl costs more per loop than manual tracing (AED 10–91, labour only). That is why it isn't priced as a CRC-only tool: the fixed cost is shared across cervical and breast now, and across the other loop types on the roadmap. Manual tracing also produces no evidence trail, no fairness monitoring and no capacity signal.
  - **Reviewer staffing.** Under the current rule CRC takes 27 reviewer-h per 1k loops: link checks by a coordinator 14.1 h, evidence checks by a clinician 12.8 h. At 1.8k / 5.7k / 12.6k loops that is 49 / 154 / 340 h a year, or 0.03 / 0.09 / 0.2 FTE.
    - Cervical review measured 7.2 h/1k; for breast we assume no better than CRC [A]. With all three programmes at 12.6k loops each: ≈770 h/yr, ≈0.45 FTE.
    - Reviews sit centrally in the ADPHC CRC programme team (the public-health QA function), not at providers.
    - One coordinator plus a sessional gastroenterologist covers even the top volume. Funding is in the operations line above.
  - **Uplift: one assumption, stated once.** Wasl's measurement layer adds no attendance; the uplift comes from its patient levers (reminders, navigation, automated referral, booking).
    - Our projection from published trial effects: ≤31-day completion 4.5% → 9.8% (+5.3 pp), and completion by day 120 22.5% → 44.1% (+21.6 pp).
    - The value model needs completion at all, not within 31 days.
    - **Base case: +13 pp, without paid navigators.** Rerunning our projection with only the automated levers (reminders RR 1.32 plus automated referral RR 1.68, overlap-discounted, 10,000 draws) gives day-120 completion of 22.5% → 40.7% (**+18.2 pp**, 95% interval 5.8–42.1). +13 pp is therefore a conservative pick within the automated-only range.
    - It also matches the PRECISE navigation RCT (+13.0 pp [6.5–19.4]). Funded human navigation is upside: the full projection with navigation is +21.6 pp.
    - Band: +5 to +22 pp.
    - **Per-lever effects (published, used in our projection):**
      - reminders: RR 1.32;
      - automated referral: RR 1.68 (JAMA IM 2026);
      - navigation: +8.5 pp (mFOCUS) to +13 pp (PRECISE);
      - teach-back and booking: no completion RCT, so they carry no effect in the projection.

      Overlap between levers is discounted; we can't simply add them.
    - **Regional evidence:** we found no GCC navigation or FIT-follow-up trial. The only Gulf RCT is SMS appointment reminders in Saudi Arabia (n=502), where non-attendance fell from 39.8% to 26.3%. That supports transferring the reminder component, not navigation. UK text reminders raised breast-screening uptake by 5.3 pp in a hard-to-reach population. The band stays wide until the pilot measures it locally.
  - **Stage-shift value model.** AED avoided per year = FIT+/yr × uplift × PPV × Δstage × cost gap.
    - **PPV:** 2.5% CRC per completed colonoscopy (SEHA, 11/436).
    - **Δstage:** 33.5% of screen-detected cancers are stage III–IV (Dutch FIT programme). Our base is ≈65% for cancers presenting without screening. Supporting figures: Abu Dhabi 67% "late" (2023); Oman ≈75% stage III/IV; non-screen-detected cancers in Europe 56.4%; Dubai 31.6% metastatic; Saudi MNG-HA 33% distant. Band: 56–75%.
    - **Survival stakes (Saudi MNG-HA registry, n=1,012):** 5-year CRC-specific survival is 79.9% localized, 63.3% regional, 20.3% distant.
    - **Cost gap:** ≈AED 96k first-year cost, stage IV vs I (Riyadh study). Range AED 48k–185k (the upper bound is US SEER-Medicare).
    - **Result:** ≈AED 742 avoided per extra colonoscopy completer (range 120–1,775).
    - **Base case at 5.7k FIT+/yr and +13 pp:** ≈740 extra completers a year, ~18 cancers found, ~6 of them at an earlier stage. That is ≈AED 550k a year (AED 89k–1.3M across the cost and Δstage range at +13 pp; AED 34k–2.2M across the full +5–22 pp band).
    - **Cost per outcome [A].** At the base case (5.7k FIT+, +13 pp, ≈3 deaths averted a year) and steady-state Wasl cost (year 2: AED 0.21–0.67M):
      - Wasl alone: ≈AED 70k–220k per death averted.
      - Including the extra colonoscopies (≈AED 3.0M), net of treatment savings (≈0.55M): ≈AED 0.9–1.0M per death averted.
      - **Life-years per CRC death:** 12.7 (Sweden 2019, 32,340 YLL / 2,543 deaths, national life table) to ≈21 (China 2020, standard life table). We use 13–21. No UAE CRC age-at-death data is published. UAE life expectancy at birth is 78.3 (WHO 2021), close to Sweden's; and the UAE FIT+ cohort is younger (screening starts at 40; most of the population is under 60). Both suggest the lower bound is conservative for the UAE.
      - That gives **≈AED 43k–77k per life-year all-in**: Wasl plus the extra colonoscopies, net of treatment savings. This is the headline figure.
      - Counting Wasl's cost alone gives ≈AED 3k–17k. That flatters Wasl, because most of the uplift comes from standard levers that Wasl orchestrates. KPI assurance (evidenced closure, sanction risk) is a separate value that isn't priced here.
      - **Threshold:** WHO-CHOICE counts under 1× GDP per capita as "highly cost-effective": UAE GDP per capita is $50.3k (World Bank 2024), about AED 185k. NICE's threshold is £20–30k/QALY. Both estimates fall under both thresholds. Life-years aren't QALYs, so this is indicative only.
    - **Break-even against year-1 cost:** +13–33 pp at 5.7k FIT+, or +6–15 pp at 12.6k. It isn't reachable at 1.8k.
    - **The colonoscopies themselves are mandated programme care** (5.5.4.2), billed by the providers that perform them, so they aren't Wasl's cost. The treatment savings above don't cover them, though, so at the health-system level the case is health gained per dirham, the same test screening itself must pass:
      - **Base, RCT-derived:** Cochrane attendance-adjusted CRC mortality RR 0.75 for FOBT screening. Applied to the 16 per 1,000 ten-year CRC death risk among FIT+ non-completers (Zorzi, Gut 2022), that is **≈4 deaths averted per 1,000 extra completers**, or ≈3 a year at the base case.
      - **Upper bound, observational:** Zorzi's completer vs non-completer gap (16.0 vs 6.8 per 1,000, HR 2.03) gives ≈9.
      - **Why we can't localise these inputs yet:**
      - DoH publishes no stage-by-stage CRC distribution; its only figure is "42% detected early, 2012–19".
      - The DoH Mandatory Tariff gives a DRG base rate of AED 8,500 (Basic product), but the colectomy weights are only in the tariff spreadsheet.
      - No GCC navigation trial exists. PRECISE used bilingual (English/Spanish) navigators in a US safety-net population; Wasl's language-concordant notices follow that design, but the transfer is still an assumption.
      - So these are sensitivity ranges, not forecasts. The pilot measures the local uplift, and DoH can rebase stage and cost on registry and tariff data, which it holds. NordICC's 13-year per-protocol RR of 0.70 was not significant, which is why we don't use the colonoscopy-invitation trials for the base.
- **One headline rate:** the ≤31-day-from-referral KPI (sim truth 33.0%; Wasl measures 32.0%). The other completion rates come from the projection model and use other denominators (Appendix B).
- **Volume:** 1.8k–12.6k FIT+/yr. This can't be narrowed from public data: there are no FIT counts, and the signals conflict ("<10% screened" vs a "+280%" rise in screenings in 2025). AHS alone had ~1,429 FIT+ in 2015–16, which is the floor. Plan on 5.7k; the pilot's first deliverable is one count query on the Malaffi Screenings registry.
- **Contracting route:**
  1. A 90-day unpaid shadow pilot through the Future Health pathway, with ADPHC as programme owner and the DDGO assessment in parallel.
  2. At scale, one of two routes:
     - **(a)** ADHDS (the Malaffi operator, a PPP project company that is now part of M42) contracts Wasl directly as a vendor. DoH funds the scope through ADPHC's programme budget.
     - This is how ADHDS engaged every public Malaffi vendor: Orion ("commercial agreement", 2019), VitalHub (5-year partnership, 2024), and **Digi7, a UAE SME** (AI facial-biomarker collaboration announced at the G42 GITEX booth, Oct 2024).
     - None of these cites a tender. The PPP's subcontracting terms aren't public [A].
     - **(b) Fallback, if ADHDS declines:** a direct DoH/ADPHC contract under the Abu Dhabi Procurement Standards. Single-source is a "limited tender" approved under the Procurement Delegation of Authority. Abu Dhabi reserves 15% of government procurement for SMEs, with bid bonds suspended (Ghadan 21, 2020).
  - **Precedents:**
    - DoH–ADHDS–Philips MoU (17 Oct 2024): a "PoC… to integrate an AI algorithm into the existing Malaffi HIE", piloted at two hospitals before rollout. This is our template.
    - ADPHC buys third-party CRC services through intermediaries: Guardant Shield joined the CRC programme via Hikma, administered by M42 (30 Jan 2025).
    - ADHDS built Malaffi's 12 registries, including Screenings.
    - **Pilot-to-partnership precedent in Abu Dhabi:** BioTwin, a Hub71 startup, piloted breast screening at Cleveland Clinic Abu Dhabi (M42), announced Jan 2025. That led to a strategic partnership with Abu Dhabi Biobank (DoH/M42) in June 2026.
    - Dubai Future Accelerators produced Dh120m of pilot agreements with government entities, though DHA's health deals stayed at MoU stage.
    - Gap: no public case of a startup pilot becoming a paid *Malaffi* contract. Malaffi's vendors (Orion, VitalHub, Digi7) were contracted directly.

  - **Why ADHDS would want it:**
    - It turns the Screenings registry ADHDS already built from a list of tests into evidenced outcomes.
    - It fits the AI-in-Malaffi path DoH has already endorsed (the Philips PoC MoU) and M42's AI portfolio.
    - It is a recurring subscription per programme, billed through ADHDS.
    - It creates no new data flows outside the Malaffi boundary.
  - **Second door, on-premise for a provider network (no Malaffi change order):**
    - SEHA/PureHealth (14 hospitals, 70+ clinics, 3.5M+ patients) or M42 tracking its *own* patients counts as "related Healthcare Operations" under HIE 4.3.13.2.1.3, which includes "quality and improvement assessments".
    - Wasl runs on-premise as the network's processor and holds pointers and decisions, not copies. This respects 4.3.13.2.2.9 (no cloud storage) and .11 (no ingesting ADHIE data into its own system).
  - **Payer signal:**
    - Thiqa claims carry mandatory CPT codes, so a colonoscopy claim is independent evidence that the procedure happened. It can reopen or escalate a loop but never close one, because claims carry no caecum, prep or histology data.
    - DoH's Pay-for-Quality standard (June 2025) covers Thiqa facilities and TPAs, a future incentive hook.
  - **Other emirates:** NABIDH (Dubai) and Riayati (federal) have no loop-tracking product found.
  - **Primary route is (a).** ADPHC owns the requirement and budget. ADHDS signs the vendor contract, and DoH approves the scope change.
  - **Under (b):** the DoH procurement head and the PDoA holder approve the limited tender, and DoH signs.
  - **What's excluded:** a single facility buying the *cross-provider, programme-wide* product. That is a DoH public-health use (4.3.12.1), not that facility's treatment or operations (4.3.13.2). A network tracking **its own patients** across **its own facilities** is permitted operations use under 4.3.13.2.1.3: that is the second door, scoped to the network's patients only.
  - **Pricing [A]:**
    - Pilot: free (shadow mode).
    - Production:
      - integration as a fixed-fee statement of work (the integration line above);
      - an annual platform subscription per programme of ≈AED 90–200k. **This is a placeholder: pricing is a separate workstream.** The figure covering the operations line plus ~30% margin, model monitoring and support. No UAE public comparator exists. **UK G-Cloud 14 listings** (1 GBP ≈ 4.65 AED):
        - NEC AAA SMaRT, a screening programme with failsafe lists: £2.50 per transaction per year, ≈£30k (AED 140k) for one programme;
        - Eclipse population health: £0.14 per unit per year, ≈£70k (AED 326k);
        - Accenda Gateway referral management: £0.25 per person per year, ≈£125k (AED 581k) for 500k people;
        - Cinapsis SmartReferrals: £18k–£750k per licence per year.

        Typical one-programme cost is ≈AED 140k–700k a year, so our AED 90–200k sits at the low end. Annual figures depend on volume assumptions; units are as listed.
        - **Per loop at 5.7k FIT+/yr:** NEC ≈AED 25; Eclipse ≈AED 57; Accenda ≈AED 102; Wasl's subscription ≈AED 16–35. Each conversion assumes a single-programme estate, and the units differ, so read these as order-of-magnitude;
      - hosting billed at cost through ADHDS.
    - Breast and cervical add a subscription each, but no second integration.
    - The terms of the Digi7 and VitalHub deals aren't public, so there's no benchmark.
- **Funding precedent:** Malaffi is already free to providers and centrally funded.
- **Provider incentive:** the CRC, IFHAS and PHC standards require quarterly KPI reporting with sanctions; Wasl produces that evidence.
- **Capacity:** Wasl doesn't fund capacity; it quantifies the need. About 51–364 ring-fenced slots a week emirate-wide (20–146 on female lists), as an input to the DoH Healthcare Capacity Master Plan. Female-provider demand is real: 69.6% of FIT+ patients in the SEHA cohort were women, and some asked for a female endoscopist. Abu Dhabi's six government hospitals had two female colorectal surgeons in 2019 (The National). The simulator's 40% female-list share is an assumption.
- **Same engine, other programmes (rules from the DoH standards):**

  | Programme | Trigger | Clocks | Closure evidence | End-states |
  |---|---|---|---|---|
  | Breast | BI-RADS 0/4/5 recall | Result ≤15 working days (3.7.1); assessment offered ≤5 working days; needle biopsy result ≤5 working days; cancer referred to treatment ≤10 working days (3.9.11) | Triple assessment documented, then MDT outcome (3.9.8) | Return to routine; early recall (audited, 3.9.10); `closed_to_treatment` |
  | Cervical (run in the merged sim; uses a coded field) | ASC-US/LSIL/HSIL+ or HPV+ | Report ≤21 days (10.6.2); colposcopy HSIL+ 1–2 wk, LSIL 2–4 wk, ASC-US/H 4–8 wk (App 5); unsatisfactory → repeat 6–12 wk (10.6.4); visible lesion → immediate gyn-onc (10.6.6) | Colposcopy ± histology | Routine recall; treatment referral; `repeat_required` |

  Breast is specified, not run.
- 90-day pilot: one provider pair, shadow mode first. Exit criteria: 0 false closures; recall ≥0.95 vs the coordinator's manual list.
- **Pilot evaluation design (live phase, months 4–9):**
  - **Design:** stepped-wedge across 6–8 PHC clusters. Each cluster switches from usual care to Wasl outreach at a random 6-week step.
  - **Primary endpoint:** colonoscopy completion by day 120 among FIT+ referrals.
  - **Sample size:**
    - Baseline: day-120 completion of 22.5% (the status quo in our projection, calibrated to SEHA). Primary endpoint: +13 pp. An individually randomised trial would need ≈190 per condition (α 0.05, power 0.8).
    - The stepped-wedge is computed with Hussey–Hughes at an ICC of 0.01–0.05 [A], typical of primary-care cluster trials. **6–8 clusters, a baseline period plus 4 steps (5 periods)**, at ≈40 FIT+ patients per cluster-period, give ≈1,200–1,600 patient observations. **Hussey–Hughes power** (22.5% → 35.5%, α 0.05): with 6 clusters, 0.85 at ICC 0.01 and 0.81 at 0.05; with 8 clusters, 0.94 and 0.91. At only 20 patients per cluster-period, power falls to 0.54–0.73, so we would extend the steps.
    - Four clusters would be underpowered at the upper ICC, so we plan 6–8 PHCs.
    - If the volume isn't reached, the pilot reports an estimate with a CI rather than a test.
  - **Secondary endpoints:** ≤31-day KPI, days-to-notice, teach-back first-pass, contest rate, false closures.
  - **Scale-up decision:** the lower 95% bound of the uplift is above 0 and there are 0 audited false closures. Otherwise we stop, or redesign the patient levers.
- **Rollout [A]:**

  | Months | Step | Gate to the next step |
  |---|---|---|
  | 0–3 | Shadow pilot on 1 PHC → 1 endoscopy unit; DDGO pre-deployment assessment; ADHICS dossier | Pilot exit criteria met; DDGO clearance |
  | 4–6 | Live outreach at the pilot pair; coordinator training (~1 day) and clinician reviewer training (~2 h) | Contest rate and false-closure audit within limits |
  | 7–12 | SEHA ambulatory network, then private providers by FIT+ volume | Per-provider reader disagreement under threshold |
  | 12+ | Cervical, then breast | Post-deployment DDGO assessment |
- Then breast and cervical on the same engine.

**Demo (3 min):**
1. The patient story, then the 12% vs 33% undercount.
2. Arabic-named patient → notice → teach-back → report under a Latin name, no EID → linked, readers agree, closed.
3. Node kill: 0 duplicates or losses.
4. Reader disagreement held; a human signs; the chain verifies.
5. Booking shown in the separate simulator.

---

## Roadmap (fenced from the product above; build status marked per item)

### Phase 2: Queue control (direct-to-procedure and prep-risk built as simulated decision support; this is what makes Wasl raise completion, not just measure it)
  - **Direct-to-procedure:**
    - 36.3% of Abu Dhabi FIT+ non-completers were "referred to a gastroenterologist but did not attend" (SEHA, PMC8343888). This is the largest single local leak.
    - The DoH spec requires a referral form (5.5.1) and the App 9 pre-colonoscopy assessment, not a GI consult.
    - Wasl has a nurse complete App 9 from Malaffi data plus one call: anticoagulants, diabetes, devices, prior GI surgery, ASA. ASA ≥3 or anticoagulated patients go to clinic; everyone else is booked straight into a colonoscopy slot, with navigation.
    - Evidence: straight-to-test 13 vs 22 days to first test (Banerjea 2017); direct access non-inferior (62.7% vs 57.1%). Unnavigated direct access was *slower* in one US system (HR 0.59), so navigation is built in.
  - **Prep-failure prevention (the one lever that creates capacity):**
    - A prep-risk score from Malaffi data: diabetes, opioids or tricyclics, constipation, prior inadequate prep, ASA. The Dik score has AUC 0.77 internally and 0.62–0.69 externally.
    - Low-risk patients get an SMS sequence. SMS raises adequate prep (RR 1.10, 7 RCTs) and cuts no-shows (RR 0.74).
    - High-risk patients get a nurse call plus a clinician-ordered extended regimen (targeted extra prep: 94% vs 49% adequate).
    - Per 1,000 adequate exams, cutting inadequate prep from 15% to 10% frees 62 slots; from 20% to 5% frees 197.
    - The DoH cap is 10% inadequate (App 9.5), and Wasl reports each unit's rate against it.
  - **Network pooling (load-balancing across a group's sites):**
    - When Wasl runs across several facilities, it books a patient into any free endoscopy slot in the network, for example another SEHA or M42 site, within travel, gender and Ramadan constraints, instead of queueing them at the screening clinic's own unit. DoH 5.4.1 already encourages online booking.
    - **Simulated (30 draws), with capacity in the wrong places:** the capital holds 40% of slots but has 55% of patients, and Al Ain has spare.
      - At 1.2 slots per FIT+ (enough in total), completion within 31 days of referral, for all FIT+, is 22.2% with no cross-site booking, 37.2% at 30% travel willingness, and **42.1% with network pooling** (60% willing). That beats the balanced network's 40.4%.
      - At 1.0 slot per FIT+ (short overall), pooling gives 7.8% vs 7.3%. It only spreads the shortage.
    - **So network pooling fixes misallocated capacity, not missing capacity.** The more facilities run Wasl, the more misallocation it can absorb. Travel willingness is assumed; the pilot measures it.
    - Ontario's central positive-FIT booking cut the mean wait from 99 to 44 days (confounded by a test switch); a systematic review found 5–47 days gained.
    - It needs per-unit slot feeds, which the buyer holds.
  - **f-Hb ordering:** used only as a tie-breaker within the 31-day window. CRC PPV rises from 14 to 90 per 1,000 from f-Hb 20–39 to ≥180 µg/g; labs must record the value (4.13.3). No programme schedules by f-Hb, so it reorders the queue; it doesn't add capacity.

### Phase 3: One engine for the programme's other follow-up loops (designed; this fixes the low-volume economics)
  - Each new loop type needs only a closure rule, an evidence schema and clocks. The linkage, gate, readers, owners and SLAs are reused.
  - Published follow-up failure rates:
    - incidental lung nodules: 38% appropriately followed;
    - adrenal incidentalomas: 34% imaged;
    - abnormal labs: 6.8–62% not followed;
    - diabetic-retinopathy referral: 55–69% attended;
    - Lung-RADS 3–4: 27.6% adherent.
  - **Rough emirate volume [A]:**

    | Loop type | Loops a year |
    |---|---|
    | Incidental imaging | 10–30k |
    | Diabetic retinopathy | 6–12k |
    | IFHAS abnormal results | 30–60k (weakest estimate) |
    | Breast recall | 2–10k |
    | Lung LDCT | <0.5k |
    | CRC + cervical | 2–6k |
    | **Total** | **≈50–110k** (25–60× CRC) |

  - **Arithmetic behind each volume row [A]:**
    - **Incidental imaging:** Malaffi expected ~2M radiology images a year (Abu Dhabi Media Office). We assume 10–15% are CT and 5–10% of those carry an actionable finding.
    - **Diabetic retinopathy:** 4.14M population (SCAD 2024), ~3.3M adults. Assume 15% diabetic (IDF UAE adult prevalence 20.7%, discounted for the migrant age mix), half diagnosed, half screened, 5–10% referable.
    - **IFHAS:** 1M+ Thiqa members (Thiqa). Assume 300k screened a year and 10–20% referred. **This is a scenario, not an estimate**: no figure is published.
    - **Breast:** 34k screens in 2024 (Aletihad) at a 5–10% recall rate.
    - **Lung LDCT:** assume <5k screened, ~10% Lung-RADS 3–4.
  - **Without the IFHAS scenario:** 20–50k loops, giving ≈AED 11–70 per loop, which still overlaps manual tracing (AED 10–91). The case doesn't depend on the weakest row.
  - **Fixed cost per loop at 50–110k loops:** ≈AED 5–28, against AED 10–91 for manual tracing (labour only). At the midpoints (≈AED 12 vs ≈AED 50) that is comparable to or below manual tracing.
  - **Reviewer staffing at scale [A]:** CRC review takes 27 h per 1k loops and cervical 7.2 h. Assume 7–27 h for the other loop types: 50–110k loops ≈ 0.35–3.0k h a year ≈ **0.2–1.7 FTE**, centrally in ADPHC (AED ≈30k–300k a year).
  - **Order:** CRC and cervical (built) → breast (specified) → lung LDCT (DoH programme since 2017; Lung-RADS intervals) → incidental lung nodules (Fleischner) → diabetic retinopathy (SEHA's AI reading at 11 centres) → IFHAS.
  - **Competition:** no incidental-finding tracker (PowerScribe Follow-up Manager, Eon, Rad AI) has a public UAE deployment. Philips AI Manager, embedded in Abu Dhabi radiology via DoH/ADHDS, *detects* findings but doesn't track follow-up, so Wasl is the closure layer behind it.

---

**Limits (after v2 design):**
- Capacity still caps the ceiling. v2 creates slots (prep) and reallocates them (pooling, direct booking), but DoH funds the endoscopy lists.
- Queue levers are simulated decision support with trial-borrowed effect sizes. The image reader is hold-only and needs UAE recalibration. The LTT guarantee covers synthetic loops only. Only one extra loop type (lung nodules) is built; wider-loop volumes are estimates until registry queries.
- Simulated patients, reviewer, noise and capacity; no clinician labels. These are expected hackathon limits.
- The 26B reader ran via a third-party proxy, on public and synthetic text only. Production uses the in-UAE 4B.
- Malaffi's interface is inferred from public documents (HL7 v2); the HL7 adapter is built and tested against a mock, not a real feed.
- The 43 evaluation reports come from MTSamples. The site permits "print, share, or link… for educational purposes" with credit; the Kaggle CC0 tag is the uploader's own. Evaluation use only; never shipped.
  - **Replacement corpus spec:**
    - ~300 synthetic colonoscopy and pathology reports in the DoH App 14 field layout: indication, prep protocol and quality, extent and landmarks, findings, polyps, interventions, impression.
    - Variation: ungraded prep, incomplete extent, suspicious impressions, mixed Arabic/English, OCR noise.
    - Each is generated from a spec that is its own answer key, written and frozen before any reader is run.
- 16 messages were sent before the patient's contact-channel consent status was known. Fix designed: hold outreach until the status arrives. Not yet coded.
- Measurement doesn't raise the ~33% true rate; the trial-effect projection reached only 9.8%.


---

## Appendix A: first-contact notice templates (CRC FIT+ referral)

**Status:**
- English is the source text. Arabic and the five other languages are **machine-drafted by the team**.
- Before the pilot, every non-English version gets professional translation, independent back-translation, and cognitive testing with ~10 patients per language (ISPOR process).
- No version discloses a result. The notice states only that a follow-up test has been referred.

**Placeholders:** `[Name]`, `[Clinic]`, `[Owner]` (named coordinator), `[Phone]`.

**Readability:** the English source is 6 sentences averaging about 11 words, written in plain language. Flesch–Kincaid grade ≈6, estimated by us; it will be confirmed with a tool before the pilot.

## English (source)
Hello [Name]. Your clinic [Clinic] has referred you for a follow-up test (colonoscopy). Your care coordinator is [Owner], phone [Phone]. Please reply 1 to confirm, or 2 if you have a question. A computer system helps track your follow-up, and a person checks any unclear cases. Reply STOP to stop messages.

## العربية (Arabic)
مرحباً [الاسم]. أحالتك عيادتك [العيادة] لإجراء فحص متابعة (تنظير القولون). منسّق رعايتك هو [المسؤول]، هاتف [الرقم]. يرجى الرد بـ 1 للتأكيد، أو 2 إذا كان لديك سؤال. يساعد نظام حاسوبي في متابعة حالتك، ويراجع شخصٌ أي حالات غير واضحة. للتوقف عن تلقي الرسائل أرسل STOP.

## اردو (Urdu)
السلام علیکم [نام]۔ آپ کے کلینک [کلینک] نے آپ کو فالو اپ ٹیسٹ (کولونوسکوپی) کے لیے ریفر کیا ہے۔ آپ کے کیئر کوآرڈینیٹر [ذمہ دار] ہیں، فون [نمبر]۔ تصدیق کے لیے 1 لکھ کر جواب دیں، یا اگر آپ کا کوئی سوال ہے تو 2۔ ایک کمپیوٹر سسٹم آپ کے فالو اپ پر نظر رکھنے میں مدد کرتا ہے، اور غیر واضح معاملات کو ایک شخص دیکھتا ہے۔ پیغامات بند کرنے کے لیے STOP بھیجیں۔

## हिन्दी (Hindi)
नमस्ते [नाम]। आपके क्लिनिक [क्लिनिक] ने आपको एक फॉलो-अप जाँच (कोलोनोस्कोपी) के लिए रेफ़र किया है। आपके केयर कोऑर्डिनेटर [ज़िम्मेदार व्यक्ति] हैं, फ़ोन [नंबर]। पुष्टि के लिए 1 लिखकर जवाब दें, या कोई सवाल हो तो 2। एक कंप्यूटर सिस्टम आपके फॉलो-अप पर नज़र रखने में मदद करता है, और अस्पष्ट मामलों को एक व्यक्ति जाँचता है। संदेश बंद करने के लिए STOP भेजें।

## മലയാളം (Malayalam)
നമസ്കാരം [പേര്]. നിങ്ങളുടെ ക്ലിനിക് [ക്ലിനിക്] ഒരു തുടർ പരിശോധനയ്ക്കായി (കൊളോനോസ്കോപ്പി) നിങ്ങളെ റഫർ ചെയ്തിരിക്കുന്നു. നിങ്ങളുടെ കെയർ കോർഡിനേറ്റർ [ചുമതലയുള്ളയാൾ] ആണ്, ഫോൺ [നമ്പർ]. സ്ഥിരീകരിക്കാൻ 1 എന്നും, എന്തെങ്കിലും ചോദ്യമുണ്ടെങ്കിൽ 2 എന്നും മറുപടി അയയ്ക്കുക. നിങ്ങളുടെ തുടർ പരിചരണം നിരീക്ഷിക്കാൻ ഒരു കമ്പ്യൂട്ടർ സംവിധാനം സഹായിക്കുന്നു; വ്യക്തമല്ലാത്ത കേസുകൾ ഒരു വ്യക്തി പരിശോധിക്കുന്നു. സന്ദേശങ്ങൾ നിർത്താൻ STOP എന്ന് അയയ്ക്കുക.

## বাংলা (Bengali)
প্রিয় [নাম], আপনার ক্লিনিক [ক্লিনিক] আপনাকে একটি ফলো-আপ পরীক্ষার (কোলোনোস্কোপি) জন্য রেফার করেছে। আপনার কেয়ার কোঅর্ডিনেটর [দায়িত্বপ্রাপ্ত ব্যক্তি], ফোন [নম্বর]। নিশ্চিত করতে 1 লিখে উত্তর দিন, অথবা কোনো প্রশ্ন থাকলে 2। একটি কম্পিউটার সিস্টেম আপনার ফলো-আপ অনুসরণ করতে সাহায্য করে, এবং অস্পষ্ট ক্ষেত্রগুলো একজন মানুষ পরীক্ষা করেন। বার্তা বন্ধ করতে STOP পাঠান।

## Tagalog
Magandang araw, [Pangalan]. Ini-refer ka ng iyong klinika na [Klinika] para sa isang follow-up na pagsusuri (colonoscopy). Ang iyong care coordinator ay si [Responsable], telepono [Numero]. Mag-reply ng 1 para kumpirmahin, o 2 kung may tanong ka. Isang computer system ang tumutulong sa pagsubaybay sa iyong follow-up, at isang tao ang sumusuri sa mga hindi malinaw na kaso. Mag-reply ng STOP para itigil ang mga mensahe.


---

## Appendix B: completion rates and their denominators


  | Figure | Denominator | Clock | Source |
  |---|---|---|---|
  | 30.5% | FIT+ (AHS 2015–16) | Ever, median 72 d | SEHA, BMC Cancer 2021 |
  | 33.0% | Referred FIT+ in our merged sim | ≤31 d from referral | Sim truth (its cohort parameters differ from the projection's) |
  | 22.5% → 44.1% | FIT+ in the trial-effect projection | ≤120 d | Projection calibrated to the SEHA curve |
  | 4.5% → 9.8% | Same projection | ≤31 d | Same |

  The merged sim and the projection are separate models, and only the projection is calibrated to SEHA's 72-day median. We cite each figure with its own denominator.
