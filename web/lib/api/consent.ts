import { apiMode, authenticated, liveBase } from "./source";
import { ApiContact, ConsentError, ConsentRequest, ConsentResponse, ConsentState, type CaseDetail, type ConsentEvent, type ConsentOverlay } from "./schema";

/*
 * Consent demo overlay client (issue #42; pipeline/api_consent.go, docs/API-CONSENT.md).
 * Writes go out with the server-side coordinator credential (WASL_API_COORDINATOR_TOKEN). The API binds that token to
 * its role and actor and overwrites any identity header, so none is sent from here. Coordinator and clinician may record
 * consent (pipeline/governance.go rbac); this client uses the coordinator role.
 * Fixtures mode has no ledger: these functions throw rather than fabricate an event or an overlay.
 */

/** The case detail's `api_contact`, parsed: absent in snapshots that predate the endpoint, unreadable if off-contract. */
export type ContactView = { kind: "absent" } | { kind: "unreadable"; error: string } | { kind: "ok"; contact: ApiContact };

export function apiContactOf(detail: CaseDetail): ContactView {
  const raw = (detail as Record<string, unknown>).api_contact;
  if (raw === undefined || raw === null) return { kind: "absent" };
  const parsed = ApiContact.safeParse(raw);
  return parsed.success ? { kind: "ok", contact: parsed.data } : { kind: "unreadable", error: parsed.error.issues[0]?.message ?? "invalid" };
}

/** The head a consent write must name: the case's current decision signature (the API compares it with `c.last.Sig`). */
export function consentHead(detail: CaseDetail): string | null {
  return detail.current.sig || null;
}

/** GET /api/cases/{id}/consent: the overlay and its event history; null for an unknown case. */
export async function getConsent(id: string): Promise<ConsentState | null> {
  if (apiMode() !== "live") throw new Error("getConsent needs WASL_API_URL: fixtures mode has no consent ledger");
  const route = `/api/cases/${encodeURIComponent(id)}/consent`;
  const res = await fetch(liveBase() + route, authenticated(undefined, "coordinator"));
  if (res.status === 404) return null;
  if (!res.ok) throw new Error(`${route}: HTTP ${res.status}`);
  return ConsentState.parse(await res.json());
}

export type ConsentResult =
  | { ok: true; status: 201; response: ConsentResponse }
  | { ok: false; status: 400 | 403 | 404 | 409 | 500; error: string; currentHead?: string; overlay?: ConsentOverlay };

/**
 * POST /api/cases/{id}/consent. 400, 403, 404, 409 (stale head, or a grant after an opt-out) and an explained 500 are
 * returned as refusals for the page to show in place; anything else throws.
 */
export async function postConsent(id: string, event: ConsentEvent, expectedHead: string, note = ""): Promise<ConsentResult> {
  const body = ConsentRequest.safeParse({ event, expected_head: expectedHead, note });
  if (!body.success) return { ok: false, status: 400, error: body.error.issues[0]?.message ?? "malformed consent event" };
  if (apiMode() !== "live") throw new Error("postConsent needs WASL_API_URL: fixtures mode cannot append consent events");

  const res = await fetch(
    `${liveBase()}/api/cases/${encodeURIComponent(id)}/consent`,
    authenticated({ method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(body.data) }, "coordinator"),
  );
  const json = await res.json();
  if (res.status === 201) return { ok: true, status: 201, response: ConsentResponse.parse(json) };
  if (res.status === 400 || res.status === 403 || res.status === 404 || res.status === 409 || res.status === 500) {
    const e = ConsentError.safeParse(json);
    if (e.success) return { ok: false, status: res.status, error: e.data.error, currentHead: e.data.current_head, overlay: e.data.api_contact };
  }
  throw new Error(`POST consent: HTTP ${res.status}`);
}
