// @vitest-environment happy-dom

import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ReviewDesk } from "@/components/review/review-desk";
import { reviewView } from "@/components/review/case-view";
import { SignoffsCard } from "@/components/case/sections";
import { CaseDetail, ReviewState } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

const actions = vi.hoisted(() => ({ claimReviewQuestion: vi.fn(), submitReviewRead: vi.fn() }));
vi.mock("@/app/app/review/[id]/actions", () => actions);
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }) }));
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const detail = CaseDetail.parse({ ...hold, case: { ...hold.case, closed: false, review: "open" }, signoffs: [], api_reviews: [] });
const next = { href: "/app/review", label: "Back to Review", last: true };
const claim = (read: "r1" | "r3") => ({
  owner: "Rev-A", role: read === "r3" ? "senior_reviewer" : "reviewer", read, claimed_at: "2026-11-13T10:00:00Z",
  heartbeat_at: "2026-11-13T10:00:00Z", expires_at: "2026-11-13T10:05:00Z", status: "active",
});
const base = { item: "rv-1", question: "link", may_submit: true, claim_head: "c".repeat(64), claim_actions: { claim: false, renew: true, release: true, takeover: false }, canonical_unchanged: true };
const firstRead = ReviewState.parse({ ...base, state: "awaiting_r1", head: "", next_read: "r1", claim: claim("r1"), reads: [{ read: "r1", submitted: false }], disagreement: false });
const seniorRead = ReviewState.parse({
  ...base, state: "awaiting_r3", head: "h2", next_read: "r3", claim: claim("r3"), disagreement: true,
  reads: [
    { read: "r1", submitted: true, decision: "same", hash: "h1", note: "R1 matching NHS number" },
    { read: "r2", submitted: true, decision: "different", hash: "h2", note: "R2 different address history" },
    { read: "r3", submitted: false },
  ],
});
const concealed = ReviewState.parse({ ...base, state: "awaiting_r2", head: "h1", next_read: "r2", may_submit: false, claim: null, claim_actions: undefined, reads: [{ read: "r1", submitted: true, concealed: true }, { read: "r2", submitted: false }], disagreement: false });

let root: Root | undefined;
let container: HTMLDivElement;
async function mount(states: ReviewState[]) {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  await act(async () => root!.render(createElement(ReviewDesk, { view: reviewView(detail), mode: "live", next, states })));
}
async function type(text: string) {
  const area = container.querySelector("textarea")!;
  const set = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!;
  await act(async () => {
    set.call(area, text);
    area.dispatchEvent(new Event("input", { bubbles: true }));
  });
}
async function pick(value: string) {
  await act(async () => container.querySelector<HTMLInputElement>(`input[type=radio][value="${value}"]`)!.click());
}
async function submit() {
  const button = [...container.querySelectorAll("button")].find((b) => /^Submit /.test(b.textContent ?? ""))!;
  await act(async () => button.click());
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-11-13T10:00:00Z"));
  actions.submitReviewRead.mockReset().mockResolvedValue({ ok: true, read: "r1", committed: false, states: [] });
});
afterEach(async () => {
  if (root) await act(async () => root!.unmount());
  container.remove();
  root = undefined;
  vi.useRealTimers();
});

describe("review rationale", () => {
  it("sends an optional first-read rationale exactly as typed, and an empty one when left blank", async () => {
    await mount([firstRead]);
    await pick("same");
    await type("  Same NHS number. ");
    await submit();
    expect(actions.submitReviewRead).toHaveBeenCalledWith(detail.case.id, expect.objectContaining({ decision: "same", note: "  Same NHS number. " }));
    await act(async () => root!.unmount());
    container.remove();
    actions.submitReviewRead.mockClear();
    await mount([firstRead]);
    await pick("different");
    await submit();
    expect(actions.submitReviewRead).toHaveBeenCalledWith(detail.case.id, expect.objectContaining({ decision: "different", note: "" }));
  });

  it("holds a senior resolution until it has a non-blank rationale", async () => {
    await mount([seniorRead]);
    expect(container.textContent).toContain("R1 matching NHS number");
    expect(container.textContent).toContain("R2 different address history");
    await pick("same");
    await submit();
    await type("  \n\t");
    await submit();
    expect(actions.submitReviewRead).not.toHaveBeenCalled();
    await type("R2 missed the matching address history.");
    await submit();
    expect(actions.submitReviewRead).toHaveBeenCalledWith(detail.case.id, expect.objectContaining({ decision: "same", note: "R2 missed the matching address history." }));
  });

  it("shows no rationale for a concealed first read", async () => {
    await mount([concealed]);
    expect(container.querySelector('[data-read="r1"]')!.textContent).not.toMatch(/Rationale/);
  });

  it("shows an ordinary API review note in Human sign-offs with its provenance", () => {
    const review = { case: detail.case.id, item: "rv-1", decision: "same", note: "Matching date of birth", role: "reviewer", actor: "Rev-A", action: "sign_link", at: "2026-11-13T10:00:00Z", mode: "api", prev: "", hash: "rev-hash" };
    const html = renderToStaticMarkup(SignoffsCard({ detail: CaseDetail.parse({ ...detail, api_reviews: [review] }) }));
    expect(html).toContain("Matching date of birth");
    expect(html).toContain("Rev-A");
  });
});
