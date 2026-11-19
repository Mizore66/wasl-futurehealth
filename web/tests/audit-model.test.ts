import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { decisionViews, groupByMonth, ledgerEvents, signatureTally, timelineEvents, UNDATED, type AuditEvent } from "@/components/case/audit-model";
import { Audit, CaseDetail } from "@/lib/api/schema";

const dir = path.join(process.cwd(), "data", "api");
const read = (f: string) => JSON.parse(readFileSync(path.join(dir, f), "utf8"));
const detail = CaseDetail.parse(read("case_false_closure_trap.json"));
const audit = Audit.parse(read("case_false_closure_trap_audit.json"));

const ev = (key: string, date: string | null, order = 0): AuditEvent => ({
  key,
  date,
  at: date ? Date.parse(date) : 0,
  order,
  kind: "source",
  label: key,
  source: "x",
});

describe("audit event history", () => {
  it("groups by month newest first, reverses both ways, and keeps undated last", () => {
    const events = [ev("a", "2026-05-02"), ev("b", "2026-07-18"), ev("c", null), ev("d", "2026-07-09"), ev("e", "2026-06-29")];
    const newest = groupByMonth(events);
    expect(newest.map((g) => g.name)).toEqual(["July 2026", "June 2026", "May 2026", UNDATED]);
    expect(newest[0].events.map((e) => e.key)).toEqual(["b", "d"]);
    const oldest = groupByMonth(events, true);
    expect(oldest.map((g) => g.name)).toEqual(["May 2026", "June 2026", "July 2026", UNDATED]);
    expect(oldest[2].events.map((e) => e.key)).toEqual(["d", "b"]);
  });

  it("dates every timeline record from its API day and names its provider", () => {
    const rows = timelineEvents(detail.timeline, detail.case.id);
    expect(rows).toHaveLength(detail.timeline.length);
    expect(rows.every((r) => /^\d{4}-\d{2}-\d{2}$/.test(r.date ?? ""))).toBe(true);
    expect(rows.find((r) => r.kind === "referral")?.label).toBe("Referral issued");
  });

  it("puts a ledger event without a time under Date unrecorded", () => {
    const history = Audit.shape.history.parse([{ type: "something_new", kind: "x", hash: "h1" }]);
    const rows = ledgerEvents(history, "crc");
    expect(rows[0].date).toBeNull();
    expect(groupByMonth(rows).map((g) => g.name)).toEqual([UNDATED]);
  });
});

describe("decision signatures", () => {
  it("labels each returned signature from the decision the case lists", () => {
    const views = decisionViews(audit, detail);
    expect(views).toHaveLength(audit.decision_signatures.length);
    expect(views[0].label).toBe("Referral opened");
    expect(views.map((v) => v.label)).toContain("Booking overdue");
    expect(views.every((v) => v.explanation.length > 0)).toBe(true);
  });

  it("never borrows a label for a signature the case does not list", () => {
    const views = decisionViews({ decision_signatures: [{ policy: "p", sig: "a", recomputed: "b" }] }, detail);
    expect(views[0].label).toBe("Decision 1");
    expect(views[0].matched).toBe(false);
    expect(views[0].explanation).toMatch(/does not say which decision/);
  });

  it("reports nothing to verify for an empty list, never a pass", () => {
    expect(signatureTally([])).toEqual({ matched: 0, total: 0, state: "none" });
    expect(signatureTally([{ policy: "p", sig: "a", recomputed: "a" }, { policy: "p", sig: "a", recomputed: "b" }]).state).toBe("some");
  });
});
