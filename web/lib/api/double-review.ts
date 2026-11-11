import { DoubleReadRequest, DoubleReadResponse, DoubleReviewError, DoubleReviewState, type ReadName, type Role } from "./schema";
import { apiMode, authenticated, credentialName, liveBase } from "./source";

/*
 * Independent double review against the live API (docs/API-DOUBLE-REVIEW.md). Server-only: it reads credentials.
 *
 * A seat (r1, r2, r3) is a separate server-held credential, never a label on one credential: the API binds each token
 * to one person and records that person. The question decides the role (link -> reviewer, evidence -> clinician), so
 * a seat's credential is WASL_API_<ROLE>_TOKEN for r1, WASL_API_<ROLE>_2_TOKEN for r2 and WASL_API_<ROLE>_3_TOKEN for r3.
 * A missing credential is reported as missing; nothing falls back to another seat's credential.
 */

export type Seat = ReadName;
export const SEATS: readonly Seat[] = ["r1", "r2", "r3"];
export const SEAT_SLOT: Record<Seat, string> = { r1: "", r2: "2", r3: "3" };

export type Question = "link" | "evidence";

/** The role every read of a question needs (signoff_link, signoff_evidence in pipeline/governance.go). */
export function questionRole(question: Question): Role {
  return question === "link" ? "reviewer" : "clinician";
}

export function seatCredentialName(seat: Seat, role: Role): string {
  return credentialName(role, SEAT_SLOT[seat]);
}

export function seatConfigured(seat: Seat, role: Role): boolean {
  return Boolean(process.env[seatCredentialName(seat, role)]?.trim());
}

const route = (caseId: string) => `${liveBase()}/api/cases/${encodeURIComponent(caseId)}/double-review`;

/** The case's rounds as this seat's credential may see them (blinded by the API). Null when the API has no such case. */
export async function getDoubleReview(caseId: string, seat: Seat, role: Role): Promise<DoubleReviewState | null> {
  if (apiMode() !== "live") throw new Error("getDoubleReview needs WASL_API_URL: double review exists only on the live API");
  const res = await fetch(route(caseId), authenticated(undefined, role, SEAT_SLOT[seat]));
  if (res.status === 404) return null;
  if (!res.ok) throw new Error(`GET double-review: HTTP ${res.status}`);
  return DoubleReviewState.parse(await res.json());
}

export type DoubleReadResult =
  | { ok: true; status: 201; response: DoubleReadResponse }
  | { ok: false; status: 400 | 403 | 404 | 409 | 500; error: string; reason?: string; conflict?: string; currentHead?: string; phase?: string };

/** Appends one read with this seat's credential. Refusals the API explains are returned, not thrown. */
export async function postDoubleRead(caseId: string, seat: Seat, role: Role, body: DoubleReadRequest): Promise<DoubleReadResult> {
  const request = DoubleReadRequest.safeParse(body);
  if (!request.success) return { ok: false, status: 400, error: request.error.message };
  if (apiMode() !== "live") throw new Error("postDoubleRead needs WASL_API_URL: fixtures mode cannot append reads");
  const res = await fetch(
    route(caseId),
    authenticated({ method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(request.data) }, role, SEAT_SLOT[seat]),
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
