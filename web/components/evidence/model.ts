import data from "@/data/evidence.json";
import {
  Evidence,
  type EvidenceLinkage,
  type EvidencePer1000Row,
  type EvidenceSafety,
  type EvidenceScenario,
  type EvidenceTornadoRow,
} from "@/lib/api/evidence-schema";

export const REPO = "https://github.com/Mizore66/wasl-futurehealth";
export const HOSTED = "https://wasl-topaz-mu.vercel.app";

/** Parsed once at build time; a contract break throws here rather than rendering blanks. */
export function loadEvidence(raw: unknown = data): Evidence {
  return Evidence.parse(raw);
}

export const fmt = (n: number) => n.toLocaleString("en-US");

/** A value given as a percentage (30.5) or as a share (0.305), as a percentage. */
export const asPercent = (v: number) => (Math.abs(v) <= 1 ? v * 100 : v);

/** A share as a percent: two decimals below 1%, else one. */
export function sharePct(share: number, digits?: number): string {
  const p = share * 100;
  return `${p.toFixed(digits ?? (Math.abs(p) < 1 ? 2 : 1))}%`;
}

/** Interval in the value's own precision: "0.92% (0.74–1.10%)". */
export function withInterval(l: Pick<EvidenceLinkage, "value" | "low" | "high">, metric = ""): string {
  // Rates and F1 are shares shown as percents; counts such as "review items per 100 loops" or "hours per 1,000" are not.
  if (!isShareMetric(metric)) {
    const n = (v: number) => (Math.abs(v) >= 10 ? v.toFixed(1) : v.toFixed(2));
    return `${n(l.value)} (${n(l.low)}–${n(l.high)})`;
  }
  const digits = Math.abs(l.value * 100) < 1 ? 2 : 1;
  const p = (v: number) => (v * 100).toFixed(digits);
  return `${p(l.value)}% (${p(l.low)}–${p(l.high)}%)`;
}

/** A metric whose value is a share (0–1): rates, precision/recall, F1. Empty means a share (the historical default). */
export function isShareMetric(metric: string): boolean {
  if (!metric) return true;
  return !/per 100|per 1,?000|hours|items/i.test(metric);
}

/** An upper bound as a percent to two significant figures: 0.00117 → "0.12%". */
export const boundPct = (share: number) => `${Number((share * 100).toPrecision(2))}%`;

/** "0 of 3,161" and, for a zero-event count with a bound, "95% upper bound 0.12%". Never a 100% score. */
export function safetyWords(s: EvidenceSafety): { count: string; bound: string | null } {
  const count = s.denominator === null ? fmt(s.events) : `${fmt(s.events)} of ${fmt(s.denominator)}`;
  return { count, bound: s.upper_95 === null ? null : `95% upper bound ${boundPct(s.upper_95)}` };
}

const isFalseClosure = (metric: string) => /false[\s_-]?closure/i.test(metric);

/** False closures first (the dangerous error), the rest in file order. */
export function safetyOrder(rows: EvidenceSafety[]): EvidenceSafety[] {
  return [...rows.filter((r) => isFalseClosure(r.metric)), ...rows.filter((r) => !isFalseClosure(r.metric))];
}

/** Machine names (crc_false_closures) read as words; human labels pass through. */
export function humanise(metric: string): string {
  if (!/_/.test(metric) || /\s/.test(metric)) return metric;
  const words = metric.replace(/_/g, " ").replace(/\b(crc|dr|ldct|ifhas|kpi|eid)\b/g, (w) => w.toUpperCase());
  return words.charAt(0).toUpperCase() + words.slice(1);
}

/** One row per metric with its v1 and v2 values side by side, in first-seen order. */
export function linkageRows(rows: EvidenceLinkage[]): { metric: string; v1: EvidenceLinkage | null; v2: EvidenceLinkage | null }[] {
  const out = new Map<string, { metric: string; v1: EvidenceLinkage | null; v2: EvidenceLinkage | null }>();
  for (const r of rows) {
    const row = out.get(r.metric) ?? { metric: r.metric, v1: null, v2: null };
    row[r.noise] = r;
    out.set(r.metric, row);
  }
  return [...out.values()];
}

const complete = (r: EvidencePer1000Row) => r.eid_only !== null && r.wasl !== null && r.truth !== null;
const perThousand = (r: EvidencePer1000Row) => /1,?000/.test(r.unit);

/**
 * The headline: the completions row drawn as bars (the first per-1,000 row with all three values) and the
 * "wrongly flagged not followed up" row as the lead figure. Either is null when the file has no such row.
 */
export function headline(rows: EvidencePer1000Row[]) {
  const flagged = rows.find((r) => /flag|not followed/i.test(r.row)) ?? null;
  const chart = rows.find((r) => r !== flagged && perThousand(r) && complete(r)) ?? null;
  return { chart, flagged };
}

/** The calibration figure on the per-1,000 scale (30.5% → 305). */
export const calibrationPer1000 = (value: number) => Math.round(asPercent(value) * 10);

/** A round axis end at or above the largest value: 319 → 400, 6.05 → 8. */
export function niceMax(max: number): number {
  if (max <= 0) return 1;
  const mag = 10 ** Math.floor(Math.log10(max));
  const step = [1, 2, 2.5, 4, 5, 8, 10].find((s) => s * mag >= max) ?? 10;
  return step * mag;
}

const ORDER: EvidenceScenario["name"][] = ["worst", "base", "best"];
export const scenarioOrder = (rows: EvidenceScenario[]) => [...rows].sort((a, b) => ORDER.indexOf(a.name) - ORDER.indexOf(b.name));

/** Tornado bars, widest swing first, on a shared domain. */
export function tornado(rows: EvidenceTornadoRow[]) {
  const bars = rows
    .map((r) => ({ ...r, lo: Math.min(r.out_low, r.out_high), hi: Math.max(r.out_low, r.out_high) }))
    .sort((a, b) => b.hi - b.lo - (a.hi - a.lo));
  const min = Math.min(0, ...bars.map((b) => b.lo));
  const max = niceMax(Math.max(0, ...bars.map((b) => b.hi)));
  return { bars, min, max };
}

/** Up to two decimals, trailing zeros dropped: 0.75, 6.05, 30. */
export const num = (v: number) => fmt(Number(v.toFixed(2)));

/** Where a claim's evidence lives: a URL as is, an app path in the app, a repo path on GitHub. */
export function evidenceHref(evidence: string): { href: string; external: boolean } | null {
  const e = evidence.trim();
  if (!e) return null;
  if (/^https?:\/\//.test(e)) return { href: e, external: true };
  if (e.startsWith("/")) return { href: e, external: false };
  const kind = e.endsWith("/") ? "tree" : "blob";
  return { href: `${REPO}/${kind}/main/${e.replace(/\/$/, "")}`, external: true };
}

/** "2 Nov 2026, 04:33 UTC" from the frozen timestamp. */
export function frozenWords(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const date = d.toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric", timeZone: "UTC" });
  const time = d.toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit", hour12: false, timeZone: "UTC" });
  return `${date}, ${time} UTC`;
}
