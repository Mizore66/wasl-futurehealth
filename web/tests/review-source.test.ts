// @vitest-environment happy-dom

import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ReviewDesk } from "@/components/review/review-desk";
import { reviewView } from "@/components/review/case-view";
import { CaseDetail, ReviewState } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

const actions = vi.hoisted(() => ({ claimReviewQuestion: vi.fn(), submitReviewRead: vi.fn(), readReviewSource: vi.fn() }));
vi.mock("@/app/app/review/[id]/actions", () => actions);
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }) }));
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const detail = CaseDetail.parse({ ...hold, case: { ...hold.case, closed: false, review: "open" }, signoffs: [], api_reviews: [] });
const next = { href: "/app/review", label: "Back to Review", last: true };
const evidenceItem = detail.review_items.find((r) => r.kinds.includes("evidence"))!.item;
const evidenceRead = ReviewState.parse({
  item: evidenceItem, question: "evidence", state: "awaiting_r1", head: "", next_read: "r1", may_submit: true, disagreement: false, canonical_unchanged: true,
  claim_head: "c".repeat(64), claim_actions: { claim: false, renew: true, release: true, takeover: false },
  claim: { owner: "clin-a", role: "clinician", read: "r1", claimed_at: "2026-11-13T10:00:00Z", heartbeat_at: "2026-11-13T10:00:00Z", expires_at: "2026-11-13T10:05:00Z", status: "active" },
  reads: [{ read: "r1", submitted: false }],
});
const report = "Synthetic colonoscopy. Scope advanced to the level of the cecum. Cold snare polypectomy; tissue retrieved and sent for histology.";

let root: Root | undefined;
let container: HTMLDivElement;
async function mount() {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  await act(async () => root!.render(createElement(ReviewDesk, { view: reviewView(detail), mode: "live", next, states: [evidenceRead] })));
}
async function answerAndSubmit() {
  await act(async () => container.querySelector<HTMLInputElement>('input[type=radio][value="adequate"]')!.click());
  const button = [...container.querySelectorAll("button")].find((b) => /^Submit /.test(b.textContent ?? ""))!;
  await act(async () => button.click());
  return button;
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-11-13T10:00:00Z"));
  actions.submitReviewRead.mockReset().mockResolvedValue({ ok: true, read: "r1", committed: false, states: [] });
  actions.readReviewSource.mockReset();
});
afterEach(async () => {
  if (root) await act(async () => root!.unmount());
  container.remove();
  root = undefined;
  vi.useRealTimers();
});

describe("evidence source report", () => {
  it("shows the item's complete report revision before the evidence read can be submitted", async () => {
    actions.readReviewSource.mockResolvedValue({ ok: true, source: { report: "skmc-endo/DiagnosticReport/SKM-dr-217", report_version: 2, day: 40, text: report } });
    await mount();
    expect(actions.readReviewSource).toHaveBeenCalledWith(detail.case.id, evidenceItem);
    const panel = container.querySelector('[data-testid="source-report"]')!;
    expect(panel.getAttribute("data-source")).toBe("available");
    expect(panel.textContent).toContain("skmc-endo/DiagnosticReport/SKM-dr-217, version 2");
    expect(panel.textContent).toContain("tissue retrieved and sent for histology");
    await answerAndSubmit();
    expect(actions.submitReviewRead).toHaveBeenCalledWith(detail.case.id, expect.objectContaining({ item: evidenceItem, decision: "adequate" }));
  });

  it.each([
    ["missing", "was not found"],
    ["ambiguous", "More than one report"],
    ["unreadable", "could not be read"],
  ])("announces a %s source and withholds the evidence read", async (status, words) => {
    actions.readReviewSource.mockResolvedValue({ ok: false, status: 409, sourceStatus: status });
    await mount();
    const alert = container.querySelector('[data-testid="source-report"][role="alert"]')!;
    expect(alert.textContent).toContain(words);
    expect(alert.textContent).toContain("Evidence signing is withheld");
    const button = await answerAndSubmit();
    expect(button.getAttribute("aria-disabled")).toBe("true");
    expect(actions.submitReviewRead).not.toHaveBeenCalled();
  });

  it("withholds the evidence read while the source report is still loading", async () => {
    actions.readReviewSource.mockReturnValue(new Promise(() => {}));
    await mount();
    expect(container.querySelector('[data-testid="source-report"]')!.getAttribute("data-source")).toBe("loading");
    await answerAndSubmit();
    expect(actions.submitReviewRead).not.toHaveBeenCalled();
  });
});
