"use client";

import { useState } from "react";
import { SegmentedToggle } from "@/components/ui/segmented-toggle";
import { ChartFigure, Mark } from "./chart-kit";
import { pct, points, type KpiSet } from "./model";

type Source = "demo" | "heldout";
type Arm = "eid" | "wasl" | "truth";

/** One row per method. Every row is labelled on the chart, so the fills need no legend. */
const ARMS: { arm: Arm; name: string; fill: string; emphasis?: boolean }[] = [
  { arm: "eid", name: "Emirates ID only", fill: "bg-jade-300" },
  { arm: "wasl", name: "Wasl", fill: "bg-jade-700", emphasis: true },
  { arm: "truth", name: "Simulated truth", fill: "bg-jade-500" },
];

const TICKS = [0, 25, 50, 75, 100];

const SOURCES: Record<Source, { button: string; table: string; scope: string }> = {
  demo: { button: "This demo run", table: "This demo run", scope: "in testing" },
  heldout: { button: "Five-seed held-out mean", table: "Five-seed held-out mean", scope: "on held-out noise, mean of 5 seeds" },
};

/** Label, track, value: the three columns every bar row and the axis share. */
const COLS = "grid grid-cols-[minmax(87px,.8fr)_minmax(0,2fr)_minmax(42px,.38fr)] gap-[7px] @min-[701px]:grid-cols-[minmax(110px,.9fr)_minmax(0,4fr)_minmax(47px,.35fr)] @min-[701px]:gap-3";

/**
 * The target rail: the 31-day rate by each method on a 0 to 100% scale against the one target line,
 * for one demo run or the five-seed held-out mean.
 */
export function KpiChart({ kpi, target }: { kpi: Record<Source, KpiSet>; target: number | null }) {
  const [source, setSource] = useState<Source>("demo");
  const set = kpi[source];
  const { wasl, truth } = set.rates;
  const label = `On a 0 to 100% scale: ${ARMS.map(({ arm, name }) => `${arm === "truth" ? name.toLowerCase() : name} ${pct(set.rates[arm])}`).join(", ")}${
    target === null ? "" : `, target ${target}%`
  }.`;

  return (
    <>
      <ChartFigure
        captionBelow
        caption={
          <>
            Colonoscopy within 31 days of a positive FIT, by method, on a 0 to 100% scale. Hover or focus a bar for detail.{" "}
            <span data-testid="kpi-why">
              Why the two sources differ: the demo run is one run on training-time noise; the five-seed mean averages 5 seeds on held-out noise.
            </span>
          </>
        }
        table={{
          caption: `31-day rate, ${SOURCES[source].table}`,
          head: ["Method", "Rate", "Detail"],
          rows: [...ARMS.map(({ arm, name }) => [name, pct(set.rates[arm]), set.detail[arm]]), ...(target === null ? [] : [["Target", `${target}%`, "DoH 31-day standard"]])],
        }}
      >
        <div role="group" aria-label={label} className={`relative ${COLS} mt-8 @min-[701px]:mt-[clamp(31px,4cqi,53px)]`}>
          {target !== null && (
            <div aria-hidden className="pointer-events-none relative z-10 col-start-2 row-start-1">
              <div data-testid="kpi-target" className="absolute inset-y-0 border-l border-dashed border-jade-500" style={{ left: `${target}%` }}>
                <span className="absolute right-[5px] top-[-22px] whitespace-nowrap text-small font-semibold leading-[1.4] text-jade-800">{target}% target</span>
              </div>
            </div>
          )}
          <ul className="col-span-full row-start-1 m-0 grid list-none p-0">
            {ARMS.map(({ arm, name, fill, emphasis }) => {
              const v = set.rates[arm];
              return (
                <li key={arm} data-method={arm} className="border-b border-rule last:border-b-0">
                  <Mark
                    tip={
                      <>
                        <strong className="font-medium">
                          {name}: {pct(v)}
                        </strong>
                        <br />
                        {set.detail[arm]}
                      </>
                    }
                    className={`${COLS} min-h-14 items-center @min-[701px]:min-h-[clamp(56px,6.4vw,100px)]`}
                  >
                    <span className={`text-[12px] leading-[1.35] @min-[701px]:text-body ${emphasis ? "font-bold text-jade-800" : "font-medium text-limestone-800"}`}>{name}</span>
                    <span aria-hidden className="relative block h-[13px] min-w-0 rounded-[3px] bg-limestone-100 @min-[701px]:h-4">
                      <span className={`block h-full rounded-[3px] ${fill} motion-safe:transition-[width] motion-safe:duration-300`} style={{ width: `${v}%`, minWidth: 4 }} />
                    </span>
                    <span data-testid={arm === "truth" ? "kpi-true" : `kpi-${arm}`} className={`text-right text-[12px] font-bold tabular-nums @min-[701px]:text-body ${emphasis ? "text-jade-800" : "text-ink"}`}>
                      {pct(v)}
                    </span>
                  </Mark>
                </li>
              );
            })}
          </ul>
        </div>
        <div aria-hidden className={`${COLS} mt-[7px]`}>
          <div className="relative col-start-2 min-h-4 text-[10px] text-muted @min-[701px]:text-[12px]">
            {TICKS.map((t, i) => (
              <span
                key={t}
                className={`absolute tabular-nums ${i === 0 ? "" : i === TICKS.length - 1 ? "-translate-x-full" : "-translate-x-1/2"} ${i % 2 === 1 ? "hidden @min-[701px]:inline" : ""}`}
                style={{ left: `${t}%` }}
              >
                {t}%
              </span>
            ))}
          </div>
        </div>
      </ChartFigure>

      <div className="mt-4 flex flex-wrap items-center gap-x-4 gap-y-2">
        <SegmentedToggle
          label="31-day rate source"
          value={source}
          onChange={setSource}
          items={[
            { value: "demo", label: SOURCES.demo.button },
            { value: "heldout", label: SOURCES.heldout.button },
          ]}
        />
        <p data-testid="kpi-truth" className="m-0 text-small text-muted">
          Wasl is within {points(truth, wasl)} of the simulated truth ({pct(truth)}) {SOURCES[source].scope}.
        </p>
      </div>
    </>
  );
}
