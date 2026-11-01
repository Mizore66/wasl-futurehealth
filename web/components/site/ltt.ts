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
  { tau: "0.999", falseClosures: 0, wrongPatient: 0, recall: 0.8053, recallNoReview: 0.2467, reviewPer100: 83.81, certified: true,
    role: "Stricter than needed", note: "Certified, but recall drops and review items rise by about two fifths against 0.99." },
  { tau: "0.995", falseClosures: 0, wrongPatient: 0, recall: 0.8157, recallNoReview: 0.3994, reviewPer100: 58.78, certified: true,
    role: "Equal to the operating tier", note: "Same result as 0.99 on this calibration set." },
  { tau: "0.99", falseClosures: 0, wrongPatient: 0, recall: 0.8157, recallNoReview: 0.3994, reviewPer100: 58.78, certified: true,
    role: "The operating tier",
    note: "Set above the 0.989 cap on name-only links, a floor added after one wrong-patient closure at τ 0.98 in the final-code evaluation." },
  { tau: "0.98", falseClosures: 0, wrongPatient: 0, recall: 0.8224, recallNoReview: 0.5026, reviewPer100: 50.9, certified: true,
    role: "Below the floor", note: BELOW },
  { tau: "0.95", falseClosures: 0, wrongPatient: 0, recall: 0.8235, recallNoReview: 0.5184, reviewPer100: 49.6, certified: true,
    role: "Below the floor", note: BELOW },
  { tau: "0.9", falseClosures: 3, wrongPatient: 0, recall: 0.8272, recallNoReview: 0.549, reviewPer100: 47.63, certified: true,
    role: "First calibration false closures",
    note: "Three false closures in 12,600 loops. Still certified at α 0.001, and still below the floor." },
  { tau: "0.8", falseClosures: 3, wrongPatient: 0, recall: 0.8274, recallNoReview: 0.5514, reviewPer100: 47.29, certified: true,
    role: "Loosest certified tier", note: "The loosest tier still certified. " + BELOW },
  { tau: "0.7", falseClosures: 8, wrongPatient: 0, recall: 0.8294, recallNoReview: 0.5648, reviewPer100: 46.42, certified: false,
    role: "Not certified",
    note: "Eight false closures in 12,600 loops for 1.4 points of recall over 0.99. Fails the α 0.001 bound." },
];
