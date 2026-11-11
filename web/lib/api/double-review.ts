import { DoubleReadRequest, DoubleReadResponse, DoubleReviewError, DoubleReviewState, type Role } from "./schema";
import { apiMode, authenticated, credentialName, liveBase } from "./source";

/*
 * Independent double review against the live API (docs/API-DOUBLE-REVIEW.md). Server-only: it reads credentials.
 *
 * The web has no per-person sign-in, so a deployment is ONE reader per question role: it acts with exactly one
 * credential, the role's own WASL_API_<ROLE>_TOKEN (link -> reviewer, evidence -> clinician), and with nothing else.
 * There is no seat switcher and no second or third credential here, so one visitor can never give two reads of the same
 * item as two people: the API records the one actor bound to that credential and refuses a second initial read by it.
 * Other readers act through their own deployments or their own API credentials. Per-person web seats (one signed-in
 * person mapped to exactly one credential) are designed, not built.
 */

export type Question = "link" | "evidence";

/** The role every read of a question needs (signoff_link, signoff_evidence in pipeline/governance.go). */
export function questionRole(question: Question): Role {
  return question === "link" ? "reviewer" : "clinician";
}

/** The one credential this deployment reads and answers with for a question role. */
export function deskCredentialName(role: Role): string {
  return credentialName(role);
}

export function deskConfigured(role: Role): boolean {
  return Boolean(process.env[deskCredentialName(role)]?.trim());
}

const route = (caseId: string) => `${liveBase()}/api/cases/${encodeURIComponent(caseId)}/double-review`;

/** The case's rounds as this deployment's credential for `role` may see them (blinded by the API). Null when the API has no such case. */
export async function getDoubleReview(caseId: string, role: Role): Promise<DoubleReviewState | null> {
  if (apiMode() !== "live") throw new Error("getDoubleReview needs WASL_API_URL: double review exists only on the live API");
  const res = await fetch(route(caseId), authenticated(undefined, role));
  if (res.status === 404) return null;
  if (!res.ok) throw new Error(`GET double-review: HTTP ${res.status}`);
  return DoubleReviewState.parse(await res.json());
}

export type DoubleReadResult =
  | { ok: true; status: 201; response: DoubleReadResponse }
  | { ok: false; status: 400 | 403 | 404 | 409 | 500; error: string; reason?: string; conflict?: string; currentHead?: string; phase?: string };

/** Appends one read with this deployment's credential for `role`. Refusals the API explains are returned, not thrown. */
export async function postDoubleRead(caseId: string, role: Role, body: DoubleReadRequest): Promise<DoubleReadResult> {
  const request = DoubleReadRequest.safeParse(body);
  if (!request.success) return { ok: false, status: 400, error: request.error.message };
  if (apiMode() !== "live") throw new Error("postDoubleRead needs WASL_API_URL: fixtures mode cannot append reads");
  const res = await fetch(
    route(caseId),
    authenticated({ method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(request.data) }, role),
  );
  const json = await res.json();
  if (res.status === 201) return { ok: true, status: 201, response: DoubleReadResponse.parse(json) };
  if (res.status === 400 || res.status === 403 || res.status === 404 || res.status === 409 || res.status === 500) {
    const e = DoubleReviewError.safeParse(json);
    if (e.success) {
      return { ok: false, status: res.status, error: e.data.error, reason: e.data.reason, conflict: e.data.conflict, currentHead: e.data.current_head, phase: e.data.phase };
    }
  }
  throw new Error(`POST double-review: HTTP ${res.status}`);
}
