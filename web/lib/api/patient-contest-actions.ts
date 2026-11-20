"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import * as session from "@/lib/api/reviewer-session";
import { apiMode, postPatientContestResolutionWithToken } from "@/lib/api/source";

/*
 * Resolution of a patient contest (#167). Like staff governance, it acts only as the signed-in identity: the
 * reviewer-session token read from its HttpOnly cookie. The browser sends the case, the contest receipt and the chosen
 * outcome with the reviewer's required reason, and nothing about who acts; any identity field is refused, not ignored. The API allows only
 * `senior_clinician` and refuses every reviewer who signed the case. That proves distinct configured actors, not
 * distinct people.
 */
const IDENTITY_FIELDS = ["credential", "token", "role", "actor"];

const Input = z.strictObject({
  caseId: z.string().min(1).max(200),
  contest: z.string().min(1).max(128),
  /* No default: the reviewer must choose. */
  outcome: z.enum(["decision_upheld", "decision_revised"]),
  /* Recorded with the resolution (staff view only); the API refuses a resolution without one. */
  reason: z.string().trim().min(1).max(500),
});

export type PatientContestActionResult = { ok: true; hash: string; message: string } | { ok: false; error: string; signIn?: true };

export async function resolvePatientContest(input: unknown): Promise<PatientContestActionResult> {
  if (input && typeof input === "object" && IDENTITY_FIELDS.some((field) => field in input)) {
    return { ok: false, error: "The page cannot choose who acts. A contest is resolved only as the signed-in credential." };
  }
  const parsed = Input.safeParse(input);
  if (!parsed.success) return { ok: false, error: "Choose whether the decision is upheld or revised, and give the reason for your decision." };
  if (apiMode() !== "live") return { ok: false, error: "Resolving a patient contest needs the live API. Fixtures cannot append events." };
  const token = await session.reviewerToken();
  if (!token) return { ok: false, error: "Sign in first. A patient contest is resolved only as the signed-in credential.", signIn: true };

  const { caseId, contest, outcome, reason } = parsed.data;
  const result = await postPatientContestResolutionWithToken(caseId, contest, outcome, reason, token);
  if (!result.ok) {
    if (result.status === 401) return { ok: false, error: "The API did not accept your sign-in. Sign in again.", signIn: true };
    return { ok: false, error: result.error };
  }
  const path = `/app/patients/${encodeURIComponent(caseId)}`;
  revalidatePath(path);
  revalidatePath(`${path}/audit`);
  revalidatePath("/app/patients");
  return {
    ok: true,
    hash: result.hash,
    message:
      outcome === "decision_upheld"
        ? "Contest resolved as upheld, as your signed-in credential. The questioned decision stands and the closure hold is released."
        : "Contest resolved as revised, as your signed-in credential. Closure stays held while the questioned decision is still current.",
  };
}
