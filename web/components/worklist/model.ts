import type { CaseDetail, CaseSummary, Dropoff, Programme } from "@/lib/api/schema";
import { effectiveOwner } from "@/lib/api/owner";
import { formatDay } from "@/lib/domain/day";
import { shortId } from "@/lib/domain/ids";
import { staffDueTime } from "@/lib/domain/staff-time";
import { compareInstants } from "@/lib/domain/instant";
import { caseStage } from "@/lib/domain/stage";
import { plain } from "@/lib/domain/terms";
import { personForLoop } from "@/lib/people";
import { referralSeed } from "@/lib/people/identity";
import type { PersonSeed } from "@/lib/people/types";
import type { ChainNode } from "@/components/wasl/sealed-chain";
import { atWords, communicationWords, dispositionWords, ledgerWordsWithHistory, reconciliationWords } from "@/components/case/words";
import { caseDeadlineDays, isNoAction, nextScreenDays, overdueClockDays } from "./days";
import { reviewDateWords } from "@/components/disposition/model";
import { isActionable, obligationWords, openObligation } from "./obligation";
import { patientContestWords, patientTaskOverdue } from "./patient-tasks";
import { languageWords } from "@/lib/domain/language";

/*
 * Pure view-models for the worklist screens (Today, Patients, Referrals).
 * The API has no wall clock: `now` is the run's clock, the latest `current.now` the API reports (see run-clock.ts).
 */

/** Whose move it is, in the nurse's words. */
export type View = "call" | "waiting" | "none";

type DropoffWords = { status: string; next: string; view: Exclude<View, "none"> };

/** One row per drop-off: the trade word on the nurse's list, her next to-do, and which tab it sits in. */
export const DROPOFF_WORDS: Record<Exclude<Dropoff, "">, DropoffWords> = {
  never_notified: { status: "Never notified", next: "Call to tell them the result", view: "call" },
  // Receipt, not comprehension: understanding is checked separately (teach-back), never by this call.
  no_acknowledgement: { status: "Notified, hasn't confirmed", next: "Call to confirm they received the result", view: "call" },
  not_booked: { status: "Not booked", next: "Arrange the colonoscopy", view: "call" },
  referral_rejected: { status: "Referral returned by the unit", next: "Call the GP to re-refer", view: "call" },
  referral_unanswered: { status: "Unit hasn't answered", next: "Chase the endoscopy unit", view: "call" },
  no_show: { status: "No-show, rebook", next: "Arrange a new date", view: "call" },
  no_followup_ct: { status: "No follow-up CT booked", next: "Arrange the CT", view: "call" },
  not_assessable: { status: "Not assessable", next: "Clinician to decide on a repeat", view: "waiting" },
  no_eye_exam: { status: "No eye-clinic exam yet", next: "Arrange the eye-clinic exam", view: "call" },
  ungradable_no_repeat: { status: "Retinal photo ungradable", next: "Arrange the repeat photo", view: "call" },
  technical_repeat_not_done: { status: "Mammogram to repeat", next: "Arrange the repeat mammogram", view: "call" },
  no_assessment_outcome: { status: "Waiting on the assessment outcome", next: "Wait for the assessment result", view: "waiting" },
  awaiting_tissue: { status: "Waiting on tissue sampling", next: "Specialist team to arrange it", view: "waiting" },
  inadequate: { status: "Report incomplete · with the doctors", next: "Doctors to decide on a repeat", view: "waiting" },
  awaiting_histology: { status: "Waiting on pathology", next: "Wait for the pathology result", view: "waiting" },
  in_progress: { status: "Appointment in the record", next: "Prep call before the appointment", view: "waiting" },
  unlinked: { status: "Report not matched yet", next: "Wait for the match", view: "waiting" },
  escalated: { status: "With the specialist team", next: "Specialist team to decide", view: "waiting" },
  out_of_scope: { status: "Clinician to decide", next: "Clinician to decide", view: "waiting" },
  incomplete_scan_not_completed: { status: "LDCT incomplete, repeat not done", next: "Arrange the repeat LDCT", view: "call" },
  followup_ldct_not_done: { status: "Follow-up LDCT not done", next: "Arrange the follow-up LDCT", view: "call" },
  no_workup_outcome: { status: "Waiting on the work-up outcome", next: "Wait for the work-up result", view: "waiting" },
  no_followup_documented: { status: "No follow-up visit documented", next: "Arrange the follow-up visit", view: "call" },
};

export function statusWords(c: CaseSummary): { status: string; next: string } {
  if (c.lifecycle === "retained") return { status: "Retired", next: "Authorized history only, no current care action" };
  if (c.crc) {
    if (c.crc.care_state === "ongoing") return { status: "Screening complete, care ongoing", next: "Track each confirmed care pathway" };
    if (c.crc.care_state === "awaiting_confirmation") return { status: "Care pathways need confirmation", next: "Review each obligation separately" };
    if (c.crc.care_state === "blocked") return { status: "Current evidence incomplete", next: "Resolve the evidence holds" };
  }
  /* A patient contest holds closure: the API serves closed:false with the canonical closure kept aside (closure_hold). */
  if (c.closure_hold === "patient_contest") return { status: "Closure held: patient contest open", next: "A senior clinician reviews the patient's contest" };
  if (c.closed) return { status: "Closed", next: "Nothing to do" };
  if (c.state === "procedure_done") return { status: "Procedure done, report pending", next: "Wait for the report" };
  if (c.dropoff === "") return { status: plain(c.state), next: "Check the record" };
  const plan = c.api_overlay?.clinician_disposition;
  if (plan) return { status: "Clinician plan recorded", next: `${PLAN_WORDS[plan.plan]} · ${plan.owner} to review by ${reviewDateWords(plan.review_date)}` };
  return DROPOFF_WORDS[c.dropoff];
}

/**
 * The case-page header phrase. "closed with evidence" only when the API serves the case as closed; a closure held by a
 * patient contest (closed:false, closure_hold) never reads as closed.
 */
export function headerStatusWords(c: CaseSummary): string {
  const status = statusWords(c).status;
  if (status === "Closed" && c.closed && !c.closure_hold) return isNoAction(c) ? "closed, no action needed" : "closed with evidence";
  if (status === "Closed") return "not closed: check the record";
  return status.toLowerCase();
}

const PLAN_WORDS: Record<NonNullable<NonNullable<CaseSummary["api_overlay"]>["clinician_disposition"]>["plan"], string> = {
  repeat_diagnostic: "Plan: repeat the test",
  alternative_diagnostic: "Plan: a different test",
  specialist_pathway: "Plan: specialist pathway",
  primary_care_pathway: "Plan: primary-care pathway",
};

/** Whole days since referral at run day `now`. */
export function elapsedDays(c: CaseSummary, now: number): number {
  return Math.floor(now - c.referral_day);
}

const NO_ACTION_WORDS: Record<string, string> = {
  no_followup_needed: "no routine follow-up needed",
  no_referral_needed: "no referral needed",
  no_recall_needed: "no recall needed",
};

/** What a no-action resolution says, with the next routine screen when the rule sets one. */
export function noActionWords(c: CaseSummary): string {
  const next = nextScreenDays(c);
  return `${NO_ACTION_WORDS[c.state] ?? "no action needed"}${next ? `, next screen in ${next} days` : ""}`;
}

/** The detail header for a closed case: the deadline comparison for a procedure closure, the resolution for a no-action one. */
export function closedHeadline(c: CaseSummary): string {
  const d = closedInDays(c);
  if (isNoAction(c)) return `Closed in ${d} days, ${noActionWords(c)}`;
  const due = caseDeadlineDays(c);
  const words = c.programme === "crc" ? `${due}-day standard` : c.ldct?.rec?.action === "annual" ? `${due}-day screening interval` : `${due}-day deadline`;
  return d <= due ? `Closed in ${d} days, ${due - d} inside the ${words}` : `Closed in ${d} days, ${d - due} past the ${words}`;
}

/**
 * The absolute run day of the next routine screen as the API serves it for a no-action resolution, or undefined when
 * the case does not carry one. The screening date is not the referral date, so it is never derived from `referral_day`.
 */
export function servedNextScreenDay(c: CaseSummary): number | undefined {
  switch (c.programme) {
    case "lung_ldct":
      return c.ldct?.due_day;
    case "diabetic_retinopathy":
      return c.dr?.next_due?.due_day ?? c.dr?.deadline_day;
    case "breast":
      return c.breast?.deadline_day;
    case "ifhas":
      return c.ifhas?.deadline_day;
    default:
      return undefined;
  }
}

/** The Patients due column: the procedure deadline, the next screen for a no-action resolution, or nothing due. */
export function dueColumnWords(c: CaseSummary): string {
  if (!isNoAction(c)) return formatDay(c.referral_day + caseDeadlineDays(c));
  if (!nextScreenDays(c)) return "None due";
  const due = servedNextScreenDay(c);
  return due === undefined ? "Next screen date unavailable" : `Next screen ${formatDay(due)}`;
}

export function closedInDays(c: CaseSummary): number {
  return Math.floor(c.closed_day - c.referral_day);
}

/** `Practitioner/ahs-mushrif-nurse-5` → `Nurse 5`. */
export function ownerLabel(owner: string): string {
  if (!owner) return "No owner";
  const nurse = /nurse-(\d+)$/.exec(owner);
  return nurse ? `Nurse ${nurse[1]}` : owner.slice(owner.lastIndexOf("/") + 1);
}

const PROVIDERS: Record<string, string> = {
  "ahs-mushrif": "Al Mushrif",
  "skmc-endo": "SKMC endoscopy",
  "noor-endo": "Noor endoscopy",
};

export function providerLabel(provider: string): string {
  return PROVIDERS[provider] ?? provider;
}

/** Programme names for filters and headings. */
export const PROGRAMME_NAMES: Record<Programme, string> = {
  crc: "Colorectal screening",
  cervical: "Cervical screening",
  lung_nodule: "Lung nodule follow-up",
  diabetic_retinopathy: "Diabetic eye screening",
  breast: "Breast screening",
  lung_ldct: "Lung cancer screening (LDCT)",
  ifhas: "IFHAS cardiometabolic follow-up",
};

export const PROCEDURE: Record<Programme | "", string> = {
  "": "procedure unavailable",
  crc: "colonoscopy",
  cervical: "colposcopy",
  lung_nodule: "follow-up CT",
  diabetic_retinopathy: "eye-clinic exam",
  breast: "breast assessment",
  lung_ldct: "follow-up LDCT",
  ifhas: "follow-up visit",
};

/** A case as the worklist shows it: name if the simulated cast has one (gap G1), else the short id. */
export type WorkItem = {
  id: string;
  short: string;
  name: string;
  nameAr?: string;
  age?: number;
  named: boolean;
  /** The referral patient, whose face the row shows. */
  seed: PersonSeed;
  view: View;
  status: string;
  next: string;
  days: number;
  /** Days from referral the case's own rule allows; null for a routine interval with no overdue clock. */
  deadline: number | null;
  overdue: boolean;
  referralDay: number;
  textsAllowed: boolean;
  /** The canonical contact permission as the case list serves it (`unknown` when none is recorded). */
  consent: CaseSummary["consent"];
  /** A recorded consent denial or opt-out: no new contact outcome may be logged, whatever the canonical consent. */
  contactBlocked: boolean;
  /** Patient denial/opt-out in the demo overlay makes simulated SMS unavailable immediately. */
  textsStopped?: boolean;
  patient?: CaseSummary["api_patient"];
  owner: string;
  review: CaseSummary["review"];
  dropoff: Dropoff;
  hasRecord: boolean;
  /** The case's `api_overlay.head` (live API only): what the next coordinator action must name as `expected_head`. */
  head?: string;
  /** Coordinator actions already on the API ledger for this case (live API only). */
  ledgerActions?: number;
  /** A running ledger snooze or a no-answer logged in the last day (live API only); these rows sink on Today. */
  snoozed?: boolean;
  /** Consecutive persisted no-answers ending the case's contact log (live API only). */
  noAnswer?: number;
  /** The latest applicable contact outcome is Reached, so Appointment agreed, Call back and Declined are still to choose. */
  awaitingNext?: boolean;
  /** `invalid`: ledger replay stopped part-way, so this row's overlay is only a prefix and not authoritative. */
  ledgerStatus?: "ok" | "invalid";
  /** An active simulated reservation (live API only). */
  reserved?: boolean;
  /** The simulated reservation was cancelled (live API only). */
  cancelled?: boolean;
  /** What the programme's procedure is called, e.g. "colonoscopy". */
  procedure?: string;
  /** A pending record retrieval after a completed-elsewhere report (live API only): chase and booking are suspended. */
  retrieval?: { owner: string; due: string; overdue: boolean };
  /** The case's programme and provider, so an all-programmes queue can say which each row is. */
  programme: Programme | "";
  programmeName: string;
  providerName: string;
  /** Recorded preferred language in words ("Arabic", "English"), or "Not recorded". */
  languageName: string;
  /**
   * The communication step this case still owes (issue #197): `notify` while the result was never told, `receipt` while
   * the patient has not confirmed receiving it. Recorded by explicit staff actions only, never by a contact outcome.
   */
  communicationStep?: "notify" | "receipt";
  /** The approved wording a staff notification must use, as the API serves it. */
  notificationTemplate?: string;
  /**
   * An open failed teach-back callback: its status words and the reference the patient's receipt shows (the first 8
   * characters of the task id). Kept apart from `status`, which a later local action may replace while the task stays open.
   */
  teachBackCallback?: { status: string; ref: string };
};

/** Today's words for a recorded staff communication (issue #197). Receipt never claims understanding. */
export const NOTIFIED_STATUS = "Result told · receipt not confirmed yet";
export const RECEIPT_STATUS = "Receipt confirmed · understanding not checked";

/** The words a served drop-off gives after a communication was recorded: the next obligation, as the API computed it. */
export function obligationAfterCommunication(dropoff: string): { status: string; next: string; view: View } | undefined {
  const w = (DROPOFF_WORDS as Record<string, DropoffWords | undefined>)[dropoff];
  return w ? { status: dropoff === "no_acknowledgement" ? NOTIFIED_STATUS : RECEIPT_STATUS, next: w.next, view: w.view } : undefined;
}

/** What happened to patient outreach for a confirmed recovery backfill; no message is ever dispatched. */
export const OUTREACH_WORDS: Record<NonNullable<NonNullable<CaseSummary["recovery"]>["backfill"]>["outreach_status"], string> = {
  none: "no outreach recorded",
  recorded_not_dispatched: "outreach recorded, not sent",
  cancelled_consent: "outreach cancelled after consent withdrawal",
};

/** An open failed teach-back callback outranks contact outcomes: logging a call never resolves it. */
export function teachBackCallbackWords(c: CaseSummary): string | null {
  const cb = c.teach_back_callback;
  if (!cb || cb.status !== "open") return null;
  if (!cb.due_at) return "Teach-back failed · nurse callback open";
  return `Teach-back failed · nurse callback ${cb.overdue ? "overdue since" : "due"} ${staffDueTime(cb.due_at)}`;
}

/** The reference a patient's teach-back callback receipt shows (components/patient/check-flow.tsx). */
export const callbackRef = (id: string) => id.slice(0, 8);

/** Today's status after an appointment was agreed: nothing is booked until a slot is confirmed. */
export const BOOKING_NEEDED = "Booking needed · date to confirm";
export const DECLINED_STATUS = "Declined · GP to sign";
export const DECLINED_RESERVED_NEXT = "GP to sign the decline · cancel the simulated reservation to release the slot";
/** A decline recorded after the reservation was cancelled needs the GP's sign-off, not another booking. */
export const DECLINED_CANCELLED_NEXT = "GP to sign the decline";
/** A signed decline with a reservation still held: the GP prompt is gone, the slot still needs releasing. */
export const SIGNED_RESERVED_NEXT = "Cancel the simulated reservation to release the slot";
export const noAnswerStatus = (n: number) => `No answer ×${n} · retry tomorrow`;

const OUTCOME_STATUS: Record<string, string> = { voicemail: "Left a voicemail · retry", wrong_number: "Wrong number · check the record" };
export const outcomeStatus = (o: string) => OUTCOME_STATUS[o] ?? plain(o);

/** Who "Hand to …" on Today hands a patient to; a ledger owner equal to this puts them in Waiting. */
export const HANDOFF_OWNER = "Dr Al Kaabi";

/**
 * `wallNow` (ms) is only used for ledger times: whether a snooze is still running (the API checks snooze `until`
 * against the server's wall clock, not the run clock) and whether a no-answer was logged within the last day.
 *
 * Live API: the board position comes from the persisted overlay, so a reload shows what the ledger says, not what one
 * browser session did. The latest contact outcome places the row the way logging it does on Today: an agreed
 * appointment or a decline moves it to Waiting, a no-answer sinks it for a day. A snooze logged after that outcome
 * sets the status instead.
 */
export function workItem(c: CaseSummary, now: number, hasRecord: boolean, wallNow = Date.now()): WorkItem {
  const person = personForLoop(c.id);
  const recall = openObligation(c, now);
  const due = isActionable(recall) ? recall : null;
  const words = due ? obligationWords(due) : { ...statusWords(c) };
  if (c.api_patient?.slot_selection.status === "selected") {
    words.status = "Patient picked a preferred time (simulated) · check it";
    words.next = "Review the preferred time. No appointment is reserved";
  }
  const patientBlocked = c.api_patient?.contact_permission.status === "denied" || c.api_patient?.contact_permission.status === "opted_out";
  const ov = c.api_overlay;
  const owner = effectiveOwner(c);
  let view: View = owner === "" ? "none" : c.dropoff === "" ? "waiting" : DROPOFF_WORDS[c.dropoff].view;
  if (due && view !== "none") view = "call";
  if (ov?.owner === HANDOFF_OWNER && view === "call") view = "waiting";
  if (ov?.owner === HANDOFF_OWNER) words.status = `With ${HANDOFF_OWNER}`;
  const lc = ov?.last_contact;
  // A clinician disposition answers only the decline it names; a newer decline is unsigned again.
  const signed = lc?.outcome === "declined" && !!ov?.decline_disposition && ov.decline_disposition.decline_event === lc.event ? dispositionWords(ov.decline_disposition) : undefined;
  let sinks = false;
  let awaitingNext = false;
  if (lc) {
    if (lc.outcome === "appointment_agreed" || lc.outcome === "declined") {
      view = "waiting";
      words.status = lc.outcome === "declined" ? (signed?.status ?? DECLINED_STATUS) : BOOKING_NEEDED;
      if (signed) words.next = signed.next;
    } else if (lc.outcome === "no_answer") {
      words.status = noAnswerStatus(Math.max(1, ov!.no_answer_streak));
      sinks = Date.parse(lc.at) + 86_400_000 > wallNow;
    } else if (lc.outcome === "reached") {
      words.status = `Reached · ${words.next}`;
      awaitingNext = true;
    } else {
      words.status = outcomeStatus(lc.outcome);
    }
  }
  // The validated shared projection folds patient AND staff consent in ledger order. api_patient is activity,
  // not current shared permission: a legitimate later staff grant can supersede a patient denial.
  // Older snapshots have no shared projection; only there retain the conservative patient-only fallback.
  const contactBlocked = c.contact_state ? c.contact_state.contact_blocked : patientBlocked;
  const textsStopped = contactBlocked && patientBlocked;
  const snoozeRunning = !!ov?.snooze && Date.parse(ov.snooze.until) > wallNow;
  const snoozeLater = snoozeRunning && (!lc || !ov!.snooze!.at || compareInstants(ov!.snooze!.at, lc.at) > 0);
  if (snoozeLater) {
    awaitingNext = false;
    words.status = ov!.snooze!.reason || `Snoozed until ${ov!.snooze!.until.slice(0, 10)}`;
  }
  const snoozed = snoozeLater || sinks;
  // Reservation state has explicit precedence over contact outcomes and canonical no-show. A snooze may still sink
  // the row, but it never disguises an active synthetic reservation as an ordinary snoozed call.
  const booking = ov?.booking;
  const declinedAfterBooking = lc?.outcome === "declined" && !!booking?.updated_at && compareInstants(lc.at, booking.updated_at) > 0;
  if (booking?.status === "reserved") {
    awaitingNext = false;
    if (declinedAfterBooking) {
      words.next = signed ? SIGNED_RESERVED_NEXT : DECLINED_RESERVED_NEXT;
    } else {
      view = "waiting";
      words.status = `Simulated slot reserved · Day ${booking.slot?.day ?? "unknown"}`;
      words.next = "Manage the simulated reservation";
    }
  } else if (booking?.status === "cancelled" && declinedAfterBooking) {
    awaitingNext = false;
    words.next = signed?.next ?? DECLINED_CANCELLED_NEXT;
  } else if (booking?.status === "cancelled") {
    const agreement = ov?.last_appointment_agreement;
    const afterAgreement = !agreement || (!!booking.updated_at && compareInstants(booking.updated_at, agreement.at) >= 0);
    if (afterAgreement) {
      awaitingNext = false;
      view = "call";
      words.status = "Simulated reservation cancelled · rebook";
      words.next = "Choose another simulated slot";
    }
  }
  const reconciliation = ov?.reconciliation?.status === "pending" ? ov.reconciliation : undefined;
  const retrieval = reconciliation && reconciliationWords(reconciliation, new Date(wallNow).toISOString().slice(0, 10));
  if (retrieval) {
    awaitingNext = false;
    view = "waiting";
    words.status = retrieval.status;
    words.next = retrieval.next;
  }
  const backfill = c.recovery?.backfill;
  if (backfill) {
    const contactable = c.consent === "granted" && !contactBlocked;
    awaitingNext = false;
    words.status = `Simulated slot confirmed · ${OUTREACH_WORDS[backfill.outreach_status]}`;
    if (backfill.outreach_status === "recorded_not_dispatched") {
      view = "waiting";
      words.next = "Wait for the simulated appointment day";
    } else if (backfill.outreach_status === "none" && contactable) {
      view = "call";
      words.next = `Contact the patient about the simulated recovery slot on day ${backfill.day}`;
    } else {
      view = "waiting";
      words.next = "Patient contact is not permitted, so the simulated slot was not offered";
    }
  }
  const communication = ov?.communication;
  const communicationStep = c.dropoff === "never_notified" ? ("notify" as const) : c.dropoff === "no_acknowledgement" ? ("receipt" as const) : undefined;
  if (communication && !lc && !snoozeLater) {
    words.status = communication.receipt ? RECEIPT_STATUS : NOTIFIED_STATUS;
  }
  const callback = teachBackCallbackWords(c);
  if (callback) {
    awaitingNext = false;
    view = "call";
    words.status = callback;
    words.next = "Call to explain the next step, then record the consultation outcome on the case page";
  }
  const contest = callback ? null : patientContestWords(c);
  if (contest) {
    awaitingNext = false;
    view = "waiting";
    words.status = contest.status;
    words.next = contest.next;
  }
  return {
    id: c.id,
    short: shortId(c.id),
    name: person?.display_name ?? shortId(c.id),
    nameAr: person?.display_name_ar,
    age: person?.age,
    named: !!person,
    seed: referralSeed(c.id),
    view,
    status: words.status,
    next: words.next,
    days: due ? Math.floor(now - due.fromDay) : elapsedDays(c, now),
    deadline: due ? Math.round(due.dueDay - due.fromDay) : overdueClockDays(c),
    overdue: (due ? due.state === "overdue" : c.overdue) || patientTaskOverdue(c),
    referralDay: c.referral_day,
    procedure: PROCEDURE[c.programme],
    programme: c.programme,
    programmeName: c.programme ? PROGRAMME_NAMES[c.programme] : "Programme unavailable",
    providerName: providerLabel(c.provider),
    languageName: languageWords(c.language),
    ...(communicationStep ? { communicationStep } : {}),
    ...(communicationStep === "notify" && c.notification_template ? { notificationTemplate: c.notification_template } : {}),
    textsAllowed: c.consent === "granted" && !contactBlocked,
    consent: c.consent,
    contactBlocked,
    textsStopped,
    patient: c.api_patient,
    owner: ownerLabel(owner),
    review: c.review,
    dropoff: c.dropoff,
    hasRecord,
    ...(ov ? { head: ov.head, ledgerActions: ov.actions, snoozed, noAnswer: ov.no_answer_streak, awaitingNext, ledgerStatus: ov.ledger_status, reserved: booking?.status === "reserved", cancelled: booking?.status === "cancelled" } : {}),
    ...(reconciliation && retrieval ? { retrieval: { owner: reconciliation.retrieval_owner, due: reconciliation.retrieval_due, overdue: retrieval.overdue } } : {}),
    ...(callback && c.teach_back_callback ? { teachBackCallback: { status: callback, ref: callbackRef(c.teach_back_callback.id) } } : {}),
  };
}

/* ---------- Patient record ---------- */

/**
 * The linker's probability for a link. Once reviewers confirm a below-line link its edge becomes `human` and
 * `path_p_cal` reads 1, so the linker's own score is the one its link review item carries.
 */
export function linkScore(c: CaseDetail, l: CaseDetail["links"][number]): number {
  if (l.weakest_edge.basis !== "human") return l.path_p_cal;
  return c.review_items.find((r) => r.kinds.includes("link") && r.loop === l.loop)?.p ?? l.path_p_cal;
}

const SIGNOFF_ROLE: Record<string, string> = { reviewer: "Reviewer", clinician: "Clinician", coordinator: "Coordinator" };

type Timed = ChainNode & { t: number; order: number };

function eventNode(e: CaseDetail["timeline"][number], c: CaseDetail, first: string): Timed | null {
  const unit = providerLabel(e.provider);
  const t = e.arrival_day ?? e.day;
  const base = { id: `${e.id}@${e.seq ?? t}`, t, order: 0, when: "" };
  const procedure = PROCEDURE[c.case.programme];
  switch (e.type) {
    case "ServiceRequest":
      if (e.id === c.case.id) return { ...base, kind: "human", who: `${unit} GP`, what: `referred ${first} for ${procedure}` };
      return { ...base, kind: "system", who: unit, what: "received the referral" };
    case "Consent":
      return { ...base, kind: "human", who: first, what: e.status === "permit" ? "allowed text messages" : "declined text messages" };
    case "Appointment":
      if (e.status === "booked") return { ...base, kind: "system", who: unit, what: `booked the ${procedure} for ${formatDay(e.day)}` };
      if (e.status === "fulfilled") return { ...base, t: e.day, kind: "system", who: unit, what: `${procedure} done` };
      if (e.status === "noshow") return { ...base, t: e.day, kind: "system", who: unit, what: "recorded a no-show" };
      return { ...base, kind: "system", who: unit, what: `appointment ${e.status ?? ""}`.trim() };
    case "DiagnosticReport":
      return { ...base, kind: "system", who: unit, what: `sent report ${shortId(e.id)}` };
    case "Encounter":
      return { ...base, kind: "system", who: unit, what: "clinic visit" };
    default:
      return null;
  }
}

/** The case, step by step: source events, Wasl's sealed decisions with their audit hashes, then where it rests now. */
export function recordNodes(c: CaseDetail, chain: { type: string; hash: string }[]): ChainNode[] {
  const person = personForLoop(c.case.id);
  const first = person?.display_name.split(" ")[0] ?? "the patient";
  const sealed = (type: string, n = 0) => chain.filter((e) => e.type === type)[n]?.hash;
  const reportDay = (report: string) => {
    const short = shortId(report);
    return Math.max(...c.timeline.filter((e) => shortId(e.id) === short).map((e) => e.arrival_day ?? e.day), c.case.referral_day);
  };

  const nodes: Timed[] = c.timeline.map((e) => eventNode(e, c, first)).filter((n): n is Timed => n !== null);

  c.links.forEach((l, i) => {
    const p = linkScore(c, l).toFixed(3);
    const by =
      l.weakest_edge.basis === "eid"
        ? "by Emirates ID"
        : l.weakest_edge.basis === "human"
          ? `by name and details, p ${p} before review, confirmed in review`
          : `by name and details, p ${p}`;
    nodes.push({ id: `link-${i}`, kind: "system", who: "Wasl", what: `matched report ${shortId(l.downstream_event)} to ${first} ${by}`, when: "", t: reportDay(l.downstream_event), order: 1, hash: sealed("link_decision", i) });
  });
  c.evidence.forEach((ev, i) => {
    const what = ev.agree ? `agree: ${ev.label}` : "disagree on the report";
    nodes.push({ id: `evidence-${i}`, kind: "system", who: "Two readers", what, when: "", t: reportDay(ev.report), order: 2, hash: sealed("evidence", i) });
  });
  c.holds.forEach((h, i) => {
    const what = h.report ? "held: the report does not show what closure needs" : `held: ${(h.reason ?? h.state).replaceAll("_", " ")}`;
    const t = h.report ? reportDay(h.report) : c.case.referral_day;
    nodes.push({ id: `hold-${i}`, kind: "system", who: "Safety gate", what, when: "", t, order: 3, hash: sealed("hold_decision", i) });
  });
  const lastT = Math.max(...nodes.map((n) => n.t));
  c.signoffs.forEach((s, i) => {
    const what = s.link ? `signed: ${s.link === "same" ? "same patient" : "not the same patient"}` : `signed: evidence ${s.evidence}`;
    nodes.push({ id: `signoff-${i}`, kind: "human", who: SIGNOFF_ROLE[s.role] ?? s.role, what: `${what} (simulated reviewer)`, when: "in review", t: lastT, order: 4 + i, hash: s.kinds.includes("evidence") ? sealed("human_signoff") : undefined });
  });
  // Coordinator reservations join the human case chain in ledger order. They never enter the canonical source timeline.
  const coordinatorEvents = c.api_reviews.filter((e) => e.ledger === "coordinator");
  coordinatorEvents.forEach((e, i) => {
    if (e.kind === "initial_notification" || e.kind === "receipt_acknowledgement") {
      // Staff-recorded communication facts (#197) join the human chain; they are never clinical evidence.
      nodes.push({ id: `communication-${e.hash}`, kind: "human", who: e.actor || "Coordinator", what: communicationWords(e.kind, e.data), when: atWords(e.at), t: lastT, order: 1000 + i, hash: e.hash });
      return;
    }
    if (!e.kind.startsWith("booking_")) return;
    const reason = e.data.reason ? `: “${e.data.reason}”` : "";
    nodes.push({
      id: `booking-${e.hash}`, kind: "human", who: e.actor || "Coordinator",
      what: `${ledgerWordsWithHistory(e, coordinatorEvents.slice(0, i))}${reason}`,
      when: atWords(e.at), t: lastT, order: 1000 + i, hash: e.hash,
    });
  });

  const sorted = nodes.sort((a, b) => a.t - b.t || a.order - b.order);
  const dayOf = (t: number) => Math.floor(t - c.case.referral_day);
  const out: ChainNode[] = sorted.map((n) => ({
    id: n.id,
    kind: n.kind,
    who: n.who,
    what: n.what,
    hash: n.hash,
    when: n.when || (dayOf(n.t) >= 0 ? `day ${dayOf(n.t)} · ${formatDay(n.t)}` : formatDay(n.t)),
  }));

  if (c.case.closed) {
    const d = closedInDays(c.case);
    const what = isNoAction(c.case) ? noActionWords(c.case) : `with evidence, dated to the ${PROCEDURE[c.case.programme]}`;
    out.push({ id: "closed", kind: "current", who: "Closed", what, when: `day ${d} · ${formatDay(c.case.closed_day)}`, hash: sealed("closure_decision") });
  } else {
    out.push({ id: "now", kind: "current", who: "Now", what: caseStage(c).label, when: statusWords(c.case).next });
  }
  return out;
}

export type Check = { name: string; result: string; differ: boolean };

const FIELD = {
  yes: "Yes",
  no: "No",
  not_stated: "Not stated",
  adequate: "Adequate",
  inadequate: "Not adequate",
} as Record<string, string>;
const word = (v: string | undefined) => (v ? (FIELD[v] ?? v) : "Not read");

/** Lung, DR and breast evidence: each reader's label, and the first reader's quote. */
export function readerLabels(ev: Pick<CaseDetail["evidence"][number], "reader_a_regex" | "reader_a" | "reader_b" | "label" | "quote" | "reader_a_fields">): { checks: Check[]; quote: string } {
  const a = ev.reader_a_regex ?? ev.reader_a ?? ev.label;
  const b = ev.reader_b ?? ev.label;
  const words = (v: string) => v.replaceAll("_", " ");
  return {
    checks: [
      { name: "Reader A", result: words(a), differ: a !== b },
      { name: "Reader B", result: words(b), differ: a !== b },
    ],
    quote: ev.quote || (typeof ev.reader_a_fields === "object" ? (ev.reader_a_fields.quote ?? "") : ""),
  };
}

/** The DoH closure checklist from the two readers' fields: caecum / prep / tissue taken / histology present. */
export function closureChecks(ev: CaseDetail["evidence"][number]): Check[] {
  const f = ev.fields ?? {};
  const reader = String(f.reader ?? "").split("/");
  const regex = String(f.regex ?? "").split("/");
  const row = (i: number) => ({ a: reader[i], b: regex[i], differ: reader[i] !== regex[i] });
  const caecum = row(0);
  const prep = row(1);
  const tissue = row(2);
  const histo = row(3);
  const histology =
    tissue.a === "no" ? "Not needed: no tissue taken" : tissue.a === "yes" ? (histo.a === "yes" ? "Present" : "Missing") : word(tissue.a);
  return [
    { name: "Caecum reached", result: word(caecum.a), differ: caecum.differ },
    { name: "Prep graded", result: word(prep.a), differ: prep.differ },
    { name: "Histology, if tissue taken", result: histology, differ: tissue.differ || histo.differ },
  ];
}
