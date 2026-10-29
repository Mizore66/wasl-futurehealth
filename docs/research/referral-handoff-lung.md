# Rejected-referral hand-off packet and lung-nodule loop gaps: sources

Compiled 30 Oct 2026 from public web sources only. Every number has a source (numbered, list at end) and a
transfer note for Abu Dhabi. Anything without a source is marked **ASSUMPTION**. "BD" = business day; the
DoH ADHIE policy defines a Business Day as any day other than Friday, Saturday or a public holiday [S14].

> Implementation notes: the hand-off packet is `pipeline/handoff.go`. Business days use the UAE Monday-Friday working week in force since January 2022, not the Friday-Saturday weekend in the 2020 ADHIE policy definition; public holidays are not modelled.

---

## Part A. Rejected-referral hand-off packet

### A1. What a structured rejection / redirect contains

| System | Decision options | What the rejecting side must send | Transfer to Abu Dhabi |
|---|---|---|---|
| NHS e-RS, Referral Assessment Service [S1][S2] | "Return to Referrer with Advice"; redirect to another service; accept | Reason the referral is returned (redirect needed, more information needed, or suitable for primary care) plus comments explaining why no appointment is needed and **onward management instructions** for the referring clinician | Good model for content. NHS is a single-payer contract setting; Abu Dhabi is mixed public/private, so "who acts next" must be explicit in the packet |
| NHS joint e-RS guidance [S3] | Reject (about 2% of referrals) vs re-direct (preferred) | Explain clearly why the service is not suitable and **suggest an alternative provider or way of managing the patient**; after a rejection the referral goes to the practice "Referrer Action Required" worklist; on a re-direct the GP does nothing | The 2% rate is NHS-specific; use only as a low anchor |
| NHS e-RS Advice and Guidance [S4] | Return with advice; ask referrer for more info; convert to referral (only if referrer authorised) | Provider must pick one of four "intended outcome" codes (for audit) | Shows outcome coding is mandatory, not free text only |
| Ocean eReferral, Ontario [S5][S6] | Accept / Decline; sender can resubmit or redirect (optional "Reason for Redirect") | Decline requires a **coded reason from a dropdown** plus an editable **message for the referrer**; coded values: `SENT_TO_WRONG_PROVIDER`, `SERVICE_NOT_AVAILABLE`, `UNABLE_TO_CONTACT`, `UNLIKELY_TO_BENEFIT`, `TECHNICAL_DELIVERY_ISSUE`. Over FHIR, the explanation goes in `Task.businessStatus.text` | Small, closed reason list transfers well; codes are Ocean-specific, not a standard |
| CPSO Transitions in Care, Ontario (regulator policy) [S7] | Accept / decline | If declining, consultant **must communicate the reasons** to the referrer; if declining because a sub-specialist is needed, the referrer is responsible for the new referral | Regulator-level rule; closest analogue to a DoH standard |
| FHIR R4 Task [S8][S9] | Task.status `requested, received, accepted, rejected, ready, cancelled, in-progress, on-hold, failed, completed, entered-in-error` | `rejected` = "potential performer who claimed the task has decided not to execute it before taking any action"; `Task.statusReason` = why the task is held, failed or refused (no standard code set) | Directly usable. ServiceRequest.status has **no** `rejected` code (draft, active, on-hold, revoked, completed, entered-in-error, unknown) [S9], so rejection lives on Task |
| Ontario eReferral FHIR IG v0.12 [S10] | Task profile with `statusReason` and a `businessStatus.extension:StatusReason` slice | Coded decline reason on Task | Evidence that a national IG puts decline reason on Task, not ServiceRequest |
| IHE 360X closed-loop referral [S11][S12] | Accept, Decline, Interim, Outcome, Cancel | Decline is sent Referral Recipient to Initiator and **ends** the referral at the receiver; the referral ID must appear in ORC and OBR of the HL7 v2 messages; HL7 v2 payload for discontinue is `OSU^O51` (the public IG pages were 403 for us, so exact ORC codes for decline are **not verified**) | Useful only if Malaffi carries HL7 v2 order messages between providers; not confirmed public |
| DoH Abu Dhabi, Standard for Primary Healthcare Services, Sep 2022 (DOH/HPI/SS/2/2022) [S13] | n/a (no rejection workflow defined) | Referral must state reason, relevant clinical information and expected outcome (opinion only, treatment, shared care) (4.9.4); **PCP remains responsible for follow-up unless another physician has accepted it** (4.9.5); facility needs a care coordinator who tracks follow-up of screening results (4.9.1, App. 3); referrals to run through the Virtual Care Platform (App. 4) | Native rule. 4.9.5 means a rejected referral hands responsibility back to the PCP by default |
| DoH 2017 Healthcare Providers Manual [S15] | cites a "DOH Patient Referral Standard" | Not found as a current public document | Gap: no public DoH rule on rejection content found |

**No public DoH or Malaffi rule was found that defines rejection content, rejection codes, or a response time for
referrals in general.** The only DoH referral clock found is in the Specialized Mental Healthcare standard
(urgent 2 to 3 working days, routine 10 working days, via search snippet only [S16]).

### A2. Deadlines

| Clock | Value | Source | Transfer |
|---|---|---|---|
| Receiving consultant actions a routine referral (accept/return/redirect) | 5 working days | NHS England elective SPoA standards, as summarised by RCGP (page updated Jul 2026) [S4b] | Recent national NHS rule; plausible for Abu Dhabi but not binding |
| Receiving consultant actions an urgent referral | 2 working days | same [S4b] | as above |
| Advice and guidance response | 5 working days (older local norm: 80% within 2 working days) | [S4b]; NCL ICB [S4c] | as above |
| Consultant must acknowledge and say accept/decline | no later than 14 days from receipt | CPSO policy provision 17-18 [S7] | Hard outer bound from a regulator |
| DoH specialised mental health referral response | urgent 2-3 working days; routine 10 working days | DoH standard, search snippet [S16] | Abu Dhabi-native but service-specific |
| Suspected-cancer referral reaches specialist care | within 14 calendar days | DoH PHC standard Appendix 1 (recommended indicator, not a mandated JAWDA KPI) [S13] | Abu Dhabi-native; directly relevant to nodule >8 mm |
| Referrer acting on a returned referral | **no published deadline found** (NHS: "responsibility rests with the referrer" [S3]; CPSO: referrer must track urgent referrals [S7]) | - | Gap; must be ASSUMPTION |
| RTT clock on return to primary care | Clock stops when it is communicated to patient and GP that return to primary care is clinically appropriate (Rule 5(a)) | NHS RTT rules Oct 2022 [S17] | Shows the patient must be told, not only the GP |

### A3. Proposal

**Minimum packet fields** (FHIR Task with focus = ServiceRequest; mirror into HL7 v2 only if needed)

| # | Field | Basis |
|---|---|---|
| 1 | `referral_id` (ServiceRequest id; also the ID in ORC/OBR if v2) | 360X [S12] |
| 2 | `decision`: reject, redirect, return_for_info, conditional | e-RS options [S1][S4]; Wasl states |
| 3 | `reason_code` from a closed list: wrong_provider, service_not_available, unable_to_contact, unlikely_to_benefit, technical, needs_more_info, manage_in_primary_care, needs_subspecialist | Ocean list [S6] + e-RS reasons [S1] + CPSO sub-specialist case [S7] |
| 4 | `advice_to_referrer` (free text, required) | e-RS onward management instructions [S2]; CPSO 18(b) [S7] |
| 5 | `suggested_alternative` (service or provider; required when decision = redirect) | NHS joint guidance [S3] |
| 6 | `info_requested` (list; required when decision = return_for_info or conditional) | e-RS [S1][S4] |
| 7 | `clinical_priority` copied from the original referral, plus `loop_id` and the originating recommendation (e.g. Fleischner ">8 mm: PET/CT, tissue or CT at 3 months") | DoH PHC 4.9.4 requires reason and clinical info [S13]; loop link is **ASSUMPTION** |
| 8 | `responsible_clinician_after_decision` (defaults to referring PCP) | DoH PHC 4.9.5 [S13] |
| 9 | `received_at`, `decided_at`, `decided_by` (clinician + facility licence id) | CPSO 14-day clock needs both timestamps [S7] |
| 10 | `patient_informed` (bool + channel + timestamp) | RTT Rule 5(a) communication to patient [S17]; DoH PHC 4.10.2 patient notification duty [S13] |
| 11 | `outreach_consent_status` (from the treating facility, never inferred) | UAE Law 2/2019 Art 16 [S20]; ADHIE policy 4.2 [S14] |
| 12 | `act_by` deadline and `escalation_target` | computed, see below (**ASSUMPTION**) |

**Deadline for the receiving referrer to act** (re-refer, manage in primary care, or close with reason):
- Urgent / suspected cancer (nodule >8 mm solid): **2 BD** after the packet lands. ASSUMPTION, mirrored from the NHS 2-working-day urgent standard [S4b]; it keeps the whole path inside the DoH 14-calendar-day suspected-cancer indicator [S13].
- Routine: **5 BD**. ASSUMPTION, mirrored from the NHS 5-working-day routine standard [S4b].

**Escalation step** (ASSUMPTION built on DoH roles):
1. At `act_by`: task to the referring facility's care coordinator (DoH PHC 4.9.1 / App. 3 require one and give them follow-up tracking) [S13].
2. At `act_by` + 2 BD (urgent) / + 5 BD (routine): escalate to the referring facility's medical director (DoH PHC 5.2.1 requires one) [S13].
3. Any urgent loop still open at day 14 from the original recommendation: flag on a DoH-facing report as a breach of the suspected-cancer 14-day indicator [S13]. Wasl itself cannot sanction; DoH escalation steps exist only for HIE non-compliance [S14].

---

## Part B. Lung-nodule loop gaps

### B1. Closing evidence for a solid nodule >8 mm

| Guideline | Recommendation | What counts as "work-up done" | Transfer |
|---|---|---|---|
| Fleischner 2017, single solid >8 mm (low and high risk) [S21][S22] | "Consider CT at 3 months, PET/CT, or tissue sampling"; any one or a combination may be appropriate depending on size, morphology, comorbidity | Any one of: chest CT about 3 months later, PET/CT, or tissue sampling. Multiple solid >8 mm: CT at 3-6 months then 18-24 months | Applies to incidental nodules in adults. Fleischner excludes lung-cancer screening, patients under 35, immunocompromised patients and known cancer (from the guideline scope) [S21]; Wasl's incidental loop should keep these exclusions |
| BTS 2015 (Callister, Thorax 70 Suppl 2) [S23][S24] | Nodule ≥8 mm (or ≥300 mm³): Brock risk; Brock >10% → offer PET-CT (if above local PET size threshold); Brock <10% → CT surveillance | After PET-CT, reassess with Herder: <10% surveillance, 10-70% image-guided biopsy favoured, >70% excision or non-surgical treatment | Brock and Herder were validated in UK/Canadian cohorts; Gulf calibration unknown. For simulation, BTS adds "PET done, then biopsy or resection" as a second closing step |
| ACR Appropriateness Criteria [S25] | FDG-PET/CT whole body "usually appropriate" as next imaging for solid indeterminate nodules >8 mm | PET/CT | US guidance, consistent with Fleischner |

**Proposed closure rule** (ASSUMPTION built on the above): the >8 mm loop closes on the first of (a) a chest CT
result 60-120 days after the index CT, (b) a PET/CT result, (c) a pathology/cytology result on a lung specimen, or
(d) a documented specialist or MDT decision to discharge. If PET/CT is the closer, open a child loop for
biopsy/resection when the report or MDT note says so (BTS Herder step [S23]).

### B1b. Typical intervals from recommendation to PET/CT or biopsy

| Interval | Value | Source | Transfer |
|---|---|---|---|
| CT to PET | median 11 days | Albano et al., Lung 2020, Stony Brook nodule programme, 2013-2019 [S26] | Organised nodule programme in a US academic centre; a best case for Abu Dhabi |
| PET to procedure | median 13 days | same [S26] | same |
| Nodule ID to referral; referral to consult; consult to biopsy | median 4 (IQR 0-18); 7 (3-14); 20 (12-34) days | Rama et al., JTCVS Open 2025, Univ. Chicago, n=157 [S27] | Single US academic centre |
| Referral to diagnosis | median 34 (IQR 22-56) days | same [S27] | same |
| NHS optimal lung pathway: CT within 72 h; all tests incl. PET-CT by day 14; MDT day 21; diagnosis communicated by day 28; treatment by day 49 | targets | NOLCP v3/v4 [S28][S29] | Target, not observed. Pathway starts at suspicion of cancer, which maps to Wasl's >8 mm recommendation date |
| Faster Diagnosis Standard | 28 days from urgent referral to diagnosis or ruling out; operational standard 80% from Mar 2026; June 2026 actual 79.0% | NHS England CWT guidance [S30]; CRUK stats [S31] | Target and observed; NHS only |
| Study endpoint for high-risk nodule completion | day 112 (counts CT, MDT, PET/CT, bronchoscopy, biopsy) | NCT07805434 [S32] | Ongoing trial; good for a "late" threshold |

### B2. Outreach consent for incidental lung findings

| Topic | Finding | Source | Transfer |
|---|---|---|---|
| PA Act 112 (2018) | Imaging entity must notify the patient directly within 20 days (most sources: after results go to the ordering clinician) when a "significant abnormality" exists = one a reasonably prudent person would follow up within 3 months; notice need not state the finding, must name ordering clinician, test date, and how to get the report | [S33][S34] | US state law; no UAE equivalent found |
| Effect of Act 112 letters | Penn Medicine, 235 findings: follow-up discussion 87% at mean 6.0 days; only 0.4% attributable to the letter; follow-up care 74% at mean 31.3 days | Mattay et al., JACR 2020 [S35] | Mandated generic letters add little; supports outreach that names the next step |
| Direct patient notification RCT | Follow-up completion within 1 month of due date: letter 58.7%, phone 60.4% (only arm significantly above control), portal 36.4%, control 53.2% | JACR 2023 RCT, numbers via secondary excerpt [S36] | Phone beats portal; check primary paper before quoting |
| AI-flagged ED nodules + patient contact | Notification 75.0% to 88.4%; nodule follow-up 53.5% to 67.9% (before/after) | PubMed 42501874 [S37] | Before/after design; method of contact not stated |
| Baseline adherence without a system | 48% of 3,150 VA patients got Fleischner-concordant follow-up | [S38] | US VA |
| UAE Federal Law 2/2019 (ICT in health), Art 16 | Patient information must be kept confidential and used only for health purposes unless the patient gives written approval; consent may be paper or electronic (Cabinet Res. 32/2020); exceptions include insurers, research (de-identified), public health and patient safety, courts, Health Authority | [S20][S19] | Binding. Reading (ours, not legal advice): a reminder about the patient's own follow-up is a health purpose; marketing is not |
| UAE Law 2/2019 Art 13 | Health data must not be stored or processed outside the UAE without Health Authority approval (fine AED 500k-700k) | [S20] | Binding; messaging vendor must be UAE-hosted |
| DoH ADHIE (Malaffi) policy, Apr 2020 | Facilities collect consent for HIE sharing in standard consent forms; DoH authorises sharing where consent is unavailable (4.2); HIE data may be used for treatment, payment, operations; **not** for marketing, not stored in cloud, not sent outside UAE, and **not ingested or stored into a participant's own system** (4.3.13.2.2.6/.9/.10/.11); breach notice to patients within 60 days (4.3.17.3) | [S14] | Binding. The no-ingest clause matters for Wasl's architecture: run as an HIE-side service or on the facility's own data, not as a third copy |
| DoH Standard on Patient Healthcare Data Privacy (Sep 2020) | Exists; sets when PHI may be used/disclosed, data localisation | Lexology summary [S18] | Full text not retrieved; nothing found on SMS/reminder consent |
| DoH PHC standard 4.10.2 | Primary care facility must have policies for notifying patients of lab and imaging results, positive or negative | [S13] | Binding duty sits with the treating facility, so outreach should go out in its name |

### B3. HL7 v2 mapping

| Element | Value | Source |
|---|---|---|
| Message | `ORU^R01` (MSH, PID, PV1, ORC, OBR, OBX..., NTE) | HL7 v2.5.1 ch.7 [S39]; Saga IT [S40] |
| OBR-4 CT chest | `24627-2^CT Chest^LN` (Doc scale, order and observation; 24628-0 = CT Chest W contrast IV; 11538-6 deprecated, maps to 24627-2) | [S41][S42] |
| OBR-25 / OBX-11 | `P` preliminary, `F` final, `C` corrected (resend full report on correction) | [S39][S43] |
| OBR-3 / OBR-18 | filler order / accession number, kept across order and result | [S40] |
| OBX-3 report sections | 18782-3 findings (required in C-CDA imaging), 19005-8 impression, 18783-1 recommendation, 18834-2 comparison | LOINC 81220-6 via [S44] |
| OBX-2 / OBX-5 | `TX` or `FT`, one line or block of report text per OBX; HL7 2.7 example also sends impression as CWE | [S39][S44] |
| OBR-4 PET/CT | 81555-5 PT+CT whole body tumour localisation W 18F-FDG IV; 82805-3 (WO contrast); 82806-1 (W contrast IV); 82808-7 skull base to mid-thigh; no chest-only PET+CT code found | [S45] |
| Pathology ORU (NAACCR v2.5.1 model) | OBR-4 `60568-3` synoptic report or 11526-1 pathology study; OBX-3 22637-3 final diagnosis, 22634-0 gross, 22635-7 microscopic, 22636-5 history; 33746-9 if one narrative blob | NAACCR Vol V [S46]; HDD Access [S47] |
| Malaffi specifics | No public HL7 v2 conformance guide found. ADHIE policy names HL7 v2.x, SNOMED CT and XML as standards; interface and data specifications are issued by the operator to participants only | [S14] |

---

## Proposed simulation settings

### Part A (rejected referrals)

| Setting | Value | Status |
|---|---|---|
| Share of referrals rejected (not redirected) | 2% | Sourced, NHS [S3]; use 2-5% sweep as ASSUMPTION |
| Receiver decision time, urgent | 2 BD target; draw from 0-4 BD | Target sourced [S4b]; distribution ASSUMPTION |
| Receiver decision time, routine | 5 BD target; draw from 1-10 BD | Target sourced [S4b]; distribution ASSUMPTION |
| Hard cap on receiver silence before Wasl treats as "no response" | 14 calendar days | Sourced, CPSO [S7] |
| Reason mix | wrong_provider 30, service_not_available 20, needs_more_info 25, manage_in_primary_care 15, unlikely_to_benefit 5, other 5 (%) | ASSUMPTION (codes sourced [S1][S6], weights not) |
| Referrer act_by after packet | urgent 2 BD, routine 5 BD | ASSUMPTION |
| Escalation | coordinator at act_by; medical director at +2 BD (urgent) / +5 BD (routine); DoH report at day 14 for urgent | Roles sourced [S13]; timings ASSUMPTION |
| Calendar | Fri-Sat weekend plus UAE public holidays | Sourced [S14] |

### Part B (lung nodule >8 mm)

| Setting | Value | Status |
|---|---|---|
| Valid closers | CT chest 60-120 d after index, PET/CT, lung pathology, documented specialist/MDT discharge | Options sourced [S21][S23]; window and MDT closer ASSUMPTION |
| Recommendation to PET/CT | median 11 d | Sourced [S26] |
| PET to biopsy/procedure | median 13 d | Sourced [S26] |
| Referral to biopsy | about 27 d (7 + 20 medians) | Derived from [S27]; sum of medians is approximate |
| "Late" flag for PET/biopsy path | day 28 | Sourced as NHS FDS target [S30] |
| "Late" flag for CT-at-3-months path | day 120 (90 + 30 grace) | 30-day grace sourced as a published adherence definition [S38b]; combination ASSUMPTION |
| Hard overdue | day 112 | Sourced as trial endpoint [S32] (alternative to day 120) |
| Baseline completion without Wasl | 48-54% | Sourced [S38][S37] |
| Completion with targeted contact | 60-68% | Sourced [S36][S37] |
| Outreach channel order | phone first, then letter; portal last | Sourced ranking [S36]; order ASSUMPTION |
| Outreach gate | send only if treating facility's consent flag present and message names the treating facility | Law sourced [S20][S14]; gate design ASSUMPTION |
| HL7 detection | OBR-4 in {24627-2, 24628-0, local CT chest codes}; closer if OBR-4 in PET+CT set {81555-5, 82805-3, 82806-1, 82808-7} or pathology OBR-4 {60568-3, 11526-1}; act only on OBR-25 F or C | Codes sourced [S41][S45][S46]; rule ASSUMPTION |

---

## Sources

- S1 NHS England Digital, Referral Assessment Services: https://digital.nhs.uk/services/e-referral-service/document-library/referral-assessment-services
- S2 NHS e-RS quick guide, reviewing RAS referrals: https://digital.nhs.uk/services/e-referral-service/document-library/quick-guide-for-clinicians-reviewing-ras-referrals
- S3 Joint guidance on the NHS e-Referral Service: https://digital.nhs.uk/services/e-referral-service/joint-guidance-on-the-use-of-the-nhs-e-referral-service
- S4 NHS e-RS reviewing A&G requests: https://digital.nhs.uk/services/e-referral-service/document-library/quick-guide-for-clinicians-responding-to-advice-and-guidance-request
- S4b RCGP Advice and guidance policy page (updated Jul 2026): https://www.rcgp.org.uk/representing-you/policy-areas/advice-and-guidance
- S4c NCL ICB e-RS A&G: https://gps.northcentrallondon.icb.nhs.uk/referral-support/advice-and-guidance/ers-advice-and-guidance
- S5 Ocean, Guide to Receiving eReferrals: https://support.cognisantmd.com/hc/en-us/sections/115000150011
- S6 Ocean, Set Up Your Site to Receive (reasonForDecline values): https://support.cognisantmd.com/hc/en-us/sections/115000899072
- S7 CPSO Transitions in Care policy: https://www.cpso.on.ca/Physicians/Policies-Guidance/Policies/Transitions-in-Care
- S8 FHIR R4 Task: https://hl7.org/fhir/R4/task.html ; S8b TaskStatus: https://hl7.org/fhir/R4/valueset-task-status.html
- S9 FHIR R4 ServiceRequest: https://hl7.org/fhir/R4/servicerequest.html
- S10 Ontario eReferral-eConsult FHIR IG v0.12.1 Task: https://simplifier.net/guide/ca-on-ereferral-r4-iguide/index/fhirartifacts/task?version=0.12.1
- S11 IHE PCC 360XL supplement (2020): https://www.ihe.net/uploadedFiles/Documents/PCC/IHE_PCC_Suppl_360XL_Rev1-0_PC_2020-08-26.pdf
- S12 ONC 360X Implementation Guide: https://oncprojectracking.healthit.gov/wiki/spaces/TechLab360X/pages/18776122/360X+Implementation+Guide
- S13 DoH Standard for Primary Healthcare Services, Sep 2022: https://www.doh.gov.ae/-/media/648753A8E56C4ED8950A6F3BD8333AC0.ashx
- S14 DoH Policy on the Abu Dhabi Health Information Exchange, Apr 2020: https://www.doh.gov.ae/-/media/A78104416DF2440E89AC90FB75F11055.ashx
- S15 DoH Healthcare Providers Manual 2017: https://www.doh.gov.ae/-/media/7EA6916C486A4D6BBA0036C8CD931B11.ashx
- S16 DoH Standard for Specialized Mental Healthcare Services (snippet only): https://www.doh.gov.ae/-/media/EB977F268676451E8180B86A06F7254C.ashx
- S17 NHS RTT rules suite Oct 2022: https://www.gov.uk/government/publications/right-to-start-consultant-led-treatment-within-18-weeks/referral-to-treatment-consultant-led-waiting-times-rules-suite-october-2022
- S18 Lexology, Abu Dhabi Standard on Healthcare Data Privacy: https://www.lexology.com/library/detail.aspx?g=f3f3cada-5c64-4fe8-b14b-213fb256e6d2
- S19 Cabinet Res. 32/2020 (Lexology): https://www.lexology.com/library/detail.aspx?g=abd696dd-9a58-419e-8d31-27728092cae3
- S20 UAE Federal Law 2/2019 (official): https://uaelegislation.gov.ae/en/legislations/1209/download ; English rendering: https://lexaidxb.com/en/legislations/uae-1209
- S21 MacMahon et al., Fleischner 2017, Radiology 284:228-243: https://pubs.rsna.org/doi/full/10.1148/radiol.2017161659
- S22 NCBI Bookshelf Fleischner table: https://www.ncbi.nlm.nih.gov/books/NBK553863/table/ch5.Tab1/
- S23 BTS pulmonary nodule guideline 2015: https://scts.org/_userfiles/pages/files/guidelines/thoracic/bts_guidelines_for_the_investigation_and_management_of_pulmonary_nodules_2015.pdf
- S24 Radiology Assistant, BTS guideline: https://radiologyassistant.nl/chest/plumonary-nodules/bts-guideline
- S25 ACR Appropriateness Criteria, indeterminate nodule: https://www.jacr.org/article/S1546-1440(23)00613-0/fulltext
- S26 Albano et al., Lung 2020;198:967-72: https://link.springer.com/article/10.1007/s00408-020-00404-8
- S27 Rama et al., JTCVS Open 2025: https://pmc.ncbi.nlm.nih.gov/articles/PMC12039447
- S28 NOLCP v4 (Jan 2024): https://rmpartners.nhs.uk/wp-content/uploads/2024/09/national-optimal-lung-cancer-pathway_v4_01jan2024.pdf
- S29 NOLCP v3: https://rmpartners.nhs.uk/wp-content/uploads/2023/07/National-Optimal-Lung-Cancer-Pathway-3.0-1.pdf
- S30 NHS England CWT dataset guidance: https://www.england.nhs.uk/long-read/national-cancer-waiting-times-monitoring-dataset-guidance/
- S31 Cancer waiting times key statistics (Aug 2026): https://assets.ctfassets.net/u7vsjnoopqo5/1CuAceKjmT72aPgZCJ3mew/2cec4781e92a4175a6023dfa74a12984/Performance_measures_across_the_cancer_pathway_2026.08.pdf
- S32 NCT07805434: https://clinicaltrials.gov/study/NCT07805434
- S33 Post & Schell on Act 112: https://www.postschell.com/insights/pennsylvanias-act-places-new-patient-notification-requirements-diagnostic
- S34 Improve-Dx on Act 112: https://www.improve-dx.org/blog/new-pennsylvania-law-requires-patient-notification-abnormal-imaging-results
- S35 Mattay et al., JACR 2020;17:1676-83: https://pubmed.ncbi.nlm.nih.gov/32579883/
- S36 JACR 2023 RCT on early direct patient notification: https://www.jacr.org/article/S1546-1440(23)00758-5/abstract
- S37 AI-supported ED nodule follow-up: https://pubmed.ncbi.nlm.nih.gov/42501874/
- S38 VA multi-system adherence study: https://pubmed.ncbi.nlm.nih.gov/38452601 ; S38b PLOS ONE 2022: https://journals.plos.org/plosone/article?id=10.1371%2Fjournal.pone.0274107
- S39 HL7 v2.5.1 Chapter 7: https://www.hl7.eu/HL7v2x/v251/std251/ch07.html
- S40 Saga IT ORU / OBR: https://saga-it.com/docs/hl7/messages/oru
- S41 LOINC 24627-2 CT Chest: https://www.findacode.com/loinc/24627-2--ct-chest.html
- S42 DICOM CID 7000: https://dicom.nema.org/medical/Dicom/current/output/chtml/part16/sect_CID_7000.html
- S43 HL7 Australia observation reporting: https://hl7.org.au/archive/hl7v2wg/4-Observation-Reporting_1278278.html
- S44 LOINC 85430-7 imaging report sections: https://loinc.org/85430-7 ; HL7 v2.7 ch.7: https://www.hl7.eu/HL7v2x/v27/std27/ch07.html
- S45 LOINC 81555-5 PET+CT: https://loinc.org/81555-5/
- S46 NAACCR Vol V v5.1: https://www.naaccr.org/wp-content/uploads/2024/04/NAACCR-EPath-Reporting-guide_v51_Revised_20240328.pdf
- S47 HDD Access, mapping anatomic pathology: https://www.hddaccess.com/tips/a-simple-tip-for-mapping-anatomic-pathology-in-loinc

Not verified directly (403 or paywall): 360X decline ORC codes (S12), Ocean decline article text (S6 via search
snippet), JACR 2023 RCT figures (S36 via secondary excerpt), DoH mental health clock (S16 via snippet).
