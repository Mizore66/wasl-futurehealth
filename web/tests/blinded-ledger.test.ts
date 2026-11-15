import { readFileSync } from "node:fs";
import path from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CaseDetail, ConsentAppendResponse, ConsentResponse } from "@/lib/api/schema";
import { postConsent, postContestWithToken, postUndoWithToken } from "@/lib/api/source";

/*
 * fixtures/ledger_blinded_*.json are the handler's own responses to a non-author while a first independent read awaits
 * its second (pipeline/api_blinded_fixture_test.go, WASL_WRITE_LEDGER_FIXTURES=1). The server omits every field that
 * would repeat the concealed hash, so the schemas must accept the redacted shape.
 */
const load = (f: string) => JSON.parse(readFileSync(path.join(__dirname, "fixtures", f), "utf8"));
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

beforeEach(() => {
  vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
  vi.stubEnv("WASL_API_COORDINATOR_TOKEN", "server-coordinator-token-1234567890");
});
afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("reads while the first independent read is concealed", () => {
  it("parses case detail with consent history chained on the concealed read", () => {
    const raw = load("ledger_blinded_case_detail.json");
    expect(raw.api_contact.history[0]).not.toHaveProperty("prev");
    const detail = CaseDetail.parse(raw);
    expect(detail.api_contact?.history).toHaveLength(1);
    expect(detail.api_contact?.history[0].prev).toBeUndefined();
  });

  it("parses the consent history response", () => {
    const parsed = ConsentResponse.parse(load("ledger_blinded_consent.json"));
    expect(parsed.history[0].prev).toBeUndefined();
  });

  it("parses the consent append response", () => {
    expect(ConsentAppendResponse.parse(load("ledger_blinded_consent_post.json")).review.prev).toBeUndefined();
  });
});

describe("governance writes chained on the concealed read", () => {
  it.each([
    ["undo", "ledger_blinded_undo.json", (token: string) => postUndoWithToken("case/1", { target: "t", contest: false, note: "reason" }, token)],
    ["contest", "ledger_blinded_contest.json", (token: string) => postContestWithToken("case/1", { target: "t", contest: true, note: "reason" }, token)],
  ] as const)("%s resolves as committed with the redacted event", async (_name, fixture, post) => {
    const body = load(fixture);
    expect(body.event).not.toHaveProperty("prev");
    expect(body.event).not.toHaveProperty("action_head");
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json(201, body)));
    const result = await post("token");
    expect(result).toMatchObject({ ok: true, status: 201 });
    expect(result.ok && result.response?.event.hash).toBe(body.event.hash);
  });

  it("reports a 201 with an unreadable body as committed", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("<html>", { status: 201 })));
    expect(await postUndoWithToken("case/1", { target: "t", contest: false, note: "reason" }, "token")).toEqual({ ok: true, status: 201, response: null });
  });

  it("reports a 201 whose body fails the contract as committed", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json(201, { event: { hash: 5 } })));
    expect(await postUndoWithToken("case/1", { target: "t", contest: false, note: "reason" }, "token")).toEqual({ ok: true, status: 201, response: null });
  });

  it("keeps refusal handling for non-201 responses", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json(409, { meta: load("ledger_blinded_undo.json").meta, error: "target is already compensated" })));
    expect(await postUndoWithToken("case/1", { target: "t", contest: false, note: "reason" }, "token")).toMatchObject({
      ok: false,
      status: 409,
      error: "target is already compensated",
    });
  });

  it("consent append of a redacted event is saved, not failed", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json(201, load("ledger_blinded_consent_post.json"))));
    const result = await postConsent("case/1", { event: "consent_granted", expected_head: "rev", note: "" });
    expect(result).toMatchObject({ ok: true, status: 201 });
    expect(result.ok && result.response).not.toBeNull();
  });
});
