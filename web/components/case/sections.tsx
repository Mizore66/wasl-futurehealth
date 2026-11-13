import { Avatar } from "@/components/avatar/avatar";
import { Pill } from "@/components/review/marks";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { TechTerm } from "@/components/wasl/tech-term";
import { closureChecks, linkScore, providerLabel, readerLabels } from "@/components/worklist/model";
import { ConsentEventName, type CaseDetail } from "@/lib/api/schema";
import { apiMode } from "@/lib/api/source";
import { formatDay, formatDayYear } from "@/lib/domain/day";
import { shortId } from "@/lib/domain/ids";
import { plain } from "@/lib/domain/terms";
import { personFor } from "@/lib/people";
import { recordFace } from "@/lib/people/identity";
import { protocolRead, READ_LABEL, resolutionProgress } from "@/lib/domain/stage";
import { cn } from "@/lib/utils";
import { ConsentActions } from "./consent-actions";
import { BookingControls } from "./booking-controls";
import {
  atWords,
  basisWords,
  closureReason,
  consentNow,
  consentRows,
  eventWords,
  holdWords,
  ledgerId,
  ledgerWords,
  ledgerWordsWithHistory,
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
  if (detail.case.crc) return <Card id="why" title="Screening completion is not care completion"><p>Each current CRC obligation below has its own evidence binding, clinician confirmation and tracking state. Completing one pathway does not discharge another.</p></Card>;
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
                <span className="text-small">{idn.registration.split("|")[1]}</span>
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
                  <dd className="m-0 text-small leading-6">{shortId(l.downstream_event)}</dd>
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
  const a = ev.reader_a_fields ?? {};
  const b = ev.reader_b_fields ?? {};
  if (typeof a === "string" || typeof b === "string") {
    return [{ field: "Reader note", a: typeof a === "string" ? a : "Not read", b: typeof b === "string" ? b : "Not read" }];
  }
  const keys = [...new Set([...Object.keys(a), ...Object.keys(b)])].filter((k) => k !== "quote");
  return keys.map((k) => ({ field: plain(k), a: a[k] === undefined ? "Not read" : String(a[k]), b: b[k] === undefined ? "Not read" : String(b[k]) }));
}

/** The readers' result for one report: the checklist, both readers' values, and the quote. */
export function ReadersCard({ detail, ev, id, title }: { detail: CaseDetail; ev: Evidence; id: string; title?: string }) {
  const read = ev.fields ? { checks: closureChecks(ev), quote: ev.quote } : readerLabels(ev);
  const rows = fieldRows(ev);
  const quoteB = typeof ev.reader_b_fields === "object" ? ev.reader_b_fields.quote : undefined;
  const reportUnit = detail.timeline.find((e) => e.id === ev.report)?.provider ?? detail.identities.find((i) => i.role === "downstream")?.provider ?? "";
  return (
    <Card
      id={id}
      title={
        title ??
        (detail.case.programme === "lung_ldct"
          ? "LDCT readers' result"
          : detail.case.programme === "ifhas"
            ? "IFHAS readers' result"
            : ev.fields
              ? "DoH closure checklist"
              : "Readers' result")
      }
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
          {providerLabel(reportUnit)} report {shortId(ev.report)}
          {ev.reader_model ? ` · second reader ${ev.reader_model}` : ""}
        </p>
      </blockquote>
    </Card>
  );
}

function FindingRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <>
      <dt className="text-muted">{label}</dt>
      <dd className="m-0 min-w-0 [overflow-wrap:anywhere]">{children}</dd>
    </>
  );
}

const IFHAS_DOMAIN: Record<string, string> = { bp: "Blood pressure", glyc: "Glycaemia", lipid: "Lipids", weight: "Weight" };

/* Words for the API's own enum values (docs/API.md). They name what the rule already decided; nothing is derived here. */
const LUNG_ACTION: Record<string, string> = { none: "No routine follow-up", ct: "Follow-up CT", ct_or_pet_or_tissue: "CT, PET/CT or tissue sampling" };
const DR_ACTION: Record<string, string> = { refer: "Refer to the eye clinic", rescreen: "Rescreen", repeat_photo: "Repeat photo" };
const BREAST_ACTION: Record<string, string> = { routine: "Routine screening, no recall", recall: "Recall for assessment", repeat_mammogram: "Repeat mammogram" };
const DR_OUTCOME: Record<string, string> = {
  examined_treated: "Examined, treated",
  examined_followup: "Examined, follow-up",
  examined_discharged: "Examined, discharged",
  not_examined: "Retina not examined",
  dna: "Did not attend",
  not_assessable: "Not assessable",
};
const BREAST_OUTCOME: Record<string, string> = {
  return_routine: "Return to routine screening",
  early_recall: "Early recall",
  to_treatment: "Referred to treatment",
  incomplete: "Incomplete",
  dna: "Did not attend",
};
const NOT_RECORDED = "Not recorded in the API data";
const yesNoWords = (v: boolean | undefined) => (v === undefined ? null : v ? "Yes" : "No");

/** A day range from the API's absolute day numbers, or null when either end is missing. */
function dayRange(from: number | undefined, to: number | undefined): string | null {
  return from === undefined || to === undefined ? null : `${formatDayYear(from)} to ${formatDayYear(to)}`;
}

/**
 * The screening clock. Refer, recall, urgent and repeat actions run a deadline with an overdue clock; a routine
 * rescreen interval (DR rescreen, breast routine) has none (pipeline/drrule.go, pipeline/breastrule.go), so it reads
 * as the next screen, not as a deadline.
 */
function ClockRow({ days, day, routine }: { days: number | undefined; day: number | undefined; routine: boolean }) {
  const length = days !== undefined ? `${days} days` : NOT_RECORDED;
  if (routine) {
    const when = day !== undefined ? `, ${formatDayYear(day)}` : "";
    return <FindingRow label="Next screen">{`${length}${when} (routine interval, no overdue clock)`}</FindingRow>;
  }
  return <FindingRow label="Deadline">{`${length}${day !== undefined ? `, due ${formatDayYear(day)}` : ""}`}</FindingRow>;
}

/** "91 to 183 days" from the rule's window, plus the absolute dates when the API sent them. */
function windowWords(lo: number | undefined, hi: number | undefined, dates: string | null): string {
  const days = lo === undefined || hi === undefined ? null : `${lo} to ${hi} days`;
  return [days, dates].filter(Boolean).join(", ") || NOT_RECORDED;
}

/** Incidental lung nodule (Fleischner 2017): index finding, Fleischner cell, risk and the follow-up window. */
function LungNoduleRows({ detail }: { detail: CaseDetail }) {
  const f = detail.case.lung ?? detail.current.lung;
  if (!f) return <Empty>The API sent no lung-nodule finding for this case yet.</Empty>;
  const rec = f.rec;
  const window = dayRange(f.window_start_day, f.window_end_day);
  // pipeline/lung.go applies the Fleischner rule as high risk when no risk was recorded, and says so with risk_recorded.
  const riskAssumed = f.risk_recorded === false;
  return (
    <>
      <dl className="m-0 mt-3 grid grid-cols-[minmax(0,auto)_minmax(0,1fr)] gap-x-4 gap-y-2" aria-label="Lung-nodule finding">
        <FindingRow label="Index CT report">{f.finding_report ? shortId(f.finding_report) : "Not on this case"}</FindingRow>
        <FindingRow label="Nodule">{f.finding_label ?? "Not read yet"}</FindingRow>
        <FindingRow label="Fleischner category">{rec?.cell ?? "Not assigned yet"}</FindingRow>
        <FindingRow label="Patient risk">{f.risk ? `${plain(f.risk)}${riskAssumed ? " (assumed: risk not recorded)" : ""}` : NOT_RECORDED}</FindingRow>
        {f.scope && <FindingRow label="Scope">{plain(f.scope)}</FindingRow>}
        <FindingRow label="Recommendation">
          {rec ? `${LUNG_ACTION[rec.action] ?? plain(rec.action)}${rec.optional ? " (optional)" : ""}` : "Not decided yet"}
        </FindingRow>
        <FindingRow label="Risk window">
          {!rec
            ? "Not set yet"
            : rec.action === "none"
              ? "No follow-up window"
              : `${windowWords(rec.win_lo_days, rec.win_hi_days, window)}${riskAssumed ? ` (assumes ${f.risk ?? "high"} risk)` : ""}`}
        </FindingRow>
        <FindingRow label="Follow-up outcome">
          {f.outcome ? plain(f.outcome) : rec?.action === "none" ? "None needed" : "Pending: no follow-up CT read yet"}
        </FindingRow>
      </dl>
      {rec && riskAssumed && (
        <p className="m-0 mt-3 border-t border-rule pt-3" role="note">
          {`Risk was not recorded, so the rule treated this patient as ${f.risk ?? "high"} risk. The Fleischner category, recommendation and follow-up window rest on that assumption.`}
        </p>
      )}
      {rec?.next_step && <p className="m-0 mt-3 border-t border-rule pt-3">Next step: {rec.next_step}.</p>}
    </>
  );
}

/** Diabetic retinopathy (doh2024dr): screening grade, the referral's urgency and deadline, the eye-clinic outcome and next due. */
function DrRows({ detail }: { detail: CaseDetail }) {
  const f = detail.case.dr ?? detail.current.dr;
  if (!f) return <Empty>The API sent no retinal screening finding for this case yet.</Empty>;
  const rec = f.rec;
  const examined = yesNoWords(f.examined);
  const within = yesNoWords(f.within_deadline);
  return (
    <dl className="m-0 mt-3 grid grid-cols-[minmax(0,auto)_minmax(0,1fr)] gap-x-4 gap-y-2" aria-label="Retinal screening finding">
      <FindingRow label="Screening report">{f.screen_report ? shortId(f.screen_report) : "Not received yet"}</FindingRow>
      <FindingRow label="Grade">
        {f.screen_label ?? "Not graded yet"}
        {f.screen_label && f.coded_grade ? " (coded result matched the text)" : ""}
      </FindingRow>
      <FindingRow label="Recommendation">{rec ? (DR_ACTION[rec.action] ?? plain(rec.action)) : "Not decided yet"}</FindingRow>
      <FindingRow label="Urgency">{rec?.urgency ? plain(rec.urgency) : rec ? "Not a referral" : "Not decided yet"}</FindingRow>
      <ClockRow days={rec?.deadline_days} day={f.deadline_day} routine={rec?.action === "rescreen"} />
      {rec?.action === "refer" && (
        <FindingRow label="Eye-clinic note">{f.clinic_note ? shortId(f.clinic_note) : "Pending: no eye-clinic note read yet"}</FindingRow>
      )}
      {(f.outcome || rec?.action === "refer") && <FindingRow label="Outcome">{f.outcome ? (DR_OUTCOME[f.outcome] ?? plain(f.outcome)) : "Pending"}</FindingRow>}
      {examined && <FindingRow label="Retina examined">{examined}</FindingRow>}
      {within && <FindingRow label="Within deadline">{within}</FindingRow>}
      <FindingRow label="Next due">
        {f.next_due
          ? `${plain(f.next_due.kind)} ${formatDayYear(f.next_due.due_day)} (${plain(f.next_due.basis).toLowerCase()})`
          : detail.case.closed
            ? NOT_RECORDED
            : "Set once the loop resolves"}
      </FindingRow>
    </dl>
  );
}

/** Breast screening (doh2025breast): BI-RADS category, the recall recommendation and the assessment outcome. */
function BreastRows({ detail }: { detail: CaseDetail }) {
  const f = detail.case.breast ?? detail.current.breast;
  if (!f) return <Empty>The API sent no breast screening finding for this case yet.</Empty>;
  const rec = f.rec;
  const within = yesNoWords(f.within_deadline);
  // Only a recall expects an assessment-unit document; a repeat mammogram (TR) waits for a new screen (pipeline/breast.go).
  const assessed = rec?.action === "recall";
  return (
    <dl className="m-0 mt-3 grid grid-cols-[minmax(0,auto)_minmax(0,1fr)] gap-x-4 gap-y-2" aria-label="Breast screening finding">
      <FindingRow label="Screening report">{f.screen_report ? shortId(f.screen_report) : "Not received yet"}</FindingRow>
      <FindingRow label="BI-RADS category">
        {f.screen_label ?? "Not read yet"}
        {f.screen_label && f.coded_birads ? " (coded assessment matched the text)" : ""}
      </FindingRow>
      <FindingRow label="Recall recommendation">
        {rec ? `${BREAST_ACTION[rec.action] ?? plain(rec.action)}${rec.urgency ? `, ${rec.urgency}` : ""}` : "Not decided yet"}
      </FindingRow>
      <ClockRow days={rec?.deadline_days} day={f.deadline_day} routine={rec?.action === "routine"} />
      {assessed && <FindingRow label="Assessment document">{f.assessment_document ? shortId(f.assessment_document) : "Pending: none read yet"}</FindingRow>}
      <FindingRow label="Outcome">{f.outcome
          ? (BREAST_OUTCOME[f.outcome] ?? plain(f.outcome))
          : assessed
            ? "Pending"
            : rec?.action === "repeat_mammogram"
              ? "None: repeat mammogram due, no assessment"
              : rec
                ? "None: no recall"
                : "Not decided yet"}</FindingRow>
      {within && <FindingRow label="Within deadline">{within}</FindingRow>}
    </dl>
  );
}

/** The programme-specific finding and recommendation, kept separate from the CRC closure checklist. */
export function ProgrammeFindingCard({ detail }: { detail: CaseDetail }): React.JSX.Element {
  const { case: c } = detail;
  if (c.crc) {
    return <Card id="programme-finding" title="CRC screening and independent care pathways" note="Simulated tracking only; no treatment delivery or referral dispatch.">
      <p>Screening: {c.crc.screening_complete ? "complete" : "evidence incomplete"}. Care: {c.crc.care_state.replaceAll("_", " ")}.</p>
      {c.crc.kind !== "screening" && <p>{c.crc.kind.replaceAll("-", " ")} tracking · source {c.crc.source_report} v{c.crc.source_version}; clock starts day {c.crc.clock_start}. {c.crc.follow_up_state?.replaceAll("_", " ")}. This is the modeled tracking state, not evidence that treatment was delivered.</p>}
      <ul aria-label="Independent CRC obligations">
        {c.crc.obligations.map((need) => <li key={need.id}>
          <strong>{need.outcome.replaceAll("_", " ")}</strong> · {need.state.replaceAll("_", " ")}
          <p>Source {need.source_report} v{need.source_version}; procedure {need.procedure_report} v{need.procedure_version}; clock starts day {need.clock_start}. Owner {need.owner}.</p>
          {need.blockers.length > 0 && <p>Holds: {need.blockers.map((b) => b.replaceAll("_", " ")).join(", ")}.</p>}
          <p>{need.confirmation ? `Confirmed by ${need.reviewer}; signature ${need.confirmation}.` : `Separate clinician confirmation required: ${need.review_item}.`}</p>
          {need.interval_days !== undefined && <p>Documented surveillance interval: {need.interval_days} days; next due day {need.due_day}.</p>}
          {need.child_loop && <p>Tracking loop: {need.child_loop}.</p>}
        </li>)}
      </ul>
    </Card>;
  }
  if (c.programme === "lung_nodule") {
    return (
      <Card id="programme-finding" title="Incidental lung nodule (Fleischner)">
        <LungNoduleRows detail={detail} />
      </Card>
    );
  }
  if (c.programme === "diabetic_retinopathy") {
    return (
      <Card id="programme-finding" title="Diabetic retinopathy screening">
        <DrRows detail={detail} />
      </Card>
    );
  }
  if (c.programme === "breast") {
    return (
      <Card id="programme-finding" title="Breast screening (BI-RADS)">
        <BreastRows detail={detail} />
      </Card>
    );
  }
  if (c.programme === "lung_ldct" && c.ldct) {
    const f = c.ldct;
    return (
      <Card id="programme-finding" title="Lung cancer screening (LDCT)">
        <dl className="m-0 mt-3 grid grid-cols-[minmax(0,auto)_minmax(0,1fr)] gap-x-4 gap-y-2">
          {f.scan_report && <FindingRow label="Scan report">{shortId(f.scan_report)}</FindingRow>}
          {f.scan_label && <FindingRow label="Lung-RADS category">{f.scan_label}</FindingRow>}
          {f.rec && (
            <>
              <FindingRow label="Recommendation">{plain(f.rec.action)}</FindingRow>
              <FindingRow label="Rule timing">
                Due in {f.rec.due_days} days{f.rec.deadline_days > 0 ? `; overdue after ${f.rec.deadline_days} days` : "; no overdue clock"}
              </FindingRow>
            </>
          )}
          {f.followup_report && <FindingRow label="Follow-up report">{shortId(f.followup_report)}</FindingRow>}
          {f.followup_label && <FindingRow label="Follow-up Lung-RADS">{f.followup_label}</FindingRow>}
          {f.workup_document && <FindingRow label="Work-up document">{shortId(f.workup_document)}</FindingRow>}
          {f.outcome && <FindingRow label="Outcome">{plain(f.outcome)}</FindingRow>}
          {f.within_deadline !== undefined && <FindingRow label="Within deadline">{f.within_deadline ? "Yes" : "No"}</FindingRow>}
        </dl>
        {(f.rec?.next_step || f.followup_next_step) && (
          <div className="mt-3 border-t border-rule pt-3">
            {f.rec?.next_step && <p className="m-0">Next step: {f.rec.next_step}.</p>}
            {f.followup_next_step && <p className="m-0 mt-2">After the follow-up: {f.followup_next_step}.</p>}
          </div>
        )}
      </Card>
    );
  }
  if (c.programme === "ifhas" && c.ifhas) {
    const f = c.ifhas;
    const domains = Object.entries(f.domain_status ?? {});
    return (
      <Card id="programme-finding" title="IFHAS screening results">
        <dl className="m-0 mt-3 grid grid-cols-[minmax(0,auto)_minmax(0,1fr)] gap-x-4 gap-y-2">
          {f.screen_report && <FindingRow label="Screening report">{shortId(f.screen_report)}</FindingRow>}
          {f.screen_label && <FindingRow label="Results label">{f.screen_label}</FindingRow>}
          {f.rec && (
            <>
              <FindingRow label="Recommendation">{plain(f.rec.action)}</FindingRow>
              <FindingRow label="Deadline">{f.rec.deadline_days} days</FindingRow>
            </>
          )}
          {domains.map(([domain, status]) => (
            <FindingRow key={domain} label={IFHAS_DOMAIN[domain] ?? plain(domain)}>
              {status ? plain(status) : "Not documented yet"}
              {f.resolved_day?.[domain as keyof typeof f.resolved_day] !== undefined
                ? ` · resolved ${formatDay(f.resolved_day[domain as keyof typeof f.resolved_day]!)}`
                : ""}
            </FindingRow>
          ))}
          {f.followup_documents && f.followup_documents.length > 0 && (
            <FindingRow label="Follow-up documents">{f.followup_documents.map(shortId).join(", ")}</FindingRow>
          )}
          {f.outcome && <FindingRow label="Outcome">{f.outcome}</FindingRow>}
          {f.within_deadline !== undefined && <FindingRow label="Within deadline">{f.within_deadline ? "Yes" : "No"}</FindingRow>}
        </dl>
        {f.next_step && <p className="m-0 mt-3 border-t border-rule pt-3">Next step: {f.next_step}.</p>}
      </Card>
    );
  }
  return <></>;
}

/** Consent history and the messages Wasl sent or held back. */
export function ConsentCard({ detail }: { detail: CaseDetail }) {
  const consent = consentRows(detail);
  const out = outreachRows(detail);
  return (
    <Card id="consent" title="Consent and messages" aside={<HonestyLabel kind="simulated">Simulated messages</HonestyLabel>}>
      <p className="m-0 mt-3">
        Canonical simulation now: <span className="font-medium">{consentNow(detail.case.consent)}</span>
      </p>
      <h3 className="m-0 mt-3 text-body font-medium">Canonical consent history</h3>
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
      <ConsentActions
        caseId={detail.case.id}
        expectedHead={detail.current.sig}
        contact={detail.api_contact}
        live={apiMode() === "live"}
        canWrite={detail.review_ledger?.status !== "invalid"}
      />
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

const CONSENT_EVENTS: ReadonlySet<string> = new Set(ConsentEventName.options);

/** Human sign-offs: the run's simulated reviewers, and anything appended through the API. */
export function SignoffsCard({ detail }: { detail: CaseDetail }) {
  const { signoffs } = detail;
  /*
   * Reviews and their undo/contest events. Coordinator actions share the ledger but are not sign-offs: LedgerCard.
   * Demo consent events are shown by ConsentCard. A confirm_contact review also carries action record_consent, so
   * match the consent event decision, not the action.
   */
  const appended = detail.api_reviews.filter(
    (r) => (r.ledger === "review" && !CONSENT_EVENTS.has(r.decision)) || r.ledger === "compensation",
  );
  return (
    <Card
      id="signoffs"
      title="Human sign-offs"
      aside={<HonestyLabel kind="simulated">Simulated reviewers</HonestyLabel>}
      note="Each answer shows who gave it and in which role. Two answers from one person are not independent double review. A first or second read is not a final sign-off; only an agreed or senior-resolved question is."
    >
      {resolutionProgress(detail) && (
        <p className="m-0 mt-3 text-small text-muted" data-testid="resolution-progress">
          Independent review: {resolutionProgress(detail)}.
        </p>
      )}
      {signoffs.length === 0 && appended.length === 0 ? (
        <Empty>No human sign-offs recorded for this case.</Empty>
      ) : (
        <ul className="m-0 mt-3 list-none p-0" aria-label="Sign-offs">
          {signoffs.map((s) => (
            <li key={s.item} className="border-t border-rule py-3">
              <p className="m-0 font-medium">{signoffWords(s, detail.case.programme)}</p>
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
              {r.ledger === "review" ? (
                <p className="m-0 font-medium">
                  {protocolRead(r) && <span className="mr-2 text-muted" data-protocol-read={protocolRead(r)}>{READ_LABEL[protocolRead(r)!]}:</span>}
                  {reviewWords(r.decision, detail.case.programme)}
                </p>
              ) : (
                <div className="font-medium">
                  <TechTerm id={ledgerId(r)} label={ledgerWords(r)} />
                </div>
              )}
              <p className="m-0 text-muted">
                {r.actor || "Unnamed"} · {roleWords(r.role)} · {atWords(r.at)} · saved through the API
                {r.ledger === "review" && protocolRead(r) && " · one independent read, not a final sign-off"}
              </p>
              {r.ledger === "compensation" && r.note && <p className="m-0 mt-1">&ldquo;{r.note}&rdquo;</p>}
            </li>
          ))}
        </ul>
      )}
    </Card>
  );
}

/**
 * Coordinator actions on the shared ledger (owner, snooze, contact outcomes, notes), in ledger order, each named by its
 * real `type/kind` and the actor the API recorded. Entries the client cannot read are listed as unrecognised.
 */
export function LedgerCard({ detail }: { detail: CaseDetail }) {
  const events = detail.api_reviews.filter((r) => r.ledger === "coordinator" || r.ledger === "unknown");
  return (
    <Card
      id="ledger"
      title="Coordinator actions"
      aside={<HonestyLabel kind="simulated">Demo ledger, synthetic patients</HonestyLabel>}
      note="Append-only overlay: these never change a decision, consent or signature, and nothing was sent to the patient."
    >
      {events.length === 0 ? (
        <Empty>No coordinator actions recorded for this case.</Empty>
      ) : (
        <ul className="m-0 mt-3 list-none p-0" aria-label="Coordinator actions">
          {events.map((e, i) => (
            <li key={e.hash || `unknown-${i}`} className="border-t border-rule py-3">
              <div className={cn("font-medium", e.ledger === "unknown" && "text-warning")}>
                <TechTerm id={ledgerId(e)} label={ledgerWordsWithHistory(e, events.slice(0, i))} />
              </div>
              <p className="m-0 text-muted">
                {e.actor || "Actor not recorded"} · {e.role ? roleWords(e.role) : "role not recorded"} · {e.at ? atWords(e.at) : "time not recorded"}
              </p>
              {e.ledger === "coordinator" && (e.data.note || e.data.text || e.data.reason) && (
                <p className="m-0 mt-1">&ldquo;{e.data.note || e.data.text || e.data.reason}&rdquo;</p>
              )}
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
            <span className="tabular-nums text-muted">{formatDay(e.day)}</span>
            <span className="min-w-0">
              {eventWords(e, detail.case.id)}
              <span className="block text-small text-muted">
                {providerLabel(e.provider)} · <span className="break-all">{shortId(e.id)}</span>
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
export function BookingCard({ detail, live = apiMode() === "live" }: { detail: CaseDetail; live?: boolean }) {
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
        Simulated only: reservations and suggestions stay in Wasl. No provider system or patient is contacted.
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
      {detail.api_overlay ? (
        <BookingControls key={detail.case.id} caseId={detail.case.id} initial={{ booking: detail.booking, overlay: detail.api_overlay, case: detail.case }} live={live} />
      ) : (
        <p className="m-0 mt-4 text-small text-muted">This older snapshot has no coordinator ledger projection, so reservation controls are unavailable.</p>
      )}
    </Card>
  );
}
