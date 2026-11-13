import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { reviewView } from "@/components/review/case-view";
import { OUTCOMES, outcomeOf, signaturesFor } from "@/components/review/decision";
import { ReviewDesk } from "@/components/review/review-desk";
import { CaseDetail, ReviewState } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

vi.mock("@/app/app/review/[id]/actions", () => ({ claimReviewQuestion: vi.fn(), submitReviewRead: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }) }));

const base = CaseDetail.parse(hold);
const EVIDENCE = "rv-d55f7ccf9f12c409";
const next = { href: "/app/review", label: "Back to Review", last: true };

function withReason(reason: string | undefined, tissue = true): CaseDetail {
  const detail = structuredClone(base);
  detail.review_items = detail.review_items.filter((i) => i.kinds.includes("evidence")).map((i) => ({ ...i, reason }));
  const fields = `yes/not_stated/${tissue ? "yes" : "no"}/no`;
  detail.evidence[0].fields = { ...detail.evidence[0].fields, regex: fields, reader: fields };
  return detail;
}

const state = ReviewState.parse({
  item: EVIDENCE, question: "evidence", state: "awaiting_r1", head: "", next_read: "r1", may_submit: true, claim_head: "c".repeat(64),
  claim: { owner: "Rev-A", role: "clinician", read: "r1", claimed_at: "2026-11-13T10:00:00Z", heartbeat_at: "2026-11-13T10:00:00Z", expires_at: "2026-11-13T10:05:00Z", status: "active" },
  claim_actions: { claim: false, renew: true, release: true, takeover: false },
  reads: [{ read: "r1", submitted: false }], disagreement: false, canonical_unchanged: true,
});

const render = (detail: CaseDetail, mode: "live" | "fixtures") =>
  renderToStaticMarkup(createElement(ReviewDesk, { view: reviewView(detail), mode, next, states: mode === "live" ? [state] : undefined }));

const CASES = [
  ["prep_ungraded_day15", "Was the bowel preparation adequate?", /bowel prep/i],
  ["completion_undocumented", "Was the caecum reached?", /caecum/i],
] as const;

describe.each(CASES)("%s review", (reason, question, scope) => {
  it.each(["live", "fixtures"] as const)("asks its own question in %s mode without whole-case closure copy", (mode) => {
    const markup = render(withReason(reason), mode);
    expect(markup).toContain(question);
    expect(markup).toContain("settles only this question");
    expect(markup).toMatch(/histology/i);
    expect(markup).toMatch(scope);
    expect(markup).not.toContain("Is the colonoscopy report adequate?");
    expect(markup).not.toContain("enough to close her case");
    expect(markup).not.toContain("Close the case");
  });

  it("keeps the adequate answer but previews only the narrow resolution, with tissue taken and histology pending", () => {
    const view = reviewView(withReason(reason, true));
    const key = outcomeOf({ report: "adequate" }, view.items);
    expect(signaturesFor({ report: "adequate" }, view.items)).toEqual([{ item: EVIDENCE, decision: "adequate" }]);
    const outcome = OUTCOMES[key!];
    const words = [outcome.commit, outcome.preview, outcome.text("Maryam", view.unit, false), outcome.recordedText].join(" ");
    expect(words).not.toMatch(/clos/i);
    expect(words).toMatch(/histology/i);
  });
});

describe("ordinary CRC evidence review", () => {
  it.each([undefined, "evidence_disputed"])("keeps the generic wording and closure preview for reason %s", (reason) => {
    const detail = withReason(reason);
    const view = reviewView(detail);
    expect(view.evidenceQuestion).toBe("Is the colonoscopy report adequate?");
    expect(outcomeOf({ report: "adequate" }, view.items)).toBe("close");
    for (const mode of ["live", "fixtures"] as const) {
      const markup = render(detail, mode);
      expect(markup).toContain("Is the colonoscopy report adequate?");
      expect(markup).toContain("enough to close her case");
      expect(markup).not.toContain("settles only this question");
    }
  });
});
