import { describe, expect, it } from "vitest";
import { reviewItemViews } from "@/components/review/case-view";
import { CaseDetail } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

// Isolated software controls, not signed clinical evidence or a producer claim.
// They preserve the existing schema and represent the already documented cases
// in which a raised item's source is absent from canonical identity/timeline rows.
const base = CaseDetail.parse(hold);
const template = base.review_items.find((item) => item.kinds.includes("evidence"))!;
const referral = "ahs-mushrif|AHS-MRN100001";
const unrelated = "noor-endo|NOO-MRN300001";
const actualReport = "skmc-endo/DiagnosticReport/SKM-item-source";

function detailForItem(timeline: CaseDetail["timeline"] = []): CaseDetail {
  const ownEvidence = {
    ...base.evidence[0],
    report: actualReport,
    agree: false,
    quote: "the current item's own report quote",
    fields: { regex: "yes/adequate/no/no", reader: "no/adequate/no/no" },
  };
  return CaseDetail.parse({
    ...base,
    identities: [
      { provider: "ahs-mushrif", registration: referral, role: "referral" },
      { provider: "noor-endo", registration: unrelated, role: "downstream" },
    ],
    evidence: [{ ...base.evidence[0], report: "noor-endo/DiagnosticReport/NOO-unrelated", quote: "an unrelated canonical report" }],
    timeline,
    review_items: [{
      ...template,
      item: "rv-source-metadata",
      kinds: ["evidence"],
      pair: [referral, "skmc-endo|SKM-report-only"],
      report: actualReport,
      report_version: 2,
      chain: [{ type: "evidence", body: ownEvidence, prev: "genesis", hash: "7".repeat(64) }],
    }],
  });
}

describe("evidence cards do not borrow another source's metadata", () => {
  it("labels the item's provider-qualified report with its own provider, not the first canonical downstream record", () => {
    const view = reviewItemViews(detailForItem())["rv-source-metadata"];
    expect(view.quote).toBe("the current item's own report quote");
    expect(view.report.id).toBe("SKM-item-source");
    expect(view.unit).toBe("SKMC endoscopy");
    expect(view.reportSource).toBe("SKMC endoscopy · colonoscopy report");
    expect(view.reportRegistration).not.toBe(unrelated);
    expect(view.reportSource).not.toContain("Noor");
  });

  it("leaves a revised report's date unavailable when same-ID timeline rows have no version association", () => {
    const view = reviewItemViews(detailForItem([
      { id: actualReport, type: "DiagnosticReport", provider: "skmc-endo", day: 2, status: "final" },
      { id: actualReport, type: "DiagnosticReport", provider: "skmc-endo", day: 5, status: "corrected" },
    ]))["rv-source-metadata"];
    expect(view.report.version).toBe(2);
    expect(view.quote).toBe("the current item's own report quote");
    // Neither first-row nor last-row date establishes which version was judged.
    expect(view.report.on).toBe("");
    expect(view.trail.find((entry) => entry.id === "report")).toBeUndefined();
  });

  it("does not infer a version2 date from a lone unversioned historical row", () => {
    const view = reviewItemViews(detailForItem([
      { id: actualReport, type: "DiagnosticReport", provider: "skmc-endo", day: 2, status: "final" },
    ]))["rv-source-metadata"];
    expect(view.report.version).toBe(2);
    expect(view.report.on).toBe("");
  });

  it("preserves a uniquely identified date for a legacy item that makes no version claim", () => {
    const detail = detailForItem([
      { id: actualReport, type: "DiagnosticReport", provider: "skmc-endo", day: 2, status: "final" },
    ]);
    delete detail.review_items[0].report_version;
    const view = reviewItemViews(detail)["rv-source-metadata"];
    expect(view.report.on).not.toBe("");
    expect(view.trail.find((entry) => entry.id === "report")).toBeDefined();
  });
});
