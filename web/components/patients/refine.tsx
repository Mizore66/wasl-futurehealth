import { Button } from "@/components/ui/button";
import type { FilterSpec } from "@/components/ui/filters/types";
import { filtersHref, type CaseFilters } from "@/components/worklist/filters";
import { FACET_LABELS, type FacetKey, type Facets } from "./facets";

const ORDER: FacetKey[] = ["programme", "provider", "consent", "review", "owner"];
const ALL_WORDS: Record<FacetKey, string> = {
  programme: "All programmes",
  provider: "All providers",
  consent: "Any consent",
  review: "Any review state",
  owner: "Anyone, or no owner",
};

const CLEARED: CaseFilters = { programme: undefined, provider: undefined, consent: undefined, review: undefined, owner: undefined };

/** Programme, provider, consent, review and owner as filter menus. Each choice keeps every other filter and the search. */
export function refineMenus(facets: Facets, filters: CaseFilters, base = "/app/patients"): FilterSpec[] {
  return ORDER.map((k) => ({
    label: FACET_LABELS[k],
    value: filters[k],
    options: facets[k].map((o) => ({ ...o, href: filtersHref(base, { ...filters, [k]: o.value }) })),
    clearHref: filtersHref(base, { ...filters, [k]: undefined }),
  }));
}

/** The URL with every refine filter removed, keeping the status view and the search. */
export const clearRefineHref = (filters: CaseFilters, base = "/app/patients") => filtersHref(base, { ...filters, ...CLEARED });

/**
 * Without JavaScript the filter menus cannot open, so the same choices come as plain labelled selects in a GET form.
 * Counts already apply the other active filters.
 */
export function NativeRefine({ facets, filters, base = "/app/patients" }: { facets: Facets; filters: CaseFilters; base?: string }) {
  return (
    <noscript>
      <div data-testid="refine">
        <form action={base} method="get" className="grid gap-3 py-3 sm:grid-cols-2 lg:grid-cols-3">
          {filters.state && <input type="hidden" name="state" value={filters.state} />}
          {filters.dropoff && <input type="hidden" name="dropoff" value={filters.dropoff} />}
          {filters.overdue !== undefined && <input type="hidden" name="overdue" value={String(filters.overdue)} />}
          {filters.closed !== undefined && <input type="hidden" name="closed" value={String(filters.closed)} />}
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
      </div>
    </noscript>
  );
}
