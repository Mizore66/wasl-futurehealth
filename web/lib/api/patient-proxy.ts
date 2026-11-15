import "server-only";
import { NextResponse } from "next/server";
import { forwardPatientRights, PATIENT_COOKIE, PATIENT_HEADERS, type PatientRightsPath } from "./patient-source";

export function patientJSON(body: unknown, status = 200) {
  const response = NextResponse.json(body, { status, headers: PATIENT_HEADERS });
  if (status === 401 || status === 410) {
    response.cookies.set(PATIENT_COOKIE, "", { httpOnly: true, secure: true, sameSite: "lax", path: "/", maxAge: 0 });
  }
  return response;
}

export function patientToken(request: Request): string {
  const cookie = request.headers.get("cookie") ?? "";
  for (const part of cookie.split(";")) {
    const [name, ...value] = part.trim().split("=");
    if (name === PATIENT_COOKIE) return value.join("=");
  }
  return "";
}

export async function proxyPatientRights(request: Request, path: PatientRightsPath) {
  const credential = patientToken(request);
  if (!credential) return patientJSON({ error: "patient link is missing" }, 401);
  let raw: string | undefined;
  if (request.method === "POST") {
    raw = await request.text();
    if (new TextEncoder().encode(raw).length > 8192) return patientJSON({ error: "body is larger than 8 KiB" }, 400);
  }
  try {
    const { status, body } = await forwardPatientRights(credential, path, raw);
    return patientJSON(body, status);
  } catch {
    return patientJSON({ error: "patient requests are unavailable" }, 503);
  }
}
