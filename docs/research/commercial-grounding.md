# Wasl: commercial grounding (answers to judges a–d)

## 1. Cost per closed loop at UAE wages (a)

**Wages (AED/month):**
- Coordinator: 5,000–12,000 base ([Glassdoor AD](https://www.glassdoor.com/Salaries/abu-dhabi-coordinator-salary-SRCH_IL.0,9_IS7280_KO10,21.htm)).
- Nurse: 6,500–11,000 for an AD staff RN ([HealthBridge 2026](https://healthbridge.ae/resources/blog/healthcare-salary-guide-uae-2026/)); 10,000–18,000 at SEHA, plus 4,000–8,000 housing ([MenaJobs](https://menajobs.me/industry-salaries/healthcare/uae)).
- Excluded: Indeed AED 4,291 (job-ad outlier) ([Indeed](https://ae.indeed.com/career/registered-nurse/salaries/Abu-Dhabi)).

**Loaded rate:** ×1.3 to ×1.5, over 160 h/month. That gives coordinators AED 41–113/h and nurses AED 53–169/h.

**GCC task times:** we found no GCC study of review or navigation time per loop. The nearest figure is a Saudi interviewee's "about 15 minutes per patient" for documentation ([PMC12940217](https://pmc.ncbi.nlm.nih.gov/articles/PMC12940217/)). US range: 15–48.5 min per FIT+ ([PMC10196769](https://pmc.ncbi.nlm.nih.gov/articles/PMC10196769/)).

**Measured hours, split by role (GPU adds AED 27 per 1k loops):**

| Run | Coordinator / nurse h per 1k loops | Closed per 1k | AED per closed loop |
|---|---|---|---|
| v7, DOB guard | 9.17 / 1.95 (11.1) | 384 | **1.3–3.6** |
| Merged | 3.40 / 12.24 (15.6) | 375 | **2.2–6.6** |
| Manual (US minutes, UAE rate) | 0.25–0.81 h per loop | — | **10–91 per loop** |

The pitch card says AED 1.7. Replace it with **AED 1.3–6.6**.

## 2. Annual volume and programme cost

**No public source gives Abu Dhabi FIT counts.** ADPHC reports only a ">280%" rise in screenings and 5,000+ Shield tests ([MENAFN/ADPHC, Oct 2026](https://menafn.com/1111766961/Abu-Dhabi-Records-More-Than-280-Increase-in-Cancer-Screenings-and-Strengthens-Adoption-of-Preventive-Health-Behaviours)). We built the estimate from its components:

- **Eligible Emiratis aged 40–75: 110k–170k.**
  - 551,535 citizens in 2016 ([SCAD via GLMM](https://gulfmigration.grc.net/uae-abu-dhabi-population-by-nationality-emiratinon-emirati-region-of-residence-and-sex-2005-2010-2013/?print=print)).
  - Projected at 3–4.8%/yr, the 4.8% being SCAD's 2005–11 rate ([SCAD](https://scad.gov.ae/documents/20122/2355136/Final%20Population%20EstimatesPP-2011.pdf/3d6052bc-27f8-2156-dbe5-e7afb5642933?t=1684164116069)), that gives about 740k–880k in 2026.
  - We assume 15–19% are aged 40–75.
  - Cross-check: AHS alone had 45,147 eligible in 2015–16 ([PMC8343888](https://pmc.ncbi.nlm.nih.gov/articles/PMC8343888/)).
- **Uptake: 23.5%–55%.**
  - Low end: the AHS 2015–16 rate.
  - High end: the spec's ">55%" acceptable level ([CRC spec](https://www.doh.gov.ae/-/media/980F1D8087214B6D9E630B457B85E4B9.ashx)).
  - FIT is offered "Annually" ([IFHAS V10](https://www.doh.gov.ae/-/media/EE758C3B77314BC394F2479AEF803AA2.ashx)).
- **Positivity: 6.8%–13.5%.**
  - Qatar: 758/11,130 ([PMC9822526](https://www.ncbi.nlm.nih.gov/pmc/articles/PMC9822526/)).
  - Dubai: 11.6% ([AlMarzooqi](https://www.acquaintpublications.com/article/colorectal_cancer_screening_of_uae_nationals_in_primary_healthcare_centers_in_dubai_health_authorities)).
  - AHS: 13.5%.

| Scenario | FIT/yr | FIT+/yr | Reviewer-h/yr | Review labour (AED/yr) |
|---|---|---|---|---|
| Low (110k × 23.5% × 6.8%) | 25,850 | 1,760 | 20–27 | 880–4,340 |
| Central (140k × 35% × 11.6%) | 49,000 | 5,680 | 63–89 | 2,840–14,040 |
| High (170k × 55% × 13.5%) | 93,500 | 12,620 | 140–197 | 6,310–31,180 |

- **Spread:** about 7× from low to high. The 280% rise points to the upper half.
- **Review labour stays under 0.1 FTE emirate-wide.** The real cost lines are hosting, integration and the DDGO assessment. Not costed here.

## 3. Why ADPHC would fund it (b)

ADPHC sets the target but has no way to evidence it:
- **Target:** the FIT+ referral → colonoscopy interval "should be within 31 days", with >90% acceptable ([CRC spec App. 2](https://www.doh.gov.ae/-/media/980F1D8087214B6D9E630B457B85E4B9.ashx)).
- **Reporting:** providers submit a "Cancer Screening Form" within 2 weeks (§4.17). It records the screening visit, not a colonoscopy done elsewhere.
- **Only one measurement, from 2015–16:** 30.5% completion at a median of 72 days ([PMC8343888](https://pmc.ncbi.nlm.nih.gov/articles/PMC8343888/)). 2026 coverage reports volumes, not completion ([Gulf News](https://gulfnews.com/uae/abu-dhabi/abu-dhabi-sees-377-surge-in-breast-cancer-screenings-as-fear-and-accessibility-drive-change-1.500710191)).
- **Tracking is still in the future tense:** "VCP will allow the DoH, payors, and providers to track referrals". Appendix 5 is a paper log ([PHC Std V2](https://www.doh.gov.ae/-/media/648753A8E56C4ED8950A6F3BD8333AC0.ashx)).
- **Malaffi:** offers "care gap identification", not closure ([Malaffi](https://www.malaffi.ae/population-health/population-health-benefits/)). The Screenings registry targets "timely diagnosis and early intervention" ([release](https://healthmagazine.ae/press_release/abu-dhabi-disease-registries-proactive-prevention-first-care/)).
- **Strategy:** PHI is "Built around two core pillars, Predict (Insights) and Prevent & Act (Interventions)", with cancer a first focus ([Media Office](https://www.mediaoffice.abudhabi/en/health/department-of-health-abu-dhabi-unveils-worlds-1st-ai-powered-population-health-intelligence-platform-at-gitex-global-2025/)). Wasl evidences "Act".
- **Challenge brief:** "an actionable screening result may be recorded without a complete view of whether the required care was received" ([brief](https://hack2skill.com/event/futurehealthchallenge/)).
- **Funding precedent:** Malaffi is already centrally funded. "connecting an EMR system to Malaffi and utilising the platform is a free service" ([Malaffi](https://www.malaffi.ae/connected-emrs/)).

## 4. Who funds endoscopy capacity (c)

**Wasl doesn't fund capacity; it quantifies the need.**
- The simulator needs about 1.5 ring-fenced slots per FIT+. That is about 29 slots/week per 1,000 FIT+/yr, with about 40% on female-endoscopist lists.
- Emirate-wide, that comes to **51–364 slots/week**, of which **20–146 are female-list slots**.

Who carries capacity today:
- **Providers:** units must "Plan capacity that matches demand for screening" (CRC spec App. 4.7).
- **Thiqa:** pays per procedure under the "DOH Mandatory Tariff" (§7.1).
- **DoH:** its Healthcare Capacity Master Plan forecasts demand "by service type and specialty" ([HCMP](https://www.doh.gov.ae/-/media/Files/Investors/160823-New-Capacity-PlanShare.ashx); [2020 update](https://www.doh.gov.ae/en/news/DoH-Launches-an-Updated-Version-of-its-Healthcare-Capacity-Master-Plan-for-Investors)). Wasl's evidenced backlog is a planning input.
- **Wait times:** Jawda reports hospital waiting times quarterly ([WT v8.4](https://www.doh.gov.ae/-/media/Feature/Muashir/Jawda/Jawda2025/Waiting-Times-Jawda-Guidance-for-Specialized-and-General-HospitalsVersion-84.ashx)). Endoscopy coverage unverified (PDF would not open).

## 5. Provider incentives (d)

**Jawda is not the hook.** None of the 17 Jawda primary-care KPIs for 2026 covers cancer-screening follow-up ([Jawda PC 2026](https://www.doh.gov.ae/-/media/Feature/Muashir/Jawda/2026/Primary-Care-and-Medical-Center-Services-Jawda-Guidance_V1_2026_Effective-From-Q3-2026.ashx)). The programme standards are:
- **CRC §4.4.3:** the CRCSP coordinator must "conduct regular audits, monitoring and evaluation to demonstrate compliance".
- **IFHAS §4.11.4.4:** "Report Key Performance Indicators… within a maximum of 21 days after the end of each quarter". Under §6, "DoH may impose sanctions".
- **IFHAS §4.9.4:** providers must "receive timely and complete feedback on referrals".
- **PHC Std:** the coordinator does "Follow up and tracking of preventive screening results to ensure the timeliness and completeness of follow-up".

**Revised claim:**
- No licence fee.
- Review runs about 0.7–0.9 min per loop, against 15–48.5 min for manual tracing.
- Done by the mandated coordinator; yields the required audit trail.

**Gaps:** no public FIT counts or GCC task times; age share assumed.
