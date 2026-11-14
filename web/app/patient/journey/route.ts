import { NextResponse } from "next/server";
import { getPatientJourney, postPatientJourney, PATIENT_COOKIE, PATIENT_HEADERS } from "@/lib/api/patient-source";

function json(request: Request, body: unknown, status = 200) {
  const response = NextResponse.json(body, { status, headers: PATIENT_HEADERS });
  if (status === 401 || status === 410) {
    response.cookies.set(PATIENT_COOKIE, "", { httpOnly: true, secure: true, sameSite: "lax", path: "/", maxAge: 0 });
  }
  return response;
}

function token(request: Request): string {
  const cookie = request.headers.get("cookie") ?? "";
  for (const part of cookie.split(";")) {
    const [name, ...value] = part.trim().split("=");
    if (name === PATIENT_COOKIE) return value.join("=");
  }
  return "";
}

export async function GET(request: Request) {
  const credential = token(request);
  if (!credential) return json(request, { error: "patient link is missing" }, 401);
  try {
    const result = await getPatientJourney(credential);
    if (!result.ok) return json(request, { error: result.error }, result.status);
    return json(request, result.response);
  } catch {
    return json(request, { error: "patient journey is unavailable" }, 503);
  }
}

export async function POST(request: Request) {
  const credential = token(request);
  if (!credential) return json(request, { error: "patient link is missing" }, 401);
  const raw = await request.text();
  if (new TextEncoder().encode(raw).length > 8192) return json(request, { error: "body is larger than 8 KiB" }, 400);
  try {
    const result = await postPatientJourney(credential, raw);
    if (!result.ok) return json(request, result.error, result.status);
    // A null body deliberately preserves 201: the browser must refetch rather
    // than retry blindly or display an unconfirmed projection as saved.
    if (!result.response) return new Response(null, { status: result.status, headers: PATIENT_HEADERS });
    return json(request, result.response, result.status);
  } catch {
    return json(request, { error: "patient journey is unavailable" }, 503);
  }
}
