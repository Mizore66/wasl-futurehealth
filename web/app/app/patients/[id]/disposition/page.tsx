import Link from "next/link";
import { notFound } from "next/navigation";
import { DispositionCard } from "@/components/disposition/disposition-card";
import { DispositionForm } from "@/components/disposition/disposition-form";
import { dispositionContext } from "@/components/disposition/model";
import { clinicianSignInWords } from "@/components/review/auth";
import { ReviewerLogin, ReviewerSessionStatus } from "@/components/review/reviewer-login";
import { PageHeader } from "@/components/shell/page-header";
import { getReviewCase, reviewerPersona } from "@/lib/api/reviewer-session";
import { apiMode } from "@/lib/api/source";
import { reviewAuthMode } from "@/lib/auth/oidc-config";
import { caseHref, caseIdFromParam, shortId } from "@/lib/domain/ids";

/* The title never reads the case: the case is read only with the signed-in clinician's own credential. */
export const metadata = { title: "Clinician disposition" };

/** The page's main card; the header is its top band. Its body is taller than the Run metadata drop-down. */
const CARD = "overflow-clip rounded-card border border-line bg-surface";

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
        <div className={CARD}>
          <PageHeader attached compact title="Clinician disposition" />
          <div className="p-4 md:p-pad">
            <p className="m-0 max-w-[72ch] text-muted">Only a clinician or senior clinician may record a disposition. {clinicianSignInWords(authMode)}</p>
            <ReviewerLogin
              caseId={id}
              mode={authMode}
              authError={query.oidc_error === "1"}
              reason={viewer.reason === "anonymous" ? undefined : viewer.reason}
              returnTo={`${caseHref("/app/patients", id)}/disposition`}
            />
          </div>
        </div>
      </>
    );
  }
  const detail = viewer.detail;
  if (!detail) notFound();
  const ctx = dispositionContext(detail);
  return (
    <>
      {back}
      <div className={CARD}>
        <PageHeader
          attached
          compact
          title="Clinician disposition"
          sub={`Case ${shortId(detail.case.id)} · ${detail.case.programme.replaceAll("_", " ")} · screening record ${detail.case.closed ? "closed" : "open"}`}
        />
        <div className="p-4 md:p-pad">
          {live && viewer.session && (
            <ReviewerSessionStatus session={viewer.session} persona={viewer.session.method === "oidc" ? await reviewerPersona() : null} />
          )}
          <div className="mt-4 grid items-start gap-gap lg:grid-cols-2">
            <DispositionCard ctx={ctx} link={false} />
            <section aria-labelledby="disposition-record-h" className="rounded-card border border-line bg-surface p-4 md:p-pad">
              <h2 id="disposition-record-h" className="m-0 text-section font-medium">
                Record a disposition
              </h2>
              {ctx.open.length === 0 ? (
                <p className="m-0 mt-3 text-muted" data-testid="disposition-none">
                  No clinician decision is needed. The case is not on hold and the patient has not declined.
                </p>
              ) : !live ? (
                <p className="m-0 mt-3 text-muted" data-testid="disposition-fixtures">
                  Dispositions can&apos;t be recorded in this demo.
                </p>
              ) : (
                ctx.open.map((kind) => (
                  <div key={kind} className="mt-4">
                    <h3 className="m-0 font-medium">
                      {kind === "clinician_disposition" ? "Plan for the clinician hold" : "Informed refusal after the patient declined"}
                    </h3>
                    <p className="m-0 text-small text-muted">
                      {kind === "clinician_disposition"
                        ? "Saving records a plan with an owner and a review date. The hold stays until the results arrive."
                        : "Saving answers the patient's decline, as recorded by the coordinator. It never records the procedure as done."}
                    </p>
                    <DispositionForm caseId={detail.case.id} expectedHead={ctx.expectedHead} kind={kind} crc={detail.case.programme === "crc"} />
                  </div>
                ))
              )}
            </section>
          </div>
        </div>
      </div>
    </>
  );
}
