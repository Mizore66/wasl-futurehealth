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

function Decision({ check, signed }: { check: Check; signed?: Outcome["prepCell"] }) {
  if (check.decision !== "disputed") return <Mark shape="tick" tone="positive">{AGREED[check.decision]}</Mark>;
  if (!signed) return <Pill tone="warning" shape="dash">reviewers decide</Pill>;
  if (signed.tone === "muted") return <Mark shape="dash" tone="muted" className="text-muted">{signed.word}</Mark>;
  return <Pill tone={signed.tone} shape={SHAPE_FOR[signed.tone]}>{signed.word}</Pill>;
}

const ROW = "grid grid-cols-[minmax(0,34fr)_minmax(0,22fr)_minmax(0,22fr)_minmax(0,22fr)] items-center";

/** Two readers side by side, one row per closure check, with the report quote below. */
export function ReadersTable({ view, signed }: { view: ReviewView; signed?: Outcome["prepCell"] }) {
  return (
    <section aria-labelledby="readers-h" className="rounded-card border border-line bg-surface p-6">
      <div className="flex items-baseline justify-between gap-3">
        <h2 id="readers-h" className="m-0 text-section font-medium">
          Is the report enough to close her case?
        </h2>
        <span className="text-small text-muted">
          {view.unit} report <span className="font-mono">{view.report.id}</span> · {view.report.on}
        </span>
      </div>
      <div className="mt-2" data-testid="readers">
        <div className={`${ROW} h-8 text-small text-muted`}>
          <span>Check</span>
          <span>Rules reader</span>
          <span>Second reader</span>
          <span>Decision</span>
        </div>
        {view.checks.map((c) => (
          <div key={c.field} data-check={c.field} className={`${ROW} min-h-10 border-t border-rule`}>
            <span>{c.field}</span>
            <Mark {...READ[c.rules]}>{READ[c.rules].word}</Mark>
            <Mark {...READ[c.second]}>{READ[c.second].word}</Mark>
            <span>
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
            {view.checks.some((c) => c.field === "Prep graded" && c.disputed) && " Neither reader found a bowel prep grade, so a clinician decides."}
          </p>
        </blockquote>
      )}
    </section>
  );
}
