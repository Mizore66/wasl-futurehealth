import { describe, expect, it } from "vitest";
import { CaseDetail, ReviewRequest } from "@/lib/api/schema";
import { statusWords } from "@/components/worklist/model";
import { reviewView } from "@/components/review/case-view";
import { caseStage } from "@/lib/domain/stage";
import { loadSample } from "./samples";

function crcDetail() {
  const d = CaseDetail.parse(loadSample("case_evidence_closure.json"));
  d.case.closed = false;
  d.case.state = "awaiting_crc_disposition";
  d.case.crc = {
    schema: "wasl-crc-plan/2", kind: "screening", screening_complete: true, care_state: "awaiting_confirmation",
    obligations: ["gastroenterology", "specialist"].map((outcome, i) => ({
      id: `need-${i}`, outcome, state: "awaiting_confirmation", source_report: "C1", source_version: 1,
      procedure_report: "C1", procedure_version: 1, clock_start: 10, review_item: `crc-review-${i}`,
      expected_head: `head-${i}`, owner: `${outcome}-coordinator`, blockers: [],
    })),
  };
  const template = d.review_items[0];
  d.review_items = d.case.crc.obligations.map((p) => ({ ...template, item: p.review_item, kinds: ["evidence"], reason: "crc_disposition", verdict: p.outcome, head: p.expected_head }));
  return CaseDetail.parse(d);
}

describe("independent CRC plan boundary", () => {
  it("parses per-obligation sources, expected heads and independent review labels", () => {
    const d = crcDetail();
    const v = reviewView(d);
    expect(v.crcItems?.["crc-review-0"].choices).toEqual(["gastroenterology"]);
    expect(v.crcItems?.["crc-review-1"].choices).toEqual(["specialist"]);
    expect(ReviewRequest.parse({ item: "crc-review-0", decision: "gastroenterology" }).decision).toBe("gastroenterology");
    expect(ReviewRequest.safeParse({ item: "crc-review-0", decision: "surveillance:0" }).success).toBe(false);
  });

  it("does not offer adequacy as a substitute for a blocked or superseded obligation", () => {
    const d = crcDetail();
    d.case.crc!.obligations[0].blockers = ["awaiting_histology"];
    d.case.crc!.obligations.splice(1, 1);
    const v = reviewView(d);
    expect(v.crcItems?.["crc-review-0"].choices).toEqual([]);
    expect(v.crcItems?.["crc-review-1"].choices).toEqual([]);
  });

  it("never labels ongoing care as Closed / Nothing to do", () => {
    const d = crcDetail();
    d.case.state = "closed";
    d.case.crc!.care_state = "ongoing";
    expect(statusWords(d.case)).toEqual({ status: "Screening complete, care ongoing", next: "Track each confirmed care pathway" });
    expect(caseStage(d).label).toBe("Screening complete, care ongoing");
  });
});
