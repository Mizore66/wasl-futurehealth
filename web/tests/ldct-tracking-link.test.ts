import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { isActionable, ldctChildren, obligationWords, openObligation } from "@/components/worklist/obligation";
import { CaseList, LdctTracking, OverlayReconciliation, type CaseSummary } from "@/lib/api/schema";
import { formatDay } from "@/lib/domain/day";

// Issue #113: the served `ldct_tracking` link (pipeline/api_obligation.go) names the closed parent's child tracking request.
const raw = JSON.parse(readFileSync(path.join(__dirname, "..", "data", "api", "cases_lung_ldct_page1.json"), "utf8"));
type Raw = { id: string; ldct?: object };
const base = (raw.cases as Raw[]).find((c) => c.id.endsWith("/LCS-sr-11"))!;
const childId = "lcs-centre/ServiceRequest/LCS-sr-900";
const parent = (tracking: object | undefined): CaseSummary =>
  CaseList.parse({
    ...raw,
    cases: [{ ...base, state: "closed", closed: true, ldct: { ...base.ldct, scan_label: "LR4A", outcome: "followup_done", followup_report: "LCS-ct-2", followup_label: "LR3" }, ldct_tracking: tracking }],
  }).cases[0];

describe("the served LDCT tracking link", () => {
  const tracked = { status: "tracked", followup_report: "LCS-ct-2", followup_label: "LR3", followup_next_step: "LDCT in 6 months", child_case_id: childId, child_state: "open", child_closed: false, child_overdue: false, child_due_day: 281, child_deadline_day: 311 };

  it("types the contract fields", () => {
    expect(LdctTracking.parse(tracked)).toMatchObject({ child_case_id: childId, child_due_day: 281 });
    expect(() => LdctTracking.parse({ ...tracked, status: "reopened" })).toThrow();
  });

  it("links the child from the API even when the child is not in the loaded list, with the child's own clock", () => {
    const ob = openObligation(parent(tracked), 120, ldctChildren([]));
    expect(ob).toEqual({ kind: "ldct_tracking", followupReport: "LCS-ct-2", child: childId, childDueDay: 281 });
    expect(isActionable(ob)).toBe(false);
    expect(obligationWords(ob!).next).toBe(`Act on LCS-sr-900 (due ${formatDay(281)}), not this case`);
  });

  it("an untracked parent says a tracking request is needed and creates no task on itself", () => {
    const ob = openObligation(parent({ status: "untracked", followup_report: "LCS-ct-2", followup_label: "LR3" }), 120);
    expect(ob).toEqual({ kind: "ldct_tracking", followupReport: "LCS-ct-2", child: null });
    expect(isActionable(ob)).toBe(false);
    expect(obligationWords(ob!).next).toBe("Open a tracking request for the follow-up scan");
  });

  it("an untracked parent never borrows a scan-id-only match from the list (another provider's loop)", () => {
    const collision = new Map([["LCS-ct-2", "other-centre/ServiceRequest/LCS-sr-5"]]);
    const ob = openObligation(parent({ status: "untracked", followup_report: "LCS-ct-2", followup_label: "LR3" }), 120, collision);
    expect(ob).toEqual({ kind: "ldct_tracking", followupReport: "LCS-ct-2", child: null });
    // Only a payload without ldct_tracking falls back to the list.
    expect(openObligation(parent(undefined), 120, collision)).toMatchObject({ child: "other-centre/ServiceRequest/LCS-sr-5" });
  });
});

describe("the served retrieval task status (issue #177)", () => {
  const pending = { status: "pending", at: "2026-10-01T08:00:00Z", procedure: "colonoscopy", performed: "2026-08", provider: "Synthetic Private Hospital", source: "patient", retrieval_owner: "coordinator", retrieval_due: "2026-10-15" };
  it("accepts the server's due/overdue/ended/evidence_closed status and nothing else", () => {
    for (const s of ["due", "overdue", "ended", "evidence_closed"]) expect(OverlayReconciliation.parse({ ...pending, retrieval_status: s }).retrieval_status).toBe(s);
    expect(OverlayReconciliation.parse(pending).retrieval_status).toBeUndefined();
    expect(() => OverlayReconciliation.parse({ ...pending, retrieval_status: "closed" })).toThrow();
  });
});
