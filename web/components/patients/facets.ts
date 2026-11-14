import type { CaseSummary, Programme } from "@/lib/api/schema";
import { effectiveOwner } from "@/lib/api/owner";
import { matches } from "@/lib/api/source";
import { toCaseQuery, UNOWNED, type CaseFilters } from "@/components/worklist/filters";
import { ownerLabel, PROGRAMME_NAMES, providerLabel } from "@/components/worklist/model";

type Consent = CaseSummary["consent"];
type ReviewStatus = CaseSummary["review"];

/* Choices and counts for the Patients refine controls, counted from the cases themselves (never a fixed list). */

export type Option = { value: string; label: string; count: number };
export type FacetKey = "programme" | "provider" | "consent" | "review" | "owner";
export type Facets = Record<FacetKey, Option[]>;

export const FACET_LABELS: Record<FacetKey, string> = {
  programme: "Programme",
  provider: "Provider",
  consent: "Consent",
  review: "Review",
  owner: "Owner",
};

export const CONSENT_LABELS: Record<Consent, string> = {
  granted: "Granted",
  denied: "Denied",
  opted_out: "Opted out",
  unknown: "Not recorded",
};

export const REVIEW_LABELS: Record<ReviewStatus, string> = {
  none: "No review needed",
  open: "Review open",
  signed: "Review signed",
};

const valueOf = (c: CaseSummary, k: FacetKey): string => (k === "owner" ? effectiveOwner(c) || UNOWNED : c[k]);

function labelOf(k: FacetKey, v: string): string {
  switch (k) {
    case "programme":
      return PROGRAMME_NAMES[v as Programme] ?? v;
    case "provider":
      return providerLabel(v);
    case "consent":
      return CONSENT_LABELS[v as Consent] ?? v;
    case "review":
      return REVIEW_LABELS[v as ReviewStatus] ?? v;
    case "owner":
      return v === UNOWNED ? "No owner" : ownerLabel(v);
  }
}

const KEYS = Object.keys(FACET_LABELS) as FacetKey[];

/**
 * For each control, the values present in `all` and how many cases each would show with every OTHER filter applied,
 * so the numbers match the list you get by choosing them. A value already chosen stays listed even at 0.
 * Unowned sorts last; the rest by label.
 */
export function facets(all: CaseSummary[], f: CaseFilters): Facets {
  const out = {} as Facets;
  for (const k of KEYS) {
    const others = toCaseQuery({ ...f, [k]: undefined });
    const counts = new Map<string, number>();
    for (const c of all) {
      const v = valueOf(c, k);
      if (!counts.has(v)) counts.set(v, 0);
      if (matches(c, others)) counts.set(v, counts.get(v)! + 1);
    }
    const chosen = f[k];
    if (chosen && !counts.has(chosen)) counts.set(chosen, 0);
    out[k] = [...counts]
      .map(([value, count]) => ({ value, label: labelOf(k, value), count }))
      .sort((a, b) => Number(a.value === UNOWNED) - Number(b.value === UNOWNED) || a.label.localeCompare(b.label, "en", { numeric: true }));
  }
  return out;
}

/** The programmes among the cases that match `f`, with how many of each. Order follows PROGRAMME_NAMES. */
export function programmesInView(all: CaseSummary[], f: CaseFilters): { programme: Programme; name: string; count: number }[] {
  const q = toCaseQuery(f);
  const counts = new Map<Programme, number>();
  for (const c of all) if (matches(c, q)) counts.set(c.programme, (counts.get(c.programme) ?? 0) + 1);
  return (Object.keys(PROGRAMME_NAMES) as Programme[])
    .filter((p) => counts.has(p))
    .map((p) => ({ programme: p, name: PROGRAMME_NAMES[p], count: counts.get(p)! }));
}

/** One phrase per active refine filter, for the summary line: ["Colorectal screening", "Provider: Al Mushrif", ...]. */
export function activeRefinements(f: CaseFilters): { key: FacetKey; text: string }[] {
  return KEYS.flatMap((k) => {
    const v = f[k];
    if (!v) return [];
    return [{ key: k, text: k === "programme" ? labelOf(k, v) : `${FACET_LABELS[k]}: ${labelOf(k, v)}` }];
  });
}

export { labelOf as facetValueLabel };
