import Link from "next/link";
import { notFound } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { ExportCard } from "@/components/case/audit-export";
import { decisionViews, ledgerEvents, signatureTally, timelineEvents } from "@/components/case/audit-model";
import { AuditTopbarControls } from "@/components/case/audit-topbar";
import {
  AnchorsCard,
  ChainCard,
  GovernanceHeader,
  LedgerControls,
  LedgerFacts,
  LedgerTitle,
  SignaturesCard,
  VerifiedCard,
  ledgerGovernance,
  type HistoryGovernance,
} from "@/components/case/audit-view";
import { AuditWorkspace, type WorkspaceEvent } from "@/components/case/audit-workspace";
import { PageHeader } from "@/components/shell/page-header";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { GlossaryPopover } from "@/components/wasl/glossary-popover";
import { statusWords } from "@/components/worklist/model";
import { gatedHistory } from "@/lib/api/protocol-history";
import { apiMode, getCase } from "@/lib/api/source";
import { formatUtcInstant } from "@/lib/domain/day";
import { caseHref, caseIdFromParam, shortId } from "@/lib/domain/ids";
import { personForLoop } from "@/lib/people";

export async function generateMetadata({ params }: PageProps<"/app/patients/[id]/audit">) {
  const id = caseIdFromParam((await params).id);
  return { title: `Audit · ${personForLoop(id)?.display_name ?? shortId(id)}` };
}

export default async function Page({ params }: PageProps<"/app/patients/[id]/audit">) {
  const id = caseIdFromParam((await params).id);
  const [detail, history] = await Promise.all([getCase(id), gatedHistory(id, { needAudit: true })]);
  const { audit, projection, status } = history;
  if (!detail || !audit) notFound();
  const caseId = detail.case.id;
  const programme = detail.case.programme;
  const reviewHref = caseHref("/app/review", caseId);

  const governance: HistoryGovernance | undefined =
    status === "ok" && projection ? { caseId: id, projection, signedIn: history.signedIn, mode: history.session?.method, expiresAt: history.session?.expiresAt } : undefined;
  const gov = ledgerGovernance(audit, governance);

  /* Source records from the case timeline, then the shared ledger (only when the viewer may see it). */
  const ledger = status === "ok" ? ledgerEvents(audit.history, programme) : [];
  const events: WorkspaceEvent[] = [
    ...timelineEvents(detail.timeline, caseId),
    ...ledger.map((e) => {
      const hash = audit.history[e.ledgerIndex!].hash;
      return { ...e, anchor: hash ? `review-event-${hash}` : undefined };
    }),
  ];
  const details: Record<string, React.ReactNode> = {};
  const controls: Record<string, React.ReactNode> = {};
  for (const e of events) {
    if (e.ledgerIndex === undefined) {
      details[e.key] = <p className="m-0">{e.note}</p>;
      continue;
    }
    const r = audit.history[e.ledgerIndex];
    details[e.key] = <LedgerFacts audit={audit} index={e.ledgerIndex} programme={programme} />;
    if (gov.status.get(r.hash) || gov.governable(r)) controls[e.key] = <LedgerControls r={r} governance={governance} gov={gov} />;
  }

  const historyIntro =
    status === "ok" ? (
      <>
        <GovernanceHeader governance={governance} gov={gov} />
        {audit.history.length > 0 && (
          <p className="m-0 mt-3">
            <HonestyLabel kind="simulated">Demo actions, not clinical sign-offs</HonestyLabel>
          </p>
        )}
      </>
    ) : (
      <div className="mt-3 text-[13px]" data-testid="history-withheld">
        <p className="m-0 max-w-[60ch] text-muted" role="status">
          {status === "signin"
            ? "Review actions are shown only to a signed-in reviewer. Sign in on the review page to see them."
            : "Review actions are withheld while a first read is hidden. They appear once the second read is in."}
        </p>
        <Link href={reviewHref} className="inline-flex min-h-11 items-center">
          Open the review page
        </Link>
      </div>
    );

  const tally = signatureTally(audit.decision_signatures);
  const intact = audit.chain.length === audit.chain_length;
  const verifyNote = (
    <div className="-mt-2 mb-4 flex flex-col items-start gap-2 text-[13px]">
      {!audit.chain_verified && (
        <p className="m-0 font-semibold text-clay-ink" role="alert">
          The sealed chain did not verify. Treat this case&apos;s record as altered until it is checked.
        </p>
      )}
      {audit.chain_verified && !intact && !history.modelWithheld && (
        <p className="m-0 font-semibold text-clay-ink">
          Only {audit.chain.length} of {audit.chain_length} sealed entries were returned.
        </p>
      )}
      {tally.state === "some" && <p className="m-0 font-semibold text-clay-ink">At least one decision signature does not match its stored value.</p>}
      <HonestyLabel kind="simulated">Simulated run, real hashes</HonestyLabel>
    </div>
  );

  const auditDetails = (
    <>
      <p className="m-0 text-[13px] text-muted">
        <strong className="font-semibold text-ink">
          {audit.chain_length} sealed {audit.chain_length === 1 ? "entry" : "entries"}
        </strong>{" "}
        · Sealing links each entry to the one before it, so a change or removal can be detected.
        <span className="mt-1 block text-[12px]">Run loaded {formatUtcInstant(audit.meta.loaded_at)}</span>
      </p>
      <GlossaryPopover />
      <VerifiedCard audit={audit} />
      {history.modelWithheld ? (
        <section aria-labelledby="chain-withheld-h" data-testid="chain-withheld">
          <h2 id="chain-withheld-h" className="m-0 text-section font-medium">
            Sealed entries
          </h2>
          <p className="m-0 mt-3 max-w-[72ch] text-muted" role="status">
            The chain carries the image reader&apos;s verdict, so it is withheld until you have completed your own independent read of the evidence question.
          </p>
          <p className="m-0 mt-3">
            <Link href={reviewHref} className="inline-flex min-h-11 items-center">
              Open the review page
            </Link>
          </p>
        </section>
      ) : (
        <ChainCard audit={audit} />
      )}
      <SignaturesCard audit={audit} />
      <AnchorsCard audit={audit} />
    </>
  );

  return (
    <div className="flex min-h-full flex-col">
      <AuditTopbarControls />
      <AuditWorkspace
        head={
          <PageHeader
            attached
            title="Audit trail"
            compact
           
            sub={
              <>
                <span className="break-all">{shortId(caseId)}</span> · {statusWords(detail.case).status}
              </>
            }
            aside={
              <Link href={caseHref("/app/patients", caseId)} className="flex min-h-11 items-center gap-1.5 whitespace-nowrap px-[7px] font-semibold text-jade-800 no-underline max-md:text-[12px]">
                <ArrowLeft aria-hidden className="size-4 max-md:size-[13px]" strokeWidth={1.8} />
                Back to case
              </Link>
            }
          />
        }
        events={events}
        details={details}
        controls={controls}
        historyIntro={historyIntro}
        historyEmpty="No dated events are recorded for this case."
        snapshotLabel={apiMode() === "live" ? "Checked on load" : "Recorded snapshot"}
        decisions={decisionViews(audit, detail)}
        verifyNote={verifyNote}
        auditDetails={auditDetails}
      />
      {status === "ok" && audit.history.length > 0 && (
        <section aria-labelledby="review-history-h" className="mt-3 rounded-[8px] border border-line bg-surface px-[clamp(15px,2.1vw,36px)] py-3">
          <h2 id="review-history-h" className="m-0 text-[14px] font-semibold">
            Review history
          </h2>
          <p className="m-0 mt-0.5 text-[12px] text-muted">Every review and coordinator entry on the sealed chain, in order. Each also appears in the event history.</p>
          <ol aria-label="Review history" className="m-0 mt-2 flex list-decimal flex-col gap-1 ps-5 text-[13px]">
            {audit.history.map((r, i) => (
              <li key={r.hash ?? i}>
                <LedgerTitle r={r} programme={programme} />
              </li>
            ))}
          </ol>
        </section>
      )}
      {status === "ok" && !history.modelWithheld && (
        <div className="mt-3">
          <ExportCard caseId={caseId} />
        </div>
      )}
    </div>
  );
}
