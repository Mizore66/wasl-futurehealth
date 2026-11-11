"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { postDoubleRead, questionRole, seatConfigured, seatCredentialName } from "@/lib/api/double-review";
import { apiMode } from "@/lib/api/source";

/*
 * One double-review read (docs/API-DOUBLE-REVIEW.md). The browser names the seat and the answer; the server picks the
 * seat's own credential, and the API records the person bound to that credential, whatever this page believes.
 */

const Input = z.object({
  caseId: z.string().min(1),
  item: z.string().min(1),
  question: z.enum(["link", "evidence"]),
  seat: z.enum(["r1", "r2", "r3"]),
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
  const { caseId, item, question, seat, decision, expectedHead } = parsed.data;
  // The role follows the question. A client that names the wrong question only gets a credential the API refuses.
  const role = questionRole(question);
  if (!seatConfigured(seat, role)) {
    return { ok: false, status: 0, error: `no credential for this seat (${seatCredentialName(seat, role)} is not set on the server)` };
  }
  const res = await postDoubleRead(caseId, seat, role, { item, read: seat, decision, note: "", expected_head: expectedHead });
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
