"use server";

import { z } from "zod";
import { apiMode, postReview } from "@/lib/api/source";
import { roleFor, SIGNER, type SaveFailure, type Signature } from "@/components/review/decision";

const Signatures = z
  .array(z.object({ item: z.string().min(1), decision: z.enum(["same", "different", "adequate", "inadequate"]) }))
  .min(1)
  .max(2);

export type SignResult = { saved: { item: string; hash: string }[]; failed?: { signature: Signature; failure: SaveFailure } };

/** Appends each signature in order and stops at the first refusal. The role comes from the decision, never the client. */
export async function signReview(caseId: string, signatures: Signature[]): Promise<SignResult> {
  if (apiMode() !== "live") throw new Error("signReview needs WASL_API_URL: fixtures mode keeps the decision in the browser");
  const parsed = Signatures.safeParse(signatures);
  if (!parsed.success) {
    return { saved: [], failed: { signature: signatures[0], failure: { status: 400, error: "malformed signatures" } } };
  }
  const saved: SignResult["saved"] = [];
  for (const signature of parsed.data) {
    const res = await postReview(caseId, { ...signature, note: "" }, { role: roleFor(signature.decision), actor: SIGNER.name });
    if (!res.ok) return { saved, failed: { signature, failure: { status: res.status, error: res.error } } };
    saved.push({ item: signature.item, hash: res.response.review.hash });
  }
  return { saved };
}
