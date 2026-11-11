import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { signReview } from "@/app/app/review/[id]/actions";
import type { Signature } from "@/components/review/decision";
import type { CaseDetail, Programme } from "@/lib/api/schema";
import { loadSample } from "./samples";

const { revalidateMock, getCaseMock } = vi.hoisted(() => ({ revalidateMock: vi.fn(), getCaseMock: vi.fn() }));
vi.mock("next/cache", () => ({ revalidatePath: revalidateMock }));
vi.mock("@/lib/api/source", async (importOriginal) => ({ ...(await importOriginal<typeof import("@/lib/api/source")>()), getCase: getCaseMock }));

/** Only what the action reads: the programme and the raised items with their kinds. */
function caseWith(programme: Programme, items: Record<string, readonly string[]>): CaseDetail {
  return { case: { programme }, review_items: Object.entries(items).map(([item, kinds]) => ({ item, kinds })) } as unknown as CaseDetail;
}

const meta = { api: "wasl-api/1", data_mode: "simulated", run: "r", policy: "p", reader_model: "m", loaded_at: "2026-11-10T00:00:00Z", note: "synthetic" };
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

function created(caseId: string, signature: Signature, hash = "hash-1") {
  const role = signature.decision === "same" || signature.decision === "different" ? "reviewer" : "clinician";
  return json(201, {
    meta,
    review: {
      case: caseId,
      item: signature.item,
      decision: signature.decision,
      note: "",
      role,
      actor: "bound-api-actor",
      action: role === "reviewer" ? "sign_link" : "sign_evidence",
      at: "2026-11-11T08:00:00Z",
      mode: "demo",
      prev: "genesis",
      hash,
    },
  });
}

beforeEach(() => {
  vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
  vi.stubEnv("WASL_API_REVIEWER_TOKEN", "reviewer-secret-1234567890");
  vi.stubEnv("WASL_API_CLINICIAN_TOKEN", "clinician-secret-1234567890");
});

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
  revalidateMock.mockReset();
  getCaseMock.mockReset();
});

describe("signReview programme evidence", () => {
  it.each([
    ["crc/evidence-only", "crc", { item: "rv-crc-evidence", decision: "adequate" }],
    ["lung_nodule", "lung_nodule", { item: "rv-lung", decision: "pet_low" }],
    ["diabetic_retinopathy", "diabetic_retinopathy", { item: "rv-dr", decision: "R3AM1/urgent" }],
    ["breast", "breast", { item: "rv-breast", decision: "B4" }],
    ["lung_ldct", "lung_ldct", { item: "rv-ldct", decision: "wk_benign" }],
    ["ifhas", "ifhas", { item: "rv-ifhas", decision: "bp=managed;glyc=not_confirmed" }],
  ] as const)("appends a 201 for %s and refreshes the accurate history", async (caseId, programme, signature) => {
    getCaseMock.mockResolvedValue(caseWith(programme, { [signature.item]: ["evidence"] }));
    const fetchMock = vi.fn().mockResolvedValue(created(caseId, signature));
    vi.stubGlobal("fetch", fetchMock);

    const result = await signReview(caseId, [signature]);
    expect(getCaseMock).toHaveBeenCalledWith(caseId);
    expect(result).toEqual({
      saved: [{ item: signature.item, hash: "hash-1", role: "clinician", actor: "bound-api-actor", at: "2026-11-11T08:00:00Z" }],
    });
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe(`http://127.0.0.1:8088/api/cases/${encodeURIComponent(caseId)}/review`);
    expect(new Headers(init.headers).get("authorization")).toBe("Bearer clinician-secret-1234567890");
    expect(JSON.parse(init.body)).toEqual({ ...signature, note: "" });
    expect(revalidateMock).toHaveBeenCalledTimes(4);
    expect(revalidateMock).toHaveBeenCalledWith("/app/patients/[id]", "page");
    expect(revalidateMock).toHaveBeenCalledWith("/app/patients/[id]/audit", "page");
    expect(revalidateMock).toHaveBeenCalledWith(`/app/review/${encodeURIComponent(caseId)}`);
    expect(revalidateMock).toHaveBeenCalledWith("/app/review");
  });

  it.each(["same", "different"])("appends a link-only %s signature as reviewer", async (decision) => {
    getCaseMock.mockResolvedValue(caseWith("lung_nodule", { "rv-link-only": ["link"] }));
    const signature: Signature = { item: "rv-link-only", decision };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(created("lung/link-only", signature)));
    const result = await signReview("lung/link-only", [signature]);
    expect(result.saved).toEqual([{ item: signature.item, hash: "hash-1", role: "reviewer", actor: "bound-api-actor", at: "2026-11-11T08:00:00Z" }]);
  });

  it("appends identity then evidence when the case raised both items", async () => {
    getCaseMock.mockResolvedValue(caseWith("crc", { "rv-link": ["link"], "rv-evidence": ["evidence"] }));
    const signatures: Signature[] = [
      { item: "rv-link", decision: "same" },
      { item: "rv-evidence", decision: "inadequate" },
    ];
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(created("crc/both", signatures[0], "hash-1"))
      .mockResolvedValueOnce(created("crc/both", signatures[1], "hash-2"));
    vi.stubGlobal("fetch", fetchMock);
    const result = await signReview("crc/both", signatures);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(result.saved.map((s) => [s.item, s.role])).toEqual([
      ["rv-link", "reviewer"],
      ["rv-evidence", "clinician"],
    ]);
  });

  it("appends both answers to one item that raised link and evidence together", async () => {
    getCaseMock.mockResolvedValue(caseWith("crc", { "rv-both": ["link", "evidence"] }));
    const signatures: Signature[] = [
      { item: "rv-both", decision: "same" },
      { item: "rv-both", decision: "adequate" },
    ];
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(created("crc/one-item", signatures[0], "hash-1"))
      .mockResolvedValueOnce(created("crc/one-item", signatures[1], "hash-2"));
    vi.stubGlobal("fetch", fetchMock);
    const result = await signReview("crc/one-item", signatures);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(result.failed).toBeUndefined();
  });

  it.each([
    ["another programme's label on an evidence item", "lung_nodule", { "rv-lung": ["evidence"] }, { item: "rv-lung", decision: "B4" }, 400],
    ["a CRC label on a non-CRC evidence item", "breast", { "rv-breast": ["evidence"] }, { item: "rv-breast", decision: "adequate" }, 400],
    ["a programme label on a link item", "lung_nodule", { "rv-link": ["link"] }, { item: "rv-link", decision: "stable" }, 400],
    ["a CRC evidence label on a link item", "crc", { "rv-link": ["link"] }, { item: "rv-link", decision: "adequate" }, 400],
    ["same on an evidence item", "crc", { "rv-evidence": ["evidence"] }, { item: "rv-evidence", decision: "same" }, 400],
    ["different on an evidence item", "breast", { "rv-breast": ["evidence"] }, { item: "rv-breast", decision: "different" }, 400],
    ["an evidence label on an unguided cervical item", "cervical", { "rv-cervical": ["evidence"] }, { item: "rv-cervical", decision: "adequate" }, 400],
    ["an item the case did not raise", "crc", { "rv-link": ["link"] }, { item: "rv-forged", decision: "same" }, 404],
  ] as const)("rejects %s before any request", async (_what, programme, items, signature, status) => {
    getCaseMock.mockResolvedValue(caseWith(programme, items));
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const result = await signReview("case-1", [signature]);
    expect(fetchMock).not.toHaveBeenCalled();
    expect(result.saved).toEqual([]);
    expect(result.failed?.signature).toEqual(signature);
    expect(result.failed?.failure.status).toBe(status);
    expect(revalidateMock).not.toHaveBeenCalled();
  });

  it("rejects the whole pair when its second answer does not fit, so nothing is half-signed", async () => {
    getCaseMock.mockResolvedValue(caseWith("crc", { "rv-link": ["link"], "rv-evidence": ["evidence"] }));
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const result = await signReview("crc/both", [
      { item: "rv-link", decision: "same" },
      { item: "rv-evidence", decision: "different" },
    ]);
    expect(fetchMock).not.toHaveBeenCalled();
    expect(result.failed?.signature).toEqual({ item: "rv-evidence", decision: "different" });
  });

  it("rejects every signature when the case is not found", async () => {
    getCaseMock.mockResolvedValue(null);
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const result = await signReview("missing", [{ item: "rv-link", decision: "same" }]);
    expect(fetchMock).not.toHaveBeenCalled();
    expect(result.failed?.failure).toEqual({ status: 404, error: "case not found" });
  });

  it("returns the API's refusal of a well-formed answer without refreshing history", async () => {
    getCaseMock.mockResolvedValue(caseWith("lung_nodule", { "rv-lung": ["evidence"] }));
    const fetchMock = vi.fn().mockResolvedValue(json(403, { meta, error: "role clinician may not sign_evidence" }));
    vi.stubGlobal("fetch", fetchMock);
    const signature: Signature = { item: "rv-lung", decision: "stable" };
    const result = await signReview("lung_nodule", [signature]);
    expect(fetchMock).toHaveBeenCalledOnce();
    expect(result.saved).toEqual([]);
    expect(result.failed).toEqual({ signature, failure: { status: 403, error: "role clinician may not sign_evidence" } });
    expect(revalidateMock).not.toHaveBeenCalled();
  });

  it("rejects a malformed evidence decision before any request", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const result = await signReview("dr", [{ item: "rv-dr", decision: "R9M0" } as Signature]);
    expect(result.failed?.failure.status).toBe(400);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe("submitReviewRead", async () => {
  const TOKEN = "reviewer-one-token-1234567890";
  const state = (over: Record<string, unknown> = {}) => ({
    item: "rv-1", question: "link", state: "awaiting_r1", head: "", next_read: "r1", may_submit: true,
    reads: [{ read: "r1", submitted: false }], disagreement: false, canonical_unchanged: true, ...over,
  });
  const detail = (states: unknown[]) => {
    const base = loadSample("case_evidence_closure.json") as Record<string, unknown> & { case: Record<string, unknown> };
    return { ...base, case: { ...base.case, programme: "crc" }, review_items: [{ ...(base.review_items as object[])[0], item: "rv-1", kinds: ["link"] }], api_review_states: states };
  };
  const input = { item: "rv-1", question: "link" as const, decision: "same", expectedHead: "" };
  const cookieStore = { get: () => ({ value: TOKEN }), set: vi.fn(), delete: vi.fn() };

  beforeEach(() => {
    vi.resetModules();
  });

  async function load() {
    vi.doMock("next/headers", () => ({ cookies: async () => cookieStore }));
    vi.doMock("next/cache", () => ({ revalidatePath: revalidateMock }));
    return (await import("@/app/app/review/[id]/actions")).submitReviewRead;
  }
  const reviewBody = { meta, review: { case: "c1", item: "rv-1", decision: "same", note: "", role: "reviewer", actor: "bound", action: "sign_link", at: "2026-11-12T00:00:00Z", mode: "demo", prev: "", hash: "h1" }, api_review_states: [state({ state: "awaiting_r2", head: "h1" })] };

  it("reads state then appends exactly one read with the same raw bearer token and no identity headers", async () => {
    const submit = await load();
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(json(200, detail([state()])))
      .mockResolvedValueOnce(json(201, reviewBody));
    vi.stubGlobal("fetch", fetchMock);
    const result = await submit("c1", input);
    expect(result).toMatchObject({ ok: true, hash: "h1", read: "r1" });
    expect(JSON.stringify(result)).not.toContain(TOKEN);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    const [get, post] = fetchMock.mock.calls;
    expect(get[1].headers.get("authorization")).toBe(`Bearer ${TOKEN}`);
    expect(post[1].headers.get("authorization")).toBe(`Bearer ${TOKEN}`);
    expect(post[1].method).toBe("POST");
    for (const call of fetchMock.mock.calls) {
      expect(call[1].headers.has("x-wasl-role")).toBe(false);
      expect(call[1].headers.has("x-wasl-actor")).toBe(false);
    }
    expect(JSON.parse(post[1].body)).toEqual({ item: "rv-1", decision: "same", note: "", read: "r1", expected_head: "" });
    expect(revalidateMock).toHaveBeenCalledWith("/app/review/c1");
  });

  it("never calls the legacy postReview path", async () => {
    const submit = await load();
    const fetchMock = vi.fn().mockResolvedValueOnce(json(200, detail([state()]))).mockResolvedValueOnce(json(201, reviewBody));
    vi.stubGlobal("fetch", fetchMock);
    await submit("c1", input);
    const body = JSON.parse(fetchMock.mock.calls[1][1].body);
    expect(body.read).toBe("r1");
    expect(body.expected_head).toBe("");
  });

  it("refuses without posting when may_submit is false, the question mismatches or the label is wrong", async () => {
    const submit = await load();
    const fetchMock = vi.fn().mockImplementation(async () => json(200, detail([state({ may_submit: false })])));
    vi.stubGlobal("fetch", fetchMock);
    expect(await submit("c1", input)).toMatchObject({ ok: false, failure: { status: 403 } });
    expect(await submit("c1", { ...input, question: "evidence", decision: "same" })).toMatchObject({ ok: false, failure: { status: 400 } });
    expect(await submit("c1", { ...input, decision: "adequate" })).toMatchObject({ ok: false, failure: { status: 400 } });
    expect(fetchMock.mock.calls.every((c) => c[1].method !== "POST")).toBe(true);
  });

  it("refuses a stale expected head without posting", async () => {
    const submit = await load();
    const fetchMock = vi.fn().mockResolvedValue(json(200, detail([state({ head: "newer" })])));
    vi.stubGlobal("fetch", fetchMock);
    expect(await submit("c1", input)).toMatchObject({ ok: false, failure: { status: 409 } });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it.each([400, 403, 404])("returns a %i refusal without a success claim", async (status) => {
    const submit = await load();
    vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(json(200, detail([state()]))).mockResolvedValueOnce(json(status, { meta, error: "refused" })));
    const result = await submit("c1", input);
    expect(result).toMatchObject({ ok: false, failure: { status, error: "refused" } });
    expect(revalidateMock).not.toHaveBeenCalled();
  });

  it("on 409 revalidates, returns current server truth and does not retry", async () => {
    const submit = await load();
    const fresh = state({ state: "awaiting_r2", head: "h9", next_read: "r2" });
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(json(200, detail([state()])))
      .mockResolvedValueOnce(json(409, { meta, error: "stale head", current_head: "h9" }))
      .mockResolvedValueOnce(json(200, detail([fresh])));
    vi.stubGlobal("fetch", fetchMock);
    const result = await submit("c1", input);
    expect(result).toMatchObject({ ok: false, failure: { status: 409, error: "stale head" }, states: [{ head: "h9" }] });
    expect(fetchMock.mock.calls.filter((c) => c[1].method === "POST")).toHaveLength(1);
    expect(revalidateMock).toHaveBeenCalledWith("/app/review/c1");
  });

  it("returns 401 without any request when signed out", async () => {
    const submit = await load();
    cookieStore.get = () => undefined as never;
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    expect(await submit("c1", input)).toMatchObject({ ok: false, failure: { status: 401 } });
    expect(fetchMock).not.toHaveBeenCalled();
    cookieStore.get = () => ({ value: TOKEN });
  });
});
