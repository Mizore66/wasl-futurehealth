import type { NextRequest } from "next/server";
import { getReviewPhotoWithToken, apiMode } from "@/lib/api/source";
import { reviewerToken } from "@/lib/api/reviewer-session";
import { caseIdFromParam } from "@/lib/domain/ids";

/**
 * GET /app/review/<id>/photo/<item>/<sha>: streams one photo behind a review item. The server fetches it with the
 * signed-in reviewer's own credential, so the API's case, item, station and role checks decide; the browser never sees
 * a token. Every refusal is a fixed body that carries nothing from the API.
 */
export async function GET(_request: NextRequest, ctx: RouteContext<"/app/review/[id]/photo/[item]/[sha]">) {
  const headers = { "cache-control": "no-store", "x-content-type-options": "nosniff" };
  const refuse = (status: number) => Response.json({ error: "Photo unavailable." }, { status, headers });
  const { id: param, item, sha } = await ctx.params;
  let id = "";
  try {
    id = caseIdFromParam(param);
  } catch {
    return refuse(404);
  }
  if (apiMode() !== "live" || !id || id.length > 200 || id === "." || id === ".." || !/^[A-Za-z0-9_-]{1,80}$/.test(item) || !/^[0-9a-f]{64}$/.test(sha)) return refuse(404);
  const token = await reviewerToken();
  if (!token) return refuse(401);
  const got = await getReviewPhotoWithToken(id, item, sha, token);
  if (!got.ok) return refuse(got.status);
  return new Response(got.bytes, { status: 200, headers: { ...headers, "content-type": got.type } });
}
