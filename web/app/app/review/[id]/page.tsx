import Link from "next/link";
import { notFound } from "next/navigation";
import { Avatar } from "@/components/avatar/avatar";
import { HistoryCard } from "@/components/case/audit-view";
import { Card, ProgrammeFindingCard, ReadersCard, SignoffsCard } from "@/components/case/sections";
import { reviewItemViews, reviewView } from "@/components/review/case-view";
import { reviewQueue } from "@/components/review/queue";
import { QueueLabel } from "@/components/review/queue-label";
import { ReviewDesk, type NextCase } from "@/components/review/review-desk";
import { RecordAvatar } from "@/components/wasl/record-avatar";
import { ReviewerLogin, ReviewerSessionStatus } from "@/components/review/reviewer-login";
import { gatedHistory } from "@/lib/api/protocol-history";
import { getReviewCase } from "@/lib/api/reviewer-session";
import { apiMode } from "@/lib/api/source";
import { caseHref, caseIdFromParam } from "@/lib/domain/ids";
import { reviewAuthMode } from "@/lib/auth/oidc-config";

/* The title never reads the case: a shared credential must not be a second path to review data. */
export const metadata = { title: "Review" };

export default async function Page({ params, searchParams }: PageProps<"/app/review/[id]">) {
  const id = caseIdFromParam((await params).id);
  const query = await searchParams;
  const authMode = reviewAuthMode();
  const live = apiMode() === "live";
  /* The viewer's own credential reads the case. Without one, live review shows the sign-in form and nothing else. */
  const viewer = await getReviewCase(id);
  if (!viewer.authenticated) {
    return (
      <>
        <h1 className="m-0 text-title font-semibold">Review</h1>
        <ReviewerLogin caseId={id} mode={authMode} authError={query.oidc_error === "1"} />
      </>
    );
  }
  /* Live mode reads only with the viewer's credential. The shared-credential queue is a fixtures-mode convenience. */
  const queue = live ? undefined : await reviewQueue();
  const detail = queue?.cases.find((c) => c.case.id === id) ?? viewer.detail;
  if (!detail) notFound();
  const view = reviewView(detail);
  const sharedEvidence = view.programme !== "crc" && view.programme !== "cervical" ? (
    <>
      <ProgrammeFindingCard detail={detail} />
      {detail.evidence.map((ev, i) => (
        <ReadersCard key={ev.report} detail={detail} ev={ev} id={`programme-readers-${i}`} />
      ))}
    </>
  ) : undefined;
  /*
   * The append-only history and governance projection exist only against the live API. They are read with shared
   * credentials, so gatedHistory withholds them while any first read is concealed, re-checking after the fetch.
   */
  const history = live ? await gatedHistory(id, { before: viewer }) : { status: "ok" as const, audit: null, projection: null, signedIn: false };
  const concealed = history.status === "concealed";
  const { audit, projection } = history;

  const total = queue?.cases.length ?? 0;
  const at = queue ? queue.cases.findIndex((c) => c.case.id === id) : live ? 0 : -1;
  const following = queue && at >= 0 ? queue.cases[at + 1] : undefined;
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
          {queue && at >= 0 && (
            <span className="text-small text-muted">
              Case {at + 1} of {total} waiting
            </span>
          )}
          {queue && <QueueLabel source={queue.source} />}
        </div>
      </header>
      {viewer.session && <ReviewerSessionStatus session={viewer.session} />}
      {view.guided ? (
        <ReviewDesk
          view={view}
          itemViews={reviewItemViews(detail)}
          mode={apiMode()}
          next={next}
          sharedEvidence={sharedEvidence}
          waiting={at >= 0}
          states={detail.api_review_states}
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
              {sharedEvidence}
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
      {concealed && (
        <p className="mt-3 max-w-[72ch] text-muted" role="status" data-testid="history-withheld">
          The review history is withheld while a first read is hidden. It appears once the second read is in.
        </p>
      )}
      {audit && (
        <div className="mt-3">
          {/* A guided live desk always shows its own reviewer sign-out; the history card then does not add a second one. */}
          <HistoryCard
            audit={audit}
            programme={detail.case.programme}
            governance={projection ? { caseId: id, projection, signedIn: history.signedIn, hostSignOut: view.guided, mode: viewer.session?.method, expiresAt: viewer.session?.expiresAt } : undefined}
          />
        </div>
      )}
    </>
  );
}
