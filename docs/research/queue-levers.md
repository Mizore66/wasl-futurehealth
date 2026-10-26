# Queue levers: kill-test (public sources, 23 Oct 2026)

Spec = `sources/doh-standards/crc.txt` (DoH CRCSP 2019). NG = [UAE National CRC Guideline 2022](https://www.isahd.ae/content/docs/Colorectal%20guideline%20Screening%20and%20Diagnosis_UAE%20National%20Guidelines.pdf).

## 1. f-Hb triage: WEAK (useful for ordering, not a KPI lever)
- **PPV by band (English OC-Sensor pilot, modelled, per 1,000):** CRC 14 at 20–39 µg/g, 35 at 80–119, 43 at 120–149, 90 at ≥180. CRC+HRA 87→243 ([PMC8366184](https://pmc.ncbi.nlm.nih.gov/articles/PMC8366184/) T5).
- **Other screening cohorts:**
  - Shanghai: AN 24% at 100–799 ng/mL vs 67% at ≥800 ng/mL ([PMC13229734](https://pmc.ncbi.nlm.nih.gov/articles/PMC13229734/)).
  - Barcelona: AN PPV 21–76%; stage III–IV cancers had higher f-Hb ([Auge 2014](https://www.gastrojournal.org/article/S0016-5085(14)00769-0/pdf)).
- **No screening table for ≥400 µg/g was found.**
- **Precedents are symptomatic only.** New Zealand books ≥150 µg/g within 2 weeks ([NZMJ 2024](https://pubmed.ncbi.nlm.nih.gov/39236328/)). I found no screening programme that schedules by f-Hb.
- **Time effect:** cancer risk rises only after 10 months, with advanced stage OR 1.97 ([Corley 2017](https://pubmed.ncbi.nlm.nih.gov/28444278)). So bringing waits under 31 days gives little stage benefit.
- **Spec:**
  - 4.13.2 requires an "explicit definition for cut-off levels".
  - 4.13.3 requires labs to "record… the actual amount of hemoglobin" for negatives and positives (NG 7.2.6.3 is the same). That implies quantitative FIT.
  - Neither document names an assay or cut-off value, and the SEHA paper gives neither ([PMC8343888](https://pmc.ncbi.nlm.nih.gov/articles/PMC8343888/)).
  - Triage is allowed only for ordering inside the "31 days of referral" window (5.5.4.2).
- **Data needed:** numeric f-Hb, units and assay in the lab result.
- **Why WEAK:** with fixed slots it reorders the queue but doesn't change the 31-day share.

## 2. Direct-to-procedure: KEEP
- **This is Abu Dhabi's biggest leak.** 36.3% of non-completers were "referred to a gastroenterologist but did not attend" ([PMC8343888](https://pmc.ncbi.nlm.nih.gov/articles/PMC8343888/) T5).
- **For:**
  - Straight-to-test: first test in 13 vs 22 days ([Banerjea 2017](https://onlinelibrary.wiley.com/doi/10.1111/codi.13667)).
  - Direct access was noninferior on timely completion, 62.7% vs 57.1% ([PubMed 38630852](https://pubmed.ncbi.nlm.nih.gov/38630852)).
- **Against:** in one US system, unnavigated direct access after a positive stool test was slower (HR 0.59), while outreach helped (HR 1.52) ([JGIM 2026](https://link.springer.com/article/10.1007/s11606-026-10226-8)). It needs navigation plus booking, which Wasl provides.
- **Spec: no GI consultation is required.**
  - 5.5.1: the unit provides "the DOH e-Cancer Screening Referral Form".
  - NG 12.5.2: "urgently referred for follow-up colonoscopy within 15 working days".
  - Appendix 9 requires only a pre-colonoscopy assessment: anticoagulants, diabetes/insulin, devices, prior GI procedures, ASA class. A nurse checklist can cover this, with ASA ≥3 routed to a clinic.
- **Data needed:** the Appendix 9 fields at referral time.

## 3. Prep-failure prevention: KEEP (the only lever that adds slots)
- **Predictors:**
  - Dik score (ASA ≥3, TCA, opioids, diabetes, constipation, prior surgery, prior inadequate prep): AUC 0.77, NPV 0.95 ([Dik 2015](https://www.researchgate.net/publication/273956255_Predicting_inadequate_bowel_preparation_for_colonoscopy_in_participants_receiving_split-dose_bowel_preparation_Development_and_validation_of_a_prediction_score)).
  - Gimeno-García score: AUC 0.70 ([PubMed 28282690](https://pubmed.ncbi.nlm.nih.gov/28282690/)).
  - External validation: AUC 0.62–0.69 ([PMC10820399](https://pmc.ncbi.nlm.nih.gov/articles/PMC10820399/)).
- **Interventions:**
  - SMS: adequate prep 81.7% vs 75.7% (RR 1.10, 7 RCTs), and no-shows fell (RR 0.74) ([IJCD 2022](https://link.springer.com/article/10.1007/s00384-022-04114-w)).
  - Multimedia education: RR 1.2 ([PubMed 32794240](https://pubmed.ncbi.nlm.nih.gov/32794240/)).
  - Model-targeted extra PEG for high-risk patients: 94% vs 49% adequate ([DLD 2023](https://www.sciencedirect.com/science/article/abs/pii/S1590865823009374)).
- **Spec:** Appendix 9.4 requires "written instruction… concerning colonic cleansing", and 9.5 says inadequate prep "must not exceed 10%".
- **Slots recovered:** procedures needed = adequate exams ÷ (1−p). Per 1,000 adequate exams:
  - 10%→5% saves **58 slots** (5%).
  - 15%→5% saves **123** (11%).
  - 20%→5% saves **197** (16%).
  - The SMS evidence realistically supports about 15%→10% (62 slots), plus fewer no-shows.
- **Data needed:** medications and comorbidities (Malaffi) and prior prep grade.

## 4. Pooled or single-entry queue: KEEP (if capacity is uneven across providers)
- **Effect sizes:**
  - Ontario centralised positive-FIT booking: mean wait 99→44 days; patients within 56 days 43%→92% (n=916). This was confounded by the FOBT→FIT switch ([PMC10872067](https://pmc.ncbi.nlm.nih.gov/articles/PMC10872067/)).
  - Winnipeg central intake: about 55% shorter waits, from a FAQ ([WRHA](https://wrha.mb.ca/files/central-intake-faq-patient.pdf)).
  - Systematic review: mandatory models gained 5–47 days, but only 6 studies were significant ([CMAJ Open 2021](https://www.cmajopen.ca/content/9/2/E413)).
- **Caveat:** limited when total supply is short ([PubMed 24040629](https://pubmed.ncbi.nlm.nih.gov/24040629/)).
- **Spec:**
  - 5.4.1 encourages the "optional online booking system" on the DOH website.
  - Appendix 4.2 and 4.7 require units to match capacity to demand.
- **Data needed:** per-provider free slots and endoscopist gender. These are not public, which is the main pilot risk.

## Net
Levers 3 and 4 change the 31-day share under binding capacity. Lever 2 removes the largest local drop-off. Lever 1 only decides who gets scarce slots.
