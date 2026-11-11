import { readFileSync } from "node:fs";
import path from "node:path";
import { NextRequest } from "next/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { GET } from "@/app/app/patients/[id]/audit/export/route";
import { exportConcealsInitialRead } from "@/lib/api/export-guard";

const FIXTURE = readFileSync(path.join(__dirname, "fixtures", "export_redacted.json"));
const CASE = "exp-prov/ServiceRequest/EXP-sr-1";
const CANARY_HASH = "r1CANARYHASH0123456789abcdef";

const ev = (read: string, extra: Record<string, unknown> = {}) => ({
  case: CASE, item: "rv-1", decision: read === "r1" ? "CANARY-DECISION" : "same", role: "reviewer", action: "signoff_link", at: "t", mode: "api",
  prev: "p", hash: read === "r1" ? CANARY_HASH : `h-${read}`, protocol: "wasl-independent-review/1", read, question: "link", ...extra,
});
const doc = (history: unknown[], api_reviews: unknown[] = history) => {
  const base = JSON.parse(FIXTURE.toString("utf8"));
  base.audit.history = history;
  base.detail.api_reviews = api_reviews;
  return base;
};
const bytes = (d: unknown) => new TextEncoder().encode(JSON.stringify(d, null, 1));

function live(body: BodyInit) {
  vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
  vi.stubEnv("WASL_API_AUDITOR_TOKEN", "auditor-secret-1234567890");
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(body, { status: 200, headers: { "content-type": "application/json" } })));
}
const download = () =>
  GET(new NextRequest("http://localhost/app/patients/x/audit/export"), { params: Promise.resolve({ id: encodeURIComponent(CASE) }) });

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("exportConcealsInitialRead", () => {
  it("is true for r1 without r2 in audit.history or detail.api_reviews", () => {
    expect(exportConcealsInitialRead(doc([ev("r1")], []))).toBe(true);
    expect(exportConcealsInitialRead(doc([], [ev("r1")]))).toBe(true);
  });
  it("groups by item and question: another question's r2 does not release r1", () => {
    expect(exportConcealsInitialRead(doc([ev("r1"), ev("r2", { question: "evidence" })]))).toBe(true);
    expect(exportConcealsInitialRead(doc([ev("r1"), ev("r2", { item: "rv-2" })]))).toBe(true);
  });
  it("is false before r1, and once r2 or r3 exists", () => {
    expect(exportConcealsInitialRead(doc([]))).toBe(false);
    expect(exportConcealsInitialRead(doc([ev("r1"), ev("r2")]))).toBe(false);
    expect(exportConcealsInitialRead(doc([ev("r1"), ev("r2"), ev("r3")]))).toBe(false);
  });
  it("ignores legacy events", () => {
    expect(exportConcealsInitialRead(doc([{ ...ev("r1"), protocol: undefined, read: undefined, question: undefined }]))).toBe(false);
  });
  it("fails closed on an unreadable document", () => {
    expect(exportConcealsInitialRead(null)).toBe(true);
    expect(exportConcealsInitialRead({ audit: { history: "x" }, detail: {} })).toBe(true);
  });
});

describe("export route blinding", () => {
  it("refuses a concealed r1 with a fixed 409 and none of its decision, hash or metadata", async () => {
    live(bytes(doc([ev("r1")])));
    const res = await download();
    expect(res.status).toBe(409);
    expect(res.headers.get("content-disposition")).toBeNull();
    const text = await res.text();
    for (const leak of ["CANARY-DECISION", CANARY_HASH, "content_sha256", "meta", "rv-1"]) expect(text).not.toContain(leak);
  });

  it("refuses malformed JSON without streaming it", async () => {
    live("{ not json CANARY-DECISION");
    const res = await download();
    expect(res.status).toBe(409);
    expect(await res.text()).not.toContain("CANARY");
  });

  it("allows an export that predates r1, byte for byte", async () => {
    const b = bytes(doc([]));
    live(b);
    const res = await download();
    expect(res.status).toBe(200);
    expect(Buffer.from(await res.arrayBuffer()).equals(Buffer.from(b))).toBe(true);
  });

  it("allows r1 plus r2, byte for byte", async () => {
    const b = bytes(doc([ev("r1"), ev("r2")]));
    live(b);
    const res = await download();
    expect(res.status).toBe(200);
    expect(Buffer.from(await res.arrayBuffer()).equals(Buffer.from(b))).toBe(true);
  });
});
