import type { NextRequest } from "next/server";
import { readFailure } from "../read-failure";
import { todayRecords } from "@/components/worklist/today-records";
import { apiMode, getCase } from "@/lib/api/source";

/**
 * GET /app/today/records?id=<case id>: the source records, held items and source events of ONE case, for the case
 * open on Today. Read-only, on the server, with the shared read credential (the same read the case list uses); the
 * browser never sees a token or the raw detail. Today asks only for the selected case, and no review-session page
 * calls it. Fixtures mode answers from the published fixture detail, which exists only for the showcase cases: any
 * other case is a 404 that says so.
 */
export async function GET(request: NextRequest) {
  const noStore = { "cache-control": "no-store" };
  const id = request.nextUrl.searchParams.get("id") ?? "";
  if (!id || id.length > 200) return Response.json({ error: "no case was chosen" }, { status: 400, headers: noStore });
  try {
    const detail = await getCase(id);
    if (!detail) {
      const error = apiMode() === "live" ? "this case could not be found" : "this demo has records for a few sample patients only";
      return Response.json({ error, missing: true }, { status: 404, headers: noStore });
    }
    return Response.json(todayRecords(detail), { headers: noStore });
  } catch (e) {
    const { status, error } = readFailure(e);
    return Response.json({ error }, { status, headers: noStore });
  }
}
