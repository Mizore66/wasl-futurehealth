import { afterEach, describe, expect, it, vi } from "vitest";
import { CaseSummary } from "@/lib/api/schema";
import { REFERRER_TOKEN_MISSING, postReferrerResponse } from "@/lib/api/source";
import { referralFrom } from "@/components/worklist/referrals";
import { EMPTY_DRAFT, referrerRequest, returnedPacket } from "@/components/worklist/returned-packets";
import synthetic from "@/data/referral-inbox-synthetic.json";

const meta = { api: "wasl-api/1", data_mode: "simulated", run: "r", policy: "p", reader_model: "m", loaded_at: "2026-11-10T00:00:00Z", note: "synthetic" };
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

const base = synthetic[0] as Record<string, unknown> & { api_overlay: Record<string, unknown> };
const response = { response: "return", reason: "anticoagulant_plan_missing", requested_info: ["anticoagulant_plan", "relevant_clinical_information"], unit: "skmc-endo", run_day: 110, at: "t", actor: "Dr Layla Haddad", event: "ret-1" };
const handoff = {
  referral_id: "x",
  reason_code: "needs_more_info",
  responsible_clinician: "referring PCP, ahs-mushrif",
  decided_by: "skmc-endo",
  decided_at: 110,
  act_by: 112,
  medical_director_day: 114,
  status: "medical_director",
  event: "ret-1",
  info_requested: ["anticoagulant_plan", "relevant_clinical_information"],
  destinations: ["skmc-endo", "noor-endo"],
};
const returned = (h: Record<string, unknown> = {}) =>
  CaseSummary.parse({ ...base, dropoff: "referral_unanswered", api_overlay: { ...base.api_overlay, head: "h3", referral_response: response, referral_handoff: { ...handoff, ...h } } });

describe("returned referral packet for the referring GP", () => {
  it("binds the answer to the case head and the unit's return event, and offers only other units for a redirect", () => {
    const p = returnedPacket(returned())!;
    expect(p).toMatchObject({ head: "h3", packet: "ret-1", unitId: "skmc-endo", infoRequested: ["anticoagulant_plan", "relevant_clinical_information"], acknowledged: false, answer: null });
    expect(p.others).toEqual([{ id: "noor-endo", label: "Noor endoscopy" }]);
  });

  it("has no packet without a persisted return", () => {
    expect(returnedPacket(CaseSummary.parse(base))).toBeNull();
  });

  it("refuses incomplete answers and builds complete ones", () => {
    const p = returnedPacket(returned())!;
    expect(referrerRequest(p, EMPTY_DRAFT).ok).toBe(false);
    expect(referrerRequest(p, { ...EMPTY_DRAFT, response: "supply_info", info: { anticoagulant_plan: "Hold apixaban" } })).toEqual({ ok: false, missing: "Fill in: Relevant clinical information." });
    expect(referrerRequest(p, { ...EMPTY_DRAFT, response: "supply_info", info: { anticoagulant_plan: " Hold apixaban ", relevant_clinical_information: "FIT 180" } })).toEqual({
      ok: true,
      request: { response: "supply_info", supplied_info: { anticoagulant_plan: "Hold apixaban", relevant_clinical_information: "FIT 180" } },
    });
    expect(referrerRequest(p, { ...EMPTY_DRAFT, response: "redirect", destination: "skmc-endo" }).ok).toBe(false);
    expect(referrerRequest(p, { ...EMPTY_DRAFT, response: "redirect", destination: "noor-endo" })).toEqual({ ok: true, request: { response: "redirect", destination: "noor-endo" } });
    expect(referrerRequest(p, { ...EMPTY_DRAFT, response: "re_refer" })).toEqual({ ok: true, request: { response: "re_refer", destination: "skmc-endo" } });
    expect(referrerRequest(p, { ...EMPTY_DRAFT, response: "close", reason: "  " }).ok).toBe(false);
    expect(referrerRequest(p, { ...EMPTY_DRAFT, response: "manage_in_primary_care", reason: "Repeat FIT" })).toEqual({ ok: true, request: { response: "manage_in_primary_care", reason: "Repeat FIT" } });
  });

  it("an acknowledgement alone still waits on the GP; a resolving answer leaves the receiving unit's waiting view", () => {
    const acked = returned({ acknowledgement: { at: "t", actor: "gp", run_day: 111, event: "ack-1" } });
    expect(returnedPacket(acked)).toMatchObject({ acknowledged: true, answer: null, status: "medical_director" });
    expect(referralFrom(acked, 120, false)!.handoff?.status).toBe("medical_director");

    const acted = returned({ status: "referrer_acted", acted_at: 115, referrer_response: { response: "close", reason: "Done privately", at: "t", actor: "gp", run_day: 115, event: "r-1" } });
    expect(returnedPacket(acted)!.answer).toMatch(/^Close the referral\. Reason: Done privately\. Answered/);
    const inbox = referralFrom(acted, 120, false)!;
    expect(inbox).toMatchObject({ state: "returned", handoff: { status: "referrer_acted" } });
    expect(acted.closed).toBe(false);
  });
});

describe("postReferrerResponse", () => {
  const id = "ahs-mushrif/ServiceRequest/AHS-sr-9001";

  it("posts the answer bound to case, packet and head with the referring GP's own token", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_CLINICIAN_TOKEN", "unit-clinician-secret-1234567890");
    vi.stubEnv("WASL_API_REFERRER_TOKEN", "referrer-secret-1234567890");
    const fetchMock = vi.fn().mockResolvedValue(json(201, { meta, event: { hash: "h4", at: "t", actor: "gp" }, api_overlay: { head: "h4", actions: 3, owner: "", last_action_at: "", ledger_status: "ok" } }));
    vi.stubGlobal("fetch", fetchMock);
    expect((await postReferrerResponse(id, "ret-1", { response: "re_refer", destination: "skmc-endo" }, "h3")).ok).toBe(true);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe(`http://127.0.0.1:8088/api/cases/${encodeURIComponent(id)}/referrer-response`);
    expect(new Headers(init.headers).get("authorization")).toBe("Bearer referrer-secret-1234567890");
    expect(JSON.parse(init.body)).toEqual({ response: "re_refer", destination: "skmc-endo", referral_id: id, packet: "ret-1", expected_head: "h3" });
  });

  it("sends nothing for an incomplete answer or without the referrer token", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_CLINICIAN_TOKEN", "unit-clinician-secret-1234567890");
    vi.stubEnv("WASL_API_REFERRER_TOKEN", "");
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    expect((await postReferrerResponse(id, "ret-1", { response: "close", reason: " " }, "h3")).status).toBe(400);
    expect(await postReferrerResponse(id, "ret-1", { response: "acknowledge" }, "h3")).toEqual({ ok: false, status: 401, error: REFERRER_TOKEN_MISSING });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("reports a stale-head conflict without retrying", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_REFERRER_TOKEN", "referrer-secret-1234567890");
    const fetchMock = vi.fn().mockResolvedValue(json(409, { meta, error: "stale expected_head", conflict: "head", current_head: "h4" }));
    vi.stubGlobal("fetch", fetchMock);
    expect(await postReferrerResponse(id, "ret-1", { response: "acknowledge" }, "h3")).toMatchObject({ ok: false, status: 409, conflict: "head", currentHead: "h4" });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
