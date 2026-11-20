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
  "Passes on simulated cases, but too loose: a match on name alone could close a case. Not used.";

export const LTT_TIERS: LttTier[] = [
  { tau: "0.999", falseClosures: 0, wrongPatient: 0, recall: 0.8053, recallNoReview: 0.2467, reviewPer100: 83.81, certified: true,
    role: "Stricter than needed", note: "Passes, but finds fewer completed checks and sends about two fifths more cases to review than 0.99." },
  { tau: "0.995", falseClosures: 0, wrongPatient: 0, recall: 0.8157, recallNoReview: 0.3994, reviewPer100: 58.78, certified: true,
    role: "Same as the level in use", note: "Same result as 0.99 in testing." },
  { tau: "0.99", falseClosures: 0, wrongPatient: 0, recall: 0.8157, recallNoReview: 0.3994, reviewPer100: 58.78, certified: true,
    role: "The level Wasl uses",
    note: "Set just above what a match on name alone can reach. Raised from 0.98 after one case was closed for the wrong patient in final testing." },
  { tau: "0.98", falseClosures: 0, wrongPatient: 0, recall: 0.8224, recallNoReview: 0.5026, reviewPer100: 50.9, certified: true,
    role: "Too loose", note: BELOW },
  { tau: "0.95", falseClosures: 0, wrongPatient: 0, recall: 0.8235, recallNoReview: 0.5184, reviewPer100: 49.6, certified: true,
    role: "Too loose", note: BELOW },
  { tau: "0.9", falseClosures: 3, wrongPatient: 0, recall: 0.8272, recallNoReview: 0.549, reviewPer100: 47.63, certified: true,
    role: "First wrong closures",
    note: "Three wrong closures in 12,600 test cases. Still passes the safety bound, but too loose." },
  { tau: "0.8", falseClosures: 3, wrongPatient: 0, recall: 0.8274, recallNoReview: 0.5514, reviewPer100: 47.29, certified: true,
    role: "Loosest level that passes", note: "The loosest level that still passes. " + BELOW },
  { tau: "0.7", falseClosures: 8, wrongPatient: 0, recall: 0.8294, recallNoReview: 0.5648, reviewPer100: 46.42, certified: false,
    role: "Does not pass",
    note: "Eight wrong closures in 12,600 test cases, to find 1.4 points more completed checks than 0.99. Fails the safety bound." },
];
