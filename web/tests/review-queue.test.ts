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
    expect(source.getCases).toHaveBeenCalledWith(expect.objectContaining({ review: "open", offset: 0 }));
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

  function openGuided(id: string) {
    const detail = programmeDetail("lung_ldct");
    detail.case = { ...detail.case, id, review: "open" };
    detail.review_items = [{ item: `rv-${id}`, loop: id, kinds: ["evidence"], pair: [], p: 0, report: detail.evidence[0].report, verdict: "disagree", quote: "", chain: [] }];
    return detail;
  }

  it.each([401, 403, 404] as const)("reports a %s detail read as an incomplete queue and never selects review_hold", async (status) => {
    const detail = openGuided("case-a");
    source.getCases.mockResolvedValue({ cases: [detail.case], total: 1, limit: 50, offset: 0, meta: {} });
    source.getCaseWithToken.mockResolvedValue({ ok: false, status });

    const queue = await reviewQueue("stored-token-1234567890");
    expect(queue).toEqual({ cases: [], source: "incomplete", failures: [{ id: "case-a", status }] });
    expect(source.getFixtures).not.toHaveBeenCalled();
  });

  it("keeps the readable cases of a mixed response and names the cases that failed", async () => {
    const good = openGuided("case-good");
    const bad = openGuided("case-bad");
    source.getCases.mockResolvedValue({ cases: [good.case, bad.case], total: 2, limit: 50, offset: 0, meta: {} });
    source.getCaseWithToken.mockImplementation(async (id: string) => (id === "case-good" ? { ok: true, detail: good } : { ok: false, status: 403 }));

    const queue = await reviewQueue("stored-token-1234567890");
    expect(queue).toEqual({ cases: [good], source: "incomplete", failures: [{ id: "case-bad", status: 403 }] });
    expect(source.getFixtures).not.toHaveBeenCalled();
  });

  it("keeps the practice queue when every detail read succeeds and none is actionable", async () => {
    const settled = openGuided("case-done");
    settled.review_items = [];
    source.getCases.mockResolvedValue({ cases: [settled.case], total: 1, limit: 50, offset: 0, meta: {} });
    source.getCaseWithToken.mockResolvedValue({ ok: true, detail: settled });
    source.getFixtures.mockResolvedValue({ fixtures: { review_hold: { id: "hold" } } });

    const queue = await reviewQueue("stored-token-1234567890");
    expect(queue.source).toBe("rebuilt");
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

function pagedCases(count: number, pageSize: number, make: (i: number) => ReturnType<typeof programmeDetail>) {
  const details = Array.from({ length: count }, (_, i) => make(i));
  source.getCases.mockImplementation(async ({ offset = 0 }: { offset?: number }) => ({
    cases: details.slice(offset, offset + pageSize).map((d) => d.case),
    total: count,
    limit: pageSize,
    offset,
    meta: {},
  }));
  source.getCase.mockImplementation(async (id: string) => details.find((d) => d.case.id === id) ?? null);
}

function guidedCase(i: number, resolved: boolean, day = i) {
  const detail = programmeDetail("lung_ldct");
  detail.case = { ...detail.case, id: `case-${String(i).padStart(3, "0")}`, review: "open", referral_day: day };
  detail.review_items = [{ item: `rv-${i}`, loop: detail.case.id, kinds: ["evidence"], pair: [], p: 0, report: detail.evidence[0].report, verdict: "disagree", quote: "", chain: [] }];
  detail.api_review_states = [{ item: `rv-${i}`, question: "evidence", state: resolved ? "resolved_agreed" : "awaiting_r1" }] as typeof detail.api_review_states;
  return detail;
}

describe("reviewQueue pagination", () => {
  it("finds an unresolved case on the second page without loading the practice fallback", async () => {
    pagedCases(51, 50, (i) => guidedCase(i, i < 50));
    const queue = await reviewQueue();
    expect(queue.source).toBe("open");
    expect(queue.cases.map((c) => c.case.id)).toEqual(["case-050"]);
    expect(source.getFixtures).not.toHaveBeenCalled();
  });

  it("lists every unresolved case exactly once, oldest referral first, across pages", async () => {
    pagedCases(120, 50, (i) => guidedCase(i, false, i === 119 ? -5 : 100 - (i % 7)));
    const ids = (await reviewQueue()).cases.map((c) => c.case.id);
    expect(ids).toHaveLength(120);
    expect(new Set(ids).size).toBe(120);
    expect(ids[0]).toBe("case-119");
  });

  it("uses the fallback only after every page held no actionable guided case", async () => {
    pagedCases(100, 50, (i) => {
      const d = guidedCase(i, true);
      if (i % 2) d.case = { ...d.case, programme: "cervical" };
      return d;
    });
    source.getFixtures.mockResolvedValue({ fixtures: { review_hold: { id: "hold" } } });
    const queue = await reviewQueue();
    expect(queue.source).toBe("rebuilt");
    expect(source.getCases).toHaveBeenCalledTimes(2);
  });
});
