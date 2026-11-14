import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { reviewView } from "@/components/review/case-view";
import { OUTCOMES, outcomeFor, recordedOutcome, signaturesFor } from "@/components/review/decision";
import { ReviewDesk } from "@/components/review/review-desk";
import { ActiveCaseSummary, CaseDetail } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

const queue = vi.hoisted(() => ({ reviewQueue: vi.fn() }));
vi.mock("@/components/review/queue", () => queue);
vi.mock("@/lib/api/source", () => ({ apiMode: () => "fixtures" }));
vi.mock("@/lib/api/reviewer-session", () => ({ reviewerToken: async () => undefined }));

function purposeCase(reason: "shadow_confirm" | "auto_audit"): CaseDetail {
  const base = CaseDetail.extend({ case: ActiveCaseSummary }).parse(hold);
  const item = base.review_items.find((i) => i.kinds.includes("evidence"))!;
  return {
    ...base,
    case: { ...base.case, closed: reason === "auto_audit", review: "open" },
    evidence: base.evidence.map((e) => ({ ...e, agree: true })),
    review_items: [{ ...item, reason }],
  };
}

describe("review purpose", () => {
  it("labels a shadow confirmation as an open suggested closure", () => {
    const view = reviewView(purposeCase("shadow_confirm"));
    expect(view.purpose).toBe("shadow_confirm");
    expect(view.held).toMatch(/suggested automatic closure/i);
    expect(view.held).toMatch(/confirm/i);
    expect(view.held).not.toMatch(/^Held:/);
  });

  it("labels an audit as post-closure, shows the closed state, and never calls it held", () => {
    const view = reviewView(purposeCase("auto_audit"));
    expect(view.purpose).toBe("auto_audit");
    expect(view.closed).toBe(true);
    expect(view.held).toMatch(/post-closure audit/i);
    expect(view.held).toMatch(/closed/);
    expect(view.held).toMatch(/reopens/);
    expect(view.held).not.toMatch(/held|blocks|Nothing closes/i);
  });

  it("adds no purpose to an ordinary evidence review", () => {
    const view = reviewView(CaseDetail.parse(hold));
    expect(view.purpose).toBeUndefined();
  });

  it("words the previews for each purpose and answer", () => {
    expect(outcomeFor("close", "shadow_confirm").preview).toBe("Would confirm the suggested closure");
    expect(outcomeFor("close", "shadow_confirm").text("Mariam", "Noor", false)).toMatch(/permits closure/);
    expect(outcomeFor("inadequate", "shadow_confirm").preview).toMatch(/Would not confirm/);
    expect(outcomeFor("close", "auto_audit").preview).toBe("Would leave the case closed");
    expect(outcomeFor("close", "auto_audit").text("Mariam", "Noor", false)).toMatch(/stays closed/);
    expect(outcomeFor("inadequate", "auto_audit").preview).toBe("Would reopen the case");
    expect(outcomeFor("close", undefined)).toBe(OUTCOMES.close);
  });

  it("reports only the appended answer unless refreshed data confirms a change", () => {
    const sigs = signaturesFor({ report: "adequate" }, { evidence: "rv-evidence" });
    expect(recordedOutcome("close", sigs, false, "shadow_confirm").recordedText).toBe(
      "Report adequate was appended to the demo review log. Current case data still shows the case as open; closure is not confirmed.",
    );
    expect(recordedOutcome("close", sigs, true, "shadow_confirm").recordedText).toContain("Refreshed case data confirms closure.");
    const agree = recordedOutcome("close", sigs, false, "auto_audit").recordedText;
    expect(agree).toMatch(/appended to the demo review log/);
    expect(agree).toMatch(/not changed/);
    const disagree = recordedOutcome("inadequate", signaturesFor({ report: "inadequate" }, { evidence: "rv-evidence" }), false, "auto_audit").recordedText;
    expect(disagree).toMatch(/appended to the demo review log/);
    expect(disagree).toMatch(/reopening is not confirmed/);
  });

  it.each(["shadow_confirm", "auto_audit"] as const)("renders the %s desk without the generic close copy", (reason) => {
    const view = reviewView(purposeCase(reason));
    const markup = renderToStaticMarkup(createElement(ReviewDesk, { view, mode: "fixtures", next: { href: "/app/review", label: "Back", last: true } }));
    expect(markup).not.toContain("Close the case");
  });
});

describe("review queue page", () => {
  it("shows each purpose in the queue and drops the universal claim", async () => {
    const shadow = purposeCase("shadow_confirm");
    const audit = purposeCase("auto_audit");
    audit.case = { ...audit.case, id: "audit-case" };
    queue.reviewQueue.mockResolvedValue({ cases: [shadow, audit], source: "open" });
    const { default: Page } = await import("@/app/app/review/page");
    const markup = renderToStaticMarkup(await Page({ searchParams: Promise.resolve({}) } as never));
    expect(markup).not.toContain("Nothing closes without it");
    expect(markup).toMatch(/Suggested closure/);
    expect(markup).toMatch(/Post-closure audit/);
  });
});
