import { readFileSync } from "node:fs";
import path from "node:path";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { reviewView } from "@/components/review/case-view";
import { answerWords, draftKey, orderedStates, readFailureSentence, readLine, stateHeadline } from "@/components/review/decision";
import { protocolFinished } from "@/components/review/queue";
import { ReviewDesk } from "@/components/review/review-desk";
import { ReviewerLogin } from "@/components/review/reviewer-login";
import { HistoryCard } from "@/components/case/audit-view";
import { SignoffsCard } from "@/components/case/sections";
import { Audit, CaseDetail, ReviewState } from "@/lib/api/schema";
import { allQuestionsResolved, caseStage, concealsInitialRead, openReviewItems } from "@/lib/domain/stage";
import { linkTrust } from "@/lib/people/identity";
import hold from "@/data/api/case_review_hold.json";
import { programmeDetail } from "./programme-cases";

vi.mock("next/navigation", () => ({ useRouter: () => ({ refresh: () => {}, push: () => {} }) }));

const LINK = "rv-636161dc42837e08";
const EVID = "rv-d55f7ccf9f12c409";

const base = CaseDetail.parse(hold);
const open = (): CaseDetail => ({ ...base, case: { ...base.case, closed: false, review: "open" }, signoffs: [], api_reviews: [] });
const state = (over: Partial<ReviewState> & Pick<ReviewState, "item" | "question" | "state">): ReviewState => {
  const value = { head: "", next_read: null, may_submit: false, reads: [], disagreement: false, canonical_unchanged: true, ...over };
  if (value.may_submit && value.next_read && value.claim === undefined) {
    value.claim_head = "c".repeat(64);
    value.claim = { owner: "Rev-A", role: "reviewer", read: value.next_read, claimed_at: "2026-11-13T10:00:00Z", heartbeat_at: "2026-11-13T10:00:00Z", expires_at: "2026-11-13T10:05:00Z", status: "active" };
    value.claim_actions = { claim: false, renew: true, release: true, takeover: false };
  }
  return ReviewState.parse(value);
};

const awaitingR1 = (item: string, question: "link" | "evidence", may = true) =>
  state({ item, question, state: "awaiting_r1", next_read: "r1", may_submit: may, reads: [{ read: "r1", submitted: false }] });
const unclaimed = (item: string, question: "link" | "evidence") =>
  state({ item, question, state: "awaiting_r1", next_read: "r1", may_submit: false, claim: null, claim_head: "", claim_actions: { claim: true, renew: false, release: false, takeover: false }, reads: [{ read: "r1", submitted: false }] });
/** What the second reader receives: the first read exists but carries no decision or hash. */
const concealedR2 = (item: string, question: "link" | "evidence", may = true) =>
  state({ item, question, state: "awaiting_r2", head: "h1", next_read: "r2", may_submit: may, reads: [{ read: "r1", submitted: true }, { read: "r2", submitted: false }] });
const agreed = (item: string, question: "link" | "evidence", decision: string) =>
  state({
    item, question, state: "resolved_agreed", head: "h2", final_decision: decision, resolution_basis: "agreement",
    reads: [{ read: "r1", submitted: true, decision, hash: "aaaaaaaaaaaa1111" }, { read: "r2", submitted: true, decision, hash: "bbbbbbbbbbbb2222" }],
  });
const disagreed = (item: string, question: "link" | "evidence", a: string, b: string, may: boolean) =>
  state({
    item, question, state: "awaiting_r3", head: "h2", next_read: "r3", may_submit: may, disagreement: true,
    reads: [{ read: "r1", submitted: true, decision: a, hash: "aaaaaaaaaaaa1111" }, { read: "r2", submitted: true, decision: b, hash: "bbbbbbbbbbbb2222" }, { read: "r3", submitted: false }],
  });
const senior = (item: string, question: "link" | "evidence", decision: string) =>
  state({
    item, question, state: "resolved_senior", head: "h3", final_decision: decision, resolution_basis: "senior",
    reads: [{ read: "r1", submitted: true, decision: "same", hash: "aaaaaaaaaaaa1111" }, { read: "r2", submitted: true, decision: "different", hash: "bbbbbbbbbbbb2222" }, { read: "r3", submitted: true, decision, hash: "cccccccccccc3333" }],
  });

const next = { href: "/app/review", label: "Back to Review", last: true };
const desk = (states: ReviewState[], detail: CaseDetail = open()) =>
  renderToStaticMarkup(createElement(ReviewDesk, { view: reviewView(detail), mode: "live", next, states }));

describe("question status wording", () => {
  it("words all five states from the viewer's side", () => {
    expect(stateHeadline(awaitingR1(LINK, "link")).text).toBe("First read due");
    expect(stateHeadline(concealedR2(LINK, "link")).text).toBe("Second read due");
    expect(stateHeadline(concealedR2(LINK, "link", false)).text).toMatch(/waiting for a different reviewer|Waiting for a second read/i);
    expect(stateHeadline(disagreed(LINK, "link", "same", "different", true)).text).toContain("senior resolution due");
    expect(stateHeadline(disagreed(LINK, "link", "same", "different", false)).text).toContain("waiting for a senior");
    expect(stateHeadline(agreed(LINK, "link", "same")).text).toBe("Resolved: both reads agree");
    expect(stateHeadline(senior(LINK, "link", "same")).text).toBe("Resolved by a senior");
  });

  it("keys drafts by item, question and the read they will become", () => {
    expect(draftKey(LINK, "link", "r2")).toBe(`${LINK}:link:r2`);
    expect(draftKey(LINK, "link", "r2")).not.toBe(draftKey(LINK, "link", "r1"));
  });

  it("orders link before evidence and keeps combined-kind items as two questions", () => {
    const ordered = orderedStates([awaitingR1("rv-x", "evidence"), awaitingR1("rv-x", "link")]);
    expect(ordered.map((s) => s.question)).toEqual(["link", "evidence"]);
  });

  it("labels reads and never invents a concealed answer", () => {
    expect(readLine({ read: "r1", submitted: true }, "link")).toContain("hidden");
    expect(readLine({ read: "r1", submitted: true }, "link")).not.toContain("Same");
    expect(readLine({ read: "r3", submitted: true, decision: "same" }, "link")).toBe("Senior resolution: Same patient");
    expect(answerWords("wk_benign", "evidence")).toBe("Evidence label wk_benign");
  });

  it("turns 401/403/404/409 into refusals that claim no save", () => {
    for (const status of [401, 403, 404, 409]) expect(readFailureSentence({ status, error: "x" })).toMatch(/^Not saved/);
  });
});

describe("blinded review desk markup", () => {
  it("renders one card per (item, question) for a combined-kind item", () => {
    const html = desk([awaitingR1("rv-both", "link"), awaitingR1("rv-both", "evidence")]);
    expect(html.match(/data-question="/g)).toHaveLength(2);
    expect(html).toContain("First read due");
    expect(html).toContain("Submit first read");
    expect(html).not.toContain("Close the case");
  });

  it("an unclaimed eligible question has a claim button and no answer form", () => {
    const html = desk([unclaimed(LINK, "link")]);
    expect(html).toContain("Unclaimed");
    expect(html).toContain("Claim first read");
    expect(html).not.toContain("Submit first read");
    expect(html).toContain(`id="review-question-${LINK}-link"`);
  });

  it("another actor's active claim shows owner and expiry but no answer or claim control", () => {
    const other = state({ item: LINK, question: "link", state: "awaiting_r1", next_read: "r1", may_submit: false,
      claim_head: "c".repeat(64), claim: { owner: "Rev-B", role: "reviewer", read: "r1", claimed_at: "2026-11-13T10:00:00Z", heartbeat_at: "2026-11-13T10:00:00Z", expires_at: "2026-11-13T10:05:00Z", status: "active" },
      claim_actions: { claim: false, renew: false, release: false, takeover: false }, reads: [{ read: "r1", submitted: false }] });
    const html = desk([other]);
    expect(html).toContain("Claimed by Rev-B until");
    expect(html).not.toContain("Submit first read");
    expect(html).not.toContain("Claim first read");
  });

  it("an expired eligible claim offers takeover without exposing a stale answer form", () => {
    const expired = state({ item: LINK, question: "link", state: "awaiting_r1", next_read: "r1", may_submit: false,
      claim_head: "c".repeat(64), claim: { owner: "Rev-B", role: "reviewer", read: "r1", claimed_at: "2026-11-13T10:00:00Z", heartbeat_at: "2026-11-13T10:00:00Z", expires_at: "2026-11-13T10:05:00Z", status: "expired" },
      claim_actions: { claim: false, renew: false, release: false, takeover: true }, reads: [{ read: "r1", submitted: false }] });
    const html = desk([expired]);
    expect(html).toContain("Claim by Rev-B expired at");
    expect(html).toContain("Take over claim");
    expect(html).not.toContain("Submit first read");
  });

  it("an owned active claim shows the answer form and release control", () => {
    const html = desk([awaitingR1(LINK, "link")]);
    expect(html).toContain("Claimed by Rev-A until");
    expect(html).toContain("Release claim");
    expect(html).toContain("Submit first read");
    expect(html).toContain("Heartbeat every 60 seconds");
  });

  it("released and frozen states expose no stale answer form or mutation", () => {
    const released = state({ ...unclaimed(LINK, "link"), claim_head: "b".repeat(64), claim_actions: { claim: false, renew: false, release: false, takeover: false } });
    const frozen = state({ ...released, item: EVID, question: "evidence", frozen: true });
    const html = desk([released, frozen]);
    expect(html).not.toContain("Submit first read");
    expect(html).not.toContain("Release claim");
    expect(html).not.toContain("Take over claim");
  });

  it("offers no credential choice or role selector", () => {
    const html = desk([awaitingR1(LINK, "link")]);
    expect(html).not.toMatch(/reviewer_2|reviewer_3|clinician_2|Choose a credential|<select[^>]*credential/i);
  });

  it("while awaiting r2 the other answer is absent from the markup", () => {
    const html = desk([concealedR2(LINK, "link"), concealedR2(EVID, "evidence")]);
    expect(html).toContain("Second read due");
    expect(html).toContain("Submit second read");
    expect(html).toContain("hidden until the second read");
    expect(html).not.toMatch(/data-read="r1"[^>]*>First read: (?!submitted)/);
    expect(html).not.toContain("aaaaaaaaaaaa");
    expect(html).not.toMatch(/First read: (Same|Not the same|Report)/);
  });

  it("hides the form from the author waiting on a different reviewer", () => {
    const html = desk([concealedR2(LINK, "link", false)]);
    expect(html).not.toContain("Submit second read");
    expect(html).toContain("stays hidden");
  });

  it("agreement shows both answers by hash and no senior form", () => {
    const html = desk([agreed(LINK, "link", "same")]);
    expect(html).toContain("First read: Same patient");
    expect(html).toContain("Second read: Same patient");
    expect(html).toContain("aaaaaaaaaaaa");
    expect(html).toContain("bbbbbbbbbbbb");
    expect(html).toContain("No senior form is needed");
    expect(html).not.toContain("Submit senior resolution");
    expect(html).toContain("Case data is unchanged");
  });

  it("disagreement shows both answers and only an authorised senior sees the r3 form", () => {
    const senior = desk([disagreed(LINK, "link", "same", "different", true)]);
    const ordinary = desk([disagreed(LINK, "link", "same", "different", false)]);
    for (const html of [senior, ordinary]) {
      expect(html).toContain("First read: Same patient");
      expect(html).toContain("Second read: Not the same patient");
    }
    expect(senior).toContain("Submit senior resolution");
    expect(ordinary).not.toContain("Submit senior resolution");
    expect(ordinary).toContain("Only an authorised senior");
  });

  it("renders terminal reads separately by hash, with the senior resolution last", () => {
    const html = desk([senior(LINK, "link", "same")]);
    const order = ["First read: Same patient", "Second read: Not the same patient", "Senior resolution: Same patient"].map((t) => html.indexOf(t));
    expect(order.every((i) => i >= 0)).toBe(true);
    expect(order).toEqual([...order].sort((a, b) => a - b));
    for (const h of ["aaaaaaaaaaaa", "bbbbbbbbbbbb", "cccccccccccc"]) expect(html).toContain(h);
  });

  it("an unresolved question keeps the case open; resolved questions offer the next case", () => {
    const partial = desk([agreed(LINK, "link", "same"), concealedR2(EVID, "evidence")]);
    expect(partial).toContain("1 of 2 questions resolved");
    expect(partial).toContain("stays open");
    expect(partial).not.toContain("Back to Review");
    const done = desk([agreed(LINK, "link", "same"), agreed(EVID, "evidence", "adequate")]);
    expect(done).toContain("2 of 2 questions resolved");
    expect(done).toContain("Back to Review");
  });

  it("keeps non-CRC evidence labels and programme wording", () => {
    const detail = programmeDetail("lung_ldct");
    detail.case = { ...detail.case, closed: false };
    detail.review_items = [{ item: "rv-ldct", loop: detail.case.id, kinds: ["evidence"], pair: [], p: 0, report: detail.evidence[0].report, verdict: "disagree", quote: "", chain: [] }];
    const html = desk([awaitingR1("rv-ldct", "evidence")], detail);
    expect(html).toContain("Programme evidence (clinician discipline)");
    expect(html).toContain("Select the LDCT report evidence label");
    expect(html).not.toContain("Is the colonoscopy report adequate?");
  });

  it("fixtures mode stays a non-persisting preview with no sign-out or submit control", () => {
    const html = renderToStaticMarkup(createElement(ReviewDesk, { view: reviewView(open()), mode: "fixtures", next }));
    expect(html).toContain("nothing here is appended or saved");
    expect(html).not.toContain("Sign out");
    expect(html).not.toContain("Submit first read");
  });

  it("says nothing can be submitted when the API reports no independent review", () => {
    expect(desk([])).toContain("did not report an independent review");
  });
});

describe("sign-in form", () => {
  it("is a token form with no role or credential picker", () => {
    const html = renderToStaticMarkup(createElement(ReviewerLogin, { caseId: "c" }));
    expect(html).toContain('type="password"');
    expect(html).toContain("Review token");
    expect(html).not.toMatch(/<select|type="radio"/);
  });
});

describe("overlays", () => {
  const withStates = (states: ReviewState[], extra: Partial<CaseDetail> = {}): CaseDetail => ({ ...open(), api_review_states: states, ...extra });
  const protocolEvent = (item: string, read: "r1" | "r2" | "r3", decision: string) => ({
    case: base.case.id, item, decision, note: "", role: "reviewer", actor: "x", action: "signoff_link", at: "2026-11-12T00:00:00Z",
    mode: "api", prev: "p", hash: `h-${read}`, protocol: "wasl-independent-review/1", read, question: "link", ledger: "review" as const,
  });

  it("r1 alone leaves the item open, the stage waiting and the link held", () => {
    const c = withStates([concealedR2(LINK, "link"), concealedR2(EVID, "evidence")], { api_reviews: [protocolEvent(LINK, "r1", "same")] as never });
    expect(openReviewItems(c).map((r) => r.item)).toEqual([LINK, EVID]);
    expect(caseStage(c).key).toBe("waiting_for_reviewers");
    expect(linkTrust(c, c.links[0])).toBe("held");
  });

  it("r1 plus r2 disagreement is still not final", () => {
    const c = withStates([disagreed(LINK, "link", "same", "different", false), agreed(EVID, "evidence", "adequate")], {
      api_reviews: [protocolEvent(LINK, "r1", "same"), protocolEvent(LINK, "r2", "different")] as never,
    });
    expect(openReviewItems(c).map((r) => r.item)).toEqual([LINK]);
    expect(linkTrust(c, c.links[0])).toBe("held");
    expect(allQuestionsResolved(c)).toBe(false);
  });

  it("agreement or a senior resolution produces the final overlay decision", () => {
    const yes = withStates([agreed(LINK, "link", "same"), agreed(EVID, "evidence", "adequate")]);
    expect(openReviewItems(yes)).toEqual([]);
    expect(linkTrust(yes, yes.links[0])).toBe("signed");
    const no = withStates([senior(LINK, "link", "different"), agreed(EVID, "evidence", "adequate")]);
    expect(linkTrust(no, no.links[0])).toBe("rejected");
  });

  it("legacy events keep their meaning when no protocol states exist", () => {
    const legacy = { ...protocolEvent(LINK, "r1", "same"), protocol: undefined, read: undefined };
    const c = withStates([], { api_reviews: [legacy] as never });
    expect(linkTrust(c, c.links[0])).toBe("signed");
    expect(openReviewItems(c).map((r) => r.item)).toEqual([EVID]);
    expect(protocolFinished(c)).toBe(false);
  });

  it("queue completion needs every question resolved", () => {
    expect(protocolFinished({ api_review_states: [agreed(LINK, "link", "same"), concealedR2(EVID, "evidence")] })).toBe(false);
    expect(protocolFinished({ api_review_states: [agreed(LINK, "link", "same"), senior(EVID, "evidence", "adequate")] })).toBe(true);
    expect(protocolFinished({})).toBe(false);
  });

  it("detects a concealed first read in either server encoding", () => {
    expect(concealsInitialRead([concealedR2(LINK, "link")])).toBe(true);
    expect(concealsInitialRead([agreed(LINK, "link", "same")])).toBe(false);
    expect(concealsInitialRead([])).toBe(false);
  });

  it("case rows label First read, Second read and Senior resolution and call none a final sign-off", () => {
    const events = [protocolEvent(LINK, "r1", "same"), protocolEvent(LINK, "r2", "different"), protocolEvent(LINK, "r3", "same")];
    const c = withStates([senior(LINK, "link", "same")], { api_reviews: events as never });
    const html = renderToStaticMarkup(SignoffsCard({ detail: c }));
    for (const label of ["First read:", "Second read:", "Senior resolution:"]) expect(html).toContain(label);
    expect(html).toContain("not a final sign-off");
    expect(html).toContain("1 of 1 question resolved");
  });

  it("audit rows label slots, keep redaction and offer no undo/contest on protocol reads", () => {
    const audit = Audit.parse({
      ...JSON.parse(readFileSync(path.join(__dirname, "fixtures", "audit_signed_reviews.json"), "utf8")),
      history: [{ case: base.case.id, item: LINK, decision: "same", role: "reviewer", action: "signoff_link", at: "2026-11-12T00:00:00Z", mode: "api", prev: "genesis", hash: "h-r1", protocol: "wasl-independent-review/1", read: "r1", question: "link" }],
    });
    const html = renderToStaticMarkup(HistoryCard({ audit, programme: "crc" }));
    expect(html).toContain("First read:");
    expect(html).not.toContain("Unnamed");
  });
});

describe("review page uses the viewer's own credential", () => {
  const page = readFileSync(path.join(__dirname, "..", "app", "app", "review", "[id]", "page.tsx"), "utf8");

  it("never calls the shared-token getCase and reads through getReviewCase", () => {
    expect(page).toContain("getReviewCase");
    expect(page).not.toMatch(/\bgetCase\b/);
  });

  it("shows sign-in before any queue or case read, and gates embedded history on concealment", () => {
    expect(page.indexOf("ReviewerLogin")).toBeLessThan(page.indexOf("reviewQueue()"));
    expect(page).toMatch(/gatedHistory/);
    expect(page).toMatch(/history\.status === "concealed"/);
  });

  it("the desk consumes only the one-read action", () => {
    const deskSource = readFileSync(path.join(__dirname, "..", "components", "review", "review-desk.tsx"), "utf8");
    expect(deskSource).toContain("submitReviewRead");
    expect(deskSource).not.toMatch(/signReview|postReview\b/);
  });
});
