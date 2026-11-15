"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { apiMode, postAction } from "@/lib/api/source";
import { CaseAction } from "@/lib/api/schema";

/**
 * `actor` is who the API recorded the event as: the actor bound server-side to the coordinator token. Today shows that,
 * never a name the frontend chose.
 */
export type RecordResult =
  | { ok: true; head: string; hash: string; at: string; actor: string; owner: string }
  | { ok: false; status: number; error: string; conflict?: string; currentHead?: string };

const Args = z.object({ caseId: z.string().min(1).max(200), head: z.string().min(1).max(128), action: CaseAction });

/**
 * Appends one coordinator action to the API ledger with the server-side coordinator credential. Nothing is recorded
 * unless this returns ok.
 */
export async function recordAction(caseId: string, head: string, action: z.input<typeof CaseAction>): Promise<RecordResult> {
  if (apiMode() !== "live") return { ok: false, status: 503, error: "No live API: this action stays in this browser" };
  const args = Args.safeParse({ caseId, head, action });
  if (!args.success) return { ok: false, status: 400, error: "malformed action" };
  const res = await postAction(args.data.caseId, args.data.action, args.data.head);
  if (!res.ok) return { ok: false, status: res.status, error: res.error, conflict: res.conflict, currentHead: res.currentHead };
  revalidatePath("/app/today");
  revalidatePath("/app/patients/[id]", "page");
  const { event, api_overlay } = res.response;
  return { ok: true, head: api_overlay.head, hash: event.hash, at: event.at, actor: event.actor, owner: api_overlay.owner };
}
