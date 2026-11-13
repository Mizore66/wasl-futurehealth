import { afterEach, describe, expect, it, vi } from "vitest";
import { confirmRecovery, getRecoverySlot, getRecoverySlots } from "@/lib/api/recovery-source";
import { CaseList } from "@/lib/api/schema";
import { workItem } from "@/components/worklist/model";
import { loadSample } from "./samples";

const meta = { api: "wasl-api/1", data_mode: "simulated", run: "r", policy: "p", reader_model: "m", loaded_at: "2026-11-10T00:00:00Z", note: "synthetic" };
const response = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
const input = { case_id: "ahs-mushrif/ServiceRequest/AHS-sr-910", expected_slot_head: "slot-head", expected_case_head: "case-head", request_id: "browser-request-0001", outreach_intent: false, note: "" };

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("recovery API contracts", () => {
  it("renders the fixture's API ordering and excluded reasons without inventing persistence", async () => {
    vi.stubEnv("WASL_API_URL", "");
    const list = await getRecoverySlots();
    const detail = await getRecoverySlot(list.slots[0].slot_id);
    expect(detail?.slot.candidates.map((candidate) => [candidate.case_id, candidate.rank])).toEqual([
      ["ahs-mushrif/ServiceRequest/AHS-sr-910", 1],
      ["ahs-mushrif/ServiceRequest/AHS-sr-900", null],
    ]);
    expect(detail?.slot.candidates[1].checks[0]).toMatchObject({ result: "fail", reason_code: "source_case_for_own_release" });
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const result = await confirmRecovery(list.slots[0].slot_id, input);
    expect(result).toMatchObject({ ok: false, status: 503 });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("uses only the server coordinator token and returns the persisted ledger hash", async () => {
    vi.stubEnv("WASL_API_URL", "http://api.test");
    vi.stubEnv("WASL_API_COORDINATOR_TOKEN", "coordinator-secret-1234567890");
    const event = { hash: "ledger-hash", at: "2026-11-13T00:00:00Z", type: "waitlist_recovery" };
    const fetchMock = vi.fn().mockResolvedValue(response(201, { meta, event, case_head: "ledger-hash", canonical_unchanged: true, dispatched: false, duplicate: false }));
    vi.stubGlobal("fetch", fetchMock);
    const result = await confirmRecovery("rel_aaaaaaaaaaaaaaaaaaaaaaaa", input);
    expect(result).toMatchObject({ ok: true, status: 201, response: { event: { hash: "ledger-hash" }, dispatched: false } });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("http://api.test/api/recovery/slots/rel_aaaaaaaaaaaaaaaaaaaaaaaa/confirm");
    const headers = new Headers(init.headers);
    expect(headers.get("authorization")).toBe("Bearer coordinator-secret-1234567890");
    expect(headers.has("x-wasl-actor")).toBe(false);
    expect(headers.has("x-wasl-role")).toBe(false);
    expect(JSON.parse(init.body)).toEqual(input);
  });

  it("returns a stale conflict as a refusal, not success", async () => {
    vi.stubEnv("WASL_API_URL", "http://api.test");
    vi.stubEnv("WASL_API_COORDINATOR_TOKEN", "coordinator-secret-1234567890");
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response(409, { meta, error: "stale expected_slot_head", conflict: "slot_head" })));
    const result = await confirmRecovery("rel_aaaaaaaaaaaaaaaaaaaaaaaa", input);
    expect(result).toEqual({ ok: false, status: 409, error: "stale expected_slot_head", conflict: "slot_head" });
  });
});

describe("recovery case projection", () => {
  it("moves a persisted simulated backfill to Waiting after reload", () => {
    const c = CaseList.parse(loadSample("cases_crc_page1.json")).cases.find((item) => !item.closed)!;
    const item = workItem({ ...c, recovery: { source_releases: [], backfill: { slot_id: "rel_aaaaaaaaaaaaaaaaaaaaaaaa", status: "simulated_backfill_confirmed", day: 40, event: "ledger-hash", outreach_status: "none" } } }, 120, false);
    expect(item.view).toBe("waiting");
    expect(item.status).toBe("Simulated slot confirmed");
    expect(c.state).not.toBe("closed");
  });
});
