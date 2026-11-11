import Link from "next/link";
import { Button } from "@/components/ui/button";
import { filtersHref, type CaseFilters } from "@/components/worklist/filters";
import { activeRefinements, FACET_LABELS, type FacetKey, type Facets } from "./facets";

const ORDER: FacetKey[] = ["programme", "provider", "consent", "review", "owner"];
const ALL_WORDS: Record<FacetKey, string> = {
  programme: "All programmes",
  provider: "All providers",
  consent: "Any consent",
  review: "Any review state",
  owner: "Anyone, or no owner",
};

const CLEARED: CaseFilters = { programme: undefined, provider: undefined, consent: undefined, review: undefined, owner: undefined };

/**
 * Programme, provider, consent, review and owner as plain labelled selects in a GET form, so they work before
 * (and without) hydration and every choice lands in the URL. Counts already apply the other active filters.
 */
export function Refine({ facets, filters, base = "/app/patients" }: { facets: Facets; filters: CaseFilters; base?: string }) {
  const active = activeRefinements(filters);
  return (
    <div className="border-b border-line" data-testid="refine">
      <details open={active.length > 0} className="group">
        <summary className="flex min-h-11 cursor-pointer items-center gap-2 px-4 text-body font-medium">
          Filter by programme, provider, consent, review or owner
          {active.length > 0 && <span className="rounded-chip bg-active px-2 text-small font-normal">{active.length} on</span>}
        </summary>
        <form action={base} method="get" className="grid gap-3 px-4 pb-4 sm:grid-cols-2 lg:grid-cols-3">
          {filters.state && <input type="hidden" name="state" value={filters.state} />}
          {filters.dropoff && <input type="hidden" name="dropoff" value={filters.dropoff} />}
          {filters.overdue !== undefined && <input type="hidden" name="overdue" value={String(filters.overdue)} />}
          {filters.q && <input type="hidden" name="q" value={filters.q} />}
          {ORDER.map((k) => {
            const options = facets[k];
            const total = options.reduce((n, o) => n + o.count, 0);
            return (
              <label key={k} className="flex min-w-0 flex-col gap-1 text-small text-muted">
                {FACET_LABELS[k]}
                <select
                  name={k}
                  defaultValue={filters[k] ?? ""}
                  className="h-11 w-full min-w-0 rounded-control border border-input bg-surface px-2 text-body text-ink"
                >
                  <option value="">
                    {ALL_WORDS[k]} ({total})
                  </option>
                  {options.map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label} ({o.count})
                    </option>
                  ))}
                </select>
              </label>
            );
          })}
          <div className="flex items-end">
            <Button variant="quiet" size="touch">Apply filters</Button>
          </div>
        </form>
      </details>
      {active.length > 0 && (
        <p className="m-0 flex flex-wrap items-center gap-x-3 gap-y-0 px-4 pb-2 text-body" data-testid="active-filters">
          <span className="text-muted">Filtered by</span>
          {active.map((a) => (
            <Link
              key={a.key}
              href={filtersHref(base, { ...filters, [a.key]: undefined })}
              className="inline-flex min-h-11 items-center rounded-chip border border-control px-3 text-ink no-underline hover:bg-ground"
              aria-label={`${a.text}. Remove this filter`}
            >
              {a.text} <span aria-hidden="true">&nbsp;×</span>
            </Link>
          ))}
          <Link href={filtersHref(base, { ...filters, ...CLEARED })} className="inline-flex min-h-11 items-center">
            Clear these filters
          </Link>
        </p>
      )}
    </div>
  );
}
