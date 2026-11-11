import { reviewWords, roleWords, atWords } from "@/components/case/words";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import type { DoubleReviewState, Programme, Role } from "@/lib/api/schema";
import { Pill } from "../marks";
import { PHASE_WORDS, READ_LABEL, ownView, roundModel, type ReadCell } from "./model";
import { DoubleReadForm } from "./read-form";

/** What this deployment's one credential for a question role returned. */
export type DeskView = { role: Role; credential: string } & (
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
          given by {cell.actor} ({roleWords(cell.role)}) {atWords(cell.at)}. <em>Answer hidden until both initial reads are in.</em>
        </span>
      )}
      {cell.state === "shown" && (
        <span>
          {reviewWords(cell.decision, programme)} · {cell.mine ? `your read, as ${cell.actor}` : cell.actor} ({roleWords(cell.role)}) · {atWords(cell.at)}
          {cell.note && <span className="block text-small text-muted">&ldquo;{cell.note}&rdquo;</span>}
        </span>
      )}
    </li>
  );
}

/**
 * Independent double review for one case, as this deployment's one credential per question role sees it.
 * Server-rendered from `ownView` of the API's blinded view: unless the API reports the round revealed AND both r1 and
 * r2 are present, the only answer that can reach this page's HTML or RSC payload is this deployment's own (judged by
 * actor name), and no settled answer is shown.
 */
export function DoubleReviewPanel({ caseId, programme, items, views }: { caseId: string; programme: Programme; items: DoubleItem[]; views: DeskView[] }) {
  const own = views.map((v) => (v.status === "ok" ? { ...v, state: ownView(v.state) } : v));
  const viewFor = (role: Role) => own.find((v) => v.role === role);
  return (
    <section id="double-review" aria-labelledby="double-review-h" className="min-w-0 scroll-mt-4 rounded-card border border-line bg-surface p-4 md:p-pad">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <h2 id="double-review-h" className="m-0 text-section font-medium">
          Independent double review
        </h2>
        <HonestyLabel kind="built">Built: demo review log, synthetic patients</HonestyLabel>
      </div>
      <p className="m-0 mt-1 max-w-[80ch] text-small text-muted">
        Two different actors read each item without seeing each other&rsquo;s answer; if they disagree, a third actor resolves it. This deployment
        is one reader: it answers with a single server-held credential per question, and the API records the actor name bound to it. It can give
        at most one read of each item, and the API refuses a second read by the same actor. The API compares actor names only, so the reads are
        independent only if the operator issues one actor name per person. The other reads come through other credentials. Signing in as
        different people on this page is designed, not built.
      </p>

      <ul className="m-0 mt-3 list-none p-0 text-small" aria-label="Who this deployment is recorded as">
        {own.map((v) => (
          <li key={v.role} data-testid={`desk-identity-${v.role}`}>
            {v.status === "ok" && (
              <>
                {v.role === "reviewer" ? "Identity" : "Report"} questions: this deployment is recorded by the API as{" "}
                <strong>{v.state.viewer.actor}</strong> ({roleWords(v.state.viewer.role)}).
              </>
            )}
            {v.status === "missing" && (
              <span className="text-warning">
                No {v.role} credential: <code>{v.credential}</code> is not set on the server, so this deployment cannot read or answer these
                questions.
              </span>
            )}
            {v.status === "unavailable" && <span className="text-warning">The API did not return double review for {v.role} questions: {v.error}</span>}
          </li>
        ))}
      </ul>

      <ol className="m-0 mt-2 list-none p-0" aria-label="Review items">
        {items.map(({ item, question }) => {
          const view = viewFor(QUESTION_ROLE[question]);
          const round = view?.status === "ok" ? view.state.rounds.find((r) => r.item === item) : undefined;
          const model = round && view?.status === "ok" ? roundModel(round, view.state.viewer.actor, programme) : undefined;
          return (
            <li key={item} className="border-t border-rule py-3" data-testid={`double-round-${item}`}>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <h3 className="m-0 text-body font-medium">{questionText(question, programme)}</h3>
                {model && <Pill tone={PHASE_WORDS[model.phase].tone}>{PHASE_WORDS[model.phase].pill}</Pill>}
              </div>
              <p className="m-0 text-small text-muted">{item}</p>
              {!model ? (
                <p className="m-0 mt-2 text-muted">
                  {view?.status === "ok" ? "The API lists no double review for this item." : "No state for this question: see above."}
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
                      read={model.form.read}
                      head={model.head}
                      choices={model.form.choices}
                      senior={model.form.senior}
                      recordedAs={view.state.viewer.actor}
                      legend={model.form.senior ? "Senior resolution: which answer stands?" : `Your read: ${questionText(question, programme)}`}
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
