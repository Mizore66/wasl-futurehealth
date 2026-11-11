"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { apiMode, postContest, postContestResolution, postUndo } from "@/lib/api/source";

const GovernanceInput = z
  .object({
    caseId: z.string().min(1),
    target: z.string().min(1),
    operation: z.enum(["undo", "contest", "resolve"]),
    resolution: z.enum(["upheld", "rejected"]).optional(),
    reason: z.string().trim().min(1).max(1000),
    credential: z.enum(["reviewer", "reviewer_2", "reviewer_3", "clinician", "coordinator"]),
  })
  .superRefine((input, ctx) => {
    if (input.operation === "resolve" && !input.resolution) ctx.addIssue({ code: "custom", message: "A resolution is required" });
    if (input.operation !== "resolve" && input.resolution) ctx.addIssue({ code: "custom", message: "Only a contest can be resolved" });
  });

/** Append-only governance. The API, not the browser, binds credentials to authorised actors and checks independence. */
export async function appendGovernanceAction(input: unknown) {
  const parsed = GovernanceInput.safeParse(input);
  if (!parsed.success) return { ok: false as const, error: "Choose an action and authorised credential, and enter a reason." };
  if (apiMode() !== "live") return { ok: false as const, error: "Governance actions need the live API; fixtures cannot append events." };
  const { caseId, target, operation, resolution, reason, credential } = parsed.data;
  const result =
    operation === "undo"
      ? await postUndo(caseId, { target, note: reason }, credential)
      : operation === "contest"
        ? await postContest(caseId, { target, note: reason }, credential)
        : await postContestResolution(caseId, { target, resolution: resolution!, note: reason }, credential);
  if (!result.ok) return { ok: false as const, error: result.error };

  revalidatePath(`/app/patients/${encodeURIComponent(caseId)}`);
  revalidatePath(`/app/patients/${encodeURIComponent(caseId)}/audit`);
  revalidatePath(`/app/review/${encodeURIComponent(caseId)}`);
  revalidatePath("/app/review");
  const message =
    operation === "undo"
      ? "Compensation appended. The original event and canonical decision are unchanged."
      : operation === "contest"
        ? "Contest appended. A different authorised actor must resolve it; the canonical decision is unchanged."
        : "Contest resolution appended. The canonical decision is unchanged.";
  return { ok: true as const, hash: result.response.event.hash, message };
}
