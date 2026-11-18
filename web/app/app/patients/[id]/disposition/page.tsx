import Link from "next/link";
import { notFound } from "next/navigation";
import { DispositionCard } from "@/components/disposition/disposition-card";
import { DispositionForm } from "@/components/disposition/disposition-form";
import { dispositionContext } from "@/components/disposition/model";
import { clinicianSignInWords } from "@/components/review/auth";
import { ReviewerLogin, ReviewerSessionStatus } from "@/components/review/reviewer-login";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { getReviewCase, reviewerPersona } from "@/lib/api/reviewer-session";
import { apiMode } from "@/lib/api/source";
import { reviewAuthMode } from "@/lib/auth/oidc-config";
import { caseHref, caseIdFromParam, shortId } from "@/lib/domain/ids";

/* The title never reads the case: the case is read only with the signed-in clinician's own credential. */
export const metadata = { title: "Clinician disposition" };

/*
 * The clinician disposition workflow, reachable directly from the case. It needs no raised review item: the
 * no-fundal-view and outside-Fleischner holds raise none, and a patient decline is a coordinator contact outcome.
 */
export default async function Page({ params, searchParams }: PageProps<"/app/patients/[id]/disposition">) {
  const id = caseIdFromParam((await params).id);
  const query = await searchParams;
  const live = apiMode() === "live";
  const back = (
    <p className="m-0">
      <Link href={caseHref("/app/patients", id)} className="inline-flex min-h-11 items-center">
        ← Back to the case
      </Link>
    </p>
  );
  const viewer = await getReviewCase(id);
  if (!viewer.authenticated) {
    const authMode = reviewAuthMode();
    return (
      <>
        {back}
        <h1 className="m-0 text-title font-semibold">Clinician disposition</h1>
        <p className="m-0 mt-1 max-w-[72ch] text-muted">Only a clinician or senior clinician may record a disposition. {clinicianSignInWords(authMode)}</p>
        <ReviewerLogin
          caseId={id}
          mode={authMode}
          authError={query.oidc_error === "1"}
          reason={viewer.reason === "anonymous" ? undefined : viewer.reason}
          returnTo={`${caseHref("/app/patients", id)}/disposition`}
        />
      </>
    );
  }
  const detail = viewer.detail;
  if (!detail) notFound();
  const ctx = dispositionContext(detail);
  return (
    <>
      {back}
      <header className="flex flex-wrap items-start justify-between gap-gap">
        <div>
          <h1 className="m-0 text-title font-semibold">Clinician disposition</h1>
          <p className="m-0 mt-1 text-muted">
            Case {shortId(detail.case.id)} · {detail.case.programme.replaceAll("_", " ")} · screening record {detail.case.closed ? "closed by evidence" : "open"}
          </p>
          {live && viewer.session && <ReviewerSessionStatus session={viewer.session} persona={viewer.session.method === "oidc" ? await reviewerPersona() : null} />}
        </div>
        <HonestyLabel kind="simulated">Synthetic case</HonestyLabel>
      </header>

      <div className="mt-6 grid items-start gap-gap lg:grid-cols-2">
        <DispositionCard ctx={ctx} link={false} />
        <section aria-labelledby="disposition-record-h" className="rounded-card border border-line bg-surface p-4 md:p-pad">
          <h2 id="disposition-record-h" className="m-0 text-section font-medium">
            Record a disposition
          </h2>
          {ctx.open.length === 0 ? (
            <p className="m-0 mt-3 text-muted" data-testid="disposition-none">
              Nothing waits on a clinician here: the case has no clinician-decision hold and no unanswered patient decline.
            </p>
          ) : !live ? (
            <p className="m-0 mt-3 text-muted" data-testid="disposition-fixtures">
              Fixtures mode: a disposition can be recorded only against the live API.
            </p>
          ) : (
            ctx.open.map((kind) => (
              <div key={kind} className="mt-4">
                <h3 className="m-0 font-medium">{kind === "clinician_disposition" ? "Plan for the clinician hold" : "Informed refusal after the patient declined"}</h3>
                <p className="m-0 text-small text-muted">
                  {kind === "clinician_disposition"
                    ? "Saving records a plan with an owner and review date. The hold stays until qualifying evidence arrives through the pipeline."
                    : "Saving answers the coordinator-recorded decline. It never records the procedure as done."}
                </p>
                <DispositionForm caseId={detail.case.id} expectedHead={ctx.expectedHead} kind={kind} crc={detail.case.programme === "crc"} />
              </div>
            ))
          )}
        </section>
      </div>
    </>
  );
}
