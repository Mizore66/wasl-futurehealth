import { describe, expect, it } from "vitest";
import { reviewView } from "@/components/review/case-view";
import { failureSentence, outcomeOf, roleFor, signaturesFor } from "@/components/review/decision";
import { CaseDetail } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

const detail = CaseDetail.parse(hold);
const view = reviewView(detail);
const items = { link: "rv-link", evidence: "rv-evidence" };

describe("review decision", () => {
  it("has no outcome until identity, then prep for the same patient, are answered", () => {
    expect(outcomeOf({})).toBeUndefined();
    expect(outcomeOf({ identity: "same" })).toBeUndefined();
    expect(outcomeOf({ identity: "same", prep: "adequate" })).toBe("close");
    expect(outcomeOf({ identity: "different" })).toBe("notSame");
  });

  it("signs identity as reviewer first, then prep as clinician", () => {
    const sigs = signaturesFor({ identity: "same", prep: "adequate" }, items);
    expect(sigs).toEqual([
      { item: "rv-link", decision: "same" },
      { item: "rv-evidence", decision: "adequate" },
    ]);
    expect(sigs.map((s) => roleFor(s.decision))).toEqual(["reviewer", "clinician"]);
  });

  it("signs one item for not-the-same and for ask-the-unit, which has no API decision", () => {
    expect(signaturesFor({ identity: "different", prep: "adequate" }, items)).toEqual([{ item: "rv-link", decision: "different" }]);
    expect(signaturesFor({ identity: "same", prep: "ask" }, items)).toEqual([{ item: "rv-link", decision: "same" }]);
    expect(signaturesFor({ identity: "same" }, items)).toEqual([]);
  });

  it("turns 403, 404 and 400 into plain sentences", () => {
    const prep = { item: "rv-evidence", decision: "adequate" } as const;
    expect(failureSentence(prep, { status: 403, error: "rbac" })).toBe("Not saved: the clinician role is not allowed to sign the bowel prep answer.");
    expect(failureSentence({ item: "x", decision: "same" }, { status: 404, error: "nope" })).toBe(
      "Not saved: this case has no open review item for the identity answer.",
    );
    expect(failureSentence(prep, { status: 400, error: "X-Wasl-Actor header required" })).toContain("could not read the bowel prep answer");
  });
});

describe("review view of the review_hold fixture", () => {
  it("takes the two review items and the report from the API", () => {
    expect(view.items).toEqual({ link: "rv-f0715223bce81123", evidence: "rv-25ead56223c7bef0" });
    expect(view.report.id).toBe("NOO-dr-391");
    expect(view.quote).toBe("advanced, under direct vision, to the cecum");
    expect(view.link).toMatchObject({ score: 1, line: 0.99, basis: "Emirates ID" });
  });

  it("shows the missing date of birth as not on report", () => {
    expect(view.identity.find((r) => r.field === "Date of birth")).toMatchObject({ report: "not on report", verdict: "missing" });
  });

  it("puts only bowel prep up for decision", () => {
    expect(view.checks.filter((c) => c.disputed).map((c) => c.field)).toEqual(["Prep graded"]);
    expect(view.checks.map((c) => c.decision)).toEqual(["agreed", "disputed", "agreed_none", "not_needed"]);
  });

  it("draws the trail from timeline days, with no signature yet", () => {
    expect(view.trail.every((n) => n.kind === "system")).toBe(true);
    expect(view.trail[0]).toMatchObject({ who: "Al Mushrif", when: "17 May" });
  });
});
