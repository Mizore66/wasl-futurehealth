# Wasl: grounding in Abu Dhabi standards

CRC (2019), CX cervical (2022), BR breast (2025), IFHAS (2025), PHC (2022), Jawda 2026. WD = working days.

## 1. SLA table

| Step | Official value | Source and quote |
|---|---|---|
| Result to patient, CRC | <15 WD | CRC 5.5.2 "less than 15 working days (acceptable standard >90%…)" |
| FIT lab result | 7 days | CRC App 2 "7 days from sample's dispatch" |
| Result to patient, cervical | 21 days | CX 10.6.2 "within 21 days (3 weeks) of the date of specimen taken" |
| Result to patient, breast | 15 WD; positives immediately | BR 3.7.1 "within 15 working days"; 3.8.1 "If positive the result shall be reported immediately" |
| Result to patient, IFHAS | 10 days | IFHAS 4.9.2.1 "within 10 days of the initial visit" |
| FIT+ → referral | At the visit; 15 WD | CRC 5.5.1 unit "must provide… DOH e-Cancer Screening Referral Form"; IFHAS App 9 "within 15 working days of releasing the results" |
| Referral → colonoscopy | 31 days | CRC 5.5.4.2 "within 31 days of referral"; App 4.7 "within 31 days from a positive FIT test" |
| Colonoscopy report | No deadline | App 14 lists fields only |
| Histology | None for CRC | Breast only: biopsy result "5 working days" (BR App 3) |
| Treatment referral | CRC 31 days; breast 10 WD | CRC 5.5.3.4 "less than 31 working days (… no more than 31 days)"; BR 3.9.11 "within 10 working days of the diagnosis" |
| Colposcopy | HSIL+ 1–2 wk; LSIL 2–4 wk; ASC-US/ASC-H 4–8 wk | CX App 5; visible lesion "referred immediately" (10.6.6) |
| Breast assessment | 5 WD + 5 WD | BR App 3 "offered assessment within 5 working days"; "Assessment and issuing of results within 5 working days" |
| PHC (guidance) | 14 days | PHC App 1 "within 2 weeks (14 calendar days)" |

Jawda 2026: no cancer follow-up KPI.

**Wasl defaults compared with the standards**

- **Notification ≤21 days.** Official for **cervical only**. CRC and breast require 15 WD; IFHAS requires 10 days. Clock per programme.
- **Acknowledgement 7 days.** **Our assumption.** CRC's "7 days" is the lab turnaround.
- **Referral response 7 days.** **Our assumption.** The nearest official figure is IFHAS's 15 WD from result to referral.
- **Colonoscopy ≤31 days.** **Official**, but the clock start is ambiguous (from referral or from FIT+). Start from the FIT+ date; stricter.
- **Conflicts in the text.** Treatment referral is given as 31 working days in one place and 31 days in another; use calendar days. ASC-H is "routine" in App 5 but grouped with HSIL+ in App 4; treat it as urgent.

## 2. Bowel prep and caecal intubation

**What the CRC standard requires:**

- **Prep documentation:** App 9.4 says "type of bowel preparation must be documented". App 9.5: "Inadequate bowel preparations must not exceed 10% of examinations."
- **Prep audit:** App 2 note: "at least more than 90 % of examinations should be rated as 'adequate' bowel cleansing or excellent". Grades are Excellent, Adequate and Inadequate.
- **Report form:** App 14 includes "Bowel preparation… Quality", "If incomplete examination, state reason" and "Cecum identified by".
- **Completion evidence:** App 11.2.2: "Auditable photo documentation of colonoscopy completion must be available; including a panoramic image of the appendiceal orifice, ileo-cecal valve and cecum or a video clip". Withdrawal time ≥6 minutes (11.2.4).
- **Caecal intubation rate (CIR):** >90% acceptable, >95% desirable (App 2); ">95%" (App 4).

**Can it count?** The standard never defines "completed follow-up" and never requires a repeat after inadequate prep. But prep quality "must be documented", so an ungraded report does not conform and cannot count toward the 90% adequacy audit. Counting it as closed would be our inference, not the standard.

**Default policy:**

- **Close the loop** only when caecum is photo-documented, prep is graded Excellent or Adequate, and histology has resulted.
- **Prep ungraded:** state is `done_quality_unverified`. The loop stays open and Wasl queries the unit.
- **Prep Inadequate, or caecum not reached:** state is `incomplete`. The case is flagged for a repeat; DoH gives no interval.

## 3. Endoscopy capacity proxies (public data only, accessed 2026-10-23)

- **Endoscopy units:** no public count for Abu Dhabi or the UAE.
- **Gastroenterologists:** 86 consultants in Abu Dhabi in 2023, up from 29 in 2018 ("female consultants from 3 to 11"); 5.23 per 100k. Sources: Al Ghamdi et al., abstract ~2025, https://www.thieme-connect.com/products/ejournals/abstract/10.1055/s-0045-1805808, and the not-peer-reviewed preprint https://papers.ssrn.com/sol3/papers.cfm?abstract_id=5516549. Snippets only.
- **Female share:** about 13% of consultants (same source). No figure for endoscopists. Requests for a female endoscopist are anecdotal only (Almansoori, below).
- **Colonoscopy volumes:** no emirate total. Cleveland Clinic Abu Dhabi reports "six AI-assisted colonoscopy units" and ">2000" screenings Jan–Jul 2024 (30 Jul 2024): https://www.clevelandclinicabudhabi.ae/en/media-center/news/ccad-screens-over-2000-patients-for-colorectal-cancer-using-ai-assisted-colonoscopy
- **Waiting times:** none published. Proxy: median **72 days** from FIT+ to colonoscopy, against the 31-day standard.
- **Abu Dhabi screening** (SEHA, Emiratis, 2015–16 data):
  - 23.5% screened, with **13.5% FIT positivity**;
  - only **30.5%** of FIT+ patients had a colonoscopy;
  - 36.3% of those without one were referred but did not attend.
  - Source: Almansoori et al., *BMC Cancer* 21:897, 6 Aug 2021, https://pmc.ncbi.nlm.nih.gov/articles/PMC8343888/
- **UAE uptake:** 9.1% (survey, 16 Oct 2023): https://pubmed.ncbi.nlm.nih.gov/37845665/
- **No public data after 2016** on Abu Dhabi programme volumes, positivity or follow-up.
