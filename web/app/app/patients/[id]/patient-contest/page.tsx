import Link from "next/link";
import { notFound } from "next/navigation";
import { atWords } from "@/components/case/words";
import { ContestReviewForm } from "@/components/patient-contest/contest-review-form";
import { contestsOf, type ContestRow } from "@/components/patient-contest/model";
import { ReviewerLogin, ReviewerSessionStatus } from "@/components/review/reviewer-login";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { getReviewCase } from "@/lib/api/reviewer-session";
import { apiMode } from "@/lib/api/source";
import { reviewAuthMode } from "@/lib/auth/oidc-config";
import { caseHref, caseIdFromParam, shortId } from "@/lib/domain/ids";

/* The title never reads the case: the case is read only with the signed-in reviewer's own credential. */
export const metadata = { title: "Patient contest review" };

function statusLine(row: ContestRow): string {
  if (row.status === "resolved") {
    const outcome = row.outcome === "decision_revised" ? "decision to be revised" : "decision upheld";
    return `Resolved: ${outcome}${row.resolved_by ? ` by ${row.resolved_by}` : ""}${row.resolved_at ? ` ${atWords(row.resolved_at)}` : ""}`;
  }
  return `${row.overdue ? "Overdue" : "Open"} · independent ${row.owner ?? "senior clinician"} review due ${row.due_at ? atWords(row.due_at) : "(no due date)"}`;
}

/*
 * The independent review of a verified patient contest (#167). A senior clinician who signed nothing on the case
 * records the outcome and the reason. Identity comes only from the signed-in session; the server holds no fallback.
 */
export default async function Page({ params, searchParams }: PageProps<"/app/patients/[id]/patient-contest">) {
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
    return (
      <>
        {back}
        <h1 className="m-0 text-title font-semibold">Patient contest review</h1>
        <p className="m-0 mt-1 max-w-[72ch] text-muted">
          Only a senior clinician who did not sign this case may resolve a patient&apos;s contest. Sign in with the token assigned to you.
        </p>
        <ReviewerLogin caseId={id} mode={reviewAuthMode()} authError={query.oidc_error === "1"} returnTo={`${caseHref("/app/patients", id)}/patient-contest`} />
      </>
    );
  }
  const detail = viewer.detail;
  if (!detail) notFound();
  const contests = contestsOf(detail);
  return (
    <>
      {back}
      <header className="flex flex-wrap items-start justify-between gap-gap">
        <div>
          <h1 className="m-0 text-title font-semibold">Patient contest review</h1>
          <p className="m-0 mt-1 text-muted">
            Case {shortId(detail.case.id)} · {detail.case.programme.replaceAll("_", " ")} ·{" "}
            {detail.case.closure_hold === "patient_contest" ? "automatic closure held by a patient contest" : detail.case.closed ? "closed" : "open"}
          </p>
          {live && viewer.session && <ReviewerSessionStatus session={viewer.session} />}
        </div>
        <HonestyLabel kind="simulated">Synthetic case</HonestyLabel>
      </header>

      <section aria-labelledby="contest-list-h" className="mt-6 rounded-card border border-line bg-surface p-4 md:p-pad">
        <h2 id="contest-list-h" className="m-0 text-section font-medium">
          Contests on this case
        </h2>
        {contests.length === 0 ? (
          <p className="m-0 mt-3 text-muted" data-testid="contest-none">
            No patient has contested a decision on this case.
          </p>
        ) : (
          <ol className="m-0 mt-2 list-none p-0" aria-label="Patient contests">
            {contests.map((row) => (
              <li key={row.receipt} className="border-t border-rule py-3" data-testid={`contest-${row.receipt.slice(0, 8)}`}>
                <p className="m-0 font-medium">Contest {row.receipt.slice(0, 8)}</p>
                <p className="m-0 mt-1" data-testid="contest-status">
                  {statusLine(row)}
                </p>
                <p className="m-0 mt-1 text-small text-muted">
                  Submitted {atWords(row.submitted_at)} through the patient link after a simulated one-time code · questioned decision {row.decision.slice(0, 12)}
                </p>
                {row.reason && <p className="m-0 mt-1">Patient&apos;s reason: {row.reason}</p>}
                {row.excluded_reviewers && row.excluded_reviewers.length > 0 && (
                  <p className="m-0 mt-1 text-small text-muted">May not resolve (signed this case): {row.excluded_reviewers.join(", ")}</p>
                )}
                {row.resolution_reason && <p className="m-0 mt-1" data-testid="contest-resolution-reason">Reviewer&apos;s reason: {row.resolution_reason}</p>}
                {row.closure_held && row.status === "resolved" && (
                  <p className="m-0 mt-1 text-small">Closure stays held until the questioned decision is replaced (owned revision task).</p>
                )}
                {live ? (
                  <ContestReviewForm key={row.receipt} caseId={detail.case.id} contest={row.receipt} open={row.status === "open"} />
                ) : (
                  row.status === "open" && (
                    <p className="m-0 mt-2 text-muted" data-testid="contest-fixtures">
                      Fixtures mode: a contest review can be recorded only against the live API.
                    </p>
                  )
                )}
              </li>
            ))}
          </ol>
        )}
      </section>
    </>
  );
}
