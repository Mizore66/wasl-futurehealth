import { describe, expect, it } from "vitest";
import { CaseDetail } from "@/lib/api/schema";
import { linkTrust } from "@/lib/people/identity";
import { caseStage, openReviewItems } from "@/lib/domain/stage";
import hold from "@/data/api/case_review_hold.json";

const base = CaseDetail.parse(hold);
const linkItem = base.review_items.find((r) => r.kinds.includes("link"))!;
const evidenceItem = base.review_items.find((r) => r.kinds.includes("evidence"))!;
const core = { case: base.case.id, role: "reviewer", actor: "synthetic", at: "2026-11-13T10:00:00Z", mode: "demo", prev: "p" };

const review = (hash: string, item: string, decision: string) =>
  ({ ...core, ledger: "review", item, decision, note: "", action: "signoff", hash }) as never;
const undo = (hash: string, target: string) =>
  ({ ...core, ledger: "compensation", kind: "undo", target, target_action: "signoff", action_head: "h", target_actor: "synthetic", note: "n", action: "undo", hash }) as never;
const contest = (hash: string, target: string) =>
  ({ ...core, ledger: "compensation", kind: "contest", target, target_action: "signoff", action_head: "h", target_actor: "synthetic", note: "n", action: "contest", hash }) as never;
const resolve = (hash: string, target: string, resolution: "upheld" | "rejected") =>
  ({ ...core, ledger: "compensation", kind: "contest_resolution", resolution, target, target_action: "contest", action_head: "h", target_actor: "synthetic", note: "n", action: "resolve", hash }) as never;

const withLedger = (items: typeof base.review_items, events: unknown[]): CaseDetail => ({
  ...base,
  case: { ...base.case, closed: false },
  signoffs: [],
  evidence: items.some((i) => i.kinds.includes("evidence")) ? base.evidence : [],
  holds: [],
  review_items: items,
  api_review_states: [],
  api_reviews: events as never,
});
const linkCase = (events: unknown[]) => withLedger([linkItem], events);
const trust = (c: CaseDetail) => linkTrust(c, c.links[0]);
const open = (c: CaseDetail) => openReviewItems(c).map((r) => r.item);

describe("compensated legacy reviews leave the clinical projections", () => {
  it("baseline: the link item is held and open before any review", () => {
    const c = linkCase([]);
    expect(trust(c)).toBe("held");
    expect(open(c)).toEqual([linkItem.item]);
    expect(caseStage(c).key).toBe("needs_second_identifier");
  });

  it("an active same signs the link and closes the item", () => {
    const c = linkCase([review("r1", linkItem.item, "same")]);
    expect(trust(c)).toBe("signed");
    expect(open(c)).toEqual([]);
  });

  it("same followed by undo reopens the item and restores the pre-review stage", () => {
    const c = linkCase([review("r1", linkItem.item, "same"), undo("u1", "r1")]);
    expect(trust(c)).toBe("held");
    expect(open(c)).toEqual([linkItem.item]);
    expect(caseStage(c).key).toBe("needs_second_identifier");
  });

  it("different followed by undo no longer rejects the link", () => {
    const c = linkCase([review("r1", linkItem.item, "different"), undo("u1", "r1")]);
    expect(trust(c)).toBe("held");
    expect(open(c)).toEqual([linkItem.item]);
  });

  it("an upheld contest compensates like undo", () => {
    const c = linkCase([review("r1", linkItem.item, "same"), contest("c1", "r1"), resolve("x1", "c1", "upheld")]);
    expect(trust(c)).toBe("held");
    expect(open(c)).toEqual([linkItem.item]);
  });

  it("an open or rejected contest leaves the review effective", () => {
    const opened = linkCase([review("r1", linkItem.item, "same"), contest("c1", "r1")]);
    expect(trust(opened)).toBe("signed");
    const rejected = linkCase([review("r1", linkItem.item, "same"), contest("c1", "r1"), resolve("x1", "c1", "rejected")]);
    expect(trust(rejected)).toBe("signed");
    expect(open(rejected)).toEqual([]);
  });

  it("compensating the latest decision reveals the earlier effective one", () => {
    const c = linkCase([review("r1", linkItem.item, "same"), review("r2", linkItem.item, "different"), undo("u1", "r2")]);
    expect(trust(c)).toBe("signed");
    expect(open(c)).toEqual([]);
  });

  it("compensating the only effective evidence review reopens that item", () => {
    const answered = withLedger([evidenceItem], [review("r1", evidenceItem.item, "agree")]);
    expect(open(answered)).toEqual([]);
    const undone = withLedger([evidenceItem], [review("r1", evidenceItem.item, "agree"), undo("u1", "r1")]);
    expect(open(undone)).toEqual([evidenceItem.item]);
    expect(caseStage(undone)).toMatchObject({ key: "waiting_for_reviewers", technical: "review · evidence" });
    const upheld = withLedger([evidenceItem], [review("r1", evidenceItem.item, "agree"), contest("c1", "r1"), resolve("x1", "c1", "upheld")]);
    expect(open(upheld)).toEqual([evidenceItem.item]);
  });

  it("does not mutate the ledger it reads", () => {
    const events = [review("r1", linkItem.item, "same"), undo("u1", "r1")];
    const snapshot = JSON.stringify(events);
    const c = linkCase(events);
    trust(c);
    open(c);
    expect(JSON.stringify(c.api_reviews)).toBe(snapshot);
  });
});
