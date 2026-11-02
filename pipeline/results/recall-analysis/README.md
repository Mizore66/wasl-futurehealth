# CRC recall-loss analysis

Read-only, offline analysis of the fresh B8 seeds **17 and 29**. No pipeline run, model training, threshold change, calibrator refit or generator change was performed. The classifier and outputs are post-hoc descriptive evidence, not a new evaluation or a tuning result.

The population is a CRC loop whose simulated truth state is closed but whose final Wasl decision is not closed. Final state is the decision with the highest `oseq`. Each miss receives the first matching category in an ordered decision list. A category is the first observed barrier, **not necessarily the sole cause**.

## Denominators

| Lane | Seed 17 missed / truth-closed | Seed 29 missed / truth-closed | Pooled recall |
|---|---:|---:|---:|
| Wrong EID, guard on | 204 / 777 | 190 / 750 | 0.7420 |
| Wrong EID, guard off | 196 / 777 | 181 / 750 | 0.7535 |
| Clean, guard on | 183 / 777 | 168 / 750 | 0.7700 |
| Clean, guard off | 181 / 777 | 168 / 750 | 0.7715 |

The wrong-EID guard-on lane therefore has **394 misses among 1,527 truth-closed loops (25.8%)**, with zero false closures. These counts match the B8 `report7` results.

These seeds reuse the same 8,871 held-out people and 43 MTSamples report texts as the frozen evaluation. A new seed is a new draw, not new people or new report phrasing.

## Wrong-EID guard-on causes

Categories are mutually exclusive by first-match precedence.

| Category | Misses | % of 394 misses | % of 1,527 truth-closed |
|---|---:|---:|---:|
| L5: no link established and no review item raised | 146 | 37.1% | 9.56% |
| E2: simulated reviewer signed evidence incorrectly | 91 | 23.1% | 5.96% |
| E1: ungraded prep; decision falls beyond day 120 | 45 | 11.4% | 2.95% |
| L3: registration Emirates IDs conflict; all injected relative-EID cases | 40 | 10.2% | 2.62% |
| E4: readers agree caecum is not documented, key says completed | 40 | 10.2% | 2.62% |
| L2: simulated reviewer incorrectly answered “different” | 14 | 3.6% | 0.92% |
| L4b: correct second-identifier “same”, loop still open | 7 | 1.8% | 0.46% |
| E6: repeat-procedure closing report itself unresolved | 4 | 1.0% | 0.26% |
| L0: final state `referral_rejected` | 4 | 1.0% | 0.26% |
| L4: EID-guard contradiction item present | 2 | 0.5% | 0.13% |
| L6: repeat report machine-adequate but never attached | 1 | 0.3% | 0.07% |
| Unclassified | 0 | 0% | 0% |

Grouped interpretation:

- **Identity/link barriers:** 195 misses (49.5%): L5, L3, L4 and L4b.
- **Simulated reviewer errors:** 105 (26.6%): E2 and L2. The 4% general reviewer rate and 25% day-15 rate are stress assumptions, not measured clinician rates.
- **Timing/horizon with ungraded prep:** 45 (11.4%): E1.
- **Model, rule or other:** 49 (12.4%): E4, E6, L0 and L6.

Another useful grouping is:

- **Needs more source information or a policy decision:** 125 (31.7%): E1, E4 and L3.
- **Simulator-assumption artifacts:** 105 (26.6%): E2 and L2.
- **Potentially improvable, hypothesis-level only:** 164 (41.6%): L5, L4, L4b, L0, E6 and L6. L5 is 146 of these; the remaining 18 are too few for a general claim.

Pending histology explains **zero** wrong-on misses: every truth-closed loop's pathology arrives within the 120-day horizon. Unresolved or unanswered review also explains zero in the offline reviewer replay. This does not mean those mechanisms can never cause misses in other cohorts.

The first classifier left five wrong-on loops unclassified. Inspecting them identified the repeat-report distinction encoded as E6/L6. That refinement is post-hoc on these same cohorts. Wrong-off retains one unclassified loop.

## What explains the gap

### 1. Most loss is already present without wrong EIDs

Of the 394 wrong-on misses, **351 are also missed in clean-on**. The L5 count is 146 in wrong-on, wrong-off and clean-on, and 145 in clean-off-final. Its prevalence is therefore not explained by enabling the guard or injecting wrong EIDs. The loop sets are not perfectly identical across lanes.

L5 means no trusted link and no review item, not that 146 more items should automatically be sent to review. The logs cannot establish whether raising those items would be worth the reviewer time.

### 2. Headline recall overstates the guard's right-patient cost

Wrong-off closes 13 truth-closed loops using another person's colonoscopy report. Ordinary state-based recall counts those as successes even though they are wrong-patient closures.

| Two-seed pooled accounting | Guard off | Guard on |
|---|---:|---:|
| Truth-closed loops closed by Wasl | 1,150 | 1,133 |
| Closed using another person's report | 13 | 0 |
| Right-patient closures | 1,137 | 1,133 |
| Right-patient recall | 0.7446 | 0.7420 |

The headline gap is 17 loops (1.15 percentage points), but the **net right-patient gap is four loops (0.26 points)**. The paired transitions include ten right-patient off-only closures and six on-only closures. This accounting does not excuse the guard-off false closures.

In clean cohorts, guard-on has two more misses than guard-off (351 versus 349). Two seeds cannot establish that small difference.

### 3. L5 is associated with DOB distortion

Among 146 L5 misses, 66 have a DOB year shifted by up to ten years and 39 have an Emirates ID differing by one or two digits. These are descriptors, not proven causes.

| Referral-versus-procedure DOB relation | Truth-closed | L5 misses | L5 share |
|---|---:|---:|---:|
| Exact | 828 | 18 | 2.2% |
| Same year, different day/month | 216 | 18 | 8.3% |
| Missing on a side | 149 | 16 | 10.7% |
| Year shifted by ≤10 years | 325 | 88 | 27.1% |
| Year shifted by >10 years | 9 | 6 | 66.7% |

The visible association points to identity-noise handling, especially DOB shifts, as the first development target. The tiny far-shift cell must not be treated as a stable rate.

### 4. Ungraded prep is the structural evidence limit

Among wrong-on truth-closed loops, **802/1,527 (52.5%)** have a closing report with caecum documented but prep ungraded. Recall is 0.700 on that subset versus 0.861 when both readers agree on adequate. Ungraded-prep reports appear in 241/394 misses (61%), including loops whose first category is linking.

For truth closure on days 105–120, recall is 0.591, versus 0.750–0.774 in the earlier buckets. A longer horizon would mostly address E1, but that is a policy/evaluation design choice, not evidence that the current horizon is wrong.

Of 404 answered day-15 decisions on wrong-on truth-closed loops, 91 were wrong under the simulator's assumed error model; 86 of those remained open. Training cannot manufacture a missing prep grade. Better source capture, query timing and explicit horizon policy are the relevant levers.

### 5. One report phrasing drives E4

All 40 E4 misses use text **u18**, including the phrase “to the ileocecal valve”. That text appears in 49 truth-closed loops, none of which close under the strict policy. This is one phrasing class, not a general 2.6% reader-error rate. Corpora 2–8 have already influenced reader development, so they cannot be relabelled as untouched tests.

### 6. Guard over-hold hypotheses are small

L4 and L4b contain nine misses. Seven closed on the right report in wrong-off but remain open in wrong-on despite correct “same” answers. This is a concrete investigation hypothesis covering about 0.5% of truth-closed loops, but the logs do not prove the mechanism. No guard change is made here.

Wrong-on has EID-guard items on 56/1,527 truth-closed loops; 45 close. Clean-on has such items on 23 loops; 18 close.

## Subgroup splits

Wrong-on pooled recall:

| Split | Recall (truth-closed n) |
|---|---:|
| Arab origin | 0.742 (818) |
| Filipino origin | 0.733 (131) |
| South Asian origin | 0.749 (375) |
| Western origin | 0.734 (203) |
| Female | 0.745 (1,070) |
| Male | 0.736 (450) |
| Unknown sex | 0.714 (7) |
| Age <40 | 0.765 (421) |
| Age 40–59 | 0.727 (406) |
| Age 60–79 | 0.727 (381) |
| Age 80+ | 0.749 (319) |

Wilson intervals overlap across these descriptive splits, but overlapping intervals are not a test of equality and two seeds cannot establish absence of bias. Ages derive from Wikidata people and extend beyond screening age; unknown sex has only seven loops. No subgroup tuning follows from this table.

## Generalized development-data proposal

This is a **split design only**. Nothing here has been trained or implemented.

A new seed alone is insufficient: it reuses the same people, noise code and report corpus. Use new draws from disjoint non-evaluation people and independently varied evidence.

1. **Keep the locked tests locked.** Frozen seeds 11/23/37/41/59 and B8 seeds 17/29 remain excluded from training, threshold selection and rule design. Also treat consumed seeds 95, 101–107, 2026 and corpus seeds 2027/3031/4049 as development exposures, not fresh tests.
2. **Build a development pool from new seeds**, for example 1001–1040, using `dev` or `train` people only. Vary CRC size (800–3,000), horizon (90/120/180 days), DOB missingness and shifts, EID capture and typo rates, wrong-EID share (0/0.5/1/2/4%), report phrasing and ungraded-prep prevalence. Use v1, v2 and at least one newly written noise model. Keep ordinary and day-15 reviewer-error sensitivity settings explicitly labelled as simulator assumptions.
3. **Pre-register untouched validation.** Reserve 5–10 seeds, for example 2001–2010, with people disjoint from development, independently authored report text and a new noise model. Freeze the manifest before the first run. Do not perform error analysis on it and then reuse it as a test.
4. **Make people disjoint.** A possible split is 70/30 of the existing `dev` people for development versus validation; never use `eval` people for development. Keep `cal` exclusively for Learn-then-Test calibration.
5. **Measure the right outcome.** Report right-patient recall at zero false closures, with paired guard on/off and clean/wrong-EID lanes. Pool enough seeds for small categories: about 750 truth-closed CRC loops per seed means L4/L4b-scale effects need dozens of seeds, not another two-seed comparison.

New report text should vary caecum phrasing and ungraded prep (for example 20–60%) without allowing one text to dominate loops. Corpora 2–8 are development data now; newly authored validation text must remain untouched until the decision-point run.

## Files and reproduction

- `classify_crc_misses.py`: deterministic first-match classifier and table generator.
- `summary.json`: denominators, categories, splits, paired lanes, guard-gap accounting and descriptors.
- `missed_loops.csv`: one row per missed loop per lane, with category and supporting facts.

The script requires the original raw B8 work directories; those large logs are not duplicated in this repository. It does not modify its inputs.

```bash
python3 -I pipeline/results/recall-analysis/classify_crc_misses.py \
  <b8-eval-dir> data/persons3.json <output-dir>
```

A coordinator rerun reproduced `summary.json`, `missed_loops.csv` and console output byte-identically. No evaluation seeds were used for tuning.
