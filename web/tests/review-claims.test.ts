// @vitest-environment happy-dom

import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ReviewDesk } from "@/components/review/review-desk";
import { reviewView } from "@/components/review/case-view";
import { CaseDetail, ReviewState } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

const actions = vi.hoisted(() => ({ claimReviewQuestion: vi.fn(), submitReviewRead: vi.fn() }));
vi.mock("@/app/app/review/[id]/actions", () => actions);
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: () => {}, push: () => {} }) }));
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const detail = CaseDetail.parse({ ...hold, case: { ...hold.case, closed: false, review: "open" }, signoffs: [], api_reviews: [] });
const next = { href: "/app/review", label: "Back to Review", last: true };
const active = (head = "c".repeat(64)) => ReviewState.parse({
  item: "rv-1", question: "link", state: "awaiting_r1", head: "", next_read: "r1", may_submit: true,
  claim_head: head,
  claim: { owner: "Rev-A", role: "reviewer", read: "r1", claimed_at: "2026-11-13T10:00:00Z", heartbeat_at: "2026-11-13T10:00:00Z", expires_at: "2026-11-13T10:05:00Z", status: "active" },
  claim_actions: { claim: false, renew: true, release: true, takeover: false },
  reads: [{ read: "r1", submitted: false }], disagreement: false, canonical_unchanged: true,
});

let root: Root | undefined;
let container: HTMLDivElement | undefined;
async function mount(states: ReviewState[]) {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  await act(async () => root!.render(createElement(ReviewDesk, { view: reviewView(detail), mode: "live", next, states })));
}
async function unmount() {
  if (root) await act(async () => root!.unmount());
  container?.remove();
  root = undefined;
  container = undefined;
}

beforeEach(() => {
  vi.useFakeTimers();
  actions.claimReviewQuestion.mockReset();
});
afterEach(async () => {
  await unmount();
  vi.useRealTimers();
});

describe("review claim heartbeat", () => {
  it("renews after 60 seconds without overlap and uses the latest head", async () => {
    let confirmFirst: ((value: { ok: true; states: ReviewState[] }) => void) | undefined;
    actions.claimReviewQuestion
      .mockImplementationOnce(() => new Promise((resolve) => { confirmFirst = resolve; }))
      .mockResolvedValueOnce({ ok: true, states: [active("e".repeat(64))] });
    await mount([active()]);
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(actions.claimReviewQuestion).toHaveBeenCalledTimes(1);
    expect(actions.claimReviewQuestion.mock.calls[0][1]).toMatchObject({ operation: "renew", expectedClaimHead: "c".repeat(64) });
    await act(async () => vi.advanceTimersByTimeAsync(120_000));
    expect(actions.claimReviewQuestion).toHaveBeenCalledTimes(1);
    await act(async () => confirmFirst?.({ ok: true, states: [active("d".repeat(64))] }));
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(actions.claimReviewQuestion).toHaveBeenCalledTimes(2);
    expect(actions.claimReviewQuestion.mock.calls[1][1]).toMatchObject({ operation: "renew", expectedClaimHead: "d".repeat(64) });
    await unmount();
    await act(async () => vi.advanceTimersByTimeAsync(120_000));
    expect(actions.claimReviewQuestion).toHaveBeenCalledTimes(2);
  });

  it("stops the timer after release consumes ownership", async () => {
    const released = ReviewState.parse({ ...active(), may_submit: false, claim: null, claim_actions: { claim: true, renew: false, release: false, takeover: false } });
    actions.claimReviewQuestion.mockResolvedValue({ ok: true, states: [released] });
    await mount([active()]);
    const release = [...container!.querySelectorAll("button")].find((b) => b.textContent === "Release claim")!;
    await act(async () => release.click());
    expect(actions.claimReviewQuestion).toHaveBeenCalledWith(detail.case.id, expect.objectContaining({ operation: "release" }));
    await act(async () => vi.advanceTimersByTimeAsync(120_000));
    expect(actions.claimReviewQuestion).toHaveBeenCalledTimes(1);
  });

  it("does not retry or automatically take over after a conflict", async () => {
    actions.claimReviewQuestion.mockResolvedValue({ ok: false, failure: { status: 409, error: "claim expired" }, states: [active()] });
    await mount([active()]);
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(actions.claimReviewQuestion).toHaveBeenCalledTimes(1);
    await act(async () => vi.advanceTimersByTimeAsync(180_000));
    expect(actions.claimReviewQuestion).toHaveBeenCalledTimes(1);
    expect(actions.claimReviewQuestion.mock.calls.every((call) => call[1].operation === "renew")).toBe(true);
  });
});
