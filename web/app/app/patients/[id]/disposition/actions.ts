"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { DispositionInput } from "@/components/disposition/model";
import { postDispositionWithToken } from "@/lib/api/disposition";
import * as session from "@/lib/api/reviewer-session";
import { apiMode } from "@/lib/api/source";

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
  if (!envelope.success) return { ok: false, error: "Malformed disposition request" };
  const raw = envelope.data.disposition;
  if (raw && typeof raw === "object" && FORBIDDEN.some((f) => f in raw)) {
    return { ok: false, error: "The page cannot choose who acts or which hold or decline is answered; the API binds those." };
  }
  const parsed = DispositionInput.safeParse(raw);
  if (!parsed.success) return { ok: false, error: parsed.error.issues[0]?.message ?? "Complete every required field" };
  if (apiMode() !== "live") return { ok: false, error: "Dispositions need the live API; fixtures cannot append events." };
  const token = await session.reviewerToken();
  if (!token) return { ok: false, error: "Sign in with the clinician token assigned to you.", signIn: true };

  const { caseId, expectedHead } = envelope.data;
  const result = await postDispositionWithToken(caseId, parsed.data, expectedHead, token);
  if (!result.ok) {
    if (result.status === 401) return { ok: false, error: "The API did not accept your sign-in. Sign in again.", signIn: true };
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
        ? "Plan saved as your signed-in credential. The hold stays open until qualifying evidence arrives; nothing was booked or completed."
        : "Informed-refusal disposition saved as your signed-in credential. No procedure was recorded and the screening record is unchanged.",
  };
}
