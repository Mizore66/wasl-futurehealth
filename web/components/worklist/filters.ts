import { CaseState, Dropoff } from "@/lib/api/schema";

/* Query-string filters for /app/patients; Programme's stall links land here. */

export type CaseFilters = { state?: CaseState; dropoff?: Exclude<Dropoff, "">; overdue?: boolean; q?: string };
type SearchParams = Record<string, string | string[] | undefined>;

const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

/** Unknown values are dropped rather than rejected: a stale link shows everything, not an error. */
export function parseCaseFilters(sp: SearchParams): CaseFilters {
  const state = CaseState.safeParse(one(sp.state));
  const dropoff = Dropoff.safeParse(one(sp.dropoff));
  const overdue = one(sp.overdue);
  const q = one(sp.q)?.trim();
  return {
    ...(state.success && { state: state.data }),
    ...(dropoff.success && dropoff.data !== "" && { dropoff: dropoff.data }),
    ...((overdue === "true" || overdue === "false") && { overdue: overdue === "true" }),
    ...(q && { q }),
  };
}

export function filtersHref(base: string, f: CaseFilters): string {
  const params = new URLSearchParams();
  if (f.state) params.set("state", f.state);
  if (f.dropoff) params.set("dropoff", f.dropoff);
  if (f.overdue !== undefined) params.set("overdue", String(f.overdue));
  if (f.q) params.set("q", f.q);
  const s = params.toString();
  return s ? `${base}?${s}` : base;
}
