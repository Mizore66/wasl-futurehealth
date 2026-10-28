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

## Queue levers (simulated): direct-to-procedure and prep-failure targeting

**Status:** built as decision support. The effects below are **simulated**: effect sizes are borrowed from cited trials, and every App 9 field is **synthetic**.
- **Code:**
  - `booking/booking/levers.py`: per-case pathway and prep risk;
  - `booking/sim/levers_sim.py`: the simulated arms;
  - `booking/tests/test_levers.py`: tests.
- **Rerun:** `cd booking/sim && python levers_sim.py 30`, about 3.5 minutes. It writes `levers_summary.json`.

### What the levers do

**Direct-to-procedure** (DoH CRC spec 5.5.1 + App 9: no GI consult is required, only a pre-colonoscopy assessment):
- A nurse completes the App 9 checklist call.
- Patients with **ASA ≥3, anticoagulation/antiplatelet, or an implanted cardiac device** keep the GI clinic pathway. Everyone else is booked straight into a colonoscopy slot.
- Insulin-treated diabetes stays on the direct pathway, with a dose-adjustment note in the checklist.

**Prep-failure risk:**
- **The score:** Dik et al., GIE 2015;81(3):665–672 (PMID 25600879). Published points:

  | Factor | Points |
  |---|---|
  | ASA ≥3 | 2 |
  | Tricyclic antidepressants | 3 |
  | Chronic constipation | 2 |
  | Opioids | 1 |
  | Diabetes | 1 |
  | Prior abdominal/pelvic surgery | 1 |
  | Current hospitalisation | 1 |
  | History of inadequate prep | 1 |

  The score runs from 0 to 12.
- **Cut-off ≥2:** sensitivity 66%, specificity 79%, PPV 29%, NPV 95%.
- **Bands and interventions:**
  - Low band: an SMS prep sequence.
  - High band: a nurse prep call plus a split/extended regimen. The regimen change is **suggested only** (`requires_clinician_order: true`).

**In the pipeline:** both levers are attached to every open case in `booking.json` (`pathway`, `prep_risk`, `app9`). The API serves them at `/api/cases/{id}` → `booking`, and the summary counts at `/api/dashboard` → `capacity.levers`.

On the stage 6 run (774 open CRC cases):

| Pathway | Cases | | Prep risk | Cases |
|---|---|---|---|---|
| Direct | 648 | | Low | 630 |
| Clinic | 126 | | High | 144 |

### How the simulation is calibrated

- **Consult leak** (SEHA, PMC8343888 T5): 360 of 1,429 FIT+ patients were "referred to a GI but did not attend". That is 36.3% of non-completers and 57.2% of the convertible pool.
  - Under the consult pathway, a convertible non-attender is lost at the consult with p = 0.572 and is never booked.
- **Direct access** brings back only part of that leak:
  - the checklist call is completed with probability U(0.4, 0.7) [A];
  - patients who complete it then attend with U(0.3, 0.6) [A].
  - Patients who don't complete the call go to a navigator and use no slot.
- **The `direct_ungated` sensitivity arm** books everyone without the call gate. This is the unnavigated direct access that was *slower* in a US system (JGIM 2026, HR 0.59).
- **Consult delay:** median 9 days. This is the Banerjea 2017 gap: first test at 22 days vs 13 days straight-to-test. The nurse call takes a median of 2 days [A].
- **Prep failure:**
  - The population inadequate-prep rate is U(0.12, 0.22) [A]. The DoH App 9.5 cap is 10%.
  - Per-patient risk is logistic in the Dik score. The slope, ln 1.6 per point, is an assumption [A].
  - An inadequate exam uses a slot and needs a repeat (at most 2). A loop counts as done only on an adequate exam.
- **Prep intervention effects:**
  - SMS: relative risk reduction (RRR) U(0.12, 0.38), centred on the pooled 7-RCT RRR of 0.25 (IJCD 2022).
  - High band: RRR U(0.3, 0.7), applied only if the clinician orders it, which happens with p = U(0.6, 0.9) [A]. The range is shrunk from 94% vs 49% adequate in one trial (DLD 2023).
- **No double counting:** the SMS no-show effect is not added, because every arm already assumes reminders.

**Statistics:** 30 parameter draws, about 2,100 FIT+ per draw, measured on year 2. Every arm uses the same engine, cohort, calendar and random numbers, so differences are paired.
- Values are means, with 2.5–97.5 percentile intervals over draws.
- "≤31 d" is the share of **all FIT+** with an adequate colonoscopy within 31 days.
- "Completion" is an adequate colonoscopy within a year.

### ≤31-day completion (all FIT+, %), adequate capacity allocation

| Slots per FIT+ | Base (consult) | + Direct | + Prep | + Both | Direct, ungated |
|---|---|---|---|---|---|
| 0.8 | 0.3 | 0.2 | 0.4 | 0.2 | 0.3 |
| 1.0 | 4.0 | 3.5 | 7.4 | 7.2 | 1.3 |
| 1.2 | 31.1 | 35.2 | 38.9 | **44.5** | 16.3 |
| 1.5 | 44.9 | 51.9 | 46.8 | **53.7** | 49.6 |

### Paired change vs base (percentage points, [95% interval]; share of draws where the arm was better)

| Slots | Arm | Δ ≤31 d | Δ completion | Repeat colonoscopies avoided per 1,000 adequate exams |
|---|---|---|---|---|
| 1.0 | Direct | −0.5 [−7.2, 7.8] (43%) | +2.4 [0.2, 4.8] | 2 |
| 1.0 | Prep | +3.4 [−0.3, 13.8] (90%) | +0.7 [0.0, 1.9] | **88 [52, 132]** |
| 1.0 | Both | +3.2 [−4.8, 27.1] (73%) | +3.2 [1.2, 5.2] | 88 |
| 1.2 | Direct | +4.1 [−18.3, 14.1] (80%) | +2.6 [0.2, 4.9] | 2 |
| 1.2 | Prep | +7.8 [1.0, 29.4] (100%) | +0.4 [0.0, 1.0] | 88 |
| 1.2 | **Both** | **+13.4 [−2.5, 37.5] (93%)** | **+3.1 [1.1, 5.4]** | 88 |
| 1.5 | Direct | +7.0 [4.4, 9.8] (100%) | +2.7 [0.3, 4.9] | 2 |
| 1.5 | Prep | +1.9 [0.6, 4.5] (100%) | +0.5 [0.1, 1.1] | 88 |
| 1.5 | **Both** | **+8.7 [6.3, 12.5] (100%)** | **+3.2 [1.0, 5.5]** | 88 |
| 1.2 | Direct, ungated | −14.8 [−34.5, 10.4] (13%) | +2.4 [0.1, 4.8] | 2 |

With capacity misallocated across regions, the pattern is the same. At 1.2 slots, Both gives +11.7 [1.6, 25.2] (97%). At 1.5, it gives +9.4 [6.5, 13.5] (100%). Full grid: `booking/sim/levers_summary.json`.

### What this shows

- **Prep targeting is the only lever that frees capacity.** The inadequate-prep rate falls from 18.8% to 12.7%. That avoids about **88 repeat colonoscopies per 1,000 adequate exams**, consistent with the 62–197 range in the queue-levers research.
  - The freed slots matter most when capacity is tight but not collapsed: +7.8 pp at 1.2 slots, better in 30/30 draws.
  - The rate is still above the DoH 10% cap. Targeting alone doesn't meet App 9.5.
- **Direct-to-procedure raises total completion by about 2.5 pp at every capacity level.** It recovers patients who would have been lost at the GI consult: 131 → 21 per 1,000.
  - It improves the 31-day share only when there is spare capacity (1.5 slots: +7.0 pp, 30/30 draws).
  - At 1.0–1.2 slots, the extra patients compete for the same slots. The interval crosses zero, and at 1.0 it loses in most draws.
- **Navigation is required.** Booking everyone directly without the checklist call gate (`direct_ungated`) **lowers** 31-day completion at 1.2 slots by 14.8 pp, because booked non-attenders waste slots. This matches the US finding that unnavigated direct access was slower.
- **Below 1.0 slot per FIT+, no lever helps the 31-day share** (≤0.4 pp at 0.8 slots). Total completion still rises by 2–4 pp. This confirms the earlier finding: only more capacity moves that regime.
- **Both levers together** are the best arm from 1.2 slots up. They are not additive below that.

### Limitations

- **Synthetic inputs.** Every App 9 and Dik field is synthetic, drawn with assumed prevalences [A]; no Abu Dhabi data was used. Note that hospitalisation is 0 for outpatient FIT+ referrals.
- **Score population mismatch.** The Dik score was derived on 1,996 split-dose colonoscopies at 4 Dutch centres, with inadequate defined as total BBPS <6. That is not a FIT+ cohort and not a UAE population, and AUC falls to 0.62–0.74 externally. Local recalibration is required.
- **Assumed parameters.** The per-point slope linking the Dik score to risk is assumed. So are the checklist-call completion rate and post-call attendance for patients who would have been lost at consult. These parameters drive the direct-to-procedure result.
- **Borrowed effect sizes.** Intervention effects come from single trials or meta-analyses in other health systems and are shrunk toward the null. The stepped-wedge pilot would measure them locally.
- **No clinical decisions.** The engine suggests a pathway and a prep plan. A nurse confirms the checklist, and a clinician orders any regimen change.

## Female-list access gap: protection for women who require a female endoscopist

**The gap.** Under (c) at 1.5 slots, the ≤31-day KPI for women who require a female endoscopist was 79%, vs 97% for everyone else. The acceptance limit is 5 points.
- **Why:** these women (about 12% of FIT+) can only use female lists, which are 15–30% of lists [A]. Other patients were filling female lists weeks ahead.
- **The remaining structural limit:** female lists are infrequent. On one draw, Abu Dhabi had 0.8 a week and Al Dhafra 0.13. So a rebook after a no-show can still miss 31 days.

**The engine change** (`BookingEngine(protect=4.0)`, off by default; on in `suggest.py`):
- **Protection:** each female list holds back ⌈frac × cap⌉ free slots for women who require one.
  - frac = 4 × (recent arrivals of such women in the region per day) ÷ (female-list slots per day in the region).
  - Overbook copies of a holding list are held too.
- **Release:** held slots are released 7 days before the list, so unused ones still get filled.
- **Why demand-sized:** it holds almost nothing when female lists are plentiful.
- **Rejected alternative:** a fixed 7-day window (`c_res7`, kept as an ablation) also closes the gap but over-reserves. At a 40% share, everyone else loses up to 10 points.
- **Test:** `tests/test_engine.py::test_female_protection`. The assignment still equals the HiGHS ILP with protection on (30/30 batches).

**How it was chosen:** factor and window were picked on design draws 150–189. The results below are on draws 0–149 (`sim/fem_summarise.py` → `sim/fem_summary.json`).

Gap is the KPI for everyone else minus the KPI for women who require a female list, paired per draw; 150 draws, medians [95% interval].

| Female-list share | No protection | **With protection** | Draws with gap ≤5 pts | All-FIT+ ≤31 d, paired change |
|---|---|---|---|---|
| 15–30% [A], as today | 18.5 [4.0, 52.2] | 12.6 [−0.2, 42.5] | 18% (was 5%) | +0.1 [−1.6, 1.0] |
| 30% | 10.6 [0.5, 22.6] | **4.5 [−2.9, 14.7]** | 55% (was 16%) | −0.3 [−2.1, 0.5] |
| 40% | 5.6 [−1.8, 13.6] | **0.6 [−6.0, 8.7]** | 86% (was 47%) | −0.6 [−3.0, 0.2] |

KPI for women who require a female list, with protection: 84% at 15–30%, 91% at 30%, 94% at 40%.

**What this shows:**
- **Protection removes the competition part of the gap.** It cuts about 6 points at every share, at essentially no cost to anyone else.
- **Getting under 5 points also needs enough female lists:** about 30% gets the median there, and 40% makes it reliable.
  - Wasl can't create female endoscopists. It reports the shortfall.
  - This matches the earlier planning figure of about 40% of ring-fenced slots on female lists.

Rerun:
```
cd booking/sim && python sim.py 150 fem '{"c_main": 1.5, "policies": ["c", "c_res7", "c_fem"], "variants": [{"name": "f_share=0.3", "override": {"f_share": 0.3}, "runs": [["c", 1.5], ["c_res7", 1.5], ["c_fem", 1.5]]}, {"name": "f_share=0.4", "override": {"f_share": 0.4}, "runs": [["c", 1.5], ["c_res7", 1.5], ["c_fem", 1.5]]}]}' && python fem_summarise.py
```
About 2.5 minutes.

**In the pipeline:** since `a66b6a2`, 69.6% of the pipeline's CRC loops are women. On the five held-out runs, `booking.json` had 133–170 open cases per run that require a female list. All got a slot within 31 days, vs 97–100% for everyone else. That is a 95-day snapshot at 1.2 slots per case, easier than the full-year simulation above.

## Cervical colposcopy and the repeat after an unsatisfactory exam

`suggest.py` now also books cervical loops that still need a colposcopy. It uses the same engine on a synthetic colposcopy-clinic calendar, at the CRC calendar's slot density per day.
- **First colposcopy:** must fall inside the loop's DoH App 5 window (HSIL+ 14 days, LSIL 28, ASC-US 56).
- **Repeat after an unsatisfactory exam:** 6–12 weeks later (DoH cervical standard 10.6.4).
  - It is booked when the unsatisfactory result arrives. `Event.earliest` sets the earliest allowed day, so the slot is held weeks ahead rather than requested at week 6.
  - Requesting at week 6 left 8 of 25 repeats late on one run, because first colposcopies had already taken the slots.
- **Results on the five held-out pipeline runs:** 127/127 repeats inside the window. First colposcopies inside their window 83–88% of the time; the misses are mostly 14-day urgent windows.
- **Assumed [A]:** half of colposcopy clinics have a female clinician. Female-list protection applies here as well.
- **Test:** `tests/test_engine.py::test_earliest`.
