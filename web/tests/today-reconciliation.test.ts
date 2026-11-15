import { readFileSync } from "node:fs";
import path from "node:path";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ElsewhereForm } from "@/components/worklist/elsewhere-form";
import { persistedLedger } from "@/components/worklist/ledger-trail";
import { workItem } from "@/components/worklist/model";
import { ActiveCaseSummary, CaseAction, CaseDetail, CaseList, type ApiOverlay } from "@/lib/api/schema";

/* Issue #177: a completed-elsewhere report opens an owned record retrieval that suspends chase until its outcome. */
const load = (f: string) => JSON.parse(readFileSync(path.join(__dirname, "fixtures", f), "utf8"));
const base = ActiveCaseSummary.parse(CaseList.parse(load("ledger_case_list.json")).cases[0]);
const NOW = 40;
const WALL = Date.parse("2026-11-20T12:00:00Z");
const report = {
  status: "pending" as const, at: "2026-11-20T09:00:00Z", event: "r".repeat(64), procedure: "colonoscopy", performed: "2026-09",
  provider: "Synthetic Private Hospital", source: "patient", retrieval_owner: "Hessa Al Ali", retrieval_due: "2026-12-04",
};
const overlay = (ov: Partial<ApiOverlay>): ApiOverlay => ({ head: "h", actions: 1, owner: "Hessa Al Ali", last_action_at: "", ledger_status: "ok", no_answer_streak: 0, ...ov });
const item = (ov: Partial<ApiOverlay>, wall = WALL) =>
  workItem({ ...base, owner: "Hessa Al Ali", dropoff: "not_booked", api_overlay: overlay(ov) }, NOW, true, wall);

describe("completed elsewhere: owned record retrieval", () => {
  it("a pending retrieval leaves To call, names its owner and due date, and never claims the procedure", () => {
    const before = item({});
    expect(before.view).toBe("call");
    const it = item({ reconciliation: report });
    expect(it.view).toBe("waiting");
    expect(it.status).toBe("Reported done elsewhere · record due 2026-12-04");
    expect(it.next).toBe("Hessa Al Ali retrieves the colonoscopy report from Synthetic Private Hospital by 2026-12-04");
    expect(it.retrieval).toEqual({ owner: "Hessa Al Ali", due: "2026-12-04", overdue: false });
    expect(it.awaitingNext).toBe(false);
    expect(`${it.status} ${it.next}`).not.toMatch(/closed|booked|fulfilled/i);
  });

  it("is overdue the day after its due date, deterministically from the wall clock", () => {
    expect(item({ reconciliation: report }, Date.parse("2026-12-04T23:00:00Z")).retrieval?.overdue).toBe(false);
    const late = item({ reconciliation: report }, Date.parse("2026-12-05T08:00:00Z"));
    expect(late.retrieval?.overdue).toBe(true);
    expect(late.status).toBe("Reported done elsewhere · record retrieval overdue since 2026-12-04");
  });

  it("a pending retrieval outranks an earlier reached call and its follow-up chips", () => {
    const reached = { at: "2026-11-19T09:00:00Z", event: "c".repeat(64), outcome: "reached", channel: "phone" };
    expect(item({ last_contact: reached }).awaitingNext).toBe(true);
    expect(item({ last_contact: reached, reconciliation: report }).awaitingNext).toBe(false);
  });

  it("a recorded retrieval outcome resumes ordinary follow-up", () => {
    const ended = { ...report, status: "resumed" as const, retrieval: { at: "2026-11-21T09:00:00Z", event: "o".repeat(64), outcome: "not_found" as const } };
    const it = item({ reconciliation: ended });
    expect(it.view).toBe("call");
    expect(it.retrieval).toBeUndefined();
    expect(it.status).not.toMatch(/elsewhere/);
  });

  it("the case trail keeps both the report and the retrieval outcome", () => {
    const raw = load("ledger_case_detail.json");
    const last = raw.api_reviews.at(-1);
    const event = (kind: string, hash: string, prev: string, seq: number, data: Record<string, string>) => ({
      ...last, ledger: undefined, type: "coordinator_action", kind, role: "coordinator", action: "coordinate_case", actor: "hessa", hash, prev, seq, data,
    });
    raw.api_reviews = [
      ...raw.api_reviews,
      event("completed_elsewhere", "1".repeat(64), last.hash, 98, { procedure: "colonoscopy", performed: "2026-09", provider: "Synthetic Private Hospital", source: "patient", retrieval_owner: "Hessa Al Ali", retrieval_due: "2026-12-04" }),
      event("retrieval_outcome", "2".repeat(64), "1".repeat(64), 99, { outcome: "not_found", report_event: "1".repeat(64) }),
    ];
    const entries = persistedLedger(CaseDetail.parse(raw)).entries.slice(-2).map((e) => e.what);
    expect(entries).toEqual([
      "Reported done elsewhere: colonoscopy at Synthetic Private Hospital · Hessa Al Ali to retrieve the record by 2026-12-04",
      "Record retrieval ended: record not found · follow-up resumes",
    ]);
  });

  it("the action contract refuses unknown fields' values and unstructured dates", () => {
    const ok = { action: "completed_elsewhere", procedure: "colonoscopy", performed: "2026-09", provider: "Synthetic Private Hospital", source: "patient", retrieval_owner: "Hessa Al Ali", retrieval_due: "2026-12-04" };
    expect(CaseAction.safeParse(ok).success).toBe(true);
    expect(CaseAction.safeParse({ ...ok, performed: "last autumn" }).success).toBe(false);
    expect(CaseAction.safeParse({ ...ok, source: "rumour" }).success).toBe(false);
    expect(CaseAction.safeParse({ action: "retrieval_outcome", outcome: "closed" }).success).toBe(false);
  });

  it("the coordinator form labels every structured field and defaults a 14-day owned retrieval", () => {
    const html = renderToStaticMarkup(createElement(ElsewhereForm, { owner: "Hessa Al Ali", procedure: "colonoscopy", nowMs: WALL, disabled: false, onRecord: () => {}, onCancel: () => {} }));
    for (const label of ["Procedure", "Roughly when (month)", "Where", "Reported by", "Report reference, if known", "Retrieves the record", "Record due by"]) {
      expect(html).toContain(`>${label}</label>`);
    }
    expect(html).toContain('value="2026-12-04"');
    expect(html).toContain('value="Hessa Al Ali"');
    expect(html).toContain("This does not close the case.");
  });
});
