import type { Check, CheckDecision, ReaderValue, ReviewView } from "./case-view";
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

/** Two readers side by side, one row per closure check, with the report quote below. */
export function ReadersTable({ view, signed }: { view: ReviewView; signed?: Outcome["answerCell"] }) {
  return (
    <section aria-labelledby={`readers-h-${view.items.evidence ?? "case"}`} className="@container rounded-card border border-line bg-surface p-4 md:p-6">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <h2 id={`readers-h-${view.items.evidence ?? "case"}`} className="m-0 text-section font-medium">
          Is the report enough to close her case?
        </h2>
        <span className="text-small text-muted">
          {view.unit} report {view.report.id}{view.report.version ? ` version ${view.report.version}` : ""} · {view.report.on}
        </span>
      </div>
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
              <Decision check={c} signed={signed} />
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
