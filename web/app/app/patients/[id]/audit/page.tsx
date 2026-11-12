import Link from "next/link";
import { notFound } from "next/navigation";
import { Avatar } from "@/components/avatar/avatar";
import { ExportCard } from "@/components/case/audit-export";
import { AnchorsCard, ChainCard, HistoryCard, SignaturesCard, VerifiedCard } from "@/components/case/audit-view";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { gatedHistory } from "@/lib/api/protocol-history";
import { getCase } from "@/lib/api/source";
import { caseHref, caseIdFromParam, shortId } from "@/lib/domain/ids";
import { personForLoop } from "@/lib/people";
import { referralSeed } from "@/lib/people/identity";

export async function generateMetadata({ params }: PageProps<"/app/patients/[id]/audit">) {
  const id = caseIdFromParam((await params).id);
  return { title: `Audit · ${personForLoop(id)?.display_name ?? shortId(id)}` };
}

export default async function Page({ params }: PageProps<"/app/patients/[id]/audit">) {
  const id = caseIdFromParam((await params).id);
  const [detail, history] = await Promise.all([getCase(id), gatedHistory(id, { needAudit: true })]);
  const { audit, projection, status } = history;
  if (!detail || !audit) notFound();
  const person = personForLoop(detail.case.id);

  return (
    <>
      <p className="m-0">
        <Link href={caseHref("/app/patients", detail.case.id)} className="inline-flex min-h-11 items-center">
          ← Back to the case
        </Link>
      </p>
      <header className="flex flex-wrap items-start justify-between gap-gap">
        <div className="flex min-w-0 items-center gap-4">
          <Avatar person={referralSeed(detail.case.id)} size={56} label="" />
          <div className="min-w-0">
            <h1 className="m-0 text-title font-semibold">Audit trail</h1>
            <p className="m-0 mt-1 text-muted">
              {person?.display_name ?? shortId(detail.case.id)} · <span className="text-small">{shortId(detail.case.id)}</span>
            </p>
          </div>
        </div>
        <HonestyLabel kind="simulated">Simulated run, real hashes</HonestyLabel>
      </header>

      <div className="mt-6 flex flex-col gap-gap">
        <VerifiedCard audit={audit} />
        <div className="grid items-start gap-gap lg:grid-cols-2">
          <ChainCard audit={audit} />
          <div className="flex min-w-0 flex-col gap-gap">
            <SignaturesCard audit={audit} />
            {status === "ok" ? (
              <HistoryCard audit={audit} programme={detail.case.programme} governance={projection ? { caseId: id, projection, signedIn: history.signedIn, mode: history.session?.method, expiresAt: history.session?.expiresAt } : undefined} />
            ) : (
              <section aria-labelledby="history-withheld-h" className="rounded-card border border-line bg-surface p-4 md:p-pad" data-testid="history-withheld">
                <h2 id="history-withheld-h" className="m-0 text-section font-medium">
                  Review history
                </h2>
                <p className="m-0 mt-3 max-w-[72ch] text-muted" role="status">
                  {status === "signin"
                    ? "Independent review reads are shown only to a signed-in reviewer. Sign in on the review page to see them."
                    : "The review history is withheld while a first read is hidden. It appears once the second read is in."}
                </p>
                <p className="m-0 mt-3">
                  <Link href={caseHref("/app/review", detail.case.id)} className="inline-flex min-h-11 items-center">
                    Open the review page
                  </Link>
                </p>
              </section>
            )}
            <AnchorsCard audit={audit} />
          </div>
        </div>
        {status === "ok" && <ExportCard caseId={detail.case.id} />}
      </div>
    </>
  );
}
