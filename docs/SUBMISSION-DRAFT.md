# Wasl (وصل): evidence behind every screening follow-up

**Challenge: Closing the Screening-to-Care Loop**

**Status labels used below:** **Built** means implemented and exercised in code. **Simulated** means measured on synthetic patients, events, reviewers or capacity. **Designed** means a proposed deployment, clinical pathway or evaluation not yet performed in practice. No clinical deployment or patient-outcome improvement is claimed.

## Problem and local baseline

A woman has a positive faecal immunochemical test (FIT+) at her clinic. Her colonoscopy may happen at another hospital, under a Latin spelling of her name and without an Emirates ID on the report. The referring clinic can therefore lose sight of a procedure that actually occurred, while another patient who still needs care receives the same generic reminder as everyone else.

The local need is substantial. In the 2015–16 Abu Dhabi Health Services/SEHA cohort, only **30.5% of FIT+ patients underwent colonoscopy**, after a median **72 days**. The Department of Health (DoH) colorectal cancer standard requires colonoscopy within **31 days of referral**, with a **>90%** programme target.[1][2] Public programme-wide completion evidence remains scarce, while Malaffi describes care-gap identification rather than evidenced closure.[12]

Wasl is built around one programme owner and one KPI: evidenced CRC follow-up for the Abu Dhabi Public Health Center screening programme. In a **merged simulation**, counting every referral, the proportion reaching colonoscopy within 31 days was:

| Tracker | Completion within 31 days |
|---|---:|
| Emirates-ID-only tracking | **12.4%** |
| Simulator's hidden truth | **33.0%** |
| Wasl | **32.0%** |

This is a measurement simulation, not an observed Abu Dhabi KPI. Its operational point is targeting: under ID-only tracking, **20.6 of every 100 referrals** were completed on time but appeared lost—**62% of true completions**—and **23.5% of the apparent chase list** was already done. Wasl recovered the simulated rate to within one percentage point of truth and attached a reason to each open loop: not notified, not understood, referral rejected or unanswered, not booked, no-show, inadequate evidence, or unresolved identity. Standard reminders, navigation and referral workflows can then be directed at patients who genuinely need them.

## Built solution

The **built core** is a Go pipeline using NATS JetStream, deterministic replay and a shared idempotency key. Production-shaped **HL7 v2.5.1 over MLLP** and a mock **FHIR R4 Subscription** feed the same internal event model. HL7 acknowledgement follows durable persistence; reordered, duplicated and replayed events are absorbed by stream deduplication and a seen-ledger. The Malaffi interface is inferred from public documentation: testing is against mocks, not a live feed.

Identity resolution combines a compact Arabic–Latin character encoder, deterministic HNSW candidate blocking, a LightGBM pair scorer and union-find clusters with human merge/split controls. The closure gate requires both a trusted patient link and field-by-field agreement between deterministic DoH rules and a model reader. Otherwise the case remains open for human review. Uncorroborated links are capped below the closure tier, and patient messaging requires a second identifier or human confirmation.

A CRC loop closes only when the required evidence is present: caecum reached and documented, bowel preparation graded Excellent or Adequate, and histology returned if any tissue was taken. Ungraded preparation remains open while the endoscopy unit is queried; inadequate preparation or an incomplete examination routes to repeat care. A suspicious mass, stricture or urgent impression enters `suspected_malignancy` and alerts the MDT pathway without waiting for histology. Surveillance, gastroenterology, MDT, specialist, decline, unfitness, transfer and death have named states and owners; sensitive non-clinical closures require humans and cannot inflate the automated KPI.

The **built patient layer** includes owner-named notices, teach-back, consultation routing, consent holds, opt-out, contest and explanation rights. A contest blocks automatic closure and triggers review by a different person. Outreach is released only with recorded contact permission and is cancelled after withdrawal. Non-English notices are drafts, not validated translations.

A separate **built, hold-only image reader** assesses caecal landmarks and preparation photographs. It can hold a loop, flag a missing landmark or request a blinded endoscopist grade; it can never close one or upgrade a text label by itself.

The **built API** exposes the materialised dashboard summary, filtered case lists, case detail, provenance audit and role-controlled append-only review actions, plus deterministic fixtures. It supports reported versus evidenced closure, drop-off reasons and review workload. A finished frontend is not claimed.

## Evidence investigator: built experimental feature

A **built experimental sidecar** investigates a narrow class of missing-demographic identity reviews. The user selected **r1agent10 for the showcase** because it supplies an agentic evidence-investigation feature while preserving human sign-off. It is human-supervised decision support, not an autonomous closure agent or a validated default-model upgrade. Development passed; stress recall equalled guarded human and reviewer time fell, but recall was five closures below unguarded and validation did not run.

The sidecar receives only generated, sanitised evidence bundles through allow-listed tools: registration demographics and identifiers, pair context and model score, cached structured report fields, and cached image-reader verdicts. It has no raw filesystem or live-NATS access, no truth files, answer keys, reviewer decisions or donor maps, and no raw clinical document or real patient data is sent to the external investigator model. Hard conflicts—conflicting demographics or valid IDs, merge-constraint failures and cannot-link cases—are excluded. Existing cluster and linkage checks still apply.

For an eligible item, the investigator returns `same`, `different` or `uncertain` with resolvable supporting citations. Invalid citations, unsupported claims, malformed output, timeout or budget exhaustion become `uncertain`. An agent `same` can replace only the second read when the human also says `same`; disagreement retains the senior-human tie-break, and every other outcome preserves ordinary human double review. The agent never signs a review or writes a clinical decision.

Responses use pinned metadata, structured output and content-hash caching. Offline replay consumes only those cached recommendations, iterates to the existing review fixed point and falls back to ordinary double review for uncovered items. Selection and validation gates cover recall, contamination, wrong-pair reviews, human time, citation validity and zero safety events. This remains an offline synthetic experiment; live shadow or assist deployment would require a separate evaluation, deterministic fallback and kill switch.

r1agent10 development recorded **658/688** closures in all three arms. Stress recorded **595/688**, identical to guarded human and five below unguarded 600/688. Human review time was **24.837 hours versus 27.854 guarded (10.8% saved)**; false merges were 2 candidate, 2 guarded and 5 unguarded, cross-person edges 1/1/4 and wrong-pair reviews 3/3/4. Eligible coverage was 214/214 and accepted citations 206/206. False closures, wrong-patient releases, offline replay mismatches, analysis errors and trace errors were zero. Stress wrong final sign-offs were 76 candidate, 76 guarded and 81 unguarded (reported, not gated). A separate live-versus-materialized comparison reported 1,800 signature divergences from the agent policy/review path, not offline replay nondeterminism. These are simulated trade-off results, not proof of improved clinical accuracy or a fully validated agent.

## Evaluation

### Frozen system evaluation — built and simulated

The final-code evaluation used **five seeds × two noise models**, held-out people and an independent v2 corruption model not used for training or tuning. Each run contained **6,314 index loops across seven programme types**. These lanes used **cached report-reader outputs**, simulated reviewers with an assumed 4% error rate, mock FHIR ingest and chaos testing. Held-out v2 results were:

| Programme | Mean recall with review | Pooled false closures |
|---|---:|---:|
| CRC | **0.785** (95% interval 0.772–0.798); precision 1.000 | **0/3,035** |
| Cervical | 0.906 | 0/1,303 |
| Incidental lung nodule | 0.952 | 0/1,907 |
| Diabetic retinopathy | 0.994; referral 0.894 | 0/5,627; referral 0/233 |
| Breast | 0.994; recall loop 0.931 | 0/338; recall loop 0/23 |
| Lung-cancer LDCT | 0.998; action loop 0.981 | 0/1,413; action loop 0/121 |
| IFHAS | 0.969; follow-up loop 0.906 | 0/3,392; follow-up 0/1,047 |

These denominators overlap where subloops are shown and must not be summed into a single end-to-end claim. LDCT and IFHAS generators reuse development texts, so their reader results are not independent generalisation evidence.

Across the same held-out lanes, Wasl recorded **0/16,486 wrong-patient messages**, **0/11,650 releases without granted consent**, zero duplicate or lost outputs, and zero forged events with effect; **85/85 tamper attacks** were detected. Reviewer time was **11.6 hours per 1,000 loops** across this programme mix. Simulated rejected-referral handling produced complete packets for **300/300 CRC**, **21/21 lung** and **35/35 diabetic-retinopathy** cases, with no extra packets. These are workflow results under simulated truth and reviewer behaviour, not clinician performance.

A separate Learn-then-Test calibration covered **12,600 CRC and cervical loops** from a disjoint synthetic split. At the operating link tier it observed zero false closures and zero wrong-patient-message loops, certifying both risks at no more than 0.1% with at least 95% confidence **only for loops exchangeable with that calibration set**. It does not certify the other programmes or real Abu Dhabi care.

### Separate reader, ingest and robustness evidence

Seven sequential unseen synthetic report corpora exposed unsafe phrasing classes in earlier readers: **6/211 automatic closures** with the 26B reader and **10/137** with the local 4B were wrong. Each class led to a disclosed generic rule fix. The final two fair tests passed with both readers, but their zero counts—**0/71** and **0/47** automatic closures respectively—still have wide upper bounds. This evidence motivates shadow-mode clinician confirmation rather than unchecked automation.

The frozen evaluation must not be confused with the separate **live-reader integrated run**. That single simulated run used the local Gemma 3 4B and recorded CRC precision 1.000, recall 0.955 and **0/567 false closures**; its live answers were ledgered for deterministic replay. Likewise, the separate HL7 stress run delivered **30,668/30,668 resources exactly once**, absorbed **888 duplicate re-sends** through connection drops, and recorded **0/574 CRC false closures**. These are distinct module and integration results, not one cumulative experiment.

## Capacity and queue impact

Measurement alone does not create an endoscopy slot. Wasl therefore exposes backlog and unit-level shortfalls while keeping queue interventions explicitly **simulated decision support**.

The booking module honours gender, region and Ramadan constraints and supports overbooking and automatic rebooking. At an assumed **1.5 ring-fenced slots per FIT+**, it reached 94.5% within 31 days among completed colonoscopies and 52% across all FIT+ referrals; capacity and booking at all, not optimisation, drove most of the gain. The daily optimiser beat greedy scheduling by no more than 0.5 percentage points, so no optimisation advantage is claimed. Below roughly **one slot per FIT+**, completion within 31 days collapsed under every tested policy.

Network pooling addresses misplaced rather than missing capacity. With slots deliberately misallocated across regions at 1.2 slots per FIT+, all-FIT+ completion within 31 days rose from **22.2% without cross-site booking to 42.1% with pooling**. At 1.0 slot, the corresponding results were only 7.3% and 7.8%. Direct-to-procedure navigation and preparation-risk targeting are also simulated: published evidence supports straight-to-test pathways and SMS preparation support, but their local effects remain unmeasured.[6][7]

A separate projection—not an outcome result—uses published reminder, referral and navigation effects. Its planning case assumes **+13 percentage points** in day-120 completion. At 5,700 FIT+ patients annually, that implies about 740 additional colonoscopies, roughly AED 3M in mandated payer-funded procedures and AED 0.55M in potential treatment savings. The resulting indicative all-in estimate is **AED 43,000–77,000 per life-year**, highly sensitive to borrowed uplift, stage-shift, mortality and cost assumptions.[9][10][11] Wasl does not fund the capacity or claim this forecast as realised value.

## Safety and governance

The **built controls** include role-based access, consent enforcement, human merge/split, double review of merges and adequacy decisions, hash-chained provenance, tamper detection, deterministic rollback and regression fixtures. An audited false closure is designed to freeze automatic closure. A hold-only reader outage or disagreement fails toward review, not closure.

The governance package self-classifies the system as **High risk** under the DoH Responsible AI standard and maps controls through a 12-item hazard log. The proposed legal basis is DoH public-health and quality-assurance use under HIE Policy 4.3.12.1 and Federal Law 2/2019 Article 16; patient outreach remains under the referring facility's treatment relationship and its recorded contact preferences.[3][4][5] This is a design mapping, not legal approval. Production is designed for in-UAE processing and storage, customer-held keys, restricted model egress and an independent write-once audit anchor. Clinical reports remain with treating facilities and Malaffi; Wasl retains the minimum linkage store, pointers, decisions and provenance needed for an auditable loop.

Fairness is monitored rather than assumed. In the held-out frozen run, the programme-standardised name-origin review-load ratio was **1.065** (p=0.185); age-band and sex ratios were 1.066 and 1.030. Roughly 16 subgroup tests and small cells make this weak evidence. In the separate booking simulation, female-list protection reduced the access gap from 18.5 to 12.6 percentage points at an assumed 15–30% female-list share; reaching the 5-point acceptance limit required approximately 30–40% female lists. That staffing condition cannot be solved by software.

## Pilot path

The **designed first step** is a 90-day unpaid shadow pilot linking one primary-care clinic and one endoscopy unit, with local governance assessment in parallel. No pilot, buyer commitment or deployment is currently claimed. Wasl would compare its loop list with the coordinator's manual list; proposed exit criteria are zero false closures and recall of at least 0.95.

Every automatic report closure would initially remain a suggestion requiring clinician confirmation. Only after **300 consecutive confirmed-correct closures** would the design permit automatic closure with a random 10% clinical audit. A confirmed error returns that phrasing class to shadow mode, adds a regression test and restarts the count.

A subsequent **designed live-outreach phase** would use a stepped-wedge across 6–8 primary-care clusters, with day-120 colonoscopy completion as the primary endpoint. Under stated volume and intra-cluster-correlation assumptions, estimated power is 0.81–0.94; otherwise the pilot reports an interval rather than claiming a significant effect. Scale-up requires the uplift's lower 95% bound to exceed zero and zero audited false closures. Local reader recalibration, professional translation, clinician co-design, staff training, conformity assessment and post-deployment monitoring are gates, not completed activities.

## Honest limitations

- Patients, reviewers, identity noise, capacity and most report text are synthetic. Real Wikidata names and 43 public US report transcripts do not make the cohorts clinical validation. Those transcripts have an LLM-produced key and are reused across loops; no clinician-labelled or human-double-labelled evaluation is claimed.
- Zero observed false closures is not zero risk, especially in small subgroups. Sequential corpus testing already found shared two-reader failure classes, and later zero counts are bounded observations, not proof.
- The final seven-programme evaluation used cached readers. Live local-reader, HL7, image, booking, pooling and Learn-then-Test results belong to separate experiments and are not combined into an end-to-end effectiveness claim.
- The image reader was trained on Norwegian video frames and needs recalibration on UAE stills. Its specificity is insufficient for autonomous adequacy decisions, which is why it is hold-only.
- A separate two-seed stress test with 2% wrong Emirates IDs produced **19 false closures with the optional demographic guard off and zero with it on**, with recall and review costs and residual linkage contamination. The guard remains off by default; two seeds do not establish a general safety guarantee.
- The evidence-investigator sidecar is built and selected experimentally for the showcase, but validation did not run. Stress saved reviewer time while losing five closures against unguarded; it is not a validated recall improvement and has no live-NATS or clinical-effectiveness claim.
- Malaffi connectivity is mock-tested only. Non-English notices require professional translation, back-translation and cognitive testing; teach-back has been tested only in English, Modern Standard Arabic, Gulf Arabic and Arabizi.
- No finished frontend, recorded demonstration, clinician endorsement, procurement agreement, local care uplift or patient benefit is claimed.

## Sources

1. [DoH Colorectal Cancer Screening Programme standard](https://www.doh.gov.ae/-/media/980F1D8087214B6D9E630B457B85E4B9.ashx)
2. [Almansoori et al., BMC Cancer 2021: Abu Dhabi FIT follow-up](https://pmc.ncbi.nlm.nih.gov/articles/PMC8343888/)
3. [DoH Health Information Exchange Policy](https://www.doh.gov.ae/-/media/A78104416DF2440E89AC90FB75F11055.ashx)
4. [DoH Abu Dhabi Healthcare Information and Cyber Security Standard](https://www.doh.gov.ae/-/media/Feature/Resources/Standards/ADHICS-v2-standard.ashx)
5. [UAE Federal Law No. 2 of 2019 on health data](https://uaelegislation.gov.ae/en/legislations/1209)
6. [Banerjea et al. 2017: straight-to-test pathway](https://onlinelibrary.wiley.com/doi/10.1111/codi.13667)
7. [Systematic review of SMS support for bowel preparation](https://link.springer.com/article/10.1007/s00384-022-04114-w)
9. [PRECISE navigation randomised trial](https://pmc.ncbi.nlm.nih.gov/articles/PMC13007779/)
10. [Zorzi et al., Gut 2022: colonoscopy completion and CRC mortality](https://doi.org/10.1136/gutjnl-2020-322192)
11. [Cochrane review of faecal occult blood screening](https://www.cochrane.org/evidence/CD001216_screening-colorectal-cancer-using-faecal-occult-blood-test-hemoccult)
12. [Malaffi population-health and care-gap description](https://www.malaffi.ae/population-health/population-health-benefits/)
