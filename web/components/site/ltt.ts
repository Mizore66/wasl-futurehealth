/**
 * Learn-then-Test tiers, from `pipeline/results/ltt/LTT.md` (12,600 calibration loops, v2 noise).
 * `results.json` has only the loop count, so the rows live here and `tests/site.test.ts` checks
 * each against the markdown tables.
 */
export type LttTier = {
  tau: string;
  falseClosures: number;
  wrongPatient: number;
  recall: number;
  recallNoReview: number;
  reviewPer100: number;
  certified: boolean;
  role: string;
  note: string;
};

export const LTT_SOURCE = "pipeline/results/ltt/LTT.md";
export const OPERATING_TAU = "0.99";

const BELOW =
  "Certified on synthetic loops, but below the floor: a name-only link at the 0.989 cap could close a loop. Not used.";

export const LTT_TIERS: LttTier[] = [
  { tau: "0.999", falseClosures: 0, wrongPatient: 0, recall: 0.8683, recallNoReview: 0.1801, reviewPer100: 108.58, certified: true,
    role: "Stricter than needed", note: "Certified, but recall drops and review items rise by about a third against 0.99." },
  { tau: "0.995", falseClosures: 0, wrongPatient: 0, recall: 0.8831, recallNoReview: 0.2946, reviewPer100: 82.44, certified: true,
    role: "Equal to the operating tier", note: "Same result as 0.99 on this calibration set." },
  { tau: "0.99", falseClosures: 0, wrongPatient: 0, recall: 0.8831, recallNoReview: 0.2946, reviewPer100: 82.44, certified: true,
    role: "The operating tier",
    note: "Set above the 0.989 cap on name-only links, a floor added after one wrong-patient closure at τ 0.98 in the final-code evaluation." },
  { tau: "0.98", falseClosures: 0, wrongPatient: 0, recall: 0.8919, recallNoReview: 0.3717, reviewPer100: 77.29, certified: true,
    role: "Below the floor", note: BELOW },
  { tau: "0.95", falseClosures: 0, wrongPatient: 0, recall: 0.8941, recallNoReview: 0.3847, reviewPer100: 76.17, certified: true,
    role: "Below the floor", note: BELOW },
  { tau: "0.9", falseClosures: 1, wrongPatient: 0, recall: 0.8961, recallNoReview: 0.4017, reviewPer100: 74.33, certified: true,
    role: "First calibration false closure",
    note: "One false closure in 12,600 loops. Still certified at α 0.001, and still below the floor." },
  { tau: "0.8", falseClosures: 1, wrongPatient: 0, recall: 0.8963, recallNoReview: 0.4039, reviewPer100: 74.1, certified: true,
    role: "Below the floor", note: BELOW },
  { tau: "0.7", falseClosures: 3, wrongPatient: 0, recall: 0.8985, recallNoReview: 0.4122, reviewPer100: 73.35, certified: true,
    role: "Three false closures",
    note: "Three false closures in 12,600 loops for 1.5 points of recall over 0.99. Still certified at α 0.001, and below the floor." },
];
