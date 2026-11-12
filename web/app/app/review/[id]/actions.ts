"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { apiMode, getCaseWithToken, postReviewWithToken } from "@/lib/api/source";
import * as session from "@/lib/api/reviewer-session";
import { type SignInResult } from "@/components/review/auth";
import { GUIDED_EVIDENCE_LABELS, isGuided, ReviewDecision, type CaseDetail, type ReviewReadInput, type ReviewState } from "@/lib/api/schema";
import type { SaveFailure, Signature } from "@/components/review/decision";

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
    ...(item.kinds.includes("evidence") && isGuided(programme) ?GUIDED_EVIDENCE_LABELS[programme] : []),
  ];
  if (!allowed.includes(signature.decision)) return { status: 400, error: `${signature.decision} does not answer this ${item.kinds.join("/")} item` };
}

/* Reviewer credential actions. The token is a server-action argument only; it is never returned, logged or stored client-side. */
export async function signInReviewer(caseId: string, token: string): Promise<SignInResult> {
  return session.signInReviewer(caseId, token);
}

export async function signOutReviewer(caseId: string): Promise<void> {
  return session.signOutReviewer(caseId);
}

export type ReadFailure = { status: 400 | 401 | 403 | 404 | 409; error: string };
/** `states` is the server's current truth for the case: the 201 response's, or a fresh read after a 409. */
export type SubmitReadResult =
  | { ok: true; hash: string; read: string; states?: ReviewState[]; committed?: undefined }
  /** The write was committed but its response was unreadable: no hash is claimed; `states` is a fresh read when available. */
  | { ok: true; committed: true; read: string; states?: ReviewState[]; hash?: undefined }
  | { ok: false; failure: ReadFailure; states?: ReviewState[] };

const ReadInput = z.strictObject({
  item: z.string().min(1),
  question: z.enum(["link", "evidence"]),
  decision: ReviewDecision.refine((decision) => decision !== "confirm_contact", "confirm_contact is not a review-desk signature"),
  expectedHead: z.string(),
});

function refresh(caseId: string) {
  revalidatePath("/app/patients/[id]", "page");
  revalidatePath("/app/patients/[id]/audit", "page");
  revalidatePath(`/app/review/${encodeURIComponent(caseId)}`);
  revalidatePath("/app/review");
}

/**
 * Appends exactly one independent-review read with the signed-in reviewer's own token. The slot comes from the
 * server's `next_read`, the labels from the raised item, the authority from the API; the browser asserts none of them.
 */
export async function submitReviewRead(caseId: string, input: ReviewReadInput): Promise<SubmitReadResult> {
  if (apiMode() !== "live") throw new Error("submitReviewRead needs WASL_API_URL: fixtures mode keeps the decision in the browser");
  const parsed = ReadInput.safeParse(input);
  if (!parsed.success) return { ok: false, failure: { status: 400, error: "malformed read" } };
  const token = await session.reviewerToken();
  if (!token) return { ok: false, failure: { status: 401, error: "sign in to review" } };

  const latest = await getCaseWithToken(caseId, token);
  if (!latest.ok) return { ok: false, failure: { status: latest.status, error: latest.status === 404 ? "case not found" : "sign in to review" } };
  const detail = latest.detail;
  const { item, question, decision, expectedHead } = parsed.data;
  const bad = refusal(detail, { item, decision });
  if (bad) return { ok: false, failure: { status: bad.status as 400 | 404, error: bad.error } };
  const raised = detail.review_items.find((r) => r.item === item)!;
  if (!raised.kinds.includes(question)) return { ok: false, failure: { status: 400, error: `item ${item} does not raise a ${question} question` } };
  const answers = decision === "same" || decision === "different" ? "link" : "evidence";
  if (answers !== question) return { ok: false, failure: { status: 400, error: `${decision} does not answer the ${question} question` } };

  const states = detail.api_review_states ?? [];
  const state = states.find((s) => s.item === item && s.question === question);
  if (!state) return { ok: false, failure: { status: 404, error: "no independent review is open for this question" }, states };
  if (!state.may_submit || !state.next_read) return { ok: false, failure: { status: 403, error: "you may not submit the next read for this question" }, states };
  if (state.head !== expectedHead) return { ok: false, failure: { status: 409, error: "the review changed; check the current state" }, states };

  const res = await postReviewWithToken(
    caseId,
    { item, decision, note: "", read: state.next_read, expected_head: expectedHead },
    token,
  );
  if (res.ok && res.response === null) {
    /* The API committed (201) but the body was unreadable: refresh, re-read, claim no hash and never retry. */
    refresh(caseId);
    const again = await getCaseWithToken(caseId, token).catch(() => undefined);
    return { ok: true, committed: true, read: state.next_read, states: again?.ok ? again.detail.api_review_states : undefined };
  }
  if (res.ok) {
    refresh(caseId);
    const saved = res.response!;
    return { ok: true, hash: saved.review.hash, read: state.next_read, states: saved.api_review_states };
  }
  let fresh: ReviewState[] | undefined;
  if (res.status === 409) {
    refresh(caseId);
    const again = await getCaseWithToken(caseId, token).catch(() => undefined);
    fresh = again?.ok ? again.detail.api_review_states : undefined;
  }
  return { ok: false, failure: { status: res.status, error: res.error }, states: fresh };
}
