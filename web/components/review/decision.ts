import type { Role, ReviewRead, ReviewState } from "@/lib/api/schema";
import { READ_LABEL, type ReviewSlot } from "@/lib/domain/stage";

/*
 * Review decisions in the order the reviewer answers them: identity first (reviewer role) when a link item exists,
 * then the raised evidence item (clinician role). Each item can also appear alone; non-CRC labels do not infer closure.
 */

export type IdentityAnswer = "same" | "different";
/** A programme evidence label, or CRC's local-only ask action. Zod validates labels again in the server action. */
export type ReportAnswer = string;
export type Draft = { identity?: IdentityAnswer; report?: ReportAnswer };

export type OutcomeKey = "close" | "inadequate" | "evidence" | "linkSame" | "ask" | "notSame";

/** The review items this case raised, by the question they answer. */
export type ReviewItems = { link?: string; evidence?: string };

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
    if (draft.report === "adequate") return "close";
    if (draft.report === "inadequate") return "inadequate";
    return "evidence";
  }
  if (items.link && draft.identity === "same") {
    if (!items.evidence) return "linkSame";
    if (draft.report === "ask") return "ask";
  }
  return undefined;
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

export const signWords = (decision: string) => SIGNS[decision] ?? `evidence ${decision}`;

export const OUTCOMES: Record<OutcomeKey, Outcome> = {
  close: {
    commit: "Close the case",
    pill: "Demo answers appended; case open",
    preview: "Would close the case",
    tone: "positive",
    text: (name, _unit, hasLink = true) =>
      hasLink
        ? `Same patient, report adequate. ${name}’s case closes with both answers and the report quote in her record.`
        : "Report adequate would be appended for this evidence-only CRC item. No identity answer would be invented.",
    next: (_unit, hasLink = true) => (hasLink ? { who: "Closed", what: "with both answers recorded" } : { who: "Case data", what: "no downstream effect inferred" }),
    recordedText: "Same patient and report adequate were appended to the demo review log. Current case data still shows the case as open; closure is not confirmed.",
    recordedAnswerCell: { word: "adequate, signed", tone: "positive" },
    answerCell: { word: "adequate, signed", tone: "positive" },
  },
  inadequate: {
    commit: "Sign: report not adequate",
    pill: "Demo answers appended",
    preview: "Would not close: repeat colonoscopy",
    tone: "negative",
    text: (name, _unit, hasLink = true) =>
      hasLink
        ? `Same patient, but the colonoscopy report was not adequate, so this colonoscopy does not count. ${name} goes back to Today for a repeat booking.`
        : "Report not adequate would be appended for this evidence-only CRC item. No identity answer or repeat booking would be invented.",
    next: () => ({ who: "Her coordinator", what: "to book a repeat colonoscopy" }),
    recordedText: "Same patient and report not adequate were appended to the demo review log. No repeat booking is confirmed.",
    recordedAnswerCell: { word: "not adequate, signed", tone: "negative" },
    answerCell: { word: "not adequate, signed", tone: "negative" },
  },
  evidence: {
    commit: "Sign evidence answer",
    pill: "Demo evidence answer appended",
    preview: "Preview evidence answer",
    tone: "muted",
    text: () => "The selected programme evidence label would be appended. No closure or other downstream effect is inferred.",
    next: () => ({ who: "Case data", what: "no downstream effect inferred" }),
    recordedText: "The evidence answer was appended to the demo review log. No downstream effect is confirmed.",
    recordedAnswerCell: { word: "signed", tone: "muted" },
    answerCell: { word: "signed", tone: "muted" },
  },
  linkSame: {
    commit: "Sign: same patient",
    pill: "Demo identity answer appended",
    preview: "Would confirm the identity link",
    tone: "positive",
    text: () => "Same patient would be appended for this link-only item. No evidence answer would be invented.",
    next: () => ({ who: "Case data", what: "no downstream effect inferred" }),
    recordedText: "Same patient was appended to the demo review log. No downstream effect is confirmed.",
    recordedAnswerCell: { word: "not answered", tone: "muted" },
    answerCell: { word: "not answered", tone: "muted" },
  },
  ask: {
    commit: "Sign and ask the unit to complete the report",
    pill: "Demo identity answer appended",
    preview: "Would ask the unit",
    tone: "warning",
    text: (_name, unit) => `Same patient is signed. The case stays held until ${unit} completes the report.`,
    next: (unit) => ({ who: unit, what: "to complete the colonoscopy report" }),
    recordedText: "Same patient was appended to the demo review log. No report answer was appended. No unit notification was sent.",
    recordedAnswerCell: { word: "not answered", tone: "muted" },
    answerCell: { word: "asked the unit", tone: "warning" },
  },
  notSame: {
    commit: "Sign: not the same patient",
    pill: "Demo identity answer appended",
    preview: "Would reject the link",
    tone: "negative",
    text: (name, unit) => `The report is not ${name}’s. She stays open, and ${unit} is told the report matches no patient on our list.`,
    next: () => ({ who: "Link undone", what: "report returned unmatched" }),
    recordedText: "Not the same patient was appended to the demo review log. No link or unit change is confirmed.",
    recordedAnswerCell: { word: "not answered", tone: "muted" },
    answerCell: { word: "not applied", tone: "muted" },
  },
};

/** Live confirmation copy is limited to answers actually appended for this case. */
export function recordedOutcome(
  outcome: OutcomeKey,
  signatures: Signature[],
  confirmedClosed = false,
): Pick<Outcome, "recordedText" | "recordedAnswerCell"> {
  const recorded = OUTCOMES[outcome];
  if (outcome === "evidence") {
    const decision = signatures.find((s) => s.decision !== "same" && s.decision !== "different")?.decision;
    return {
      recordedText: decision
        ? `Evidence answer ${decision} was appended to the demo review log. No downstream effect is confirmed.`
        : "No evidence answer was appended. No downstream effect is confirmed.",
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
      ? "Refreshed case data confirms closure."
      : outcome === "close"
        ? "Current case data still shows the case as open; closure is not confirmed."
        : "No repeat booking is confirmed.";
    return {
      recordedText: `${answer} appended to the demo review log. ${remaining}`,
      recordedAnswerCell: recorded.recordedAnswerCell,
    };
  }

  const remaining = outcome === "close" && confirmedClosed
    ? "Refreshed case data confirms closure."
    : outcome === "close"
      ? "Current case data still shows the case as open; closure is not confirmed."
      : "No repeat booking is confirmed.";
  return {
    recordedText:
      outcome === "close" && confirmedClosed
        ? `Same patient was appended to the demo review log. ${remaining}`
        : `Same patient was appended to the demo review log. No report answer was appended. ${remaining}`,
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
      return `Not saved: the server could not read ${what} (${failure.error}).`;
  }
}

/*
 * Independent double review: one server-projected state per (item, question). The browser only drafts and submits one
 * read at a time; what each viewer may see and submit is decided by the API and never inferred here.
 */

export type Question = "link" | "evidence";

export const questionAnchor = (item: string, question: Question) => `review-question-${item.replace(/[^A-Za-z0-9_-]/g, "-")}-${question}`;

function leaseTime(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : `${date.toISOString().slice(0, 10)} ${date.toISOString().slice(11, 16)} UTC`;
}

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

/** A decision as a sentence, keeping the programme's own evidence labels. CRC keeps its adequacy wording. */
export function answerWords(decision: string, question: Question): string {
  if (decision === "same") return "Same patient";
  if (decision === "different") return "Not the same patient";
  if (decision === "adequate") return "Report adequate";
  if (decision === "inadequate") return "Report not adequate";
  return question === "evidence" ? `Evidence label ${decision}` : decision;
}

const SLOT_ORDER: Record<string, number> = { r1: 0, r2: 1, r3: 2 };

/** Submitted reads in slot order. A concealed read keeps its slot but has no decision. */
export function submittedReads(s: ReviewState): ReviewRead[] {
  return s.reads.filter((r) => r.submitted).sort((a, b) => (SLOT_ORDER[a.read] ?? 9) - (SLOT_ORDER[b.read] ?? 9));
}

export function readLine(r: ReviewRead, question: Question): string {
  const label = READ_LABEL[r.read];
  return r.decision === undefined ? `${label}: submitted; the answer stays hidden until the second read` : `${label}: ${answerWords(r.decision, question)}`;
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
      return "Not saved: sign in with your reviewer token to continue.";
    case 403:
      return "Not saved: this credential cannot submit the next read for this question.";
    case 404:
      return "Not saved: no independent review is open for this question.";
    case 409:
      return "Not saved: the review changed while you were answering. The current state is shown; check it before answering again.";
    default:
      return `Not saved: the server could not read the answer (${failure.error}).`;
  }
}
