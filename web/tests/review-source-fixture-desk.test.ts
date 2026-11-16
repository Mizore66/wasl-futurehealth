// @vitest-environment happy-dom

import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ReviewDesk } from "@/components/review/review-desk";
import { reviewView } from "@/components/review/case-view";
import { CaseDetail } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

// #162 in fixtures mode (simulated preview): the evidence answer is previewed only with its source report on screen.
const actions = vi.hoisted(() => ({ claimReviewQuestion: vi.fn(), submitReviewRead: vi.fn(), readReviewSource: vi.fn() }));
vi.mock("@/app/app/review/[id]/actions", () => actions);
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }) }));
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const detail = CaseDetail.parse(hold);
const evidenceItem = detail.review_items.find((r) => r.kinds.includes("evidence"))!.item;
const next = { href: "/app/review", label: "Back to Review", last: true };
const text = "Synthetic report. It was passed to the level of the cecum. A 3 mm polyp was removed with cold biopsy forceps.";

let root: Root | undefined;
let container: HTMLDivElement;
async function mount() {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  await act(async () => root!.render(createElement(ReviewDesk, { view: reviewView(detail), mode: "fixtures", next })));
}
const radio = (value: string) => container.querySelector<HTMLInputElement>(`input[type=radio][value="${value}"]`)!;
const previewButton = () => [...container.querySelectorAll("button")].find((b) => /^Preview: /.test(b.textContent ?? ""));

beforeEach(() => {
  // happy-dom cancels Web Animations on unmount with an unhandled AbortError; motion falls back to its JS driver without them.
  Object.defineProperty(Element.prototype, "animate", { value: undefined, configurable: true, writable: true });
  actions.readReviewSource.mockReset();
});
afterEach(async () => {
  if (root) await act(async () => root!.unmount());
  container.remove();
  root = undefined;
});

describe("fixture review desk source report", () => {
  it("shows the complete report for the evidence question, then lets the preview go ahead", async () => {
    actions.readReviewSource.mockResolvedValue({ ok: true, source: { report: "skmc-endo/DiagnosticReport/SKM-dr-217", report_version: 1, day: 30.4762, text } });
    await mount();
    await act(async () => radio("same").click());
    expect(actions.readReviewSource).toHaveBeenCalledWith(detail.case.id, evidenceItem);
    const panel = container.querySelector('[data-testid="source-report"]')!;
    expect(panel.getAttribute("data-source")).toBe("available");
    expect(panel.textContent).toContain("skmc-endo/DiagnosticReport/SKM-dr-217, version 1");
    expect(panel.textContent).toContain("cold biopsy forceps");
    await act(async () => radio("adequate").click());
    expect(previewButton()?.textContent).toBe("Preview: Close the case");
    expect(container.querySelector('[data-testid="source-withheld"]')).toBeNull();
  });

  it.each([
    ["unavailable", () => Promise.resolve({ ok: false, status: 409, sourceStatus: "missing" })],
    ["loading", () => new Promise(() => {})],
  ])("withholds the evidence preview while the source is %s", async (_, result) => {
    actions.readReviewSource.mockImplementation(result);
    await mount();
    await act(async () => radio("same").click());
    await act(async () => radio("adequate").click());
    expect(previewButton()).toBeUndefined();
    expect(container.querySelector('[data-testid="source-withheld"]')?.textContent).toContain("source report");
  });

  it("uses the page's server-side read without calling the server action", async () => {
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
    const fixtureSource = { ok: true as const, source: { report: "skmc-endo/DiagnosticReport/SKM-dr-217", report_version: 1, day: 30.4762, text } };
    await act(async () => root!.render(createElement(ReviewDesk, { view: reviewView(detail), mode: "fixtures", next, fixtureSource })));
    await act(async () => radio("same").click());
    await act(async () => radio("adequate").click());
    expect(actions.readReviewSource).not.toHaveBeenCalled();
    expect(container.querySelector('[data-testid="source-report"]')?.textContent).toContain("cold biopsy forceps");
    expect(previewButton()?.textContent).toBe("Preview: Close the case");
  });

  it("leaves the identity-only answer unchanged when the source cannot be opened", async () => {
    actions.readReviewSource.mockResolvedValue({ ok: false, status: 409, sourceStatus: "unreadable" });
    await mount();
    await act(async () => radio("different").click());
    expect(previewButton()).toBeDefined();
    expect(container.querySelector('[data-testid="source-withheld"]')).toBeNull();
  });
});
