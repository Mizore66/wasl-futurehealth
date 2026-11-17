import { z } from "zod";
import type { CaseDetail } from "@/lib/api/schema";

/** The independent review of a patient contest (pipeline/api_patient_request.go casePatientContest). */
export const ContestOutcome = z.enum(["decision_upheld", "decision_revised"]);
export type ContestOutcome = z.infer<typeof ContestOutcome>;

/** The only fields the page sends. Who resolves comes from the signed-in credential, never from the body. */
export const ContestResolution = z.strictObject({
  contest: z.string().regex(/^[0-9a-f]{64}$/, "Unknown contest receipt"),
  outcome: ContestOutcome,
  reason: z.string().trim().min(1, "Give the reason for your decision (required)").max(500, "Keep the reason under 500 characters"),
});
export type ContestResolution = z.infer<typeof ContestResolution>;

export const OUTCOME_WORDS: Record<ContestOutcome, string> = {
  decision_upheld: "Uphold the decision: the closure stands and the hold is released",
  decision_revised: "Revise the decision: closure stays held until the questioned decision is replaced",
};

export type ContestRow = NonNullable<CaseDetail["patient_requests"]>[number];

/** Patient contests on a case, open first. Explanation requests are answered at once and need no review. */
export function contestsOf(detail: CaseDetail): ContestRow[] {
  const rows = (detail.patient_requests ?? []).filter((r) => r.kind === "contest_submitted");
  return [...rows.filter((r) => r.status === "open"), ...rows.filter((r) => r.status !== "open")];
}
