"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { apiMode, postReferralResponse } from "@/lib/api/source";
import { ReferralResponseRequest } from "@/lib/api/schema";

export type ReferralAnswer = z.input<typeof ReferralResponseRequest>;
export type RespondResult = { ok: true } | { ok: false; status: number; error: string; conflict?: string };

const Args = z.object({ caseId: z.string().min(1).max(200), head: z.string().min(1).max(128), response: ReferralResponseRequest });

/** Appends the receiving unit's answer with the server-side clinician credential. Nothing is recorded unless this returns ok. */
export async function respondToReferral(caseId: string, head: string, response: ReferralAnswer): Promise<RespondResult> {
  if (apiMode() !== "live") return { ok: false, status: 503, error: "No live API: nothing was recorded" };
  const args = Args.safeParse({ caseId, head, response });
  if (!args.success) return { ok: false, status: 400, error: "malformed referral response" };
  const res = await postReferralResponse(args.data.caseId, args.data.response, args.data.head);
  if (!res.ok) return { ok: false, status: res.status, error: res.error, conflict: res.conflict };
  revalidatePath("/app/referrals");
  revalidatePath("/app/patients/[id]", "page");
  return { ok: true };
}
