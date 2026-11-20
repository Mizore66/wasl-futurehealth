"use client";

import { useState } from "react";
import { Axis, Bar, ChartRow, CheckedBar, Dot, Grid, Label, Legend, Seg, Tick, TipRows, Toggle, ZeroFigure, scale, ticksTo } from "@/components/charts/kit";
import { CHROME, SERIES } from "@/components/charts/palette";
import { ChartFigure } from "@/components/programme/chart-kit";
import type {
  EvidenceBenchmark,
  EvidenceLinkage,
  EvidenceScenario,
  EvidenceSafety,
  EvidenceTornadoRow,
} from "@/lib/api/evidence-schema";
import {
  RUN_LABEL,
  boundPct,
  closureName,
  fmt,
  isPer1000,
  niceMax,
  num,
  safetyWords,
  sharePct,
  tornado,
  withInterval,
  type GapRow,
  type RatePair,
  type RunKey,
  type SafetyPair,
  type WorstRange,
} from "./model";

const round = (v: number) => fmt(Math.round(v));
const unitWord = (r: GapRow) => (r.unit === "%" ? "%" : "");

/* ---------- The measured gap ---------- */

type Calibration = { per1000: number; short: string; caveat: string };

/** One dumbbell row: Emirates-ID-only (ochre) to Wasl (jade), with the simulated truth as an ink tick. */
function DumbbellRow({ r, x, show, calibration }: { r: GapRow; x: (v: number) => number; show: (v: number) => string; calibration?: Calibration | null }) {
  const eid = r.eid_only;
  const wasl = r.wasl;
  return (
    <ChartRow
      label={r.metric}
      track="h-11"
      testId="gap-row"
      tip={
        <TipRows
          title={r.metric}
          rows={[
            ...(wasl !== null ? [{ color: SERIES.wasl, name: "Wasl", value: show(wasl) }] : []),
            ...(eid !== null ? [{ color: SERIES.comparator, name: "Emirates ID only", value: show(eid) }] : []),
            ...(r.truth !== null ? [{ color: CHROME.reference, name: "Simulated truth", value: show(r.truth) }] : []),
            ...(calibration ? [{ color: CHROME.deemphasis, name: calibration.short, value: show(calibration.per1000) }] : []),
          ]}
          note={calibration?.caveat}
        />
      }
    >
      <Grid ticks={[0]} x={x} />
      {calibration && <Tick x={x(calibration.per1000)} color={CHROME.deemphasis} y={50} h={44} w={2} />}
      {eid !== null && wasl !== null && <Seg from={x(eid)} to={x(wasl)} color={CHROME.deemphasis} y={68} />}
      {r.truth !== null && <Tick x={x(r.truth)} color={CHROME.reference} y={68} h={16} />}
      {eid !== null && <Dot x={x(eid)} y={68} color={SERIES.comparator} />}
      {wasl !== null && <Dot x={x(wasl)} y={68} color={SERIES.wasl} />}
      {eid !== null && (
        <Label x={x(eid)} y={68} side="above">
          {show(eid)}
        </Label>
      )}
      {wasl !== null && (
        <Label x={x(wasl)} y={68} side="above" strong>
          {show(wasl)}
        </Label>
      )}
    </ChartRow>
  );
}

/**
 * The headline gap for one run at a time (held-out v2 by default): the wrongly-flagged figure as the hero, the
 * per-1,000 rows as a dumbbell with the SEHA rate as a reference line, and the % rows as a second dumbbell.
 */
export function GapChart({
  runs,
  calibration,
  source,
}: {
  runs: { run: RunKey; runText: string; rows: GapRow[] }[];
  calibration: Calibration | null;
  source: React.ReactNode;
}) {
  const [run, setRun] = useState<RunKey>(runs[0]?.run ?? "v2");
  const current = runs.find((r) => r.run === run) ?? runs[0];
  if (!current) return <p className="m-0 mt-2 text-small text-muted">No per-1,000 rows in this evidence file.</p>;
  const per1000 = current.rows.filter(isPer1000);
  const percent = current.rows.filter((r) => r.unit === "%");
  const flagged = current.rows.find((r) => /flag|not followed/i.test(r.metric)) ?? null;
  const completions = per1000.find((r) => /completions/i.test(r.metric)) ?? null;
  const max1000 = niceMax(Math.max(...per1000.flatMap((r) => [r.eid_only ?? 0, r.wasl ?? 0, r.truth ?? 0]), calibration?.per1000 ?? 0));
  const x1000 = scale(0, max1000);
  const xPct = scale(0, 100);
  const legend = [
    { color: SERIES.comparator, label: "Emirates ID only" },
    { color: SERIES.wasl, label: "Wasl" },
    { color: CHROME.reference, label: "Simulated truth", shape: "tick" as const },
  ];
  const cell = (r: GapRow, v: number | null) => (v === null ? "–" : `${fmt(v)}${unitWord(r)}`);

  return (
    <div className="mt-3">
      {runs.length > 1 && (
        <Toggle label="Evaluation run" value={run} onChange={setRun} options={runs.map((r) => ({ value: r.run, label: RUN_LABEL[r.run] }))} />
      )}
      {flagged && flagged.eid_only !== null && flagged.wasl !== null && (
        <p data-testid="flagged" className="m-0 mt-4 flex flex-wrap items-baseline gap-x-3 gap-y-1">
          <span className="whitespace-nowrap font-sans text-[52px] font-semibold leading-[56px] tracking-[-0.02em] text-ink">
            {round(flagged.eid_only)} <span aria-hidden>→</span>
            <span className="sr-only">to</span> {round(flagged.wasl)}
          </span>
          <span className="text-body text-ink-soft">wrongly flagged per 1,000 · Emirates ID only, then Wasl</span>
        </p>
      )}

      <ChartFigure
        className="mt-5"
        caption={`Per 1,000 positive FITs, 31-day colonoscopy · ${current.runText}`}
        table={{
          caption: `Measured gap, ${current.runText}`,
          head: ["Measure", "Emirates ID only", "Wasl", "Truth"],
          rows: [
            ...current.rows.map((r) => [`${r.metric} (${r.unit})`, cell(r, r.eid_only), cell(r, r.wasl), cell(r, r.truth)]),
            ...(calibration ? [[`${calibration.short} reference (per 1,000)`, "–", "–", fmt(calibration.per1000)]] : []),
          ],
        }}
      >
        <Legend items={[...legend, ...(calibration ? [{ color: CHROME.deemphasis, label: `${calibration.short} reference`, shape: "tick" as const }] : [])]} />
        <div className="mt-2" data-testid="gap-chart">
          {per1000.map((r) => (
            <DumbbellRow key={r.metric} r={r} x={x1000} show={round} calibration={r === completions ? calibration : null} />
          ))}
          <Axis ticks={ticksTo(max1000)} x={x1000} format={fmt} />
        </div>
        {percent.length > 0 && (
          <div className="mt-4" data-testid="gap-chart-pct">
            {percent.map((r) => (
              <DumbbellRow key={r.metric} r={r} x={xPct} show={(v) => `${num(Math.round(v * 10) / 10)}%`} />
            ))}
            <Axis ticks={ticksTo(100)} x={xPct} format={(v) => `${v}%`} />
          </div>
        )}
        <div className="mt-3 text-small text-muted">{source}</div>
      </ChartFigure>
    </div>
  );
}

/* ---------- Accuracy ---------- */

const V2 = { color: SERIES.wasl, label: "Held-out noise (v2)" };
const V1 = { color: SERIES.tuned, label: "Tuned-on noise (v1)" };

/**
 * The zero-event safety counters as "cases checked, none wrong": per counter a bar for the held-out (v2) cases checked
 * and a thinner one for the tuned-on (v1) run, on one count axis per panel, with any wrong ones as a clay segment at the
 * bar's end. "0 of N" with a check and the 95% upper bound sit under the label; both runs are in the tooltip and table.
 */
export function SafetyChart({ closures, other }: { closures: SafetyPair[]; other: SafetyPair[] }) {
  const bounded = other.filter((p) => (p.v2 ?? p.v1)?.upper_95 != null);
  const countOnly = other.filter((p) => (p.v2 ?? p.v1)?.upper_95 == null);
  const words = (s: EvidenceSafety | null) => (s ? safetyWords(s) : null);
  const checked = (s: EvidenceSafety | null) => s?.denominator ?? 0;
  const panel = (pairs: SafetyPair[], name: (p: SafetyPair) => string) => {
    const max = niceMax(Math.max(...pairs.flatMap((p) => [checked(p.v1), checked(p.v2)])));
    const x = scale(0, max);
    const ticks = ticksTo(max, 2);
    return (
      <>
        {pairs.map((p) => {
          const w2 = words(p.v2);
          const w1 = words(p.v1);
          const lead = p.v2 ?? p.v1;
          return (
            <ChartRow
              key={p.name}
              label={name(p)}
              sub={
                lead &&
                lead.denominator !== null && (
                  <>
                    <ZeroFigure events={lead.events} of={lead.denominator} className="mt-0.5" />
                    {lead.upper_95 != null && <span className="block">≤ {boundPct(lead.upper_95)} at 95%</span>}
                  </>
                )
              }
              labelWidth="12rem"
              track="h-8"
              testId="safety-row"
              tip={
                <TipRows
                  title={p.name}
                  rows={[
                    ...(w2 ? [{ color: V2.color, name: `${V2.label}${w2.bound ? `, ${w2.bound}` : ""}`, value: `${w2.count} wrong` }] : []),
                    ...(w1 ? [{ color: V1.color, name: `${V1.label}${w1.bound ? `, ${w1.bound}` : ""}`, value: `${w1.count} wrong` }] : []),
                  ]}
                />
              }
            >
              <Grid ticks={ticks} x={x} />
              {p.v2 && <CheckedBar to={x(checked(p.v2))} wrong={x(p.v2.events)} color={V2.color} y={36} thick={10} />}
              {p.v1 && <CheckedBar to={x(checked(p.v1))} wrong={x(p.v1.events)} color={V1.color} y={74} thick={4} />}
            </ChartRow>
          );
        })}
        <Axis ticks={ticks} x={x} format={fmt} labelWidth="12rem" title="Cases checked" />
      </>
    );
  };
  const tableRows = [...closures, ...other].map((p) => {
    const w2 = words(p.v2);
    const w1 = words(p.v1);
    return [p.name, w2?.count ?? "–", w2?.bound?.replace("95% upper bound ", "") ?? "–", w1?.count ?? "–", w1?.bound?.replace("95% upper bound ", "") ?? "–"];
  });
  return (
    <ChartFigure
      caption="Cases checked (bar) and how many were wrong; under each count, the 95% upper bound on the true rate. Seeds pooled"
      table={{ caption: "Safety events and 95% upper bounds", head: ["Error", "v2 wrong of checked", "v2 upper bound", "v1 wrong of checked", "v1 upper bound"], rows: tableRows }}
    >
      <Legend items={[{ ...V2, shape: "bar" }, { ...V1, shape: "bar" }]} />
      <div data-testid="safety" className="mt-2">
        {closures.length > 0 && panel(closures, (p) => closureName(p.name))}
        {bounded.length > 0 && <p className="m-0 mt-4 text-small font-medium text-ink">Other safety counters</p>}
        {bounded.length > 0 && panel(bounded, (p) => p.name)}
      </div>
      {countOnly.length > 0 && (
        <ul data-testid="safety-counts" className="m-0 mt-3 flex list-none flex-wrap gap-2 p-0 text-small">
          {countOnly.map((p) => {
            const s = p.v2 ?? p.v1;
            return (
              <li key={p.name} className="inline-flex items-center gap-1 rounded-chip bg-raised px-2 py-1 text-ink-soft">
                {p.name.replace(/\s*\(not zero; see definitions\)/, "")}{" "}
                {s && s.denominator !== null ? <ZeroFigure events={s.events} of={s.denominator} /> : <span className="font-medium text-ink tabular-nums">{words(s)?.count}</span>}
              </li>
            );
          })}
        </ul>
      )}
    </ChartFigure>
  );
}

const showRate = (l: EvidenceLinkage) => withInterval(l);

/** False-match and missed-match rates per programme, v2 and v1 side by side, as small multiples on one % axis. */
export function RateChart({ falseMatch, missed }: { falseMatch: RatePair[]; missed: RatePair[] }) {
  const all = [...falseMatch, ...missed];
  const max = niceMax(Math.max(...all.flatMap((p) => [p.v1?.high ?? 0, p.v2?.high ?? 0])) * 100) / 100;
  const x = scale(0, max);
  const ticks = ticksTo(max, max * 100 > 20 ? 5 : 4);
  const panel = (title: string, rows: RatePair[], id: string) => {
    const top = rows.reduce<RatePair | null>((a, p) => ((p.v2?.value ?? 0) > (a?.v2?.value ?? 0) ? p : a), null);
    return (
      <div className="min-w-0" data-testid={id}>
        <p className="m-0 text-small font-medium text-ink">{title}</p>
        {rows.map((p) => (
          <ChartRow
            key={p.name}
            label={p.name}
            labelWidth="10rem"
            track="h-7"
            testId="rate-row"
            tip={
              <TipRows
                title={`${p.name}: ${title.toLowerCase()}`}
                rows={[
                  ...(p.v2 ? [{ color: V2.color, name: V2.label, value: showRate(p.v2) }] : []),
                  ...(p.v1 ? [{ color: V1.color, name: V1.label, value: showRate(p.v1) }] : []),
                ]}
              />
            }
          >
            <Grid ticks={ticks} x={x} />
            {(
              [
                [p.v2, V2.color, 32],
                [p.v1, V1.color, 72],
              ] as const
            ).map(([l, color, y]) =>
              l ? (
                <span key={y}>
                  <Seg from={x(l.low)} to={x(l.high)} color={color} y={y} />
                  <Dot x={x(l.value)} y={y} color={color} size={8} />
                </span>
              ) : null,
            )}
            {p === top && p.v2 && (
              <Label x={x(p.v2.high)} y={32} strong>
                {sharePct(p.v2.value)}
              </Label>
            )}
          </ChartRow>
        ))}
        <Axis ticks={ticks} x={x} format={(v) => `${Math.round(v * 100)}%`} labelWidth="10rem" />
      </div>
    );
  };
  const table = all.map((p, i) => [
    `${p.name}: ${i < falseMatch.length ? "false-match" : "missed-match"}`,
    p.v2 ? showRate(p.v2) : "–",
    p.v1 ? showRate(p.v1) : "–",
  ]);
  return (
    <ChartFigure
      caption="Mean over 5 seeds (dot) and 95% interval (line), per programme"
      table={{ caption: "Linkage error rates by noise model", head: ["Rate", "v2 held-out", "v1 tuned on"], rows: table }}
    >
      <Legend items={[V2, V1]} />
      <div data-testid="linkage" className="mt-2 grid gap-y-4">
        {panel("False match", falseMatch, "rates-false-match")}
        {panel("Missed match", missed, "rates-missed")}
      </div>
    </ChartFigure>
  );
}

/** Worst and best seed or stratum, as a range per measure: v2 above v1, the worst end labelled. */
export function WorstStrip({ ranges }: { ranges: WorstRange[] }) {
  const names = [...new Set(ranges.map((r) => r.label))];
  const lo = Math.floor(Math.min(...ranges.map((r) => r.low)) * 10) / 10;
  const x = scale(lo, 1);
  const ticks = Array.from({ length: Math.round((1 - lo) * 10) + 1 }, (_, i) => Number((lo + i / 10).toFixed(1)));
  return (
    <ChartFigure
      caption="Recall, worst to best seed or stratum"
      table={{ caption: "Worst cases", head: ["Measure", "v2 held-out", "v1 tuned on"], rows: names.map((n) => [n, ranges.find((r) => r.label === n && r.noise === "v2")?.value ?? "–", ranges.find((r) => r.label === n && r.noise === "v1")?.value ?? "–"]) }}
    >
      <Legend items={[V2, V1]} />
      <div data-testid="worst" className="mt-2">
        {names.map((n) => {
          const v2 = ranges.find((r) => r.label === n && r.noise === "v2");
          const v1 = ranges.find((r) => r.label === n && r.noise === "v1");
          return (
            <ChartRow
              key={n}
              label={n}
              labelWidth="12rem"
              track="h-12"
              testId="worst-row"
              tip={
                <TipRows
                  title={n}
                  rows={[
                    ...(v2 ? [{ color: V2.color, name: "v2", value: v2.value }] : []),
                    ...(v1 ? [{ color: V1.color, name: "v1", value: v1.value }] : []),
                  ]}
                />
              }
            >
              <Grid ticks={ticks} x={x} />
              {(
                [
                  [v2, V2.color, 62],
                  [v1, V1.color, 88],
                ] as const
              ).map(([r, color, y]) =>
                r ? (
                  <span key={y}>
                    <Seg from={x(r.low)} to={x(r.high)} color={color} y={y} />
                    <Dot x={x(r.low)} y={y} color={color} size={8} />
                    <Dot x={x(r.high)} y={y} color={color} size={8} hollow />
                  </span>
                ) : null,
              )}
              {v2 && (
                <Label x={x(v2.low)} y={62} side="above" strong>
                  {v2.low} · {v2.lowWhat}
                </Label>
              )}
            </ChartRow>
          );
        })}
        <Axis ticks={ticks} x={x} format={(v) => v.toFixed(1)} labelWidth="12rem" />
      </div>
    </ChartFigure>
  );
}

/* ---------- Benchmarks ---------- */

const benchName = (b: EvidenceBenchmark) => {
  const n = b.name.replace(new RegExp(`^${b.dataset}\\s*`, "i"), "").replace(/^Arabic-Latin name encoder\s*/i, "");
  return n.charAt(0).toUpperCase() + n.slice(1);
};
const baselineShort = (s: string | null) => (s ? s.split(/[(:]/)[0].trim() : null);
const lowerIsBetter = (b: EvidenceBenchmark) => /false_match|false-match|error/i.test(b.metric);

/** FEBRL4: Wasl and the published baseline as paired bars per measure, on one 0–100% axis. */
export function BenchPairs({ rows }: { rows: EvidenceBenchmark[] }) {
  const x = scale(0, 1);
  const ticks = ticksTo(1);
  return (
    <ChartFigure
      caption={`${rows[0]?.dataset ?? ""}: Wasl against the published baseline`}
      table={{
        caption: "Benchmarks with a baseline",
        head: ["Benchmark", "Wasl", "Baseline"],
        rows: rows.map((b) => [b.name, num(b.wasl), b.baseline === null ? "–" : num(b.baseline)]),
        details: rows.map((b) => [b.baseline_source && `Baseline: ${b.baseline_source}`, b.note].filter(Boolean).join(" · ") || null),
      }}
    >
      <Legend items={[{ color: SERIES.wasl, label: "Wasl", shape: "bar" }, { color: SERIES.comparator, label: "Baseline", shape: "bar" }]} />
      <div data-testid="bench-pairs" className="mt-2">
        {rows.map((b) => (
          <ChartRow
            key={b.name}
            label={benchName(b)}
            sub={[lowerIsBetter(b) ? "lower is better" : null, baselineShort(b.baseline_source) && `vs ${baselineShort(b.baseline_source)}`].filter(Boolean).join(" · ")}
            labelWidth="15rem"
            track="h-8"
            testId="bench-row"
            tip={
              <TipRows
                title={b.name}
                rows={[
                  { color: SERIES.wasl, name: "Wasl", value: sharePct(b.wasl) },
                  ...(b.baseline !== null ? [{ color: SERIES.comparator, name: baselineShort(b.baseline_source) ?? "Baseline", value: sharePct(b.baseline) }] : []),
                ]}
                note={b.note}
              />
            }
          >
            <Grid ticks={ticks} x={x} />
            <Bar to={x(b.wasl)} color={SERIES.wasl} y={30} thick={10} />
            <Label x={x(b.wasl)} y={30} strong>
              {sharePct(b.wasl)}
            </Label>
            {b.baseline !== null && (
              <>
                <Bar to={x(b.baseline)} color={SERIES.comparator} y={72} thick={10} />
                <Label x={x(b.baseline)} y={72}>
                  {sharePct(b.baseline)}
                </Label>
              </>
            )}
          </ChartRow>
        ))}
        <Axis ticks={ticks} x={x} format={(v) => `${Math.round(v * 100)}%`} labelWidth="15rem" />
      </div>
    </ChartFigure>
  );
}

/** Benchmarks with no baseline (the name encoder): one bar per measure on a 0–100% track. */
export function BenchBullets({ rows }: { rows: EvidenceBenchmark[] }) {
  const x = scale(0, 1);
  const ticks = ticksTo(1);
  return (
    <ChartFigure
      caption={`${rows[0]?.dataset ?? ""}: Arabic–Latin name encoder alone, no published baseline`}
      table={{ caption: "Name encoder benchmarks", head: ["Benchmark", "Wasl"], rows: rows.map((b) => [b.name, num(b.wasl)]), details: rows.map((b) => b.note || null) }}
    >
      <div data-testid="bench-bullets">
        {rows.map((b) => (
          <ChartRow
            key={b.name}
            label={benchName(b)}
            labelWidth="15rem"
            track="h-7"
            testId="bench-row"
            tip={<TipRows title={b.name} rows={[{ color: SERIES.wasl, name: b.metric.replace(/_/g, " "), value: sharePct(b.wasl) }]} note={b.note} />}
          >
            <span aria-hidden className="absolute inset-x-0 top-1/2 h-3 -translate-y-1/2 rounded-[4px]" style={{ backgroundColor: CHROME.track }} />
            <Bar to={x(b.wasl)} color={SERIES.wasl} thick={12} />
            <Label x={x(b.wasl)} strong>
              {sharePct(b.wasl)}
            </Label>
          </ChartRow>
        ))}
        <Axis ticks={ticks} x={x} format={(v) => `${Math.round(v * 100)}%`} labelWidth="15rem" />
      </div>
    </ChartFigure>
  );
}

/* ---------- Modelled benefit ---------- */

const MEASURES: { label: string; unit: string; get: (s: EvidenceScenario) => number }[] = [
  { label: "Cancers found per 1,000", unit: "", get: (s) => s.cancers_found_per_1000 },
  { label: "Extra colonoscopies per 1,000", unit: "", get: (s) => s.extra_colonoscopies_per_1000 },
  { label: "Uplift in completion", unit: " pp", get: (s) => s.uplift_pp },
  { label: "CRC found per colonoscopy", unit: "%", get: (s) => s.ppv * 100 },
];

/** Worst, base and best case per measure, each row on its own scale from 0: a range with the base case marked. */
export function ScenarioRanges({ scenarios }: { scenarios: EvidenceScenario[] }) {
  const by = (n: EvidenceScenario["name"]) => scenarios.find((s) => s.name === n);
  const [worst, base, best] = [by("worst"), by("base"), by("best")];
  return (
    <ChartFigure
      caption="Worst to best case (line), base case (dot) · each row on its own scale from 0"
      table={{
        caption: "Modelled scenarios per 1,000 positive FITs",
        head: ["Measure", "Worst", "Base", "Best"],
        rows: MEASURES.map((m) => [m.label, ...[worst, base, best].map((s) => (s ? `${num(m.get(s))}${m.unit}` : "–"))]),
      }}
    >
      <div data-testid="scenarios">
        {MEASURES.map((m) => {
          const vals = [worst, base, best].map((s) => (s ? m.get(s) : null));
          const max = niceMax(Math.max(...vals.map((v) => v ?? 0)));
          const x = scale(0, max);
          const [w, b, t] = vals;
          const show = (v: number) => `${num(v)}${m.unit}`;
          return (
            <ChartRow
              key={m.label}
              label={m.label}
              labelWidth="13rem"
              track="h-11"
              testId="scenario-row"
              tip={
                <TipRows
                  title={m.label}
                  rows={(
                    [
                      ["Worst", w],
                      ["Base", b],
                      ["Best", t],
                    ] as const
                  ).flatMap(([n, v]) => (v === null ? [] : [{ color: SERIES.wasl, name: n, value: show(v) }]))}
                />
              }
            >
              <Grid ticks={[0]} x={x} />
              {w !== null && t !== null && <Seg from={x(w)} to={x(t)} color={CHROME.deemphasis} y={68} thick={4} />}
              {w !== null && <Dot x={x(w)} y={68} color={SERIES.wasl} hollow />}
              {t !== null && <Dot x={x(t)} y={68} color={SERIES.wasl} hollow />}
              {b !== null && <Dot x={x(b)} y={68} color={SERIES.wasl} />}
              {w !== null && (
                <Label x={x(w)} y={68} side="left">
                  {show(w)}
                </Label>
              )}
              {t !== null && (
                <Label x={x(t)} y={68} side="right" strong>
                  {show(t)}
                </Label>
              )}
              {b !== null && (
                <Label x={x(b)} y={68} side="above">
                  base {show(b)}
                </Label>
              )}
            </ChartRow>
          );
        })}
      </div>
    </ChartFigure>
  );
}

/** One-way sensitivity: each input moved from its low to its high value, widest swing first, base case marked. */
export function TornadoChart({ rows, output, base }: { rows: EvidenceTornadoRow[]; output: string; base: number | null }) {
  const { bars, min, max } = tornado(rows);
  const x = scale(min, max);
  const ticks = ticksTo(max);
  return (
    <ChartFigure
      caption={`${output} as each input moves low → high, the rest at base`}
      table={{
        caption: `Sensitivity of ${output}`,
        head: ["Input", "Low", "High"],
        rows: bars.map((b) => [b.input, `${b.low}: ${num(b.out_low)}`, `${b.high}: ${num(b.out_high)}`]),
      }}
    >
      {base !== null && <Legend items={[{ color: SERIES.wasl, label: "Swing", shape: "bar" }, { color: CHROME.reference, label: `Base case ${num(base)}`, shape: "tick" }]} />}
      <div data-testid="tornado" className="mt-2">
        {bars.map((b) => (
          <ChartRow
            key={b.input}
            label={b.input}
            labelWidth="13rem"
            track="h-8"
            testId="tornado-row"
            tip={
              <TipRows
                title={b.input}
                rows={[
                  { color: SERIES.wasl, name: b.low, value: num(b.out_low) },
                  { color: SERIES.wasl, name: b.high, value: num(b.out_high) },
                ]}
              />
            }
          >
            <Grid ticks={ticks} x={x} />
            <span
              aria-hidden
              className="absolute top-1/2 h-3 -translate-y-1/2 rounded-[4px]"
              style={{ left: `${x(b.lo)}%`, width: `${x(b.hi) - x(b.lo)}%`, backgroundColor: SERIES.wasl }}
            />
            {base !== null && <Tick x={x(base)} color={CHROME.reference} h={22} />}
            <Label x={x(b.lo)} side="left">
              {b.out_low <= b.out_high ? b.low : b.high}
            </Label>
            <Label x={x(b.hi)} strong>
              {b.out_low <= b.out_high ? b.high : b.low} → {num(b.hi)}
            </Label>
          </ChartRow>
        ))}
        <Axis ticks={ticks} x={x} format={(v) => num(v)} labelWidth="13rem" />
      </div>
    </ChartFigure>
  );
}
