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

export const SIGNER = { name: "Dr Omar Al Kaabi", initials: "OK" } as const;

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
  text: (name: string, unit: string) => string;
  next: (unit: string) => { who: string; what: string };
  /** The bowel prep row of the readers table once signed. */
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
    pill: "Closed with two signatures",
    tone: "positive",
    text: (name) => `Same patient, prep adequate. ${name}’s case closes with both signatures and the report quote in her record.`,
    next: () => ({ who: "Closed", what: "with two signatures" }),
    prepCell: { word: "adequate, signed", tone: "positive" },
  },
  inadequate: {
    commit: "Sign: prep not adequate",
    pill: "Not closed: repeat colonoscopy",
    tone: "negative",
    text: (name) => `Same patient, but the prep was not adequate, so this colonoscopy does not count. ${name} goes back to Today for a repeat booking.`,
    next: () => ({ who: "Her coordinator", what: "to book a repeat colonoscopy" }),
    prepCell: { word: "not adequate, signed", tone: "negative" },
  },
  ask: {
    commit: "Sign and ask the unit to grade the prep",
    pill: "Asked the unit",
    tone: "warning",
    text: (_name, unit) => `Same patient is signed. The case stays held until ${unit} grades the prep.`,
    next: (unit) => ({ who: unit, what: "to grade the bowel prep" }),
    prepCell: { word: "asked the unit", tone: "warning" },
  },
  notSame: {
    commit: "Sign: not the same patient",
    pill: "Link rejected",
    tone: "negative",
    text: (name, unit) => `The report is not ${name}’s. She stays open, and ${unit} is told the report matches no patient on our list.`,
    next: () => ({ who: "Link undone", what: "report returned unmatched" }),
    prepCell: { word: "not applied", tone: "muted" },
  },
};

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
