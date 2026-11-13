"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { apiMode, getCase, postBookingAction } from "@/lib/api/source";
import { BookingAction, type ActionResponse, type ApiOverlay, type Booking, type CaseSummary } from "@/lib/api/schema";

export type BookingProjection = { booking: Booking; overlay: ApiOverlay; case: CaseSummary };
export type BookingWriteResult =
  | { ok: true; response: ActionResponse | null; projection: BookingProjection | null; warning?: string }
  | {
      ok: false;
      status: number;
      error: string;
      conflict?: string;
      currentHead?: string;
      /** Conflicts re-read the server before another attempt is offered. Null means a reload is required. */
      projection: BookingProjection | null;
    };

const Args = z.object({ caseId: z.string().min(1).max(200), expectedHead: z.string().min(1).max(128), action: BookingAction });

function paths(caseId: string) {
  revalidatePath("/app/today");
  revalidatePath("/app/patients/[id]", "page");
  revalidatePath(`/app/patients/${encodeURIComponent(caseId)}/audit`);
}

async function projection(caseId: string): Promise<BookingProjection | null> {
  const detail = await getCase(caseId).catch(() => null);
  if (!detail?.api_overlay) return null;
  return { booking: detail.booking, overlay: detail.api_overlay, case: detail.case };
}

/** Append exactly one simulated reservation event, then render only the server's freshly read projection. */
export async function recordBooking(caseId: string, expectedHead: string, action: z.input<typeof BookingAction>): Promise<BookingWriteResult> {
  if (apiMode() !== "live") return { ok: false, status: 503, error: "No live API: fixture mode cannot save a simulated reservation", projection: null };
  const args = Args.safeParse({ caseId, expectedHead, action });
  if (!args.success) return { ok: false, status: 400, error: "Malformed simulated booking action", projection: null };
  const result = await postBookingAction(args.data.caseId, args.data.action, args.data.expectedHead);
  paths(args.data.caseId);
  if (!result.ok) {
    const latest = result.status === 409 ? await projection(args.data.caseId) : null;
    return {
      ok: false, status: result.status, error: result.error, conflict: result.conflict, currentHead: result.currentHead,
      projection: result.status === 409 ? latest : null,
    };
  }
  const latest = await projection(args.data.caseId);
  if (!latest) {
    return {
      ok: true, response: result.response, projection: null,
      warning: "The simulated booking event was saved, but the updated case could not be read. Reload before another action.",
    };
  }
  return { ok: true, response: result.response, projection: latest };
}
