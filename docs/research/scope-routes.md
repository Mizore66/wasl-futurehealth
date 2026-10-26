# Wasl v20: scope and route expansion (public sources, 23 Oct 2026)

## 1. Wider loops on the same engine

| Loop | Published follow-up failure | Abu Dhabi programme / volume |
|---|---|---|
| Incidental lung nodule | 38% appropriate follow-up ([JACR via RadBiz](https://radiologybusiness.com/topics/patient-care/care-delivery/only-38-patients-receive-appropriate-follow-care-incidental-pulmonary-nodules)); 39% no surveillance ([Diagnosis](https://doi.org/10.1515/dx-2019-0008)); 9.9% of ED CTPAs carry a follow-up nodule, 29% followed ([JACR](https://www.sciencedirect.com/science/article/abs/pii/S1546144015013642)) | Malaffi expected ~2M radiology images/yr ([Media Office](https://www.mediaoffice.abudhabi/en/health/malaffi-platform-to-enable-healthcare-providers-to-exchange-radiology-images/)) |
| Adrenal / thyroid incidentaloma | Adrenal: median 34% imaging, 18% biochemical work-up ([review](https://pmc.ncbi.nlm.nih.gov/articles/PMC12130141/)); thyroid: 11% got ultrasound (UK) ([PMC](https://www.ncbi.nlm.nih.gov/pmc/articles/PMC12012347/)) | No programme |
| Abnormal labs | 6.8–62% not followed up (labs), 1–35.7% (radiology) ([Callen 2012](https://pubmed.ncbi.nlm.nih.gov/22183961/)); 7.1% patients not informed ([Casalino 2009](https://psnet.ahrq.gov/issue/frequency-failure-inform-patients-clinically-significant-outpatient-test-results)) | No programme |
| Lung LDCT screening | Lung-RADS adherence 27.6% ([PMC](https://www.ncbi.nlm.nih.gov/pmc/articles/PMC12335435/)) | Yes: DoH programme since 2017, spec Dec 2018 ([DoH](https://www.doh.gov.ae/-/media/Feature/Resources/Standards/Lung-Cancer-Screening-Service-Specifications_Publish.ashx), [Springer](https://link.springer.com/chapter/10.1007/978-981-99-6794-0_33)); still "in their infancy" ([SSMC](https://ssmc.ae/health-blog/lung-cancer-in-the-uae-a-hidden-threat-beyond-smoking/)). No volume published |
| Diabetic retinopathy referral | 69% attended (England), 54.9% (US teleretinal) ([PMC](https://www.ncbi.nlm.nih.gov/pmc/articles/PMC11759212/), [PubMed](https://pubmed.ncbi.nlm.nih.gov/39710903/)) | SEHA AI retinal reading at 11 centres ([ADAFSA/AARD](https://www.aard.gov.ae/Media-Centre/News/02-06-2022-1)); no volume. UAE adult diabetes prevalence 20.7% ([IDF](https://idf.org/our-network/regions-and-members/middle-east-and-north-africa/members/united-arab-emirates/)) |
| IFHAS abnormal | None published | 150+ facilities, all Thiqa 18–75 ([ADPHC](https://www.adphc.gov.ae/en/Media-Center/News/more-than-150-healthcare-facilities-provide-ifhas-programme-services)); referral required on abnormal results ([DoH standard](https://www.doh.gov.ae/-/media/EE758C3B77314BC394F2479AEF803AA2.ashx)); Thiqa 1M+ members ([thiqa.ae](https://www.thiqa.ae/)) |
| Breast recall | 27–72% not followed up by 6 months (US) ([BMC Cancer](https://bmccancer.biomedcentral.com/articles/10.1186/s12885-021-08100-3)); 0.2% lost in Dutch programme ([Lancet RH-Eur](https://www.thelancet.com/journals/lanepe/article/PIIS2666-7762(25)00318-7/fulltext?rss=yes)) | 34,000+ screened 2024; target 3–4x ([Aletihad](https://en.aletihad.ae/news/business/4610562/abu-dhabi-aims-to-boost-breast-cancer-screenings--by-3-4-tim)); Oct 2025 up 377% ([Gulf News](https://gulfnews.com/uae/abu-dhabi/abu-dhabi-sees-377-surge-in-breast-cancer-screenings-as-fear-and-accessibility-drive-change-1.500710191)) |

**Rough emirate loop volume (A = assumption):**
- Incidental imaging: 2M images/yr; A: 10–15% CT; A: 5–10% actionable gives **10–30k**.
- DR referral: 4.14M population ([SCAD](https://www.statad.gov.ae/w/abu-dhabi-population-in-2024-grows-7-5-to-reach-4-14m)). A: ~3.3M adults, 15% diabetic, half diagnosed, half screened, 5–10% referable gives **6–12k**.
- IFHAS: A: 300k screened/yr, 10–20% referred, gives **30–60k** (weakest estimate).
- Breast: 34k screens; A: 5–10% recall gives **1.7–3.4k** now, ~10k at target.
- LDCT: A: <5k screened, ~10% Lung-RADS 3–4 gives **<0.5k**.
- CRC + cervical: 1.8k (judges) to 5.7k (base) FIT+, plus cervical.

**Total about 50–110k loops/yr, excluding raw labs: 25–60x the 1.8k CRC floor.** The fixed cost (AED 0.44–1.39M) is spread to about AED 4–28 per loop. The value per loop is lower for most incidentalomas (many need no work-up: [RSNA](https://pubs.rsna.org/doi/full/10.1148/radiol.2021212116)).

## 2. Non-Malaffi routes

**Provider networks.** SEHA: 14 hospitals, 70+ clinics ([seha.ae](https://www.seha.ae/about-us)); SEHA CLINICS served 3.5M+ patients in 2024 ([mid-east.info](https://mid-east.info/purehealth-rebrands-ahs-to-seha-clinics-expanding-primary-care-across-abu-dhabi/)). M42: 480 facilities in 28 countries, 15M patients/yr globally, including CCAD and Danat Al Emarat ([m42.ae](https://m42.ae/)).

HIE 4.3.13.2 limits use to "clinical treatment, operations and uses as described in this policy". 4.3.13.2.1.3 permits "Patient diagnosis, treatment, payment, and related Healthcare Operations". "Healthcare Operations" includes "conducting quality and improvement assessments" (Definitions). **So a network following up its own patients fits.** It is blocked, though, by:
- 4.3.13.2.2.3: no access by professionals "not associated with the treatment of that specific Patient or cohort";
- 4.3.13.2.2.4–5: no third parties;
- 4.3.13.2.2.9: no "storage of patient data in a cloud environment";
- 4.3.13.2.2.11: no "ingesting, storing, or recording" ADHIE data "into its own system".

So Wasl must run on-premise as the network's processor and keep pointers, not copies.

**Payer.** Daman administers Thiqa and is a PureHealth subsidiary ([ScienceDirect](https://www.sciencedirect.com/science/article/pii/S259022962500022X)). There is no Daman care-gap programme found. DoH's **Pay for Quality standard (June 2025)** covers facilities *and TPAs* under Thiqa, with incentive/deduction tiers, starting with ART ([FrontierView](https://app.frontierview.com/insightBite/3361/will-abu-dhabis-new-pay-for-quality-model-reward-or-squeeze-medtech-and-diagnostics-mncs%3F)). Jawda KPIs use Thiqa denominators and Shafafiya claims for verification ([Jawda PC](https://www.doh.gov.ae/-/media/Feature/Muashir/Jawda/Jawda2025/Primary-Care-PC-Services-Jawda-GuidanceApplicable-from-Q4-2025-Version-86.ashx)). CPT is mandatory for all procedures ([DoH Coding Manual](https://www.doh.gov.ae/-/media/Feature/shafifya/standards/coding/DOH-Coding-Manual-CSv2021.ashx)), so a colonoscopy claim (CPT 4537x–4539x; code range from CPT, not DoH) is **independent evidence that the procedure happened**. It cannot show caecum, prep or histology, so it reopens or escalates loops but cannot close them. Coverage is Thiqa only (1M+).

**Other HIEs.** NABIDH: 10.41M records, 1,888 facilities (vendor-reported, [Zavis](https://www.zavis.ai/guides/nabidh-ehr-integration-dubai-clinics)). Riayati: 4,952 facilities, 14M unique patients, linked to Malaffi and NABIDH ([MOHAP](https://mohap.gov.ae/en/riayati/about-numr)). No public loop-tracking product was found for either. These are new gatekeepers, not a bypass.

## 3. Competitors

There is no public UAE deployment of PowerScribe Follow-up Manager ([URMC, US](https://www.globenewswire.com/news-release/2019/01/07/1681202/0/en/University-of-Rochester-Medical-Center-Reduces-Risk-of-Delayed-Diagnoses-with-Nuance-Diagnostic-Solutions.html)), Eon (US customers only, [Eon](https://www.eonhealth.com/)), Rad AI Continuity ([radai.com](https://www.radai.com/)) or an Optum product. Nuance's only UAE footprint found is a MoH Dragon Medical deal ([AuntMinnie](https://www.auntminnieeurope.com/industry-news/article/15645606/nuance-gets-medical-reporting-deal-with-uae)). **The nearest threat** is DoH/ADHDS/Philips AI Manager, embedded in all Abu Dhabi radiology reporting (detection, not follow-up) ([DoH](https://www.doh.gov.ae/en/news/doh-adhds-and-philips-are-set-to-launch-a-new-precedent-for-ai-powered-healthcare-in-abu-dhabi)). Philips could add a follow-up module.
