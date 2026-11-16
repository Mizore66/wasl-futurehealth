import { describe, expect, it } from "vitest";
import { reviewView } from "@/components/review/case-view";
import { outcomeOf } from "@/components/review/decision";
import { ActiveCaseSummary, CaseDetail } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

function onlyEvidencePurpose(reason: string): CaseDetail {
  const detail = CaseDetail.extend({ case: ActiveCaseSummary }).parse(hold);
  const item = detail.review_items.find((candidate) => candidate.kinds.includes("evidence"))!;
  return { ...detail, case: { ...detail.case, closed: reason === "auto_audit" }, review_items: [{ ...item, reason }] };
}

describe("CRC narrow questions and pilot purposes compose independently", () => {
  it.each([
    ["completion_undocumented", "Caecum reached"],
    ["prep_ungraded_day15", "Prep graded"],
  ] as const)("keeps %s scoped without assigning a pilot purpose", (reason, field) => {
    const view = reviewView(onlyEvidencePurpose(reason));
    expect(view.purpose).toBeUndefined();
    expect(view.evidenceField).toBe(field);
    expect(view.evidenceScope).toContain("settles only this question");
    expect(view.items.scoped).toBe(true);
    expect(outcomeOf({ report: "adequate" }, view.items)).toBe("scoped");
  });

  it.each(["shadow_confirm", "auto_audit"] as const)("keeps %s out of the CRC narrow-question outcome", (reason) => {
    const view = reviewView(onlyEvidencePurpose(reason));
    expect(view.purpose).toBe(reason);
    expect(view.evidenceField).toBeUndefined();
    expect(view.evidenceScope).toBeUndefined();
    expect(view.items.scoped).toBeUndefined();
    expect(outcomeOf({ report: "adequate" }, view.items)).toBe("close");
  });
});
