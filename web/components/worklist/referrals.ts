import type { CaseSummary, CurrentDecision, HandoffStatus, ReferralCondition, ReferralReturnReason } from "@/lib/api/schema";
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

export const CONDITION_LABELS: Record<ReferralCondition, string> = {
  hold_anticoagulants: "Hold blood thinners as advised",
  arabic_interpreter: "Arabic interpreter on the day",
  female_endoscopist: "Female endoscopist",
  kidney_function_check: "Kidney function before booking",
};
export const REASON_LABELS: Record<ReferralReturnReason, string> = {
  needs_clinic_visit: "Needs a clinic visit first",
  anticoagulant_plan_missing: "Blood thinner plan missing",
  wrong_unit: "Wrong unit",
  not_fit_for_procedure: "Not fit for colonoscopy",
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
  /** Hand-off of a returned referral: the persisted unit return's packet, else the canonical one; null when none. */
  handoff: Handoff | null;
  /** The case's `api_overlay.head`, which a referral response must name; null when the API sent no overlay. */
  head: string | null;
  /** The receiving unit's persisted answer, in words, or null when the unit has not answered through the API. */
  answer: string | null;
};

const CONSENT_WORDS: Record<CaseSummary["consent"], string> = {
  granted: "Granted",
  denied: "Declined",
  opted_out: "Opted out",
  unknown: "Not recorded",
};

export function referralFrom(c: CaseSummary, now: number, hasRecord: boolean, handoff: Handoff | null = null): Referral | null {
  const answered = c.closed ? undefined : c.api_overlay?.referral_response;
  const returned = c.api_overlay?.referral_handoff;
  const state = c.closed ? "done" : answered ? (answered.response === "return" ? "returned" : "accepted") : STATE_BY_DROPOFF[c.dropoff];
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
    handoff: answered && returned ? { status: returned.status, actBy: returned.act_by } : handoff,
    head: c.api_overlay?.head ?? null,
    answer: answered ? answerWords(answered) : null,
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

function answerWords(a: NonNullable<NonNullable<CaseSummary["api_overlay"]>["referral_response"]>): string {
  if (a.response === "return") return `Returned to the GP: ${a.reason ? REASON_LABELS[a.reason].toLowerCase() : "no reason recorded"}`;
  if (a.response === "accept_with_conditions") return `Accepted with conditions: ${a.conditions.map((c) => CONDITION_LABELS[c]).join("; ")}`;
  return "Accepted the referral";
}
