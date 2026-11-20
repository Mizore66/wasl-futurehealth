import Link from "next/link";
import { CATEGORICAL, CHROME } from "@/components/charts/palette";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { cellText, closedShare, loopsText, type PatternRow, type Patterns } from "./model";

/*
 * Provider links (issue #45), drawn as a meter list: per provider, its name and case count, then the share of its cases
 * closed with evidence as a bar and a figure. The whole row opens the patient list filtered to that provider's closed
 * cases, and only when the closed count is a positive, unsuppressed number. Suppressed small cells draw no bar and no
 * link; the small-cell rule is their tooltip.
 *
 * "Closed with evidence" is the provider table's evidenced_closed (state closed, the patient list's Closed filter). It is
 * not the programme chart's "Finished", which also counts cases that needed no follow-up.
 */

/** Name and count | bar | figure; a 44px target on touch widths, compact from md up. */
const ROW = "grid min-h-11 grid-cols-[minmax(0,1fr)_minmax(64px,0.9fr)_2.75rem] items-center gap-x-3 py-1.5 md:min-h-10";

function Line({ row, rule }: { row: PatternRow; rule: string | null }) {
  const closed = row.patterns.find((p) => p.kind === "closed");
  const share = closedShare(closed);
  const hide = Boolean(closed?.href) || undefined;
  const body = (
    <>
      <span aria-hidden={hide} className="min-w-0 text-body leading-tight">
        <span className="font-medium text-ink">{row.key}</span>
        <span className="text-ink-soft tabular-nums"> · {loopsText(row.loops)}</span>
      </span>
      <span aria-hidden className="block h-2.5 min-w-0 rounded-full" style={{ backgroundColor: share === null ? "transparent" : CHROME.grid }}>
        {share !== null && <span className="block h-full rounded-full" style={{ width: `${share}%`, minWidth: share > 0 ? 6 : 0, backgroundColor: CATEGORICAL[0] }} />}
      </span>
      <span aria-hidden={hide} className="text-right text-body font-semibold tabular-nums">
        {share !== null ? (
          `${share}%`
        ) : closed ? (
          <span title={rule ?? undefined} className="text-small font-normal text-muted">
            &lt;{closed.cell.kind === "suppressed" ? closed.cell.below : ""}
          </span>
        ) : null}
      </span>
    </>
  );
  return (
    <li data-testid={`provider-${row.key}`} className="border-b border-rule last:border-b-0">
      {closed?.href ? (
        <Link
          href={closed.href}
          data-item
          className={`${ROW} rounded-[4px] text-ink no-underline hover:bg-raised [&:hover_.font-medium]:underline`}
        >
          {/* The name starts with the visible provider name and carries the count behind the bar; the visible row is hidden from it. */}
          <span className="sr-only">{`${row.key}: ${cellText(closed.cell, closed.of)} closed with evidence (simulated)`}</span>
          {body}
        </Link>
      ) : (
        <div data-item className={ROW} aria-label={closed ? `${row.key}: ${cellText(closed.cell, closed.of)} closed with evidence` : undefined} role={closed ? "group" : undefined}>
          {body}
        </div>
      )}
    </li>
  );
}

/** A chart panel like its neighbours: it sits in a card under "Where patients stall". */
export function PatternLinks({ patterns, className }: { patterns: Patterns; className?: string }) {
  const { providers, smallCellRule } = patterns;
  if (providers.length === 0) return null;
  const rule = smallCellRule && "Counts under 5 are hidden to protect privacy.";
  return (
    <section aria-labelledby="patterns-h" className={className} data-testid="provider-links">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <h2 id="patterns-h" className="m-0 text-section font-medium">
          By provider
        </h2>
        <HonestyLabel plain kind="simulated">Simulated data</HonestyLabel>
      </div>
      <p data-testid="provider-takeaway" className="m-0 mt-1 text-small text-muted">
        Share of each provider&apos;s cases closed with evidence.
      </p>
      <ul aria-labelledby="patterns-h" className="m-0 mt-2 list-none p-0">
        {providers.map((r) => (
          <Line key={r.key} row={r} rule={rule || null} />
        ))}
      </ul>
    </section>
  );
}
