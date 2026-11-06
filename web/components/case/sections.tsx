import { Avatar } from "@/components/avatar/avatar";
import { Pill } from "@/components/review/marks";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { TechTerm } from "@/components/wasl/tech-term";
import { closureChecks, linkScore, providerLabel, readerLabels } from "@/components/worklist/model";
import type { CaseDetail } from "@/lib/api/schema";
import { formatDay } from "@/lib/domain/day";
import { shortId } from "@/lib/domain/ids";
import { plain } from "@/lib/domain/terms";
import { personFor } from "@/lib/people";
import { recordFace } from "@/lib/people/identity";
import { cn } from "@/lib/utils";
import {
  atWords,
  basisWords,
  closureReason,
  consentNow,
  consentRows,
  eventWords,
  holdWords,
  outreachRows,
  reviewWords,
  roleWords,
  signoffWords,
  stampWords,
} from "./words";

/** The card every case section sits in. Padding shrinks on a phone so text keeps its width. */
export function Card({
  id,
  title,
  note,
  aside,
  children,
  className,
}: {
  id: string;
  title: string;
  note?: React.ReactNode;
  aside?: React.ReactNode;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <section id={id} aria-labelledby={`${id}-h`} className={cn("min-w-0 scroll-mt-4 rounded-card border border-line bg-surface p-4 md:p-pad", className)}>
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <h2 id={`${id}-h`} className="m-0 text-section font-medium">
          {title}
        </h2>
        {aside}
      </div>
      {note && <p className="m-0 mt-1 text-small text-muted">{note}</p>}
      {children}
    </section>
  );
}

const Empty = ({ children }: { children: React.ReactNode }) => <p className="m-0 mt-3 text-muted">{children}</p>;

const CLOSE_TONE = { closed: "positive", held: "negative", open: "neutral" } as const;

/** Why the case is where it is: the closure reason when closed, the hold reasons when open. */
export function ReasonCard({ detail }: { detail: CaseDetail }) {
  const reason = closureReason(detail);
  const tone = detail.case.closed ? CLOSE_TONE.closed : detail.holds.length ? CLOSE_TONE.held : CLOSE_TONE.open;
  return (
    <Card id="why" title={detail.case.closed ? "Why it closed" : "Why it is still open"}>
      <p className="m-0 mt-3 flex flex-wrap items-center gap-2">
        <span
          className={cn(
            "inline-flex min-h-[22px] items-center rounded-chip px-2 text-small font-medium",
            tone === "positive" && "bg-pill-positive-bg text-pill-positive-ink",
            tone === "negative" && "bg-pill-negative-bg text-pill-negative-ink",
            tone === "neutral" && "bg-pill-neutral-bg text-pill-neutral-ink",
          )}
        >
          {reason.title}
        </span>
      </p>
      <p className="m-0 mt-2">{reason.body}</p>
      {detail.holds.length > 0 && (
        <ul className="m-0 mt-3 list-none p-0" aria-label="Holds">
          {detail.holds.map((h, i) => (
            <li key={i} className="border-t border-rule py-3">
              <p className="m-0 font-medium">{holdWords(h)}</p>
              <p className="m-0 mt-1 text-small text-muted">{h.policy ?? "No policy text sent."}</p>
              <details className="mt-1 text-small text-muted">
                <summary className="min-h-6 cursor-pointer">Rule and system state</summary>
                <p className="m-0 mt-1">Rule: {h.rule}</p>
                <p className="m-0 mt-1">
                  State: <code className="font-mono">{h.state}</code>
                  {h.reason && (
                    <>
                      {" "}
                      · reason <code className="font-mono">{h.reason}</code>
                    </>
                  )}
                </p>
              </details>
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}

/** Both records of the patient, how each script writes the name, and how Wasl joined them. */
export function IdentitiesCard({ detail }: { detail: CaseDetail }) {
  return (
    <Card id="people" title="One patient, two records" note="Names are the simulated cast; the record numbers and the link facts come from the API.">
      <ul className="m-0 mt-3 list-none p-0" aria-label="Records">
        {detail.identities.map((idn) => {
          const p = personFor(idn.registration);
          const face = recordFace(detail, idn.registration);
          const arabic = p?.recorded_script === "arabic";
          return (
            <li key={idn.registration} className="flex items-center gap-3 border-t border-rule py-2.5 first:border-t-0">
              <Avatar {...face} size={32} label={face.state === "unknown" ? "Not yet confirmed as this patient" : ""} />
              <span className="min-w-0 flex-1">
                <span className="block text-small text-muted">
                  {providerLabel(idn.provider)} · {idn.role === "referral" ? "referring record" : "report's record"}
                </span>
                {p ? (
                  <>
                    <span lang={arabic ? "ar" : undefined} dir={arabic ? "rtl" : undefined} className="font-medium">
                      {p.recorded_name}
                    </span>
                    <span className="block text-small text-muted">written in {p.recorded_script === "arabic" ? "Arabic" : "Latin"} script</span>
                  </>
                ) : (
                  <span className="text-muted">Name not on file</span>
                )}
              </span>
              <span className="flex-none text-right">
                <span className="block text-small text-muted">Record number</span>
                <span className="font-mono text-small">{idn.registration.split("|")[1]}</span>
              </span>
            </li>
          );
        })}
      </ul>
      {detail.links.length === 0 ? (
        <Empty>No link joins these records yet.</Empty>
      ) : (
        <ul className="m-0 mt-2 list-none p-0" aria-label="How the records were linked">
          {detail.links.map((l, i) => {
            const score = linkScore(detail, l);
            return (
              <li key={i} className="border-t border-rule pt-3">
                <p className="m-0 font-medium">
                  {l.weakest_edge.basis === "eid" ? "Joined by Emirates ID." : `Joined by ${basisWords(l.weakest_edge.basis)}, no Emirates ID.`}
                </p>
                <dl className="m-0 mt-2 grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1">
                  <dt className="text-muted">Match probability</dt>
                  <dd className="m-0">{score.toFixed(3)}</dd>
                  <dt className="text-muted">Auto-close line</dt>
                  <dd className="m-0">{l.tau}</dd>
                  <dt className="text-muted">Weakest link</dt>
                  <dd className="m-0">
                    {basisWords(l.weakest_edge.basis)}, p {l.weakest_edge.p.toFixed(3)}
                  </dd>
                  <dt className="text-muted">Report joined</dt>
                  <dd className="m-0 font-mono text-small leading-6">{shortId(l.downstream_event)}</dd>
                </dl>
                <details className="mt-2 text-small text-muted">
                  <summary className="min-h-6 cursor-pointer">Linker details</summary>
                  <p className="m-0 mt-1">
                    Linker <code className="font-mono">{l.linker}</code> · {l.hops} {l.hops === 1 ? "hop" : "hops"}
                    {l.near_miss ? " · near miss" : ""}
                  </p>
                  <p className="m-0 mt-1 break-all font-mono">
                    {l.weakest_edge.a} → {l.weakest_edge.b}
                  </p>
                </details>
              </li>
            );
          })}
        </ul>
      )}
    </Card>
  );
}

const WORD: Record<string, string> = { yes: "Yes", no: "No", not_stated: "Not stated", adequate: "Adequate", inadequate: "Not adequate" };
const word = (v: string | undefined) => (v ? (WORD[v] ?? plain(v)) : "Not read");
const CRC_FIELDS = ["Caecum reached", "Prep graded", "Tissue taken", "Histology back"] as const;

type Evidence = CaseDetail["evidence"][number];

/** Field-by-field values for the two readers: CRC and cervical carry four slash-joined fields, the others one object per reader. */
function fieldRows(ev: Evidence): { field: string; a: string; b: string }[] {
  if (ev.fields) {
    const rules = String(ev.fields.regex ?? "").split("/");
    const model = String(ev.fields.reader ?? "").split("/");
    return CRC_FIELDS.map((field, i) => ({ field, a: word(rules[i]), b: word(model[i]) }));
  }
  const a = (ev.reader_a_fields ?? {}) as Record<string, unknown>;
  const b = (ev.reader_b_fields ?? {}) as Record<string, unknown>;
  const keys = [...new Set([...Object.keys(a), ...Object.keys(b)])].filter((k) => k !== "quote");
  return keys.map((k) => ({ field: plain(k), a: a[k] === undefined ? "Not read" : String(a[k]), b: b[k] === undefined ? "Not read" : String(b[k]) }));
}

/** The readers' result for one report: the checklist, both readers' values, and the quote. */
export function ReadersCard({ detail, ev, id, title }: { detail: CaseDetail; ev: Evidence; id: string; title?: string }) {
  const read = ev.fields ? { checks: closureChecks(ev), quote: ev.quote } : readerLabels(ev);
  const rows = fieldRows(ev);
  const quoteB = ev.reader_b_fields?.quote;
  const reportUnit = detail.timeline.find((e) => e.id === ev.report)?.provider ?? detail.identities.find((i) => i.role === "downstream")?.provider ?? "";
  return (
    <Card
      id={id}
      title={title ?? (ev.fields ? "DoH closure checklist" : "Readers' result")}
      note={`Two readers read report ${shortId(ev.report)}: ${ev.agree ? "they agree" : "they disagree"}.`}
    >
      <ul className="m-0 mt-3 list-none p-0">
        {read.checks.map((k) => (
          <li key={k.name} className="flex items-center justify-between gap-3 border-t border-rule py-2.5">
            <span>{k.name}</span>
            <span className="text-right text-muted">
              {k.result}
              {k.differ && <span className="block text-small text-warning">Readers differ</span>}
            </span>
          </li>
        ))}
      </ul>
      {ev.human_override && <p className="m-0 mt-2 text-small text-muted">A clinician signed the evidence {ev.human_override}.</p>}
      {rows.length > 0 && (
        <div className="mt-3">
          <table className="w-full border-collapse text-left">
            <caption className="pb-1 text-left text-small text-muted">What each reader read, field by field</caption>
            <thead>
              <tr className="text-small text-muted">
                <th scope="col" className="py-1 pr-3 font-normal">
                  Field
                </th>
                <th scope="col" className="py-1 pr-3 font-normal">
                  Reader A (rules)
                </th>
                <th scope="col" className="py-1 font-normal">
                  Reader B (language model)
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.field} className={cn("border-t border-rule align-top", r.a !== r.b && "text-warning")}>
                  <th scope="row" className="py-2 pr-3 font-normal text-ink">
                    {r.field}
                  </th>
                  <td className="py-2 pr-3 text-ink [overflow-wrap:anywhere]">{r.a}</td>
                  <td className="py-2 text-ink [overflow-wrap:anywhere]">
                    {r.b}
                    {r.a !== r.b && <span className="block text-small text-warning">differs</span>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <blockquote className="m-0 mt-3 rounded-control bg-raised px-4 py-3">
        <p className="m-0">{read.quote ? `“${read.quote}”` : "The report states none of these in words."}</p>
        {quoteB && quoteB !== read.quote && <p className="m-0 mt-2 text-muted">Reader B quotes: “{quoteB}”</p>}
        <p className="m-0 mt-1 text-small text-muted">
          {providerLabel(reportUnit)} report <span className="font-mono">{shortId(ev.report)}</span>
          {ev.reader_model ? ` · second reader ${ev.reader_model}` : ""}
        </p>
      </blockquote>
    </Card>
  );
}

/** Consent history and the messages Wasl sent or held back. */
export function ConsentCard({ detail }: { detail: CaseDetail }) {
  const consent = consentRows(detail);
  const out = outreachRows(detail);
  return (
    <Card id="consent" title="Consent and messages" aside={<HonestyLabel kind="simulated">Simulated messages</HonestyLabel>}>
      <p className="m-0 mt-3">
        Now: <span className="font-medium">{consentNow(detail.case.consent)}</span>
      </p>
      <h3 className="m-0 mt-3 text-body font-medium">Consent history</h3>
      {consent.length === 0 ? (
        <Empty>No consent events on record.</Empty>
      ) : (
        <ul className="m-0 mt-1 list-none p-0" aria-label="Consent history">
          {consent.map((c) => (
            <li key={c.key} className="flex flex-wrap justify-between gap-x-3 border-t border-rule py-2">
              <span>{c.what}</span>
              <span className="text-muted">{c.when}</span>
            </li>
          ))}
        </ul>
      )}
      <h3 className="m-0 mt-3 text-body font-medium">Messages</h3>
      {out.length === 0 ? (
        <Empty>No messages were sent or held for this case.</Empty>
      ) : (
        <ul className="m-0 mt-1 list-none p-0" aria-label="Messages">
          {out.map((o) => (
            <li key={o.key} className="border-t border-rule py-2">
              <span className="block">{o.what}</span>
              <span className={cn("block", o.sent ? "text-ink" : "text-warning")}>{o.outcome}</span>
              <details className="text-small text-muted">
                <summary className="min-h-6 cursor-pointer">Raw message record</summary>
                <code className="block break-all font-mono">{JSON.stringify(o.raw)}</code>
              </details>
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}

/** Human sign-offs: the run's simulated reviewers, and anything appended through the API. */
export function SignoffsCard({ detail }: { detail: CaseDetail }) {
  const { signoffs, api_reviews: appended } = detail;
  return (
    <Card
      id="signoffs"
      title="Human sign-offs"
      aside={<HonestyLabel kind="simulated">Simulated reviewers</HonestyLabel>}
      note="Nothing closes on one signature."
    >
      {signoffs.length === 0 && appended.length === 0 ? (
        <Empty>No person signed anything on this case. Two readers agreed, so none was needed.</Empty>
      ) : (
        <ul className="m-0 mt-3 list-none p-0" aria-label="Sign-offs">
          {signoffs.map((s) => (
            <li key={s.item} className="border-t border-rule py-3">
              <p className="m-0 font-medium">{signoffWords(s)}</p>
              <p className="m-0 text-muted">
                {roleWords(s.role)} · simulated reviewer · {stampWords(s.t)}
              </p>
              <details className="text-small text-muted">
                <summary className="min-h-6 cursor-pointer">Reviewer record</summary>
                <p className="m-0 mt-1 break-words">{s.reviewer}</p>
                <p className="m-0 mt-1 font-mono">{s.item}</p>
              </details>
            </li>
          ))}
          {appended.map((r) => (
            <li key={r.hash} className="border-t border-rule py-3">
              <p className="m-0 font-medium">{reviewWords(r.decision)}</p>
              <p className="m-0 text-muted">
                {r.actor || "Unnamed"} · {roleWords(r.role)} · {atWords(r.at)} · saved through the API
              </p>
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}

/** Every linked source event, oldest first. */
export function TimelineCard({ detail }: { detail: CaseDetail }) {
  const events = [...detail.timeline].sort((a, b) => (a.arrival_day ?? a.day) - (b.arrival_day ?? b.day) || (a.seq ?? 0) - (b.seq ?? 0));
  return (
    <Card id="timeline" title="Timeline" note={`${events.length} source events, no free text.`}>
      <ol className="m-0 mt-3 list-none p-0" aria-label="Timeline">
        {events.map((e) => (
          <li key={`${e.id}@${e.seq}`} className="grid grid-cols-[4.5rem_minmax(0,1fr)] gap-x-3 border-t border-rule py-2">
            <span className="text-muted">{formatDay(e.day)}</span>
            <span className="min-w-0">
              {eventWords(e, detail.case.id)}
              <span className="block text-small text-muted">
                {providerLabel(e.provider)} · <span className="break-all font-mono">{shortId(e.id)}</span>
              </span>
            </span>
          </li>
        ))}
      </ol>
    </Card>
  );
}

const BOOKING_STATUS: Record<string, string> = {
  suggested: "Slot suggested",
  no_slot_within_horizon: "No slot within the booking window",
  not_applicable: "No booking needed",
  not_generated: "No suggestion made",
};

type BookingData = {
  region?: string;
  procedure?: string;
  wait_days?: number;
  requested_day?: number;
  capacity_condition?: string;
  pooling_note?: string;
  pooling_relevant?: boolean;
  cross_region?: boolean;
  booking?: {
    unit?: string;
    day?: number;
    deadline?: number;
    within_deadline?: boolean;
    predicted_no_show?: number;
    endoscopist_female?: boolean;
    prep_send_day?: number;
  };
  pathway?: { route?: string; reasons?: string[]; checklist_notes?: string[] };
  prep_risk?: {
    band?: string;
    score?: number;
    factors?: string[];
    limitation?: string;
    requires_clinician_order?: boolean;
    suggested_intervention?: { text?: string; evidence?: string };
  };
  constraints?: Record<string, unknown>;
  app9?: Record<string, unknown>;
};

const yesNo = (v: unknown) => (v === true ? "yes" : v === false ? "no" : String(v));

/** The simulated booking suggestion and the App 9 prep-risk fields (issue #2). Decision support only. */
export function BookingCard({ detail }: { detail: CaseDetail }) {
  const b = detail.booking;
  const d = b as unknown as BookingData;
  const slot = d.booking;
  const risk = d.prep_risk;
  return (
    <Card id="booking" title="Booking" aside={<HonestyLabel kind="simulated">Simulated decision support</HonestyLabel>}>
      <p className="m-0 mt-3 flex flex-wrap items-center gap-2">
        <Pill tone={b.status === "suggested" ? "positive" : "muted"}>{BOOKING_STATUS[b.status] ?? plain(b.status)}</Pill>
        {b.data_mode && <span className="text-small text-muted">data mode: {b.data_mode}</span>}
      </p>
      {b.reason && <p className="m-0 mt-2 text-muted">{plain(b.reason)}.</p>}
      {b.status === "suggested" && slot && (
        <dl className="m-0 mt-3 grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1">
          {slot.unit && (
            <>
              <dt className="text-muted">Unit</dt>
              <dd className="m-0">
                {slot.unit}
                {d.region ? `, ${d.region} region` : ""}
              </dd>
            </>
          )}
          {slot.day !== undefined && (
            <>
              <dt className="text-muted">Slot</dt>
              <dd className="m-0">
                Day {slot.day}
                {d.wait_days !== undefined ? `, ${d.wait_days} days after the request` : ""}
              </dd>
            </>
          )}
          {slot.deadline !== undefined && (
            <>
              <dt className="text-muted">Deadline</dt>
              <dd className="m-0">
                Day {slot.deadline}, {slot.within_deadline ? "slot is inside it" : "slot is past it"}
              </dd>
            </>
          )}
          {slot.predicted_no_show !== undefined && (
            <>
              <dt className="text-muted">Predicted no-show</dt>
              <dd className="m-0">{Math.round(slot.predicted_no_show * 100)}%</dd>
            </>
          )}
          {slot.endoscopist_female !== undefined && (
            <>
              <dt className="text-muted">Female endoscopist</dt>
              <dd className="m-0">{yesNo(slot.endoscopist_female)}</dd>
            </>
          )}
          {slot.prep_send_day !== undefined && (
            <>
              <dt className="text-muted">Prep sent</dt>
              <dd className="m-0">Day {slot.prep_send_day}</dd>
            </>
          )}
        </dl>
      )}
      {b.status !== "suggested" && d.region && <p className="m-0 mt-2 text-muted">Region: {d.region}.</p>}
      {d.pooling_note && <p className="m-0 mt-3">{d.pooling_note[0].toUpperCase() + d.pooling_note.slice(1)}.</p>}
      {d.pathway && (
        <p className="m-0 mt-3">
          Pathway:{" "}
          <span className="font-medium">
            {d.pathway.route === "direct" ? "straight to colonoscopy" : d.pathway.route === "clinic" ? "clinic first" : plain(d.pathway.route ?? "unknown")}
          </span>
          {d.pathway.reasons?.length ? `. ${d.pathway.reasons.join("; ")}.` : "."}
        </p>
      )}
      {risk && (
        <div className="mt-3 rounded-control bg-ground p-4">
          <h3 className="m-0 text-body font-medium">Bowel-prep risk (App 9)</h3>
          <p className="m-0 mt-1">
            {risk.band === "high" ? "High" : "Low"} risk, score {risk.score ?? 0}
            {risk.factors?.length ? `: ${risk.factors.map((f) => plain(f).toLowerCase()).join(", ")}` : ", no risk factors found"}.
          </p>
          {risk.suggested_intervention?.text && <p className="m-0 mt-1">Suggested: {risk.suggested_intervention.text}.</p>}
          {risk.requires_clinician_order && <p className="m-0 mt-1 text-muted">A clinician must order any prep change. Wasl only suggests.</p>}
          {risk.limitation && <p className="m-0 mt-1 text-small text-muted">Limitation: {risk.limitation}.</p>}
        </div>
      )}
      {d.app9 && (
        <details className="mt-3 text-muted">
          <summary className="min-h-6 cursor-pointer">Checklist fields behind it (synthetic)</summary>
          <dl className="m-0 mt-2 grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-1">
            {Object.entries(d.app9)
              .filter(([k]) => k !== "data_mode")
              .map(([k, v]) => (
                <div key={k} className="contents">
                  <dt>{plain(k)}</dt>
                  <dd className="m-0 text-ink">{typeof v === "boolean" ? yesNo(v) : String(v)}</dd>
                </div>
              ))}
          </dl>
        </details>
      )}
      <div className="mt-3 text-small text-muted">
        Simulated: no slot is held and no one is told. It is decision support for the coordinator, not a booking.
        {d.capacity_condition && (
          <>
            {" "}
            Capacity in this run: <TechTerm id={d.capacity_condition} />
          </>
        )}
      </div>
      {d.constraints && (
        <details className="mt-1 text-small text-muted">
          <summary className="min-h-6 cursor-pointer">Booking constraints used</summary>
          <dl className="m-0 mt-1 grid grid-cols-[minmax(0,1fr)_auto] gap-x-4">
            {Object.entries(d.constraints).map(([k, v]) => (
              <div key={k} className="contents">
                <dt>{plain(k)}</dt>
                <dd className="m-0 text-ink">{typeof v === "boolean" ? yesNo(v) : String(v)}</dd>
              </div>
            ))}
          </dl>
        </details>
      )}
    </Card>
  );
}
