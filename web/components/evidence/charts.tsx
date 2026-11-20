"use client";

import { ChartFigure, FILL, type FillName } from "@/components/programme/chart-kit";
import type { EvidencePer1000Row, EvidenceTornadoRow } from "@/lib/api/evidence-schema";
import { fmt, niceMax, num, tornado } from "./model";

const at = (v: number, min: number, max: number) => `${((v - min) / (max - min)) * 100}%`;

function Ticks({ min, max }: { min: number; max: number }) {
  const ticks = [min, (min + max) / 2, max];
  return (
    <div data-axis aria-hidden className="relative mt-1 h-4 text-small text-muted">
      {ticks.map((t, i) => (
        <span
          key={i}
          className={i === 0 ? "absolute left-0" : i === ticks.length - 1 ? "absolute right-0" : "absolute -translate-x-1/2"}
          style={i === 1 ? { left: at(t, min, max) } : undefined}
        >
          {num(t)}
        </span>
      ))}
    </div>
  );
}

const SERIES: { key: "eid_only" | "wasl" | "truth"; label: string; fill: FillName }[] = [
  { key: "eid_only", label: "Emirates ID only", fill: "muted" },
  { key: "wasl", label: "Wasl", fill: "solid" },
  { key: "truth", label: "Simulated truth", fill: "ink" },
];

/** Completions seen per 1,000 positive FITs, three bars, with the published Abu Dhabi rate as a dashed reference. */
export function GapChart({
  row,
  calibration,
}: {
  row: EvidencePer1000Row;
  calibration: { per1000: number; short: string } | null;
}) {
  const values = SERIES.map((s) => ({ ...s, value: row[s.key] ?? 0 }));
  const max = niceMax(Math.max(...values.map((v) => v.value), calibration?.per1000 ?? 0));
  const said = values.map((v) => `${v.label} ${fmt(v.value)}`).join(", ");
  return (
    <ChartFigure
      caption={`${row.row}, ${row.unit} positive FITs. Synthetic cohort.`}
      table={{
        caption: `${row.row}, ${row.unit} positive FITs`,
        head: ["Measured by", row.unit],
        rows: [...values.map((v) => [v.label, fmt(v.value)]), ...(calibration ? [[calibration.short, fmt(calibration.per1000)]] : [])],
      }}
    >
      <div
        role="img"
        aria-label={`${row.row} per 1,000 positive FITs: ${said}.${calibration ? ` ${calibration.short}: ${fmt(calibration.per1000)}.` : ""}`}
      >
        <div className="relative">
          {calibration && (
            <>
              <span
                className="absolute top-0 -translate-x-1/2 whitespace-nowrap text-small font-medium text-ochre-ink max-sm:translate-x-[-80%]"
                style={{ left: at(calibration.per1000, 0, max) }}
              >
                {calibration.short} = {fmt(calibration.per1000)}
              </span>
              <div
                className="absolute bottom-0 top-5 w-0 border-l-2 border-dashed border-ochre-ink"
                style={{ left: at(calibration.per1000, 0, max) }}
              />
            </>
          )}
          <div className={calibration ? "pt-6" : ""}>
            {values.map((v) => (
              <div key={v.key} className="py-1.5">
                <span className="block text-small text-ink-soft">
                  {v.label} <span className="font-medium text-ink">{fmt(v.value)}</span>
                </span>
                <span className="mt-1 block h-4 rounded-[2px]" style={{ ...FILL[v.fill], width: at(v.value, 0, max) }} />
              </div>
            ))}
          </div>
        </div>
        <Ticks min={0} max={max} />
      </div>
    </ChartFigure>
  );
}

/** One-way sensitivity: each input moved from its low to its high value, widest swing first. */
export function TornadoChart({ rows, output }: { rows: EvidenceTornadoRow[]; output: string }) {
  const { bars, min, max } = tornado(rows);
  return (
    <ChartFigure
      caption={`${output}, as each input moves from low to high. The others stay at the base case.`}
      table={{
        caption: `Sensitivity of ${output}`,
        head: ["Input", "Low", "High"],
        rows: bars.map((b) => [b.input, `${b.low}: ${num(b.out_low)}`, `${b.high}: ${num(b.out_high)}`]),
      }}
    >
      <div
        role="img"
        aria-label={`Sensitivity of ${output}: ${bars.map((b) => `${b.input} ${b.low} gives ${num(b.out_low)}, ${b.high} gives ${num(b.out_high)}`).join("; ")}.`}
      >
        {bars.map((b) => (
          <div key={b.input} className="py-1.5">
            <span className="block text-small text-ink-soft">{b.input}</span>
            <div className="relative mt-1 h-4 rounded-[2px] bg-raised">
              <span
                className="absolute inset-y-0 rounded-[2px]"
                style={{ ...FILL.solid, left: at(b.lo, min, max), width: `calc(${at(b.hi, min, max)} - ${at(b.lo, min, max)})` }}
              />
            </div>
            <span className="mt-0.5 flex justify-between gap-2 text-small text-muted">
              <span>
                {b.low}: <span className="font-medium text-ink">{num(b.out_low)}</span>
              </span>
              <span>
                {b.high}: <span className="font-medium text-ink">{num(b.out_high)}</span>
              </span>
            </span>
          </div>
        ))}
        <Ticks min={min} max={max} />
      </div>
    </ChartFigure>
  );
}
