import type { CaseDetail } from "@/lib/api/schema";

/*
 * The 8-step pipeline vocabulary (pipeline-brief "The state vocabulary").
 * Plain words lead; `technical` is the backend term shown small and grey.
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

function openReviewItems(c: CaseDetail) {
  const signed = new Set([...c.signoffs.map((s) => s.item), ...c.api_reviews.map((r) => r.item)]);
  return c.review_items.filter((r) => !signed.has(r.item));
}

/** Where a case rests now. Steps 2 and 4 are transient and appear only in the replay. */
export function caseStage(c: CaseDetail): CaseStage {
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
