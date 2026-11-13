import { readFileSync } from "node:fs";
import path from "node:path";
import { NextRequest } from "next/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { GET } from "@/app/app/today/ledger/route";
import { renderToStaticMarkup } from "react-dom/server";
import { LedgerInvalidBanner, listLedgerInvalid } from "@/components/worklist/ledger-banner";
import { persistedLedger } from "@/components/worklist/ledger-trail";
import { BOOKING_NEEDED, DECLINED_STATUS, noAnswerStatus, workItem } from "@/components/worklist/model";
import { CaseDetail, CaseList, type ApiOverlay } from "@/lib/api/schema";
import { COORDINATOR_TOKEN_MISSING, COORDINATOR_TOKEN_REFUSED, postAction } from "@/lib/api/source";
import { compareInstants } from "@/lib/domain/instant";
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

  it("uses ledger order rather than sorting wall-clock timestamps", () => {
    const times = ["2026-11-09T08:00:00.25Z", "2026-11-09T07:59:59.999999999Z", "2026-11-09T08:00:00Z"];
    const events = full.api_reviews.filter((event) => event.ledger === "coordinator" && (event.kind === "contact_outcome" || event.kind === "note")).slice(0, 3).map((event, i) => ({ ...event, at: times[i] })) as CaseDetail["api_reviews"];
    const l = persistedLedger({ ...full, api_reviews: events });
    expect(l.entries.map((e) => e.hash)).toEqual(events.map((e) => e.hash));
  });

  it("a detail with no overlay (an older snapshot) has no entries and is not flagged", () => {
    const older = { ...full, api_overlay: undefined, review_ledger: undefined, api_reviews: [] };
    expect(persistedLedger(older)).toEqual({ status: "ok", detail: "", applied: null, entries: [] });
  });

  it("an older overlay-only snapshot keeps its persisted contact history", () => {
    const older = { ...full, api_reviews: [] };
    const entries = persistedLedger(older).entries;
    expect(entries).toHaveLength(4);
    expect(entries.map((entry) => entry.what)).toContain("Note: “synthetic coordinator note”");
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

  it("a snooze saved before the last outcome in the same second does not set the status (zero-nanosecond snooze time)", () => {
    const until = new Date(NOW + 86_400_000).toISOString();
    const earlier = withOverlay({
      last_contact: { at: "2026-11-20T10:00:00.5Z", event: "e", outcome: "reached", channel: "phone" },
      snooze: { until, reason: "Call back Sat 21 Nov", at: "2026-11-20T10:00:00Z" },
    });
    expect(earlier.status).toBe(`Reached · ${earlier.next}`);
    const later = withOverlay({
      last_contact: { at: "2026-11-20T10:00:00Z", event: "e", outcome: "reached", channel: "phone" },
      snooze: { until, reason: "Call back Sat 21 Nov", at: "2026-11-20T10:00:00.000000001Z" },
    });
    expect(later.status).toBe("Call back Sat 21 Nov");
  });

  it("a persisted Reached outcome is still waiting for its follow-up after a reload", () => {
    const reached = withOverlay({ last_contact: { at: at(2), event: "e", outcome: "reached", channel: "phone" } });
    expect(reached.awaitingNext).toBe(true);
    expect(withOverlay({}).awaitingNext).toBe(false);
  });

  it("a later outcome, an active snooze or a booking projection leaves no Reached follow-up to restore", () => {
    for (const outcome of ["appointment_agreed", "declined", "no_answer", "voicemail", "wrong_number"]) {
      expect(withOverlay({ last_contact: { at: at(2), event: "e", outcome, channel: "phone" }, no_answer_streak: 1 }).awaitingNext).toBe(false);
    }
    const snoozed = withOverlay({
      last_contact: { at: at(5), event: "e", outcome: "reached", channel: "phone" },
      snooze: { until: new Date(NOW + 86_400_000).toISOString(), reason: "Call back Sat 21 Nov", at: at(1) },
    });
    expect(snoozed.awaitingNext).toBe(false);
    const olderSnooze = withOverlay({
      last_contact: { at: at(1), event: "e", outcome: "reached", channel: "phone" },
      snooze: { until: new Date(NOW + 86_400_000).toISOString(), reason: "Call back Sat 21 Nov", at: at(5) },
    });
    expect(olderSnooze.awaitingNext).toBe(true);
    const reserved = withOverlay({
      last_contact: { at: at(2), event: "e", outcome: "reached", channel: "phone" },
      booking: { status: "reserved", slot: { day: 44 } } as ApiOverlay["booking"],
    });
    expect([reserved.awaitingNext, reserved.view]).toEqual([false, "waiting"]);
  });

  it("the served lists place the real fixtures: the last outcome was a decline", () => {
    const it = workItem(fullList.cases[0], 40, true, NOW);
    expect([it.view, it.status]).toEqual(["waiting", DECLINED_STATUS]);
    const p = workItem(partialList.cases[0], 40, true, NOW);
    expect([p.ledgerStatus, p.view]).toEqual(["invalid", "waiting"]);
  });

  it("one listed case reporting a partial replay flags the whole board, since replay failure is server-wide", () => {
    const ok = fullList.cases.map((c) => workItem(c, 40, true, NOW));
    expect(listLedgerInvalid(ok)).toBe(false);
    expect(listLedgerInvalid([...ok, workItem(partialList.cases[0], 40, true, NOW)])).toBe(true);
    expect(listLedgerInvalid([])).toBe(false);
    const html = renderToStaticMarkup(LedgerInvalidBanner({ what: "this board" }));
    expect(html).toContain('role="alert"');
    expect(html).toContain("nothing on this board is authoritative");
    // pipeline/api.go sets ledgerErr on a failed replay, an indeterminate append and a ledger changed by another writer:
    // the banner must not claim it was a replay that stopped.
    expect(html).not.toContain("stopped replaying");
    expect(html).toContain("refuses new actions until an operator restores it");
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
    expect(await res.json()).toEqual({
      ...persistedLedger(partial),
      bookingPanel: { booking: partial.booking, overlay: partial.api_overlay, case: partial.case },
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe(`http://127.0.0.1:8088/api/cases/${encodeURIComponent(partial.case.id)}`);
  });

  it("says which kind of failure it was: no credential here, an unrecognised answer, an API refusal, or no answer", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_WEB_REVIEWER_TOKEN", "");
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const missing = await GET(req("?id=x"));
    expect([missing.status, (await missing.json()).error]).toEqual([500, "this server has no API read credential configured"]);
    expect(fetchMock).not.toHaveBeenCalled();

    vi.stubEnv("WASL_API_WEB_REVIEWER_TOKEN", "reviewer-secret-1234567890");
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ meta: {}, case: "not a case" }), { status: 200 }));
    const shape = await GET(req("?id=x"));
    expect([shape.status, (await shape.json()).error]).toEqual([502, "the API answered in a shape this page does not recognise"]);

    // A 200 whose body is not JSON at all is an answer this page does not recognise, not "no answer".
    fetchMock.mockResolvedValueOnce(new Response("<html>proxy error page</html>", { status: 200 }));
    const notJson = await GET(req("?id=x"));
    expect([notJson.status, (await notJson.json()).error]).toEqual([502, "the API answered in a shape this page does not recognise"]);

    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ error: "authentication required" }), { status: 401 }));
    const refused = await GET(req("?id=x"));
    expect([refused.status, (await refused.json()).error]).toEqual([502, "the API refused this server's read credential (HTTP 401)"]);

    fetchMock.mockResolvedValueOnce(new Response("{}", { status: 503 }));
    const down = await GET(req("?id=x"));
    expect([down.status, (await down.json()).error]).toEqual([502, "the API answered HTTP 503"]);

    fetchMock.mockRejectedValueOnce(new TypeError("fetch failed"));
    const silent = await GET(req("?id=x"));
    expect([silent.status, (await silent.json()).error]).toEqual([502, "the API did not answer"]);
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

describe("coordinator credential refusals", () => {
  it("a 401 from the API is returned as a refusal that names the credential, not thrown as no answer", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_COORDINATOR_TOKEN", "coordinator-secret-1234567890");
    // The Go API's 401 body: no meta (pipeline/api_security.go authenticate).
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: "authentication required" }), { status: 401 })));
    const res = await postAction(partial.case.id, { action: "note", text: "x" }, "h");
    expect(res).toEqual({ ok: false, status: 401, error: COORDINATOR_TOKEN_REFUSED });
  });

  it("a missing coordinator token sends nothing and says so", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_COORDINATOR_TOKEN", "");
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const res = await postAction(partial.case.id, { action: "note", text: "x" }, "h");
    expect(res).toEqual({ ok: false, status: 401, error: COORDINATOR_TOKEN_MISSING });
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe("compareInstants", () => {
  const sorted = (xs: string[]) => [...xs].sort(compareInstants);

  it("orders RFC3339Nano times by instant, including the zero-nanosecond form with no fraction", () => {
    expect(sorted(["2026-11-09T08:00:00.1Z", "2026-11-09T08:00:00Z", "2026-11-09T08:00:00.000000001Z", "2026-11-09T07:59:59.9Z"])).toEqual([
      "2026-11-09T07:59:59.9Z",
      "2026-11-09T08:00:00Z",
      "2026-11-09T08:00:00.000000001Z",
      "2026-11-09T08:00:00.1Z",
    ]);
  });

  it("keeps nanoseconds within one millisecond, treats equal instants as equal and honours offsets", () => {
    expect(compareInstants("2026-11-09T08:00:00.000000002Z", "2026-11-09T08:00:00.000000001Z")).toBeGreaterThan(0);
    expect(compareInstants("2026-11-09T08:00:00.100Z", "2026-11-09T08:00:00.1Z")).toBe(0);
    expect(compareInstants("2026-11-09T12:00:00+04:00", "2026-11-09T08:00:00.5Z")).toBeLessThan(0);
  });

  it("sorts an unparseable time after every real one", () => {
    expect(sorted(["garbage", "2026-11-09T08:00:00Z"])).toEqual(["2026-11-09T08:00:00Z", "garbage"]);
  });
});

it("the committed fixture snapshots still parse with the overlay fields absent", () => {
  expect(() => CaseList.parse(loadSample("cases_crc_page1.json"))).not.toThrow();
});
