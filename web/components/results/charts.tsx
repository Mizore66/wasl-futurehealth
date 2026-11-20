"use client";

import { Axis, Bar, ChartRow, CheckMark, CheckedBar, Dot, Grid, Label, Legend, Seg, Tick, TipRows, ZeroFigure, niceMax, scale, ticksTo } from "@/components/charts/kit";
import { CHROME, SERIES } from "@/components/charts/palette";
import { ChartFigure } from "@/components/programme/chart-kit";

/*
 * The Results page's charts. The page reads every number from results.json or the dashboard payload and passes it
 * in; nothing here states a number of its own.
 */

const fmt = (n: number) => n.toLocaleString("en-US");
const pctText = (share: number, digits = 2) => `${Number((share * 100).toFixed(digits))}%`;

export type Bound = { label: string; events: number; denominator: number; upper: number; testId?: string };

/**
 * Zero-event counters as "cases checked, none wrong": a bar per counter for the cases checked on one count axis, a
 * clay segment at its end for any that were wrong (none here), and "0 of N" with a check beside the label. The 95%
 * upper bound on the true rate stays under each count, in the tooltip and in the table.
 */
export function BoundChart({ rows }: { rows: Bound[] }) {
  const max = niceMax(Math.max(...rows.map((r) => r.denominator)));
  const x = scale(0, max);
  const ticks = ticksTo(max, 2);
  return (
    <ChartFigure
      caption="Cases checked (bar) and how many were wrong; under each count, the 95% upper bound on the true rate. 5 seeds pooled"
      table={{
        caption: "Safety counters with 95% upper bounds",
        head: ["Counter", "Wrong of checked", "95% upper bound"],
        rows: rows.map((r) => [r.label, `${r.events} of ${fmt(r.denominator)}`, pctText(r.upper)]),
      }}
    >
      <div data-testid="bound-chart">
        {rows.map((r) => (
          <ChartRow
            key={r.label}
            label={r.label}
            sub={
              <>
                <ZeroFigure events={r.events} of={r.denominator} className="mt-0.5" />
                <span className="block">≤ {pctText(r.upper)} at 95%</span>
              </>
            }
            labelWidth="11rem"
            track="h-7"
            testId={r.testId}
            tip={
              <TipRows
                title={r.label}
                rows={[
                  { color: SERIES.wasl, name: "checked", value: fmt(r.denominator) },
                  { color: "var(--color-negative)", name: "wrong", value: String(r.events) },
                ]}
                note={`95% upper bound on the true rate: ${pctText(r.upper)}`}
              />
            }
          >
            <Grid ticks={ticks} x={x} />
            <CheckedBar to={x(r.denominator)} wrong={x(r.events)} color={SERIES.wasl} />
          </ChartRow>
        ))}
        <Axis ticks={ticks} x={x} format={fmt} labelWidth="11rem" title="Cases checked" />
      </div>
    </ChartFigure>
  );
}

/** One estimate with its 95% interval on a share axis (CRC closure recall). */
export function IntervalChart({ label, value, low, high, from = 0.5 }: { label: string; value: number; low: number; high: number; from?: number }) {
  const x = scale(from, 1);
  const ticks = [from, (from + 1) / 2, 1].map((t) => Number(t.toFixed(2)));
  const said = `${value.toFixed(3)} (${low.toFixed(3)}–${high.toFixed(3)})`;
  return (
    <ChartFigure caption="Mean over 5 seeds (dot) and 95% interval (line)" table={{ caption: label, head: ["Measure", "Mean", "95% interval"], rows: [[label, value.toFixed(3), `${low.toFixed(3)}–${high.toFixed(3)}`]] }}>
      <div data-testid="recall-chart">
        <ChartRow label={label} labelWidth="11rem" track="h-7" tip={<TipRows title={label} rows={[{ color: SERIES.wasl, name: "mean, 95% interval", value: said }]} />}>
          <Grid ticks={ticks} x={x} />
          <Seg from={x(low)} to={x(high)} color={SERIES.wasl} />
          <Dot x={x(value)} color={SERIES.wasl} />
          <Label x={x(high)} strong>
            {value.toFixed(3)}
          </Label>
        </ChartRow>
        <Axis ticks={ticks} x={x} format={(v) => v.toFixed(2)} labelWidth="11rem" />
      </div>
    </ChartFigure>
  );
}

export type Kpi = { label: string; eid: number; wasl: number; truth: number; eidText: string; waslText: string; truthText: string; eidCi?: [number, number]; waslCi?: [number, number] };

/** 31-day completion: Emirates-ID-only and Wasl bars (with 95% whiskers where known) against the truth tick, one group per run. */
export function KpiChart({ groups }: { groups: Kpi[] }) {
  const x = scale(0, 0.4);
  const ticks = ticksTo(0.4, 2);
  return (
    <ChartFigure
      caption="Referrals with a colonoscopy within 31 days"
      table={{
        caption: "31-day completion by method",
        head: ["Run", "Emirates ID only", "Wasl", "Truth"],
        rows: groups.map((g) => [g.label, g.eidText, g.waslText, g.truthText]),
      }}
    >
      <Legend
        items={[
          { color: SERIES.comparator, label: "Emirates ID only", shape: "bar" },
          { color: SERIES.wasl, label: "Wasl", shape: "bar" },
          { color: CHROME.reference, label: "Synthetic truth", shape: "tick" },
        ]}
      />
      <div data-testid="kpi-chart" className="mt-2">
        {groups.map((g) => (
          <ChartRow
            key={g.label}
            label={g.label}
            labelWidth="9rem"
            track="h-9"
            testId="kpi-row"
            tip={
              <TipRows
                title={g.label}
                rows={[
                  { color: SERIES.wasl, name: "Wasl", value: g.waslText },
                  { color: SERIES.comparator, name: "Emirates ID only", value: g.eidText },
                  { color: CHROME.reference, name: "Truth", value: g.truthText },
                ]}
              />
            }
          >
            <Grid ticks={ticks} x={x} />
            <Bar to={x(g.wasl)} color={SERIES.wasl} y={30} thick={10} />
            <Bar to={x(g.eid)} color={SERIES.comparator} y={72} thick={10} />
            {g.waslCi && <Seg from={x(g.waslCi[0])} to={x(g.waslCi[1])} color={CHROME.reference} y={30} thick={1} />}
            {g.eidCi && <Seg from={x(g.eidCi[0])} to={x(g.eidCi[1])} color={CHROME.reference} y={72} thick={1} />}
            <Tick x={x(g.truth)} color={CHROME.reference} h={34} />
            <Label x={Math.max(x(g.wasl), x(g.truth))} y={30} strong>
              {g.waslText}
            </Label>
            <Label x={x(g.eid)} y={72}>
              {g.eidText}
            </Label>
          </ChartRow>
        ))}
        <Axis ticks={ticks} x={x} format={(v) => `${Math.round(v * 100)}%`} labelWidth="9rem" />
      </div>
    </ChartFigure>
  );
}

/** A count out of a whole as a bar (meter list): the bar is the share, the figure is the count. */
export type CountRow = { label: string; count: number; of?: number; testId?: string; color?: string; sub?: string; /** The definition: in the tooltip and the table, never under the label. */ what?: string };

/**
 * `checked`: every row is a zero-event counter out of a whole (consent guards). The bar is then the whole, in the Wasl
 * colour, with any rule-breaking ones as a clay segment at its end; the figure reads "0 of 1,891" with a check.
 */
export function CountBars({
  rows,
  caption,
  tableHead,
  axisMax,
  checked = false,
}: {
  rows: CountRow[];
  caption: string;
  tableHead: [string, string];
  axisMax?: number;
  checked?: boolean;
}) {
  const shares = rows.every((r) => r.of);
  const max = axisMax ?? (shares ? 1 : Math.max(...rows.map((r) => r.count)));
  const x = scale(0, max);
  const said = (r: CountRow) => (r.of ? `${fmt(r.count)} of ${fmt(r.of)}` : fmt(r.count));
  const defined = rows.some((r) => r.what);
  return (
    <ChartFigure
      caption={caption}
      table={{ caption, head: defined ? [...tableHead, "Meaning"] : tableHead, rows: rows.map((r) => (defined ? [r.label, said(r), r.what ?? ""] : [r.label, said(r)])) }}
    >
      <div>
        {rows.map((r) => (
          <ChartRow
            key={r.label}
            label={r.label}
            sub={checked && r.of ? <ZeroFigure events={r.count} of={r.of} className="mt-0.5" /> : r.sub}
            labelWidth="10rem"
            track="h-7"
            testId={r.testId}
            tip={<TipRows title={r.label} rows={[{ color: r.color ?? SERIES.wasl, name: r.of ? "" : "count", value: said(r) }]} note={r.what} />}
          >
            {checked && r.of ? (
              <CheckedBar to={100} wrong={x(r.count / r.of)} color={SERIES.wasl} />
            ) : (
              <>
                <span aria-hidden className="absolute inset-x-0 top-1/2 h-3 -translate-y-1/2 rounded-[4px]" style={{ backgroundColor: CHROME.track }} />
                {r.count > 0 && <Bar to={x(r.of ? r.count / r.of : r.count)} color={r.color ?? SERIES.wasl} thick={12} />}
                <Label x={x(r.of ? r.count / r.of : r.count)} strong>
                  <span className="inline-flex items-center gap-1">
                    {r.count === 0 && <CheckMark className="text-positive" />}
                    {said(r)}
                  </span>
                </Label>
              </>
            )}
          </ChartRow>
        ))}
      </div>
    </ChartFigure>
  );
}

/** Unit chart: one square per corpus, the ones that found an unsafe phrasing filled. */
export function UnitChart({ total, hit, label }: { total: number; hit: number; label: string }) {
  return (
    <ChartFigure
      caption={label}
      table={{ caption: label, head: ["Corpora", "Count"], rows: [["Found an unsafe phrasing", String(hit)], ["Passed", String(total - hit)]] }}
    >
      <div data-testid="unit-chart" role="img" aria-label={`${hit} of ${total} corpora found an unsafe phrasing`} className="flex flex-wrap gap-1.5">
        {Array.from({ length: total }, (_, i) => (
          <span
            key={i}
            className="block size-6 rounded-[4px]"
            style={i < hit ? { backgroundColor: SERIES.comparator } : { backgroundColor: CHROME.surface, boxShadow: `inset 0 0 0 2px ${SERIES.wasl}` }}
          />
        ))}
      </div>
      <Legend
        className="mt-2"
        items={[
          { color: SERIES.comparator, label: "Found an unsafe phrasing, fixed by a general rule", shape: "bar" },
          { color: SERIES.wasl, label: "Passed", shape: "bar" },
        ]}
      />
    </ChartFigure>
  );
}
