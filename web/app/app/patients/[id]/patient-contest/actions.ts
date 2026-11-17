"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { ContestResolution } from "@/components/patient-contest/model";
import { postPatientContestWithToken } from "@/lib/api/patient-contest";
import * as session from "@/lib/api/reviewer-session";
import { apiMode } from "@/lib/api/source";

/*
 * The independent contest review server action (#167). It acts only as the signed-in credential (the reviewer-session
 * token) and has no server-held credential fallback. The browser sends the case, the contest receipt, the outcome and
 * the reason; any identity field is refused, not ignored, because the API binds role and actor from the token.
 */
const FORBIDDEN = ["credential", "token", "role", "actor", "reviewer", "resolved_by"];

const Envelope = z.strictObject({
  caseId: z.string().min(1).max(200),
  resolution: z.unknown(),
});

export type ContestActionResult = { ok: true; message: string } | { ok: false; error: string; signIn?: true };

export async function resolvePatientContest(input: unknown): Promise<ContestActionResult> {
  const envelope = Envelope.safeParse(input);
  if (!envelope.success) return { ok: false, error: "Malformed contest review" };
  const raw = envelope.data.resolution;
  if (raw && typeof raw === "object" && FORBIDDEN.some((f) => f in raw)) {
    return { ok: false, error: "The page cannot choose who reviews the contest; the API binds that from your sign-in." };
  }
  const parsed = ContestResolution.safeParse(raw);
  if (!parsed.success) return { ok: false, error: parsed.error.issues[0]?.message ?? "Complete every required field" };
  if (apiMode() !== "live") return { ok: false, error: "Contest reviews need the live API; fixtures cannot append events." };
  const token = await session.reviewerToken();
  if (!token) return { ok: false, error: "Sign in with the token assigned to you. Nothing was saved.", signIn: true };

  const { caseId } = envelope.data;
  const result = await postPatientContestWithToken(caseId, parsed.data, token);
  if (!result.ok) {
    if (result.status === 401) return { ok: false, error: "The API did not accept your sign-in. Sign in again.", signIn: true };
    if (result.status === 403) return { ok: false, error: `Not saved: ${result.error}. Only a senior clinician who did not sign this case may resolve the contest.` };
    if (result.status === 409) return { ok: false, error: "This contest is already resolved. Nothing was saved; reload to see the recorded outcome." };
    return { ok: false, error: `Not saved: ${result.error}` };
  }
  const path = `/app/patients/${encodeURIComponent(caseId)}`;
  for (const p of [path, `${path}/patient-contest`, `${path}/audit`, "/app/today", "/app/patients"]) revalidatePath(p);
  return {
    ok: true,
    message:
      parsed.data.outcome === "decision_upheld"
        ? "Contest resolved as your signed-in credential: decision upheld. Earlier evidence and signatures are unchanged."
        : "Contest resolved as your signed-in credential: decision to be revised. Closure stays held, with an owned revision task, until the decision is replaced.",
  };
}
