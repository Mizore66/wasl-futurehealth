// @vitest-environment happy-dom

import { act, createElement } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ReadersCard } from "@/components/case/sections";
import { ReviewDesk } from "@/components/review/review-desk";
import { ReadersTable } from "@/components/review/readers-table";
import { reviewItemViews, reviewView } from "@/components/review/case-view";
import { CaseDetail, ReviewState } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

vi.mock("@/app/app/review/[id]/actions", () => ({ claimReviewQuestion: vi.fn(), submitReviewRead: vi.fn(), readReviewSource: vi.fn((..._a: unknown[]): Promise<unknown> => new Promise(() => {})) }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }) }));
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });

// Software presentation fixture only: the API states/signoff are mock inputs,
// not a claim that a clinical reviewer or model generated this evidence.
const base = CaseDetail.parse(hold);
const template = base.review_items.find((item) => item.kinds.includes("evidence"))!;
const report = "noor-endo/DiagnosticReport/DR-versioned-software-control";
const previousItem = "rv-dr-previous-v1";
const currentItem = "rv-dr-current-v2";
const firstEvidence = {
  report,
  agree: false,
  label: "",
  quote: "",
  human_override: "R2M0",
  reader_a_regex: "R2M0",
  reader_b: "R1M0",
  reader_a_fields: { r_right: "2", r_left: "1", m_right: "0", m_left: "0", label: "R2M0", quote: "V1_SIGNED_RULES_HISTORY" },
  reader_b_fields: { r_right: "1", r_left: "1", m_right: "0", m_left: "0", label: "R1M0", quote: "V1_SIGNED_SECOND_HISTORY" },
};
const currentEvidence = {
  ...firstEvidence,
  human_override: "",
  reader_a_regex: "R1M0",
  reader_b: "R2M0",
  reader_a_fields: { r_right: "1", r_left: "1", m_right: "0", m_left: "0", label: "R1M0", quote: "V2_CURRENT_RULES_QUOTE" },
  reader_b_fields: { r_right: "2", r_left: "1", m_right: "0", m_left: "0", label: "R2M0", quote: "V2_CURRENT_SECOND_QUOTE" },
};
const raised = (item: string, version: number, evidence: typeof firstEvidence) => ({
  ...template,
  item,
  kinds: ["evidence"],
  report,
  report_version: version,
  chain: [{ type: "evidence", body: evidence, prev: "genesis", hash: String(version).repeat(64) }],
});
const detail = CaseDetail.parse({
  ...base,
  case: { ...base.case, programme: "diabetic_retinopathy", closed: false },
  evidence: [firstEvidence, currentEvidence],
  review_items: [raised(previousItem, 1, firstEvidence), raised(currentItem, 2, currentEvidence)],
  signoffs: [{ item: previousItem, loop: template.loop, kinds: ["evidence"], role: "clinician", reviewer: "Synthetic Previous Clinician", reads: ["synthetic-previous-read"], link: "", evidence: "R2M0", verdict: "" }],
});
const states = [
  ReviewState.parse({
    item: previousItem, question: "evidence", state: "resolved_agreed", head: "old-head", next_read: null,
    may_submit: false, frozen: true, claim: null, reads: [], disagreement: false,
    final_decision: "R2M0", canonical_unchanged: true,
  }),
  ReviewState.parse({
    item: currentItem, question: "evidence", state: "awaiting_r1", head: "", next_read: "r1", may_submit: true,
    claim_head: "c".repeat(64),
    claim: { owner: "Synthetic Current Clinician", role: "clinician", read: "r1", claimed_at: "2026-11-13T10:00:00Z", heartbeat_at: "2026-11-13T10:00:00Z", expires_at: "2026-11-13T10:05:00Z", status: "active" },
    claim_actions: { claim: false, renew: true, release: true, takeover: false },
    reads: [{ read: "r1", submitted: false }], disagreement: false, canonical_unchanged: true,
  }),
];

let root: Root | undefined;
let container: HTMLDivElement | undefined;
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
});

it("shows the current DR item's own version2 automated readers beside its active form, not only signed version1 shared history", async () => {
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  const sharedEvidence = detail.evidence.map((ev, i) => createElement(ReadersCard, { key: i, detail, ev, id: `shared-history-${i}` }));
  await act(async () => root!.render(createElement(ReviewDesk, {
    view: reviewView(detail), itemViews: reviewItemViews(detail), mode: "live",
    next: { href: "/app/review", label: "Back to Review", last: true }, states, sharedEvidence,
  })));

  const oldCard = container.querySelector<HTMLElement>(`section[data-item="${previousItem}"][data-question="evidence"]`);
  expect(oldCard?.querySelector('input[type="radio"], select[aria-label="Evidence label"]') ?? null).toBeNull();
  const currentCard = container.querySelector<HTMLElement>(`section[data-item="${currentItem}"][data-question="evidence"]`);
  expect(currentCard).not.toBeNull();
  const currentChoices = currentCard!.querySelector<HTMLSelectElement>('select[aria-label="Evidence label"]');
  expect(currentChoices).not.toBeNull();
  expect(currentChoices!.disabled).toBe(false);
  expect(currentChoices!.querySelector('option[value="R2M0"]')).not.toBeNull();
  const ownContext = container.querySelector<HTMLElement>(`[data-context-item="${currentItem}"]`);
  expect(ownContext).not.toBeNull();
  expect(ownContext!.textContent).toContain("V2_CURRENT_RULES_QUOTE");
  expect(ownContext!.textContent).toContain("V2_CURRENT_SECOND_QUOTE");
  expect(ownContext!.textContent).not.toContain("V1_SIGNED_RULES_HISTORY");
  expect(ownContext!.textContent).not.toContain("V1_SIGNED_SECOND_HISTORY");
});

it("serializes only known automated reader fields, never human overrides or loose private fields", () => {
  const item = detail.review_items.find((value) => value.item === currentItem)!;
  const privateDetail = CaseDetail.parse({
    ...detail,
    review_items: [{
      ...item,
      chain: [{ type: "evidence", prev: "genesis", hash: "8".repeat(64), body: {
        ...currentEvidence,
        human_override: "PRIVATE_HUMAN_OVERRIDE",
        independent_read_answer: "PRIVATE_OTHER_READ",
        reader_a_fields: { ...currentEvidence.reader_a_fields, unknown_private: "PRIVATE_NESTED_A" },
        reader_b_fields: { ...currentEvidence.reader_b_fields, human_answer: "PRIVATE_NESTED_B" },
      } }],
    }],
  });
  const snapshot = reviewItemViews(privateDetail)[currentItem].automatedEvidence;
  expect(snapshot).toBeDefined();
  expect(snapshot!.reader_a_fields).toMatchObject({ r_right: "1", quote: "V2_CURRENT_RULES_QUOTE" });
  expect(snapshot!.reader_b_fields).toMatchObject({ r_right: "2", quote: "V2_CURRENT_SECOND_QUOTE" });
  expect(JSON.stringify(snapshot)).not.toContain("PRIVATE_");
  expect(snapshot).not.toHaveProperty("human_override");
  expect(snapshot).not.toHaveProperty("independent_read_answer");
});

it("shows unavailable individual reader output instead of using an overall label as two invented answers", async () => {
  const item = detail.review_items.find((value) => value.item === currentItem)!;
  const unavailableDetail = CaseDetail.parse({
    ...detail,
    review_items: [{
      ...item,
      chain: [{ type: "evidence", prev: "genesis", hash: "9".repeat(64), body: {
        report, agree: false, label: "OVERALL_LABEL_NOT_READER_OUTPUT", quote: "",
      } }],
    }],
  });
  const view = reviewItemViews(unavailableDetail)[currentItem];
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
  await act(async () => root!.render(createElement(ReadersTable, { view })));
  expect(container.querySelector("dl")!.textContent).toContain("Reader AUnavailable");
  expect(container.querySelector("dl")!.textContent).toContain("Reader BUnavailable");
  expect(container.textContent).not.toContain("OVERALL_LABEL_NOT_READER_OUTPUT");
  expect(container.textContent).toContain("No quote available");
});
