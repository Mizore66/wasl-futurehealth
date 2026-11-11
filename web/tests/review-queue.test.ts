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
});
