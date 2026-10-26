# v7-booking: capacity-aware colonoscopy booking (SIMULATED)

150 parameter draws, 1,000 FIT+ per year; medians [95% interval]. KPI = DoH Appendix 2: share of colonoscopies done within 31 days.

## Engine (`booking/`)
- **Interface:** `handle(Event)` returns a `Booking` or FHIR `Appointment`.
- **Method:** a daily batch solved as an exact min-cost assignment. Overbook copies carry a convex Poisson-binomial collision cost, so each batch is ILP-optimal. Over the year it is myopic.
- **Checks:** matches a HiGHS ILP on 40/40 batches. Hard constraints held on 996/996 bookings.
- **What it does:**
  - books at acceptance;
  - enforces female, Ramadan and region constraints;
  - prioritises by deadline slack;
  - overbooks while P(collision) ≤ε;
  - calls high-risk patients by nurse;
  - rebooks twice, then escalates.

**Calibration (status quo vs SEHA, PMC8343888):** median 70 [56–90] vs 72 days; IQR 111 vs 114; no-show 17.4% vs 17.4%.

## 1.5 slots per FIT+ per year
| | (a) Status quo | (b) First available | **(c) Optimiser** |
|---|---|---|---|
| KPI | 23 [10–35] | 90 [76–97] | **94.5 [89–98]** |
| ≤31 d, all FIT+ | 6 | 43 [26–64] | **52 [35–74]** |
| Median days | 71 | 17 | **12** |
| Utilisation | 19% | 34% | 38% |
| Collisions per 1,000 | 0 | 0 | 0 [0–0.5] |

- **(c) vs (b), paired:** +8.9 points within 31 days for all FIT+ [4.4–15.8]; c was better in 150/150 draws.
- **Ablations, all-FIT+ ≤31 d:**
  - Greedy scores the same as ILP (52.1).
  - No overbooking: 49.4, with collapses in some draws.
  - No rebooking: 49.9, yet the KPI rises to 97. The KPI can be gamed, so pair it with the all-FIT+ rate.

**Fairness (KPI):**
- Men: 97. Al Dhafra: 88.
- Women requiring a female endoscopist: 79 [44–94], against 72 under (b).
- With 40% female lists, these women reach **91**.

## Capacity: P(KPI >90%)
| Slots per FIT+ | 1.0 | 1.2 | 1.3 | 1.4 | **1.5** | 2.5 |
|---|---|---|---|---|---|---|
| (b) | 4% | 27% | 38% | 47% | 53% | 71% |
| **(c)** | 3% | 21% | 53% | 80% | **95%** | 100% |

**To hit the target:** (c) plus about 1.5 ring-fenced slots per FIT+, which is about 29 per week per 1,000 FIT+ a year, with about 40% of them on female lists. Capacity alone under (b) does not reliably get there.

Sensitivity, chosen post hoc: aggressive overbooking (κ=0.6) gives 95.7 [91–98], with 2 [0–5.5] collisions per 1,000.

## Assumptions
- **Sourced:**
  - pools and 69.6% women: PMC8343888;
  - lead-time OR 1.006 per day: PeerJ 2022;
  - prior no-show OR 3.8;
  - nurse call OR 0.67: GIE 2016;
  - automated-referral RR 1.68, which sets attendance: JAMA IM 2026;
  - female preference 45–70%: Kamani GIE 2021;
  - Ramadan −20–40%: Brunei.
- **Assumed:**
  - regions;
  - 4 slots per list;
  - female-list share;
  - prep 3–7 days;
  - mismatch ORs;
  - hard-mismatch no-show 0.5–0.9;
  - referral latency;
  - self-rebooking;
  - cost weights.
- **Sweeps:**
  - lead-time OR 1.0/1.02/1.05 gives 94.5/94.2/94.1;
  - ε from 0.02 to 0.2 makes no difference.
- **Limits:**
  - no real Abu Dhabi capacity data;
  - refusal is untouched;
  - completion is about 55%, so the all-FIT+ rate stays well below 90%.
