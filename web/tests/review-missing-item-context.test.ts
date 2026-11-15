// @vitest-environment happy-dom

import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ReviewDesk } from "@/components/review/review-desk";
import { reviewItemViews, reviewView } from "@/components/review/case-view";
import { CaseDetail, ReviewState } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

/*
 * Partial structural-binding checks only. They do not show that any context is current, original or guard-checked.
 * The synthetic cases are built from the accepted hold fixture using existing wire fields only.
 */
const actions = vi.hoisted(() => ({ claimReviewQuestion: vi.fn(), submitReviewRead: vi.fn(), readReviewSource: vi.fn((..._a: unknown[]): Promise<unknown> => new Promise(() => {})) }));
vi.mock("@/app/app/review/[id]/actions", () => actions);
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }) }));
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const base = CaseDetail.parse(hold);
const next = { href: "/app/review", label: "Back to Review", last: true };
const linkTemplate = base.review_items[0];
const evidenceTemplate = base.review_items[1];
const A = "ahs-mushrif|AHS-MRN001006";
const C = "skmc-endo|SKM-MRN000409";
const B = "noor-endo|NOO-OPAQUE-b7|x";

const linkItem = (item: string, pair: [string, string]) => ({ ...linkTemplate, item, pair });
const evidenceItem = (item: string, report: string) => ({ ...evidenceTemplate, item, report });

const state = (item: string, question: "link" | "evidence") =>
  ReviewState.parse({
    item,
    question,
    state: "awaiting_r1",
    head: "",
    next_read: "r1",
    may_submit: true,
    claim_head: "c".repeat(64),
    claim: { owner: "Rev-A", role: question === "link" ? "reviewer" : "clinician", read: "r1", claimed_at: "2026-11-13T10:00:00Z", heartbeat_at: "2026-11-13T10:00:00Z", expires_at: "2026-11-13T10:05:00Z", status: "active" },
    claim_actions: { claim: false, renew: true, release: true, takeover: false },
    reads: [{ read: "r1", submitted: false }],
    disagreement: false,
    canonical_unchanged: true,
  });

// Canonical identities and links describe A/C only; the A/B item's own pair names B, which the case never lists.
const missingB = CaseDetail.parse({ ...base, review_items: [linkItem("rv-ab", [A, B]), linkItem("rv-ac", [A, C])] });

// B is listed, but the case's only link is A/C. The A/B item's own chain carries its own link decision.
const own = { type: "link_decision", body: { ...base.links[0], weakest_edge: { a: A, b: B, basis: "p_cal", p: 0.5 }, path_p_cal: 0.5, tau: 0.9 }, prev: "genesis", hash: "3".repeat(64) };
const presentB = CaseDetail.parse({
  ...base,
  identities: [...base.identities, { provider: "noor-endo", registration: B, role: "downstream" }],
  review_items: [{ ...linkItem("rv-ab", [A, B]), chain: [own] }, linkItem("rv-ac", [A, C])],
});

// Both A/B endpoints are listed, but A/B has no own link decision and no exact same-loop edge; another pair carries distinct numbers.
const OTHER_LOOP = "ahs-mushrif/ServiceRequest/OTHER-LOOP";
const distinct = { ...base.links[0], path_p_cal: 0.123, tau: 0.456 };
const listedB = [...base.identities, { provider: "noor-endo", registration: B, role: "downstream" }];
const noDetail = CaseDetail.parse({
  ...base,
  identities: listedB,
  links: [{ ...distinct, weakest_edge: { ...base.links[0].weakest_edge, b: C } }],
  review_items: [linkItem("rv-ab", [A, B]), linkItem("rv-ac", [A, C])],
});
const wrongLoop = CaseDetail.parse({
  ...base,
  identities: listedB,
  links: [{ ...distinct, loop: OTHER_LOOP, weakest_edge: { a: A, b: B, basis: "p_cal", p: 0.7 } }],
  review_items: [linkItem("rv-ab", [A, B]), linkItem("rv-ac", [A, C])],
});

const missingEvidence = CaseDetail.parse({
  ...base,
  review_items: [evidenceItem("rv-ev-own", base.evidence[0].report), evidenceItem("rv-ev-none", "skmc-endo/DiagnosticReport/NOT-IN-CASE")],
});

let root: Root | undefined;
let container: HTMLDivElement | undefined;
async function mount(detail: CaseDetail, states: ReviewState[]) {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  await act(async () =>
    root!.render(createElement(ReviewDesk, { view: reviewView(detail), itemViews: reviewItemViews(detail), mode: "live", next, states })),
  );
  return container;
}
beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-11-13T10:00:00Z"));
});
afterEach(async () => {
  vi.useRealTimers();
  if (root) await act(async () => root!.unmount());
  container?.remove();
  root = undefined;
  container = undefined;
  actions.submitReviewRead.mockReset();
});

const card = (c: HTMLElement, item: string, question: string) => c.querySelector<HTMLElement>(`section[data-item="${item}"][data-question="${question}"]`)!;
const context = (c: HTMLElement, item: string) => c.querySelector<HTMLElement>(`[data-context-item="${item}"]`);
const submitButtons = (el: HTMLElement) => [...el.querySelectorAll("button")].filter((x) => /record|submit|first/i.test(x.textContent ?? ""));

describe("an item whose own pair is not in the case borrows no other pair", () => {
  it("builds an unavailable view for A/B and keeps A/C's own", () => {
    const views = reviewItemViews(missingB);
    expect(views["rv-ab"].context.link).toBe(false);
    expect(views["rv-ab"].reportRegistration).toBe("");
    expect(views["rv-ab"].referralRegistration).toBe(A);
    expect(views["rv-ab"].link.score).toBe(0);
    expect(views["rv-ac"].context.link).toBe(true);
    expect(views["rv-ac"].reportRegistration).toBe(C);
  });

  it("withholds A/B's form and context, and A/C's stays answerable", async () => {
    actions.submitReviewRead.mockResolvedValue({ ok: false, failure: { status: 400, error: "stub" } });
    const c = await mount(missingB, [state("rv-ab", "link"), state("rv-ac", "link")]);
    const ab = card(c, "rv-ab", "link");
    expect(ab.querySelector('input[type="radio"]')).toBeNull();
    expect(submitButtons(ab)).toHaveLength(0);
    expect(ab.textContent).toContain("not available");
    expect(ab.textContent).not.toContain("SKM-MRN000409");
    expect(context(c, "rv-ab")?.textContent ?? "").not.toContain("SKM-MRN000409");
    const ac = card(c, "rv-ac", "link");
    expect(ac.textContent).toContain("SKM-MRN000409");
    expect(context(c, "rv-ac")!.textContent).toContain("SKM-MRN000409");
    const radio = ac.querySelector<HTMLInputElement>('input[type="radio"]')!;
    await act(async () => radio.click());
    await act(async () => submitButtons(ac)[0].click());
    expect(actions.submitReviewRead).toHaveBeenCalledTimes(1);
    expect(actions.submitReviewRead.mock.calls[0][1]).toMatchObject({ item: "rv-ac", question: "link" });
  });

  it("with A/B alone raised, no A/C context or form is shown", async () => {
    const c = await mount(missingB, [state("rv-ab", "link")]);
    expect(card(c, "rv-ab", "link").querySelector('input[type="radio"]')).toBeNull();
    expect(c.textContent).not.toContain("SKM-MRN000409");
  });
});

describe("an item whose pair is listed uses its own link and exact opaque registration", () => {
  it("takes the link decision from the item's own chain, not links[0]", async () => {
    const views = reviewItemViews(presentB);
    expect(views["rv-ab"].context.link).toBe(true);
    expect(views["rv-ab"].reportRegistration).toBe(B);
    expect(views["rv-ab"].link.score).toBe(0.5);
    expect(views["rv-ab"].link.line).toBe(0.9);
    expect(views["rv-ac"].link.score).toBe(base.links[0].path_p_cal);
    const c = await mount(presentB, [state("rv-ab", "link"), state("rv-ac", "link")]);
    expect(card(c, "rv-ab", "link").querySelector('input[type="radio"]')).not.toBeNull();
    expect(card(c, "rv-ab", "link").textContent).toContain("NOO-OPAQUE-b7");
    expect(card(c, "rv-ab", "link").textContent).not.toContain("SKM-MRN000409");
  });
});

describe("an evidence item with no source of its own borrows none", () => {
  it("withholds the form rather than showing evidence[0]", async () => {
    const views = reviewItemViews(missingEvidence);
    expect(views["rv-ev-none"].context.evidence).toBe(false);
    expect(views["rv-ev-none"].quote).toBe("");
    expect(views["rv-ev-own"].context.evidence).toBe(true);
    expect(views["rv-ev-own"].quote).toBe(base.evidence[0].quote);
    const c = await mount(missingEvidence, [state("rv-ev-none", "evidence"), state("rv-ev-own", "evidence")]);
    const none = card(c, "rv-ev-none", "evidence");
    expect(none.querySelector('input[type="radio"]')).toBeNull();
    expect(submitButtons(none)).toHaveLength(0);
    expect(none.textContent).not.toContain("SKM-dr-217");
    expect(context(c, "rv-ev-none")?.textContent ?? "").not.toContain(base.evidence[0].quote);
    const ok = card(c, "rv-ev-own", "evidence");
    expect(ok.querySelector('input[type="radio"]')).not.toBeNull();
    expect(ok.textContent).toContain("SKM-dr-217");
    expect(context(c, "rv-ev-own")!.textContent).toContain(base.evidence[0].quote);
  });
});

describe("an item whose endpoints are listed but whose own link detail is absent invents none", () => {
  for (const [name, detail] of [["no own chain link and no exact edge", noDetail], ["an edge for the same pair on a different loop", wrongLoop]] as const) {
    it(`withholds the form for ${name}`, async () => {
      const views = reviewItemViews(detail);
      expect(views["rv-ab"].context.link).toBe(false);
      expect(views["rv-ab"].link).toMatchObject({ score: 0, line: 0, basis: "" });
      const c = await mount(detail, [state("rv-ab", "link")]);
      const ab = card(c, "rv-ab", "link");
      expect(ab.querySelector('input[type="radio"]')).toBeNull();
      expect(submitButtons(ab)).toHaveLength(0);
      expect(ab.querySelector("form")).toBeNull();
      expect(ab.textContent).toContain("not available");
      const shown = `${ab.textContent}${context(c, "rv-ab")?.textContent ?? ""}`;
      expect(shown).not.toMatch(/0\.123|0\.456|0\.7\b/);
      expect(shown).not.toMatch(/Link score\s*0\.000/);
    });
  }

  it("keeps the exact same-loop A/C edge usable", async () => {
    const views = reviewItemViews(noDetail);
    expect(views["rv-ac"].context.link).toBe(true);
    expect(views["rv-ac"].link.score).toBe(0.123);
    const c = await mount(noDetail, [state("rv-ab", "link"), state("rv-ac", "link")]);
    expect(card(c, "rv-ac", "link").querySelector('input[type="radio"]')).not.toBeNull();
    expect(card(c, "rv-ab", "link").querySelector('input[type="radio"]')).toBeNull();
  });
});
