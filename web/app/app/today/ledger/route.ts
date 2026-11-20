import type { NextRequest } from "next/server";
import { readFailure } from "../read-failure";
import { persistedLedger } from "@/components/worklist/ledger-trail";
import { todayRecords } from "@/components/worklist/today-records";
import { apiMode, getCase } from "@/lib/api/source";

/**
 * GET /app/today/ledger?id=<case id>: the persisted contact outcomes, notes and ledger status of ONE case, for the
 * patient open on Today (issue #47). Read on the server with the server-side API credential; the browser never sees a
 * token. Today calls it only for the selected case, never for every row. A failure says which kind it was: no
 * credential configured here (500), an answer that does not parse, an API refusal or error status, or no answer (502).
 */
function recordsOf(detail: Parameters<typeof todayRecords>[0]) {
  try {
    return todayRecords(detail);
  } catch {
    return undefined;
  }
}

export async function GET(request: NextRequest) {
  const noStore = { "cache-control": "no-store" };
  if (apiMode() !== "live") return Response.json({ error: "this demo keeps the call log in this browser" }, { status: 404, headers: noStore });
  const id = request.nextUrl.searchParams.get("id") ?? "";
  if (!id || id.length > 200) return Response.json({ error: "no case was chosen" }, { status: 400, headers: noStore });
  try {
    const detail = await getCase(id);
    if (!detail) return Response.json({ error: "this case could not be found" }, { status: 404, headers: noStore });
    return Response.json({
      ...persistedLedger(detail),
      bookingPanel: detail.api_overlay ? { booking: detail.booking, overlay: detail.api_overlay, case: detail.case } : null,
      // Today's Records and source Activity, from this same read, so the selected case's detail is read once. A detail
      // Today cannot project leaves them out (Records then says so); it never costs the contact trail.
      records: recordsOf(detail),
    }, { headers: noStore });
  } catch (e) {
    const { status, error } = readFailure(e);
    return Response.json({ error }, { status, headers: noStore });
  }
}
