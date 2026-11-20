# Wasl product roadmap

**Challenge:** Closing the Screening-to-Care Loop. **Status labels:** **Built** = implemented and exercised in code. **Simulated** = measured on synthetic data. **Designed** = planned, not yet done. Detail and sources: `docs/SUBMISSION-DRAFT.md`, `docs/EVIDENCE.md`, `docs/ROADMAP.md` (v2 technical limits).

## 1. Fit against the challenge's desired outcomes

| Desired outcome | What Wasl does today | Status | Gap the roadmap closes |
|---|---|---|---|
| **End-to-end case visibility** | One loop per actionable result, from issuance through notification, acknowledgement, referral, booking, procedure, histology and evidenced closure. Arabic–Latin identity linkage follows the patient across providers when the Emirates ID is missing. Today/Patients/Audit views show every state and its evidence. | Built; measured on synthetic data | Live Malaffi/HL7 feeds (mock-tested only) |
| **Clear ownership and timely progression** | Every state has a named owner and a clock (31-day DoH colonoscopy window, referral-response SLA). Overdue, rejected and unowned cases have their own queues ("No owner", rejected-referral hand-off). | Built | SLA values agreed with the programme |
| **Patient-centred communication** | Owner-named notices, teach-back, consent holds, opt-out, contest and explanation rights; outreach only with recorded contact permission. | Built; English/MSA/Gulf/Arabizi teach-back tested | Professional translation and cognitive testing; patient co-design |
| **Cross-provider consultation and coordination** | Referral packets, referral acceptance and rejection tracking, consultation routing, cross-site booking with region, gender and Ramadan constraints. | Built (booking and pooling simulated) | Clinician-to-clinician consult channel inside the provider's workflow |
| **Safe, human-governed escalation** | A loop closes only on documented evidence agreed by two readers, or a named human decision. Suspected malignancy alerts the MDT without waiting for histology. 0 of 3,035 CRC false closures in the frozen held-out evaluation (95% upper bound 0.12%). | Built; simulated evaluation | Clinical safety case signed by a local clinician |
| **Closure and patient impact measures** | Measured rate raised toward the true rate: per 1,000 FIT+, wrongly flagged "not followed up" drops from 207 to 31. Drop-off reasons, time-to-closure and review workload on the Programme page. | Simulated | Real baseline and uplift, measured in the pilot |

## 2. Phases

### Phase 0 — Now (built)
Seven programmes (colorectal, cervical, breast, lung LDCT, lung nodule, diabetic retinopathy, IFHAS) on one closure engine; hosted demo; one-command reproducible evaluation (`make verify`).

### Phase 1 — Shadow pilot (months 0–3)
- **Where:** one primary-care clinic and one endoscopy unit, colorectal only.
- **What:** Wasl runs beside the coordinator's manual list and changes nothing; every closure is a suggestion a clinician confirms.
- **Build:** live HL7 feed from both sites; on-premise deployment inside the provider network; Arabic notices professionally translated.
- **Exit gate:** zero false closures; recall ≥ 0.95 against the manual list; DoH Responsible AI conformity assessment started.

### Phase 2 — Assisted closure (months 3–9)
- **What:** after 300 consecutive confirmed-correct closures, automatic closure with a random 10% clinical audit; any error returns that phrasing class to shadow mode.
- **Build:** local recalibration of the report and image readers on UAE reports; clinician co-design of notices and the referral packet; staff training.
- **Add:** cervical and breast at the same sites.
- **Exit gate:** zero audited false closures; coordinator hours per 100 loops measured against the manual baseline.

### Phase 3 — Measured outreach (months 9–18)
- **What:** stepped-wedge rollout across 6–8 primary-care clusters with live patient outreach and navigation.
- **Primary endpoint:** colonoscopy completion by day 120 (estimated power 0.81–0.94).
- **Secondary:** time to notification, referral acceptance, appointment completion, unresolved results, drop-off reasons, patient understanding (teach-back).
- **Scale gate:** lower 95% bound of the uplift above zero, and zero audited false closures.

### Phase 4 — Emirate scale and more loops (months 18–36)
- **What:** emirate-level module for the Abu Dhabi Public Health Centre via ADHDS/Malaffi; pooled regional booking queue.
- **Add loops, in order:** lung LDCT → incidental lung nodules → diabetic retinopathy → IFHAS abnormal results (≈50–110k loops a year across the emirate).
- **Expand:** NABIDH (Dubai) and Riayati (federal) as later markets.

### Authentication in production
Staff sign in through the provider's SSO or UAE Pass, and every sign-off is bound to a named clinician. The open demo persona switcher, where any visitor can act as a demo reviewer or clinician, exists only in the hosted demo. **Designed**; the web app's OIDC sign-in mode is built and tested against a synthetic provider.

## 3. How each phase answers the assessment

| Criterion | Evidence now | Next proof |
|---|---|---|
| Health impact and clinical relevance | 207 → 31 wrongly flagged per 1,000 FIT+; DoH 31-day standard | Day-120 completion uplift (Phase 3) |
| Technical depth and architecture | Go/NATS pipeline, HL7 v2.5.1 + FHIR R4, Arabic–Latin linkage (FEBRL4 F1 0.919 vs 0.870 baseline), Learn-then-Test gate | Live feed and on-premise deployment (Phase 1) |
| Working solution and demo quality | Hosted demo, reproducible evaluation | Shadow-run against a real coordinator list |
| Responsible AI, safety and data governance | Self-classified High risk; hazard log; two-reader gate; hold-only image reader; hash-chained audit | Conformity assessment and clinical safety sign-off |
| Scalability and deployment readiness | One engine for seven programmes; in-UAE processing design | Second site, then emirate module |
| Commercial viability and path to pilot | Estimated AED 4–28 per loop at emirate volume vs AED 10–91 manual tracing | Unpaid 90-day shadow pilot → paid programme module |

## 4. What the roadmap cannot promise
- Software does not create endoscopy slots; capacity still caps the ceiling.
- Every route runs through a regulated buyer (ADPHC/ADHDS or a provider network).
- Volumes for the wider loops are estimates until registry queries in Phase 4.
