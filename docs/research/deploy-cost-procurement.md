# Wasl: deployment, cost and procurement (v10)

[A] = assumption. Prices: [Azure Retail Prices API](https://prices.azure.com/api/retail/prices), 23 Oct 2026, Linux, 1-yr reserved; AED at the [3.6725 peg](https://www.thenationalnews.com/lifestyle/2024/11/22/uae-dirham-dollar-peg/).

## 1. UAE-sovereign topology

| Option | Facts | Fit |
|---|---|---|
| **Core42 Sovereign Public Cloud (recommended)** | Azure plus Core42 controls; "External HSM with Customer Managed Keys" ([Core42](https://www.core42.ai/products/sovereign-public-cloud)); Abu Dhabi government's platform ([w.media](https://w.media/abu-dhabi-govt-partners-with-microsoft-and-core42-to-implement-sovereign-cloud/)). | No public price; costed at Azure list [A]. |
| Azure UAE North | Dubai, 3 zones; paired UAE Central (Abu Dhabi) is DR-restricted ([Microsoft](https://learn.microsoft.com/en-us/azure/reliability/regions-list)). | Same estate, no Core42 layer. |
| On-prem, Malaffi DC | ADHDS runs Malaffi as a DoH PPP ([G42](https://www.g42.ai/resources/case-studies/malaffi-the-first-health-information-exchange-HIE-platform-in-the-MENA-region)); site not public. | Simplest residency; capex unpriced. |

**Readers.** At 1.24 reports per loop (v6 cost run), 1.8k–12.6k FIT+ gives 2.2k–15.6k reports a year: **4.5–33 GPU-hours/yr** at the M2's 7.5 s/report.
- Two `NC24lds_xl_RTXPRO6000BSE_v6` (¼ GPU, 24 GB) in separate zones, $9,065/yr each ([specs](https://learn.microsoft.com/en-us/azure/virtual-machines/sizes/gpu-accelerated/nc-rtxpro6000-bse-v6-series)). The 2.5 GB model uses about 10% of that memory.
- Optional 26B tier: one `NC72lds_xl` (½ GPU, 48 GB), $19,574/yr.
- UAE Central lists only 8-GPU ND SKUs. In DR the gate therefore runs regex-only, so nothing auto-closes.

**NATS.** Three `D4s_v5` ($1,272/yr each), one per UAE North zone. Tagged `az:1..3` with `unique_tag: "az"`; all streams R3, so a zone can fail ([placement](https://docs.nats.io/learn/clustering/placement); [R3](https://docs.nats.io/learn/clustering/replication-and-r3)). Async mirror in UAE Central.

**Keys, ADHICS v2 (local text), backup.**
- CS 1.2(5)/(7)/(8): the key is not supplied, stored or controlled by the cloud provider. We use Azure Managed HSM ($3.20/h) with External Key Management, GA in "all Azure public regions" ([notice](https://mc.merill.net/message/AZ574296)). The root key sits in an ADHDS/DoH-held HSM [A].
- Also: SA 3.1 key lifecycle; CS 1.2(4) encryption at rest and in transit; CS 1.2(10) external testing; CO 5.1 tested restores; SC 2.1 RTO/RPO. The ledger is kept on WORM storage replicated in-country.

**Law 2/2019 Art. 13, copies included.** Log, ledger, inference, backups, DR mirror and telemetry all stay in the UAE. Weights are imported only; no external API is called. ADHICS CS 1.2(2) names "backup and disaster recovery". CS 1.2(3)(c) bars support from outside the UAE. DP 1.4(4) covers encrypted and de-identified copies.

**Prototype data.** Proxy calls carried only public MTSamples reports, public de-identified TCGA reports (distillation) and synthetic data. No patient data was ever sent.

## 2. Year-1 cost (AED)

| Line | Low | High | Basis |
|---|---|---|---|
| Hosting | 101k | 309k | Low: NATS + 2 `D8s_v5` + 2 GPU VMs + storage. High adds HSM, the 26B tier and 30% for DR/monitoring [A]. |
| Malaffi/ADHDS integration | 168k | 540k | **28–54 person-weeks [A]** at AED 6–10k/wk, loaded from [Glassdoor](https://www.glassdoor.com/Salaries/abu-dhabi-united-arab-emirates-software-engineer-salary-SRCH_IL.0,30_IM953_KO31,48.htm) AED 9–27k/month. Malaffi onboarding "can take up to several months" ([Malaffi](https://www.malaffi.ae/providers/definitions/)). A plain EMR feed takes 8–12 weeks ([vendor](https://www.careup.ai/blog/malaffi-nabidh-hie-integration-hms-uae)); subscriptions, Task write-back and SSO add roughly 3–5× that. |
| DDGO + ADHICS | 88k | 290k | Dossier 8–14 person-weeks plus external test AED 40–150k [A]. |
| Operations (0.25–0.5 FTE) | 66k | 156k | [A] |
| Review labour | 0.9k | 31k | commercial-grounding.md |
| **Total** | **≈0.42M** | **≈1.33M** | |

**DDGO.** The RAI Standard says only that High-risk systems "must undergo conformity assessments both prior to and following deployment… in coordination with the DoH DDGO" (3.3.1.2.2.2). The process sits in the DoH Risk Management Protocol (not public to us), and §5 adds periodic DoH audits. No public fee, duration or form exists ([standard](https://www.doh.gov.ae/-/media/Feature/Resources/Standards/2025/Responsible-AI-Standard-V1.ashx); DDG@doh.gov.ae). For comparison, whole-facility ADHICS certification runs AED 85k–550k over 8–16 weeks ([ASC Global](https://ascglobal.ae/insight-details/adhics-certification-cost-timeline-2026-budget-planning-for-uae-healthcare-providers), vendor estimate).

**All-in AED per closed loop** (about 38% of FIT+ close):

| FIT+/yr | Closed | Year 1 | Year 2 |
|---|---|---|---|
| 1,760 | 669 | **634–1,943** | 311–927 |
| 12,620 | 4,796 | **90–277** | 45–135 |

At low volume Wasl costs more than manual tracing (AED 10–91, labour only). The case rests on evidenced KPI assurance, and on sharing fixed cost with breast and cervical screening.

## 3. Procurement route

**Evidence:**
- The Future Health Digital Innovation Challenge offers "validation, incubation, piloting and scale", with no procurement commitment ([BusinessWire](https://www.financialcontent.com/article/bizwire-2026-10-15-future-health-launches-global-challenge-to-move-health-innovation-towards-real-world-application)).
- The DoH regulatory sandbox is reachable only via HealthX, which targets life sciences ([DoH](https://www.doh.gov.ae/en/news/startad-and-doh-launch-healthx-to-advance-abu-dhabi-as-global-biotech-and-life-sciences-hub)).
- Hub71 offers "proof-of-concept tests" and HTA priority ([Hub71](https://www.hub71.com/latest-news/press-release/department-of-health-abu-dhabi-and-hub71-to-foster-innovation-ecosystem-in-healthcare)).
- ADHDS built 12 Malaffi registries with DoH and ADPHC, including Screenings ([release](https://healthmagazine.ae/press_release/abu-dhabi-disease-registries-proactive-prevention-first-care/)).
- A Single Source tender needs "a valid reason", a business case and PDoA approval ([AD Procurement Standards §10.5](https://www.ku.ac.ae/wp-content/uploads/2025/01/Abu-Dhabi-Procurement-Standards.pdf)).

**Recommendation: an ADHDS change request for a new Malaffi module, funded by ADPHC.**
1. A 90-day unpaid shadow pilot through the Future Health pathway, signed by ADPHC as programme owner. The DDGO assessment runs in parallel.
2. At scale, DoH issues a change order to ADHDS under the PPP, with Wasl as subcontractor [A].

**Signatories:** the DoH PDoA holder signs the change order; the ADHDS CEO signs the subcontract; ADPHC owns the requirement and budget. A provider-level purchase is ruled out because HIE 4.3.13.2 limits facilities to treatment and operations uses.

## 4. Volume: cannot be narrowed

No public Abu Dhabi FIT, FIT+ or Thiqa-member count exists ([ADPHC](https://www.adphc.gov.ae/en/Public-Health-Programs/Liquid-Biopsy), [IFHAS 2025](https://www.mediaoffice.abudhabi/en/health/abu-dhabi-public-health-centre-expands-comprehensive-screening-programme-ifhas-to-advance-community-health-and-preventive-care/), [SCAD](https://scad.gov.ae/web/guest/w/abu-dhabi-population-in-2024-grows-7-5-to-reach-4-14m)). The signals conflict:
- ADPHC says "less than 10% of eligible individuals have undergone colorectal cancer screening" (undated).
- Total cancer screenings rose more than 280% in 2025 ([ADPHC](https://menafn.com/1111766961/Abu-Dhabi-Records-More-Than-280-Increase-in-Cancer-Screenings-and-Strengthens-Adoption-of-Preventive-Health-Behaviours)).
- AHS alone had about 1,429 FIT+ in 2015–16 ([PMC8343888](https://pmc.ncbi.nlm.nih.gov/articles/PMC8343888/)), so 1.8k is a credible floor.

Plan on 5.7k. The pilot's first deliverable is one count query against the Screenings registry.

## 5. Female GI consultants: drop "13%"

We found no Abu Dhabi workforce figure in DoH open data, Statista (paywalled) or medRxiv. The 13% looks like an *international* estimate ("13% of GE consultants are women", [ResearchGate](https://www.researchgate.net/publication/349720046_Rising_Trend_of_Women_in_Gastroenterology_A_Paradigm_Shift), snippet only).

Verified replacements:
- 69.6% of FIT+ patients were women, and some requested a female endoscopist (PMC8343888).
- Abu Dhabi's six government hospitals had "just two" female colorectal surgeons in 2019 ([The National](https://www.thenationalnews.com/uae/health/lack-of-female-colon-surgeons-in-uae-leaves-women-embarrassed-to-ask-for-help-1.885133)).

Label the simulator's 40% female-list share [A]. It can be checked by a manual count in the [TAMM directory](https://www.tamm.abudhabi/wb/doh/find-a-doctor?lang=en).
