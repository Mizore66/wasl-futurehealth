import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { reviewView } from "@/components/review/case-view";
import { failureSentence, outcomeOf, OUTCOMES, recordedOutcome, roleFor, signaturesFor } from "@/components/review/decision";
import { ReviewDesk } from "@/components/review/review-desk";
import { ActiveCaseSummary, CaseDetail } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";
import multiReport from "@/data/api/case_dr_exam_closure.json";
import retainedConflict from "./fixtures/retained/conflict-case.json";
import retainedRetired from "./fixtures/retained/retired-case.json";
import { programmeDetail } from "./programme-cases";

const detail = CaseDetail.extend({ case: ActiveCaseSummary }).parse(hold);
const view = reviewView(detail);
const items = { link: "rv-link", evidence: "rv-evidence" };

describe("review lifecycle boundary", () => {
  it("accepts legacy active cases without lifecycle or inactive metadata", () => {
    const legacy = CaseDetail.parse({ ...detail, case: { ...detail.case, lifecycle: undefined, inactive: undefined } });
    expect(legacy.case.lifecycle).toBeUndefined();
    expect(legacy.case.inactive).toBeUndefined();
    expect(reviewView(legacy).programme).toBe("crc");
  });

  it.each([
    { scenario: "retired", source: retainedRetired },
    { scenario: "conflict", source: retainedConflict },
  ])("rejects the actual $scenario retained case before building active review copy", ({ source }) => {
    const retained = CaseDetail.parse(source);
    expect(retained.case.lifecycle).toBe("retained");
    expect(() => reviewView(retained)).toThrow("Retained cases have no current review work");
  });
});

describe("review decision", () => {
  it("has no outcome until identity, then the report for the same patient, are answered", () => {
    expect(outcomeOf({})).toBeUndefined();
    expect(outcomeOf({ identity: "same" })).toBeUndefined();
    expect(outcomeOf({ identity: "same", report: "adequate" })).toBe("close");
    expect(outcomeOf({ identity: "different" })).toBe("notSame");
  });

  it("signs identity as reviewer first, then the report evidence as clinician", () => {
    const sigs = signaturesFor({ identity: "same", report: "adequate" }, items);
    expect(sigs).toEqual([
      { item: "rv-link", decision: "same" },
      { item: "rv-evidence", decision: "adequate" },
    ]);
    expect(sigs.map((s) => roleFor(s.decision))).toEqual(["reviewer", "clinician"]);
  });

  it("signs one item for not-the-same and for ask-the-unit, which has no API decision", () => {
    expect(signaturesFor({ identity: "different", report: "adequate" }, items)).toEqual([{ item: "rv-link", decision: "different" }]);
    expect(signaturesFor({ identity: "same", report: "ask" }, items)).toEqual([{ item: "rv-link", decision: "same" }]);
    expect(signaturesFor({ identity: "same" }, items)).toEqual([]);
  });

  it("signs a link-only case without requiring an evidence answer", () => {
    expect(outcomeOf({ identity: "same" }, { link: "rv-link" })).toBe("linkSame");
    expect(signaturesFor({ identity: "same" }, { link: "rv-link" })).toEqual([{ item: "rv-link", decision: "same" }]);
  });

  it("signs an evidence-only CRC case without inventing a link answer", () => {
    expect(outcomeOf({ report: "adequate" }, { evidence: "rv-evidence" })).toBe("close");
    const signatures = signaturesFor({ report: "adequate" }, { evidence: "rv-evidence" });
    expect(signatures).toEqual([{ item: "rv-evidence", decision: "adequate" }]);
    expect(recordedOutcome("close", signatures)).toEqual({
      recordedText: "Report adequate was appended to the demo review log. Current case data still shows the case as open; closure is not confirmed.",
      recordedAnswerCell: { word: "adequate, signed", tone: "positive" },
    });
    expect(OUTCOMES.close.next("Noor endoscopy", false)).toEqual({ who: "Case data", what: "no downstream effect inferred" });
  });

  it("signs a non-CRC evidence label without turning it into a CRC closure answer", () => {
    expect(outcomeOf({ report: "wk_benign" }, { evidence: "rv-ldct" })).toBe("evidence");
    const signatures = signaturesFor({ report: "wk_benign" }, { evidence: "rv-ldct" });
    expect(signatures).toEqual([{ item: "rv-ldct", decision: "wk_benign" }]);
    expect(recordedOutcome("evidence", signatures).recordedText).toBe(
      "Evidence answer wk_benign was appended to the demo review log. No downstream effect is confirmed.",
    );
  });

  it.each([
    { report: "adequate", outcome: "close", remaining: "Current case data still shows the case as open; closure is not confirmed." },
    { report: "inadequate", outcome: "inadequate", remaining: "No repeat booking is confirmed." },
  ] as const)("records only identity for a link-only $outcome outcome", ({ report, outcome, remaining }) => {
    const linkOnly = { link: "rv-link" };
    const signatures = signaturesFor({ identity: "same", report }, linkOnly);
    expect(signatures).toEqual([{ item: "rv-link", decision: "same" }]);

    const recorded = recordedOutcome(outcome, signatures);
    expect(recorded.recordedText).toBe(`Same patient was appended to the demo review log. No report answer was appended. ${remaining}`);
    expect(recorded.recordedText).not.toContain("report adequate were appended");
    expect(recorded.recordedText).not.toContain("report not adequate were appended");
    expect(recorded.recordedAnswerCell).toEqual({ word: "not signed", tone: "muted" });
  });

  it("keeps canonical closure confirmation without inventing a link-only report signature", () => {
    const identityOnly = signaturesFor({ identity: "same", report: "adequate" }, { link: "rv-link" });
    expect(recordedOutcome("close", identityOnly, true)).toEqual({
      recordedText: "Same patient was appended to the demo review log. Refreshed case data confirms closure.",
      recordedAnswerCell: { word: "not signed", tone: "muted" },
    });
    expect(recordedOutcome("close", signaturesFor({ identity: "same", report: "adequate" }, items), true).recordedText).toBe(
      "Same patient and report adequate were appended to the demo review log. Refreshed case data confirms closure.",
    );
  });

  it("keeps the complete close and inadequate wording when both answers were recorded", () => {
    expect(recordedOutcome("close", signaturesFor({ identity: "same", report: "adequate" }, items))).toEqual({
      recordedText: "Same patient and report adequate were appended to the demo review log. Current case data still shows the case as open; closure is not confirmed.",
      recordedAnswerCell: { word: "adequate, signed", tone: "positive" },
    });
    expect(recordedOutcome("inadequate", signaturesFor({ identity: "same", report: "inadequate" }, items))).toEqual({
      recordedText: "Same patient and report not adequate were appended to the demo review log. No repeat booking is confirmed.",
      recordedAnswerCell: { word: "not adequate, signed", tone: "negative" },
    });
  });

  it("turns 403, 404 and 400 into plain sentences", () => {
    const report = { item: "rv-evidence", decision: "adequate" } as const;
    expect(failureSentence(report, { status: 403, error: "rbac" })).toBe("Not saved: the clinician role is not allowed to sign the report answer.");
    expect(failureSentence({ item: "x", decision: "same" }, { status: 404, error: "nope" })).toBe(
      "Not saved: this case has no open review item for the identity answer.",
    );
    expect(failureSentence(report, { status: 400, error: "X-Wasl-Actor header required" })).toContain("could not read the report answer");
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

describe("evidence-only CRC review desk", () => {
  it("offers only evidence answers that can produce an outcome", () => {
    const evidenceOnly = reviewView({
      ...detail,
      review_items: detail.review_items.filter((item) => item.kinds.includes("evidence")),
    });
    const markup = renderToStaticMarkup(
      createElement(ReviewDesk, {
        view: evidenceOnly,
        mode: "fixtures",
        next: { href: "/app/review", label: "Back to Review", last: true },
      }),
    );

    expect(markup).toContain("Answer the evidence question to sign.");
    expect(markup).not.toContain("Ask the unit");
    expect(markup).not.toContain("records the identity answer only");
  });
});

describe("programme-specific review copy", () => {
  it.each(["lung_ldct", "ifhas"] as const)("does not invent a bowel-prep hold for a closed, agreeing %s case", (programme) => {
    const detail = programmeDetail(programme);
    expect(detail.case.closed).toBe(true);
    expect(detail.evidence[0].agree).toBe(true);

    const view = reviewView(detail);
    expect(view.held).toBe("");
    expect(view.checks).toEqual([]);
  });

  it.each(["lung_ldct", "ifhas"] as const)("derives a %s disagreement hold from its own evidence", (programme) => {
    const detail = programmeDetail(programme);
    detail.case.closed = false;
    detail.evidence[0].agree = false;

    const report = programme === "lung_ldct" ? "LDCT report" : "IFHAS results report";
    expect(reviewView(detail).held).toBe(`Held: ${report} readers disagree.`);
  });

  it("describes LDCT evidence as a screening LDCT, never a colonoscopy", () => {
    const view = reviewView(programmeDetail("lung_ldct"));
    expect(view.guided).toBe(true);
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
    expect(view.guided).toBe(true);
    expect(view.referralSource).toBe("ifh-screen · IFHAS screening referral");
    expect(view.reportSource).toBe("ifh-screen · IFHAS results report");
    expect(view.trail.map((n) => n.what).join(" ")).not.toContain("colonoscopy");
    expect(view.trail.map((n) => n.what)).toContain("IFHAS results report IFH-dr-100");
  });
});

describe("multi-report evidence selection", () => {
  it("uses the report named by the open evidence item throughout the review view", () => {
    const open = CaseDetail.parse(multiReport);
    const openItem = detail.review_items.find((item) => item.kinds.includes("evidence"))!;
    open.case.closed = false;
    open.review_items = [{ ...openItem, report: open.evidence[1].report }];
    open.evidence[0].quote = "first report quote";
    open.evidence[1].quote = "later report quote";
    open.evidence[1].agree = false;

    const selected = reviewView(open);
    expect(selected.report.id).toBe("EYE-dr-83");
    expect(selected.quote).toBe("later report quote");
    expect(selected.held).toBe("Held: eye exam report readers disagree.");
    expect(selected.trail.map((node) => node.what)).toContain("eye exam report EYE-dr-83");
    expect(selected.trail.map((node) => node.what)).toContain("eye exam report readers disagree; needs a person");
  });
});

describe("review view of the review_hold fixture", () => {
  it("takes the two review items and the report from the API", () => {
    // data/api/case_review_hold.json: review_items[0] is the link item (reason second_identifier), [1] the evidence item.
    expect(detail.review_items.map((r) => [r.kinds, r.item])).toEqual([
      [["link"], "rv-636161dc42837e08"],
      [["evidence"], "rv-d55f7ccf9f12c409"],
    ]);
    expect(view.items).toEqual({ link: "rv-636161dc42837e08", evidence: "rv-d55f7ccf9f12c409" });
    expect(view.report.id).toBe("SKM-dr-217");
    expect(view.quote).toBe("the level of the cecum");
    // links[0]: path_p_cal 1, tau 0.99, weakest edge basis "eid". The guard still holds it: the pair has no confirmed second identifier.
    expect(view.link).toMatchObject({ score: 1, line: 0.99, basis: "Emirates ID" });
  });

  it("shows the unconfirmed second identifier, not a date of birth the API never says is missing", () => {
    expect(view.identity.find((r) => r.field === "Second identifier")).toMatchObject({ report: "not confirmed", verdict: "missing" });
    expect(view.identity.some((r) => r.field === "Date of birth")).toBe(false);
    expect(view.held).toBe(
      "Held: no second identifier is confirmed for the two records, and the two readers could not settle the bowel prep grade and whether tissue was taken.",
    );
  });

  it("is the guard-only review_hold: outreach stopped by the second-identifier guard, no consent held then granted", () => {
    const outreach = detail.decisions.flatMap((d) => (Array.isArray(d.outreach) ? (d.outreach as { suppress?: string; guard?: string }[]) : []));
    expect(outreach.map((o) => [o.suppress, o.guard])).toEqual([["second_identifier_unconfirmed", "held"]]);
    expect(detail.decisions.flatMap((d) => d.consent_log)).toEqual([]);
    expect(detail.review_items[0].reason).toBe("second_identifier");
  });

  it("puts bowel prep and tissue up for decision, under the one evidence item", () => {
    // fields: reader yes/not_stated/yes/no, rules reader yes/not_stated/no/no.
    expect(view.checks.filter((c) => c.disputed).map((c) => c.field)).toEqual(["Prep graded", "Tissue taken"]);
    expect(view.checks.map((c) => c.decision)).toEqual(["agreed", "disputed", "disputed", "agreed_none"]);
    expect(view.disputed.map((d) => d.says)).toEqual([
      "neither reader found the bowel prep grade",
      "the readers differ on whether tissue was taken: rules reader none, second reader yes",
    ]);
    expect(detail.review_items.filter((r) => r.kinds.includes("evidence"))).toHaveLength(1);
  });

  it("signs the one evidence item once for both disputed checks", () => {
    expect(signaturesFor({ identity: "same", report: "adequate" }, view.items)).toEqual([
      { item: "rv-636161dc42837e08", decision: "same" },
      { item: "rv-d55f7ccf9f12c409", decision: "adequate" },
    ]);
  });

  it("draws the trail from timeline days, with no signature yet", () => {
    expect(view.trail.every((n) => n.kind === "system")).toBe(true);
    // referral_day 17.1095 (1 May + 17 days = 18 May); the guard met the appointment at arrival_day 24.7666 (25 May), the report at day 30.4762 (31 May).
    expect(view.trail.map((n) => [n.who, n.when])).toEqual([
      ["Al Mushrif", "18 May"],
      ["Mariam", "18 May"],
      ["SKMC endoscopy", "22 May"],
      ["Wasl", "25 May"],
      ["SKMC endoscopy", "31 May"],
      ["Two readers", "31 May"],
    ]);
  });

  it("carries canonical closure instead of inferring it from appended answers", () => {
    expect(view.closed).toBe(true);
    expect(reviewView({ ...detail, case: { ...detail.case, closed: false } }).closed).toBe(false);
    expect(reviewView({ ...detail, case: { ...detail.case, closed: true } }).closed).toBe(true);
  });
});
