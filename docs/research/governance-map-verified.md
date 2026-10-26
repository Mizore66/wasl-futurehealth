# Wasl: governance map (verified 23 Oct 2026)

Verified? means correct, corrected or not found. RAI clauses were already verified locally. Saved texts are in `sources/legal/`.

| Topic | Wasl does | DoH RAI | Law 2/2019 [L2] | PDPL [PD] | DoH standards [AD][HIE] | Verified? |
|---|---|---|---|---|---|---|
| **Lawful basis** | Programme duty; reads only data already in the HIE | 3.2.1.3; 3.2.1.7 | Art. 16(3), (5) | Art. 2(2)(e) excludes it. Fallback is Art. 4, items 4–5 | HIE 4.2.1.2; HIE 4.3.12.1.6, .8; ADHICS DP 1.2(1)(d), (e), (h) | L2 **correct**. PDPL **correct** (Art. 4 narrowed to items 4–5). HIE "4.2.x" **corrected** to 4.2.1.2 + 4.3.12.1. ⚠ HIE 4.3.13.2 limits facilities to "clinical treatment, operations", so frame Wasl as DoH public-health/QI use. |
| **Roles (RBAC)** | 5-role matrix, separation of duties | 3.1.4.2.1–3; 3.1.7.1.2 | Art. 8(1)(A); 14; 15(1)(A)–(B) | — | ADHICS AC 2.2 (need to know; MFA for privileged access); DP 1.4(5)–(6) | L2 **correct**. ADHICS **corrected** to AC 2.2 / DP 1.4. |
| **Retention** | Log and ledger kept ≥25 y, in the UAE, write-once. Derived state is rebuilt. | 3.1.4.4.1; 3.2.4.1 | Art. 20(1)(A) | — | ADHICS DP 1.3(5) and DP 1.6(4) (delete when purpose ends) cover derived state only | **Correct**. ADHICS added. |
| **Consent** | `Consent` (deny) suppresses outreach, not tracking | 3.1.6.1.3; 3.1.4.4.2 | Art. 16 | Art. 18 (see oversight) | ADHICS DP 1.2(4) (withdrawal); HIE 4.2.1.2, 4.2.2.3 | All **correct**. |
| **Data residency** | In-UAE Gemma; no external APIs in the pilot | 3.1.4.5.1 | Art. 13; penalty Art. 24 (AED 500k–700k) | — | ADHICS DP 1.4(3)–(4); CO 9.2(6)–(7) | Art. 13 **correct**. MR 51/2021 point **correct in substance** [MR]. ADHICS DP 1.4 **correct**; CO 9.2 added. |
| **Human oversight** | Auto-close only if trusted link + agreement | 3.1.1.2.1–2; 3.1.7.2.2–3 | — | Art. 18(1), (4) | — | **Correct** (PDPL only as a fallback). |
| **Incident handling** | Replay finds wrong-patient messages; anchor mismatch freezes the ledger | 3.1.2.1.4; 3.1.3.1.4; 3.1.4.5.3; 3.3.1.4.1 | Art. 25 (sanctions only; no notification duty) | Art. 9 ("immediately"; period set by Exec. Regs) | ADHICS DP 1.7(3)–(5); IM 2.1(5) + Guidelines SLA matrix [AG]; HIE 4.3.17.3; HIE 6.1.2.27 | DP 1.7 72 h **correct**. IM 2.1 **corrected**: notify AD Health SOC within 2 h (P1/P2), 8 h (P3) or 24 h (P4) *of resolution*, not "near-real-time". HIE 60 d **correct**. **Added**: DoH + ADHIE within 5 working days. |
| **Audit** | Hash-chained provenance, anchored heads, replay | 3.1.6.2.3; 3.1.7.1.3; 3.1.3.1.5 | Art. 4(2); 15(1)(C) | Art. 21 (fallback) | HIE 6.1.3.12–13 (ADHIE's own duty); ADHICS CO 6.1(11), DP 1.5 (DPIA) | **Correct**. DP 1.5 = binding DPIA. |
| **Fairness** | Report by name origin; suppress cells <5 | 3.1.5.1.2–3 | — | — | — | RAI only |
| **Risk tier** | Self-classified High; DDGO conformity assessment | 3.3.1.2.2.2; 3.3.1.1.2 | — | — | — | RAI only |

## Key sentences

- **L2 Art. 13:** "It is not permissible to store, process, generate or transform the health data and information outside State … except in the case where a resolution is issued from the Health Authority in coordination with the Ministry."
- **L2 Art. 20(1)(A):** The keeping period "shall not be less than (25) twenty-five years from the last date of the health procedure."
- **L2 Art. 16:** Patient information may not be used "except in the health purposes without written approval from the patient." Exceptions (3) public health; (5) Health Authority monitoring.
- **PD Art. 2:** The law does not apply to "e. health personal data that is subject to legislation regulating the protection and Processing thereof."
- **PD Art. 9:** The controller shall, "immediately upon becoming aware … report … to the Office within such period … as set by the Executive Regulations."
- **AD DP 1.4(4):** "Ensure health information and its copies in any form, whether encrypted, anonymized, deidentified, pseudonymized, etc., are not stored, processed, or transferred outside the UAE."
- **AD DP 1.7(3):** The "Data Breach Form … shall be shared with the DoH within 72 hours of acknowledging the incident."
- **HIE 4.2.1.2:** Where consent is unavailable, "DOH … requires and authorizes Healthcare Facilities to share and receive such Patient Data."
- **HIE 4.3.17.3:** Patients "must be notified of a Breach without undue delay but in no event later than 60 days from discovery."
- **HIE 6.1.2.27:** Participants notify "the ADHIE and the DOH of any breach … in any event within 5 working days."

## Caveats

- **PDPL:** Likely does not apply (Art. 2(2)(e); government authorities are also excluded, Art. 2(2)(a)–(b)). Its breach period awaits Executive Regulations, reportedly unissued in mid-2026. "72 h to the Data Office" is **not found** in the law.
- **MR 51/2021:** Verified from a law-firm summary only. None of its cases (treatment abroad, research, insurance, telehealth etc.) covers AI processing or hosting.
- HIE policy is v1.1 (Apr 2020). The SLA matrix is "suggested" in the 2019 Guidelines. English law texts are unofficial; Arabic prevails.

## Sources

- [L2] Federal Law 2/2019 (official site blocked here): https://uaelegislation.gov.ae/en/legislations/1209. Read via mirror: https://lexaidxb.com/en/legislations/uae-1209
- [PD] FDL 45/2021: https://uaelegislation.gov.ae/en/legislations/1972. (read via Lexis Middle East copy)
- [MR] https://cms.law/en/are/legal-updates/uae-health-data-new-data-transfer-permissions
- [AD] ADHICS v2 (DOH/SD/ICSO/ADHICS/V2/2024): https://www.doh.gov.ae/-/media/Feature/Resources/Standards/ADHICS-v2-standard.ashx
- [AG] ADHICS Implementation Guidelines (2019): https://www.doh.gov.ae/-/media/Feature/Resources/Guidelines/20191209_ADHICS-Implementation-Guidelines.ashx
- [HIE] DOH/POL/STR/HIE/1.1/2020: https://www.doh.gov.ae/-/media/A78104416DF2440E89AC90FB75F11055.ashx
