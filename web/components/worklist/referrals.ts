import type { CaseSummary, CurrentDecision, HandoffStatus } from "@/lib/api/schema";
import { formatDay } from "@/lib/domain/day";
import { handoffUrgent } from "@/lib/domain/handoff";
import { shortId } from "@/lib/domain/ids";
import { personForLoop } from "@/lib/people";
import { referralSeed } from "@/lib/people/identity";
import type { PersonSeed } from "@/lib/people/types";
import { ownerLabel, providerLabel } from "./model";

/** Where a referral sits in the endoscopy unit's inbox, derived from the case's drop-off. */
export type ReferralState = "new" | "grade" | "returned" | "accepted" | "done";

/** Drop-offs before the unit holds the referral (the clinic still owns them) are absent. */
const STATE_BY_DROPOFF: Partial<Record<CaseSummary["dropoff"], ReferralState>> = {
  referral_unanswered: "new",
  not_booked: "new",
  inadequate: "grade",
  referral_rejected: "returned",
  in_progress: "accepted",
  no_show: "accepted",
  awaiting_histology: "accepted",
  "": "done",
};

/** Where a returned referral stands with the referring GP, from the canonical hand-off packet or a session return. */
export type Handoff = { status: HandoffStatus; actBy: number };

export function handoffOf(current: CurrentDecision): Handoff | null {
  return current.handoff ? { status: current.handoff.status, actBy: current.handoff.act_by } : null;
}

const HANDOFF_WORDS: Record<HandoffStatus, string> = {
  awaiting_referrer: "Act on the return",
  referrer_acted: "The GP has acted",
  coordinator: "Escalated to the care coordinator",
  medical_director: "Escalated to the medical director",
};

/** A past-due packet keeps its original deadline; only one still awaiting the referrer is phrased as upcoming. */
export function handoffWords(h: Handoff | null): { what: string; when: string } {
  if (!h) return { what: HANDOFF_WORDS.awaiting_referrer, when: "deadline unavailable" };
  return { what: HANDOFF_WORDS[h.status], when: `${h.status === "awaiting_referrer" ? "by" : "was due"} ${formatDay(h.actBy)}` };
}

export type Referral = {
  id: string;
  short: string;
  name: string;
  nameAr?: string;
  age?: number;
  seed: PersonSeed;
  clinic: string;
  state: ReferralState;
  /** Whole days since the referral at run day `now`. */
  waiting: number;
  sent: string;
  /** Reasons to look before accepting; any flag turns the inline Accept into Review. */
  flags: string[];
  details: [string, string][];
  hasRecord: boolean;
  /** Whether a return clocks the referrer two business days (suspected cancer) or five. */
  urgent: boolean;
  /** Canonical hand-off of an existing returned referral; null when the case carries none. */
  handoff: Handoff | null;
};

const CONSENT_WORDS: Record<CaseSummary["consent"], string> = {
  granted: "Granted",
  denied: "Declined",
  opted_out: "Opted out",
  unknown: "Not recorded",
};

export function referralFrom(c: CaseSummary, now: number, hasRecord: boolean, handoff: Handoff | null = null): Referral | null {
  const state = c.closed ? "done" : STATE_BY_DROPOFF[c.dropoff];
  if (!state) return null;
  const person = personForLoop(c.id);
  const flags = [
    c.consent !== "granted" && `Consent ${CONSENT_WORDS[c.consent].toLowerCase()}`,
    c.owner === "" && "No owner at the clinic",
    c.review === "open" && "Open review",
    c.state === "inadequate" && "Report incomplete",
  ].filter((f): f is string => !!f);
  return {
    id: c.id,
    short: shortId(c.id),
    name: person?.display_name ?? shortId(c.id),
    nameAr: person?.display_name_ar,
    age: person?.age,
    seed: referralSeed(c.id),
    clinic: providerLabel(c.provider),
    state,
    waiting: Math.floor(now - c.referral_day),
    sent: formatDay(c.referral_day),
    flags,
    hasRecord,
    urgent: handoffUrgent(c.programme, c.priority),
    handoff,
    details: [
      ["Indication", "Positive FIT, colorectal screening"],
      ["Referred", formatDay(c.referral_day)],
      ["Day 31", formatDay(c.referral_day + 31)],
      ["Consent to texts", CONSENT_WORDS[c.consent]],
      ["Owner at the clinic", ownerLabel(c.owner)],
      ["Case", shortId(c.id)],
    ],
  };
}
