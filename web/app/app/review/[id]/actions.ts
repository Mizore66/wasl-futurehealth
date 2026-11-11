"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { apiMode, getCase, postReview } from "@/lib/api/source";
import { GUIDED_EVIDENCE_LABELS, ReviewDecision, type CaseDetail } from "@/lib/api/schema";
import { roleFor, SIGNER, type SaveFailure, type Signature } from "@/components/review/decision";

const Signatures = z
  .array(z.object({ item: z.string().min(1), decision: ReviewDecision.refine((decision) => decision !== "confirm_contact", "confirm_contact is not a review-desk signature") }))
  .min(1)
  .max(2);

export type SavedEvent = { item: string; hash: string; role: string; actor: string; at: string };
export type SignResult = { saved: SavedEvent[]; failed?: { signature: Signature; failure: SaveFailure } };

/**
 * The API accepts any programme label on any raised item, so each answer is checked against the item it signs:
 * a link item takes same/different, an evidence item only its programme's evidence labels.
 */
function refusal(detail: CaseDetail | null, signature: Signature): SaveFailure | undefined {
  if (!detail) return { status: 404, error: "case not found" };
  const item = detail.review_items.find((r) => r.item === signature.item);
  if (!item) return { status: 404, error: "review item not raised for this case" };
  const programme = detail.case.programme;
  const allowed = [
    ...(item.kinds.includes("link") ? ["same", "different"] : []),
    ...(item.kinds.includes("evidence") && programme !== "cervical" ? GUIDED_EVIDENCE_LABELS[programme] : []),
  ];
  if (!allowed.includes(signature.decision)) return { status: 400, error: `${signature.decision} does not answer this ${item.kinds.join("/")} item` };
}

/** Appends each signature in order and stops at the first refusal. The role comes from the decision, never the client. */
export async function signReview(caseId: string, signatures: Signature[]): Promise<SignResult> {
  if (apiMode() !== "live") throw new Error("signReview needs WASL_API_URL: fixtures mode keeps the decision in the browser");
  const parsed = Signatures.safeParse(signatures);
  if (!parsed.success) {
    return { saved: [], failed: { signature: signatures[0], failure: { status: 400, error: "malformed signatures" } } };
  }
  const detail = await getCase(caseId);
  for (const signature of parsed.data) {
    const failure = refusal(detail, signature);
    if (failure) return { saved: [], failed: { signature, failure } };
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
