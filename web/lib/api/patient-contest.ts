import "server-only";
import { z } from "zod";
import { ContestResolution } from "@/components/patient-contest/model";
import { apiMode, liveBase } from "./source";

const Saved = z.looseObject({ event: z.looseObject({ hash: z.string() }) });
const Refused = z.looseObject({ error: z.string() });

export type ContestPostResult = { ok: true; hash: string } | { ok: false; status: 400 | 401 | 403 | 404 | 409 | 500; error: string };

/**
 * Records one contest resolution with the signed-in person's own token (the reviewer-session credential), never a
 * server-held role token. The API binds the token to its role and actor, refuses every role without
 * `resolve_patient_contest` and every original reviewer of the case.
 */
export async function postPatientContestWithToken(caseId: string, input: ContestResolution, token: string): Promise<ContestPostResult> {
  if (apiMode() !== "live") throw new Error("postPatientContestWithToken needs WASL_API_URL: fixtures mode cannot append resolutions");
  const body = ContestResolution.safeParse(input);
  if (!body.success) return { ok: false, status: 400, error: body.error.issues[0]?.message ?? "Malformed resolution" };
  const res = await fetch(`${liveBase()}/api/cases/${encodeURIComponent(caseId)}/patient-contest`, {
    method: "POST",
    cache: "no-store",
    headers: new Headers({ "content-type": "application/json", authorization: `Bearer ${token}` }),
    body: JSON.stringify(body.data),
  });
  const json = await res.json().catch(() => undefined);
  if (res.status === 201) {
    const saved = Saved.safeParse(json);
    return { ok: true, hash: saved.success ? saved.data.event.hash : "" };
  }
  if ([400, 401, 403, 404, 409, 500].includes(res.status)) {
    const e = Refused.safeParse(json);
    return { ok: false, status: res.status as 400, error: e.success ? e.data.error : `HTTP ${res.status}` };
  }
  throw new Error(`POST patient-contest: HTTP ${res.status}`);
}
