import Link from "next/link";
import { Avatar } from "@/components/avatar/avatar";
import { reviewView } from "@/components/review/case-view";
import { reviewQueue } from "@/components/review/queue";
import { QueueLabel } from "@/components/review/queue-label";
import { apiMode } from "@/lib/api/source";
import { RecordAvatar } from "@/components/wasl/record-avatar";
import { caseHref } from "@/lib/domain/ids";

export const metadata = { title: "Review" };

export default async function Page({ searchParams }: PageProps<"/app/review">) {
  const query = await searchParams;
  const queue = await reviewQueue();
  const rows = queue.cases.map((c) => ({ detail: c, view: reviewView(c) }));
  /** The API's fallback for review_hold: outreach stopped by the guard, and no consent log entry anywhere in the case. */
  const guardOnly =
    rows.length === 1 &&
    rows[0].detail.decisions.some((d) => Array.isArray(d.outreach) && (d.outreach as { suppress?: string }[]).some((o) => o.suppress === "second_identifier_unconfirmed")) &&
    rows[0].detail.decisions.every((d) => d.consent_log.length === 0);

  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="m-0 text-title font-semibold">Review</h1>
          <p className="m-0 mt-1 text-muted">
            {rows.length === 1 ? "1 case waits" : `${rows.length} cases wait`} for a person&rsquo;s answer. Nothing closes without it.
          </p>
        </div>
        <QueueLabel source={queue.source} />
      </header>

      {query.oidc_error === "1" && (
        <p role="status" className="m-0 mt-3 max-w-[72ch] rounded-control bg-pill-negative-bg px-3 py-2 text-small text-pill-negative-ink" data-testid="signed-out-notice">
          OIDC sign-in could not be completed. Try again from a review case or contact the operator.
        </p>
      )}

      {queue.source === "rebuilt" && (
        <div className="mt-3 max-w-[72ch] text-muted">
          <p className="m-0">
            The sample run has no open review items: its simulated reviewers signed them all. This queue rebuilds one from the demo case{" "}
            <span className="font-medium">Held for human review</span>, using its {rows[0]?.detail.review_items.length ?? 0} review items as they stood before they were signed.
            {guardOnly && (
              <>
                {" "}
                In this run no loop had consent held and then granted, so this case is the run&rsquo;s closed, signed loop whose appointment reminder was
                stopped by the second-identifier guard, not by a consent hold.
              </>
            )}
          </p>
          {apiMode() === "live" && (
            <p className="m-0 mt-2">
              This screen is connected to the live API, which already shows that case as signed. Answers given here are still sent: each is
              appended to the API&rsquo;s review ledger as a new sign-off. The run&rsquo;s recorded decisions and earlier sign-offs are not changed.
            </p>
          )}
          <details className="text-small">
            <summary className="inline-flex min-h-11 cursor-pointer items-center">Show the system name of that case</summary>
            <code className="font-mono">review_hold</code>
          </details>
        </div>
      )}

      <ol className="m-0 mt-6 flex list-none flex-col gap-2 p-0">
        {rows.map(({ detail, view }) => (
          <li key={view.caseId} className="relative">
            <Link
              href={caseHref("/app/review", view.caseId)}
              className="grid min-h-[72px] grid-cols-[3.5rem_minmax(0,1fr)] items-center gap-x-4 gap-y-1 rounded-card border border-line bg-surface px-4 py-4 text-ink no-underline hover:bg-active hover:text-ink md:grid-cols-[auto_minmax(0,2fr)_minmax(0,3fr)_auto] md:px-6"
            >
              <span aria-hidden className="row-span-3 block h-8 w-14 md:row-span-1" />
              <span className="min-w-0">
                <span className="block font-medium">{view.name}</span>
                <span className="block break-words text-small text-muted">
                  {view.age > 0 && `${view.age} · `}
                  {view.shortId} · report {view.report.id}
                </span>
              </span>
              <span className="col-start-2 min-w-0 text-muted md:col-start-auto">{view.held}</span>
              <span className="col-start-2 text-small text-muted md:col-start-auto">
                {detail.review_items.map((r) => (r.kinds.includes("link") ? "Identity" : detail.case.programme === "crc" ? "Colonoscopy report" : "Evidence")).join(" · ")}
              </span>
            </Link>
            <span className="pointer-events-none absolute left-[17px] top-1/2 flex -translate-y-1/2 items-center md:left-[25px]">
              <RecordAvatar id={view.caseId} name={view.name} linked className="pointer-events-auto">
                <Avatar {...view.faces.referral} size={32} label="" />
              </RecordAvatar>
              <span className="-ml-2 flex rounded-full ring-2 ring-surface">
                <Avatar {...view.faces.report} size={32} label={view.faces.report.state === "unknown" ? "Report not yet confirmed as this patient" : ""} />
              </span>
            </span>
          </li>
        ))}
      </ol>
    </>
  );
}
