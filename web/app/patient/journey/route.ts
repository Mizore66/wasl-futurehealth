import { patientJSON as json, patientToken as token } from "@/lib/api/patient-proxy";
import { getPatientJourney, postPatientJourney, PATIENT_HEADERS } from "@/lib/api/patient-source";

export async function GET(request: Request) {
  const credential = token(request);
  if (!credential) return json({ error: "patient link is missing" }, 401);
  try {
    const result = await getPatientJourney(credential);
    if (!result.ok) return json({ error: result.error }, result.status);
    return json(result.response);
  } catch {
    return json({ error: "patient journey is unavailable" }, 503);
  }
}

export async function POST(request: Request) {
  const credential = token(request);
  if (!credential) return json({ error: "patient link is missing" }, 401);
  const raw = await request.text();
  if (new TextEncoder().encode(raw).length > 8192) return json({ error: "body is larger than 8 KiB" }, 400);
  try {
    const result = await postPatientJourney(credential, raw);
    if (!result.ok) return json(result.error, result.status);
    // A null body deliberately preserves 201: the browser must refetch rather
    // than retry blindly or display an unconfirmed projection as saved.
    if (!result.response) return new Response(null, { status: result.status, headers: PATIENT_HEADERS });
    return json(result.response, result.status);
  } catch {
    return json({ error: "patient journey is unavailable" }, 503);
  }
}
