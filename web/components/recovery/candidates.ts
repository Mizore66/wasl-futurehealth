import type { RecoveryCandidate, RecoveryList } from "@/lib/api/recovery-schema";

/** Candidates shown per page; a slot can rank more than a thousand, which the browser cannot render usefully at once. */
export const CANDIDATE_PAGE = 25;

/** The released slots in day order (then unit and id), so the list reads chronologically. */
export function slotsByDay<T extends Pick<RecoveryList["slots"][number], "day" | "slot_id">>(slots: readonly T[]): T[] {
  return [...slots].sort((a, b) => a.day - b.day || a.slot_id.localeCompare(b.slot_id));
}

/**
 * Eligible candidates in rank order and excluded ones after them, each cut to what the coordinator asked to see.
 * `hiddenEligible` and `hiddenExcluded` say how many more each list holds.
 */
export function candidatePages(candidates: readonly RecoveryCandidate[], eligibleShown: number, excludedShown: number) {
  const eligible = candidates.filter((c) => c.eligible);
  const excluded = candidates.filter((c) => !c.eligible);
  return {
    eligible: eligible.slice(0, eligibleShown),
    excluded: excluded.slice(0, excludedShown),
    hiddenEligible: Math.max(0, eligible.length - eligibleShown),
    hiddenExcluded: Math.max(0, excluded.length - excludedShown),
    totalExcluded: excluded.length,
  };
}
