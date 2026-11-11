"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { getDoubleReview, postDoubleRead, questionRole, deskConfigured, deskCredentialName } from "@/lib/api/double-review";
import { apiMode } from "@/lib/api/source";
import { slotFor } from "@/components/review/double-review/model";

/*
 * One double-review read (docs/API-DOUBLE-REVIEW.md). The browser names the item, whether it answers an initial or the
 * senior question, and the answer. It names no seat and no credential: the server always sends this deployment's one
 * credential for the question's role (WASL_API_<ROLE>_TOKEN), and the slot (r1, r2 or r3) is the one the API itself
 * offers that credential now (`viewer_may`). The API records the person bound to the credential, whatever this page
 * believes, and refuses a second initial read by that person.
 */

const Input = z.object({
  caseId: z.string().min(1),
  item: z.string().min(1),
  question: z.enum(["link", "evidence"]),
  senior: z.boolean(),
  decision: z.string().min(1).max(80),
  expectedHead: z.string().min(1).max(128),
});

export type DoubleReadInput = z.input<typeof Input>;

export type DoubleReadOutcome =
  | { ok: true; recorded: { read: string; actor: string; role: string; hash: string; at: string }; phase: string }
  | { ok: false; status: number; error: string; reason?: string; conflict?: string };

export async function submitDoubleRead(input: DoubleReadInput): Promise<DoubleReadOutcome> {
  if (apiMode() !== "live") throw new Error("submitDoubleRead needs WASL_API_URL: fixtures mode cannot append reads");
  const parsed = Input.safeParse(input);
  if (!parsed.success) return { ok: false, status: 400, error: "malformed read" };
  const { caseId, item, question, senior, decision, expectedHead } = parsed.data;
  // The role follows the question. A client that names the wrong question only gets a credential the API refuses.
  const role = questionRole(question);
  if (!deskConfigured(role)) {
    return { ok: false, status: 0, error: `no credential for this question (${deskCredentialName(role)} is not set on the server)` };
  }
  const state = await getDoubleReview(caseId, role);
  const round = state?.rounds.find((r) => r.item === item);
  if (!state || !round) return { ok: false, status: 404, error: "this case has no such review item" };
  if (round.head !== expectedHead) {
    refresh(caseId);
    return { ok: false, status: 409, error: "stale expected_head: the round changed since it was read", conflict: "head" };
  }
  const read = slotFor(round, senior);
  if (!read) {
    return { ok: false, status: 0, error: `the API offers this deployment (${state.viewer.actor}) no ${senior ? "senior" : "initial"} read of this item now` };
  }
  const res = await postDoubleRead(caseId, role, { item, read, decision, note: "", expected_head: expectedHead });
  if (!res.ok) {
    if (res.conflict === "head") refresh(caseId);
    return { ok: false, status: res.status, error: res.error, reason: res.reason, conflict: res.conflict };
  }
  refresh(caseId);
  const e = res.response.event;
  return { ok: true, recorded: { read: e.read, actor: e.actor ?? "", role: e.role, hash: e.hash, at: e.at }, phase: res.response.round.phase };
}

/** A 201 appended to the log: drop the cached case, audit and review pages so every view reads it again. */
function refresh(caseId: string) {
  revalidatePath("/app/patients/[id]", "page");
  revalidatePath("/app/patients/[id]/audit", "page");
  revalidatePath(`/app/review/${encodeURIComponent(caseId)}`);
  revalidatePath("/app/review");
}
