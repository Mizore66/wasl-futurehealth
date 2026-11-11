import Link from "next/link";
import { reviewWords, roleWords, atWords } from "@/components/case/words";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import type { DoubleReviewState, Programme, ReadName, Role } from "@/lib/api/schema";
import { cn } from "@/lib/utils";
import { Pill } from "../marks";
import { PHASE_WORDS, READ_LABEL, SEAT_LABEL, roundModel, type ReadCell } from "./model";
import { DoubleReadForm } from "./read-form";

/** What one seat's credential returned for one question role. */
export type SeatView = { role: Role; credential: string } & (
  | { status: "ok"; state: DoubleReviewState }
  | { status: "missing" }
  | { status: "unavailable"; error: string }
);

export type DoubleItem = { item: string; question: "link" | "evidence" };

export function questionText(question: DoubleItem["question"], programme: Programme): string {
  if (question === "link") return "Is it the same patient?";
  return programme === "crc" ? "Is the colonoscopy report adequate?" : "Is the report evidence adequate?";
}

export const QUESTION_ROLE: Record<DoubleItem["question"], Role> = { link: "reviewer", evidence: "clinician" };

function CellRow({ cell, programme }: { cell: ReadCell; programme: Programme }) {
  return (
    <li className="border-t border-rule py-2" data-testid={`read-${cell.read}`}>
      <span className="font-medium">{READ_LABEL[cell.read]}: </span>
      {cell.state === "empty" && <span className="text-muted">not given yet</span>}
      {cell.state === "hidden" && (
        <span>
          given by {cell.actor} ({roleWords(cell.role)}) {atWords(cell.at)}. <em>Answer hidden until your own read is in.</em>
        </span>
      )}
      {cell.state === "shown" && (
        <span>
          {reviewWords(cell.decision, programme)} · {cell.mine ? `you, as ${cell.actor}` : cell.actor} ({roleWords(cell.role)}) · {atWords(cell.at)}
          {cell.note && <span className="block text-small text-muted">&ldquo;{cell.note}&rdquo;</span>}
        </span>
      )}
    </li>
  );
}

/**
 * Independent double review for one case, as one seat's credential sees it. Server-rendered from the API's blinded
 * view: an answer the API withheld is never in this page's HTML.
 */
export function DoubleReviewPanel({
  caseId,
  programme,
  seat,
  hrefFor,
  items,
  views,
}: {
  caseId: string;
  programme: Programme;
  seat: ReadName;
  hrefFor: Record<ReadName, string>;
  items: DoubleItem[];
  views: SeatView[];
}) {
  const viewFor = (role: Role) => views.find((v) => v.role === role);
  return (
    <section id="double-review" aria-labelledby="double-review-h" className="min-w-0 scroll-mt-4 rounded-card border border-line bg-surface p-4 md:p-pad">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <h2 id="double-review-h" className="m-0 text-section font-medium">
          Independent double review
        </h2>
        <HonestyLabel kind="built">Built: demo review log, synthetic patients</HonestyLabel>
      </div>
      <p className="m-0 mt-1 max-w-[80ch] text-small text-muted">
        Two people read each item without seeing each other&rsquo;s answer; if they disagree, a third person resolves it. Each seat below sends its
        own credential from the server, and the API records the person bound to that credential. Switching seats in this browser does not make
        one person into independent reviewers: if two seats are bound to the same person, the API refuses the second initial read.
      </p>

      <nav aria-label="Read as" className="mt-3 flex flex-wrap gap-2">
        {(["r1", "r2", "r3"] as const).map((s) => (
          <Link
            key={s}
            href={hrefFor[s]}
            aria-current={s === seat ? "page" : undefined}
            className={cn(
              "inline-flex min-h-11 items-center rounded-control px-3 no-underline",
              s === seat ? "border-2 border-ink bg-active font-medium text-ink" : "border border-control bg-surface text-ink hover:bg-active",
            )}
          >
            {SEAT_LABEL[s]} ({s})
          </Link>
        ))}
      </nav>

      <ul className="m-0 mt-3 list-none p-0 text-small" aria-label="Who this seat is recorded as">
        {views.map((v) => (
          <li key={v.role} data-testid={`seat-identity-${v.role}`}>
            {v.status === "ok" && (
              <>
                {SEAT_LABEL[seat]} for {v.role === "reviewer" ? "identity" : "report"} questions is recorded by the API as{" "}
                <strong>{v.state.viewer.actor}</strong> ({roleWords(v.state.viewer.role)}).
              </>
            )}
            {v.status === "missing" && (
              <span className="text-warning">
                {SEAT_LABEL[seat]} has no {v.role} credential: <code>{v.credential}</code> is not set on the server. Nothing falls back to another
                seat&rsquo;s credential.
              </span>
            )}
            {v.status === "unavailable" && <span className="text-warning">The API did not return double review for this seat: {v.error}</span>}
          </li>
        ))}
      </ul>

      <ol className="m-0 mt-2 list-none p-0" aria-label="Review items">
        {items.map(({ item, question }) => {
          const view = viewFor(QUESTION_ROLE[question]);
          const round = view?.status === "ok" ? view.state.rounds.find((r) => r.item === item) : undefined;
          const model = round && view?.status === "ok" ? roundModel(round, seat, view.state.viewer.actor, programme) : undefined;
          return (
            <li key={item} className="border-t border-rule py-3" data-testid={`double-round-${item}`}>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h3 className="m-0 text-body font-medium">{questionText(question, programme)}</h3>
                {model && <Pill tone={PHASE_WORDS[model.phase].tone}>{PHASE_WORDS[model.phase].pill}</Pill>}
              </div>
              <p className="m-0 text-small text-muted">{item}</p>
              {!model ? (
                <p className="m-0 mt-2 text-muted">
                  {view?.status === "ok" ? "The API lists no double review for this item." : "No state for this seat: see above."}
                </p>
              ) : (
                <>
                  <ul className="m-0 mt-2 list-none p-0" aria-label="Reads">
                    {model.cells.map((c) => (
                      <CellRow key={c.read} cell={c} programme={programme} />
                    ))}
                  </ul>
                  {model.final && (
                    <p className="m-0 mt-1 font-medium" data-testid="settled">
                      Settled answer: {reviewWords(model.final, programme)}
                    </p>
                  )}
                  {model.form && view?.status === "ok" ? (
                    <DoubleReadForm
                      caseId={caseId}
                      item={item}
                      question={question}
                      seat={model.form.read}
                      head={model.head}
                      choices={model.form.choices}
                      senior={model.form.senior}
                      recordedAs={view.state.viewer.actor}
                      legend={model.form.senior ? "Senior resolution: which answer stands?" : `${READ_LABEL[model.form.read]}: ${questionText(question, programme)}`}
                    />
                  ) : (
                    <p className="m-0 mt-2 text-muted" data-testid="why">
                      {model.why}
                    </p>
                  )}
                </>
              )}
            </li>
          );
        })}
      </ol>
    </section>
  );
}
