"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { apiMode, postAction } from "@/lib/api/source";
import { CaseAction } from "@/lib/api/schema";

/** Who Today's actions are recorded as. The role is always coordinator: the only role the API lets coordinate a case. */
const COORDINATOR = "Hessa Al Ali";

export type RecordResult =
  | { ok: true; head: string; hash: string; at: string }
  | { ok: false; status: number; error: string; currentHead?: string };

const Args = z.object({ caseId: z.string().min(1).max(200), head: z.string().min(1).max(128), action: CaseAction });

/** Appends one coordinator action to the API ledger. Nothing is recorded unless this returns ok. */
export async function recordAction(caseId: string, head: string, action: z.input<typeof CaseAction>): Promise<RecordResult> {
  if (apiMode() !== "live") return { ok: false, status: 503, error: "No live API: this action stays in this browser" };
  const args = Args.safeParse({ caseId, head, action });
  if (!args.success) return { ok: false, status: 400, error: "malformed action" };
  const res = await postAction(args.data.caseId, args.data.action, args.data.head, COORDINATOR);
  if (!res.ok) return { ok: false, status: res.status, error: res.error, currentHead: res.currentHead };
  revalidatePath("/app/today");
  revalidatePath("/app/patients/[id]", "page");
  return { ok: true, head: res.response.api_overlay.head, hash: res.response.event.hash, at: res.response.event.at };
}
