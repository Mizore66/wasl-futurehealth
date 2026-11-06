import { CaseState, Dropoff, Programme } from "@/lib/api/schema";

/* Query-string filters for /app/patients; Programme's stall links land here. */

export type CaseFilters = {
  programme?: Programme;
  state?: CaseState;
  dropoff?: Exclude<Dropoff, "">;
  overdue?: boolean;
  q?: string;
};
type SearchParams = Record<string, string | string[] | undefined>;

/** Cases per page on /app/patients. */
export const PAGE_SIZE = 25;

const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);

/** Unknown values are dropped rather than rejected: a stale link shows everything, not an error. */
export function parseCaseFilters(sp: SearchParams): CaseFilters {
  const programme = Programme.safeParse(one(sp.programme));
  const state = CaseState.safeParse(one(sp.state));
  const dropoff = Dropoff.safeParse(one(sp.dropoff));
  const overdue = one(sp.overdue);
  const q = one(sp.q)?.trim();
  return {
    ...(programme.success && { programme: programme.data }),
    ...(state.success && { state: state.data }),
    ...(dropoff.success && dropoff.data !== "" && { dropoff: dropoff.data }),
    ...((overdue === "true" || overdue === "false") && { overdue: overdue === "true" }),
    ...(q && { q }),
  };
}

/** `?page=` as a 1-based whole number; anything else is page 1. */
export function parsePage(sp: SearchParams): number {
  const n = Number(one(sp.page));
  return Number.isInteger(n) && n >= 1 ? n : 1;
}

/** Filters (and optionally a page) as a link. Page 1 leaves `page` out. */
export function filtersHref(base: string, f: CaseFilters, page = 1): string {
  const params = new URLSearchParams();
  if (f.programme) params.set("programme", f.programme);
  if (f.state) params.set("state", f.state);
  if (f.dropoff) params.set("dropoff", f.dropoff);
  if (f.overdue !== undefined) params.set("overdue", String(f.overdue));
  if (f.q) params.set("q", f.q);
  if (page > 1) params.set("page", String(page));
  const s = params.toString();
  return s ? `${base}?${s}` : base;
}
