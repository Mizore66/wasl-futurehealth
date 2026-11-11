import "server-only";
import { z } from "zod";
import { apiMode, authenticated, liveBase } from "./source";

/*
 * The auditor-only redacted evidence export (issue #46): GET /api/cases/{id}/export on the Go API
 * (pipeline/api_export.go, docs/API-EXPORT.md). The API builds and redacts the document; this module only fetches it
 * with the server-side auditor credential. Nothing here builds, edits or re-serialises an export, and no backend error
 * body (which carries API metadata) is passed on: refusals become the fixed messages below.
 */

export const EXPORT_SCHEMA = "wasl-case-export/1";

const Sha256 = z.string().regex(/^[0-9a-f]{64}$/);

/** Only what the audit page shows. Zod drops every other key, so the page holds no detail or audit content. */
export const ExportHashes = z.object({
  schema: z.literal(EXPORT_SCHEMA),
  redacted: z.literal(true),
  content_sha256: Sha256,
  comparison_sha256: Sha256,
});
export type ExportHashes = z.infer<typeof ExportHashes>;

export type ExportRefusalReason = "fixtures" | "credential" | "unauthenticated" | "forbidden" | "missing" | "unavailable";

/** Fixed wording for each refusal. None of it comes from the API's response. */
export const EXPORT_REFUSAL: Record<ExportRefusalReason, { status: number; message: string }> = {
  fixtures: {
    status: 404,
    message:
      "This page is reading the published snapshot, which has no auditor API. The export only ever comes from the Wasl API, so there is no file to download here.",
  },
  credential: { status: 503, message: "This server has no auditor credential set up, so it cannot ask the Wasl API for the export." },
  unauthenticated: { status: 401, message: "The Wasl API did not accept this server's auditor credential, so no export was downloaded." },
  forbidden: { status: 403, message: "The Wasl API refused the export. Only the auditor role may download it." },
  missing: { status: 404, message: "The Wasl API has no case with this id, so there is nothing to export." },
  unavailable: { status: 502, message: "The Wasl API did not return the export. Try again later." },
};

export type ExportRefusal = { ok: false; reason: ExportRefusalReason; status: number; message: string };

function refusal(reason: ExportRefusalReason): ExportRefusal {
  return { ok: false, reason, ...EXPORT_REFUSAL[reason] };
}

/**
 * Asks the API for the export with the auditor token. On 200 the response is returned unread, so a caller can stream
 * its bytes unchanged. Any other answer is reduced to a refusal and its body is discarded.
 */
export async function fetchCaseExport(id: string): Promise<{ ok: true; response: Response } | ExportRefusal> {
  if (apiMode() !== "live") return refusal("fixtures");
  let init: RequestInit;
  try {
    init = authenticated(undefined, "auditor");
  } catch {
    return refusal("credential");
  }
  let res: Response;
  try {
    res = await fetch(`${liveBase()}/api/cases/${encodeURIComponent(id)}/export`, init);
  } catch {
    return refusal("unavailable");
  }
  if (res.status === 200 && /^application\/json\b/i.test(res.headers.get("content-type") ?? "")) return { ok: true, response: res };
  // Drain and drop the refusal body (small, and it carries API metadata). Not body.cancel(): Next's fetch may hand back
  // one branch of a tee, whose cancel does not settle until the other branch is read.
  await res.arrayBuffer().catch(() => undefined);
  if (res.status === 401) return refusal("unauthenticated");
  if (res.status === 403) return refusal("forbidden");
  if (res.status === 404) return refusal("missing");
  return refusal("unavailable");
}

/** The export's content and comparison hashes, read from the same document the download serves. */
export async function getExportHashes(id: string): Promise<{ ok: true; hashes: ExportHashes } | ExportRefusal> {
  const got = await fetchCaseExport(id);
  if (!got.ok) return got;
  try {
    const hashes = ExportHashes.safeParse(await got.response.json());
    return hashes.success ? { ok: true, hashes: hashes.data } : refusal("unavailable");
  } catch {
    return refusal("unavailable");
  }
}

/**
 * A download name that is safe in a Content-Disposition header and on any file system: ASCII letters, digits, `.`,
 * `_` and `-` only, no leading dot, bounded length, and labelled as synthetic and redacted.
 */
export function exportFilename(id: string): string {
  const slug =
    id
      .replace(/[^A-Za-z0-9._-]+/g, "_")
      .replace(/\.{2,}/g, ".")
      .replace(/^[._-]+|[._-]+$/g, "")
      .slice(0, 96) || "case";
  return `wasl-synthetic-redacted-export_${slug}.json`;
}
