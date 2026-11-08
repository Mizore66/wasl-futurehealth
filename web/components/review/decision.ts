import type { Role } from "@/lib/api/schema";

/*
 * The second-signature decision, in the order the reviewer answers it:
 * identity first (reviewer role), then the disputed field, bowel prep (clinician role).
 */

export type IdentityAnswer = "same" | "different";
export type PrepAnswer = "adequate" | "inadequate" | "ask";
export type Draft = { identity?: IdentityAnswer; prep?: PrepAnswer };

export type OutcomeKey = "close" | "inadequate" | "ask" | "notSame";

/** The review items this case raised, by the question they answer. */
export type ReviewItems = { link?: string; evidence?: string };

/** One appended review event: what the API receives. */
export type Signature = { item: string; decision: "same" | "different" | "adequate" | "inadequate" };

export const SIGNER = { name: "Dr Omar Al Kaabi", title: "Medical director", initials: "OK" } as const;

export function roleFor(decision: Signature["decision"]): Role {
  return decision === "same" || decision === "different" ? "reviewer" : "clinician";
}

/** No outcome until every question the identity answer opens is answered. */
export function outcomeOf(draft: Draft): OutcomeKey | undefined {
  if (draft.identity === "different") return "notSame";
  if (draft.identity !== "same" || !draft.prep) return undefined;
  return draft.prep === "adequate" ? "close" : draft.prep;
}

/** "Ask the unit" has no API decision: it signs identity only. */
export function signaturesFor(draft: Draft, items: ReviewItems): Signature[] {
  const outcome = outcomeOf(draft);
  if (!outcome || !items.link) return [];
  const identity: Signature = { item: items.link, decision: outcome === "notSame" ? "different" : "same" };
  if (outcome === "notSame" || outcome === "ask" || !items.evidence) return [identity];
  return [identity, { item: items.evidence, decision: outcome === "close" ? "adequate" : "inadequate" }];
}

export type Outcome = {
  commit: string;
  pill: string;
  tone: "positive" | "negative" | "warning";
  /** Hypothetical downstream language for fixture previews only. */
  text: (name: string, unit: string) => string;
  next: (unit: string) => { who: string; what: string };
  /** What the same answer reads as in the demo, where nothing is appended. */
  preview: string;
  /** What a 201 proves, without claiming any downstream effect. */
  recordedText: string;
  recordedPrepCell: { word: string; tone: "positive" | "negative" | "warning" | "muted" };
  /** The bowel prep row of the readers table in a fixture preview. */
  prepCell: { word: string; tone: "positive" | "negative" | "warning" | "muted" };
};

/** What each signature says, as it reads in the trail. */
export const SIGNS: Record<Signature["decision"], string> = {
  same: "same patient",
  different: "not the same patient",
  adequate: "prep adequate",
  inadequate: "prep not adequate",
};

export const OUTCOMES: Record<OutcomeKey, Outcome> = {
  close: {
    commit: "Close the case",
    pill: "Demo answers appended; case open",
    preview: "Would close the case",
    tone: "positive",
    text: (name) => `Same patient, prep adequate. ${name}’s case closes with both answers and the report quote in her record.`,
    next: () => ({ who: "Closed", what: "with both answers recorded" }),
    recordedText: "Same patient and prep adequate were appended to the demo review log. Current case data still shows the case as open; closure is not confirmed.",
    recordedPrepCell: { word: "adequate, signed", tone: "positive" },
    prepCell: { word: "adequate, signed", tone: "positive" },
  },
  inadequate: {
    commit: "Sign: prep not adequate",
    pill: "Demo answers appended",
    preview: "Would not close: repeat colonoscopy",
    tone: "negative",
    text: (name) => `Same patient, but the prep was not adequate, so this colonoscopy does not count. ${name} goes back to Today for a repeat booking.`,
    next: () => ({ who: "Her coordinator", what: "to book a repeat colonoscopy" }),
    recordedText: "Same patient and prep not adequate were appended to the demo review log. No repeat booking is confirmed.",
    recordedPrepCell: { word: "not adequate, signed", tone: "negative" },
    prepCell: { word: "not adequate, signed", tone: "negative" },
  },
  ask: {
    commit: "Sign and ask the unit to grade the prep",
    pill: "Demo identity answer appended",
    preview: "Would ask the unit",
    tone: "warning",
    text: (_name, unit) => `Same patient is signed. The case stays held until ${unit} grades the prep.`,
    next: (unit) => ({ who: unit, what: "to grade the bowel prep" }),
    recordedText: "Same patient was appended to the demo review log. No bowel prep answer was appended. No unit notification was sent.",
    recordedPrepCell: { word: "not graded", tone: "muted" },
    prepCell: { word: "asked the unit", tone: "warning" },
  },
  notSame: {
    commit: "Sign: not the same patient",
    pill: "Demo identity answer appended",
    preview: "Would reject the link",
    tone: "negative",
    text: (name, unit) => `The report is not ${name}’s. She stays open, and ${unit} is told the report matches no patient on our list.`,
    next: () => ({ who: "Link undone", what: "report returned unmatched" }),
    recordedText: "Not the same patient was appended to the demo review log. No link or unit change is confirmed.",
    recordedPrepCell: { word: "not graded", tone: "muted" },
    prepCell: { word: "not applied", tone: "muted" },
  },
};

/** Live confirmation copy is limited to answers actually appended for this case. */
export function recordedOutcome(
  outcome: OutcomeKey,
  signatures: Signature[],
  confirmedClosed = false,
): Pick<Outcome, "recordedText" | "recordedPrepCell"> {
  const recorded = OUTCOMES[outcome];
  if (outcome !== "close" && outcome !== "inadequate") {
    return { recordedText: recorded.recordedText, recordedPrepCell: recorded.recordedPrepCell };
  }

  const prepDecision = outcome === "close" ? "adequate" : "inadequate";
  if (signatures.some((s) => s.decision === prepDecision)) {
    if (outcome === "close" && confirmedClosed) {
      return {
        recordedText: "Same patient and prep adequate were appended to the demo review log. Refreshed case data confirms closure.",
        recordedPrepCell: recorded.recordedPrepCell,
      };
    }
    return { recordedText: recorded.recordedText, recordedPrepCell: recorded.recordedPrepCell };
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
        : `Same patient was appended to the demo review log. No bowel prep answer was appended. ${remaining}`,
    recordedPrepCell: { word: "not signed", tone: "muted" },
  };
}

export type SaveFailure = { status: 400 | 403 | 404; error: string };

/** The API's refusals as sentences a reviewer can act on. */
export function failureSentence(sig: Signature, failure: SaveFailure): string {
  const what = sig.decision === "same" || sig.decision === "different" ? "the identity answer" : "the bowel prep answer";
  switch (failure.status) {
    case 403:
      return `Not saved: the ${roleFor(sig.decision)} role is not allowed to sign ${what}.`;
    case 404:
      return `Not saved: this case has no open review item for ${what}.`;
    case 400:
      return `Not saved: the server could not read ${what} (${failure.error}).`;
  }
}
