"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { apiMode, postReferralResponse, postReferrerResponse } from "@/lib/api/source";
import { ReferralResponseRequest, ReferrerResponseRequest } from "@/lib/api/schema";

export type ReferralAnswer = z.input<typeof ReferralResponseRequest>;
export type ReferrerAnswer = z.input<typeof ReferrerResponseRequest>;
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

const ReferrerArgs = z.object({ caseId: z.string().min(1).max(200), head: z.string().min(1).max(128), packet: z.string().min(1).max(128), response: ReferrerResponseRequest });

/** Appends the referring GP's answer to a returned packet with the server-side referrer credential. */
export async function answerAsReferrer(caseId: string, head: string, packet: string, response: ReferrerAnswer): Promise<RespondResult> {
  if (apiMode() !== "live") return { ok: false, status: 503, error: "No live API: nothing was recorded" };
  const args = ReferrerArgs.safeParse({ caseId, head, packet, response });
  if (!args.success) return { ok: false, status: 400, error: "malformed referrer response" };
  const res = await postReferrerResponse(args.data.caseId, args.data.packet, args.data.response, args.data.head);
  if (!res.ok) return { ok: false, status: res.status, error: res.error, conflict: res.conflict };
  revalidatePath("/app/referrals", "layout");
  revalidatePath("/app/patients/[id]", "page");
  return { ok: true };
}
