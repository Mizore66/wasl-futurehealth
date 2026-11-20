import type { CaseSummary, CurrentDecision, HandoffStatus, ReferralCondition, ReferralInfo, ReferralReturnReason, ReferrerResponseCode } from "@/lib/api/schema";
import { formatDay } from "@/lib/domain/day";
import { handoffUrgent } from "@/lib/domain/handoff";
import { shortId } from "@/lib/domain/ids";
import { personForLoop } from "@/lib/people";
import { referralSeed } from "@/lib/people/identity";
import type { PersonSeed } from "@/lib/people/types";
import type { SessionReturn } from "./session-returns";
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

const HANDOFF_WORDS: Record<HandoffStatus, (referrer: string) => string> = {
  awaiting_referrer: () => "Act on the return",
  referrer_acted: (referrer) => `The ${referrer} has acted`,
  coordinator: () => "Escalated to the care coordinator",
  medical_director: () => "Escalated to the medical director",
};

/** A past-due packet keeps its original deadline; only one still awaiting the referrer is phrased as upcoming. */
export function handoffWords(h: Handoff | null, referrer = "GP"): { what: string; when: string } {
  if (!h) return { what: HANDOFF_WORDS.awaiting_referrer(referrer), when: "deadline unavailable" };
  /* An answered packet is not overdue: its act-by day is history, not a missed deadline. */
  const when = h.status === "awaiting_referrer" ? "by" : h.status === "referrer_acted" ? "answer was due by" : "was due";
  return { what: HANDOFF_WORDS[h.status](referrer), when: `${when} ${formatDay(h.actBy)}` };
}

/** Who holds a returned referral at each packet status: the responsible referrer until escalation moves it on. */
export function handoffOwner(h: Handoff | null, referrer: string): string {
  return h?.status === "coordinator" ? "Care coordinator" : h?.status === "medical_director" ? "Medical director" : referrer;
}

/**
 * A referral journey the inbox can show, addressed by programme and referring provider. CRC is the receiving endoscopy
 * unit's journey; lung work-up and DR show the canonical returned-referral packets of their single receiving unit.
 * `credential` is the web server's referring-clinician credential for the view's referring provider (lib/api/source.ts),
 * and `returned` describes the referrer's returned-referral page.
 */
export type ReferralView = {
  programme: "crc" | "lung_nodule" | "diabetic_retinopathy";
  provider: string;
  label: string;
  unit: string;
  referrer: string;
  credential: "referrer" | "referrer_lung" | "referrer_dr";
  returned: string;
  /** The open referral's heading and its programme chip. */
  title: string;
  indication: string;
};
export const REFERRAL_VIEWS: readonly ReferralView[] = [
  { programme: "crc", provider: "ahs-mushrif", label: "Colorectal, Al Mushrif", unit: "SKMC endoscopy", referrer: "GP", credential: "referrer", returned: "Referring GP, Al Mushrif. Referrals the endoscopy unit handed back to you.",
    title: "Colonoscopy referral", indication: "Positive FIT" },
  { programme: "lung_nodule", provider: "rad-imaging", label: "Lung nodule work-up", unit: "Radiology department, regional hospital", referrer: "referrer", credential: "referrer_lung",
    returned: "Referring clinician, outpatient imaging centre. Nodule work-up referrals the radiology department handed back to you.",
    title: "Nodule work-up referral", indication: "Lung nodule" },
  { programme: "diabetic_retinopathy", provider: "phc-retina", label: "Diabetic eye", unit: "Eye clinic, regional hospital", referrer: "referrer", credential: "referrer_dr",
    returned: "Referring clinician, primary care eye screening. Eye-clinic referrals the eye clinic handed back to you.",
    title: "Eye clinic referral", indication: "Diabetic eye" },
];

/** The view a URL names: no programme and no provider is the CRC default, and an unknown pair is null. */
export function referralView(programme?: string, provider?: string): ReferralView | null {
  if (!programme && !provider) return REFERRAL_VIEWS[0];
  return REFERRAL_VIEWS.find((v) => v.programme === programme && v.provider === provider) ?? null;
}

/** The query string that keeps a selection across search and reload; the CRC default needs none. */
export function viewScope(programme = "", provider = ""): string {
  return programme || provider ? new URLSearchParams({ programme, provider }).toString() : "";
}

const UNIT_NAMES: Record<string, string> = {
  "rad-imaging": "Outpatient imaging centre",
  "mafraq-rad": "Radiology department, regional hospital",
  "phc-retina": "Primary care eye screening",
  "eye-hosp": "Eye clinic, regional hospital",
};
const PACKET_REASONS: Record<string, string> = {
  needs_more_info: "Needs more information",
  wrong_provider: "Not the patient's network",
  service_not_available: "No capacity for the procedure",
  duplicate: "Duplicate referral",
  other: "Other",
};
export const INFO_LABELS: Record<ReferralInfo, string> = {
  reason_for_referral: "Reason for referral",
  relevant_clinical_information: "Relevant clinical information",
  expected_outcome: "Expected outcome",
  anticoagulant_plan: "Blood thinner plan",
  kidney_function_result: "Kidney function result",
};
const INFO_WORDS: Record<string, string> = INFO_LABELS;
const PACKET_STATUS: Record<HandoffStatus, string> = {
  awaiting_referrer: "Awaiting the referrer",
  referrer_acted: "The referrer has acted",
  coordinator: "Escalated to the care coordinator",
  medical_director: "Escalated to the medical director",
};
const UNAVAILABLE = "Not recorded";
const unitName = (id: string) => UNIT_NAMES[id] ?? id;

/** The packet's "referring PCP, phc-retina" in words; a missing clinician stays explicitly unavailable. */
function responsibleWords(who: string | undefined): string {
  if (!who) return UNAVAILABLE;
  const [role, unit] = who.split(", ");
  return unit ? `${role === "referring PCP" ? "Referring clinician" : role}, ${unitName(unit)}` : who;
}

export type Referral = {
  id: string;
  /** "crc": the endoscopy unit answers it. "packet": a canonical returned-referral packet, read-only bar a session re-referral. */
  kind: "crc" | "packet";
  /** Who a return goes back to, in words. */
  referrer: string;
  /** The receiving unit, in words. */
  unit: string;
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
  /** Where `answer` comes from, in words. */
  answerWhen: string;
  /** The referrer acknowledged the returned packet through the API (it still waits on their answer). */
  acknowledged?: boolean;
  /** The unit's note to the GP on a return, when the case detail was read; `hasNote` when the list says one exists. */
  note?: string;
  hasNote?: boolean;
  /** "browser": the return and any acknowledgement are kept in this browser session only (fixtures mode). */
  kept?: "browser";
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
    kind: "crc",
    referrer: `${providerLabel(c.provider)} GP`,
    unit: "SKMC endoscopy",
    answerWhen: "Saved to the case record",
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
    acknowledged: !!(answered && returned?.acknowledgement),
    ...(answered?.note ? { note: answered.note } : {}),
    ...(answered?.has_note || answered?.note ? { hasNote: true } : {}),
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

/**
 * A CRC referral as a return kept in this browser shows it (fixtures mode): returned to the GP with the reason and note,
 * waiting on the GP until they acknowledge it on their view in this same browser. Nothing of it is in the case ledger.
 */
export function withSessionReturn(r: Referral, s: SessionReturn | undefined): Referral {
  if (!s || r.kind !== "crc" || r.state === "done") return r;
  return {
    ...r,
    state: "returned",
    handoff: { status: "awaiting_referrer", actBy: s.actBy },
    answer: `Returned to the GP: ${REASON_LABELS[s.reason].toLowerCase()}`,
    answerWhen: "Kept in this browser",
    acknowledged: !!s.acknowledgedAt,
    kept: "browser",
    ...(s.note ? { note: s.note, hasNote: true } : { note: undefined, hasNote: false }),
  };
}

function answerWords(a: NonNullable<NonNullable<CaseSummary["api_overlay"]>["referral_response"]>): string {
  if (a.response === "return") return `Returned to the GP: ${a.reason ? REASON_LABELS[a.reason].toLowerCase() : "no reason recorded"}`;
  if (a.response === "accept_with_conditions") return `Accepted with conditions: ${a.conditions.map((c) => CONDITION_LABELS[c]).join(", ")}`;
  return "Accepted the referral";
}

/** A lung work-up or DR referral the receiving unit returned, read from its canonical packet; no packet, no row. */
export function packetReferral(c: CaseSummary, current: CurrentDecision | null, now: number, hasRecord: boolean): Referral | null {
  const p = current?.handoff;
  if (c.closed || !p) return null;
  const person = personForLoop(c.id);
  const unit = p.decided_by ? unitName(p.decided_by) : UNAVAILABLE;
  const referrer = responsibleWords(p.responsible_clinician);
  const reason = p.reason_code ? (PACKET_REASONS[p.reason_code] ?? p.reason_code) : UNAVAILABLE;
  // The referrer's answers recorded through the API project onto this packet (api_overlay.referral_handoff, source
  // canonical); the canonical packet itself is never rewritten.
  const projected = c.api_overlay?.referral_handoff;
  const answered = projected?.source === "canonical" && projected.event === p.referral_id ? projected : undefined;
  const status = answered?.status ?? p.status;
  const r = answered?.referrer_response;
  return {
    id: c.id,
    kind: "packet",
    referrer,
    unit,
    short: shortId(c.id),
    name: person?.display_name ?? shortId(c.id),
    nameAr: person?.display_name_ar,
    age: person?.age,
    seed: referralSeed(c.id),
    clinic: unitName(c.provider),
    state: "returned",
    waiting: Math.floor(now - c.referral_day),
    sent: formatDay(c.referral_day),
    flags: [],
    hasRecord,
    urgent: p.clinical_priority === "urgent",
    handoff: { status, actBy: p.act_by },
    head: null,
    acknowledged: !!answered?.acknowledgement,
    answer: `Returned the referral: ${reason.toLowerCase()}`,
    answerWhen: `Returned ${formatDay(p.decided_at)}`,
    details: [
      ["Recommendation", p.recommendation ?? UNAVAILABLE],
      ["Returned by", unit],
      ["Reason", reason],
      ["Advice to the referrer", p.advice_to_referrer ?? UNAVAILABLE],
      ["Information requested", p.info_requested.length ? p.info_requested.map((i) => INFO_WORDS[i] ?? i).join("; ") : "None"],
      ["Other unit", p.suggested_alternative === undefined ? UNAVAILABLE : p.suggested_alternative ? unitName(p.suggested_alternative) : "None suggested"],
      ["Priority", p.clinical_priority === "urgent" ? "Urgent" : p.clinical_priority === "routine" ? "Routine" : UNAVAILABLE],
      ["Responsible clinician", referrer],
      ["Status", PACKET_STATUS[status]],
      ["Referrer acknowledged", answered?.acknowledgement ? formatDay(answered.acknowledgement.run_day) : "Not yet"],
      ["Referrer's answer", r ? referrerAnswerWords(r) : p.status === "referrer_acted" ? "Already acted" : "None yet"],
      ["Act by", formatDay(p.act_by)],
      ["Medical director step", p.medical_director_day === undefined ? UNAVAILABLE : formatDay(p.medical_director_day)],
      ["Referred", formatDay(c.referral_day)],
      ["Case", shortId(c.id)],
    ],
  };
}

/** The referrer's resolving answers to a returned packet, in words (pipeline/api_referrer.go). */
export const REFERRER_ANSWER_LABELS: Record<Exclude<ReferrerResponseCode, "acknowledge">, string> = {
  supply_info: "Send the information requested",
  re_refer: "Re-refer to the same unit",
  redirect: "Refer to another unit",
  manage_in_primary_care: "Manage in primary care",
  close: "Close the referral",
};

/** A unit or provider id in words. */
export const placeName = (id: string) => UNIT_NAMES[id] ?? providerLabel(id);

/** A resolving answer in words; a reason appears only where the projection carries the referrer's free text. */
export function referrerAnswerWords(r: { response: Exclude<ReferrerResponseCode, "acknowledge">; destination?: string; reason?: string; run_day: number }): string {
  return `${REFERRER_ANSWER_LABELS[r.response]}${r.destination ? `: ${placeName(r.destination)}` : ""}${r.reason ? `. Reason: ${r.reason}` : ""}. Answered ${formatDay(r.run_day)}`;
}

/** The packet's reason code in words, or the code itself when it has no words. */
export const packetReasonWords = (code: string) => PACKET_REASONS[code] ?? code;

/** A referral's place in the inbox, as one status a filter can name. */
export type ReferralStatus = "needs" | "report" | "waiting" | "acknowledged" | "acted" | "escalated" | "accepted" | "closed";
export type StatusTone = "pending" | "held" | "done";

/** The status words of a referral; `referrer` is who a return waits on ("GP" or "referrer"). */
export function referralStatus(r: Pick<Referral, "state" | "handoff" | "acknowledged">, referrer = "GP"): { key: ReferralStatus; label: string; tone: StatusTone } {
  switch (r.state) {
    case "new":
      return { key: "needs", label: "Needs response", tone: "pending" };
    case "grade":
      return { key: "report", label: "Report incomplete", tone: "pending" };
    case "accepted":
      return { key: "accepted", label: "Accepted", tone: "done" };
    case "done":
      return { key: "closed", label: "Closed", tone: "done" };
    case "returned": {
      const s = r.handoff?.status;
      if (s === "referrer_acted") return { key: "acted", label: `The ${referrer} has acted`, tone: "done" };
      if (s === "coordinator" || s === "medical_director") return { key: "escalated", label: "Escalated", tone: "held" };
      return r.acknowledged ? { key: "acknowledged", label: `Acknowledged by ${referrer}`, tone: "done" } : { key: "waiting", label: `Waiting on ${referrer}`, tone: "held" };
    }
  }
}

/** The inbox's status filter, in queue order. */
export const STATUS_FILTERS: readonly ReferralStatus[] = ["needs", "report", "waiting", "acknowledged", "escalated", "acted", "accepted", "closed"];
export function statusFilterLabel(key: ReferralStatus, referrer = "GP"): string {
  return {
    needs: "Needs response",
    report: "Report incomplete",
    waiting: `Waiting on ${referrer}`,
    acknowledged: `Acknowledged by ${referrer}`,
    escalated: "Escalated",
    acted: `The ${referrer} has acted`,
    accepted: "Accepted",
    closed: "Closed",
  }[key];
}
