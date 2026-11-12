import type { NextRequest } from "next/server";
import { ZodError } from "zod";
import { persistedLedger } from "@/components/worklist/ledger-trail";
import { apiMode, ApiStatusError, getCase, MissingCredentialError } from "@/lib/api/source";

/** Why a read failed, in words Today shows as given, and the status this route answers with. */
function failure(e: unknown): { status: number; error: string } {
  if (e instanceof MissingCredentialError) return { status: 500, error: "this server has no API read credential configured" };
  if (e instanceof ZodError) return { status: 502, error: "the API answered in a shape this page does not recognise" };
  if (e instanceof ApiStatusError) {
    if (e.status === 401 || e.status === 403) return { status: 502, error: `the API refused this server's read credential (HTTP ${e.status})` };
    return { status: 502, error: `the API answered HTTP ${e.status}` };
  }
  return { status: 502, error: "the API did not answer" };
}

/**
 * GET /app/today/ledger?id=<case id>: the persisted contact outcomes, notes and ledger status of ONE case, for the
 * patient open on Today (issue #47). Read on the server with the server-side API credential; the browser never sees a
 * token. Today calls it only for the selected case, never for every row. A failure says which kind it was: no
 * credential configured here (500), an answer that does not parse, an API refusal or error status, or no answer (502).
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
  } catch (e) {
    const { status, error } = failure(e);
    return Response.json({ error }, { status, headers: noStore });
  }
}
