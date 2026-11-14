import { readFileSync } from "node:fs";
import path from "node:path";
import { NextRequest } from "next/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Audit, ReviewState } from "@/lib/api/schema";

const m = vi.hoisted(() => ({ getReviewCase: vi.fn(), getAudit: vi.fn(), getUndoProjection: vi.fn(), getCase: vi.fn() }));
vi.mock("@/lib/api/reviewer-session", () => ({ getReviewCase: m.getReviewCase }));
vi.mock("@/lib/api/source", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/source")>()),
  apiMode: () => "live",
  getAudit: m.getAudit,
  getUndoProjection: m.getUndoProjection,
  getCase: m.getCase,
}));

import { GET } from "@/app/app/patients/[id]/audit/export/route";
import { gatedHistory } from "@/lib/api/protocol-history";

const CASE = "exp-prov/ServiceRequest/EXP-sr-1";
const VERDICT = "MODEL-VERDICT-CANARY";
const MODEL_HASH = "modelchainhash0123456789abcdef";
const FIXTURE = JSON.parse(readFileSync(path.join(__dirname, "fixtures", "export_redacted.json"), "utf8"));

const evidenceBody = { report: "synthetic", image_reader: { model: VERDICT, caecal_photo: "verified", prep: "adequate", hint: "" } };
const modelBlock = { type: "evidence", body: evidenceBody, prev: "p0", hash: MODEL_HASH };
const reviewItem = (item: string) => ({ item, loop: "l", kinds: ["evidence"], pair: [], p: 1, report: "r", verdict: "v", chain: [modelBlock] });

const state = (item: string, extra: Partial<ReviewState> = {}): ReviewState =>
  ({ item, question: "evidence", state: "awaiting_r1", head: "", next_read: "r1", may_submit: true, reads: [], disagreement: false, canonical_unchanged: true, ...extra }) as ReviewState;
const ownRead = { read: "r1", submitted: true, decision: "adequate", hash: "a" };
const detailOf = (items: string[], states: ReviewState[]) => ({ review_items: items.map(reviewItem), api_review_states: states });
const viewer = (detail: unknown) => ({ authenticated: true as const, detail }) as never;
const signedOut = { authenticated: false as const, detail: null };

const modelAudit = (history: unknown[] = []) =>
  ({ case: CASE, chain: [modelBlock], chain_length: 1, chain_verified: true, decision_signatures: [], anchors: {}, history }) as unknown as Audit;
const modelFree = () => ({ ...modelAudit(), chain: [{ type: "evidence", body: { report: "synthetic" }, prev: "p0", hash: "safehash" }] }) as unknown as Audit;

beforeEach(() => {
  vi.resetAllMocks();
  m.getUndoProjection.mockResolvedValue({ events: [] });
});
afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("gatedHistory withholds an unread model verdict (page, RSC props)", () => {
  it("history=[] and no human reads: chain, hashes and verdict never leave", async () => {
    m.getAudit.mockResolvedValue(modelAudit([]));
    m.getReviewCase.mockResolvedValue(viewer(detailOf(["rv-1"], [state("rv-1")])));
    const out = await gatedHistory("c", { needAudit: true });
    expect(out.modelWithheld).toBe(true);
    const wire = JSON.stringify(out);
    for (const leak of [VERDICT, MODEL_HASH, "image_reader"]) expect(wire).not.toContain(leak);
  });

  it("another question's or another item's read does not reveal it", async () => {
    m.getAudit.mockResolvedValue(modelAudit());
    const other = state("rv-2", { question: "link" as never, state: "awaiting_r2", reads: [ownRead as never] });
    const otherItem = state("rv-9", { reads: [ownRead as never] });
    m.getReviewCase.mockResolvedValue(viewer(detailOf(["rv-1"], [state("rv-1"), other, otherItem])));
    expect((await gatedHistory("c", { needAudit: true })).modelWithheld).toBe(true);
  });

  it("a frozen/old-source read does not reveal it", async () => {
    m.getAudit.mockResolvedValue(modelAudit());
    m.getReviewCase.mockResolvedValue(viewer(detailOf(["rv-1"], [state("rv-1", { frozen: true, reads: [ownRead as never] })])));
    expect((await gatedHistory("c", { needAudit: true })).modelWithheld).toBe(true);
  });

  it("missing contracts (no states, no items) and signed-out viewers are withheld, not guessed", async () => {
    m.getAudit.mockResolvedValue(modelAudit());
    m.getReviewCase.mockResolvedValue(viewer({}));
    expect((await gatedHistory("c", { needAudit: true })).modelWithheld).toBe(true);
    m.getReviewCase.mockResolvedValue(signedOut);
    const out = await gatedHistory("c", { needAudit: true });
    expect(out.modelWithheld).toBe(true);
    expect(JSON.stringify(out)).not.toContain(VERDICT);
  });

  it("state change during the privileged fetch: eligible before, ineligible after, is withheld", async () => {
    m.getAudit.mockResolvedValue(modelAudit());
    const ok = viewer(detailOf(["rv-1"], [state("rv-1", { reads: [ownRead as never] })]));
    const bad = viewer(detailOf(["rv-1"], [state("rv-1", { frozen: true, reads: [ownRead as never] })]));
    m.getReviewCase.mockResolvedValueOnce(ok).mockResolvedValueOnce(bad);
    expect((await gatedHistory("c", { needAudit: true })).modelWithheld).toBe(true);
    m.getReviewCase.mockResolvedValueOnce(bad).mockResolvedValueOnce(ok);
    expect((await gatedHistory("c", { needAudit: true })).modelWithheld).toBe(true);
  });

  it("the viewer's own visible read of every evidence question releases it unchanged", async () => {
    const a = modelAudit();
    m.getAudit.mockResolvedValue(a);
    m.getReviewCase.mockResolvedValue(viewer(detailOf(["rv-1"], [state("rv-1", { state: "resolved_agreed", reads: [ownRead as never] })])));
    const out = await gatedHistory("c", { needAudit: true });
    expect(out.modelWithheld).toBeUndefined();
    expect(out.audit).toBe(a);
  });

  it("a concealed human read is still withheld even when the model would be eligible", async () => {
    m.getAudit.mockResolvedValue(modelAudit());
    const concealed = state("rv-1", { state: "awaiting_r2", reads: [ownRead as never, { read: "r1", submitted: true, concealed: true } as never] });
    m.getReviewCase.mockResolvedValue(viewer(detailOf(["rv-1"], [concealed])));
    expect((await gatedHistory("c", { needAudit: true })).status).toBe("concealed");
  });

  it("a model-free audit is unchanged", async () => {
    const a = modelFree();
    m.getAudit.mockResolvedValue(a);
    m.getReviewCase.mockResolvedValue(signedOut);
    const out = await gatedHistory("c", { needAudit: true });
    expect(out.modelWithheld).toBeUndefined();
    expect(out.audit).toBe(a);
  });
});

describe("export route (download boundary)", () => {
  const withModel = () => {
    const d = structuredClone(FIXTURE);
    d.audit.chain = [modelBlock];
    d.detail.evidence = [evidenceBody];
    d.detail.review_items = [reviewItem("rv-1")];
    return d;
  };
  const bytesOf = (d: unknown) => new TextEncoder().encode(JSON.stringify(d, null, 1));
  function live(b: Uint8Array) {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_AUDITOR_TOKEN", "auditor-secret-1234567890");
    vi.stubGlobal("fetch", vi.fn().mockImplementation(async () => new Response(b, { status: 200, headers: { "content-type": "application/json" } })));
  }
  const download = () => GET(new NextRequest("http://localhost/app/patients/x/audit/export"), { params: Promise.resolve({ id: encodeURIComponent(CASE) }) });

  it("refuses model bytes for an unread viewer, a failed session and a thrown session, with none of the content", async () => {
    live(bytesOf(withModel()));
    for (const v of [viewer(detailOf(["rv-1"], [state("rv-1")])), signedOut, viewer({})]) {
      m.getReviewCase.mockResolvedValueOnce(v);
      const res = await download();
      expect(res.status).toBe(409);
      expect(res.headers.get("content-disposition")).toBeNull();
      const text = await res.text();
      for (const leak of [VERDICT, MODEL_HASH, "content_sha256"]) expect(text).not.toContain(leak);
    }
    m.getReviewCase.mockRejectedValueOnce(new Error("boom"));
    expect((await download()).status).toBe(409);
  });

  it("releases model bytes byte for byte to a viewer with a visible read of every evidence question", async () => {
    const b = bytesOf(withModel());
    live(b);
    m.getReviewCase.mockResolvedValue(viewer(detailOf(["rv-1"], [state("rv-1", { state: "awaiting_r2", reads: [ownRead as never] })])));
    const res = await download();
    expect(res.status).toBe(200);
    expect(Buffer.from(await res.arrayBuffer()).equals(Buffer.from(b))).toBe(true);
  });

  it("a model-free export needs no session and stays byte for byte", async () => {
    const d = structuredClone(FIXTURE);
    d.audit.history = [];
    d.detail.api_reviews = [];
    const b = bytesOf(d);
    live(b);
    const res = await download();
    expect(res.status).toBe(200);
    expect(m.getReviewCase).not.toHaveBeenCalled();
    expect(Buffer.from(await res.arrayBuffer()).equals(Buffer.from(b))).toBe(true);
  });

  it("the existing concealed-r1 refusal still wins over an eligible model viewer", async () => {
    const d = withModel();
    d.audit.history = [{ item: "rv-1", question: "evidence", protocol: "wasl-independent-review/1", read: "r1" }];
    d.detail.api_reviews = d.audit.history;
    live(bytesOf(d));
    m.getReviewCase.mockResolvedValue(viewer(detailOf(["rv-1"], [state("rv-1", { reads: [ownRead as never] })])));
    expect((await download()).status).toBe(409);
  });
});
