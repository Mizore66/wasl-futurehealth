import { atWords, reviewWords, roleWords } from "@/components/case/words";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import type { CaseDetail, ReadName } from "@/lib/api/schema";
import { READ_LABEL, readsByItem } from "./model";

const label = (read: string) => (read === "r1" || read === "r2" || read === "r3" ? READ_LABEL[read as ReadName] : `Read ${read || "(unnamed)"}`);

const TRANSITION: Record<string, string> = {
  disagreement: "The initial reads disagree: a senior read is open.",
  agreed: "The initial reads agree.",
  resolved: "Resolved by the senior read.",
};

/**
 * Every double-review read on the case's ledger, each on its own line, as case detail `api_reviews` serves them. A read
 * the API still blinds is listed without its answer, and so is every read of an item whose two initial reads are not
 * both on the ledger, whatever the response says (case detail blinds for everyone, the author included). Renders
 * nothing when the case has no reads.
 */
export function DoubleReviewReadsCard({ detail }: { detail: CaseDetail }) {
  const events = detail.api_reviews.flatMap((e) => (e.ledger === "double_review" ? [e] : []));
  if (events.length === 0) return null;
  const programme = detail.case.programme;
  return (
    <section id="double-review-reads" aria-labelledby="double-review-reads-h" className="min-w-0 scroll-mt-4 rounded-card border border-line bg-surface p-4 md:p-pad">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <h2 id="double-review-reads-h" className="m-0 text-section font-medium">
          Double-review reads
        </h2>
        <HonestyLabel kind="built">Built: demo review log, synthetic patients</HonestyLabel>
      </div>
      <p className="m-0 mt-1 text-small text-muted">
        Each read was given by the person the API recorded for its credential. A demo overlay: the case&rsquo;s own decision and signatures are unchanged.
      </p>
      {[...readsByItem(events)].map(([item, reads]) => {
        const open = !(reads.some((r) => r.read === "r1") && reads.some((r) => r.read === "r2"));
        return (
          <div key={item} className="mt-3">
            <p className="m-0 text-small text-muted">{item}</p>
            <ol className="m-0 list-none p-0" aria-label={`Reads for ${item}`}>
              {reads.map((r) => (
                <li key={r.hash} className="border-t border-rule py-2">
                  <span className="font-medium">{label(r.read)}: </span>
                  {open || r.blinded || r.decision === undefined ? <em>answer hidden until both initial reads are in</em> : reviewWords(r.decision, programme)}
                  <span className="text-muted">
                    {" "}
                    · {r.actor || "actor not shown"} ({roleWords(r.role)}) · {atWords(r.at)}
                  </span>
                  {TRANSITION[r.phase_after] && <span className="block text-small text-muted">{TRANSITION[r.phase_after]}</span>}
                  {r.final && <span className="block text-small font-medium">Settled answer: {reviewWords(r.final, programme)}</span>}
                </li>
              ))}
            </ol>
          </div>
        );
      })}
    </section>
  );
}
