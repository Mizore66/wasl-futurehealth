import { readFileSync } from "node:fs";
import path from "node:path";
import { NextRequest } from "next/server";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { GET } from "@/app/app/today/records/route";
import { sourceEventLabel } from "@/components/case/audit-model";
import { holdWords } from "@/components/case/words";
import { RecordsPanel } from "@/components/worklist/today-file";
import { heldCount, todayRecords, TodayRecords } from "@/components/worklist/today-records";
import { CaseDetail } from "@/lib/api/schema";

/* Today's Records and source Activity, from the published fixture details (data/api) the fixtures-mode route serves. */
const fixture = (name: string) => CaseDetail.parse(JSON.parse(readFileSync(path.join(__dirname, "..", "data", "api", `case_${name}.json`), "utf8")));
const trap = fixture("false_closure_trap");
const closed = fixture("evidence_closure");
const text = (html: string) => html.replace(/<[^>]+>/g, " ").replace(/\s+/g, " ").trim();

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("todayRecords", () => {
  it("marks the held report and the identity-held appointment with the reasons the detail gives, held first", () => {
    const r = todayRecords(trap);
    const report = r.sources.find((s) => s.id === "noor-endo/DiagnosticReport/NOO-dr-316")!;
    // Both review items are signed, so only the gate's own hold stands on the report; the split is settled.
    expect(report.held).toEqual([holdWords(trap.holds[0])]);
    expect(report).toMatchObject({ type: "Report", provider: "Noor endoscopy", day: expect.any(Number) });
    expect(r.sources[0].id).toBe(report.id);
    expect(heldCount(r)).toBe(1);
  });

  it("an unsigned identity review holds the record its chain names; an unanswered reader split holds the report", () => {
    const d: CaseDetail = { ...trap, signoffs: [] };
    const r = todayRecords(d);
    const appt = r.sources.find((s) => s.id === "noor-endo/Appointment/NOO-appt-364")!;
    expect(appt.held).toEqual(["Identity in review: no second identifier is confirmed for the two records."]);
    const report = r.sources.find((s) => s.id === "noor-endo/DiagnosticReport/NOO-dr-316")!;
    expect(report.held).toEqual([holdWords(trap.holds[0]), "Evidence in review: a person must check what the report shows."]);
    const noItems = todayRecords({ ...trap, review_items: [], signoffs: [] });
    expect(noItems.sources.find((s) => s.id === report.id)!.held).toEqual([holdWords(trap.holds[0]), "The two automatic report checks disagree."]);
    const firstFree = r.sources.findIndex((s) => s.held.length === 0);
    expect(r.sources.slice(firstFree).every((s) => s.held.length === 0)).toBe(true);
    expect(heldCount(r)).toBe(2);
  });

  it("lists one source per record, its latest entry deciding where it stands", () => {
    const r = todayRecords(trap);
    const ids = r.sources.map((s) => s.id);
    expect(new Set(ids).size).toBe(ids.length);
    expect(new Set(trap.timeline.map((e) => e.id))).toEqual(new Set(ids));
  });

  it("names source events with the Audit page's words, earliest first", () => {
    const r = todayRecords(trap);
    const byDay = [...trap.timeline].sort((a, b) => a.day - b.day || (a.seq ?? 0) - (b.seq ?? 0));
    expect(r.events.map((e) => e.label)).toEqual(byDay.map((e) => sourceEventLabel(e, trap.case.id).label));
    expect(r.events.find((e) => e.kind === "referral")?.label).toBe("Referral issued");
    const dates = r.events.map((e) => e.date!);
    expect([...dates].sort()).toEqual(dates);
  });

  it("holds nothing on a closed case", () => {
    expect(heldCount(todayRecords(closed))).toBe(0);
  });

  it("a hold without a report is held on the case, not on a record", () => {
    const hold = { loop: trap.case.id, rule: "r", state: "review_hold", reason: "awaiting_eye_clinic_exam" };
    const d: CaseDetail = { ...trap, evidence: [], review_items: [], current: { ...trap.current, chain: [{ type: "hold_decision", body: hold, prev: "g", hash: "h" }] } };
    const r = todayRecords(d);
    expect(r.caseHeld).toEqual([holdWords(hold)]);
    expect(r.sources.every((s) => s.held.length === 0)).toBe(true);
    expect(heldCount(r)).toBe(1);
  });
});

describe("GET /app/today/records", () => {
  const req = (q: string) => new NextRequest(`http://localhost/app/today/records${q}`);

  it("fixtures mode: serves the showcase case's records, read-only", async () => {
    vi.stubEnv("WASL_API_URL", "");
    const res = await GET(req(`?id=${encodeURIComponent(trap.case.id)}`));
    expect(res.status).toBe(200);
    expect(res.headers.get("cache-control")).toBe("no-store");
    const body = TodayRecords.parse(await res.json());
    expect(heldCount(body)).toBe(heldCount(todayRecords(trap)));
  });

  it("fixtures mode: a case without a detail record is a 404 that says so; a missing id is a 400", async () => {
    vi.stubEnv("WASL_API_URL", "");
    const res = await GET(req(`?id=${encodeURIComponent("ahs-mushrif/ServiceRequest/AHS-sr-1002")}`));
    expect(res.status).toBe(404);
    expect(await res.json()).toEqual({ error: "this demo has records for a few sample patients only", missing: true });
    expect((await GET(req(""))).status).toBe(400);
  });

  it("live: reads with the shared read credential and maps an API failure to words", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_WEB_REVIEWER_TOKEN", "reviewer-secret-1234567890");
    const fetchMock = vi.fn(async () => new Response("nope", { status: 503 }));
    vi.stubGlobal("fetch", fetchMock);
    const res = await GET(req(`?id=${encodeURIComponent(trap.case.id)}`));
    expect(res.status).toBe(502);
    expect(await res.json()).toEqual({ error: "the case system returned an error; try again" });
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe(`http://127.0.0.1:8088/api/cases/${encodeURIComponent(trap.case.id)}`);
    expect(new Headers(init.headers).get("authorization")).toBe("Bearer reviewer-secret-1234567890");
    expect(init.method ?? "GET").toBe("GET");
  });
});

describe("RecordsPanel", () => {
  const render = (read: Parameters<typeof RecordsPanel>[0]["read"]) => text(renderToStaticMarkup(RecordsPanel({ read, onRetry: () => {} })));

  it("says it is reading, why it failed (with Retry), or that the demo has no records for the case", () => {
    expect(render(undefined)).toBe("Loading records…");
    expect(render({ ok: false, error: "no response; check your connection", missing: false })).toBe("Could not load the records: no response; check your connection Retry");
    expect(render({ ok: false, error: "this demo has records for a few sample patients only", missing: true })).toBe("No records to show: this demo has records for a few sample patients only.");
    expect(render({ ok: true, records: { sources: [], caseHeld: [], events: [] } })).toBe("No records on this case yet.");
  });

  it("marks held records with their reason beside their source and date", () => {
    const html = render({ ok: true, records: todayRecords({ ...trap, signoffs: [] }) });
    expect(html).toContain(`Held Noor`);
    expect(html).toContain(holdWords(trap.holds[0]));
    expect(html).toContain("Identity in review: no second identifier is confirmed for the two records.");
  });
});
