import type { CaseDetail, ReviewState } from "@/lib/api/schema";

/*
 * The 8-step pipeline vocabulary (pipeline-brief "The state vocabulary").
 * Plain words lead; `technical` is the backend term shown small and muted.
 */

export const PIPELINE_STEPS = [
  { step: 1, label: "Received" },
  { step: 2, label: "Matching identity" },
  { step: 3, label: "Matched" },
  { step: 4, label: "Reading the report" },
  { step: 5, label: "Readers compared" },
  { step: 6, label: "Safety gate" },
  { step: 7, label: "Outcome" },
  { step: 8, label: "Recorded in the ledger" },
] as const;

export type Step = (typeof PIPELINE_STEPS)[number]["step"];

export const STAGES = {
  received: { step: 1, label: "Received" },
  matching: { step: 2, label: "Matching identity" },
  matched: { step: 3, label: "Matched" },
  needs_second_identifier: { step: 3, label: "Needs a second identifier" },
  reading: { step: 4, label: "Reading the report: two readers" },
  readers_agree: { step: 5, label: "Readers agree" },
  readers_disagree: { step: 5, label: "Readers disagree" },
  gate_passed: { step: 6, label: "Safety gate: passed" },
  gate_held: { step: 6, label: "Safety gate: held" },
  closed_with_evidence: { step: 7, label: "Closed with evidence" },
  waiting_for_reviewers: { step: 7, label: "Waiting for two reviewers" },
  message_held_for_consent: { step: 7, label: "Message held until consent" },
  recorded: { step: 8, label: "Recorded in the ledger" },
  retained: { step: 8, label: "Retired: history only" },
} as const satisfies Record<string, { step: Step; label: string }>;

export type StageKey = keyof typeof STAGES;

export type CaseStage = {
  key: StageKey;
  step: Step;
  label: string;
  /** Backend term, e.g. `needs_human · missing_documentation`. */
  technical: string;
};

function stage(key: StageKey, technical: string): CaseStage {
  return { key, ...STAGES[key], technical };
}

/*
 * Independent double-review overlay. The API projects one state machine per (item, question) in `api_review_states`;
 * only a terminal state (agreed, or resolved by a senior) is a final answer. A first or second read, even the viewer's
 * own, is never a sign-off, so `caseStage` and `linkTrust` ignore it. Canonical case data is never rewritten.
 */
export type ReviewSlot = "r1" | "r2" | "r3";
export const READ_LABEL: Record<ReviewSlot, string> = { r1: "First read", r2: "Second read", r3: "Senior resolution" };
const TERMINAL: ReadonlySet<string> = new Set(["resolved_agreed", "resolved_senior"]);

type WithStates = Pick<CaseDetail, "api_review_states">;

/** The independent-review slot a ledger event carries, or undefined for a legacy review event or any other ledger entry. */
export function protocolRead(event: object): ReviewSlot | undefined {
  const e = event as { ledger?: unknown; protocol?: unknown; read?: unknown };
  if (e.ledger !== "review" || typeof e.protocol !== "string") return undefined;
  return e.read === "r1" || e.read === "r2" || e.read === "r3" ? e.read : undefined;
}

export const isTerminalState = (s: Pick<ReviewState, "state">) => TERMINAL.has(s.state);
export const reviewStates = (c: WithStates): ReviewState[] => c.api_review_states ?? [];

/** Items whose every raised question has reached a terminal state. */
export function resolvedItems(c: WithStates): Set<string> {
  const byItem = new Map<string, boolean>();
  for (const s of reviewStates(c)) byItem.set(s.item, (byItem.get(s.item) ?? true) && isTerminalState(s));
  return new Set([...byItem].filter(([, done]) => done).map(([item]) => item));
}

/** True once the server reports states and every one is terminal. No states means the legacy workflow, never "done". */
export function allQuestionsResolved(c: WithStates): boolean {
  const states = reviewStates(c);
  return states.length > 0 && states.every(isTerminalState);
}

/** Final decisions of terminal states for one question. */
export function finalDecisions(c: WithStates, question: ReviewState["question"]): string[] {
  return reviewStates(c).flatMap((s) => (s.question === question && isTerminalState(s) && s.final_decision ? [s.final_decision] : []));
}

/** True when any first read is hidden from this viewer (or from anyone else): the auditor history would leak it. */
export function concealsInitialRead(states: readonly ReviewState[]): boolean {
  return states.some((s) => s.state === "awaiting_r2" || s.reads.some((r) => r.submitted && r.read !== "r3" && (r.decision === undefined || (r as { concealed?: boolean }).concealed === true)));
}

/** "1 of 2 questions resolved", or undefined for the legacy workflow. */
export function resolutionProgress(c: WithStates): string | undefined {
  const states = reviewStates(c);
  if (states.length === 0) return undefined;
  const done = states.filter(isTerminalState).length;
  return `${done} of ${states.length} ${states.length === 1 ? "question" : "questions"} resolved`;
}

/** Review items with no final answer: signed in the run, answered by a legacy event, or resolved by the protocol. */
export function openReviewItems(c: CaseDetail) {
  // Only review events answer a review item; coordinator and undo/contest events share the ledger but carry no item.
  // Protocol reads answer nothing until the server reports a terminal state for every question of the item.
  const signed = new Set([
    ...c.signoffs.map((s) => s.item),
    ...c.api_reviews.flatMap((r) => (r.ledger === "review" && !protocolRead(r) ? [r.item] : [])),
    ...resolvedItems(c),
  ]);
  return c.review_items.filter((r) => !signed.has(r.item));
}

/** Where a case rests now. Steps 2 and 4 are transient and appear only in the replay. */
export function caseStage(c: CaseDetail): CaseStage {
  if (c.case.lifecycle === "retained") return stage("retained", "inactive");
  if (c.case.closed) {
    const via = c.signoffs.length > 0 ? "closed · human_signoff" : "closed · auto";
    return stage("recorded", via);
  }

  const hold = c.holds[0];
  if (hold) return stage("gate_held", `${hold.state} · ${hold.rule}`);

  const open = openReviewItems(c);
  if (open.some((r) => r.kinds.includes("evidence"))) return stage("waiting_for_reviewers", "review · evidence");
  if (open.some((r) => r.kinds.includes("link"))) {
    return c.evidence.length > 0
      ? stage("waiting_for_reviewers", "review · link")
      : stage("needs_second_identifier", open.find((r) => r.kinds.includes("link"))?.reason ?? "review · link");
  }

  const lastConsent = c.decisions.at(-1)?.consent_log.at(-1);
  if (lastConsent?.to === "held") return stage("message_held_for_consent", `consent · ${lastConsent.contact ?? "unknown"}`);

  const ev = c.evidence.at(-1);
  if (ev) return stage(ev.agree ? "readers_agree" : "readers_disagree", ev.gemma ?? ev.label);
  if (c.links.length > 0) return stage("matched", `p_cal ${c.links[0].path_p_cal.toFixed(3)}`);
  return stage("received", c.case.state);
}
