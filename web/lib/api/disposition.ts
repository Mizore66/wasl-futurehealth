import "server-only";
import { z } from "zod";
import { apiMode, liveBase } from "./source";
import { DispositionInput } from "@/components/disposition/model";

/* Only the fields this action reads; the full envelopes are pinned by the shared ActionResponse/ActionError schemas. */
const Saved = z.looseObject({ event: z.looseObject({ hash: z.string() }) });
const Refused = z.looseObject({ error: z.string(), conflict: z.string().optional() });

export type DispositionPostResult =
  | { ok: true; hash: string }
  | { ok: false; status: 400 | 401 | 403 | 404 | 409 | 500; error: string; conflict?: string };

/**
 * Appends one clinician disposition with the signed-in person's own assigned token (the reviewer-session credential),
 * never a server-held role token. The API binds the token to its role and actor, stamps the hold or decline it answers,
 * checks `expected_head`, and refuses every role without `record_clinician_disposition`.
 */
export async function postDispositionWithToken(caseId: string, input: DispositionInput, expectedHead: string, token: string): Promise<DispositionPostResult> {
  if (apiMode() !== "live") throw new Error("postDispositionWithToken needs WASL_API_URL: fixtures mode cannot append dispositions");
  const body = DispositionInput.safeParse(input);
  if (!body.success) return { ok: false, status: 400, error: body.error.issues[0]?.message ?? "Malformed disposition" };
  const headers = new Headers({ "content-type": "application/json", authorization: `Bearer ${token}` });
  const res = await fetch(`${liveBase()}/api/cases/${encodeURIComponent(caseId)}/actions`, {
    method: "POST",
    cache: "no-store",
    headers,
    body: JSON.stringify({ ...body.data, expected_head: expectedHead }),
  });
  const json = await res.json().catch(() => undefined);
  if (res.status === 201) {
    const saved = Saved.safeParse(json);
    return { ok: true, hash: saved.success ? saved.data.event.hash : "" };
  }
  if ([400, 401, 403, 404, 409, 500].includes(res.status)) {
    const e = Refused.safeParse(json);
    return { ok: false, status: res.status as 400, error: e.success ? e.data.error : `HTTP ${res.status}`, conflict: e.success ? e.data.conflict : undefined };
  }
  throw new Error(`POST disposition: HTTP ${res.status}`);
}
