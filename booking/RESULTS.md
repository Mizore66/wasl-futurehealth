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


## Stage 6: booking as simulated decision support, attached to pipeline cases

`python booking/suggest.py <run dir>` (also run by `scripts/demo.sh all`) writes `<run>/booking.json`.

**How it works:**
- It takes every CRC case that still needs a colonoscopy at the end of the integrated run (open, no-show, inadequate, referral rejected) and feeds it to the real engine (`booking/engine.py`) on a **synthetic** endoscopy calendar.
- Constraints are derived from the case: female-endoscopist preference for women, Arabic-only, region from the provider, travel willingness, prior no-show.
- The API attaches each case's suggestion: `/api/cases/{id}` → `booking`, and `/api/dashboard` → `capacity`.

**Capacity condition** is reported per run:
- `adequate`
- `misallocated`: slots exist, but in the wrong region, so **network pooling is relevant**
- `short_overall`: pooling only spreads the shortage

**Same run, three capacity scenarios** (774 open CRC cases):
```
booking (simulated): 774 open CRC cases; capacity adequate (924 slots, 1.19/case); suggested 774, within 31 d 774 (1.0), cross-region 51, no slot 0; pooling relevant: False
booking (simulated): 774 open CRC cases; capacity misallocated (924 slots, 1.19/case); suggested 774, within 31 d 769 (0.994), cross-region 155, no slot 0; pooling relevant: True
booking (simulated): 774 open CRC cases; capacity short_overall (612 slots, 0.79/case); suggested 576, within 31 d 325 (0.42), cross-region 55, no slot 198; pooling relevant: False
```

**What these show:**
- **Misallocated capacity is absorbed by cross-region suggestions:** 155 vs 51 cross-region bookings, with ≤31-day share 0.994.
- **When capacity is short overall, about 1 in 4 cases gets no slot within the horizon whatever the routing.** That shortfall is the number DoH needs for planning.
- **No optimisation claim:** the optimiser ties greedy in our tests.
