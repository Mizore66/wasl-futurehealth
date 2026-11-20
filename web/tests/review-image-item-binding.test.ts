import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { reviewItemViews } from "@/components/review/case-view";
import { ReviewDesk } from "@/components/review/review-desk";
import { CaseDetail, ReviewState } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

vi.mock("@/app/app/review/[id]/actions", () => ({ claimReviewQuestion: vi.fn(), submitReviewRead: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }) }));

const base = CaseDetail.parse(hold);
const A = "rv-image-answered";
const B = "rv-image-other";
const MODEL = "hk-mnv3s-v1/exact-item-chain";
/** The model name is not shown on screen; the AI reading block marks where it is revealed. */
const SHOWN = 'data-testid="image-model"';
const next = { href: "/app/review", label: "Back to Review", last: true };

/* Backend-style redaction: aggregate evidence has no image verdict; only the answered item's own chain exposes it. */
function detail(): CaseDetail {
  const d = structuredClone(base);
  const tpl = d.review_items.find((i) => i.kinds.includes("evidence"))!;
  const mk = (item: string, withModel: boolean) => ({
    ...structuredClone(tpl),
    item,
    reason: "image_prep_conflict",
    chain: tpl.chain.map((e) =>
      e.type === "evidence" && withModel ? { ...e, body: { ...e.body, image_reader: { model: MODEL, caecal_photo: "verified", prep: "adequate", hint: "image_prep_conflict" } } } : e,
    ),
  });
  d.review_items = [mk(A, true), mk(B, false)];
  d.evidence[0].image_reader = undefined;
  return d;
}

const state = (item: string, answered: boolean) =>
  ReviewState.parse({
    item, question: "evidence", state: answered ? "awaiting_r2" : "awaiting_r1", head: "", next_read: answered ? "r2" : "r1", may_submit: !answered,
    claim_head: "c".repeat(64),
    claim: { owner: "Rev-A", role: "clinician", read: "r1", claimed_at: "2026-11-13T10:00:00Z", heartbeat_at: "2026-11-13T10:00:00Z", expires_at: "2026-11-13T10:05:00Z", status: "active" },
    claim_actions: { claim: false, renew: true, release: true, takeover: false },
    reads: [{ read: "r1", submitted: answered, ...(answered ? { decision: "adequate", hash: "aaaaaaaaaaaa1111" } : {}) }],
    disagreement: false, canonical_unchanged: true,
  });

const render = (d: CaseDetail, states: ReviewState[]) => {
  const views = reviewItemViews(d);
  return renderToStaticMarkup(createElement(ReviewDesk, { view: views[A], itemViews: views, mode: "live", next, states }));
};
const section = (markup: string, item: string) => {
  const start = markup.indexOf(`data-context-item="${item}"`);
  expect(start).toBeGreaterThan(-1);
  const other = markup.indexOf("data-context-item=", start + 10);
  return markup.slice(start, other === -1 ? undefined : other);
};

describe("image model comes only from the exact item chain", () => {
  it("takes the model from the answered item's chain and never from aggregate evidence", () => {
    const views = reviewItemViews(detail());
    expect(views[A].imageContext?.model).toBe(MODEL);
    expect(views[B].imageContext).toBeUndefined();
  });

  it("reveals the model in the answered item's context only, with unique heading ids, and does not unlock the other item", () => {
    const markup = render(detail(), [state(A, true), state(B, false)]);
    expect(section(markup, A)).toContain(SHOWN);
    expect(section(markup, B)).not.toContain(SHOWN);
    expect(section(markup, B)).toContain("image-model-hidden");
    expect(markup.split(SHOWN).length - 1).toBe(1);
    const ids = [...markup.matchAll(/ id="([^"]+)"/g)].map((m) => m[1]);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("keeps both models hidden before any answer", () => {
    const markup = render(detail(), [state(A, false), state(B, false)]);
    expect(markup).not.toContain(SHOWN);
    expect(markup).not.toContain(MODEL);
  });
});
