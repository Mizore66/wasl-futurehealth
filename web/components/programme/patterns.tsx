import Link from "next/link";
import { CATEGORICAL, CHROME } from "@/components/charts/palette";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { closedShare, loopsText, patternText, STATUS_PART, type Pattern, type PatternRow, type Patterns } from "./model";

/*
 * Provider links (issue #45), drawn as a meter list: per provider, the share of its cases closed with evidence as a bar,
 * the figure, and the largest stall. A count links only when the case list can filter to the same provider and state or
 * stall. Suppressed small cells show as "fewer than 5" and never link or draw a bar.
 *
 * "Closed with evidence" is the provider table's evidenced_closed (state closed, the patient list's Closed filter). It is
 * not the programme chart's "Finished", which also counts cases that needed no follow-up.
 */

/** Every item is a 44px row on touch widths, linked or not, so the rhythm stays even; compact from md up. */
const item = "inline-flex min-h-11 items-center md:min-h-0";

/** Provider | bar | figure | largest stall; the bar column drops under the name on narrow widths. */
const ROW = "grid grid-cols-1 items-center gap-x-4 md:grid-cols-[minmax(120px,170px)_minmax(150px,1fr)_minmax(0,auto)_minmax(0,1.4fr)]";

function PatternLink({ row, p }: { row: PatternRow; p: Pattern }) {
  const text = patternText(p);
  return p.href ? (
    <Link href={p.href} data-item className={item}>
      <span className="sr-only">{row.key}: </span>
      {text}
      <span className="sr-only"> (simulated)</span>
    </Link>
  ) : (
    <span data-item className={`${item} text-ink-soft`}>
      {text}
    </span>
  );
}

function Line({ row }: { row: PatternRow }) {
  const count = `${row.key}, ${loopsText(row.loops)}`;
  const closed = row.patterns.find((p) => p.kind === "closed");
  const stall = row.patterns.find((p) => p.kind === "stall");
  const share = closedShare(closed);
  return (
    <li data-testid={`provider-${row.key}`} className={`${ROW} border-b border-rule py-1.5 last:border-b-0 md:py-2`}>
      {row.href ? (
        <Link href={row.href} data-item className={`${item} font-medium`}>
          {count}
          <span className="sr-only"> (simulated)</span>
        </Link>
      ) : (
        <span data-item className={`${item} font-medium`}>
          {count}
        </span>
      )}
      <span className="flex min-w-0 items-center gap-2.5 py-1 md:py-0">
        <span aria-hidden className="block h-2.5 min-w-0 flex-1 rounded-full" style={{ backgroundColor: share === null ? "transparent" : CHROME.grid }}>
          {share !== null && <span className="block h-full rounded-full" style={{ width: `${share}%`, minWidth: share > 0 ? 6 : 0, backgroundColor: CATEGORICAL[0] }} />}
        </span>
        <span className="w-10 flex-none text-right text-body font-semibold tabular-nums">{share !== null && `${share}%`}</span>
      </span>
      <span className="tabular-nums">{closed && <PatternLink row={row} p={closed} />}</span>
      <span className="text-ink-soft">{stall && <PatternLink row={row} p={stall} />}</span>
    </li>
  );
}

/** Supporting links with no object state, so they sit on the page grid unframed, like Safety (DEC-3). */
export function PatternLinks({ patterns }: { patterns: Patterns }) {
  const { providers, smallCellRule } = patterns;
  if (providers.length === 0) return null;
  const shares = providers.map((r) => closedShare(r.patterns.find((p) => p.kind === "closed"))).filter((v): v is number => v !== null);
  const range = shares.length > 1 ? ` runs from ${Math.min(...shares)}% to ${Math.max(...shares)}%` : "";
  return (
    <section aria-labelledby="patterns-h" className="mt-8" data-testid="provider-links">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <h2 id="patterns-h" className="m-0 text-section font-medium">
          By provider
        </h2>
        <HonestyLabel kind="simulated">Simulated cohort</HonestyLabel>
      </div>
      <p data-testid="provider-takeaway" className="m-0 mt-1 text-body font-medium text-ink">
        Closed with evidence{range}
      </p>
      <p className="m-0 mt-1 max-w-[80ch] text-small text-muted">
        Out of each provider&apos;s cases across every programme. Closed with evidence counts closures with evidence on file only, so it is lower than the
        programme chart&apos;s {STATUS_PART.closed.toLowerCase()}, which also counts cases that needed no follow-up. Each figure opens the filtered patient list.
        {smallCellRule && <> Small cells are suppressed at source ({smallCellRule}) and are never shown as a number or linked.</>}
      </p>
      <ul aria-labelledby="patterns-h" className="m-0 mt-2 list-none p-0 text-body">
        {providers.map((r) => (
          <Line key={r.key} row={r} />
        ))}
      </ul>
    </section>
  );
}
