import { readFileSync } from "node:fs";
import path from "node:path";
import { NextRequest } from "next/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { GET } from "@/app/app/today/ledger/route";
import { persistedLedger } from "@/components/worklist/ledger-trail";
import { BOOKING_NEEDED, DECLINED_STATUS, noAnswerStatus, workItem } from "@/components/worklist/model";
import { CaseDetail, CaseList, type ApiOverlay } from "@/lib/api/schema";
import { postAction } from "@/lib/api/source";
import { loadSample } from "./samples";

/*
 * Issue #47. fixtures/ledger_partial_*.json are real Go handler output (pipeline/api_ledger_contract_test.go,
 * TestLedgerPartialReplayFixtures): four valid coordinator events, then a chain-valid event that names a case head the
 * case no longer has, so replay stops there; the served detail, list and the refused write were captured.
 */
const load = (f: string) => JSON.parse(readFileSync(path.join(__dirname, "fixtures", f), "utf8"));
const full = CaseDetail.parse(load("ledger_case_detail.json"));
const partial = CaseDetail.parse(load("ledger_partial_case_detail.json"));
const partialList = CaseList.parse(load("ledger_partial_case_list.json"));
const fullList = CaseList.parse(load("ledger_case_list.json"));

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("persisted trail from the case-detail overlay", () => {
  it("maps every persisted contact outcome and note, oldest first, with the actor the API recorded", () => {
    const l = persistedLedger(full);
    expect(l.status).toBe("ok");
    expect(l.entries.map((e) => e.what)).toEqual([
      "Reached (phone): “synthetic call note”",
      "Appointment agreed (phone)",
      "Note: “synthetic coordinator note”",
      "Declined the procedure (phone)",
    ]);
    for (const e of l.entries) {
      expect(e.who).toBe("coordinator-bound");
      expect(e.when).toMatch(/^Saved · \d{4}-\d{2}-\d{2} \d{2}:\d{2} UTC · [0-9a-f]{8}$/);
      expect(e.hash).toMatch(/^[0-9a-f]{64}$/);
    }
  });

  it("a partial replay keeps only the valid prefix and says why", () => {
    const l = persistedLedger(partial);
    expect(l.status).toBe("invalid");
    expect(l.detail).toBe("line 5: head does not follow the case's previous event");
    expect(l.applied).toBe(4);
    // The fifth (invalid) line agreed an appointment: it must not appear.
    expect(l.entries.map((e) => e.what)).toEqual(["No answer (phone)", "Reached (phone): “synthetic call note”", "Note: “synthetic coordinator note”", "Declined the procedure (phone)"]);
  });

  it("a detail with no overlay (an older snapshot) has no entries and is not flagged", () => {
    const older = { ...full, api_overlay: undefined, review_ledger: undefined, api_reviews: [] };
    expect(persistedLedger(older)).toEqual({ status: "ok", detail: "", applied: null, entries: [] });
  });
});

describe("board position from persisted outcomes", () => {
  const base = fullList.cases[0];
  const NOW = Date.parse("2026-11-20T12:00:00Z");
  const at = (h: number) => new Date(NOW - h * 3_600_000).toISOString();
  const withOverlay = (ov: Partial<ApiOverlay>) =>
    workItem({ ...base, owner: "someone", dropoff: "not_booked", api_overlay: { head: "h", actions: 1, owner: "", last_action_at: "", ledger_status: "ok", no_answer_streak: 0, ...ov } }, 40, true, NOW);

  it("starts in To call with no contact", () => {
    expect(withOverlay({}).view).toBe("call");
  });

  it("an agreed appointment or a decline puts the patient in Waiting after a reload", () => {
    const agreed = withOverlay({ last_contact: { at: at(2), event: "e", outcome: "appointment_agreed", channel: "phone" } });
    expect([agreed.view, agreed.status]).toEqual(["waiting", BOOKING_NEEDED]);
    const declined = withOverlay({ last_contact: { at: at(2), event: "e", outcome: "declined", channel: "phone" } });
    expect([declined.view, declined.status]).toEqual(["waiting", DECLINED_STATUS]);
  });

  it("a no-answer within the last day sinks the row with its persisted count; an older one does not", () => {
    const recent = withOverlay({ last_contact: { at: at(3), event: "e", outcome: "no_answer", channel: "phone" }, no_answer_streak: 2 });
    expect([recent.view, recent.snoozed, recent.status, recent.noAnswer]).toEqual(["call", true, noAnswerStatus(2), 2]);
    const old = withOverlay({ last_contact: { at: at(30), event: "e", outcome: "no_answer", channel: "phone" }, no_answer_streak: 1 });
    expect(old.snoozed).toBe(false);
  });

  it("a snooze saved after the last outcome sets the status", () => {
    const it = withOverlay({
      last_contact: { at: at(5), event: "e", outcome: "reached", channel: "phone" },
      snooze: { until: new Date(NOW + 86_400_000).toISOString(), reason: "Call back Sat 21 Nov", at: at(1) },
    });
    expect([it.view, it.snoozed, it.status]).toEqual(["call", true, "Call back Sat 21 Nov"]);
  });

  it("the served lists place the real fixtures: the last outcome was a decline", () => {
    const it = workItem(fullList.cases[0], 40, true, NOW);
    expect([it.view, it.status]).toEqual(["waiting", DECLINED_STATUS]);
    const p = workItem(partialList.cases[0], 40, true, NOW);
    expect([p.ledgerStatus, p.view]).toEqual(["invalid", "waiting"]);
  });
});

describe("GET /app/today/ledger", () => {
  const req = (q: string) => new NextRequest(`http://localhost/app/today/ledger${q}`);

  it("reads the one selected case with the server-side token and returns its persisted trail", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_REVIEWER_TOKEN", "reviewer-secret-1234567890");
    vi.stubEnv("WASL_API_WEB_REVIEWER_TOKEN", "reviewer-secret-1234567890");
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(load("ledger_partial_case_detail.json")), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    const res = await GET(req(`?id=${encodeURIComponent(partial.case.id)}`));
    expect(res.status).toBe(200);
    expect(await res.json()).toEqual(persistedLedger(partial));
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe(`http://127.0.0.1:8088/api/cases/${encodeURIComponent(partial.case.id)}`);
  });

  it("is not available without a live API, and needs an id", async () => {
    vi.stubEnv("WASL_API_URL", "");
    expect((await GET(req("?id=x"))).status).toBe(404);
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    expect((await GET(req(""))).status).toBe(400);
  });
});

describe("writes while the ledger is invalid", () => {
  it("the API's 500 refusal is returned, not thrown", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_COORDINATOR_TOKEN", "coordinator-secret-1234567890");
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify(load("ledger_partial_action_refused.json")), { status: 500 })));
    const res = await postAction(partial.case.id, { action: "note", text: "x" }, "h");
    expect(res).toEqual({ ok: false, status: 500, error: "review ledger failed validation; writes refused" });
  });
});

it("the committed fixture snapshots still parse with the overlay fields absent", () => {
  expect(() => CaseList.parse(loadSample("cases_crc_page1.json"))).not.toThrow();
});
