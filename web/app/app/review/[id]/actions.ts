"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { apiMode, postReview } from "@/lib/api/source";
import { roleFor, SIGNER, type SaveFailure, type Signature } from "@/components/review/decision";

const Signatures = z
  .array(z.object({ item: z.string().min(1), decision: z.enum(["same", "different", "adequate", "inadequate"]) }))
  .min(1)
  .max(2);

export type SavedEvent = { item: string; hash: string; role: string; actor: string; at: string };
export type SignResult = { saved: SavedEvent[]; failed?: { signature: Signature; failure: SaveFailure } };

/** Appends each signature in order and stops at the first refusal. The role comes from the decision, never the client. */
export async function signReview(caseId: string, signatures: Signature[]): Promise<SignResult> {
  if (apiMode() !== "live") throw new Error("signReview needs WASL_API_URL: fixtures mode keeps the decision in the browser");
  const parsed = Signatures.safeParse(signatures);
  if (!parsed.success) {
    return { saved: [], failed: { signature: signatures[0], failure: { status: 400, error: "malformed signatures" } } };
  }
  const saved: SavedEvent[] = [];
  for (const signature of parsed.data) {
    const res = await postReview(caseId, { ...signature, note: "" }, { role: roleFor(signature.decision), actor: SIGNER.name });
    if (!res.ok) {
      refreshHistory(caseId, saved.length);
      return { saved, failed: { signature, failure: { status: res.status, error: res.error } } };
    }
    const e = res.response.review;
    saved.push({ item: signature.item, hash: e.hash, role: e.role, actor: e.actor, at: e.at });
  }
  refreshHistory(caseId, saved.length);
  return { saved };
}

/** A 201 appended to the log: drop the cached case, audit and queue pages so the history is read again. */
function refreshHistory(caseId: string, appended: number) {
  if (appended === 0) return;
  revalidatePath("/app/patients/[id]", "page");
  revalidatePath("/app/patients/[id]/audit", "page");
  revalidatePath(`/app/review/${encodeURIComponent(caseId)}`);
  revalidatePath("/app/review");
}
