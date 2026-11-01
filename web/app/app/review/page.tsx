import Link from "next/link";
import { Avatar } from "@/components/avatar/avatar";
import { reviewView } from "@/components/review/case-view";
import { reviewQueue } from "@/components/review/queue";
import { QueueLabel } from "@/components/review/queue-label";
import { caseHref } from "@/lib/domain/ids";

export const metadata = { title: "Review" };

export default async function Page() {
  const queue = await reviewQueue();
  const rows = queue.cases.map((c) => ({ detail: c, view: reviewView(c) }));

  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="m-0 text-title font-semibold">Review</h1>
          <p className="m-0 mt-1 text-muted">
            {rows.length === 1 ? "1 case waits" : `${rows.length} cases wait`} for a second signature. Nothing closes on one.
          </p>
        </div>
        <QueueLabel source={queue.source} />
      </header>

      {queue.source === "rebuilt" && (
        <p className="m-0 mt-3 max-w-[72ch] text-small text-muted">
          The sample run has no open review items: its simulated reviewers signed them all. This queue rebuilds one from the review_hold case&rsquo;s{" "}
          {rows[0]?.detail.review_items.length ?? 0} review items, as they stood before they were signed.
        </p>
      )}

      <ol className="m-0 mt-6 flex list-none flex-col gap-2 p-0">
        {rows.map(({ detail, view }) => (
          <li key={view.caseId}>
            <Link
              href={caseHref("/app/review", view.caseId)}
              className="grid grid-cols-[auto_minmax(0,2fr)_minmax(0,3fr)_auto] items-center gap-4 rounded-card border border-line bg-surface px-6 py-4 text-ink no-underline hover:bg-active hover:text-ink"
            >
              <span className="flex items-center">
                <Avatar {...view.faces.referral} size={32} label="" />
                <span className="-ml-2 flex rounded-full ring-2 ring-surface">
                  <Avatar {...view.faces.report} size={32} label={view.faces.report.state === "unknown" ? "Report not yet confirmed as this patient" : ""} />
                </span>
              </span>
              <span className="min-w-0">
                <span className="block font-medium">{view.name}</span>
                <span className="block text-small text-muted">
                  {view.age > 0 && `${view.age} · `}
                  <span className="font-mono">{view.shortId}</span> · report <span className="font-mono">{view.report.id}</span>
                </span>
              </span>
              <span className="min-w-0 text-muted">{view.held}</span>
              <span className="text-small text-muted">
                {detail.review_items.map((r) => (r.kinds.includes("link") ? "Identity" : "Bowel prep")).join(" · ")}
              </span>
            </Link>
          </li>
        ))}
      </ol>
    </>
  );
}
