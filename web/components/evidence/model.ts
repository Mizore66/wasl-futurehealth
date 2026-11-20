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

/* ---------- Chart shapes ---------- */

export type RunKey = "v2" | "v1" | "deployed";
export const RUN_LABEL: Record<RunKey, string> = { v2: "Unseen test data", v1: "Tuning data", deployed: "Demo run" };

/** "Worklist size (patients to chase) (frozen v2 five-seed mean)" → metric "Worklist size (patients to chase)", run "v2". */
export function splitRun(row: string): { metric: string; run: RunKey | null; runText: string } {
  const m = row.match(/^(.*)\s+\(([^()]*)\)\s*$/);
  if (!m) return { metric: row, run: null, runText: "" };
  const t = m[2];
  const run: RunKey | null = /\bv2\b/.test(t) ? "v2" : /\bv1\b/.test(t) ? "v1" : /deployed|demo/i.test(t) ? "deployed" : null;
  return run ? { metric: m[1], run, runText: t } : { metric: row, run: null, runText: "" };
}

export type GapRow = EvidencePer1000Row & { metric: string };

/** The per-1,000 rows grouped by run, in the order v2, v1, deployed (only runs present). */
export function gapRuns(rows: EvidencePer1000Row[]): { run: RunKey; runText: string; rows: GapRow[] }[] {
  const out = new Map<RunKey, { run: RunKey; runText: string; rows: GapRow[] }>();
  for (const r of rows) {
    const { metric, run, runText } = splitRun(r.row);
    if (!run) continue;
    const g = out.get(run) ?? { run, runText, rows: [] };
    g.rows.push({ ...r, metric });
    out.set(run, g);
  }
  return (["v2", "v1", "deployed"] as RunKey[]).flatMap((k) => (out.has(k) ? [out.get(k)!] : []));
}

export const isPer1000 = (r: EvidencePer1000Row) => /1,?000/.test(r.unit);

/** "[v2] Colorectal (CRC) false closures" → { noise: "v2", name: "Colorectal (CRC) false closures" }. */
export function noiseOf(metric: string): { noise: "v1" | "v2" | null; name: string } {
  const m = metric.match(/^\[(v1|v2)\]\s*(.*)$/);
  return m ? { noise: m[1] as "v1" | "v2", name: m[2] } : { noise: null, name: metric };
}

export type SafetyPair = { name: string; v1: EvidenceSafety | null; v2: EvidenceSafety | null };

/** Safety rows paired by name across noise models: false closures first, then the rest, each in file order. */
export function safetyPairs(rows: EvidenceSafety[]): { closures: SafetyPair[]; other: SafetyPair[] } {
  const by = new Map<string, SafetyPair>();
  for (const r of rows) {
    const { noise, name } = noiseOf(r.metric);
    const p = by.get(name) ?? { name, v1: null, v2: null };
    p[noise ?? "v2"] = r;
    by.set(name, p);
  }
  const all = [...by.values()];
  return { closures: all.filter((p) => isFalseClosure(p.name)), other: all.filter((p) => !isFalseClosure(p.name)) };
}

/** "Colorectal (CRC) false closures" → "Colorectal (CRC)". */
export const closureName = (name: string) => name.replace(/\s*false closures$/i, "");

export type RatePair = { name: string; v1: EvidenceLinkage | null; v2: EvidenceLinkage | null };

/**
 * Linkage rates as two panels on one share axis: false-match and missed-match, one row per programme plus the
 * pair-level linkage row. Rows that are not shares (review load, F1) come back separately for stat tiles.
 */
export function ratePanels(rows: EvidenceLinkage[]) {
  const pairs = linkageRows(rows);
  const panel = (re: RegExp): RatePair[] =>
    pairs
      .filter((p) => re.test(p.metric) && isShareMetric(p.metric) && !/F1/.test(p.metric))
      .map((p) => ({ name: p.metric.split(":")[0].trim(), v1: p.v1, v2: p.v2 }));
  return {
    falseMatch: panel(/false-match/i),
    missed: panel(/missed-match/i),
    other: pairs.filter((p) => !/false-match|missed-match/i.test(p.metric)),
  };
}

export type WorstRange = { label: string; noise: "v1" | "v2" | null; low: number; high: number; lowWhat: string; highWhat: string; value: string };

/**
 * Worst-case rows that state a range of a share: "0.818 (seed 37) to 0.855 (seed 41)" or
 * "40-59: 0.815 (range 0.815 to 0.867); …". Rows in another shape (points, items per 100 loops) are left out.
 */
export function worstRanges(rows: { label: string; value: string }[]): WorstRange[] {
  const out: WorstRange[] = [];
  for (const r of rows) {
    const { noise, name } = noiseOf(r.label);
    const seeds = r.value.match(/^(0\.\d+) \((seed \d+)\) to (0\.\d+) \((seed \d+)\)/);
    const strata = r.value.match(/^([^:]+): (0\.\d+) \(range (0\.\d+) to (0\.\d+)\)/);
    if (seeds) out.push({ label: name, noise, low: +seeds[1], high: +seeds[3], lowWhat: seeds[2], highWhat: seeds[4], value: r.value });
    else if (strata) out.push({ label: name, noise, low: +strata[3], high: +strata[4], lowWhat: strata[1].trim(), highWhat: "best stratum", value: r.value });
  }
  return out;
}

/** Worst rows that are not a share range, for stat tiles: the label and the leading figure of its value. */
export function worstOthers(rows: { label: string; value: string }[]) {
  const ranged = new Set(worstRanges(rows).map((w) => w.value));
  return rows
    .filter((r) => !ranged.has(r.value))
    .map((r) => {
      const { noise, name } = noiseOf(r.label);
      const lead = r.value.match(/^(?:([A-Za-z_]+): )?(\d+(?:\.\d+)?) ([a-z]+)/);
      return { name, noise, value: r.value, lead: lead ? `${lead[2]} ${lead[3]}` : r.value, who: lead?.[1] ?? null };
    });
}

/** Claim counts by status, in the order built, simulated, designed. */
export function claimCounts(claims: { status: "built" | "simulated" | "designed" }[]) {
  return (["built", "simulated", "designed"] as const).map((s) => ({ status: s, count: claims.filter((c) => c.status === s).length }));
}

/** The first URL in a free-text source line. */
export const firstUrl = (s: string | null) => s?.match(/https?:\/\/\S+/)?.[0]?.replace(/[).,]+$/, "") ?? null;

/** A share as a percent for chart labels: 0.978983 → "97.9%", 0.0134 → "1.34%". */
export const pctLabel = (share: number) => sharePct(share);
