"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { postConsent } from "@/lib/api/consent";
import { apiMode } from "@/lib/api/source";
import { ConsentEvent, type ConsentOverlay } from "@/lib/api/schema";

/** `actor` is the one the API bound to the coordinator token, never a name the browser chose. */
export type ConsentActionResult =
  | { ok: true; hash: string; at: string; actor: string; overlay: ConsentOverlay }
  | { ok: false; status: number; error: string; currentHead?: string };

const Args = z.object({ caseId: z.string().min(1).max(200), head: z.string().min(1).max(128), event: ConsentEvent });

/**
 * Appends one consent event (demo overlay) to the API ledger with the server-side coordinator credential, then drops the
 * cached case and audit pages so the page re-reads the case state. Nothing is recorded unless this returns ok.
 */
export async function recordConsent(caseId: string, head: string, event: string): Promise<ConsentActionResult> {
  if (apiMode() !== "live") return { ok: false, status: 503, error: "No live API: consent actions need WASL_API_URL" };
  const args = Args.safeParse({ caseId, head, event });
  if (!args.success) return { ok: false, status: 400, error: "malformed consent action" };
  const res = await postConsent(args.data.caseId, args.data.event, args.data.head);
  if (!res.ok) return { ok: false, status: res.status, error: res.error, currentHead: res.currentHead };
  revalidatePath("/app/patients/[id]", "page");
  revalidatePath("/app/patients/[id]/audit", "page");
  const { review, overlay } = res.response;
  return { ok: true, hash: review.hash, at: review.at, actor: review.actor, overlay };
}
