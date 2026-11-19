import { z } from "zod";
import type { CaseDetail } from "@/lib/api/schema";
import { caseHolds } from "@/lib/domain/holds";
import { shortId } from "@/lib/domain/ids";
import { openReviewItems } from "@/lib/domain/stage";
import { timelineEvents, type AuditEventKind } from "@/components/case/audit-model";
import { eventWords, holdWords } from "@/components/case/words";
import { providerLabel } from "./model";

/*
 * The open case's Activity and Records on Today, built on the server from ONE case's detail (GET /app/today/records).
 * Nothing is added: a source is a timeline record the API returned, an event uses the Audit page's own words for it,
 * and a source is held only for a reason the detail states (a current hold, an open review item or a reader split).
 */

export const TodaySource = z.object({
  /** The source record id, as the API gave it. */
  id: z.string(),
  /** What the record is, e.g. "Report, colonoscopy" or "Appointment, booked". */
  title: z.string(),
  /** The record type in a word: Referral, Report, Appointment… */
  type: z.string(),
  provider: z.string(),
  /** Simulation day of the record's latest entry; null when the record is only named by a hold or review item. */
  day: z.number().nullable(),
  /** Why this record is held, one sentence each; empty when it is not held. */
  held: z.array(z.string()),
});
export type TodaySource = z.infer<typeof TodaySource>;

export const TodaySourceEvent = z.object({
  key: z.string(),
  label: z.string(),
  /** UTC calendar day, `YYYY-MM-DD`; null when the record has no usable date. */
  date: z.string().nullable(),
  kind: z.string(),
  note: z.string(),
});
export type TodaySourceEvent = z.infer<typeof TodaySourceEvent>;

export const TodayRecords = z.object({
  sources: z.array(TodaySource),
  /** Holds on the case that name no source record (e.g. a report that has not arrived). */
  caseHeld: z.array(z.string()),
  /** Source events, earliest first. */
  events: z.array(TodaySourceEvent),
});
export type TodayRecords = z.infer<typeof TodayRecords>;

/** How many things are held: each held source record, plus each hold that names none. */
export const heldCount = (r: TodayRecords) => r.sources.filter((s) => s.held.length > 0).length + r.caseHeld.length;

const TYPE_WORDS: Record<string, string> = {
  ServiceRequest: "Referral",
  Consent: "Consent",
  Appointment: "Appointment",
  DiagnosticReport: "Report",
  Encounter: "Clinic visit",
};

const IDENTITY_SECOND = "Identity in review: no second identifier is confirmed for the two records.";
const IDENTITY = "Identity in review: a person must confirm the two records are the same patient.";
const EVIDENCE = "Evidence in review: a person must check what the report shows.";
const SPLIT = "The two readers disagree.";

const str = (v: unknown) => (typeof v === "string" && v ? v : undefined);

/** The source record a review item is about: its report, else the event its chain names. */
function reviewTarget(r: CaseDetail["review_items"][number]): string | undefined {
  if (r.report) return r.report;
  for (const e of r.chain) {
    const target = str(e.body.event) ?? str(e.body.downstream_event);
    if (target) return target;
  }
  return undefined;
}

export function todayRecords(d: CaseDetail): TodayRecords {
  const caseId = d.case.id;
  const reasons = new Map<string, string[]>();
  const caseHeld: string[] = [];
  // Review items may name a report by its short id; the timeline and evidence carry the full one.
  const known = [...d.timeline.map((e) => e.id), ...d.evidence.map((e) => e.report)];
  const full = (id: string | undefined) => (id && !id.includes("/") ? (known.find((k) => shortId(k) === id) ?? id) : id);
  const hold = (raw: string | undefined, why: string) => {
    const id = full(raw);
    if (!id) {
      if (!caseHeld.includes(why)) caseHeld.push(why);
      return;
    }
    const list = reasons.get(id) ?? [];
    if (!list.includes(why)) list.push(why);
    reasons.set(id, list);
  };
  // A closed case holds nothing now; its holds and review items are history (the Audit trail has them).
  if (!d.case.closed) {
    for (const h of caseHolds(d).current) hold(h.report, holdWords(h));
    const open = openReviewItems(d);
    for (const r of open) {
      if (r.kinds.includes("link")) hold(reviewTarget(r), r.reason === "second_identifier" ? IDENTITY_SECOND : IDENTITY);
      if (r.kinds.includes("evidence")) hold(reviewTarget(r), EVIDENCE);
    }
    // A reader split is its own reason only where no evidence review covers the report: an open one says so above,
    // and a signed one has settled it.
    const reviewed = new Set(d.review_items.filter((r) => r.kinds.includes("evidence")).map((r) => full(reviewTarget(r))));
    for (const ev of d.evidence) if (!ev.agree && !ev.human_override && !reviewed.has(ev.report)) hold(ev.report, SPLIT);
  }

  // One source per record: its latest entry says where it stands (an appointment booked, then fulfilled).
  const latest = new Map<string, CaseDetail["timeline"][number]>();
  for (const e of d.timeline) {
    const prev = latest.get(e.id);
    if (!prev || (e.seq ?? 0) >= (prev.seq ?? 0)) latest.set(e.id, e);
  }
  const sources: TodaySource[] = [...latest.values()].map((e) => ({
    id: e.id,
    title: eventWords(e, caseId),
    type: TYPE_WORDS[e.type] ?? e.type,
    provider: providerLabel(e.provider),
    day: Number.isFinite(e.day) ? e.day : null,
    held: reasons.get(e.id) ?? [],
  }));
  // A held record the timeline does not carry is still shown, named by its id.
  for (const [id, held] of reasons) {
    if (latest.has(id)) continue;
    const type = id.split("/")[1] ?? "";
    sources.push({ id, title: `${TYPE_WORDS[type] ?? "Record"} ${shortId(id)}`, type: TYPE_WORDS[type] ?? "Record", provider: providerLabel(id.split("/")[0]), day: null, held });
  }
  sources.sort((a, b) => Number(b.held.length > 0) - Number(a.held.length > 0) || (b.day ?? -Infinity) - (a.day ?? -Infinity));

  const events = timelineEvents(d.timeline, caseId)
    .sort((a, b) => a.at - b.at || a.order - b.order)
    .map((e) => ({ key: e.key, label: e.label, date: e.date, kind: e.kind satisfies AuditEventKind, note: `${e.source} · ${e.note ?? ""}` }));

  return { sources, caseHeld, events };
}
