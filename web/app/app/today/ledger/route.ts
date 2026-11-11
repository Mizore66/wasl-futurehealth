import type { NextRequest } from "next/server";
import { persistedLedger } from "@/components/worklist/ledger-trail";
import { apiMode, getCase } from "@/lib/api/source";

/**
 * GET /app/today/ledger?id=<case id>: the persisted contact outcomes, notes and ledger status of ONE case, for the
 * patient open on Today (issue #47). Read on the server with the server-side API credential; the browser never sees a
 * token. Today calls it only for the selected case, never for every row.
 */
export async function GET(request: NextRequest) {
  const noStore = { "cache-control": "no-store" };
  if (apiMode() !== "live") return Response.json({ error: "no live API: Today keeps its log in this browser" }, { status: 404, headers: noStore });
  const id = request.nextUrl.searchParams.get("id") ?? "";
  if (!id || id.length > 200) return Response.json({ error: "id is required" }, { status: 400, headers: noStore });
  try {
    const detail = await getCase(id);
    if (!detail) return Response.json({ error: "unknown case" }, { status: 404, headers: noStore });
    return Response.json(persistedLedger(detail), { headers: noStore });
  } catch {
    return Response.json({ error: "the API did not answer" }, { status: 502, headers: noStore });
  }
}
