"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { OUTCOME_NEXT_STEPS } from "@/lib/api/patient-schema";
import * as session from "@/lib/api/reviewer-session";
import { apiMode, postOutcomeHandoffApprovalWithToken, postOutcomeHandoffRelease, type OutcomeHandoffResult } from "@/lib/api/source";

/*
 * Staff steps of the post-closure outcome hand-off (docs/API-OUTCOME-HANDOFF.md). The browser sends the case and the
 * content it saw, never who acts; any identity field is refused, not ignored.
 *
 * - Approval is a clinical act: it is sent only with the signed-in person's own token (the reviewer-session cookie), like
 *   a disposition or a patient-contest resolution. The API allows only `clinician` and records that token's actor.
 * - Release is a coordinator action: it is sent with the server-side coordinator credential, like every other
 *   coordinator action on the case page. The API evaluates consent and records the gate it found.
 *
 * Nothing is dispatched: canonical closure, decisions and outreach are unchanged either way.
 */
const IDENTITY_FIELDS = ["credential", "token", "role", "actor"];

const ApproveInput = z.strictObject({
  caseId: z.string().min(1).max(200),
  /** The case's current decision signature, as shown on the page; the API refuses any other. */
  decision: z.string().min(1).max(128),
  /* No default: the clinician must choose from the closed list. */
  nextStep: z.enum(OUTCOME_NEXT_STEPS),
  /** The approval the clinician saw ("" for none); a stale one is refused by the API instead of replacing a newer one. */
  version: z.string().max(128),
});

const ReleaseInput = z.strictObject({
  caseId: z.string().min(1).max(200),
  version: z.string().min(1).max(128),
});

export type OutcomeHandoffActionResult = { ok: true; message: string; hash: string } | { ok: false; error: string; signIn?: true };

function identityRefused(input: unknown): boolean {
  return !!input && typeof input === "object" && IDENTITY_FIELDS.some((field) => field in input);
}

function revalidate(caseId: string) {
  const path = `/app/patients/${encodeURIComponent(caseId)}`;
  revalidatePath(path);
  revalidatePath(`${path}/audit`);
}

function refused(result: Extract<OutcomeHandoffResult, { ok: false }>): string {
  return `Not recorded: ${result.error}`;
}

export async function approveOutcomeHandoff(input: unknown): Promise<OutcomeHandoffActionResult> {
  if (identityRefused(input)) return { ok: false, error: "The page cannot choose who acts. An approval is recorded only as the signed-in credential." };
  const parsed = ApproveInput.safeParse(input);
  if (!parsed.success) return { ok: false, error: "Choose a next step from the list before approving." };
  if (apiMode() !== "live") return { ok: false, error: "Approving an outcome hand-off needs the live API. Fixtures cannot append events." };
  const token = await session.reviewerToken();
  if (!token) return { ok: false, error: "Sign in with the clinician token assigned to you. An approval is recorded only as the signed-in credential.", signIn: true };

  const { caseId, decision, nextStep, version } = parsed.data;
  const result = await postOutcomeHandoffApprovalWithToken(caseId, { decision, next_step: nextStep, version }, token);
  if (!result.ok) {
    if (result.status === 401) return { ok: false, error: "The API did not accept your sign-in. Sign in again.", signIn: true };
    return { ok: false, error: refused(result) };
  }
  revalidate(caseId);
  return {
    ok: true,
    hash: result.hash,
    message: result.replayed
      ? "This content was already approved. Nothing new was recorded."
      : "Content approved as your signed-in credential. Nothing is shown to the patient until a coordinator records a release.",
  };
}

export async function releaseOutcomeHandoff(input: unknown): Promise<OutcomeHandoffActionResult> {
  if (identityRefused(input)) return { ok: false, error: "The page cannot choose who acts. A release is recorded with the server's coordinator credential." };
  const parsed = ReleaseInput.safeParse(input);
  if (!parsed.success) return { ok: false, error: "There is no approved version to release." };
  if (apiMode() !== "live") return { ok: false, error: "Recording a release needs the live API. Fixtures cannot append events." };

  const { caseId, version } = parsed.data;
  const result = await postOutcomeHandoffRelease(caseId, version);
  if (!result.ok) return { ok: false, error: refused(result) };
  revalidate(caseId);
  return {
    ok: true,
    hash: result.hash,
    message: result.replayed
      ? "A release with the same consent result is already recorded for this version. Nothing new was recorded."
      : "Simulated release recorded with the coordinator credential. The server checked consent and recorded the result. No message was sent.",
  };
}
