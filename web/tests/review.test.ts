import { describe, expect, it } from "vitest";
import { reviewView } from "@/components/review/case-view";
import { failureSentence, outcomeOf, roleFor, signaturesFor } from "@/components/review/decision";
import { CaseDetail } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";
import { programmeDetail } from "./programme-cases";

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

describe("fixtures mode cannot append", () => {
  it("postReview refuses to fabricate an event when there is no live API", async () => {
    const { postReview } = await import("@/lib/api/source");
    await expect(postReview("any", { item: "rv-x", decision: "same", note: "" }, { role: "reviewer", actor: "Dr Omar Al Kaabi" })).rejects.toThrow(
      /needs WASL_API_URL/,
    );
  });
});

describe("programme-specific review copy", () => {
  it("describes LDCT evidence as a screening LDCT, never a colonoscopy", () => {
    const view = reviewView(programmeDetail("lung_ldct"));
    expect(view.guided).toBe(false);
    expect(view.referralSource).toBe("lcs-centre · lung cancer screening referral");
    expect(view.reportSource).toBe("lcs-centre · LDCT report");
    expect(view.trail.map((n) => n.what).join(" ")).not.toContain("colonoscopy");
    expect(view.trail.map((n) => n.what)).toContain("LDCT report LCS-ct-12");

    const disagreed = programmeDetail("lung_ldct");
    disagreed.evidence[0].agree = false;
    expect(reviewView(disagreed).trail.map((n) => n.what).join(" ")).not.toContain("bowel prep");
  });

  it("describes IFHAS evidence as cardiometabolic screening and follow-up", () => {
    const view = reviewView(programmeDetail("ifhas"));
    expect(view.guided).toBe(false);
    expect(view.referralSource).toBe("ifh-screen · IFHAS screening referral");
    expect(view.reportSource).toBe("ifh-screen · IFHAS results report");
    expect(view.trail.map((n) => n.what).join(" ")).not.toContain("colonoscopy");
    expect(view.trail.map((n) => n.what)).toContain("IFHAS results report IFH-dr-100");
  });
});

describe("review view of the review_hold fixture", () => {
  it("takes the two review items and the report from the API", () => {
    expect(view.items).toEqual({ link: "rv-2853a89572f0bc86", evidence: "rv-02bd10f694ad3d6e" });
    expect(view.report.id).toBe("SKM-dr-293");
    expect(view.quote).toBe("advanced to the cecum");
    expect(view.link).toMatchObject({ score: 0.9985, line: 0.99, basis: "name and details" });
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
    expect(view.trail[0]).toMatchObject({ who: "Al Mushrif", when: "21 May" });
  });

  it("carries canonical closure instead of inferring it from appended answers", () => {
    expect(view.closed).toBe(true);
    expect(reviewView({ ...detail, case: { ...detail.case, closed: false } }).closed).toBe(false);
    expect(reviewView({ ...detail, case: { ...detail.case, closed: true } }).closed).toBe(true);
  });
});
