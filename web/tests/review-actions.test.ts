import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { loadSample } from "./samples";

const { revalidateMock } = vi.hoisted(() => ({ revalidateMock: vi.fn() }));
vi.mock("next/cache", () => ({ revalidatePath: revalidateMock }));

const meta = { api: "wasl-api/1", data_mode: "simulated", run: "r", policy: "p", reader_model: "m", loaded_at: "2026-11-10T00:00:00Z", note: "synthetic" };
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

beforeEach(() => {
  vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
  vi.stubEnv("WASL_API_REVIEWER_TOKEN", "reviewer-secret-1234567890");
  vi.stubEnv("WASL_API_CLINICIAN_TOKEN", "clinician-secret-1234567890");
});

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
  revalidateMock.mockReset();
});

describe("submitReviewRead", async () => {
  const TOKEN = "reviewer-one-token-1234567890";
  const state = (over: Record<string, unknown> = {}) => ({
    item: "rv-1", question: "link", state: "awaiting_r1", head: "", next_read: "r1", may_submit: true,
    claim_head: "c".repeat(64), claim: { owner: "Rev-A", role: "reviewer", read: "r1", claimed_at: "2026-11-13T10:00:00Z", heartbeat_at: "2026-11-13T10:00:00Z", expires_at: "2026-11-13T10:05:00Z", status: "active" },
    claim_actions: { claim: false, renew: true, release: true, takeover: false },
    reads: [{ read: "r1", submitted: false }], disagreement: false, canonical_unchanged: true, ...over,
  });
  const detail = (states: unknown[]) => {
    const base = loadSample("case_evidence_closure.json") as Record<string, unknown> & { case: Record<string, unknown> };
    return { ...base, case: { ...base.case, programme: "crc" }, review_items: [{ ...(base.review_items as object[])[0], item: "rv-1", kinds: ["link"] }], api_review_states: states };
  };
  const input = { item: "rv-1", question: "link" as const, decision: "same", expectedHead: "", claimHead: "c".repeat(64) };
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
    expect(JSON.parse(post[1].body)).toEqual({ item: "rv-1", decision: "same", note: "", read: "r1", expected_head: "", claim_head: "c".repeat(64) });
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

  it("after a committed 201 with an unreadable body, refreshes, re-reads, claims no hash and does not retry", async () => {
    const submit = await load();
    const fresh = state({ state: "awaiting_r2", head: "h1", next_read: "r2", may_submit: false });
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(json(200, detail([state()])))
      .mockResolvedValueOnce(json(201, { meta, review: { not: "an event" } }))
      .mockResolvedValueOnce(json(200, detail([fresh])));
    vi.stubGlobal("fetch", fetchMock);
    const result = await submit("c1", input);
    expect(result).toMatchObject({ ok: true, committed: true, read: "r1", states: [{ head: "h1" }] });
    expect(result).not.toHaveProperty("hash", expect.anything());
    expect(fetchMock.mock.calls.filter((c) => c[1].method === "POST")).toHaveLength(1);
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(revalidateMock).toHaveBeenCalledWith("/app/review/c1");
  });

  it("refuses a read locally without an active owned claim", async () => {
    const submit = await load();
    const fetchMock = vi.fn().mockResolvedValue(json(200, detail([state({ claim: null })])));
    vi.stubGlobal("fetch", fetchMock);
    expect(await submit("c1", input)).toMatchObject({ ok: false, failure: { status: 409 } });
    expect(fetchMock.mock.calls.filter((c) => c[1].method === "POST")).toHaveLength(0);
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

describe("claimReviewQuestion", () => {
  const TOKEN = "reviewer-claim-token-1234567890";
  const cookieStore = { get: () => ({ value: TOKEN }) };
  const claimEvent = (operation: string) => ({
    type: "review_claim", kind: operation, case: "c1", item: "rv-1", question: "link", read: "r1", role: "reviewer", actor: "Rev-A",
    action: "signoff_link", at: "2026-11-13T10:00:00Z", claimed_at: "2026-11-13T10:00:00Z", heartbeat_at: "2026-11-13T10:00:00Z",
    expires_at: "2026-11-13T10:05:00Z", expected_claim_head: "", mode: "demo", prev: "p", hash: "h-claim",
  });
  const response = (operation: string) => ({ meta, claim_event: claimEvent(operation), review_state: undefined, api_review_states: [], note: "appended" });

  beforeEach(() => vi.resetModules());
  async function load() {
    vi.doMock("next/headers", () => ({ cookies: async () => cookieStore }));
    vi.doMock("next/cache", () => ({ revalidatePath: revalidateMock }));
    return (await import("@/app/app/review/[id]/actions")).claimReviewQuestion;
  }

  it.each(["claim", "renew", "release", "takeover"] as const)("posts one %s with only the bearer credential and contract fields", async (operation) => {
    const claim = await load();
    const fetchMock = vi.fn().mockResolvedValue(json(201, response(operation)));
    vi.stubGlobal("fetch", fetchMock);
    const result = await claim("c1", { item: "rv-1", question: "link", operation, expectedClaimHead: "" });
    expect(result).toMatchObject({ ok: true, states: [] });
    expect(JSON.stringify(result)).not.toContain(TOKEN);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("http://127.0.0.1:8088/api/cases/c1/review/claim");
    expect(init.method).toBe("POST");
    expect(init.headers.get("authorization")).toBe(`Bearer ${TOKEN}`);
    expect(init.headers.has("x-wasl-role")).toBe(false);
    expect(init.headers.has("x-wasl-actor")).toBe(false);
    expect(JSON.parse(init.body)).toEqual({ item: "rv-1", question: "link", operation, expected_claim_head: "" });
  });

  it("on 409 refreshes and re-reads once without retrying", async () => {
    const claim = await load();
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(json(409, { meta, error: "claim active", conflict: "claim_active", current_claim_head: "new" }))
      .mockResolvedValueOnce(json(200, { ...(loadSample("case_evidence_closure.json") as Record<string, unknown>), api_review_states: [] }));
    vi.stubGlobal("fetch", fetchMock);
    const result = await claim("c1", { item: "rv-1", question: "link", operation: "claim", expectedClaimHead: "" });
    expect(result).toMatchObject({ ok: false, failure: { status: 409, error: "claim active" }, states: [] });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls.filter((c) => c[1].method === "POST")).toHaveLength(1);
  });

  it("reports a committed unreadable 201, refreshes and never retries", async () => {
    const claim = await load();
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(json(201, { meta, claim_event: null }))
      .mockResolvedValueOnce(json(200, { ...(loadSample("case_evidence_closure.json") as Record<string, unknown>), api_review_states: [] }));
    vi.stubGlobal("fetch", fetchMock);
    expect(await claim("c1", { item: "rv-1", question: "link", operation: "claim", expectedClaimHead: "" })).toMatchObject({ ok: true, committed: true, states: [] });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls.filter((c) => c[1].method === "POST")).toHaveLength(1);
  });
});
