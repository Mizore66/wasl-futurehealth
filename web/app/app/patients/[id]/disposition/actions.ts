"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { DispositionInput } from "@/components/disposition/model";
import { postDispositionWithToken } from "@/lib/api/disposition";
import * as session from "@/lib/api/reviewer-session";
import { apiMode } from "@/lib/api/source";
import { reviewAuthMode } from "@/lib/auth/oidc-config";
import { clinicianSignInWords } from "@/components/review/auth";

/*
 * The clinician disposition server action. It acts only as the signed-in credential (the reviewer-session token). The
 * browser sends the case, the head it read and the disposition fields; any identity, hold or decline field is refused,
 * not ignored, because the API binds those from the token and the case.
 */
const FORBIDDEN = ["credential", "token", "role", "actor", "hold", "decline_event"];

const Envelope = z.strictObject({
  caseId: z.string().min(1).max(200),
  expectedHead: z.string().min(1).max(128),
  disposition: z.unknown(),
});

export type DispositionActionResult = { ok: true; message: string } | { ok: false; error: string; signIn?: true; stale?: true };

export async function recordDisposition(input: unknown): Promise<DispositionActionResult> {
  const envelope = Envelope.safeParse(input);
  if (!envelope.success) return { ok: false, error: "Request not recognised. Nothing was saved." };
  const raw = envelope.data.disposition;
  if (raw && typeof raw === "object" && FORBIDDEN.some((f) => f in raw)) {
    return { ok: false, error: "This request isn't allowed. Nothing was saved." };
  }
  const parsed = DispositionInput.safeParse(raw);
  if (!parsed.success) return { ok: false, error: parsed.error.issues[0]?.message ?? "Complete every required field" };
  if (apiMode() !== "live") return { ok: false, error: "Dispositions can't be saved in this demo." };
  const token = await session.reviewerToken();
  if (!token) return { ok: false, error: clinicianSignInWords(reviewAuthMode()), signIn: true };

  const { caseId, expectedHead } = envelope.data;
  const result = await postDispositionWithToken(caseId, parsed.data, expectedHead, token);
  if (!result.ok) {
    if (result.status === 401) return { ok: false, error: "Your sign-in has expired. Sign in again.", signIn: true };
    if (result.status === 403) return { ok: false, error: "Only a clinician or senior clinician may record a disposition. Nothing was saved." };
    if (result.status === 409 && result.conflict === "head") {
      return { ok: false, error: "The case changed since you opened it. Nothing was saved; reload and check before saving again.", stale: true };
    }
    return { ok: false, error: `Not saved: ${result.error}` };
  }
  const path = `/app/patients/${encodeURIComponent(caseId)}`;
  for (const p of [path, `${path}/disposition`, `${path}/audit`, "/app/today", "/app/patients"]) revalidatePath(p);
  return {
    ok: true,
    message:
      parsed.data.action === "clinician_disposition"
        ? "Plan saved under your name. The hold stays until the results arrive. Nothing was booked or completed."
        : "Informed refusal saved under your name. No procedure was recorded and the screening record is unchanged.",
  };
}
