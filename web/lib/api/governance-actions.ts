"use server";

import { refresh, revalidatePath } from "next/cache";
import { z } from "zod";
import { type SignInResult } from "@/components/review/auth";
import * as session from "@/lib/api/reviewer-session";
import { apiMode, postContestResolutionWithToken, postContestWithToken, postUndoWithToken } from "@/lib/api/source";

/*
 * Undo, contest and contest resolution act only as the signed-in identity: the reviewer-session token from PR #69, read
 * here from its HttpOnly cookie. The browser sends the action, target, resolution and reason, and nothing about who is
 * acting. Any identity field in the input is refused, not ignored, so a stale or crafted client fails loudly.
 * The API binds the token to its configured role and actor and enforces that a resolver differs from both the contester
 * and the original reviewer. That proves distinct configured actors, not distinct people.
 */
const IDENTITY_FIELDS = ["credential", "token", "role", "actor"];

const GovernanceInput = z
  .strictObject({
    caseId: z.string().min(1),
    target: z.string().min(1),
    operation: z.enum(["undo", "contest", "resolve"]),
    /* No default: a resolution is upheld or rejected only when the person chose it. */
    resolution: z.enum(["upheld", "rejected"]).optional(),
    reason: z.string().trim().min(1).max(1000),
  })
  .superRefine((input, ctx) => {
    if (input.operation === "resolve" && !input.resolution) ctx.addIssue({ code: "custom", message: "A resolution is required" });
    if (input.operation !== "resolve" && input.resolution) ctx.addIssue({ code: "custom", message: "Only a contest can be resolved" });
  });

export type GovernanceActionResult =
  | { ok: true; hash: string; message: string }
  /**
   * `signIn` is set when the action needs a (fresh) sign-in rather than a different input. The client then clears the
   * stale session with `signOutForGovernance`, so the page shows its sign-in prompt.
   */
  | { ok: false; error: string; signIn?: true };

const SIGN_IN = "Sign in with the token assigned to you. Undo, contest and resolve act only as the signed-in credential.";

export async function appendGovernanceAction(input: unknown): Promise<GovernanceActionResult> {
  if (input && typeof input === "object" && IDENTITY_FIELDS.some((field) => field in input)) {
    return { ok: false, error: "The page cannot choose who acts. Governance uses only the signed-in credential." };
  }
  const parsed = GovernanceInput.safeParse(input);
  if (!parsed.success) return { ok: false, error: "Choose an action, choose a resolution where asked, and enter a reason." };
  if (apiMode() !== "live") return { ok: false, error: "Governance actions need the live API; fixtures cannot append events." };
  const token = await session.reviewerToken();
  if (!token) return { ok: false, error: SIGN_IN, signIn: true };

  const { caseId, target, operation, resolution, reason } = parsed.data;
  const result =
    operation === "undo"
      ? await postUndoWithToken(caseId, { target, note: reason }, token)
      : operation === "contest"
        ? await postContestWithToken(caseId, { target, note: reason }, token)
        : await postContestResolutionWithToken(caseId, { target, resolution: resolution!, note: reason }, token);
  if (!result.ok) {
    if (result.status === 401) return { ok: false, error: "The API did not accept your sign-in. Sign in again.", signIn: true };
    return { ok: false, error: result.error };
  }

  revalidatePath(`/app/patients/${encodeURIComponent(caseId)}`);
  revalidatePath(`/app/patients/${encodeURIComponent(caseId)}/audit`);
  revalidatePath(`/app/review/${encodeURIComponent(caseId)}`);
  revalidatePath("/app/review");
  const message =
    operation === "undo"
      ? "Compensation appended as your signed-in credential. The original event and canonical decision are unchanged."
      : operation === "contest"
        ? "Contest appended as your signed-in credential. A different actor, signed in separately, must resolve it; the canonical decision is unchanged."
        : "Contest resolution appended as your signed-in credential. The canonical decision is unchanged.";
  return { ok: true, hash: result.response.event.hash, message };
}

/*
 * The same assigned-token sign-in as the review desk (one HttpOnly cookie, one identity per browser), offered where the
 * governance controls are. `refresh` re-renders the current page so the controls replace the prompt.
 */
export async function signInForGovernance(caseId: string, token: string): Promise<SignInResult> {
  const result = await session.signInReviewer(caseId, token);
  if (result.ok) refresh();
  return result;
}

export async function signOutForGovernance(caseId: string): Promise<void> {
  await session.signOutReviewer(caseId);
  refresh();
}
