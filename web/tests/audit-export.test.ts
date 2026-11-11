import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import path from "node:path";
import { NextRequest } from "next/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { GET } from "@/app/app/patients/[id]/audit/export/route";
import { EXPORT_REFUSAL, exportFilename, getExportHashes } from "@/lib/api/export";

/*
 * Issue #46. fixtures/export_redacted.json is the exact byte output of the Go handler's secured
 * GET /api/cases/{id}/export for an auditor, on a synthetic case with clinical canaries planted in every path
 * (pipeline/api_export_contract_test.go, TestExportContractFixture; regenerate with WASL_WRITE_EXPORT_FIXTURE=1).
 * The same Go test proves reviewer and coordinator tokens get 403 and that a restarted load compares identically.
 */
const FIXTURE = readFileSync(path.join(__dirname, "fixtures", "export_redacted.json"));
const CASE = "exp-prov/ServiceRequest/EXP-sr-1";
const BASE = "http://127.0.0.1:8088";
const SAFE_DISPOSITION = /^attachment; filename="wasl-synthetic-redacted-export_[A-Za-z0-9._-]+\.json"$/;

/** Go's encoding/json canonical form (docs/API-EXPORT.md): sorted keys, no whitespace, HTML characters escaped. */
function goCanon(v: unknown): string {
  if (Array.isArray(v)) return `[${v.map(goCanon).join(",")}]`;
  if (v && typeof v === "object") {
    const o = v as Record<string, unknown>;
    return `{${Object.keys(o)
      .sort()
      .map((k) => `${goCanon(k)}:${goCanon(o[k])}`)
      .join(",")}}`;
  }
  if (typeof v === "string") {
    return JSON.stringify(v).replace(/[<>&\u2028\u2029]/g, (c) => `\\u${c.charCodeAt(0).toString(16).padStart(4, "0")}`);
  }
  return JSON.stringify(v);
}
const sha256 = (s: string) => createHash("sha256").update(s).digest("hex");

/** Recomputes both documented hashes from downloaded bytes. */
function verifyHashes(bytes: Uint8Array) {
  const doc = JSON.parse(Buffer.from(bytes).toString("utf8"));
  const { content_sha256, ...content } = doc;
  const { comparison_sha256, ...rest } = content;
  const meta = { ...rest.meta };
  delete meta.loaded_at;
  return {
    content: sha256(goCanon(content)) === content_sha256,
    comparison: sha256(goCanon({ ...rest, meta })) === comparison_sha256,
  };
}

/** The planted clinical canaries, raw and encoded, as in pipeline/api_export_contract_test.go. */
const CANARIES = ["CANARY-RAW-CLINICAL-TEXT", "CANARY-RAW-CLINICAL-TEXT caecum reached and prep was good", "caecum reached", "prep was good", "1976-01-01"].flatMap((s) => [
  s,
  Buffer.from(s).toString("base64"),
  Buffer.from(s).toString("base64").replace(/=+$/, ""),
  Buffer.from(s).toString("base64url"),
  Buffer.from(s).toString("hex"),
  encodeURIComponent(s),
]);

function live(fetchMock: ReturnType<typeof vi.fn>) {
  vi.stubEnv("WASL_API_URL", BASE);
  vi.stubEnv("WASL_API_AUDITOR_TOKEN", "auditor-secret-1234567890");
  vi.stubEnv("WASL_API_REVIEWER_TOKEN", "reviewer-secret-1234567890");
  vi.stubGlobal("fetch", fetchMock);
}

const download = (id: string) =>
  GET(new NextRequest(`http://localhost/app/patients/${encodeURIComponent(id)}/audit/export`), {
    params: Promise.resolve({ id: encodeURIComponent(id) }),
  });

const backend = (status: number, body: BodyInit, type = "application/json") => new Response(body, { status, headers: { "content-type": type } });

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("the committed Go export fixture", () => {
  it("verifies both documented hashes and carries none of the planted canaries", () => {
    expect(verifyHashes(FIXTURE)).toEqual({ content: true, comparison: true });
    const text = FIXTURE.toString("utf8");
    for (const c of CANARIES) expect(text).not.toContain(c);
    const doc = JSON.parse(text);
    expect([doc.schema, doc.redacted, doc.meta.data_mode]).toEqual(["wasl-case-export/1", true, "simulated"]);
  });
});

describe("GET /app/patients/[id]/audit/export", () => {
  it("streams the backend bytes unchanged with the auditor token and a safe, labelled filename", async () => {
    const fetchMock = vi.fn().mockResolvedValue(backend(200, new Uint8Array(FIXTURE)));
    live(fetchMock);

    const res = await download(CASE);
    expect(res.status).toBe(200);
    const got = new Uint8Array(await res.arrayBuffer());
    expect(Buffer.from(got).equals(FIXTURE)).toBe(true);
    expect(verifyHashes(got)).toEqual({ content: true, comparison: true });

    expect(res.headers.get("content-disposition")).toMatch(SAFE_DISPOSITION);
    expect(res.headers.get("content-disposition")).toBe('attachment; filename="wasl-synthetic-redacted-export_exp-prov_ServiceRequest_EXP-sr-1.json"');
    expect(res.headers.get("content-type")).toBe("application/json; charset=utf-8");
    expect(res.headers.get("cache-control")).toBe("no-store");

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe(`${BASE}/api/cases/${encodeURIComponent(CASE)}/export`);
    expect(init.headers.get("authorization")).toBe("Bearer auditor-secret-1234567890");
    expect(init.headers.has("x-wasl-role")).toBe(false);
    expect(init.headers.has("x-wasl-actor")).toBe(false);
  });

  it.each([
    [401, "unauthenticated"],
    [403, "forbidden"],
    [404, "missing"],
    [405, "unavailable"],
    [500, "unavailable"],
  ] as const)("answers a backend %i with fixed wording and none of the backend's body or metadata", async (status, reason) => {
    const leaky = JSON.stringify({ error: "rbac: CANARY-ERROR", id: CASE, hint: "CANARY-HINT", meta: { run: "CANARY-META-RUN", policy: "CANARY-POLICY" } });
    live(vi.fn().mockResolvedValue(backend(status, leaky)));

    const res = await download(CASE);
    expect(res.status).toBe(EXPORT_REFUSAL[reason].status);
    expect(res.headers.get("content-disposition")).toBeNull();
    const text = await res.text();
    expect(JSON.parse(text)).toEqual({ error: EXPORT_REFUSAL[reason].message });
    expect(text).not.toMatch(/CANARY|meta|rbac|auditor-secret/);
  });

  it("settles a refusal whose body is one branch of a tee, as Next's patched fetch can return", async () => {
    const [branch] = new Response(JSON.stringify({ error: "rbac", meta: { run: "CANARY" } })).body!.tee();
    live(vi.fn().mockResolvedValue(new Response(branch, { status: 403, headers: { "content-type": "application/json" } })));
    const res = await download(CASE);
    expect(res.status).toBe(403);
    expect(await res.json()).toEqual({ error: EXPORT_REFUSAL.forbidden.message });
  }, 2000);

  it("refuses a 200 that is not JSON rather than passing it on", async () => {
    live(vi.fn().mockResolvedValue(backend(200, "<html>proxy login</html>", "text/html")));
    const res = await download(CASE);
    expect(res.status).toBe(502);
    expect(await res.json()).toEqual({ error: EXPORT_REFUSAL.unavailable.message });
  });

  it("refuses when the API cannot be reached", async () => {
    live(vi.fn().mockRejectedValue(new Error("connect ECONNREFUSED 127.0.0.1:8088")));
    const res = await download(CASE);
    expect(res.status).toBe(502);
    expect(await res.text()).not.toContain("ECONNREFUSED");
  });

  it("in fixtures mode serves no file and never calls out", async () => {
    vi.stubEnv("WASL_API_URL", "");
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const res = await download(CASE);
    expect(res.status).toBe(404);
    expect(await res.json()).toEqual({ error: EXPORT_REFUSAL.fixtures.message });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("without an auditor credential serves no file and does not name the variable", async () => {
    vi.stubEnv("WASL_API_URL", BASE);
    vi.stubEnv("WASL_API_AUDITOR_TOKEN", "");
    vi.stubEnv("WASL_API_REVIEWER_TOKEN", "reviewer-secret-1234567890");
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const res = await download(CASE);
    expect(res.status).toBe(503);
    const text = await res.text();
    expect(text).not.toMatch(/WASL_API|TOKEN|reviewer-secret/);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("rejects a malformed or oversized id before calling the API", async () => {
    const fetchMock = vi.fn();
    live(fetchMock);
    const bad = await GET(new NextRequest("http://localhost/app/patients/%E0%A4%A/audit/export"), { params: Promise.resolve({ id: "%E0%A4%A" }) });
    expect(bad.status).toBe(400);
    expect((await download("x".repeat(201))).status).toBe(400);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("refuses a bare dot segment, which would point the auditor token at another API route", async () => {
    const fetchMock = vi.fn();
    live(fetchMock);
    // Next hands the route an already-decoded param, so %252E%252E in the URL arrives as "%2E%2E" and decodes to "..".
    for (const param of ["%2E%2E", "%2E", "..", "."]) {
      const res = await GET(new NextRequest(`http://localhost/app/patients/${param}/audit/export`), { params: Promise.resolve({ id: param }) });
      expect(res.status).toBe(400);
    }
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe("export filename", () => {
  it.each([
    "ahs-mushrif/ServiceRequest/AHS-sr-1002",
    "../../etc/passwd",
    '..\\a"b\r\nSet-Cookie: x=1',
    "مريض/١٢٣",
    ".hidden",
    "x".repeat(500),
  ])("is header- and file-system-safe for %j", (id) => {
    const name = exportFilename(id);
    expect(`attachment; filename="${name}"`).toMatch(SAFE_DISPOSITION);
    expect(name).not.toMatch(/\.\.|[/\\"\r\n]/);
    expect(name.length).toBeLessThanOrEqual(140);
    expect(name).toContain("synthetic-redacted");
  });
});

describe("export hashes on the audit page", () => {
  it("are read from the backend document and nothing else is kept", async () => {
    live(vi.fn().mockResolvedValue(backend(200, new Uint8Array(FIXTURE))));
    const got = await getExportHashes(CASE);
    const doc = JSON.parse(FIXTURE.toString("utf8"));
    expect(got).toEqual({
      ok: true,
      hashes: { schema: "wasl-case-export/1", redacted: true, content_sha256: doc.content_sha256, comparison_sha256: doc.comparison_sha256 },
    });
  });

  it("are not shown for a document without the export schema", async () => {
    live(vi.fn().mockResolvedValue(backend(200, JSON.stringify({ content_sha256: "0".repeat(64), comparison_sha256: "0".repeat(64) }))));
    expect(await getExportHashes(CASE)).toMatchObject({ ok: false, reason: "unavailable" });
  });
});
