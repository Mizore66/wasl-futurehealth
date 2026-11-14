import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { reviewView } from "@/components/review/case-view";
import { OUTCOMES, outcomeOf } from "@/components/review/decision";
import { ReviewDesk } from "@/components/review/review-desk";
import { CaseDetail, ReviewState } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

vi.mock("@/app/app/review/[id]/actions", () => ({ claimReviewQuestion: vi.fn(), submitReviewRead: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }) }));

const base = CaseDetail.parse(hold);
const EVIDENCE = "rv-d55f7ccf9f12c409";
const next = { href: "/app/review", label: "Back to Review", last: true };

/* Supported reader fields only: caecum/prep/tissue/histology. Both readers leave the purpose row unsettled and disagree on histology. */
const FIELDS = {
  completion_undocumented: { regex: "not_stated/adequate/yes/yes", reader: "not_stated/adequate/yes/no" },
  prep_ungraded_day15: { regex: "yes/not_stated/yes/yes", reader: "yes/not_stated/yes/no" },
} as const;

function detailFor(reason: string | undefined): CaseDetail {
  const detail = structuredClone(base);
  detail.review_items = detail.review_items.filter((i) => i.kinds.includes("evidence")).map((i) => ({ ...i, reason }));
  const f = FIELDS[reason as keyof typeof FIELDS] ?? { regex: "yes/not_stated/yes/no", reader: "yes/not_stated/yes/no" };
  detail.evidence[0].fields = { ...detail.evidence[0].fields, ...f };
  return detail;
}

const resolved = (decision: string, how: "resolved_agreed" | "resolved_senior") =>
  ReviewState.parse({
    item: EVIDENCE, question: "evidence", state: how, head: "", next_read: null, may_submit: false, final_decision: decision,
    reads: [{ read: "r1", submitted: true, decision }, { read: "r2", submitted: true, decision }], disagreement: how === "resolved_senior", canonical_unchanged: true,
  });
const pending = ReviewState.parse({
  item: EVIDENCE, question: "evidence", state: "awaiting_r1", head: "", next_read: "r1", may_submit: true, claim_head: "c".repeat(64),
  claim: { owner: "Rev-A", role: "clinician", read: "r1", claimed_at: "2026-11-13T10:00:00Z", heartbeat_at: "2026-11-13T10:00:00Z", expires_at: "2026-11-13T10:05:00Z", status: "active" },
  claim_actions: { claim: false, renew: true, release: true, takeover: false },
  reads: [{ read: "r1", submitted: false }], disagreement: false, canonical_unchanged: true,
});

const render = (detail: CaseDetail, mode: "live" | "fixtures", states?: ReviewState[]) =>
  renderToStaticMarkup(createElement(ReviewDesk, { view: reviewView(detail), mode, next, states }));

const row = (markup: string, field: string) => {
  const start = markup.indexOf(`data-check="${field}"`);
  expect(start).toBeGreaterThan(-1);
  const end = markup.indexOf("data-check=", start + 10);
  return markup.slice(start, end === -1 ? undefined : end);
};
const OWN = { completion_undocumented: "Caecum reached", prep_ungraded_day15: "Prep graded" } as const;

describe.each(["completion_undocumented", "prep_ungraded_day15"] as const)("%s settles only its own row", (reason) => {
  const detail = detailFor(reason);

  it("fixture preview signs only the purpose row and leaves histology disputed", () => {
    const view = reviewView(detail);
    expect(view.checks.find((c) => c.field === "Histology back")?.disputed).toBe(true);
    expect(view.checks.find((c) => c.field === OWN[reason])?.disputed).toBe(true);
    expect(view.evidenceField).toBe(OWN[reason]);
  });

  it.each(["resolved_agreed", "resolved_senior"] as const)("live %s: own row is resolved, histology is not", (how) => {
    for (const decision of ["adequate", "inadequate"]) {
      const markup = render(detail, "live", [resolved(decision, how)]);
      expect(row(markup, OWN[reason])).toMatch(/resolved/);
      expect(row(markup, "Histology back")).toContain("reviewers decide");
      expect(row(markup, "Histology back")).not.toMatch(/resolved|signed/);
      expect(markup).not.toContain("Every question is resolved.");
      expect(markup).not.toMatch(/Report adequate|Report not adequate/);
      expect(markup).toContain("Histology and other checks still decide the case");
      expect(markup).not.toContain("Close the case");
    }
  });

  it("live pending first read warns that the answer settles only this question and keeps wire values", () => {
    const markup = render(detail, "live", [pending]);
    expect(markup).toContain("Your answer settles only this question");
    expect(markup).toMatch(/histology/i);
    expect(markup).not.toContain("One answer covers");
    expect(markup).not.toContain("Is the colonoscopy report adequate?");
  });

  it("fixture mode never promises that one answer covers histology", () => {
    const markup = render(detail, "fixtures");
    expect(markup).not.toContain("One answer covers");
    expect(markup).toContain("Your answer settles only this question");
    expect(markup).not.toContain("adequate, signed");
  });

  it("not adequate previews no repeat booking, no report verdict, and keeps the wire decision", () => {
    const view = reviewView(detail);
    const key = outcomeOf({ report: "inadequate" }, view.items);
    expect(key).toBe("scopedNo");
    const o = OUTCOMES[key!];
    const words = [o.commit, o.preview, o.text("Maryam", view.unit, false), o.recordedText, o.next(view.unit).what].join(" ");
    expect(words).not.toMatch(/repeat colonoscopy|repeat booking is confirmed|report not adequate|clos/i);
    expect(words).toMatch(/histology/i);
  });
});

describe("ordinary CRC evidence keeps whole-row resolution", () => {
  it.each([undefined, "evidence_disputed"])("reason %s resolves every disputed row and keeps report wording", (reason) => {
    const detail = detailFor(reason);
    detail.evidence[0].fields = { ...detail.evidence[0].fields, regex: "yes/not_stated/yes/yes", reader: "yes/adequate/yes/no" };
    const markup = render(detail, "live", [resolved("adequate", "resolved_agreed")]);
    expect(row(markup, "Prep graded")).toContain("adequate, resolved");
    expect(row(markup, "Histology back")).toContain("adequate, resolved");
    expect(markup).toContain("Every question is resolved.");
    expect(markup).toContain("Report adequate");
    expect(outcomeOf({ report: "inadequate" }, reviewView(detail).items)).toBe("inadequate");
  });
});
