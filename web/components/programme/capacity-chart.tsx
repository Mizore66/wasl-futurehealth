"use client";

import { ChartFigure, FILL, Legend, Mark } from "./chart-kit";
import type { CapacityRegion, PoolRun } from "./model";

const n = (v: number) => v.toLocaleString("en-US");
const w = (v: number, max: number) => ({ width: `${(v / max) * 100}%`, minWidth: v > 0 ? 4 : 0 });

/** Slots against open cases that need one, in each region: a grouped bar chart. */
export function RegionCapacityChart({ regions }: { regions: CapacityRegion[] }) {
  const max = Math.max(1, ...regions.flatMap((r) => [r.slots, r.cases]));
  return (
    <ChartFigure
      caption="Endoscopy slots against open cases that need one, by region. Both bars share one scale that starts at zero."
      table={{ caption: "Slots and open cases by region", head: ["Region", "Slots", "Open cases"], rows: regions.map((r) => [r.name, n(r.slots), n(r.cases)]) }}
    >
      <ul className="m-0 list-none p-0">
        {regions.map((r) => (
          <li key={r.key} className="border-b border-rule py-2 first:pt-0 last:border-b-0">
            <div className="text-body font-medium">{r.name}</div>
            {(
              [
                ["Slots", r.slots, "solid"],
                ["Open cases", r.cases, "ink"],
              ] as const
            ).map(([name, v, fill]) => (
              <Mark
                key={name}
                className="flex flex-col justify-center"
                tip={<><strong className="font-medium">{r.name}</strong>: {n(r.slots)} slots for {n(r.cases)} open cases.</>}
              >
                <span className="text-small leading-4 text-ink-soft">{name}</span>
                <span className="relative mt-0.5 block h-4">
                  <span className="absolute left-0 top-0 block h-4 rounded-[4px]" style={{ ...FILL[fill], ...w(v, max) }} />
                  <span className="absolute top-0 text-small font-medium leading-4" style={{ left: `calc(${(v / max) * 100}% + 6px)` }}>
                    {n(v)}
                  </span>
                </span>
              </Mark>
            ))}
          </li>
        ))}
      </ul>
      <div className="mt-2">
        <Legend
          items={[
            { fill: "solid", label: "Slots" },
            { fill: "ink", label: "Open cases needing a slot" },
          ]}
        />
      </div>
    </ChartFigure>
  );
}

/** The measured pooling test: share within 31 days with and without cross-site booking. */
export function PoolingChart({ runs }: { runs: PoolRun[] }) {
  const max = 50;
  return (
    <ChartFigure
      caption="Simulated booking test with capacity in the wrong regions: share of all FIT+ patients with a colonoscopy within 31 days. Bars start at zero."
      table={{
        caption: "Share within 31 days, by scenario",
        head: ["Scenario", "Slots per FIT+", "Within 31 days"],
        rows: runs.flatMap((run) => run.rows.map((r) => [`${r.label} (${run.slotsPerCase} slots)`, run.slotsPerCase, r.display])),
      }}
    >
      {runs.map((run) => (
        <div key={run.slotsPerCase} className="mt-3 first:mt-0">
          <h3 className="m-0 text-small font-medium text-muted">
            {run.slotsPerCase} slots per FIT+ {run.enough ? "(enough in total)" : "(short overall)"}
          </h3>
          <ul className="m-0 list-none p-0">
            {run.rows.map((r) => {
              const pooled = r.id.endsWith("skewed_pool_60");
              return (
                <li key={r.id}>
                  <Mark
                    className="flex flex-col justify-center"
                    tip={<><strong className="font-medium">{r.label}</strong>: {r.display} within 31 days at {run.slotsPerCase} slots per FIT+.</>}
                  >
                    <span className={pooled ? "text-small font-medium leading-4" : "text-small leading-4 text-ink-soft"}>{r.label}</span>
                    <span className="relative mt-0.5 block h-3.5">
                      <span className="absolute left-0 top-0 block h-3.5 rounded-[4px]" style={{ ...FILL[pooled ? "solid" : "muted"], ...w(r.share, max) }} />
                      <span className="absolute top-0 text-small font-medium leading-3.5" style={{ left: `calc(${(r.share / max) * 100}% + 6px)` }}>
                        {r.display}
                      </span>
                    </span>
                  </Mark>
                </li>
              );
            })}
          </ul>
        </div>
      ))}
      <div className="mt-2">
        <Legend
          items={[
            { fill: "solid", label: "With network pooling" },
            { fill: "muted", label: "Other scenarios" },
          ]}
        />
      </div>
    </ChartFigure>
  );
}
