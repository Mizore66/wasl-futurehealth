import { describe, expect, it } from "vitest";
import { CaseDetail, type ReviewState } from "@/lib/api/schema";
import { openReviewItems } from "@/lib/domain/stage";
import { linkTrust, recordFace } from "@/lib/people/identity";
import hold from "@/data/api/case_review_hold.json";

const base = CaseDetail.parse(hold);
const loop = base.case.id;
const R = "ahs-mushrif|AHS-MRN001006";
const A = "skmc-endo|H1";
const B = "hospital-b|H1"; // Equal local MRNs are different provider-qualified registrations.
type Pair = [string, string];
type Link = CaseDetail["links"][number];
type Item = CaseDetail["review_items"][number];
type Answer = "same" | "different";
type RawEvent = Record<string, unknown>;

const path = (registration: string, event = "DiagnosticReport/report-b"): Link => ({
  ...base.links[0], loop, referral_patient: R, downstream_event: `${registration.split("|")[0]}/${event}`,
  hops: 1, weakest_edge: { a: R, b: registration, basis: "eid", p: 1 },
});
const a = path(A, "DiagnosticReport/report-a");
const b = path(B);

// Mirror both real producers: outreach_guard items for individual registration pairs (closure.go),
// and a link question raised on a report's weakest pair. No one-link-item-per-case assumption.
const linkItem = (item: string, pair: Pair, source: Link, guard = false): Item => ({
  item, loop, kinds: ["link"], pair, p: source.path_p_cal, report: "", verdict: "", quote: "",
  ...(guard ? { reason: "second_identifier" } : {}),
  chain: [{ type: guard ? "outreach_guard" : "link_decision", prev: "genesis", hash: `chain-${item}`,
    body: guard
      ? { loop, pair, event: source.downstream_event, basis: "eid", p_cal: 1, rule: "second identifier or human confirmation" }
      : { ...source },
  }],
});
const itemA = linkItem("item-a", [R, A], path(A, "Appointment/appointment-a"), true);
const itemB = linkItem("item-b", [R, B], path(B, "Appointment/appointment-b"), true);

// These are API projection inputs, not a signed-ledger verifier fixture. Preserve append order,
// target/action/actor relationships and schema tags through the actual CaseDetail parser.
function ledger() {
  const events: RawEvent[] = [];
  const append = (fields: RawEvent) => {
    const hash = `event-${events.length + 1}`;
    events.push({ case: loop, role: "reviewer", actor: "reviewer-one", at: "2026-11-13T10:00:00Z",
      mode: "demo", note: "synthetic review", prev: events.at(-1)?.hash ?? "genesis", hash, ...fields });
    return hash;
  };
  const review = (item: string, decision: string) => append({ item, decision, head_signed: "canonical-head",
    action: decision === "agree" ? "signoff_evidence" : "signoff_link", role: decision === "agree" ? "clinician" : "reviewer" });
  const compensation = (target: string, kind: "undo" | "contest" | "contest_resolution", resolution?: "upheld" | "rejected") => {
    const original = events.find((e) => e.hash === target)!;
    return append({ kind, target, target_action: original.action, action: original.action, role: original.role,
      target_actor: kind === "contest_resolution" ? original.target_actor : original.actor,
      actor: kind === "contest_resolution" ? "reviewer-three" : "reviewer-two",
      head_signed: "canonical-head", action_head: events.at(-1)!.hash, ...(resolution ? { resolution } : {}) });
  };
  const compensate = (target: string, method: "undo" | "upheld") => {
    if (method === "undo") compensation(target, "undo");
    else compensation(compensation(target, "contest"), "contest_resolution", "upheld");
  };
  return { events, append, review, compensate, compensation };
}

function example(extra: Partial<CaseDetail> = {}, events: RawEvent[] = []): CaseDetail {
  return CaseDetail.parse({ ...base, case: { ...base.case, closed: false, closed_day: 0, state: "open", review: "open" },
    current: { ...base.current, state: "open", chain: [] }, decisions: [], timeline: [], outreach: [], links: [a, b],
    identities: [{ provider: "ahs-mushrif", registration: R, role: "referral" },
      { provider: "skmc-endo", registration: A, role: "downstream" },
      { provider: "hospital-b", registration: B, role: "downstream" }],
    review_items: [itemA, itemB], evidence: [], holds: [], signoffs: [], api_review_states: [],
    ...extra, api_reviews: events,
  });
}
const canonical = (item: Item, answer: Answer) => ({ ...base.signoffs[0], loop, item: item.item, kinds: ["link"], link: answer });
const terminal = (item: Item, answer: Answer): ReviewState => ({
  item: item.item, question: "link", state: "resolved_agreed", final_decision: answer,
  head: "protocol-head", next_read: null, may_submit: false, disagreement: false, canonical_unchanged: true,
  reads: [{ read: "r1", submitted: true, decision: answer, hash: "r1-hash" }, { read: "r2", submitted: true, decision: answer, hash: "r2-hash" }],
});

// Removing pair scoping would make an independent same sign B, or an independent different reject it.
// None of these additions has been executed locally: runtime execution is reserved for remote CI.
describe("registration-scoped identity answers after compensation", () => {
  for (const method of ["undo", "upheld"] as const) {
    for (const independent of ["same", "different"] as const) {
      for (const compensated of ["same", "different"] as const) {
        it(`${method}: ${independent} for A cannot answer B after B's ${compensated} is removed`, () => {
          const l = ledger();
          l.review(itemA.item, independent);
          const target = l.review(itemB.item, compensated);
          l.compensate(target, method);
          const c = example({}, l.events);
          expect(linkTrust(c, c.links[0])).toBe(independent === "same" ? "signed" : "rejected");
          expect(linkTrust(c, c.links[1])).toBe("held");
          expect(openReviewItems(c).map((i) => i.item)).toEqual(["item-b"]);
          expect(recordFace(c, B).state).toBe("unknown");
          expect(recordFace(c, A)).toMatchObject(independent === "same"
            ? { state: "known", person: { key: R } } : { state: "known", person: { key: A } });
        });
      }
    }
  }

  it("a historical outreach item A is not an answer to the only current link B", () => {
    const l = ledger();
    l.review(itemA.item, "same");
    const target = l.review(itemB.item, "different");
    l.compensate(target, "undo");
    const c = example({ links: [b] }, l.events);
    expect(linkTrust(c, c.links[0])).toBe("held");
    expect(recordFace(c, B).state).toBe("unknown");
    expect(recordFace(c, A).state).toBe("unknown");
  });

  for (const answer of ["same", "different"] as const) {
    for (const resolution of ["open", "rejected"] as const) {
      it(`${resolution} contest retains B's own ${answer}, not A's opposite answer`, () => {
        const l = ledger();
        l.review(itemA.item, answer === "same" ? "different" : "same");
        const target = l.review(itemB.item, answer);
        const contest = l.compensation(target, "contest");
        if (resolution === "rejected") l.compensation(contest, "contest_resolution", resolution);
        const c = example({}, l.events);
        expect(linkTrust(c, c.links[1])).toBe(answer === "same" ? "signed" : "rejected");
        expect(openReviewItems(c)).toEqual([]);
      });
    }
    for (const method of ["undo", "upheld"] as const) {
      it(`${method} preserves an earlier effective ${answer} within B only`, () => {
        const l = ledger();
        l.review(itemA.item, answer === "same" ? "different" : "same");
        l.review(itemB.item, answer);
        const target = l.review(itemB.item, answer === "same" ? "different" : "same");
        l.compensate(target, method);
        const c = example({}, l.events);
        expect(linkTrust(c, c.links[1])).toBe(answer === "same" ? "signed" : "rejected");
      });
    }
  }

  for (const reverseEdge of [false, true]) {
    for (const reverseItem of [false, true]) {
      it(`matches the same full pair with reversed edge=${reverseEdge}, item=${reverseItem}`, () => {
        const reversed = { ...b, weakest_edge: reverseEdge ? { ...b.weakest_edge, a: B, b: R } : b.weakest_edge };
        const pair: Pair = reverseItem ? [B, R] : [R, B];
        const l = ledger();
        l.review(itemA.item, "different");
        l.review(itemB.item, "same");
        const raised = { ...reversed, path_p_cal: 0.98, path_strength: 0.98,
          weakest_edge: { ...reversed.weakest_edge, p: 0.98, basis: "wasl" } };
        const c = example({ links: [reversed], review_items: [itemA, linkItem(itemB.item, pair, raised)] }, l.events);
        expect(linkTrust(c, c.links[0])).toBe("signed");
        expect(recordFace(c, B)).toMatchObject({ state: "known", person: { key: R } });
        expect(recordFace(c, A).state).toBe("unknown");
      });
    }
  }

  it("keeps provider-qualified equal local MRNs separate in both answer directions", () => {
    const l = ledger();
    l.review(itemA.item, "same");
    l.review(itemB.item, "different");
    const c = example({}, l.events);
    expect(linkTrust(c, c.links[0])).toBe("signed");
    expect(linkTrust(c, c.links[1])).toBe("rejected");
    expect(recordFace(c, B)).toMatchObject({ state: "known", person: { key: B } });
  });

  it("joins opaque API registration pseudonyms without guessing a provider from their text", () => {
    const aliases = ["reg_" + "1".repeat(64), "reg_" + "2".repeat(64), "reg_" + "3".repeat(64)];
    const renamedA = { ...a, referral_patient: aliases[0], weakest_edge: { ...a.weakest_edge, a: aliases[0], b: aliases[1] } };
    const renamedB = { ...b, referral_patient: aliases[0], weakest_edge: { ...b.weakest_edge, a: aliases[0], b: aliases[2] } };
    const l = ledger();
    l.review(itemA.item, "same");
    const c = example({ links: [renamedA, renamedB], review_items: [
      linkItem(itemA.item, [aliases[0], aliases[1]], renamedA, true),
      linkItem(itemB.item, [aliases[0], aliases[2]], renamedB, true),
    ], identities: [
      { provider: "ahs-mushrif", registration: aliases[0], role: "referral" },
      { provider: "skmc-endo", registration: aliases[1], role: "downstream" },
      { provider: "hospital-b", registration: aliases[2], role: "downstream" },
    ] }, l.events);
    expect(linkTrust(c, c.links[0])).toBe("signed");
    expect(linkTrust(c, c.links[1])).toBe("held");
    expect(recordFace(c, aliases[2]).state).toBe("unknown");
  });
});

describe("canonical and protocol precedence stays inside the validated pair", () => {
  for (const channel of ["canonical", "protocol"] as const) {
    for (const answer of ["same", "different"] as const) {
      it(`${channel} ${answer} for A cannot answer B`, () => {
        const extra = channel === "canonical" ? { signoffs: [canonical(itemA, answer)] }
          : { api_review_states: [terminal(itemA, answer)] };
        const c = example(extra);
        expect(linkTrust(c, c.links[0])).toBe(answer === "same" ? "signed" : "rejected");
        expect(linkTrust(c, c.links[1])).toBe("held");
        expect(recordFace(c, B).state).toBe("unknown");
      });

      it(`compensating legacy B does not remove its ${channel} ${answer}`, () => {
        const l = ledger();
        l.review(itemA.item, answer === "same" ? "different" : "same");
        const target = l.review(itemB.item, answer === "same" ? "different" : "same");
        l.compensate(target, "undo");
        const extra = channel === "canonical" ? { signoffs: [canonical(itemB, answer)] }
          : { api_review_states: [terminal(itemB, answer)] };
        const c = example(extra, l.events);
        expect(linkTrust(c, c.links[1])).toBe(answer === "same" ? "signed" : "rejected");
      });
    }
  }

  it("keeps different over same across channels within B, with no effect on A", () => {
    const l = ledger();
    l.review(itemA.item, "same");
    l.review(itemB.item, "same");
    const c = example({ signoffs: [canonical(itemB, "same")], api_review_states: [terminal(itemB, "different")] }, l.events);
    expect(linkTrust(c, c.links[0])).toBe("signed");
    expect(linkTrust(c, c.links[1])).toBe("rejected");
  });

  it("uses a canonical signoff's explicit pair when the item is absent, never its loop alone", () => {
    const c = example({ review_items: [], signoffs: [{ ...canonical(itemA, "different"), pair: [A, R] }] });
    expect(linkTrust(c, c.links[0])).toBe("rejected");
    expect(linkTrust(c, c.links[1])).toBe("auto");
  });

  it("does not promote nonterminal independent reads to answers", () => {
    const l = ledger();
    l.review(itemA.item, "same");
    l.append({ item: itemB.item, decision: "same", action: "signoff_link", head_signed: "canonical-head",
      protocol: "wasl-independent-review/1", nonce: "synthetic-read", read: "r1", question: "link", expected_head: "", claim_head: "claim-hash" });
    const c = example({ api_review_states: [{ ...terminal(itemB, "same"), state: "awaiting_r2", final_decision: undefined,
      reads: [{ read: "r1", submitted: true, decision: "same" }, { read: "r2", submitted: false }] }] }, l.events);
    expect(linkTrust(c, c.links[1])).toBe("held");
  });
});

describe("missing or ambiguous registration/path metadata cannot lend a face", () => {
  for (const pair of [[], [R], [R, B, A], [R, R], [R, ""]]) {
    it(`holds when a same-loop link item's pair is incomplete or ambiguous: ${JSON.stringify(pair)}`, () => {
      const l = ledger();
      l.review(itemA.item, "same");
      l.review(itemB.item, "same");
      const c = example({ review_items: [itemA, { ...itemB, pair }] }, l.events);
      expect(linkTrust(c, c.links[1])).toBe("held");
      expect(recordFace(c, B).state).toBe("unknown");
    });
  }

  it("does not resolve one item ID to two different registration pairs", () => {
    const l = ledger();
    l.review(itemB.item, "same");
    const c = example({ review_items: [{ ...itemA, item: itemB.item }, itemB] }, l.events);
    expect(linkTrust(c, c.links[1])).toBe("held");
    expect(recordFace(c, B).state).toBe("unknown");
  });

  it("holds a canonical signoff whose explicit pair contradicts its raised item", () => {
    const c = example({ signoffs: [{ ...canonical(itemB, "same"), pair: [R, A] }] });
    expect(linkTrust(c, c.links[1])).toBe("held");
  });

  it("holds an orphan canonical link answer without either pair or item metadata", () => {
    const c = example({ review_items: [], signoffs: [canonical(itemB, "same")] });
    expect(linkTrust(c, c.links[1])).toBe("held");
    expect(recordFace(c, B).state).toBe("unknown");
  });

  it("holds orphan legacy and terminal answers when their item-to-pair metadata is missing", () => {
    const l = ledger();
    l.review(itemB.item, "same");
    const legacy = example({ review_items: [] }, l.events);
    const protocol = example({ review_items: [], api_review_states: [terminal(itemB, "same")] });
    expect(linkTrust(legacy, legacy.links[1])).toBe("held");
    expect(linkTrust(protocol, protocol.links[1])).toBe("held");
    expect(recordFace(legacy, B).state).toBe("unknown");
    expect(recordFace(protocol, B).state).toBe("unknown");
  });

  for (const weakest_edge of [{ ...b.weakest_edge, b: "" }, { ...b.weakest_edge, a: A }, { ...b.weakest_edge, b: R }]) {
    it(`holds an unidentifiable direct link ${JSON.stringify(weakest_edge)}`, () => {
      const c = example({ links: [{ ...b, weakest_edge }] });
      expect(linkTrust(c, c.links[0])).toBe("held");
      expect(recordFace(c, B).state).toBe("unknown");
    });
  }

  for (const weakest_edge of [{ a: A, b: B, p: 1, basis: "eid" }, { a: R, b: A, p: 1, basis: "eid" }]) {
    it(`does not equate a multi-hop weakest edge with referral/report endpoints: ${JSON.stringify(weakest_edge)}`, () => {
      const multi = { ...b, hops: 2, path_p_cal: 0.98, path_strength: 0.98,
        weakest_edge: { ...weakest_edge, p: 0.98, basis: "wasl" } };
      const edgeItem = linkItem("item-edge", [weakest_edge.a, weakest_edge.b], multi);
      const l = ledger();
      l.review(edgeItem.item, "same");
      const c = example({ links: [multi], review_items: [edgeItem] }, l.events);
      expect(linkTrust(c, c.links[0])).toBe("held");
      expect(recordFace(c, B).state).toBe("unknown");
      expect(recordFace(c, A).state).toBe("unknown");
    });
  }

  it("does not treat a substituted review pair as a graph path endpoint", () => {
    const l = ledger();
    l.review(itemB.item, "same");
    const c = example({ links: [{ ...b, review_pair_substituted: true, eid_contra: "sex" }] }, l.events);
    expect(linkTrust(c, c.links[0])).toBe("held");
    expect(recordFace(c, B).state).toBe("unknown");
  });

  it("holds conflicting registration associations for one downstream event", () => {
    const l = ledger();
    l.review(itemA.item, "same");
    const c = example({ links: [a, { ...b, downstream_event: a.downstream_event }] }, l.events);
    expect(linkTrust(c, c.links[0])).toBe("held");
    expect(recordFace(c, A).state).toBe("unknown");
  });

  it("never falls back to the first trusted link for an unconfirmed supplied downstream identity", () => {
    const l = ledger();
    l.review(itemA.item, "same");
    const c = example({ links: [a] }, l.events);
    expect(recordFace(c, B).state).toBe("unknown");
    expect(recordFace(c, "unlisted-provider|H1").state).toBe("unknown");
    expect(recordFace(c, R)).toMatchObject({ state: "known", person: { key: R } });
  });
});

describe("independent evidence items and immutable inputs", () => {
  it("compensates evidence item one independently of item two and identity answers", () => {
    const evidenceItem = (item: string, registration: string, report: string): Item => {
      const source = path(registration, `DiagnosticReport/${report}`);
      const raised = linkItem(item, [R, registration], source);
      return { ...raised, kinds: ["evidence"], report, verdict: "disagree", quote: "synthetic report quote",
        chain: [...raised.chain, { type: "evidence", prev: raised.chain[0].hash, hash: `evidence-chain-${item}`,
          body: { ...base.evidence[0], report: source.downstream_event } }],
      };
    };
    const e1 = evidenceItem("evidence-one", A, "report-one");
    const e2 = evidenceItem("evidence-two", A, "report-two");
    for (const method of ["undo", "upheld"] as const) {
      const l = ledger();
      l.review(itemA.item, "same");
      const target = l.review(e1.item, "agree");
      l.review(e2.item, "agree");
      l.compensate(target, method);
      const c = example({ review_items: [itemA, itemB, e1, e2] }, l.events);
      const snapshot = JSON.stringify(c);
      expect(openReviewItems(c).map((i) => i.item)).toEqual(["item-b", "evidence-one"]);
      expect(linkTrust(c, c.links[0])).toBe("signed");
      expect(linkTrust(c, c.links[1])).toBe("held");
      recordFace(c, B);
      expect(JSON.stringify(c)).toBe(snapshot);
    }
  });

  it("evidence-only canonical signoffs cannot sign a link, and bare valid pairs keep automatic scoring", () => {
    const c = example({ review_items: [], signoffs: [{ ...canonical(itemB, "same"), kinds: ["evidence"], evidence: "adequate" }] });
    expect(linkTrust(c, c.links[1])).toBe("auto");
    expect(linkTrust(c, { ...c.links[1], path_p_cal: 0.98 })).toBe("below");
  });
});
