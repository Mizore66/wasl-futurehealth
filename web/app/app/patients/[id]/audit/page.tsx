import Link from "next/link";
import { notFound } from "next/navigation";
import { Avatar } from "@/components/avatar/avatar";
import { AnchorsCard, ChainCard, HistoryCard, SignaturesCard, VerifiedCard } from "@/components/case/audit-view";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { getAudit, getCase } from "@/lib/api/source";
import { caseHref, caseIdFromParam, shortId } from "@/lib/domain/ids";
import { personForLoop } from "@/lib/people";
import { referralSeed } from "@/lib/people/identity";

export async function generateMetadata({ params }: PageProps<"/app/patients/[id]/audit">) {
  const id = caseIdFromParam((await params).id);
  return { title: `Audit · ${personForLoop(id)?.display_name ?? shortId(id)}` };
}

export default async function Page({ params }: PageProps<"/app/patients/[id]/audit">) {
  const id = caseIdFromParam((await params).id);
  const [detail, audit] = await Promise.all([getCase(id), getAudit(id)]);
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
              {person?.display_name ?? shortId(detail.case.id)} · <span className="font-mono text-small">{shortId(detail.case.id)}</span>
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
            <HistoryCard audit={audit} />
            <AnchorsCard audit={audit} />
          </div>
        </div>
      </div>
    </>
  );
}
