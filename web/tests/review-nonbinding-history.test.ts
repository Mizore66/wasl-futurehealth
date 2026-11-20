import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { HistoryCard } from "@/components/case/audit-view";
import { SignoffsCard } from "@/components/case/sections";
import { Audit, AuditLedgerEvent, CaseDetail, LedgerEvent, UndoProjection } from "@/lib/api/schema";
import { isNonbindingLegacyHistory, legacyReviewMayAnswer } from "@/lib/domain/review-authority";
import { caseStage, openReviewItems } from "@/lib/domain/stage";
import { linkTrust } from "@/lib/people/identity";
import hold from "@/data/api/case_review_hold.json";
import auditFixture from "./fixtures/audit_signed_reviews.json";

vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: () => {}, push: () => {} }) }));

const base = CaseDetail.parse(hold);
const item = base.review_items.find((r) => r.kinds.includes("evidence"))!;
const archived = {
  case: base.case.id, item: item.item, decision: "adequate", note: "synthetic old-format history",
  role: "clinician", actor: "clin-a", action: "signoff_evidence", at: "2026-11-12T00:00:00Z",
  mode: "demo", prev: "genesis", hash: "legacy-hash", source_binding: "legacy_unbound_nonbinding",
};
const detail = (): CaseDetail => ({
  ...base, case: { ...base.case, closed: false, review: "open" },
  holds: [], signoffs: [], api_review_states: [],
  review_items: [{ ...item, kinds: ["link", "evidence"] }],
  api_reviews: [LedgerEvent.parse(archived)],
});

describe("recognized nonbinding legacy history", () => {
  it("keeps the server annotation in typed detail and audit without trusting a bound client flag", () => {
    const review = LedgerEvent.parse(archived);
    const auditReview = AuditLedgerEvent.parse(archived);
    if (review.ledger !== "review" || auditReview.ledger !== "review") throw new Error("recognized legacy history did not retain its review shape");
    expect(review.source_binding).toBe("legacy_unbound_nonbinding");
    expect(auditReview.source_binding).toBe("legacy_unbound_nonbinding");
    expect(LedgerEvent.parse({ ...archived, source_binding: "bound" }).ledger).toBe("unknown");
  });

  it("does not resolve either question of a combined item or confer identity authority", () => {
    const c = detail();
    expect(openReviewItems(c).map((r) => r.item)).toEqual([item.item]);
    expect(caseStage(c).key).toBe("waiting_for_reviewers");
    expect(linkTrust(c, c.links[0])).toBe("held");
  });

  it("preserves the existing unmarked legacy and absent-field behavior", () => {
    const legacy = LedgerEvent.parse({ ...archived, source_binding: undefined, decision: "same", action: "signoff_link", role: "reviewer", protocol: undefined, read: undefined, question: "link" });
    expect(legacyReviewMayAnswer(legacy)).toBe(true);
    const c = { ...detail(), api_reviews: [legacy] };
    expect(linkTrust(c, c.links[0])).toBe("signed");
    expect(openReviewItems(c)).toEqual([]);
  });

  it("cannot turn protocol or claim provenance into a legacy answer with any binding flag", () => {
    for (const field of ["protocol", "read", "nonce", "claim_head", "claimed_at", "heartbeat_at", "expires_at", "expected_claim_head", "expected_head", "resolution_reads"]) {
      for (const source_binding of [undefined, "bound", "legacy_unbound_nonbinding"]) {
        const event = { ...archived, ledger: "review", source_binding, [field]: "modern-provenance" };
        expect(legacyReviewMayAnswer(event), field).toBe(false);
        expect(isNonbindingLegacyHistory(event), field).toBe(false);
      }
    }
    expect(legacyReviewMayAnswer({ ...archived, ledger: "claim", type: "review_claim" })).toBe(false);
    expect(isNonbindingLegacyHistory({ ...archived, ledger: "claim", type: "review_claim" })).toBe(false);
  });

  it("labels both clinical-detail history and auditor history without mutating original labels", () => {
    const c = detail();
    const audit = Audit.parse({ ...auditFixture, history: [archived] });
    const before = JSON.stringify([c.api_reviews, audit.history]);
    for (const html of [renderToStaticMarkup(createElement(SignoffsCard, { detail: c })), renderToStaticMarkup(createElement(HistoryCard, { audit, programme: "crc" }))]) {
      expect(html).toContain("Older entry — not binding");
      expect(html).toContain("does not settle a review item");
    }
    expect(JSON.stringify([c.api_reviews, audit.history])).toBe(before);
  });

  it("keeps original-action compensation controls available without treating history as authority", () => {
    const audit = Audit.parse({ ...auditFixture, history: [archived] });
    const projection = UndoProjection.parse({ ...base, case: base.case.id, case_status: "active", canonical_state: "open", note: "review-ledger status only",
      events: [{ hash: archived.hash, kind: "review", actor: "clin-a", action: "signoff_evidence", target: "", status: "active", at: archived.at }] });
    const html = renderToStaticMarkup(createElement(HistoryCard, { audit, programme: "crc", governance: { caseId: base.case.id, projection, signedIn: true } }));
    expect(html).toContain("Older entry — not binding");
    expect(html).toMatch(/Undo/i);
    expect(html).toMatch(/Contest/i);
    expect(legacyReviewMayAnswer(LedgerEvent.parse(archived))).toBe(false);
  });
});
