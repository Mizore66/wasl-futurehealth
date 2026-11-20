import Link from "next/link";
import { notFound } from "next/navigation";
import { LedgerCard, TimelineCard } from "@/components/case/sections";
import { loadCaseRecord, renderEvidenceFile } from "@/components/case/evidence-file";
import { PageHeader } from "@/components/shell/page-header";
import { BackToFiles } from "@/components/patients/back-to-files";
import { referralDate } from "@/components/patients/calendar";
import { caseHref, caseIdFromParam, shortId } from "@/lib/domain/ids";
import { personForLoop } from "@/lib/people";
import "@/components/patients/patients.css";

export async function generateMetadata({ params }: PageProps<"/app/patients/[id]">) {
  const id = caseIdFromParam((await params).id);
  return { title: personForLoop(id)?.display_name ?? shortId(id) };
}

const BUTTON = "inline-flex min-h-11 items-center justify-center rounded-control px-4 font-medium no-underline";

export default async function Page({ params }: PageProps<"/app/patients/[id]">) {
  const id = caseIdFromParam((await params).id);
  const record = await loadCaseRecord(id);
  if (!record) notFound();
  const { detail } = record;
  const c = detail.case;
  if (c.lifecycle === "retained")
    return (
      <section aria-label="Retired case" className="overflow-hidden rounded-card border border-line bg-surface">
        <PageHeader attached compact title={<>Retired case {shortId(c.id)}</>} />
        <div className="p-4 md:p-pad">
          <p className="m-0">This tracking request is inactive. Its original decisions, signed reviews and source history are retained, not current care or outreach work.</p>
          <p>{c.programme_availability === "unavailable" ? "No current programme was selected." : "The recorded programme does not confer active eligibility."}</p>
          <Link className={BUTTON} href={`${caseHref("/app/patients", c.id)}/audit`}>Authorized audit history</Link>
          <Link className={BUTTON} href={`/app/people?case_id=${encodeURIComponent(c.id)}`}>Historical person association</Link>
          <TimelineCard detail={detail} />
          <LedgerCard detail={detail} />
        </div>
      </section>
    );
  // The same Evidence File an open folder shows; its tab goes back to the case files this record was opened from.
  return renderEvidenceFile({ record, back: <BackToFiles id={c.id} date={referralDate(c)} /> });
}
