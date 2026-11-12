// @vitest-environment happy-dom

import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ReviewDesk } from "@/components/review/review-desk";
import { reviewView } from "@/components/review/case-view";
import { CaseDetail, ReviewState } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

const actions = vi.hoisted(() => ({ claimReviewQuestion: vi.fn(), submitReviewRead: vi.fn() }));
const router = vi.hoisted(() => ({ refresh: vi.fn(), push: vi.fn() }));
vi.mock("@/app/app/review/[id]/actions", () => actions);
vi.mock("next/navigation", () => ({ useRouter: () => router }));
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const detail = CaseDetail.parse({ ...hold, case: { ...hold.case, closed: false, review: "open" }, signoffs: [], api_reviews: [] });
const next = { href: "/app/review", label: "Back to Review", last: true };
const desk = (states: ReviewState[]) => createElement(ReviewDesk, { view: reviewView(detail), mode: "live", next, states });
const active = (head = "c".repeat(64), question: "link" | "evidence" = "link") => ReviewState.parse({
  item: question === "link" ? "rv-1" : "rv-2", question, state: "awaiting_r1", head: "", next_read: "r1", may_submit: true,
  claim_head: head,
  claim: { owner: "Rev-A", role: question === "link" ? "reviewer" : "clinician", read: "r1", claimed_at: "2026-11-13T10:00:00Z", heartbeat_at: "2026-11-13T10:00:00Z", expires_at: "2026-11-13T10:05:00Z", status: "active" },
  claim_actions: { claim: false, renew: true, release: true, takeover: false },
  reads: [{ read: "r1", submitted: false }], disagreement: false, canonical_unchanged: true,
});

let root: Root | undefined;
let container: HTMLDivElement | undefined;
async function mount(states: ReviewState[]) {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  await act(async () => root!.render(desk(states)));
}
async function unmount() {
  if (root) await act(async () => root!.unmount());
  container?.remove();
  root = undefined;
  container = undefined;
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-11-13T10:00:00Z"));
  actions.claimReviewQuestion.mockReset();
  router.refresh.mockReset();
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
    actions.claimReviewQuestion.mockResolvedValue({ ok: false, failure: { status: 409, error: "claim expired" }, states: [active("d".repeat(64))] });
    await mount([active()]);
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(actions.claimReviewQuestion).toHaveBeenCalledTimes(1);
    await act(async () => vi.advanceTimersByTimeAsync(180_000));
    expect(actions.claimReviewQuestion).toHaveBeenCalledTimes(1);
    expect(actions.claimReviewQuestion.mock.calls.every((call) => call[1].operation === "renew")).toBe(true);
  });

  it("restarts a halted heartbeat after a successful manual renewal", async () => {
    actions.claimReviewQuestion
      .mockResolvedValueOnce({ ok: false, failure: { status: 409, error: "stale claim" }, states: [active("d".repeat(64))] })
      .mockResolvedValueOnce({ ok: true, states: [active("e".repeat(64))] })
      .mockResolvedValueOnce({ ok: true, states: [active("f".repeat(64))] });
    await mount([active()]);
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(actions.claimReviewQuestion).toHaveBeenCalledTimes(1);

    const renew = [...container!.querySelectorAll("button")].find((button) => button.textContent === "Renew claim")!;
    await act(async () => renew.click());
    expect(actions.claimReviewQuestion).toHaveBeenCalledTimes(2);
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(actions.claimReviewQuestion).toHaveBeenCalledTimes(3);
  });

  it("preserves a populated draft when the heartbeat advances the claim head", async () => {
    actions.claimReviewQuestion.mockResolvedValue({ ok: true, states: [active("d".repeat(64))] });
    await mount([active()]);
    const answer = container!.querySelector<HTMLInputElement>("input[type=radio][value=same]")!;
    await act(async () => answer.click());
    expect(container!.querySelector<HTMLInputElement>("input[type=radio][value=same]")!.checked).toBe(true);

    await act(async () => vi.advanceTimersByTimeAsync(60_000));

    expect(actions.claimReviewQuestion).toHaveBeenCalledWith(detail.case.id, expect.objectContaining({ operation: "renew" }));
    expect(container!.querySelector<HTMLInputElement>("input[type=radio][value=same]")!.checked).toBe(true);
  });

  it("applies fresh updates for other questions when no concurrent operation can overwrite them", async () => {
    const externalEvidence = ReviewState.parse({
      ...active("d".repeat(64), "evidence"), state: "resolved_agreed", next_read: null, may_submit: false,
      final_decision: "adequate", resolution_basis: "agreement", claim: null,
      claim_actions: { claim: false, renew: false, release: false, takeover: false },
    });
    const idleEvidence = ReviewState.parse({
      ...active("d".repeat(64), "evidence"), may_submit: false,
      claim_actions: { claim: false, renew: false, release: false, takeover: false },
    });
    actions.claimReviewQuestion.mockResolvedValue({ ok: true, states: [active("e".repeat(64)), externalEvidence] });
    await mount([active(), idleEvidence]);

    await act(async () => vi.advanceTimersByTimeAsync(60_000));

    expect(container!.querySelector('[data-testid="final-evidence"]')!.textContent).toContain("Both reads agree");
  });

  it("renews simultaneous leases independently and keeps both timers alive", async () => {
    const confirmations: ((value: { ok: true; states: ReviewState[] }) => void)[] = [];
    actions.claimReviewQuestion.mockImplementation(() => new Promise((resolve) => confirmations.push(resolve)));
    await mount([active(), active("d".repeat(64), "evidence")]);

    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(actions.claimReviewQuestion).toHaveBeenCalledTimes(2);
    const renewed = [active("e".repeat(64)), active("f".repeat(64), "evidence")];
    await act(async () => confirmations.forEach((confirm) => confirm({ ok: true, states: renewed })));

    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(actions.claimReviewQuestion).toHaveBeenCalledTimes(4);
    expect(actions.claimReviewQuestion.mock.calls.map((call) => [call[1].item, call[1].operation])).toEqual([
      ["rv-1", "renew"], ["rv-2", "renew"], ["rv-1", "renew"], ["rv-2", "renew"],
    ]);
  });

  it("does not restart another question's heartbeat when one lease changes", async () => {
    actions.claimReviewQuestion.mockResolvedValue({ ok: true, states: [active("e".repeat(64)), active("d".repeat(64), "evidence")] });
    await mount([active(), active("d".repeat(64), "evidence")]);

    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    const linkCard = container!.querySelector('[data-item="rv-1"]')!;
    const manualRenew = [...linkCard.querySelectorAll("button")].find((button) => button.textContent === "Renew claim")!;
    await act(async () => manualRenew.click());
    expect(actions.claimReviewQuestion).toHaveBeenCalledTimes(1);

    await act(async () => vi.advanceTimersByTimeAsync(30_000));
    expect(actions.claimReviewQuestion.mock.calls.some((call) => call[1].item === "rv-2" && call[1].operation === "renew")).toBe(true);
  });

  it("does not let an older whole-case response regress another lease", async () => {
    const confirmations = new Map<string, (value: { ok: true; states: ReviewState[] }) => void>();
    actions.claimReviewQuestion.mockImplementation((_caseId: string, input: { item: string }) => new Promise((resolve) => confirmations.set(input.item, resolve)));
    await mount([active(), active("d".repeat(64), "evidence")]);
    await act(async () => vi.advanceTimersByTimeAsync(60_000));

    await act(async () => confirmations.get("rv-2")?.({ ok: true, states: [active(), active("e".repeat(64), "evidence")] }));
    await act(async () => confirmations.get("rv-1")?.({ ok: true, states: [active("f".repeat(64)), active("d".repeat(64), "evidence")] }));
    await act(async () => vi.advanceTimersByTimeAsync(60_000));

    expect(actions.claimReviewQuestion).toHaveBeenCalledTimes(4);
    expect(actions.claimReviewQuestion.mock.calls.slice(2).map((call) => [call[1].item, call[1].expectedClaimHead])).toEqual(expect.arrayContaining([
      ["rv-1", "f".repeat(64)], ["rv-2", "e".repeat(64)],
    ]));
  });

  it("keeps the heartbeat alive after a transient request failure", async () => {
    actions.claimReviewQuestion.mockRejectedValueOnce(new Error("offline")).mockResolvedValueOnce({ ok: true, states: [active("d".repeat(64))] });
    await mount([active()]);

    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(actions.claimReviewQuestion).toHaveBeenCalledTimes(1);
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(actions.claimReviewQuestion).toHaveBeenCalledTimes(2);
  });

  it("does not postpone a non-owner expiry when another question renews", async () => {
    const foreign = ReviewState.parse({
      ...active("d".repeat(64), "evidence"), may_submit: false,
      claim: { ...active("d".repeat(64), "evidence").claim!, owner: "Rev-B" },
      claim_actions: { claim: false, renew: false, release: false, takeover: false },
    });
    actions.claimReviewQuestion.mockResolvedValue({ ok: true, states: [active("e".repeat(64)), foreign] });
    await mount([active(), foreign]);

    await act(async () => vi.advanceTimersByTimeAsync(300_000));

    expect(actions.claimReviewQuestion).toHaveBeenCalledTimes(5);
    expect(router.refresh).toHaveBeenCalledTimes(1);
  });

  it("retries an expiry refresh while the active server state is unchanged", async () => {
    const foreign = ReviewState.parse({
      ...active(), may_submit: false,
      claim: { ...active().claim!, owner: "Rev-B" },
      claim_actions: { claim: false, renew: false, release: false, takeover: false },
    });
    vi.setSystemTime(new Date("2026-11-13T10:04:50Z"));
    await mount([foreign]);

    await act(async () => vi.advanceTimersByTimeAsync(10_000));
    expect(router.refresh).toHaveBeenCalledTimes(1);
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(router.refresh).toHaveBeenCalledTimes(2);
  });

  it("does not enter a zero-delay refresh loop while the server still reports an active lease", async () => {
    const foreign = ReviewState.parse({
      ...active(), may_submit: false,
      claim: { ...active().claim!, owner: "Rev-B" },
      claim_actions: { claim: false, renew: false, release: false, takeover: false },
    });
    vi.setSystemTime(new Date("2026-11-13T10:06:00Z"));
    router.refresh.mockImplementation(() => {
      if (router.refresh.mock.calls.length < 3) root!.render(desk([foreign]));
    });
    await mount([foreign]);

    await act(async () => vi.advanceTimersByTimeAsync(0));
    expect(router.refresh).not.toHaveBeenCalled();
    await act(async () => vi.advanceTimersByTimeAsync(60_000));
    expect(router.refresh).toHaveBeenCalledTimes(1);
  });

  it("refreshes a non-owner claim at expiry and shows takeover from the fresh state", async () => {
    const foreign = ReviewState.parse({
      ...active(), may_submit: false,
      claim: { ...active().claim!, owner: "Rev-B" },
      claim_actions: { claim: false, renew: false, release: false, takeover: false },
    });
    const expired = ReviewState.parse({
      ...foreign,
      claim: { ...foreign.claim!, status: "expired" },
      claim_actions: { claim: false, renew: false, release: false, takeover: true },
    });
    vi.setSystemTime(new Date("2026-11-13T10:04:50Z"));
    router.refresh.mockImplementation(() => root!.render(desk([expired])));
    await mount([foreign]);
    expect([...container!.querySelectorAll("button")].some((button) => button.textContent === "Take over claim")).toBe(false);

    await act(async () => vi.advanceTimersByTimeAsync(9_000));
    expect(router.refresh).not.toHaveBeenCalled();
    await act(async () => vi.advanceTimersByTimeAsync(1_000));

    expect(router.refresh).toHaveBeenCalledTimes(1);
    expect([...container!.querySelectorAll("button")].some((button) => button.textContent === "Take over claim")).toBe(true);
  });
});
