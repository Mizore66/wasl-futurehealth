import { afterEach, describe, expect, it, vi } from "vitest";
import { reviewQueue } from "@/components/review/queue";
import { programmeDetail } from "./programme-cases";

const source = vi.hoisted(() => ({ getCases: vi.fn(), getCase: vi.fn(), getFixtures: vi.fn() }));
vi.mock("@/lib/api/source", () => source);

afterEach(() => vi.clearAllMocks());

describe("reviewQueue", () => {
  it("asks the API for every programme with an open review item", async () => {
    const detail = programmeDetail("lung_ldct");
    detail.case.review = "open";
    detail.review_items = [
      { item: "rv-ldct", loop: detail.case.id, kinds: ["evidence"], pair: [], p: 0, report: detail.evidence[0].report, verdict: "disagree", quote: "", chain: [] },
    ];
    source.getCases.mockResolvedValue({ cases: [detail.case], total: 1, limit: 50, offset: 0, meta: {} });
    source.getCase.mockResolvedValue(detail);

    const queue = await reviewQueue();
    expect(source.getCases).toHaveBeenCalledWith({ review: "open", limit: 50 });
    expect(source.getFixtures).not.toHaveBeenCalled();
    expect(queue.source).toBe("open");
    expect(queue.cases.map((c) => c.case.programme)).toEqual(["lung_ldct"]);
  });

  it("keeps every guided programme and leaves out cervical, which has no answer panel", async () => {
    const programmes = ["crc", "cervical", "lung_nodule", "diabetic_retinopathy", "breast", "lung_ldct", "ifhas"] as const;
    const details = programmes.map((programme, i) => {
      const detail = programmeDetail("lung_ldct");
      detail.case = { ...detail.case, id: `case-${programme}`, programme, review: "open", referral_day: i };
      detail.review_items = [
        { item: `rv-${programme}`, loop: detail.case.id, kinds: ["evidence"], pair: [], p: 0, report: detail.evidence[0].report, verdict: "disagree", quote: "", chain: [] },
      ];
      return detail;
    });
    source.getCases.mockResolvedValue({ cases: details.map((d) => d.case), total: details.length, limit: 50, offset: 0, meta: {} });
    source.getCase.mockImplementation(async (id: string) => details.find((d) => d.case.id === id) ?? null);

    const queue = await reviewQueue();
    expect(queue.source).toBe("open");
    expect(queue.cases.map((c) => c.case.programme)).toEqual(["crc", "lung_nodule", "diabetic_retinopathy", "breast", "lung_ldct", "ifhas"]);
    expect(source.getCase).not.toHaveBeenCalledWith("case-cervical");
  });

  it("falls back to the rebuilt hold case when only cervical cases are open", async () => {
    const detail = programmeDetail("lung_ldct");
    detail.case = { ...detail.case, programme: "cervical", review: "open" };
    source.getCases.mockResolvedValue({ cases: [detail.case], total: 1, limit: 50, offset: 0, meta: {} });
    source.getFixtures.mockResolvedValue({ fixtures: { review_hold: { id: "hold" } } });
    source.getCase.mockResolvedValue(null);

    const queue = await reviewQueue();
    expect(source.getCase).toHaveBeenCalledExactlyOnceWith("hold");
    expect(queue).toEqual({ cases: [], source: "rebuilt" });
  });
});
