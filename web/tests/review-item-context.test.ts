// @vitest-environment happy-dom

import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ReviewDesk } from "@/components/review/review-desk";
import { reviewItemViews, reviewView } from "@/components/review/case-view";
import { CaseDetail, ReviewState } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

const actions = vi.hoisted(() => ({ claimReviewQuestion: vi.fn(), submitReviewRead: vi.fn() }));
vi.mock("@/app/app/review/[id]/actions", () => actions);
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }) }));
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const base = CaseDetail.parse(hold);
const next = { href: "/app/review", label: "Back to Review", last: true };
const linkTemplate = base.review_items[0];
const evidenceTemplate = base.review_items[1];

const evidence = (id: string, quote: string, regex: string, reader: string) => ({
  ...base.evidence[0],
  report: `skmc-endo/DiagnosticReport/${id}`,
  quote,
  agree: false,
  fields: { regex, reader },
});
const evidenceItem = (item: string, report: string) => ({ ...evidenceTemplate, item, report });
const linkItem = (item: string, pair: [string, string]) => ({ ...linkTemplate, item, pair });

const state = (item: string, question: "link" | "evidence", slot: "r1" | "r2" | "r3" = "r1") =>
  ReviewState.parse({
    item,
    question,
    state: slot === "r1" ? "awaiting_r1" : slot === "r2" ? "awaiting_r2" : "awaiting_r3",
    head: "",
    next_read: slot,
    may_submit: true,
    claim_head: "c".repeat(64),
    claim: { owner: "Rev-A", role: question === "link" ? "reviewer" : "clinician", read: slot, claimed_at: "2026-11-13T10:00:00Z", heartbeat_at: "2026-11-13T10:00:00Z", expires_at: "2026-11-13T10:05:00Z", status: "active" },
    claim_actions: { claim: false, renew: true, release: true, takeover: false },
    reads: [{ read: slot, submitted: false }],
    disagreement: false,
    canonical_unchanged: true,
  });

const twoReports = CaseDetail.parse({
  ...base,
  evidence: [evidence("CRC-dr-A", "quote from report A", "yes/yes/no/no", "no/yes/no/no"), evidence("CRC-dr-B", "quote from report B", "yes/no/yes/no", "yes/yes/yes/no")],
  review_items: [evidenceItem("rv-a", "skmc-endo/DiagnosticReport/CRC-dr-A"), evidenceItem("rv-b", "skmc-endo/DiagnosticReport/CRC-dr-B")],
});

const twoPairs = CaseDetail.parse({
  ...base,
  identities: [
    { provider: "ahs-mushrif", registration: "ahs-mushrif|AHS-MRN100001", role: "referral" },
    { provider: "skmc-endo", registration: "skmc-endo|SKM-MRN200001", role: "downstream" },
    { provider: "noor-endo", registration: "noor-endo|NOO-MRN300001", role: "downstream" },
  ],
  review_items: [
    linkItem("rv-l1", ["ahs-mushrif|AHS-MRN100001", "skmc-endo|SKM-MRN200001"]),
    linkItem("rv-l2", ["ahs-mushrif|AHS-MRN100001", "noor-endo|NOO-MRN300001"]),
  ],
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
const context = (c: HTMLElement, item: string) => c.querySelector<HTMLElement>(`[data-context-item="${item}"]`)!;

describe("review cards are bound to the item being judged", () => {
  it("builds one view per raised item from that item's report", () => {
    const views = reviewItemViews(twoReports);
    expect(views["rv-a"]).toMatchObject({ report: { id: "CRC-dr-A" }, quote: "quote from report A" });
    expect(views["rv-b"]).toMatchObject({ report: { id: "CRC-dr-B" }, quote: "quote from report B" });
    expect(views["rv-b"].checks.map((k) => [k.rules, k.second])).toEqual([["yes", "yes"], ["no", "yes"], ["yes", "yes"], ["no", "no"]]);
  });

  it("shows each evidence card its own report id, quote and reader values, and submits against that item", async () => {
    actions.submitReviewRead.mockResolvedValue({ ok: false, failure: { status: 400, error: "stub" } });
    const c = await mount(twoReports, [state("rv-a", "evidence"), state("rv-b", "evidence")]);
    expect(card(c, "rv-a", "evidence").textContent).toContain("CRC-dr-A");
    expect(card(c, "rv-b", "evidence").textContent).toContain("CRC-dr-B");
    expect(card(c, "rv-b", "evidence").textContent).not.toContain("CRC-dr-A");
    const a = context(c, "rv-a");
    const b = context(c, "rv-b");
    expect(a.textContent).toContain("CRC-dr-A");
    expect(a.textContent).toContain("quote from report A");
    expect(a.textContent).not.toContain("quote from report B");
    expect(b.textContent).toContain("CRC-dr-B");
    expect(b.textContent).toContain("quote from report B");
    expect(b.textContent).not.toContain("CRC-dr-A");
    expect(b.querySelector('[data-check="Prep graded"]')!.textContent).toContain("none");
    expect(a.querySelector('[data-check="Prep graded"]')!.textContent).not.toContain("none");

    const radio = card(c, "rv-b", "evidence").querySelector<HTMLInputElement>('input[type="radio"]')!;
    await act(async () => radio.click());
    const submit = [...card(c, "rv-b", "evidence").querySelectorAll("button")].find((x) => /record|submit|first/i.test(x.textContent ?? ""))!;
    await act(async () => submit.click());
    expect(actions.submitReviewRead).toHaveBeenCalledTimes(1);
    expect(actions.submitReviewRead.mock.calls[0][1]).toMatchObject({ item: "rv-b", question: "evidence" });
  });

  it("shows each identity question the pair it asks about", async () => {
    const views = reviewItemViews(twoPairs);
    expect(views["rv-l1"].reportRegistration).toBe("skmc-endo|SKM-MRN200001");
    expect(views["rv-l2"].reportRegistration).toBe("noor-endo|NOO-MRN300001");
    const c = await mount(twoPairs, [state("rv-l1", "link"), state("rv-l2", "link")]);
    expect(context(c, "rv-l1").textContent).toContain("SKM-MRN200001");
    expect(context(c, "rv-l1").textContent).not.toContain("NOO-MRN300001");
    expect(context(c, "rv-l2").textContent).toContain("NOO-MRN300001");
    expect(context(c, "rv-l2").textContent).not.toContain("SKM-MRN200001");
    expect(card(c, "rv-l1", "link").textContent).toContain("SKM-MRN200001");
    expect(card(c, "rv-l2", "link").textContent).toContain("NOO-MRN300001");
  });

  it("offers no answer form for a question whose item has no shown context", async () => {
    const c = await mount(twoReports, [state("rv-unknown", "evidence")]);
    expect(card(c, "rv-unknown", "evidence").querySelector('input[type="radio"]')).toBeNull();
    expect(card(c, "rv-unknown", "evidence").textContent).toContain("context");
  });

  it("keeps a combined link and evidence item and its revised report aligned from r1 to senior resolution", async () => {
    const revised = {
      type: "evidence",
      body: { ...evidence("CRC-dr-A", "revised quote", "yes/yes/no/no", "yes/yes/no/no"), version: 2 },
      prev: "genesis",
      hash: "2".repeat(64),
    };
    const detail = CaseDetail.parse({
      ...base,
      evidence: [evidence("CRC-dr-A", "first version quote", "yes/yes/no/no", "no/yes/no/no")],
      review_items: [{ ...evidenceTemplate, item: "rv-both", kinds: ["link", "evidence"], report: "CRC-dr-A", report_version: 2, chain: [revised] }],
    });
    const view = reviewItemViews(detail)["rv-both"];
    expect(view.items).toEqual({ link: "rv-both", evidence: "rv-both" });
    expect(view.quote).toBe("revised quote");
    expect(view.report).toMatchObject({ id: "CRC-dr-A", version: 2 });
    for (const slot of ["r1", "r2", "r3"] as const) {
      const c = await mount(detail, [state("rv-both", "link", slot), state("rv-both", "evidence", slot)]);
      expect(context(c, "rv-both").textContent).toContain("revised quote");
      expect(context(c, "rv-both").textContent).not.toContain("first version quote");
      expect(card(c, "rv-both", "evidence").textContent).toContain("version 2");
      await act(async () => root!.unmount());
      container!.remove();
      root = undefined;
    }
  });
});
