import { NextResponse, type NextRequest } from "next/server";
import { getPatientJourney, PATIENT_COOKIE, PATIENT_HEADERS } from "@/lib/api/patient-source";

const cookieOptions = { httpOnly: true, secure: true, sameSite: "lax", path: "/" } as const;

function leave(request: NextRequest, reason: "invalid" | "expired" | "unavailable") {
  const target = new URL("/patient/notice", request.url);
  target.searchParams.set("error", reason);
  const res = NextResponse.redirect(target, 303);
  res.headers.set("cache-control", PATIENT_HEADERS["cache-control"]);
  res.headers.set("referrer-policy", PATIENT_HEADERS["referrer-policy"]);
  res.cookies.set(PATIENT_COOKIE, "", { ...cookieOptions, maxAge: 0 });
  return res;
}

export async function GET(request: NextRequest) {
  const token = request.nextUrl.searchParams.get("link") ?? "";
  if (!token) return leave(request, "invalid");
  try {
    const result = await getPatientJourney(token);
    if (!result.ok) return leave(request, result.status === 410 ? "expired" : "invalid");
    const res = NextResponse.redirect(new URL("/patient/notice", request.url), 303);
    res.headers.set("cache-control", PATIENT_HEADERS["cache-control"]);
    res.headers.set("referrer-policy", PATIENT_HEADERS["referrer-policy"]);
    res.cookies.set(PATIENT_COOKIE, token, { ...cookieOptions, expires: new Date(result.response.expires_at) });
    return res;
  } catch {
    return leave(request, "unavailable");
  }
}
