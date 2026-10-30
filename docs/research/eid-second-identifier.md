# Emirates ID (EID) as the "second identifier" for Wasl follow-up release

Research date: 2026-10-30. Public web sources only (24 searches, a few page fetches). The DoH website (doh.gov.ae) refused direct fetches from this machine, so quotes from DoH documents come from search-engine snippets of those pages and are marked "(via search snippet)". Treat them as needing one manual check against the PDF before they go into the write-up.

## Summary (5 lines)

1. Standards: JCI and the Joint Commission accept any two person-specific identifiers, explicitly including an assigned ID number; JCI's own FAQ suggests a national ID as one of the two, so name + EID satisfies the rule without DOB (but these are bedside-ID rules, not record-linkage rules; no public DoH/MOHAP bedside policy was found).
2. Error evidence: no public EID error rate exists; DoH and DHA both name wrong/poorly captured EIDs and duplicates as known problems and mandate the card reader. Comparable data (US SSN, Just et al. 2016) show ID mismatches are mostly blanks/default values, and >92% of non-blank mismatches differ by 3+ digits, i.e. not keying errors.
3. Luhn catches all single-digit and ~98% of adjacent-transposition errors, but nothing when the number is another real person's (a relative's, the mother's for a newborn, an "intertwined" record), and it is not officially documented that EID uses Luhn (developers report real IDs that fail it).
4. Placeholder DOBs are real and large-scale in the region (Saudi 1 Rajab for "about half" of citizens per one outlet; 1 Jan common for Pakistani/Afghan nationals); Wikidata stores year-only dates with precision 9, sometimes as YYYY-01-01.
5. Recommendation: YES WITH CONDITIONS. Accept a shared EID only if: same 15-digit number on both registrations, prefix 784, not a DoH default (000/111/222/999 patterns) or other repeated-digit dummy, Luhn-valid (failures go to human review, not rejection), name agreement (transliteration-tolerant) and sex agreement, DOB not contradictory (at least birth year equal when one side is a placeholder), the EID is not seen with a different name/sex anywhere else in the dataset, and the patient is not a newborn/infant registered before their own EID existed.

---

## 1. Which identifiers count under patient-identification standards

### Joint Commission NPSG.01.01.01 (US)
- Requirement: two identifiers; an assigned identification number is listed as acceptable.
  - Quote (Standards FAQ): "name, an assigned identification number, telephone number, date of birth" (list of acceptable identifiers).
  - URL: https://www.jointcommission.org/en/knowledge-library/support-center/standards-interpretation/standards-faqs/000001463 (direct fetch returned HTTP 403; quote via search snippet)
  - The NPSG text (Hospital program, effective Jan 2025) rationale lists name, "an assigned identification number, telephone number" or other person-specific identifier: https://digitalassets.jointcommission.org/api/public/content/9be383450fc941df806b76c5fbdd9ae6?v=3c600c3a (via search snippet)
- Not acceptable: room number or physical location.
- DOB is not mandatory: it is one option among several. Name + ID number satisfies the rule.

### JCI IPSG.1 (international; most relevant for UAE hospitals, many are JCI-accredited)
- ME 1: "two patient identifiers, not including the use of the patient's room number" (via search snippet of copies of the JCI standard).
- JCI Standards Interpretation FAQ (copy on SlideShare/Scribd, not jointcommission.org) recommends specific identifiers and notes: "in many countries, they use their national ID number as one of the two".
  - https://www.slideshare.net/slideshow/jci-standards-interpretation-for-hospital/277581933
  - https://www.scribd.com/document/407280807/JCI-Standards-Interpretation-FAQs
  - NOT VERIFIED against JCI's own current (8th edition) FAQ; the copies may be older.
- Same FAQ copy notes name + DOB fails for newborns born the same day with the same surname (relevant: DOB is not always discriminating either).

### WHO Patient Safety Solution 2 (2007)
- Recommends "at least two identifiers (e.g. name and date of birth)" on admission/transfer and before care; neither should be the room number. Name + DOB is an example, not a requirement.
- URL: https://cdn.who.int/media/docs/default-source/patient-safety/patient-safety-solutions/ps-solution2-patient-identification.pdf

### Abu Dhabi DoH / UAE MOHAP / DHA
- No public DoH or MOHAP bedside patient-identification policy naming which two identifiers to use was found. NOT FOUND.
- DoH HIE (Malaffi) Standard 1 - Patient Identifier Information (via search snippet): poor demographic capture "is impacting patient matching in the Malaffi HIE"; facilities must capture EID with an integrated card reader, which populates "the EID number, all names of the patient and DOB correctly". Circular date not visible in snippet.
  - https://www.doh.gov.ae/-/media/Feature/Resources/Health-Information-Exchange-standards/Standard-1.ashx
- DoH Standard for Visa Screening Program is the closest DoH "two identifiers" rule (via search snippet): applicants are "Uniquely identified using at least two identifiers", namely the unified number (new applicants) or Emirates ID (renewals) plus at least one of full name, DOB, nationality. So DoH itself treats EID + name as a valid two-identifier pair in that program.
  - https://www.doh.gov.ae/-/media/58A2D120CEEA4627BC887D28BDED902B.ashx
- DHA (Dubai) Health Data Quality Policy 2022 (via search snippet): names "incorrect Emirates ID numbers" and duplicate records as data-quality issues and requires an Emirates ID reader at registration.
  - https://dha.gov.ae/uploads/082022/Health%20Data%20Quality%20Policy2022839141.pdf
- Federal Law No. 2 of 2019, Article 21 reportedly requires recording the EID in all health transactions (secondary source only: https://www.uaeexperthub.com/medical-records-uae-who-can-see-them/). NOT VERIFIED against the law text.
- Emirates Health Services (MOHAP's operator) is replacing its health card with EID as "the primary patient identifier" (Gulf News): https://gulfnews.com/uae/health/uae-to-scrap-health-card-use-emirates-id-for-treatment-1.500461904
- EID number is fixed for life and "may not be amended" even when name/DOB change (Gulf News, EIDA statement): https://gulfnews.com/amp/story/uae%2Fgovernment%2Fid-number-may-not-be-amended-eida-says-1.1654705. The same piece says the 4-digit block "does not match the year of birth necessarily", so never infer DOB from the EID.

### Answer to Q1
- Yes, a government/national ID number counts as one of the two identifiers under JC, JCI and WHO wording (all list an assigned ID number or give name + DOB only as an example).
- Name + EID without DOB is acceptable under the wording of all three; DoH's visa-screening standard explicitly pairs EID with name.
- Caveat for the write-up: these standards govern bedside identification of a present patient, not deterministic linkage of two registrations. Using them for linkage is an analogy, not a direct authority.

---

## 2. Published error rates for national IDs in health records

### Emirates ID specifically
- No published EID error rate, duplicate rate, or overlay rate was found (Malaffi, Riayati, NABIDH, ICP, academic). NOT FOUND.
- Qualitative official evidence that EID errors occur:
  - DoH Standard 1 (above): demographic capture problems affect Malaffi matching and raise duplicate likelihood.
  - DHA Health Data Quality Policy 2022 (above): "incorrect Emirates ID numbers" listed as a common issue.
  - Malaffi FAQ: "Incorrectly entered demographic details in an EMR could prevent proper matching" (https://www.malaffi.ae/frequently-asked-questions-end-users/).
- DoH-defined default EIDs, i.e. one number shared by many people (Shafafiya data dictionary): 000-0000-0000000-0 (national without card), 111-1111-1111111-1 (expatriate resident without card), 222-2222-2222222-2 (non-national non-resident), 999-9999-9999999-9 ("Unknown status, without a card").
  - https://www.doh.gov.ae/en/shafafiya/dictionary/type/claim-emirates-id-number
  - https://shafafiyaportal.doh.gov.ae/dictionary/CommonTypes_xsd~s~PersonEmiratesIDNumber.html
  - These must be excluded. My check: none of the four start with 784; 000000000000000 passes Luhn, the other three fail. So "prefix 784 + Luhn" excludes all four, but an explicit blocklist is still needed for ad-hoc dummies such as 784-0000-0000000-x.
- Newborns: the mother's insurance covers the baby for the first 30 days and the baby's own EID comes only after birth certificate, passport and visa (up to 120 days), per u.ae and insurer guides. https://u.ae/en/information-and-services/social-affairs/Life-events/having-a-baby , https://hayah.com/knowledge-centre/articles/add-newborn-health-insurance-uae . Whether hospitals record the mother's EID on the baby's registration: NOT VERIFIED (inference; this is the classic "ID of a relative" risk).

### US Social Security number (best quantitative source found)
Just et al. 2016, "Why Patient Matching Is a Challenge", Perspectives in Health Information Management (AHIMA). https://pmc.ncbi.nlm.nih.gov/articles/PMC4832129/
- 398,939 confirmed duplicate pairs (same person, two records), 10 US states.
- SSN discrepancy in 213,003 / 398,939 pairs (53.4%; abstract says 53.54%).
- Of the 213,003 SSN discrepancies: 106,965 (50.2%) involved default values like 999-99-9999; 123,894 (58.2%) were blank on one side (categories overlap).
- Of 87,473 pairs with two comparable SSNs: 1 digit different 4,712 (5.4%), 2 digits 2,087 (2.4%), i.e. 7.8% plausible keying errors; "More than 92 percent" differed in 3+ digits (wholly different numbers: wrong person's/relative's SSN or fabricated).
- DOB discrepancy in 24,781 / 398,939 pairs (6.2%); within those, month-day swap 635 (2.6%).
- Background: an EHR typically has an "8 percent to 12 percent rate of duplicate records".
- Implication: a shared ID is far more often missing/default than mistyped, and when two IDs differ they are usually entirely different numbers, which a check digit cannot detect.

### NHS number (England/Wales)
- Hagger-Johnson et al., BMJ Open ~2015 (PICANet vs HES linkage using HESID, which uses NHS number + other fields): false match rate 0.2%, missed match rate 4.1%. https://discovery.ucl.ac.uk/id/eprint/1472285/1/Data%20linkage%20errors%20in%20hospital%20administrative%20data%20when%20applying%20a%20pseudonymisation%20algorithm%20to%20paediatric%20intensive%20care%20records..pdf . Denominator (number of PICANet records) NOT VERIFIED from the full text; snippet reports false matches n=176 (0.1%) in one table.
- NHS PDS definitions: "confusion" = "one NHS Number is being used by 2 or more NHS patients" (the overlay case). https://digital.nhs.uk/services/personal-demographics-service/national-back-office-for-the-personal-demographics-service/data-quality-incidents . No rate given.
- Hansard 1996, duplicate NHS numbers: "approximately 1 per cent" appears but its referent is unclear. https://hansard.parliament.uk/commons/1996-11-12/debates/4ec91c44-42a2-4742-824d-6f9da4d4171e/DuplicateNhsNumbers . NOT VERIFIED; do not cite as a rate.
- A vendor blog claims 3-5% PAS duplication in some trusts (unsourced "2023 audit"). NOT VERIFIED; do not cite.

### Australian Medicare number / IHI (wrong person sharing an ID)
- ANAO: "intertwined" Medicare records are where a single Medicare record "has been used interchangeably between two or more individuals", most often with similar names and DOBs. https://www.anao.gov.au/work/performance-audit/implementation-the-my-health-record-system
- 2015-16: Medicare claims uploaded to the wrong person's My Health Record 86 times; these were most of 94 breaches affecting 103 people. https://www.itnews.com.au/news/medicare-claims-data-sent-to-the-wrong-health-records-441292
- 2017-18: 17 of 42 breaches (28 notifications) came from identifying intertwined Medicare records. https://www.healthcareitnews.com/news/anz/my-health-record-system-data-breaches-rise
- 2018-19: 27 of 38 breaches from intertwined Medicare records. https://ia.acs.org.au/article/2019/myhealthrecord-complaints-skyrocket.html
- Denominators are breaches, not records; no per-record rate found. IHI has a "replica" resolution status (two patients sharing one IHI) but no counts were found. https://meteor.aihw.gov.au/content/index.phtml/itemId/743464
- Lesson for Wasl: a valid, correctly keyed national ID can still belong to someone else, and similar name + DOB is exactly the situation where this happens. Name agreement is necessary but not sufficient; uniqueness checks across the registry help.

### Danish CPR
- DNPR automatically checks for "errors in CPR numbers" and returns records to hospitals (Schmidt et al. 2015). https://www.tandfonline.com/doi/full/10.2147/CLEP.S91125 . No rate in the accessible text. NOT FOUND.
- Since October 2007 some real CPR numbers fail the check digit (number space exhausted). https://en.wikipedia.org/wiki/Personal_identification_number_(Denmark) . Relevant analogy: check-digit failure does not prove an ID is fake.
- Missing CPR 5-9% in the Danish Prehospital Medical Record (secondary snippet on ResearchGate). NOT VERIFIED.

### Saudi national ID, Singapore NRIC
- Saudi: no published ID-error rate found. One Saudi QI audit found two identifiers used in only 33 observations (18%); 147 (82%) checked name only (via ResearchGate snippet; denominator 180 implied). https://www.researchgate.net/publication/334968218_Assessing_Knowledge_and_Compliance_of_Patient_Identification_Methods_in_a_Specialized_Hospital_in_Saudi_Arabia . NOT VERIFIED beyond the snippet.
- Singapore chose NRIC for its National Patient Master Index in 1995 to prevent duplicates (review, no error rates). https://arxiv.org/pdf/1803.05994

---

## 3. What the Luhn check catches and misses

- Wikipedia: Luhn "will detect all single-digit errors, as well as almost all transpositions of adjacent digits"; it does not detect 09 <-> 90; it misses twin errors 22<->55, 33<->66, 44<->77. https://en.wikipedia.org/wiki/Luhn_algorithm
- Error mix in manual transcription (Verhoeff's 1969 study of 6-digit numbers, via Wikipedia): single-digit 9,574 (79.05%), adjacent transposition 1,237 (10.21%), jump transposition 99 (0.82%), twin 67 (0.55%), phonetic 59 (0.49%). https://en.wikipedia.org/wiki/Verhoeff_algorithm
- My calculation (not from a source): Luhn misses 2 of 90 ordered adjacent transpositions (09/90), so detects ~97.8% of them; it detects 0% of jump transpositions (abc -> cba; same-weight positions); twins ~93% (6 of 90 missed). Applied to Verhoeff's mix, Luhn catches roughly 89% of keying errors overall. Verhoeff's data are old and Dutch; treat as indicative only.
- What Luhn cannot catch at all: a valid number that belongs to another person (relative's EID, mother's EID on a newborn, card picked up for the wrong family member, intertwined records). In Just et al. 2016, >92% of non-blank SSN mismatches were 3+ digit differences, i.e. the dominant error class is "different valid-looking number", invisible to any check digit.
- Also irrelevant to Wasl's main case: if both registrations carry the identical EID, Luhn adds no matching evidence; its value is filtering garbage/dummy entries before they are trusted.
- EID uses Luhn? Microsoft Purview's EID classifier says the number "must pass the Luhn check" (https://learn.microsoft.com/en-us/purview/sit-defn-uae-identity-card-number). But ICP publishes no spec, and a developer gist reports a real ICP-accepted EID that fails Luhn (https://gist.github.com/geordee/e51d111426de675c0c0f8503c2003047). NOT VERIFIED officially. Hence: Luhn failure should route to human review, not be treated as proof of a bad number.

---

## 4. Placeholder DOBs and Wikidata year-only dates

### Gulf / South Asian placeholder DOBs
- Saudi Arabia: a Supreme Order adopted 1 Rajab (Hijri 1/7) as the official birth date when day and month are unknown (Saudipedia). https://saudipedia.com/en/national-identity-card
  - Arab News: assigned "as the date of birth for millions of Saudis". https://www.arabnews.jp/en/saudi-arabia/article_38906/
  - News50 claims it is the birthday of "about half of the Saudi citizens". https://news50.sa/en/politics/today-july-1st-marks-the-birthday-of-millions-of-saudis/ . Single non-official source; treat as unverified. Note: 1 Rajab is a Hijri date, so its Gregorian equivalent varies by year; "1 July" in that headline is a loose rendering.
- Pakistan / Afghanistan (large UAE expatriate communities): NPR reports 1 January is a common recorded birthday; a visa officer first thought it "was a glitch". https://www.npr.org/2024/01/01/1222380438/the-reason-why-there-are-many-jan-1-birthdays-in-pakistan-and-afghanistan . No official figures.
- UAE-specific: no source found on how ICP records unknown DOBs (1 Jan or 1 Jul) on Emirates ID, and no count of placeholder DOBs in UAE registries. NOT FOUND. ICP follows the passport for DOB corrections (third-party guide: https://visasimplified.com/blog/emirates-id-data-correction-name-dob), which suggests passport placeholders flow into EID records (inference).

### Wikidata
- Year precision is precision 9; the Help page's precision table shows "+2026-00-00T00:00:00Z" for a year-only value, but its own example stores a precision-9 date as "+1838-01-01T00:00:00Z". https://www.wikidata.org/wiki/Help:Dates
- The same page warns a 1 January DOB can arise when a source database "stores year precision dates with January 1", and says such dates should be corrected to year precision.
- So yes: year-only Wikidata DOBs can appear as YYYY-01-01 (and in SPARQL/RDF output xsd:dateTime cannot hold 00, so they surface as -01-01; this RDF point is from general knowledge, NOT VERIFIED on an official page in this session). Always read wikibase:timePrecision (9 = year) before treating a 1 Jan DOB as exact.

---

## Proposed rule (for the pipeline)

Release on EID as second identifier only if ALL hold; otherwise human review:
1. Both registrations have a 15-digit EID, identical after stripping hyphens, starting 784.
2. Not a DoH default (000.../111.../222.../999...) and not a repeated-digit or all-zero-body dummy (blocklist).
3. Luhn-valid (failure -> review, not rejection; no official confirmation EID uses Luhn).
4. Name agreement (transliteration-tolerant, token-set) and sex agreement.
5. DOB not contradictory: if one side is a placeholder, birth years must match; two different real DOBs -> review.
6. Uniqueness: the EID never appears in the registry with a different name or sex (guards against intertwined/relative/shared IDs).
7. Not an infant registration created before the child could have its own EID (e.g. age < 120 days at registration), where the mother's EID may have been used.
8. Prefer, where a field exists, EID captured by card reader over manual entry (DoH Standard 1 and DHA 2022 both mandate the reader).

Note for Wasl: the screening programmes (CRC, cervical, lung) enrol adults only, so the newborn condition does not arise; the rule does not check card-reader capture because the feeds do not record it.
