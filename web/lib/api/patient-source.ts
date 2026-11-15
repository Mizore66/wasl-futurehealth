import "server-only";
import { PatientError, PatientJourneyResponse, PatientMutationResponse, type PatientError as PatientErrorBody, type PatientJourneyResponse as Journey, type PatientMutationResponse as Mutation } from "./patient-schema";

export const PATIENT_COOKIE = "__Host-wasl_patient_link";
export const PATIENT_HEADERS = { "cache-control": "no-store", "referrer-policy": "no-referrer" } as const;

function patientBase(): string {
  const value = process.env.WASL_PATIENT_API_URL ?? process.env.WASL_API_URL;
  if (!value) throw new Error("WASL_API_URL is required for the live synthetic patient journey");
  return value.replace(/\/$/, "");
}

function authorized(token: string, init?: RequestInit): RequestInit {
  const headers = new Headers(init?.headers);
  headers.set("authorization", `Bearer ${token}`);
  return { cache: "no-store", ...init, headers };
}

export type PatientReadResult = { ok: true; response: Journey } | { ok: false; status: 401 | 410; error: string };

export async function getPatientJourney(token: string): Promise<PatientReadResult> {
  const res = await fetch(`${patientBase()}/api/patient/journey`, authorized(token));
  if (res.status === 401 || res.status === 410) {
    const parsed = PatientError.safeParse(await res.json().catch(() => undefined));
    return { ok: false, status: res.status, error: parsed.success ? parsed.data.error : `HTTP ${res.status}` };
  }
  if (!res.ok) throw new Error(`GET patient journey: HTTP ${res.status}`);
  return { ok: true, response: PatientJourneyResponse.parse(await res.json()) };
}

export type PatientMutationResult =
  /** Null response means the append succeeded but its body was unreadable; callers must refetch before another action. */
  | { ok: true; status: 200 | 201; response: Mutation | null }
  | { ok: false; status: 400 | 401 | 409 | 410 | 500; error: PatientErrorBody };

/** Forward the original bytes so Go still rejects duplicate JSON fields and oversized bodies. */
export async function postPatientJourney(token: string, rawBody: string): Promise<PatientMutationResult> {
  const res = await fetch(
    `${patientBase()}/api/patient/journey`,
    authorized(token, { method: "POST", headers: { "content-type": "application/json" }, body: rawBody }),
  );
  const json = await res.json().catch(() => undefined);
  if (res.status === 200 || res.status === 201) {
    const saved = PatientMutationResponse.safeParse(json);
    return { ok: true, status: res.status, response: saved.success ? saved.data : null };
  }
  if ([400, 401, 409, 410, 500].includes(res.status)) {
    const parsed = PatientError.safeParse(json);
    return {
      ok: false,
      status: res.status as 400 | 401 | 409 | 410 | 500,
      error: parsed.success ? parsed.data : { error: `HTTP ${res.status}` },
    };
  }
  throw new Error(`POST patient journey: HTTP ${res.status}`);
}
