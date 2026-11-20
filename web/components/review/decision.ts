import type { Role, ReviewRead, ReviewState } from "@/lib/api/schema";
import { READ_LABEL, type ReviewSlot } from "@/lib/domain/stage";
import { staffDueTime } from "@/lib/domain/staff-time";

/*
 * Review decisions in the order the reviewer answers them: identity first (reviewer role) when a link item exists,
 * then the raised evidence item (clinician role). Each item can also appear alone; non-CRC labels do not infer closure.
 */

export type IdentityAnswer = "same" | "different";
/** A programme evidence label, or CRC's local-only ask action. Zod validates labels again in the server action. */
export type ReportAnswer = string;
export type Draft = { identity?: IdentityAnswer; report?: ReportAnswer };

export type OutcomeKey = "close" | "inadequate" | "evidence" | "linkSame" | "ask" | "notSame" | "scoped" | "scopedNo";

/** The review items this case raised, by the question they answer. */
export type ReviewItems = { link?: string; evidence?: string; /** The evidence item asks one narrow question, so "adequate" does not mean the case closes. */ scoped?: boolean };

/** One appended review event: what the API receives. */
export type Signature = { item: string; decision: string };

export const SIGNER = { name: "Dr Omar Al Kaabi", title: "Medical director", initials: "OK" } as const;

export function roleFor(decision: Signature["decision"]): Role {
  return decision === "same" || decision === "different" ? "reviewer" : "clinician";
}

/** No outcome until every question raised by this case is answered. Link and evidence items can each appear alone. */
export function outcomeOf(draft: Draft, items: ReviewItems = { link: "link", evidence: "evidence" }): OutcomeKey | undefined {
  if (items.link && draft.identity === "different") return "notSame";
  const linkAnswered = !items.link || draft.identity === "same";
  if (items.evidence && draft.report && draft.report !== "ask" && linkAnswered) {
    if (draft.report === "adequate") return items.scoped ? "scoped" : "close";
    if (draft.report === "inadequate") return items.scoped ? "scopedNo" : "inadequate";
    return "evidence";
  }
  if (items.link && draft.identity === "same") {
    if (!items.evidence) return "linkSame";
    if (draft.report === "ask") return "ask";
  }
  return undefined;
}

/** The two pilot purposes of an evidence review item, from `review_items[].reason`. */
export type ReviewPurpose = "shadow_confirm" | "auto_audit";

export function purposeOf(reason: string | undefined): ReviewPurpose | undefined {
  return reason === "shadow_confirm" || reason === "auto_audit" ? reason : undefined;
}

/** Each present item signs independently; a case with both keeps identity first and suppresses evidence after "different" or "ask". */
export function signaturesFor(draft: Draft, items: ReviewItems): Signature[] {
  const outcome = outcomeOf(draft, items);
  if (!outcome) return [];
  const signatures: Signature[] = [];
  if (items.link && draft.identity) signatures.push({ item: items.link, decision: draft.identity });
  if (outcome === "notSame" || outcome === "ask" || outcome === "linkSame") return signatures;
  if (items.evidence && draft.report && draft.report !== "ask") signatures.push({ item: items.evidence, decision: draft.report });
  return signatures;
}

export type Outcome = {
  commit: string;
  pill: string;
  tone: "positive" | "negative" | "warning" | "muted";
  /** Hypothetical downstream language for fixture previews only. */
  text: (name: string, unit: string, hasLink?: boolean) => string;
  next: (unit: string, hasLink?: boolean) => { who: string; what: string };
  /** What the same answer reads as in the demo, where nothing is appended. */
  preview: string;
  /** What a 201 proves, without claiming any downstream effect. */
  recordedText: string;
  /** The disputed rows after a live save: one recorded evidence answer covers all of them. */
  recordedAnswerCell: { word: string; tone: "positive" | "negative" | "warning" | "muted" };
  /** The disputed rows in a fixture preview: one evidence answer covers all of them. */
  answerCell: { word: string; tone: "positive" | "negative" | "warning" | "muted" };
};

/** What each signature says, as it reads in the trail. */
export const SIGNS: Record<string, string> = {
  same: "same patient",
  different: "not the same patient",
  adequate: "report adequate",
  inadequate: "report not adequate",
};

/** A narrow-purpose evidence answer settles its own question only, so it is never worded as a report verdict. */
export const signWords = (decision: string, scoped = false) =>
  scoped && (decision === "adequate" || decision === "inadequate") ? `${decision === "adequate" ? "adequate" : "not adequate"} for this question only` : (SIGNS[decision] ?? `evidence ${decision}`);

export const OUTCOMES: Record<OutcomeKey, Outcome> = {
  close: {
    commit: "Close the case",
    pill: "Demo answers saved; case open",
    preview: "Would close the case",
    tone: "positive",
    text: (name, _unit, hasLink = true) =>
      hasLink
        ? `Same patient, report adequate. ${name}’s case closes with both answers and the report quote in her record.`
        : "Report adequate would be saved. No identity answer is assumed.",
    next: (_unit, hasLink = true) => (hasLink ? { who: "Closed", what: "with both answers recorded" } : { who: "The case", what: "no further change shown" }),
    recordedText: "Same patient and report adequate were saved in the demo. The case still shows as open; closure is not confirmed.",
    recordedAnswerCell: { word: "adequate, signed", tone: "positive" },
    answerCell: { word: "adequate, signed", tone: "positive" },
  },
  scoped: {
    commit: "Sign: adequate",
    pill: "Demo answer saved",
    preview: "Would record this answer only",
    tone: "muted",
    text: () => "Adequate would be saved for this question only. Histology and other requirements are still checked, so nothing is promised about the case.",
    next: () => ({ who: "The case", what: "no further change shown" }),
    recordedText: "Adequate was saved in the demo for this question only. Histology and other requirements are still checked.",
    recordedAnswerCell: { word: "adequate, signed", tone: "positive" },
    answerCell: { word: "adequate, signed", tone: "positive" },
  },
  scopedNo: {
    commit: "Sign: not adequate",
    pill: "Demo answer saved",
    preview: "Would record this answer only",
    tone: "muted",
    text: () => "Not adequate would be saved for this question only. Histology and other requirements are still checked, so no repeat booking or other effect is promised.",
    next: () => ({ who: "The case", what: "no further change shown" }),
    recordedText: "Not adequate was saved in the demo for this question only. Histology and other requirements are still checked.",
    recordedAnswerCell: { word: "not adequate, signed", tone: "negative" },
    answerCell: { word: "not adequate, signed", tone: "negative" },
  },
  inadequate: {
    commit: "Sign: report not adequate",
    pill: "Demo answers saved",
    preview: "Would not close: repeat colonoscopy",
    tone: "negative",
    text: (name, _unit, hasLink = true) =>
      hasLink
        ? `Same patient, but the colonoscopy report was not adequate, so this colonoscopy does not count. ${name} goes back to Today for a repeat booking.`
        : "Report not adequate would be saved. No identity answer or repeat booking is assumed.",
    next: () => ({ who: "Her coordinator", what: "to book a repeat colonoscopy" }),
    recordedText: "Same patient and report not adequate were saved in the demo. No repeat booking is confirmed.",
    recordedAnswerCell: { word: "not adequate, signed", tone: "negative" },
    answerCell: { word: "not adequate, signed", tone: "negative" },
  },
  evidence: {
    commit: "Sign evidence answer",
    pill: "Demo evidence answer saved",
    preview: "Preview evidence answer",
    tone: "muted",
    text: () => "The chosen label would be saved. No closure or other change to the case is assumed.",
    next: () => ({ who: "The case", what: "no further change shown" }),
    recordedText: "The evidence answer was saved in the demo. No change to the case is confirmed.",
    recordedAnswerCell: { word: "signed", tone: "muted" },
    answerCell: { word: "signed", tone: "muted" },
  },
  linkSame: {
    commit: "Sign: same patient",
    pill: "Demo identity answer saved",
    preview: "Would confirm the identity link",
    tone: "positive",
    text: () => "Same patient would be saved. No evidence answer is assumed.",
    next: () => ({ who: "The case", what: "no further change shown" }),
    recordedText: "Same patient was saved in the demo. No change to the case is confirmed.",
    recordedAnswerCell: { word: "not answered", tone: "muted" },
    answerCell: { word: "not answered", tone: "muted" },
  },
  ask: {
    commit: "Sign and ask the unit to complete the report",
    pill: "Demo identity answer saved",
    preview: "Would ask the unit",
    tone: "warning",
    text: (_name, unit) => `Same patient is signed. The case stays held until ${unit} completes the report.`,
    next: (unit) => ({ who: unit, what: "to complete the colonoscopy report" }),
    recordedText: "Same patient was saved in the demo. No report answer was saved. The unit was not told.",
    recordedAnswerCell: { word: "not answered", tone: "muted" },
    answerCell: { word: "asked the unit", tone: "warning" },
  },
  notSame: {
    commit: "Sign: not the same patient",
    pill: "Demo identity answer saved",
    preview: "Would reject the link",
    tone: "negative",
    text: (name, unit) => `The report is not ${name}’s. She stays open, and ${unit} is told the report matches no patient on our list.`,
    next: () => ({ who: "Link undone", what: "report returned unmatched" }),
    recordedText: "Not the same patient was saved in the demo. No change to the records or the unit is confirmed.",
    recordedAnswerCell: { word: "not answered", tone: "muted" },
    answerCell: { word: "not applied", tone: "muted" },
  },
};

/*
 * A pilot review answers a different question from an ordinary evidence review, so its adequate / not adequate answers
 * read as confirming or disagreeing with the automatic closure. Keyed by the generic outcome they replace.
 */
const PURPOSE_COPY: Record<ReviewPurpose, Partial<Record<OutcomeKey, Pick<Outcome, "commit" | "preview" | "text">>>> = {
  shadow_confirm: {
    close: {
      commit: "Confirm the suggested closure",
      preview: "Would confirm the suggested closure",
      text: () => "Confirming the automatic suggestion permits closure of the case.",
    },
    inadequate: {
      commit: "Sign: do not confirm closure",
      preview: "Would not confirm the suggested closure",
      text: () => "The suggestion is not confirmed, so the case follows your answer instead of closing automatically.",
    },
  },
  auto_audit: {
    close: {
      commit: "Sign: audit agrees",
      preview: "Would leave the case closed",
      text: () => "Agreeing with the audit means the case stays closed.",
    },
    inadequate: {
      commit: "Sign: audit disagrees",
      preview: "Would reopen the case",
      text: () => "Disagreeing with the audit reopens the case.",
    },
  },
};

/** The outcome as the review's purpose words it; ordinary reviews keep the generic copy. */
export function outcomeFor(key: OutcomeKey, purpose?: ReviewPurpose): Outcome {
  const copy = purpose && PURPOSE_COPY[purpose][key];
  return copy ? { ...OUTCOMES[key], ...copy } : OUTCOMES[key];
}

/** Live confirmation copy is limited to answers actually appended for this case. */
export function recordedOutcome(
  outcome: OutcomeKey,
  signatures: Signature[],
  confirmedClosed = false,
  purpose?: ReviewPurpose,
): Pick<Outcome, "recordedText" | "recordedAnswerCell"> {
  const recorded = OUTCOMES[outcome];
  if (purpose === "auto_audit" && (outcome === "close" || outcome === "inadequate")) {
    return {
      recordedText:
        outcome === "close"
          ? "Audit agreement was saved in the demo. The case is not changed here."
          : "Audit disagreement was saved in the demo. The case is not changed here; reopening is not confirmed until the case shows it.",
      recordedAnswerCell: recorded.recordedAnswerCell,
    };
  }
  if (outcome === "evidence") {
    const decision = signatures.find((s) => s.decision !== "same" && s.decision !== "different")?.decision;
    return {
      recordedText: decision
        ? `Evidence answer ${decision} was saved in the demo. No change to the case is confirmed.`
        : "No evidence answer was saved. No change to the case is confirmed.",
      recordedAnswerCell: decision ? { word: `${decision}, signed`, tone: "muted" } : { word: "not signed", tone: "muted" },
    };
  }
  if (outcome !== "close" && outcome !== "inadequate") {
    return { recordedText: recorded.recordedText, recordedAnswerCell: recorded.recordedAnswerCell };
  }

  const reportDecision = outcome === "close" ? "adequate" : "inadequate";
  if (signatures.some((s) => s.decision === reportDecision)) {
    const withIdentity = signatures.some((s) => s.decision === "same");
    const answer = withIdentity
      ? outcome === "close"
        ? "Same patient and report adequate were"
        : "Same patient and report not adequate were"
      : outcome === "close"
        ? "Report adequate was"
        : "Report not adequate was";
    const remaining = outcome === "close" && confirmedClosed
      ? "The case now shows as closed."
      : outcome === "close"
        ? "The case still shows as open; closure is not confirmed."
        : "No repeat booking is confirmed.";
    return {
      recordedText: `${answer} saved in the demo. ${remaining}`,
      recordedAnswerCell: recorded.recordedAnswerCell,
    };
  }

  const remaining = outcome === "close" && confirmedClosed
    ? "The case now shows as closed."
    : outcome === "close"
      ? "The case still shows as open; closure is not confirmed."
      : "No repeat booking is confirmed.";
  return {
    recordedText:
      outcome === "close" && confirmedClosed
        ? `Same patient was saved in the demo. ${remaining}`
        : `Same patient was saved in the demo. No report answer was saved. ${remaining}`,
    recordedAnswerCell: { word: "not signed", tone: "muted" },
  };
}

export type SaveFailure = { status: 400 | 403 | 404; error: string };

/** The API's refusals as sentences a reviewer can act on. */
export function failureSentence(sig: Signature, failure: SaveFailure): string {
  const what = sig.decision === "same" || sig.decision === "different" ? "the identity answer" : "the report answer";
  switch (failure.status) {
    case 403:
      return `Not saved: the ${roleFor(sig.decision)} role is not allowed to sign ${what}.`;
    case 404:
      return `Not saved: this case has no open review item for ${what}.`;
    case 400:
      return `Not saved: ${what} could not be read.`;
  }
}

/*
 * Independent double review: one server-projected state per (item, question). The browser only drafts and submits one
 * read at a time; what each viewer may see and submit is decided by the API and never inferred here.
 */

export type Question = "link" | "evidence";

export const questionAnchor = (item: string, question: Question) => `review-question-${item.replace(/[^A-Za-z0-9_-]/g, "-")}-${question}`;

const leaseTime = (value: string) => staffDueTime(value);

/** Server-projected ownership, formatted consistently on the queue and desk. */
export function claimSummary(state: Pick<ReviewState, "claim">): string {
  const claim = state.claim;
  if (!claim) return "Unclaimed";
  return claim.status === "expired"
    ? `Claim by ${claim.owner} expired at ${leaseTime(claim.expires_at)}`
    : `Claimed by ${claim.owner} until ${leaseTime(claim.expires_at)}`;
}

export function claimActionLabel(state: Pick<ReviewState, "claim_actions">): string | undefined {
  const actions = state.claim_actions;
  if (actions?.claim) return "Claim";
  if (actions?.takeover) return "Take over";
  if (actions?.renew || actions?.release) return "Renew or release";
  return undefined;
}

export function canAnswerClaim(state: ReviewState): boolean {
  return Boolean(
    state.may_submit &&
      state.next_read &&
      state.claim?.status === "active" &&
      state.claim.read === state.next_read &&
      state.claim_head &&
      !state.frozen,
  );
}

/** The draft for a question is keyed by the read it will become, so a changed server state never reuses a stale answer. */
export const draftKey = (item: string, question: Question, nextRead: ReviewSlot) => `${item}:${question}:${nextRead}`;

export const SUBMIT_LABEL: Record<ReviewSlot, string> = {
  r1: "Submit first read",
  r2: "Submit second read",
  r3: "Submit senior resolution",
};

/** Link first, then evidence; items keep the server's order. */
export function orderedStates(states: readonly ReviewState[]): ReviewState[] {
  const rank = (s: ReviewState) => (s.question === "link" ? 0 : 1);
  return states.map((s, i) => ({ s, i })).sort((a, b) => rank(a.s) - rank(b.s) || a.i - b.i).map((x) => x.s);
}

/** What this question needs now, from the viewer's side. Concealment is the server's: this only words what arrived. */
export function stateHeadline(s: ReviewState): { text: string; tone: "positive" | "negative" | "warning" | "muted" } {
  const own = s.reads.some((r) => r.read === "r1" && r.submitted && r.decision !== undefined);
  const actionable = s.may_submit || Boolean(s.claim_actions?.claim || s.claim_actions?.takeover);
  switch (s.state) {
    case "awaiting_r1":
      return actionable ? { text: "First read due", tone: "warning" } : { text: "Waiting for a first read", tone: "muted" };
    case "awaiting_r2":
      if (actionable) return { text: "Second read due", tone: "warning" };
      return { text: own ? "Your first read is in; waiting for a different reviewer" : "Waiting for a second read", tone: "muted" };
    case "awaiting_r3":
      return actionable ? { text: "Disagreement: senior resolution due", tone: "warning" } : { text: "Disagreement: waiting for a senior", tone: "negative" };
    case "resolved_agreed":
      return { text: "Resolved: both reads agree", tone: "positive" };
    case "resolved_senior":
      return { text: "Resolved by a senior", tone: "positive" };
  }
}

/** How the desk words each CRC evidence answer: colonoscopy adequacy, or pathology histology. */
export const CRC_CHOICE_LABELS: Record<string, string> = {
  adequate: "Adequate",
  inadequate: "Not adequate",
  histology: "Histology present",
  no_histology: "No histology",
};

/** A decision as a sentence, keeping the programme's own evidence labels. CRC keeps its adequacy wording. */
export function answerWords(decision: string, question: Question, scoped = false): string {
  if (scoped && question === "evidence" && (decision === "adequate" || decision === "inadequate")) return `${decision === "adequate" ? "Adequate" : "Not adequate"} for this question only`;
  if (decision === "same") return "Same patient";
  if (decision === "different") return "Not the same patient";
  if (decision === "adequate") return "Report adequate";
  if (decision === "inadequate") return "Report not adequate";
  if (decision === "histology") return "Histology present";
  if (decision === "no_histology") return "No histology";
  return question === "evidence" ? `Evidence label ${decision}` : decision;
}

const SLOT_ORDER: Record<string, number> = { r1: 0, r2: 1, r3: 2 };

/** Submitted reads in slot order. A concealed read keeps its slot but has no decision. */
export function submittedReads(s: ReviewState): ReviewRead[] {
  return s.reads.filter((r) => r.submitted).sort((a, b) => (SLOT_ORDER[a.read] ?? 9) - (SLOT_ORDER[b.read] ?? 9));
}

export function readLine(r: ReviewRead, question: Question, scoped = false): string {
  const label = READ_LABEL[r.read];
  return r.decision === undefined ? `${label}: submitted; the answer stays hidden until the second read` : `${label}: ${answerWords(r.decision, question, scoped)}`;
}

/** The disputed-rows cell once the evidence question is final. Never set before a terminal state. */
export function resolvedCell(decision: string): { word: string; tone: "positive" | "negative" | "warning" | "muted" } {
  if (decision === "adequate") return { word: "adequate, resolved", tone: "positive" };
  if (decision === "inadequate") return { word: "not adequate, resolved", tone: "negative" };
  return { word: "resolved", tone: "muted" };
}

/** The API's refusals for one read, as sentences a reviewer can act on. Nothing here claims a save. */
export function readFailureSentence(failure: { status: number; error: string }): string {
  switch (failure.status) {
    case 401:
      return "Not saved: sign in again to continue.";
    case 403:
      return "Not saved: you cannot give the next answer for this question.";
    case 404:
      return "Not saved: this question is no longer open for review.";
    case 409:
      return "Not saved: the review changed while you were answering. Check the latest answers before answering again.";
    default:
      return "Not saved: the answer could not be saved. Try again.";
  }
}
