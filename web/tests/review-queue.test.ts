import { afterEach, describe, expect, it, vi } from "vitest";
import { reviewQueue } from "@/components/review/queue";
import { claimActionLabel, claimSummary, questionAnchor } from "@/components/review/decision";
import { programmeDetail } from "./programme-cases";
import { ReviewState } from "@/lib/api/schema";

const source = vi.hoisted(() => ({ getCases: vi.fn(), getCase: vi.fn(), getCaseWithToken: vi.fn(), getFixtures: vi.fn() }));
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

  it("keeps every programme, cervical included", async () => {
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
    expect(queue.cases.map((c) => c.case.programme)).toEqual(["crc", "cervical", "lung_nodule", "diabetic_retinopathy", "breast", "lung_ldct", "ifhas"]);
  });

  it("uses the viewer token for signed-in claim projections and never uses it as web_reviewer", async () => {
    const detail = programmeDetail("lung_ldct");
    detail.case.review = "open";
    detail.review_items = [{ item: "rv-ldct", loop: detail.case.id, kinds: ["evidence"], pair: [], p: 0, report: detail.evidence[0].report, verdict: "disagree", quote: "", chain: [] }];
    source.getCases.mockResolvedValue({ cases: [detail.case], total: 1, limit: 50, offset: 0, meta: {} });
    source.getCaseWithToken.mockResolvedValue({ ok: true, detail });
    const queue = await reviewQueue("assigned-token-1234567890");
    expect(queue.cases).toEqual([detail]);
    expect(source.getCaseWithToken).toHaveBeenCalledExactlyOnceWith(detail.case.id, "assigned-token-1234567890");
    expect(source.getCase).not.toHaveBeenCalled();
  });

  it("keeps anonymous details on shared reads and derives no browser-side claim action", async () => {
    const detail = programmeDetail("lung_ldct");
    detail.case.review = "open";
    detail.review_items = [{ item: "rv-ldct", loop: detail.case.id, kinds: ["evidence"], pair: [], p: 0, report: detail.evidence[0].report, verdict: "disagree", quote: "", chain: [] }];
    source.getCases.mockResolvedValue({ cases: [detail.case], total: 1, limit: 50, offset: 0, meta: {} });
    source.getCase.mockResolvedValue(detail);
    await reviewQueue();
    expect(source.getCase).toHaveBeenCalledWith(detail.case.id);
    expect(source.getCaseWithToken).not.toHaveBeenCalled();
  });

  it("formats each question's server-projected owner and action independently", () => {
    const base = { item: "rv-both", state: "awaiting_r1", head: "", next_read: "r1", may_submit: false, reads: [], disagreement: false, canonical_unchanged: true } as const;
    const actions = { claim: true, renew: false, release: false, takeover: false };
    const link = ReviewState.parse({ ...base, question: "link", claim: null, claim_head: "", claim_actions: actions });
    const claim = { owner: "Rev-B", role: "clinician", read: "r1", claimed_at: "2026-11-13T10:00:00Z", heartbeat_at: "2026-11-13T10:00:00Z", expires_at: "2026-11-13T10:05:00Z", status: "expired" };
    const evidence = ReviewState.parse({ ...base, question: "evidence", claim, claim_head: "c".repeat(64), claim_actions: { ...actions, claim: false, takeover: true } });
    expect(claimSummary(link)).toBe("Unclaimed");
    expect(claimActionLabel(link)).toBe("Claim");
    expect(claimSummary(evidence)).toContain("Claim by Rev-B expired at");
    expect(claimActionLabel(evidence)).toBe("Take over");
    expect(questionAnchor(link.item, "link")).not.toBe(questionAnchor(evidence.item, "evidence"));
  });

  const cervical = (id: string, kinds: ("link" | "evidence")[], states?: { state: string }[]) => {
    const detail = programmeDetail("lung_ldct");
    detail.case = { ...detail.case, id, programme: "cervical", review: "open" };
    detail.review_items = [{ item: `rv-${id}`, loop: id, kinds, pair: [], p: 0, report: detail.evidence[0].report, verdict: "disagree", quote: "", chain: [] }];
    if (states) detail.api_review_states = states.map((s) => ({ item: `rv-${id}`, question: kinds[0], head: "", next_read: null, may_submit: false, reads: [], disagreement: false, canonical_unchanged: true, claim: null, claim_head: "", claim_actions: { claim: false, renew: false, release: false, takeover: false }, ...s })) as never;
    return detail;
  };

  it("returns an open cervical evidence case without loading the practice case", async () => {
    const detail = cervical("cx-ev", ["evidence"]);
    source.getCases.mockResolvedValue({ cases: [detail.case], total: 1, limit: 50, offset: 0, meta: {} });
    source.getCase.mockResolvedValue(detail);

    const queue = await reviewQueue();
    expect(source.getCase).toHaveBeenCalledExactlyOnceWith("cx-ev");
    expect(source.getFixtures).not.toHaveBeenCalled();
    expect(queue).toEqual({ cases: [detail], source: "open" });
  });

  it("keeps only cervical cases with outstanding protocol work and falls back when none remain", async () => {
    const done = cervical("cx-done", ["link"], [{ state: "resolved_agreed" }]);
    const waiting = cervical("cx-wait", ["link"], [{ state: "awaiting_r2" }]);
    const details = [done, waiting];
    source.getCases.mockResolvedValue({ cases: details.map((d) => d.case), total: 2, limit: 50, offset: 0, meta: {} });
    source.getCase.mockImplementation(async (id: string) => details.find((d) => d.case.id === id) ?? null);
    expect((await reviewQueue()).cases.map((c) => c.case.id)).toEqual(["cx-wait"]);
    expect(source.getFixtures).not.toHaveBeenCalled();

    source.getCases.mockResolvedValue({ cases: [done.case], total: 1, limit: 50, offset: 0, meta: {} });
    source.getFixtures.mockResolvedValue({ fixtures: { review_hold: { id: "hold" } } });
    source.getCase.mockImplementation(async (id: string) => (id === "cx-done" ? done : null));
    expect(await reviewQueue()).toEqual({ cases: [], source: "rebuilt" });
  });
});
