import Link from "next/link";
import { notFound } from "next/navigation";
import { Avatar } from "@/components/avatar/avatar";
import { PageHeader } from "@/components/shell/page-header";
import { HistoryCard } from "@/components/case/audit-view";
import { Card, ProgrammeFindingCard, ReadersCard, SignoffsCard } from "@/components/case/sections";
import { reviewItemViews, reviewView } from "@/components/review/case-view";
import { reviewQueue } from "@/components/review/queue";
import { QueueLabel } from "@/components/review/queue-label";
import { ReviewDesk, type NextCase } from "@/components/review/review-desk";
import { RecordAvatar } from "@/components/wasl/record-avatar";
import { ReviewerLogin, ReviewerSessionStatus } from "@/components/review/reviewer-login";
import { gatedHistory } from "@/lib/api/protocol-history";
import { getReviewCase, reviewerPersona } from "@/lib/api/reviewer-session";
import { apiMode, getFixtureReviewSource } from "@/lib/api/source";
import { caseHref, caseIdFromParam } from "@/lib/domain/ids";
import { reviewAuthMode } from "@/lib/auth/oidc-config";
import { GlossaryPopover } from "@/components/wasl/glossary-popover";
import { settledWords } from "@/components/case/crc-confirmation";

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
      <section aria-labelledby="review-title" className="rounded-card border border-line bg-surface">
        <PageHeader attached run title={<span id="review-title">Review desk</span>} sub="Sign in to see the case." />
        <div className="rounded-b-[7px] bg-limestone-100 p-4 sm:p-pad [&>*:first-child]:mt-0">
          <ReviewerLogin caseId={id} mode={authMode} authError={query.oidc_error === "1"} reason={viewer.reason === "anonymous" ? undefined : viewer.reason} />
        </div>
      </section>
    );
  }
  /* Live mode reads only with the viewer's credential. The shared-credential queue is a fixtures-mode convenience. */
  const queue = live ? undefined : await reviewQueue();
  const detail = queue?.cases.find((c) => c.case.id === id) ?? viewer.detail;
  if (!detail) notFound();
  if (detail.case.lifecycle === "retained")
    return (
      <section aria-labelledby="review-title" className="rounded-card border border-line bg-surface">
        <PageHeader attached run title={<span id="review-title">Retired case: review history only</span>} />
        <div className="p-4 sm:p-pad">
          <p className="m-0">No new claim, clinical read or review work is available from this tracking request.</p>
          <Link href={caseHref("/app/patients", id)} className="mt-2 inline-flex min-h-11 items-center underline">
            Open the retained case and authorized audit history
          </Link>
        </div>
      </section>
    );
  const view = reviewView(detail);
  /* Fixtures (simulated): read the evidence item's committed synthetic report here, so the browser makes no request for it. */
  const fixtureSource = !live && view.items.evidence ? await getFixtureReviewSource(id, view.items.evidence) : undefined;
  const sharedEvidence = detail.case.crc || view.programme !== "crc" && view.programme !== "cervical" ? (
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
      {/* Not clipped: the desk's own cards and popovers must not be cut, so the band rounds its own corners. */}
      <section aria-labelledby="review-title" className="rounded-card border border-line bg-surface">
        <PageHeader
          attached
          run
         
          title={<span id="review-title">{view.items.link ? "Identity review" : view.items.evidence ? "Evidence review" : "Review"}</span>}
          sub={
            <>
              <span className="font-semibold text-ink">{view.shortId}</span>
              {view.held && <span className="mt-1 block max-w-[72ch]">{view.held}</span>}
            </>
          }
          aside={
            <div className="flex flex-col gap-1 sm:items-end">
              {/* Fixtures only: live review shows no patient identity before the viewer claims a read. */}
              {!live && (
                <RecordAvatar id={view.caseId} name={view.name} linked>
                  <Avatar {...view.faces.referral} size={36} label="" />
                </RecordAvatar>
              )}
              {queue && at >= 0 && (
                <span className="text-small text-muted">
                  Case {at + 1} of {total} waiting
                </span>
              )}
              {queue && <QueueLabel source={queue.source} />}
              <GlossaryPopover terms={["reads", "canonical", "overlay"]} />
            </div>
          }
        />
        <div className="rounded-b-[7px] bg-limestone-100 p-4 sm:p-pad [&>*:first-child]:mt-0">
          {viewer.session && <ReviewerSessionStatus session={viewer.session} persona={viewer.session.method === "oidc" ? await reviewerPersona() : null} />}
          {view.guided ? (
            <ReviewDesk
              view={view}
              itemViews={reviewItemViews(detail)}
              mode={apiMode()}
              fixtureSource={fixtureSource}
              next={next}
              sharedEvidence={sharedEvidence}
              waiting={at >= 0}
              states={detail.api_review_states}
              settled={
                <div className="mt-3 flex flex-col gap-3">
                  <Card id="not-waiting" title={settledWords(detail).title}>
                    <p className="m-0 mt-3 max-w-[72ch]">{settledWords(detail).body}</p>
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
            <div className="mt-3 grid items-start gap-3 lg:grid-cols-2">
              <ProgrammeFindingCard detail={detail} />
              {detail.evidence.map((ev, i) => (
                <ReadersCard key={ev.report} detail={detail} ev={ev} id={`programme-readers-${i}`} />
              ))}
              <SignoffsCard detail={detail} />
            </div>
          )}
        </div>
      </section>
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
