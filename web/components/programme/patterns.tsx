import Link from "next/link";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { loopsText, patternText, type PatternRow, type Patterns } from "./model";

/*
 * Provider and origin patterns as compact rows of text (issue #45). The owner chose filtered Patients links over a
 * dedicated breakdown table: a pattern links only when the case list can filter to exactly its cases. Suppressed
 * small cells show as "fewer than 5" and never link.
 */

const link = "inline-flex min-h-11 items-center md:min-h-0";

function Row({ row, group }: { row: PatternRow; group: string }) {
  const count = loopsText(row.loops);
  return (
    <li className="border-b border-rule py-2 last:border-b-0" data-testid={`pattern-${group}-${row.key}`}>
      <p className="m-0 text-body tabular-nums">
        {row.href ? (
          <Link href={row.href} className={`${link} font-medium`}>
            {row.name}, {count}
            <span className="sr-only"> (simulated). Opens the patient list for this provider.</span>
          </Link>
        ) : (
          <>
            <span className="font-medium">{row.name}</span>, {count}
          </>
        )}
      </p>
      {row.patterns.length > 0 && (
        <ul className="m-0 mt-0.5 flex list-none flex-col gap-x-4 p-0 text-small text-ink-soft tabular-nums sm:flex-row sm:flex-wrap">
          {row.patterns.map((p) => {
            const text = patternText(p);
            return (
              <li key={p.kind}>
                {p.href ? (
                  <Link href={p.href} className={link}>
                    <span className="sr-only">{row.name}: </span>
                    {text}
                    <span className="sr-only"> (simulated). Opens the patient list filtered to these cases.</span>
                  </Link>
                ) : (
                  text
                )}
              </li>
            );
          })}
        </ul>
      )}
    </li>
  );
}

function Group({ id, title, note, rows }: { id: string; title: string; note: string; rows: PatternRow[] }) {
  return (
    <div>
      <h3 id={`${id}-h`} className="m-0 border-b border-rule pb-1 text-small font-medium text-muted">
        {title}
      </h3>
      <p className="m-0 mt-1 text-small text-muted">{note}</p>
      <ul aria-labelledby={`${id}-h`} className="m-0 list-none p-0">
        {rows.map((r) => (
          <Row key={r.key} row={r} group={id} />
        ))}
      </ul>
    </div>
  );
}

/** Static supporting rows with no object state, so they sit on the page grid unframed, like Safety (DEC-3). */
export function PatternLinks({ patterns }: { patterns: Patterns }) {
  const { providers, origins, smallCellRule } = patterns;
  if (providers.length === 0 && origins.length === 0) return null;
  return (
    <section aria-labelledby="patterns-h" className="mt-8">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 id="patterns-h" className="m-0 text-section font-medium">
          By provider and origin
        </h2>
        <HonestyLabel kind="simulated">Simulated cohort</HonestyLabel>
      </div>
      <p className="m-0 mt-1 text-body text-ink-soft">
        Each count is out of the row&apos;s cases, across every programme in the run. Linked counts open the patient list filtered to exactly those
        cases. Notification counts only the colorectal and cervical result letter; the other pathways record none.
        {smallCellRule && <> Small cells are suppressed at source ({smallCellRule}) and are never shown or linked.</>}
      </p>
      <div className="mt-4 grid items-start gap-gap lg:grid-cols-2">
        {providers.length > 0 && (
          <Group id="patterns-providers" title="By provider" note="Closure and the largest stall link to the filtered patient list." rows={providers} />
        )}
        {origins.length > 0 && (
          <Group
            id="patterns-origins"
            title="By origin"
            note="Origin is a synthetic cohort attribute, not part of the case record, so these counts do not link. Groups differ in size and programme mix; differences describe this simulated cohort and do not show that origin causes them."
            rows={origins}
          />
        )}
      </div>
    </section>
  );
}
