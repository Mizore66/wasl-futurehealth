import { notFound } from "next/navigation";
import { Avatar } from "@/components/avatar/avatar";
import { reviewView } from "@/components/review/case-view";
import { reviewQueue } from "@/components/review/queue";
import { QueueLabel } from "@/components/review/queue-label";
import { ReviewDesk, type NextCase } from "@/components/review/review-desk";
import { apiMode, getCase } from "@/lib/api/source";
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
            <Avatar {...view.faces.referral} size={40} label="" />
            <h1 className="m-0 text-title font-semibold">{view.name}</h1>
            {view.nameAr && (
              <span lang="ar" dir="rtl" className="font-arabic text-section text-muted">
                {view.nameAr}
              </span>
            )}
            <span className="text-small text-muted">
              {view.age > 0 && `${view.age} · `}
              <span className="font-mono">{view.shortId}</span>
            </span>
          </div>
          <p className="m-0 mt-1 text-muted">
            {view.held} Referred {view.referredOn}, report {view.report.on}.
          </p>
        </div>
        <div className="flex flex-col items-end gap-1">
          {at >= 0 && (
            <span className="text-small leading-8 text-muted">
              Case {at + 1} of {total} waiting
            </span>
          )}
          <QueueLabel source={queue.source} />
        </div>
      </header>
      <ReviewDesk view={view} mode={apiMode()} next={next} />
    </>
  );
}
