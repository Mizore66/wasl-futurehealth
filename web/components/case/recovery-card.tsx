import Link from "next/link";
import type { CaseDetail } from "@/lib/api/schema";

export function RecoveryCard({ detail }: { detail: CaseDetail }) {
  const recovery = detail.case.recovery;
  if (!recovery || (!recovery.backfill && recovery.source_releases.length === 0)) return null;
  const backfill = recovery.backfill;
  return (
    <section id="recovery" aria-labelledby="recovery-h" className="rounded-card border border-line bg-surface p-4 md:p-pad">
      <h2 id="recovery-h" className="m-0 text-section font-medium">Recovery</h2>
      <p className="text-small text-muted">Simulated released-capacity workflow. Canonical clinical decisions and the original booking are unchanged.</p>
      {backfill && (
        <div data-testid="case-recovery-backfill">
          <p><strong>Simulated slot confirmed</strong> · day {backfill.day}</p>
          <p className="text-small">No provider booking or message was made. Outreach: {backfill.outreach_status.replaceAll("_", " ")}.</p>
          <p className="text-small break-all">Ledger {backfill.event}</p>
          <Link href={`/app/recovery?slot=${backfill.slot_id}`} className="inline-flex min-h-11 items-center">Open recovery projection</Link>
        </div>
      )}
      {recovery.source_releases.length > 0 && (
        <div className="mt-3">
          <h3 className="text-body font-medium">No-show release history</h3>
          <ul className="list-disc pl-5 text-small">
            {recovery.source_releases.map((release) => <li key={release.slot_id}>Day {release.day} · {release.status} · <Link href={`/app/recovery?slot=${release.slot_id}`}>projection</Link></li>)}
          </ul>
        </div>
      )}
    </section>
  );
}
