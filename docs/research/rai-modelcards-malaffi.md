# Wasl: RAI controls, model cards, Malaffi ingestion

Evidence files:
- **M** `pipeline/RESULTS.md` (merged run)
- **P** `pipeline/v7-gate/RESULTS.md` (v7 gate)
- **B** `booking/RESULTS.md`
- **G4** `reader/RESULTS-gemma4b.md`
- **IMP** `patient-parsers/` (teach-back and e-consult scores)
- **S, V2, V3, GOV, V5:** earlier experiment runs (scale, encoder v2/v3, governance, integration v5). Their results are summarised in the docs, and their files aren't in this repo.

All data are synthetic or public, and the reviewer is simulated.

## 1. RAI Standard (DoH/ST/DDGO/RAI/V1/2025)

Only 3.3.1.2.2.2 is specific to High risk; 3.1.8 also covers "high-impact" systems. All other "must" clauses apply to every AI system.

| Clause | Wasl | Status |
|---|---|---|
| 3.3.1.2.2.2 High risk: "conformity assessments both prior to and following deployment", with DDGO | Pilot step | **Gap** |
| 3.1.8 "Sustainability impact assessments… high-impact AI" | 0.74 H100-h/1k loops (S); no energy report | **Gap** |
| 3.3.1.1.1–3, 3.3.1.4.1 Risk register, owners, communicate to DoH | Hazard log H1–H12 with controls and owner roles (WRITEUP §RAI) | **Evidence** |
| 3.1.1.2.1–2 Human-in-the-loop, escalation | CRC recall 0.242 without review, 0.960 with; 0/570 false closures (M) | **Evidence** |
| 3.1.1.1.1 Co-design with clinicians and patients | None | **Gap** |
| 3.1.2.1.1–2 Robustness, degradation | 0/15,769 duplicates or losses across 4 kills (M) | **Evidence** |
| 3.1.2.1.3–4, 3.1.3.1.3–4, 3.2.3.3 Monitoring, alerts | Triggers in §2 | **Designed** |
| 3.1.3.1.1–2 Validation, diverse data | Synthetic data; frozen 5-seed × 2-noise-model evaluation run (`pipeline/results/final`) | **Evidence (synthetic)** |
| 3.1.3.1.5, 3.1.6.2.3, 3.1.7.1.3, 3.1.7.2.3 Traceability, override | Hash-chained audit; tamper 17/17 (GOV) | **Evidence** |
| 3.1.4.2.1–3 RBAC, MFA | 24/24 forged events denied (GOV); MFA not built | **Evidence / Gap** |
| 3.1.4.3.1 Secure code review | None | **Gap** |
| 3.1.4.4.1, 3.1.6.1.3 Privacy, opt-out | 0 sends after opt-out; consent hold built, 0 sends without granted consent in every frozen run (M) | **Evidence** |
| 3.1.4.4.2 Encryption; "Production data shall not be used to train" | Public and synthetic training data only; JetStream store encrypted at rest (ChaCha20-Poly1305, key from env/HSM), no plaintext identifiers on disk (M); TLS in transit not yet configured | **Evidence / Gap** |
| 3.1.4.4.3 Anonymisation, DP, federated learning | Linkage needs identifiers | **Gap** |
| 3.1.4.5.1 Compute in the UAE | Local 4B reader (G4); the 26B used a proxy | **Designed** |
| 3.1.4.5.2–3, 3.1.4.6 Network, IDS, physical | Assumed from ADHDS hosting | **Gap** |
| 3.1.5.1.1–3 Representativeness, bias | By origin (V3, M), sex (B); no age or socioeconomic data | **Evidence / Gap** |
| 3.1.6.1.1–2 Disclose AI use | Notices don't disclose it | **Gap** |
| 3.1.6.3 Role-specific explanations | Reviewer sees p, quote, chain (V5) | **Designed** |
| 3.1.7.1.1–2, 3.1.7.2.2 Owners; clinician authority | Role matrix; clinician signs evidence (GOV) | **Designed** |
| 3.2.1.5 Open-data licence review | Wikidata CC0; MTSamples not reviewed | **Gap** |
| 3.2.2.1 Attributes used | Scorer uses sex, script, DOB (`text.go`); origin used for reporting only | **Evidence** |
| 3.2.3.1.1 Clinically validated labels | One non-clinician labeller | **Gap** |
| 3.2.4–3.2.5 Versioning, datasheets | Labels hash-frozen (IMP); no datasheets | **Designed** |
| 3.4.1 AI literacy | None | **Gap** |

## 2. Model cards

The owner of each model is our **assumption**, using RAI §4 roles: the ADPHC programme lead as Product Owner, plus a Data Scientist.

**Name encoder** (char dual-tower transformer, Go, 2.3 MB)
- **Use:** proposes Arabic↔Latin candidates.
- **Out of scope:** making link decisions; other scripts.
- **Training data:**
  - Wikidata (CC0), 38.5k persons;
  - retrained on 13.5k mixed-origin names plus 2.7k LLM-written Arabic forms (V2, V3).
- **Evaluation:**
  - dev R@10 0.954;
  - R@10 0.932 vs 0.604 for e5-small (P);
  - HNSW recall@10 0.9992 at 1M records (S).
- **Failure modes:**
  - trained on the eval's own noise model;
  - LLM-written South Asian names flatter the F1 (.975 vs .946);
  - in V5, 9 of 10 missed closures had an Arabic-script name at referral, and 6 were South Asian.
- **Trigger** (proposed): R@10 below 0.95 on human-confirmed links.

**Pair scorer** (LightGBM with a stratified isotonic calibrator)
- **Use:** a calibrated p per link. Only p≥0.99 is trusted.
- **Training data:** Wikidata pairs.
- **Evaluation:**
  - F1: Arab .955, South Asian .952, Filipino .963, Western .957 (V3);
  - 0/5,142 wrong at p≥0.99; ECE .051 (P);
  - CRC precision 1.000, recall 0.960 (M).
- **Subgroups:** review items per 100 loops: Arab 39.8, South Asian 34.9, Filipino 34.6, Western 43.5 (M).
- **Failure modes:**
  - a same-name cousin linked via an EID chain; the DOB guard cuts wrong-patient sends from 6.7 to 0 per 10k (P);
  - thresholds were chosen after seeing the eval.
- **Oversight:** two reviewers below τ; outreach held.
- **Triggers:** any audited false merge at p≥0.99; ECE above 0.05.

**Evidence readers** (regex/DoH rules plus Gemma 3 4B or 26B, zero-shot)
- **Use:** extracts caecum, prep, polyps and histology. Closure needs both readers to agree.
- **Out of scope:** diagnosis.
- **Data:** 300 synthetic DoH App 14 reports (spec = answer key) and 43 US MTSamples reports, team-labelled.
- **Evaluation:**
  - App 14 corpus, after the v3 fix: 26B 300/300 on every field, 4B 264–289/300; 0 unsafe auto-closures for either; frozen evaluation 0 CRC false closures with either reader;
  - before the fix the corpus showed a field-definition gap (biopsies not counted as tissue) giving 50–52 false closures with either reader;
  - 4B hybrid 40/43 against the MTSamples key; 13 auto-closures, 0 wrong (G4);
  - 0/455 agreed labels wrong (V5).
- **Failure modes:**
  - the 4B over-calls histology;
  - each text was reused 17–44 times;
  - tested only on synthetic Arabic/English headings, not real Abu Dhabi reports.
- **Oversight:** clinician on disagreement; double review on "adequate".
- **Triggers:** disagreement rate rises; any false "adequate"; a new report template.

**Teach-back parser** (Gemma 4 26B)
- **Use:** classifies patient replies as understood, partial, misunderstood or needs-human.
- **Out of scope:** clinical advice.
- **Data:** 100 items in EN, MSA, Gulf, Arabizi and code-switched text; one non-clinician labeller.
- **Evaluation:** 98/100 and 0/50 missed escalations, vs a tuned keyword baseline at 85/100 and 3/50 (IMP).
- **Gap:** the local 4B reader is untested on this task.
- **Oversight:** nurse callback.
- **Trigger:** any missed escalation.

**No-show predictor** (logistic, `booking/model.py`, not trained)
- **Use:** sets overbooking and nurse calls.
- **Out of scope:** deprioritising patients.
- **Coefficients:**
  - sourced: base rate 0.174 (SEHA), prior no-show OR 3.8, nurse-call OR 0.67;
  - several are assumed.
- **Evaluation:** simulation only, where truth is drawn from the same model family. Removing overbooking cuts the all-FIT+ rate from 52 to 49.4 (B).
- **Subgroups:** women needing a female endoscopist 79 vs men 97; Al Dhafra 88.
- **Oversight:** the scheduler.
- **Trigger:** observed no-shows outside the predicted band.

## 3. Malaffi capability and fallback

**Publicly documented:**
- Connection requires "exchanging ADT… HL7 messages in real-time" (https://www.malaffi.ae/providers/definitions/).
- Onboarding needs an HL7 EMR, an Etisalat SD-WAN link and an ADHICS assessment (https://www.malaffi.ae/providers/facilities-required-to-connect/).
- The HL7 specs are gated behind a form (https://www.malaffi.ae/providers/resources/).
- The HIE Policy names "HL7 (version 2.x or 3.0)" and does not mention FHIR (https://staticcdn.malaffi.ae/wpblobe9e5523f25/2020/04/doh-policy-on-the-adhie-april-2020.pdf).
- DoH HIE Standard 5 puts LOINC codes "in the OBX segments", which implies ORU (https://www.doh.gov.ae/en/resources/HIE; seen as a search snippet only).
- The platform is Orion Health Amadeus (https://orionhealth.com/us/media-releases/orion-health-delivers-the-first-health-information-exchange-hie-in-the-middle-east/).

**Unknown:**
- FHIR version or CapabilityStatement;
- Subscriptions;
- IHE XDS/PIX/PDQ;
- MLLP;
- a developer portal.

One vendor claims there is a Malaffi FHIR implementation guide (https://www.bonamisoftware.com/malaffi-integration; low confidence).

**Adapter tiers.**
- **Key:** `source|resourceType|id|version`, sent as `Nats-Msg-Id`.
- **JetStream dedupe window:** 1 h.
- **Older replays:** caught by a KV create-if-absent seen-ledger (**assumption**: not yet built).

| Tier | Exactly-once |
|---|---|
| (a) R4 Subscription rest-hook | Event numbers plus `$events` backfill. Mock run: 20,906 versions exactly once despite 392 drops, 568 duplicates and 682 reorders (S) |
| (b) Poll `_lastUpdated` from the watermark minus an overlap | Watermark moves only after the publish ack; re-reads are deduped. Version = `meta.versionId`, else a content hash (assumption). Designed |
| (c) HL7 v2 ORU/ADT via MLLP | ACK only after the publish ack. Key = MSH-4 + OBR-3 + result status, so corrections become new versions. Designed |
| (d) Nightly `$export` | Same key; only missed versions pass through. Designed; Bulk Data support unknown |

**Limit:** the same result arriving as both an ORU and a FHIR resource gets two keys. The state machine should treat it as a duplicate fact, but this is untested.
