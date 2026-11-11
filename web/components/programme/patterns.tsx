import Link from "next/link";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { loopsText, patternText, type PatternRow, type Patterns } from "./model";

/*
 * Provider links (issue #45). The owner chose filtered Patients links over a dedicated breakdown section, so this adds one
 * collapsed line to the dashboard; opened, it is one line per provider. A count links only when the case list can filter
 * to the same provider and state or stall. Suppressed small cells show as "fewer than 5" and never link.
 */

/** Every item is a 44px row on touch widths, linked or not, so the rhythm stays even; compact from md up. */
const item = "inline-flex min-h-11 items-center md:min-h-0";

const Dot = () => (
  <span aria-hidden className="text-muted">
    ·
  </span>
);

function Line({ row }: { row: PatternRow }) {
  const count = `${row.key}, ${loopsText(row.loops)}`;
  return (
    <li data-testid={`provider-${row.key}`} className="flex flex-wrap items-center gap-x-2 border-b border-rule py-1 last:border-b-0 md:py-1.5">
      {row.href ? (
        <Link href={row.href} data-item className={`${item} font-medium`}>
          {count}
          <span className="sr-only"> (simulated)</span>
        </Link>
      ) : (
        <span data-item className={`${item} font-medium`}>{count}</span>
      )}
      {row.patterns.map((p) => {
        const text = patternText(p);
        return (
          // The separator travels with its item, so a wrapped line never ends on a dangling dot.
          <span key={p.kind} className="inline-flex items-center gap-x-2">
            <Dot />
            {p.href ? (
              <Link href={p.href} data-item className={item}>
                <span className="sr-only">{row.key}: </span>
                {text}
                <span className="sr-only"> (simulated)</span>
              </Link>
            ) : (
              <span data-item className={`${item} text-ink-soft`}>{text}</span>
            )}
          </span>
        );
      })}
    </li>
  );
}

/** Supporting links with no object state, so they sit on the page grid unframed, like Safety (DEC-3). */
export function PatternLinks({ patterns }: { patterns: Patterns }) {
  const { providers, smallCellRule } = patterns;
  if (providers.length === 0) return null;
  return (
    <section aria-labelledby="patterns-h" className="mt-8" data-testid="provider-links">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <h2 id="patterns-h" className="m-0 text-section font-medium">
          By provider
        </h2>
        <HonestyLabel kind="simulated">Simulated cohort</HonestyLabel>
      </div>
      <details className="group">
        <summary className="flex min-h-11 cursor-pointer list-none items-center gap-2 text-body [&::-webkit-details-marker]:hidden">
          <svg
            aria-hidden
            width="16"
            height="16"
            viewBox="0 0 16 16"
            fill="none"
            stroke="var(--color-muted)"
            strokeWidth="1.5"
            strokeLinecap="round"
            strokeLinejoin="round"
            className="flex-none transition-transform group-open:rotate-90 motion-reduce:transition-none"
          >
            <path d="M6 3.5l4.5 4.5L6 12.5" />
          </svg>
          <span>Closure and largest stall for {providers.length} providers, linked to the patient list</span>
        </summary>
        <p className="m-0 mt-1 text-small text-muted">
          Each link opens the patient list filtered to the same provider and state or stall. Counts are out of the provider&apos;s cases across every
          programme in the run.
          {smallCellRule && <> Small cells are suppressed at source ({smallCellRule}) and are never shown as a number or linked.</>} Origin is not broken
          down here: it is a synthetic cohort attribute, not part of the case record, so it cannot open a filtered list.
        </p>
        <ul aria-labelledby="patterns-h" className="m-0 mt-2 list-none p-0 text-body tabular-nums">
          {providers.map((r) => (
            <Line key={r.key} row={r} />
          ))}
        </ul>
      </details>
    </section>
  );
}
