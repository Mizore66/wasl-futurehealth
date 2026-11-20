import { Button } from "@/components/ui/button";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { getExportHashes } from "@/lib/api/export";
import { caseHref } from "@/lib/domain/ids";

/**
 * The auditor's redacted evidence export (issue #46), below the chain so it stays secondary to inspecting it.
 * The file comes only from the Wasl API through the download route; without a live API the action is disabled and
 * says why, and nothing is generated in its place.
 */
export async function ExportCard({ caseId }: { caseId: string }) {
  const got = await getExportHashes(caseId);
  const href = `${caseHref("/app/patients", caseId)}/audit/export`;
  return (
    <section aria-labelledby="export-h" className="min-w-0 rounded-card border border-line bg-surface p-4 md:p-pad" data-testid="audit-export">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <h2 id="export-h" className="m-0 text-section font-medium">
          Redacted evidence export
        </h2>
        <HonestyLabel kind="simulated">Simulated data</HonestyLabel>
      </div>
      <p className="m-0 mt-1 max-w-[72ch] text-small text-muted">
        A copy of this case&apos;s record and audit trail for internal audit, with some details removed. It can still
        identify the patient. Do not share it outside the audit team.
      </p>

      {got.ok ? (
        <>
          <dl className="m-0 mt-3 text-small">
            {[
              { id: "content", label: "File check code", value: got.hashes.content_sha256, note: "matches the downloaded file" },
              { id: "comparison", label: "Case check code", value: got.hashes.comparison_sha256, note: "stays the same while the case is unchanged" },
            ].map((h) => (
              <div key={h.id} className="mt-2">
                <dt className="font-medium">
                  {h.label} <span className="font-normal text-muted">· {h.note}</span>
                </dt>
                <dd className="m-0 mt-1">
                  <code className="block break-all font-mono" data-testid={`export-${h.id}-hash`}>{h.value}</code>
                </dd>
              </div>
            ))}
          </dl>
          <p className="m-0 mt-2 max-w-[72ch] text-small text-muted">
            A new review changes both codes. They show the file is unchanged, not that every identifying detail was
            removed.
          </p>
          <p className="m-0 mt-3">
            <Button asChild variant="quiet" size="touch">
              <a href={href} download>
                Download redacted export (JSON)
              </a>
            </Button>
          </p>
        </>
      ) : (
        <>
          <p className="m-0 mt-3 max-w-[72ch]" role="status" id="export-unavailable" data-reason={got.reason}>
            {got.message}
          </p>
          <p className="m-0 mt-3">
            <Button variant="quiet" size="touch" disabled aria-describedby="export-unavailable">
              Download redacted export (JSON)
            </Button>
          </p>
        </>
      )}
    </section>
  );
}
