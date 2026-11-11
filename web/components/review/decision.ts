import type { Role } from "@/lib/api/schema";

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
