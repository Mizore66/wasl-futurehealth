"use client";

import { useState } from "react";
import { ChartFigure, FILL, Legend, Mark, Segmented, type FillName } from "./chart-kit";
import { pct, points, type KpiSet } from "./model";

type Source = "demo" | "heldout";
type Arm = "eid" | "wasl" | "truth";

const ARMS: { arm: Arm; name: string; fill: FillName }[] = [
  { arm: "eid", name: "Reported, Emirates ID only", fill: "muted" },
  { arm: "wasl", name: "Measured by Wasl", fill: "solid" },
  { arm: "truth", name: "True rate", fill: "outline" },
];

const SOURCES: Record<Source, { button: string; table: string; scope: string }> = {
  demo: { button: "This demo run", table: "This demo run", scope: "in testing" },
  heldout: { button: "Five-seed held-out mean", table: "Five-seed held-out mean", scope: "on held-out noise, mean of 5 seeds" },
};

/** The 31-day rate: reported, Wasl and true against the target, for one demo run or the five-seed held-out mean. */
export function KpiChart({ kpi, target }: { kpi: Record<Source, KpiSet>; target: number | null }) {
  const [source, setSource] = useState<Source>("demo");
  const set = kpi[source];
  const { eid, wasl, truth } = set.rates;
  const label = `On a 0 to 100% scale: reported ${pct(eid)}, measured by Wasl ${pct(wasl)}, true ${pct(truth)}${target === null ? "" : `, target ${target}%`}.`;

  return (
    <>
      <div className="mt-5">
        <Segmented
          label="31-day rate source"
          value={source}
          onChange={setSource}
          options={[
            { value: "demo", label: SOURCES.demo.button },
            { value: "heldout", label: SOURCES.heldout.button },
          ]}
        />
      </div>

      <div className="mt-5 flex flex-wrap items-end gap-x-12 gap-y-3 tabular-nums">
        <div data-testid="kpi-eid">
          <div className="text-small text-muted">Reported, Emirates ID only</div>
          <div className="mt-1 text-lead font-medium tracking-[-0.02em] text-muted">{pct(eid)}</div>
        </div>
        <div data-testid="kpi-wasl">
          <div className="text-small text-muted">Measured by Wasl</div>
          <div className="mt-1 text-lead font-bold tracking-[-0.02em] text-hue">{pct(wasl)}</div>
        </div>
      </div>

      <ChartFigure
        className="mt-6"
        caption="Colonoscopy within 31 days of a positive FIT, three ways, on a 0 to 100% scale. Hover or focus a bar for detail."
        table={{
          caption: `31-day rate, ${SOURCES[source].table}`,
          head: ["Measure", "Rate", "Detail"],
          rows: ARMS.map(({ arm, name }) => [name, pct(set.rates[arm]), set.detail[arm]]),
        }}
      >
        <div role="group" aria-label={label} className="relative pt-6">
          {target !== null && (
            <>
              <span
                aria-hidden
                className="absolute top-0 -translate-x-full whitespace-nowrap pr-1.5 text-small leading-4 text-ink"
                style={{ left: `${target}%` }}
              >
                Target {target}%
              </span>
              <span aria-hidden className="absolute bottom-0 top-5 w-0 -translate-x-px border-l-2 border-dashed border-ink" style={{ left: `${target}%` }} />
            </>
          )}
          <ul className="m-0 list-none p-0">
            {ARMS.map(({ arm, name, fill }) => {
              const v = set.rates[arm];
              return (
                <li key={arm}>
                  <Mark
                    tip={
                      <>
                        <strong className="font-medium">{name}: {pct(v)}</strong>
                        <br />
                        {set.detail[arm]}
                      </>
                    }
                    className="flex flex-col justify-center py-1"
                  >
                    <span className="text-small leading-4 text-ink-soft">{name}</span>
                    <span className="relative mt-1 block h-4">
                      <span
                        className="absolute left-0 top-0 block h-4 rounded-[4px] motion-safe:transition-[width] motion-safe:duration-300"
                        style={{ ...FILL[fill], width: `${v}%`, minWidth: 4 }}
                      />
                      <span className="absolute top-0 text-small font-medium leading-4" style={{ left: `calc(${v}% + 6px)` }}>
                        {pct(v)}
                      </span>
                    </span>
                  </Mark>
                </li>
              );
            })}
          </ul>
        </div>
        <div className="mt-2">
          <Legend items={ARMS.map(({ name, fill }) => ({ fill, label: name }))} />
        </div>
      </ChartFigure>

      <p data-testid="kpi-truth" className="m-0 mt-3 text-small text-muted">
        Within {points(truth, wasl)} of the true rate ({pct(truth)}) {SOURCES[source].scope}
      </p>
      <p data-testid="kpi-why" className="m-0 mt-2 text-small leading-[18px] text-ink-soft">
        Why the two differ: the demo run is one run on training-time noise; the five-seed mean averages 5 seeds on held-out noise.
      </p>
    </>
  );
}
