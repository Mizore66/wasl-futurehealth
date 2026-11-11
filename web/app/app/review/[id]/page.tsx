import Link from "next/link";
import { notFound } from "next/navigation";
import { Avatar } from "@/components/avatar/avatar";
import { HistoryCard } from "@/components/case/audit-view";
import { Card, ProgrammeFindingCard, ReadersCard, SignoffsCard } from "@/components/case/sections";
import { reviewView } from "@/components/review/case-view";
import { reviewQueue } from "@/components/review/queue";
import { QueueLabel } from "@/components/review/queue-label";
import { ReviewDesk, type NextCase } from "@/components/review/review-desk";
import { DoubleReviewSection } from "@/components/review/double-review/section";
import { RecordAvatar } from "@/components/wasl/record-avatar";
import { apiMode, getAudit, getCase } from "@/lib/api/source";
import { caseHref, caseIdFromParam } from "@/lib/domain/ids";

export async function generateMetadata({ params }: PageProps<"/app/review/[id]">) {
  const detail = await getCase(caseIdFromParam((await params).id));
  return { title: detail ? `Review ${reviewView(detail).name}` : "Review" };
}

export default async function Page({ params }: PageProps<"/app/review/[id]">) {
  const id = caseIdFromParam((await params).id);
  const queue = await reviewQueue();
  const detail = queue.cases.find((c) => c.case.id === id) ?? (await getCase(id));
  if (!detail) notFound();
  const view = reviewView(detail);
  /** The append-only history exists only against the live API; a snapshot has nothing the desk could have added. */
  const audit = apiMode() === "live" ? await getAudit(id) : null;

  const total = queue.cases.length;
  const at = queue.cases.findIndex((c) => c.case.id === id);
  const following = at >= 0 ? queue.cases[at + 1] : undefined;
  const next: NextCase = following
    ? { href: caseHref("/app/review", following.case.id), label: `Next case (${at + 2} of ${total})`, last: false }
    : { href: "/app/review", label: "Back to Review", last: at >= 0 };

  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-3">
            <RecordAvatar id={view.caseId} name={view.name} linked>
              <Avatar {...view.faces.referral} size={40} label="" />
            </RecordAvatar>
            <h1 className="m-0 text-title font-semibold">{view.name}</h1>
            {view.nameAr && (
              <span lang="ar" dir="rtl" className="font-arabic text-section text-muted">
                {view.nameAr}
              </span>
            )}
            <span className="text-small text-muted">
              {view.age > 0 && `${view.age} · `}
              {view.shortId}
            </span>
          </div>
          <p className="m-0 mt-1 text-muted">
            {view.held} Referred {view.referredOn}
            {view.report.on && `, report ${view.report.on}`}.
          </p>
        </div>
        <div className="flex flex-col gap-1 sm:items-end">
          {at >= 0 && (
            <span className="text-small text-muted">
              Case {at + 1} of {total} waiting
            </span>
          )}
          <QueueLabel source={queue.source} />
        </div>
      </header>
      {view.guided ? (
        <ReviewDesk
          view={view}
          mode={apiMode()}
          next={next}
          waiting={at >= 0}
          settled={
            <div className="mt-6 flex flex-col gap-3">
              <Card id="not-waiting" title="This case is not waiting for a signature">
                <p className="m-0 mt-3 max-w-[72ch]">
                  {detail.review_items.length === 0
                    ? "No review item was raised for this case. Open the case page to see the recorded evidence and outcomes."
                    : "Every review item on this case already has its answer. The answers are below, and the case page shows what they changed."}
                </p>
                <div className="mt-3 flex flex-wrap gap-3">
                  <Link
                    href={caseHref("/app/patients", view.caseId)}
                    className="inline-flex min-h-11 items-center rounded-control bg-cta px-4 font-medium text-white no-underline hover:bg-ink hover:text-white"
                  >
                    Open the case
                  </Link>
                  <Link
                    href="/app/review"
                    className="inline-flex min-h-11 items-center rounded-control border border-control bg-surface px-4 text-ink no-underline hover:bg-active hover:text-ink"
                  >
                    Back to Review
                  </Link>
                </div>
              </Card>
              <SignoffsCard detail={detail} />
            </div>
          }
        />
      ) : (
        <div className="mt-6 grid items-start gap-3 lg:grid-cols-2">
          <ProgrammeFindingCard detail={detail} />
          {detail.evidence.map((ev, i) => (
            <ReadersCard key={ev.report} detail={detail} ev={ev} id={`programme-readers-${i}`} />
          ))}
          <SignoffsCard detail={detail} />
        </div>
      )}
      {/* Independent double review (issue #41): this deployment is one reader, with one credential per question role. */}
      <div className="mt-3">
        <DoubleReviewSection detail={detail} />
      </div>
      {audit && (
        <div className="mt-3">
          <HistoryCard audit={audit} programme={detail.case.programme} />
        </div>
      )}
    </>
  );
}
