// @vitest-environment happy-dom

import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ReviewDesk } from "@/components/review/review-desk";
import { reviewItemViews, reviewView } from "@/components/review/case-view";
import { CaseDetail, GUIDED_EVIDENCE_LABELS, ReviewState } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

// #162 acceptance 4 on the live desk: every supported non-CRC programme shows its own source report revision beside its
// own evidence labels, and withholds the evidence read when that report cannot be opened. Software presentation fixture
// only: the item, states and report text are synthetic mock inputs, not clinical evidence.
const actions = vi.hoisted(() => ({ claimReviewQuestion: vi.fn(), submitReviewRead: vi.fn(), readReviewSource: vi.fn() }));
vi.mock("@/app/app/review/[id]/actions", () => actions);
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }) }));
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

const base = CaseDetail.parse(hold);
const template = base.review_items.find((item) => item.kinds.includes("evidence"))!;

/** The programme-specific label each case signs; none of them is in the CRC adequacy set except cervical's own. */
const SIGNED: Record<string, string> = {
  cervical: "adequate",
  lung_nodule: GUIDED_EVIDENCE_LABELS.lung_nodule[0],
  diabetic_retinopathy: "R2M0",
  breast: "B4",
  lung_ldct: GUIDED_EVIDENCE_LABELS.lung_ldct[0],
};
const PROGRAMMES = ["cervical", "lung_nodule", "diabetic_retinopathy", "breast", "lung_ldct", "ifhas"] as const;

function caseFor(programme: string) {
  const item = `rv-src-${programme}`;
  const report = `synthetic-${programme}/DiagnosticReport/SYN-${programme}-1`;
  const evidence = { report, agree: false, label: "", quote: "", human_override: "", reader_a_regex: "", reader_b: "" };
  const detail = CaseDetail.parse({
    ...base,
    case: { ...base.case, programme, closed: false, review: "open" },
    evidence: [evidence],
    review_items: [{ ...template, item, kinds: ["evidence"], report, report_version: 3, chain: [{ type: "evidence", body: evidence, prev: "genesis", hash: "3".repeat(64) }] }],
    signoffs: [],
    api_reviews: [],
  });
  const state = ReviewState.parse({
    item, question: "evidence", state: "awaiting_r1", head: "", next_read: "r1", may_submit: true, disagreement: false, canonical_unchanged: true,
    claim_head: "c".repeat(64), claim_actions: { claim: false, renew: true, release: true, takeover: false },
    claim: { owner: "clin-a", role: "clinician", read: "r1", claimed_at: "2026-11-13T10:00:00Z", heartbeat_at: "2026-11-13T10:00:00Z", expires_at: "2026-11-13T10:05:00Z", status: "active" },
    reads: [{ read: "r1", submitted: false }],
  });
  return { detail, state, item, report, text: `Synthetic ${programme} report for the software test. Finding stated in full.` };
}

let root: Root | undefined;
let container: HTMLDivElement;
async function mount(c: ReturnType<typeof caseFor>) {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  await act(async () =>
    root!.render(createElement(ReviewDesk, {
      view: reviewView(c.detail), itemViews: reviewItemViews(c.detail), mode: "live",
      next: { href: "/app/review", label: "Back to Review", last: true }, states: [c.state],
    })),
  );
}
async function pick(label: string) {
  const select = container.querySelector<HTMLSelectElement>('select[aria-label="Evidence label"]');
  if (select) {
    await act(async () => {
      select.value = label;
      select.dispatchEvent(new Event("change", { bubbles: true }));
    });
    return [...select.options].map((o) => o.value).filter(Boolean);
  }
  const radios = [...container.querySelectorAll<HTMLInputElement>("input[type=radio]")];
  await act(async () => radios.find((r) => r.value === label)!.click());
  return radios.map((r) => r.value);
}
async function submit() {
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

describe("non-CRC evidence review opens the item's own source report before signing", () => {
  it.each(PROGRAMMES)("%s: shows the bound report revision beside the programme's own labels", async (programme) => {
    const c = caseFor(programme);
    actions.readReviewSource.mockResolvedValue({ ok: true, source: { report: c.report, report_version: 3, day: 40, text: c.text } });
    await mount(c);
    expect(actions.readReviewSource).toHaveBeenCalledWith(c.detail.case.id, c.item);
    const panel = container.querySelector('[data-testid="source-report"]')!;
    expect(panel.getAttribute("data-source")).toBe("available");
    expect(panel.textContent).toContain(`${c.report}, version 3`);
    expect(panel.textContent).toContain("Finding stated in full.");
    if (programme === "ifhas") return; // the IFHAS label composer is covered by tests/ifhas-composer.test.ts
    const offered = await pick(SIGNED[programme]);
    expect(offered).toEqual(GUIDED_EVIDENCE_LABELS[programme]);
    await submit();
    expect(actions.submitReviewRead).toHaveBeenCalledWith(c.detail.case.id, expect.objectContaining({ item: c.item, decision: SIGNED[programme] }));
  });

  it.each(PROGRAMMES.filter((p) => p !== "ifhas"))("%s: a missing source is announced and the evidence read is withheld", async (programme) => {
    const c = caseFor(programme);
    actions.readReviewSource.mockResolvedValue({ ok: false, status: 409, sourceStatus: "missing" });
    await mount(c);
    const alert = container.querySelector('[data-testid="source-report"][role="alert"]')!;
    expect(alert.textContent).toContain("You cannot sign until the report is shown");
    expect(alert.textContent).not.toContain(c.text);
    await pick(SIGNED[programme]);
    const button = await submit();
    expect(button.getAttribute("aria-disabled")).toBe("true");
    expect(actions.submitReviewRead).not.toHaveBeenCalled();
  });
});
