import type { CaseDetail, CaseSummary, Dropoff, Programme } from "@/lib/api/schema";
import { formatDay } from "@/lib/domain/day";
import { shortId } from "@/lib/domain/ids";
import { compareInstants } from "@/lib/domain/instant";
import { caseStage } from "@/lib/domain/stage";
import { plain } from "@/lib/domain/terms";
import { personForLoop } from "@/lib/people";
import { referralSeed } from "@/lib/people/identity";
import type { PersonSeed } from "@/lib/people/types";
import type { ChainNode } from "@/components/wasl/sealed-chain";
import { atWords, ledgerWordsWithHistory } from "@/components/case/words";

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
  no_acknowledgement: { status: "Notified, hasn't confirmed", next: "Call to check they understood", view: "call" },
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
  if (c.closed) return { status: "Closed", next: "Nothing to do" };
  if (c.state === "procedure_done") return { status: "Procedure done, report pending", next: "Wait for the report" };
  if (c.dropoff === "") return { status: plain(c.state), next: "Check the record" };
  return DROPOFF_WORDS[c.dropoff];
}

/** Whole days since referral at run day `now`. */
export function elapsedDays(c: CaseSummary, now: number): number {
  return Math.floor(now - c.referral_day);
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

export const PROCEDURE: Record<Programme, string> = {
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
  overdue: boolean;
  referralDay: number;
  textsAllowed: boolean;
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
};

/** Today's status after an appointment was agreed: nothing is booked until a slot is confirmed. */
export const BOOKING_NEEDED = "Booking needed · date to confirm";
export const DECLINED_STATUS = "Declined · GP to sign";
export const noAnswerStatus = (n: number) => `No answer ×${n} · retry tomorrow`;

const OUTCOME_STATUS: Record<string, string> = { voicemail: "Left a voicemail · retry", wrong_number: "Wrong number · check the record" };
const outcomeStatus = (o: string) => OUTCOME_STATUS[o] ?? plain(o);

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
  const words = { ...statusWords(c) };
  const ov = c.api_overlay;
  const owner = ov?.owner || c.owner;
  let view: View = owner === "" ? "none" : c.dropoff === "" ? "waiting" : DROPOFF_WORDS[c.dropoff].view;
  if (ov?.owner === HANDOFF_OWNER && view === "call") view = "waiting";
  if (ov?.owner === HANDOFF_OWNER) words.status = `With ${HANDOFF_OWNER}`;
  const lc = ov?.last_contact;
  let sinks = false;
  let awaitingNext = false;
  if (lc) {
    if (lc.outcome === "appointment_agreed" || lc.outcome === "declined") {
      view = "waiting";
      words.status = lc.outcome === "declined" ? DECLINED_STATUS : BOOKING_NEEDED;
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
  const snoozeRunning = !!ov?.snooze && Date.parse(ov.snooze.until) > wallNow;
  const snoozeLater = snoozeRunning && (!lc || !ov!.snooze!.at || compareInstants(ov!.snooze!.at, lc.at) > 0);
  if (snoozeLater) {
    awaitingNext = false;
    words.status = ov!.snooze!.reason || `Snoozed until ${ov!.snooze!.until.slice(0, 10)}`;
  }
  const snoozed = snoozeRunning || sinks;
  // Reservation state has explicit precedence over contact outcomes and canonical no-show. A snooze may still sink
  // the row, but it never disguises an active synthetic reservation as an ordinary snoozed call.
  const booking = ov?.booking;
  if (booking?.status === "reserved") {
    awaitingNext = false;
    view = "waiting";
    words.status = `Simulated slot reserved · Day ${booking.slot?.day ?? "unknown"}`;
    words.next = "Manage the simulated reservation";
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
  if (c.recovery?.backfill) {
    awaitingNext = false;
    view = "waiting";
    words.status = "Simulated slot confirmed";
    words.next = "Wait for the simulated appointment day";
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
    days: elapsedDays(c, now),
    overdue: c.overdue,
    referralDay: c.referral_day,
    textsAllowed: c.consent === "granted",
    owner: ownerLabel(owner),
    review: c.review,
    dropoff: c.dropoff,
    hasRecord,
    ...(ov ? { head: ov.head, ledgerActions: ov.actions, snoozed, noAnswer: ov.no_answer_streak, awaitingNext, ledgerStatus: ov.ledger_status } : {}),
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
    out.push({ id: "closed", kind: "current", who: "Closed", what: `with evidence, dated to the ${PROCEDURE[c.case.programme]}`, when: `day ${d} · ${formatDay(c.case.closed_day)}`, hash: sealed("closure_decision") });
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
export function readerLabels(ev: CaseDetail["evidence"][number]): { checks: Check[]; quote: string } {
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
