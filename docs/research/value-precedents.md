# Wasl: stage-shift value, integration benchmark, precedents (v10)

[A] = assumption. AED = USD × 3.6725. Accessed 23 Oct 2026.

## 1. Stage-shift value model

**Yield per 1,000 completed colonoscopies after FIT+**
- **CRC:** 25 at SEHA/AHS (11/436) ([PMC8343888](https://pmc.ncbi.nlm.nih.gov/articles/PMC8343888/)) and 31 at Kaiser ([Corley, JAMA 2017](https://pmc.ncbi.nlm.nih.gov/articles/PMC6343838/)).
- **Advanced adenoma:** at least 46 at SEHA (about 20 tubulovillous or high-grade lesions; size not reported).

**Stage at diagnosis**
- Dutch FIT screen-detected CRC: 66.5% stage I–II, so **33.5% III–IV**. Clinically detected CRC: 40–57% I–II ([Kooyker 2023](https://pubmed.ncbi.nlm.nih.gov/37619823/)).
- Abu Dhabi: **67%** of 2023 CRC was diagnosed late ([WAM/Khaleej Times](https://www.khaleejtimes.com/uae/abu-dhabi-introduces-blood-based-biopsy-test-for-colorectal-cancer-screening)).

**Cost gap per stage-shifted cancer (C).** There is no UAE study.
- **Base:** Riyadh, first-year costs (2020 USD): stage I $45,317, stage IV $71,374. The gap is **≈AED 96k** ([PMC10630777](https://pmc.ncbi.nlm.nih.gov/articles/PMC10630777/)).
- **High:** US SEER-Medicare, initial phase (2014 USD), I/II mean versus III/IV mean. The gap is $50,286, **≈AED 185k** ([PMC7225669](https://pmc.ncbi.nlm.nih.gov/articles/PMC7225669/)).
- **Low:** AED 48k [A].

**Formula.** AED avoided per year = **V × u × PPV × Δs × C**, where:
- V = FIT+ per year
- u = uplift in colonoscopy completion
- PPV = CRC found per completed colonoscopy
- Δs = the drop in the stage III–IV share when the cancer is found by colonoscopy rather than presenting later

| Scenario | PPV | Δs | C | AED per extra completer |
|---|---|---|---|---|
| Low | 2.5% | 0.10 (43.5 − 33.5) | 48k | 120 |
| **Base** | 2.5% | 0.31 (≈65 − 33.5) [A] | 96k | **742** |
| High | 3.1% | 0.31 | 185k | 1,775 |

**Uplift.** Wasl doesn't raise attendance by itself.
- Our projection uses published effects of reminders, navigation and automated referral. It gives **+5.3 pp at ≤31 days** and **+21.6 pp by day 120**.
- The PRECISE navigation RCT found **+13.0 pp** [6.5–19.4] ([Annals 2025](https://pmc.ncbi.nlm.nih.gov/articles/PMC13007779/)).
- The booking simulator reaches 52%, but only at assumed capacity.

**Base case.** Ranges in brackets are low–high.

| FIT+/yr | +3 pp | +10 pp | +20 pp |
|---|---|---|---|
| 1,760 | 1.3 CRC earlier; 0.4 shifted; **39k** [6–94k] | 4.4; 1.4; **131k** [21–312k] | 8.8; 2.7; **261k** [42–625k] |
| 5,700 | 4.3; 1.3; **127k** [20–303k] | 14; 4.4; **423k** [68k–1.0M] | 29; 8.8; **845k** [136k–2.0M] |
| 12,620 | 9.5; 2.9; **281k** [45–672k] | 32; 9.8; **936k** [151k–2.2M] | 63; 20; **1.87M** [302k–4.5M] |

**Against the year-1 cost of AED 0.42–1.33M**
- In the base case, break-even needs **566–1,793 extra completers**.
- That means +10 to +31 pp at 5.7k FIT+/yr, or +4.5 to +14 pp at 12.6k. It is unreachable at 1.8k.

**Caveats**
- Each extra completer also costs a colonoscopy (about AED 4,000, a weak indicative figure from [zavis](https://www.zavis.ai/directory/abu-dhabi/procedures/endoscopy)). After that, the net cash saving is small or negative.
- The stronger value is mortality. Non-compliers had 16.0 vs 6.8 CRC deaths per 1,000 over 10 years (adjusted HR 2.03; [Zorzi, Gut 2022](https://doi.org/10.1136/gutjnl-2020-322192)). Transferred, that is about **9 deaths averted per 1,000 extra completers** [A].
- Not counted: adenoma removal, continuing and end-of-life costs, and breast/cervical.
- The SEHA PPV rests on only 11 cancers. Δs assumes unscoped cancers present like Abu Dhabi's average.

## 2. Integration-effort benchmark

- **Malaffi:** onboarding "can take up to several months" ([Malaffi](https://www.malaffi.ae/providers/definitions/)). Hospitals went from 60% to 100% connected over 12 months ([milestones](https://www.malaffi.ae/what-is-malaffi/history-and-milestones/)).
- **Orion Amadeus and NextGate MPI:** live "six months after contracts were signed" ([Orion](https://orionhealth.com/us/media-releases/orion-health-delivers-the-first-health-information-exchange-hie-in-the-middle-east/)). No effort figure is published.
- **Riayati:** about 2 months ([Balsam](https://blog.balsammedico.com/blog/2024/11/14/riayati-onboarding-guide/)). **NABIDH:** 6–8 weeks ([Zavis](https://www.zavis.ai/guides/nabidh-ehr-integration-dubai-clinics)) to 2–6 months ([EMR Magazine](https://emrmagazine.com/2025/06/20/nabidh-implementation-process/)). These are vendor figures.
- **US:** $50k per interface (Walker, Health Affairs 2005, expert opinion; [PubMed](https://pubmed.ncbi.nlm.nih.gov/15659453/)). Vendors quote $10–25k ([Idea Usher](https://ideausher.com/blog/hl7-integration-cost/)), 3–6 months for basic ADT, 12–18 months for comprehensive HIE work, and 15–25%/yr maintenance ([Invene](https://www.invene.com/blog/hl7-interface)).

**Verdict: keep 28–54 person-weeks (≈$46–147k).**
- That covers 4 interfaces [A]: ORU/ADT, report text, Task write-back and SSO.
- It sits above the vendor range ($40–100k) and below Walker ×4 ($200k).
- **Add:** 6–9 months elapsed time, 15–25% maintenance in year 2, and ADHDS-side effort, which is unpriced [A].

## 3. Precedents

| Precedent | Vehicle | Date |
|---|---|---|
| **Philips AI: a "PoC… to integrate an AI algorithm into the existing Malaffi HIE", piloted at two hospitals before rollout** | DoH–ADHDS–Philips MoU | **17 Oct 2024** ([DoH](https://www.doh.gov.ae/en/news/doh-adhds-and-philips-are-set-to-launch-a-new-precedent-for-ai-powered-healthcare-in-abu-dhabi)) |
| **Guardant Shield in the CRC programme** | ADPHC agreement via Hikma; M42 administers | **30 Jan 2025** ([Guardant](https://investors.guardanthealth.com/press-releases/press-releases/2025/Guardant-Healths-Shield-Test-Selected-by-Abu-Dhabi-Department-of-Health-for-Blood-Based-Colorectal-Cancer-Screening-Program/default.aspx)) |
| Orion and NextGate MPI as Malaffi's core | ADHDS contract under the PPP | Jan 2019 |
| 12 registries, including Screenings; AI next, via Population Health Intelligence | DoH and ADPHC, with ADHDS delivering | 20 Jul 2026 ([release](https://healthmagazine.ae/press_release/abu-dhabi-disease-registries-proactive-prevention-first-care/)) |
| Population risk analytics; Patient Risk Profile (no vendor named) | Malaffi features | Dec 2020; Aug 2022 ([Zawya](https://www.zawya.com/en/press-release/malaffi-platform-to-use-predictive-analytics-technology-to-improve-the-health-of-abu-dhabi-residents-681016)) |
| Microsoft Population Health Intelligence framework | DoH MoU | 21 Apr 2025 ([EME](https://economymiddleeast.com/news/adghw-2025-abu-dhabi-doh-microsoft-ai-powered-population-health-intelligence-framework/)) |
| VitalHub (5-year deal); Digi7 (UAE AI startup) | ADHDS partnerships | Feb 2024; Oct 2024 ([milestones](https://www.malaffi.ae/what-is-malaffi/history-and-milestones/)) |

**Gap.** DoH has supported 80 startups since 2021, but we found **no public case** of a startup pilot turning into a Malaffi contract.

**Use** two precedents together:
- Philips, as the template: an MoU, then a PoC inside Malaffi, then rollout.
- Guardant, as proof that ADPHC buys third-party CRC services through an intermediary.
