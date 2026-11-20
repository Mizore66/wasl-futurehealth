"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { apiMode, getCase, postConsent, postTeachBackCallback, type TeachBackCallbackResult } from "@/lib/api/source";
import { ConsentEventName, type ApiContact } from "@/lib/api/schema";

export type ConsentActionResult =
  | { ok: true; contact: ApiContact | null; expectedHead: string; warning?: string }
  | { ok: false; status: number; error: string; currentHead?: string };

const Args = z.object({
  caseId: z.string().min(1).max(200),
  expectedHead: z.string().min(1).max(128),
  event: ConsentEventName,
  note: z.string().max(500),
});

/** Append a demo consent event, then read the case back. The coordinator credential and actor stay bound on the server. */
export async function recordConsent(
  caseId: string,
  expectedHead: string,
  event: z.input<typeof ConsentEventName>,
  note = "",
): Promise<ConsentActionResult> {
  if (apiMode() !== "live") return { ok: false, status: 503, error: "Consent can't be saved in this demo." };
  const args = Args.safeParse({ caseId, expectedHead, event, note });
  if (!args.success) return { ok: false, status: 400, error: "Consent update not recognised. Nothing was saved." };

  const result = await postConsent(args.data.caseId, {
    event: args.data.event,
    expected_head: args.data.expectedHead,
    note: args.data.note,
  });
  if (!result.ok) return { ok: false, status: result.status, error: result.error, currentHead: result.currentHead };

  const path = `/app/patients/${encodeURIComponent(args.data.caseId)}`;
  revalidatePath(path);
  revalidatePath(`${path}/audit`);
  /* The review page renders SignoffsCard from the same ledger. */
  revalidatePath(`/app/review/${encodeURIComponent(args.data.caseId)}`);
  /* The event is already appended: a failed read-back is a warning, not "the API did not answer". */
  const detail = await getCase(args.data.caseId).catch(() => null);
  if (!detail?.api_contact) {
    return {
      ok: true,
      contact: null,
      expectedHead: "",
      warning: "Consent saved. Reload the page before doing anything else.",
    };
  }
  return { ok: true, contact: detail.api_contact, expectedHead: detail.api_contact.overlay.revision };
}

const CallbackArgs = z.strictObject({
  caseId: z.string().min(1).max(200),
  callback: z.string().min(1).max(128),
  outcome: z.enum(["understood_on_call", "another_explanation"]),
});

/** Record the outcome of a failed teach-back callback as the server's coordinator credential. */
export async function recordTeachBackCallback(input: unknown): Promise<TeachBackCallbackResult> {
  if (apiMode() !== "live") return { ok: false, status: 503, error: "Call outcomes can't be saved in this demo." };
  const args = CallbackArgs.safeParse(input);
  if (!args.success) return { ok: false, status: 400, error: "Call outcome not recognised. Nothing was saved." };
  const result = await postTeachBackCallback(args.data.caseId, args.data.callback, args.data.outcome);
  if (result.ok) {
    const path = `/app/patients/${encodeURIComponent(args.data.caseId)}`;
    revalidatePath(path);
    revalidatePath(`${path}/audit`);
    revalidatePath("/app/patients");
  }
  return result;
}
