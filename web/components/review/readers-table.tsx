import { readerLabels } from "@/components/worklist/model";
import { plain } from "@/lib/domain/terms";
import type { AutomatedReaderEvidence, Check, CheckDecision, ReaderValue, ReviewView } from "./case-view";
import type { Outcome } from "./decision";
import { Mark, Pill, type Shape, type Tone } from "./marks";

const READ: Record<ReaderValue, { shape: Shape; tone: Tone; word: string }> = {
  yes: { shape: "tick", tone: "positive", word: "yes" },
  adequate: { shape: "tick", tone: "positive", word: "adequate" },
  no: { shape: "dash", tone: "muted", word: "none" },
  inadequate: { shape: "cross", tone: "negative", word: "not adequate" },
  not_stated: { shape: "cross", tone: "negative", word: "not stated" },
};

const AGREED: Record<Exclude<CheckDecision, "disputed">, string> = {
  agreed: "agreed",
  agreed_none: "agreed, none",
  not_needed: "not needed",
};

const SHAPE_FOR: Record<Tone, Shape> = { positive: "tick", negative: "cross", warning: "dash", muted: "dash" };

function Decision({ check, signed }: { check: Check; signed?: Outcome["answerCell"] }) {
  if (check.decision !== "disputed")
    return (
      <Mark shape="tick" tone="positive">
        {AGREED[check.decision]}
      </Mark>
    );
  if (!signed)
    return (
      <Pill tone="warning" shape="dash">
        reviewers decide
      </Pill>
    );
  if (signed.tone === "muted")
    return (
      <Mark shape="dash" tone="muted" className="text-muted">
        {signed.word}
      </Mark>
    );
  return (
    <Pill tone={signed.tone} shape={SHAPE_FOR[signed.tone]}>
      {signed.word}
    </Pill>
  );
}

const ROW = "grid grid-cols-1 gap-y-1 @xl:grid-cols-[minmax(0,34fr)_minmax(0,22fr)_minmax(0,22fr)_minmax(0,22fr)] @xl:items-center";
/** The column name, shown on each value only when the columns have folded into a card. */
const Label = ({ children }: { children: string }) => <span className="mr-2 inline-block w-28 flex-none text-small text-muted @xl:hidden">{children}</span>;

function automatedRows(ev: AutomatedReaderEvidence): { field: string; a: string; b: string }[] {
  const a = ev.reader_a_fields;
  const b = ev.reader_b_fields;
  if (typeof a === "string" || typeof b === "string") {
    return [{ field: "Reader note", a: typeof a === "string" && a ? a : "Unavailable", b: typeof b === "string" && b ? b : "Unavailable" }];
  }
  const keys = [...new Set([...Object.keys(a ?? {}), ...Object.keys(b ?? {})])].filter((key) => key !== "quote");
  return keys.map((key) => ({ field: plain(key), a: a?.[key] === undefined ? "Unavailable" : String(a?.[key]), b: b?.[key] === undefined ? "Unavailable" : String(b?.[key]) }));
}

/** Client-safe automated facts only; never another human's independent answer. */
function AutomatedReadersTable({ view }: { view: ReviewView }) {
  const ev = view.automatedEvidence;
  // The snapshot is already explicitly whitelisted. Do not use the overall
  // label as a substitute for a missing individual reader's output.
  const labels = ev && readerLabels({ ...ev, label: "" });
  const rows = ev ? automatedRows(ev) : [];
  const quoteB = typeof ev?.reader_b_fields === "object" ? ev.reader_b_fields.quote : undefined;
  return (
    <section aria-labelledby={`automated-readers-h-${view.items.evidence ?? "case"}`} className="@container rounded-card border border-line bg-surface p-4 md:p-6">
      <h2 id={`automated-readers-h-${view.items.evidence ?? "case"}`} className="m-0 text-section font-medium">What the automatic readers found</h2>
      <p className="m-0 mt-1 text-small text-muted">
        {view.unit} report {view.report.id}{view.report.version ? ` version ${view.report.version}` : ""} · {view.report.on || "Date unavailable"}
      </p>
      {!ev ? <p className="m-0 mt-3 text-small text-muted">No automatic reading for this item.</p> : (
        <>
          <dl className="m-0 mt-3 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 text-small">
            {labels?.checks.map((check) => (
              <div key={check.name} className="contents"><dt>{check.name}</dt><dd className="m-0">{check.result || "Unavailable"}</dd></div>
            ))}
          </dl>
          {rows.length ? (
            <table className="mt-3 w-full border-collapse text-left text-small">
              <caption className="pb-1 text-left text-muted">What each automatic reader found</caption>
              <thead><tr><th scope="col" className="py-1 pr-3 font-normal">Item</th><th scope="col" className="py-1 pr-3 font-normal">Reader A</th><th scope="col" className="py-1 font-normal">Reader B</th></tr></thead>
              <tbody>{rows.map((row) => (
                <tr key={row.field} className="border-t border-rule align-top"><th scope="row" className="py-2 pr-3 font-normal">{row.field}</th><td className="py-2 pr-3 [overflow-wrap:anywhere]">{row.a}</td><td className="py-2 [overflow-wrap:anywhere]">{row.b}</td></tr>
              ))}</tbody>
            </table>
          ) : <p className="m-0 mt-3 text-small text-muted">No details from the automatic readers.</p>}
          <blockquote className="m-0 mt-3 rounded-control bg-raised px-4 py-3 text-small">
            <p className="m-0">{labels?.quote ? `“${labels.quote}”` : "No quote available."}</p>
            {quoteB && quoteB !== labels?.quote ? <p className="m-0 mt-2">Reader B quotes: “{quoteB}”</p> : !quoteB && <p className="m-0 mt-2 text-muted">Reader B quote unavailable.</p>}
          </blockquote>
          <p className="m-0 mt-2 text-small text-muted">Reader B is an AI model.</p>
        </>
      )}
    </section>
  );
}

/** Two readers side by side, one row per closure check, with the report quote below. */
export function ReadersTable({ view, signed }: { view: ReviewView; signed?: Outcome["answerCell"] }) {
  if (view.programme !== "crc") return <AutomatedReadersTable view={view} />;
  return (
    <section aria-labelledby={`readers-h-${view.items.evidence ?? "case"}`} className="@container rounded-card border border-line bg-surface p-4 md:p-6">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <h2 id={`readers-h-${view.items.evidence ?? "case"}`} className="m-0 text-section font-medium">
          {view.evidenceScope ? view.evidenceQuestion : "Is the report enough to close her case?"}
        </h2>
        <span className="text-small text-muted">
          {view.unit} report {view.report.id}{view.report.version ? ` version ${view.report.version}` : ""} · {view.report.on}
        </span>
      </div>
      {view.evidenceScope && <p className="m-0 mt-1 text-small text-muted">{view.evidenceScope}</p>}
      <div className="mt-2" data-testid="readers">
        <div className="hidden h-8 grid-cols-[minmax(0,34fr)_minmax(0,22fr)_minmax(0,22fr)_minmax(0,22fr)] items-center text-small text-muted @xl:grid">
          <span>Check</span>
          <span>Rules reader</span>
          <span>Second reader</span>
          <span>Decision</span>
        </div>
        {view.checks.map((c) => (
          <div key={c.field} data-check={c.field} className={`${ROW} min-h-10 border-t border-rule py-2 @xl:py-0`}>
            <span className="font-medium @xl:font-normal">{c.field}</span>
            <span className="flex items-center">
              <Label>Rules reader</Label>
              <Mark {...READ[c.rules]}>{READ[c.rules].word}</Mark>
            </span>
            <span className="flex items-center">
              <Label>Second reader</Label>
              <Mark {...READ[c.second]}>{READ[c.second].word}</Mark>
            </span>
            <span className="flex items-center">
              <Label>Decision</Label>
              <Decision check={c} signed={view.evidenceField && c.field !== view.evidenceField ? undefined : signed} />
            </span>
          </div>
        ))}
      </div>
      {view.quote && (
        <blockquote className="m-0 mt-3 rounded-control bg-raised px-4 py-3">
          <p className="m-0">
            “<mark className="bg-pill-warning-bg text-inherit">{view.quote}</mark>”
          </p>
          <p className="m-0 mt-1 text-small text-muted">
            The words the readers cite from the report.
            {view.disputed.length > 0 && ` The readers could not settle ${view.disputed.length === 1 ? "one check" : `${view.disputed.length} checks`}, so a clinician decides.`}
          </p>
        </blockquote>
      )}
    </section>
  );
}
