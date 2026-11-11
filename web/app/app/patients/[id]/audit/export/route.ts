import type { NextRequest } from "next/server";
import { exportFilename, fetchCaseExport } from "@/lib/api/export";
import { caseIdFromParam } from "@/lib/domain/ids";

/**
 * GET /app/patients/<id>/audit/export: downloads the API's auditor-only redacted evidence export (issue #46).
 * The server fetches it with the auditor credential and streams the API's bytes unchanged, so the file and its
 * content_sha256 are exactly what the backend served; the browser never sees a token. Refusals (401/403/404, no live
 * API, no credential) get a fixed message and never the API's error body or metadata.
 */
export async function GET(_request: NextRequest, ctx: RouteContext<"/app/patients/[id]/audit/export">) {
  const noStore = { "cache-control": "no-store", "x-content-type-options": "nosniff" };
  const param = (await ctx.params).id;
  let id = "";
  try {
    id = caseIdFromParam(param);
  } catch {
    // A malformed percent-escape: answered below as a missing id.
  }
  if (!id || id.length > 200) return Response.json({ error: "A case id is required." }, { status: 400, headers: noStore });

  const got = await fetchCaseExport(id);
  if (!got.ok) return Response.json({ error: got.message }, { status: got.status, headers: noStore });
  return new Response(got.response.body, {
    status: 200,
    headers: {
      ...noStore,
      "content-type": "application/json; charset=utf-8",
      "content-disposition": `attachment; filename="${exportFilename(id)}"`,
    },
  });
}
