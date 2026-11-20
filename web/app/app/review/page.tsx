import Link from "next/link";
import { redirect } from "next/navigation";
import { PagerLinks } from "@/components/worklist/pager";
import { paginate } from "@/components/worklist/paging";
import { filtersHref, parsePage } from "@/components/worklist/filters";
import { Avatar } from "@/components/avatar/avatar";
import { PURPOSE_LABEL, reviewView } from "@/components/review/case-view";
import { reviewQueue } from "@/components/review/queue";
import { QueueLabel } from "@/components/review/queue-label";
import { apiMode } from "@/lib/api/source";
import { reviewerToken } from "@/lib/api/reviewer-session";
import { claimActionLabel, claimSummary, questionAnchor } from "@/components/review/decision";
import { RecordAvatar } from "@/components/wasl/record-avatar";
import { caseHref } from "@/lib/domain/ids";
import { PageHeader } from "@/components/shell/page-header";
import { Pill } from "@/components/review/marks";
import { stateHeadline } from "@/components/review/decision";

export async function generateMetadata({ searchParams }: PageProps<"/app/review">) {
  const page = parsePage(await searchParams);
  return { title: page > 1 ? `Review, page ${page}` : "Review" };
}

const FAILURE_COPY = {
  401: "Your sign-in has ended. Open a case and sign in again.",
  403: "Your account cannot open them. Ask your administrator to check your access.",
  404: "They could not be opened. Reload the page, or contact your administrator.",
} as const;

export default async function Page({ searchParams }: PageProps<"/app/review">) {
  const query = await searchParams;
  const live = apiMode() === "live";
  const token = live ? await reviewerToken() : undefined;
  const queue = await reviewQueue(token);
  const rows = queue.cases.map((c) => ({ detail: c, view: reviewView(c) }));
  const paging = paginate(rows, parsePage(query));
  const hrefFor = (page: number) => filtersHref("/app/review", {}, page);
  if (parsePage(query) > paging.pages) redirect(hrefFor(paging.pages));
  const complete = queue.source !== "incomplete";
  /** The API's fallback for review_hold: outreach stopped by the guard, and no consent log entry anywhere in the case. */
  const guardOnly =
    rows.length === 1 &&
    rows[0].detail.decisions.some((d) => Array.isArray(d.outreach) && (d.outreach as { suppress?: string }[]).some((o) => o.suppress === "second_identifier_unconfirmed")) &&
    rows[0].detail.decisions.every((d) => d.consent_log.length === 0);

  return (
    // Not clipped: a short queue is shorter than the run notice's drop-down, so the band rounds its own corners.
    <section aria-labelledby="review-desk-title" className="rounded-card border border-line bg-surface">
      <PageHeader
        attached
       
        title={<span id="review-desk-title">Review desk</span>}
        sub={
          <>
            {queue.source !== "incomplete" && <>{rows.length === 1 ? "1 case waits" : `${rows.length} cases wait`} for a person&rsquo;s answer. </>}
            {queue.source !== "open" && (
              <>
                <QueueLabel source={queue.source} />.
              </>
            )}
          </>
        }
        help={["reads"]}
      />
      <div className="rounded-b-[7px] bg-limestone-100 p-4 sm:p-pad [&>*:first-child]:mt-0">
        {query.oidc_error === "1" && (
          <p role="status" className="m-0 mt-0 max-w-[72ch] rounded-control bg-pill-negative-bg px-3 py-2 text-small text-pill-negative-ink" data-testid="signed-out-notice">
            Sign-in could not be completed. Open a case and try again, or contact your administrator.
          </p>
        )}

        {queue.source === "incomplete" && (
          <div role="alert" className="m-0 mt-0 max-w-[72ch] rounded-control bg-pill-negative-bg px-3 py-2 text-small text-pill-negative-ink" data-testid="queue-incomplete">
            <p className="m-0">
              {queue.failures.length === 1 ? "1 open case could not be loaded" : `${queue.failures.length} open cases could not be loaded`}, so this queue is incomplete.
            </p>
            {[...new Set(queue.failures.map((f) => f.status))].map((status) => (
              <p key={status} className="m-0 mt-1">{FAILURE_COPY[status]}</p>
            ))}
          </div>
        )}

        {live && !token && <p className="m-0 mt-3 max-w-[72ch] text-muted">Open a case on the review desk and sign in to claim or answer a question.</p>}

        {queue.source === "rebuilt" && (
          <div className="mt-3 max-w-[72ch] text-muted">
            <p className="m-0">
              No cases are waiting in this demo, so the practice case <span className="font-medium">Held for human review</span> is shown with its{" "}
              {rows[0]?.detail.review_items.length ?? 0} questions cleared.
              {guardOnly && <> It was held because a second identifier was not confirmed, so its appointment reminder was stopped.</>}
            </p>
            {apiMode() === "live" && (
              <p className="m-0 mt-2">This case is already signed. Answers you give here are still saved as new sign-offs. Earlier decisions are not changed.</p>
            )}
          </div>
        )}

        <PagerLinks paging={paging} label="Pages above the list" testId="range-top" hrefFor={hrefFor} complete={complete} />
        <ol className="m-0 mt-3 flex list-none flex-col gap-2 p-0" aria-label="Cases waiting">
          {paging.rows.map(({ detail, view }) => {
            const kinds = detail.review_items.map((r) => (r.kinds.includes("link") ? "Identity" : view.purpose ? PURPOSE_LABEL[view.purpose] : detail.case.programme === "crc" ? "Colonoscopy report" : "Evidence"));
            const questions = (detail.api_review_states ?? []).filter((s) => s.next_read !== null && !s.frozen && s.claim_actions);
            return (
              <li key={view.caseId} className="relative min-w-0 overflow-hidden rounded-card border border-line bg-surface">
                <Link
                  href={caseHref("/app/review", view.caseId)}
                  className="grid min-h-[76px] grid-cols-[3.5rem_minmax(0,1fr)] items-center gap-x-4 gap-y-1 px-4 py-4 text-ink no-underline hover:bg-jade-50 hover:text-ink md:grid-cols-[auto_minmax(0,2fr)_minmax(0,3fr)_auto] md:px-6"
                >
                  <span aria-hidden className="row-span-3 block h-8 w-14 md:row-span-1" />
                  <span className="min-w-0">
                    <span className="staff-display block text-[22px] leading-tight text-jade-900">{view.name}</span>
                    <span className="block break-words text-small text-muted">
                      {view.age > 0 && `${view.age} · `}
                      {view.shortId} · report {view.report.id}
                    </span>
                  </span>
                  <span className="col-start-2 min-w-0 text-small text-muted md:col-start-auto">{view.held}</span>
                  <span className="col-start-2 flex flex-wrap gap-1.5 md:col-start-auto md:justify-end">
                    <span className="sr-only">Review type: </span>
                    <span className="rounded-chip bg-jade-50 px-2 py-0.5 text-small font-semibold text-jade-800">{kinds.join(" · ")}</span>
                  </span>
                </Link>
                {questions.length > 0 && (
                  <div className="border-t border-rule bg-limestone-100">
                    {questions.map((state) => {
                      const action = token ? claimActionLabel(state) : undefined;
                      const headline = stateHeadline(state);
                      return (
                        <div
                          key={`${state.item}:${state.question}`}
                          className="flex min-h-11 flex-wrap items-center gap-x-3 gap-y-1 px-4 py-2 text-small md:pl-[5.5rem] md:pr-6"
                          data-queue-question={`${state.item}:${state.question}`}
                        >
                          <span className="font-semibold">{state.question === "link" ? "Identity" : "Evidence"}</span>
                          <Pill tone={headline.tone} wrap>{headline.text}</Pill>
                          <span className="text-muted">{claimSummary(state)}</span>
                          {action && (
                            <Link
                              href={`${caseHref("/app/review", view.caseId)}#${questionAnchor(state.item, state.question)}`}
                              className="ml-auto inline-flex min-h-11 items-center rounded-control border border-control bg-surface px-3 font-semibold text-ink no-underline hover:bg-active hover:text-ink"
                            >
                              {action}
                            </Link>
                          )}
                        </div>
                      );
                    })}
                  </div>
                )}
                <span className="pointer-events-none absolute left-[17px] top-[38px] flex -translate-y-1/2 items-center md:left-[25px]">
                  <RecordAvatar id={view.caseId} name={view.name} linked className="pointer-events-auto">
                    <Avatar {...view.faces.referral} size={32} label="" />
                  </RecordAvatar>
                  <span className="-ml-2 flex rounded-full ring-2 ring-surface">
                    <Avatar {...view.faces.report} size={32} label={view.faces.report.state === "unknown" ? "Report not yet confirmed as this patient" : ""} />
                  </span>
                </span>
              </li>
            );
          })}
        </ol>
        <PagerLinks paging={paging} label="Pages below the list" testId="range-bottom" hrefFor={hrefFor} complete={complete} />
      </div>
    </section>
  );
}
