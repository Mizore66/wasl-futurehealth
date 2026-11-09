/*
 * Plain words for the backend's identifiers (issue #5).
 * Screens show the plain label; the raw identifier stays available behind a disclosure (components/wasl/tech-term.tsx).
 */

export const TERMS = {
  // Decision-chain entry types (pipeline/governance.go).
  link_decision: "Report matched to the referral",
  evidence: "Readers' result",
  hold_decision: "Held by the safety gate",
  human_signoff: "Signed by a person",
  closure_decision: "Closed with evidence",
  // Gate outcomes.
  needs_human: "Waiting for two reviewers",
  not_closed: "Kept open",
  // The demo fixtures.
  evidence_closure: "Closed with evidence",
  false_closure_trap: "Held: incomplete evidence",
  review_hold: "Held for human review",
  dr_exam_closure: "Eye exam closed the loop",
  // LDCT reader fields (docs/API.md).
  lungrads_overall: "Lung-RADS overall",
  lungrads_worst: "Lung-RADS worst",
} as const satisfies Record<string, string>;

export type Term = keyof typeof TERMS;

export function isTerm(id: string): id is Term {
  return Object.hasOwn(TERMS, id);
}

/** The plain label for a backend identifier; an unknown identifier reads as words ("awaiting_tissue" → "Awaiting tissue"). */
export function plain(id: string): string {
  if (isTerm(id)) return TERMS[id];
  const words = id.replaceAll("_", " ").trim();
  return words ? words[0].toUpperCase() + words.slice(1) : id;
}
