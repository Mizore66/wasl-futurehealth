import type { CaseSummary } from "@/lib/api/schema";
import type { CaseQuery } from "@/lib/api/source";
import { CaseState, Consent, Dropoff, Programme, ReviewStatus } from "@/lib/api/schema";

/* Query-string filters for /app/patients; Programme's stall links land here. */

export type CaseFilters = {
  programme?: Programme;
  state?: CaseState;
  dropoff?: Exclude<Dropoff, "">;
  overdue?: boolean;
  /** The `closed` projection: the closed state plus every no-action resolution. `state: "closed"` stays the single state. */
  closed?: boolean;
  /** A provider id from the run data. Unknown ids are kept, so a stale link shows an honest empty list. */
  provider?: string;
  consent?: CaseSummary["consent"];
  review?: CaseSummary["review"];
  /** A practitioner reference from the run data, or `UNOWNED`. */
  owner?: string;
  q?: string;
};

/** `?owner=none`: cases nobody owns (the API's `owner` is an empty string for these). */
export const UNOWNED = "none";
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
  const closed = one(sp.closed);
  const q = one(sp.q)?.trim();
  const provider = one(sp.provider)?.trim();
  const consent = Consent.safeParse(one(sp.consent));
  const review = ReviewStatus.safeParse(one(sp.review));
  const owner = one(sp.owner)?.trim();
  return {
    ...(programme.success && { programme: programme.data }),
    ...(state.success && { state: state.data }),
    ...(dropoff.success && dropoff.data !== "" && { dropoff: dropoff.data }),
    ...((overdue === "true" || overdue === "false") && { overdue: overdue === "true" }),
    ...((closed === "true" || closed === "false") && { closed: closed === "true" }),
    ...(provider && { provider }),
    ...(consent.success && { consent: consent.data }),
    ...(review.success && { review: review.data }),
    ...(owner && { owner }),
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
  if (f.closed !== undefined) params.set("closed", String(f.closed));
  if (f.provider) params.set("provider", f.provider);
  if (f.consent) params.set("consent", f.consent);
  if (f.review) params.set("review", f.review);
  if (f.owner) params.set("owner", f.owner);
  if (f.q) params.set("q", f.q);
  if (page > 1) params.set("page", String(page));
  const s = params.toString();
  return s ? `${base}?${s}` : base;
}

/** The filters as a `getCases` query. `owner=none` becomes `unowned`, which the live API cannot express. */
export function toCaseQuery(f: CaseFilters): CaseQuery {
  const { owner, ...rest } = f;
  return { ...rest, ...(owner === UNOWNED ? { unowned: true } : owner ? { owner } : {}) };
}
