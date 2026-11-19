import type { Audit, CaseDetail, Programme } from "@/lib/api/schema";
import { dayToDate, formatDayYear } from "@/lib/domain/day";
import { isNonbindingLegacyHistory } from "@/lib/domain/review-authority";
import { protocolRead, READ_LABEL } from "@/lib/domain/stage";
import { plain } from "@/lib/domain/terms";
import { ownerLabel, providerLabel } from "@/components/worklist/model";
import { eventWords, ledgerWordsWithHistory, reviewWords, roleWords } from "./words";

/*
 * The audit page's event history and decision inspector, built only from what the case and audit endpoints return.
 * Nothing here is invented: an event without a date goes under "Date unrecorded", and a signature whose decision the
 * case does not list says so instead of borrowing a label.
 */

type Timeline = CaseDetail["timeline"][number];
type LedgerRow = Audit["history"][number];
type Decision = CaseDetail["decisions"][number];

/** Picks the event mark. One icon per kind, chosen in the view. */
export type AuditEventKind =
  | "referral"
  | "intake"
  | "consent"
  | "booked"
  | "procedure"
  | "report"
  | "visit"
  | "source"
  | "review"
  | "call"
  | "owner"
  | "booking"
  | "coordinator"
  | "patient"
  | "compensation"
  | "claim"
  | "unknown";

export type AuditEvent = {
  key: string;
  /** UTC calendar day, `YYYY-MM-DD`; null when the record has no usable date. */
  date: string | null;
  /** Milliseconds for ordering; ties keep the order the API gave. */
  at: number;
  order: number;
  kind: AuditEventKind;
  label: string;
  /** Who or where the event came from: the provider, or the role on a ledger event. */
  source: string;
  /** For a ledger event: its index in `audit.history`, so the page can render its detail. */
  ledgerIndex?: number;
  /** For a source event: one line of record detail. */
  note?: string;
};

export const UNDATED = "Date unrecorded";

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
const MONTH_NAMES = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];

const isoDay = (d: Date) => d.toISOString().slice(0, 10);

/** `2026-07-18` → `18 Jul 2026`. */
export function dayLabel(date: string): string {
  const [y, m, d] = date.split("-").map(Number);
  return `${d} ${MONTHS[m - 1]} ${y}`;
}

/** `2026-07-18` → `July 2026`. */
export function monthLabel(date: string): string {
  const [y, m] = date.split("-").map(Number);
  return `${MONTH_NAMES[m - 1]} ${y}`;
}

/** A source record's short name for the history row. */
export function sourceEventLabel(e: Timeline, caseId: string): { label: string; kind: AuditEventKind } {
  const status = e.status ?? "";
  switch (e.type) {
    case "ServiceRequest":
      return e.id === caseId ? { label: "Referral issued", kind: "referral" } : { label: "Referral received", kind: "intake" };
    case "Consent":
      return status === "permit" ? { label: "Text contact allowed", kind: "consent" } : { label: `Consent ${plain(status || "recorded").toLowerCase()}`, kind: "consent" };
    case "Appointment":
      if (status === "booked") return { label: "Appointment booked", kind: "booked" };
      if (status === "fulfilled") return { label: "Procedure recorded", kind: "procedure" };
      return { label: `Appointment ${plain(status || "recorded").toLowerCase()}`, kind: "booked" };
    case "DiagnosticReport":
      return { label: "Report issued", kind: "report" };
    case "Encounter":
      return { label: "Clinic visit", kind: "visit" };
    default:
      return { label: eventWords(e, caseId), kind: "source" };
  }
}

export function timelineEvents(timeline: Timeline[], caseId: string): AuditEvent[] {
  return timeline.map((e, i) => {
    const valid = Number.isFinite(e.day);
    const when = valid ? dayToDate(e.day) : null;
    const { label, kind } = sourceEventLabel(e, caseId);
    const arrived = typeof e.arrival_day === "number" && e.arrival_day !== e.day ? ` · arrived ${formatDayYear(e.arrival_day)}` : "";
    return {
      key: `src-${e.id}@${e.seq ?? i}`,
      date: when ? isoDay(when) : null,
      at: when ? when.getTime() : 0,
      order: e.seq ?? i,
      kind,
      label,
      source: providerLabel(e.provider),
      note: `${eventWords(e, caseId)} · record ${e.id.slice(e.id.lastIndexOf("/") + 1)}${arrived}`,
    };
  });
}

const COORDINATOR_KIND: Record<string, AuditEventKind> = {
  contact_outcome: "call",
  assign_owner: "owner",
  booking_accept: "booking",
  booking_choose_slot: "booking",
  booking_reschedule: "booking",
  booking_cancel: "booking",
};

/** The ledger event's name, the same words the review history uses. */
export function ledgerLabel(r: LedgerRow, prior: LedgerRow[], programme: Programme | ""): string {
  if (r.ledger === "review") {
    const read = protocolRead(r);
    const base = isNonbindingLegacyHistory(r) ? "Legacy history — nonbinding" : r.decision ? reviewWords(r.decision, programme) : roleWords(r.action);
    return read ? `${READ_LABEL[read]}: ${base}` : base;
  }
  return ledgerWordsWithHistory(r, prior);
}

export function ledgerEvents(history: LedgerRow[], programme: Programme | ""): AuditEvent[] {
  return history.map((r, i) => {
    const t = r.at ? Date.parse(r.at) : Number.NaN;
    const ok = Number.isFinite(t);
    const kind: AuditEventKind =
      r.ledger === "coordinator" ? (COORDINATOR_KIND[r.kind] ?? "coordinator") : r.ledger;
    return {
      key: `ledger-${r.hash || i}`,
      date: ok ? isoDay(new Date(t)) : null,
      at: ok ? t : 0,
      order: i,
      kind,
      label: ledgerLabel(r, history.slice(0, i), programme),
      source: r.role ? roleWords(r.role) : "Role not recorded",
      ledgerIndex: i,
    };
  });
}

export type EventGroup = { name: string; events: AuditEvent[] };

/** Month groups, newest first by default; "Date unrecorded" always comes last. */
export function groupByMonth(events: AuditEvent[], oldestFirst = false): EventGroup[] {
  const dated = events.filter((e) => e.date).sort((a, b) => b.at - a.at || b.order - a.order);
  const undated = events.filter((e) => !e.date);
  const groups = new Map<string, AuditEvent[]>();
  for (const e of dated) {
    const name = monthLabel(e.date!);
    groups.set(name, [...(groups.get(name) ?? []), e]);
  }
  const shown = [...groups].map(([name, rows]) => ({ name, events: oldestFirst ? rows.slice().reverse() : rows }));
  if (oldestFirst) shown.reverse();
  if (undated.length) shown.push({ name: UNDATED, events: oldestFirst ? undated : undated.slice().reverse() });
  return shown;
}

/* ---- Decision signatures ---- */

export type DecisionView = {
  key: string;
  index: number;
  label: string;
  explanation: string;
  matched: boolean;
  stored: string;
  recomputed: string;
  decisionId?: string;
  policy: string;
};

const ESCALATION_WORDS: Record<string, string> = {
  overdue_crc_deadline: "Booking overdue",
  referral_unanswered_7d: "Referral unanswered for 7 days",
  no_ack_7d: "No acknowledgement in 7 days",
  open_econsult: "eConsult opened",
  unowned: "No owner assigned",
  noncompliant_message: "Message flagged as noncompliant",
  rejected_inadequate: "Report rejected as not adequate",
  econsult_re_refer: "eConsult to re-refer",
  prep_grade_unanswered_5wd: "Prep grade unanswered in 5 working days",
  prep_ungraded_clinician_decision: "Ungraded prep sent to a clinician",
};
const escalationWords = (code: string) => ESCALATION_WORDS[code] ?? plain(code);

const STATE_LABEL: Record<string, string> = {
  closed: "Case closed",
  booked: "Appointment booked",
  procedure_done: "Procedure recorded",
  inadequate: "Report not adequate",
  no_show: "Did not attend",
  referral_rejected: "Referral rejected",
  escalated: "Escalated",
  review: "Sent to review",
};
const stateWords = (state: string) => plain(state).toLowerCase();

/** `…@tick-d33-p0` → 33: the day of a scheduled check. */
function tickDay(id: string | undefined): number | undefined {
  const m = id ? /@tick-d(\d+)(?:-p\d+)?$/.exec(id) : null;
  return m ? Number(m[1]) : undefined;
}

/** A short name for a decision, from the record that triggered it or from what changed against the one before. */
export function decisionLabel(dec: Decision, prev: Decision | undefined, trigger: Timeline | undefined, caseId: string): string {
  if (trigger) {
    const { label, kind } = sourceEventLabel(trigger, caseId);
    if (kind === "referral") return "Referral opened";
    if (kind === "consent" && trigger.status === "permit") return "Contact permitted";
    return label;
  }
  const fresh = dec.escalations.filter((e) => !prev?.escalations.includes(e));
  if (dec.overdue && !prev?.overdue) return "Booking overdue";
  if (prev && dec.state !== prev.state) return STATE_LABEL[dec.state] ?? plain(dec.state);
  if (fresh.length) return escalationWords(fresh[0]);
  if (prev && dec.owner && !prev.owner) return /nurse-\d+$/.test(dec.owner) ? "Nurse assigned" : "Owner assigned";
  if (prev && dec.owner !== prev.owner) return "Owner changed";
  if (dec.consent_log.length > (prev?.consent_log.length ?? 0)) return "Consent changed";
  if (tickDay(dec.decision_id) !== undefined) return "Scheduled check";
  return prev ? "Decision recorded" : "Case opened";
}

/** What the decision recorded, in plain sentences built from its fields. */
export function decisionExplanation(dec: Decision, prev: Decision | undefined, trigger: Timeline | undefined, caseId: string): string {
  const day = tickDay(dec.decision_id);
  const parts: string[] = [];
  if (trigger) parts.push(`Made on the source record "${sourceEventLabel(trigger, caseId).label.toLowerCase()}" from ${providerLabel(trigger.provider)}, ${formatDayYear(trigger.day)}.`);
  else if (day !== undefined) parts.push(`Made at the scheduled check on ${formatDayYear(day)}.`);
  else parts.push("Made without a source record in the case timeline.");
  parts.push(`Case state: ${stateWords(dec.state)}.`);
  parts.push(dec.owner ? `Owner: ${ownerLabel(dec.owner)}.` : "No owner yet.");
  if (dec.overdue) parts.push("The follow-up window had passed.");
  const fresh = dec.escalations.filter((e) => !prev?.escalations.includes(e));
  if (fresh.length) parts.push(`Raised: ${fresh.map((e) => escalationWords(e).toLowerCase()).join("; ")}.`);
  return parts.join(" ");
}

/**
 * Every returned signature with a label and explanation. A signature is matched to the case's decision by its id;
 * when the audit leaves the id out or the case does not list it, the label is "Decision N" and the explanation says why.
 */
export function decisionViews(audit: Pick<Audit, "decision_signatures">, detail: Pick<CaseDetail, "decisions" | "timeline" | "case">): DecisionView[] {
  const byId = new Map(detail.decisions.map((d, i) => [d.decision_id, i]));
  const bySeq = new Map(detail.timeline.filter((e) => typeof e.seq === "number").map((e) => [e.seq!, e]));
  return audit.decision_signatures.map((s, i) => {
    const at = s.decision_id ? byId.get(s.decision_id) : undefined;
    const base = { key: s.decision_id ?? `decision-${i}`, index: i, matched: s.sig === s.recomputed, stored: s.sig, recomputed: s.recomputed, decisionId: s.decision_id, policy: s.policy };
    if (at === undefined) {
      return {
        ...base,
        label: `Decision ${i + 1}`,
        explanation: s.decision_id
          ? "The case record does not list this decision, so only its signature can be shown."
          : "The audit does not say which decision this signature belongs to, so only its signature can be shown.",
      };
    }
    const dec = detail.decisions[at];
    const prev = at > 0 ? detail.decisions[at - 1] : undefined;
    const trigger = bySeq.get(dec.in_seq);
    return { ...base, label: decisionLabel(dec, prev, trigger, detail.case.id), explanation: decisionExplanation(dec, prev, trigger, detail.case.id) };
  });
}

/** The matched count against the total; "none" when there is nothing to verify, never a pass. */
export function signatureTally(sigs: Audit["decision_signatures"]): { matched: number; total: number; state: "none" | "all" | "some" } {
  const matched = sigs.filter((s) => s.sig === s.recomputed).length;
  return { matched, total: sigs.length, state: sigs.length === 0 ? "none" : matched === sigs.length ? "all" : "some" };
}
