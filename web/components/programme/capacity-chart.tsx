"use client";

import { CATEGORICAL, CHROME } from "@/components/charts/palette";
import { cn } from "@/lib/utils";
import { ChartFigure, Mark } from "./chart-kit";
import type { CapacityRegion, PoolRun } from "./model";

const n = (v: number) => v.toLocaleString("en-US");
const w = (v: number, max: number) => ({ width: `${(v / max) * 80}%`, minWidth: v > 0 ? 4 : 0 });

const SLOTS = CATEGORICAL[0];
const OPEN = CATEGORICAL[1];

function Key({ items }: { items: { color: string; label: string }[] }) {
  return (
    <ul aria-hidden className="m-0 flex list-none flex-wrap gap-x-4 gap-y-1 p-0 text-small text-ink-soft">
      {items.map((i) => (
        <li key={i.label} className="flex items-center gap-1.5">
          <span className="block size-2.5 flex-none rounded-[2px]" style={{ backgroundColor: i.color }} />
          {i.label}
        </li>
      ))}
    </ul>
  );
}

/** "29 short" or "86 spare": slots minus the open cases that need one. */
export function regionGap(r: CapacityRegion): string {
  const d = r.slots - r.cases;
  return d < 0 ? `${n(-d)} short` : `${n(d)} spare`;
}

/** The takeaway for the region chart, from the numbers: which regions are short and which have spare. */
export function regionTakeaway(regions: CapacityRegion[]): string {
  const short = regions.filter((r) => r.slots < r.cases);
  const spare = regions.filter((r) => r.slots > r.cases);
  const names = (rs: CapacityRegion[]) => (rs.length <= 1 ? rs.map((r) => r.name).join("") : `${rs.slice(0, -1).map((r) => r.name).join(", ")} and ${rs.at(-1)!.name}`);
  if (short.length === 0) return "Every region has a slot for each open case.";
  const gap = short.reduce((t, r) => t + r.cases - r.slots, 0);
  const lead = `${names(short)} ${short.length === 1 ? "is" : "are"} ${n(gap)} slots short`;
  return spare.length ? `${lead} while ${names(spare)} ${spare.length === 1 ? "has" : "have"} spare.` : `${lead}.`;
}

/** Slots against open cases that need one, in each region: paired bars on one scale, the difference printed per row. */
export function RegionCapacityChart({ regions }: { regions: CapacityRegion[] }) {
  const max = Math.max(1, ...regions.flatMap((r) => [r.slots, r.cases]));
  return (
    <ChartFigure
      caption={<><span className="font-medium text-ink">{regionTakeaway(regions)}</span> Endoscopy slots and open cases needing one, by region.</>}
      table={{
        caption: "Slots and open cases by region",
        head: ["Region", "Slots", "Open cases", "Difference"],
        rows: regions.map((r) => [r.name, n(r.slots), n(r.cases), regionGap(r)]),
      }}
    >
      <Key items={[{ color: SLOTS, label: "Slots" }, { color: OPEN, label: "Open cases needing a slot" }]} />
      <ul className="m-0 mt-2 list-none p-0">
        {regions.map((r) => (
          <li key={r.key}>
            <Mark
              className="grid grid-cols-[minmax(0,6.5rem)_minmax(0,1fr)_auto] items-center gap-x-3 py-2"
              tip={<><strong className="font-medium">{r.name}</strong>: {n(r.slots)} slots for {n(r.cases)} open cases, {regionGap(r)}.</>}
            >
              <span className="text-body font-medium">{r.name}</span>
              <span className="flex min-w-0 flex-col gap-0.5 border-l border-limestone-400 py-0.5">
                {(
                  [
                    [r.slots, SLOTS],
                    [r.cases, OPEN],
                  ] as const
                ).map(([v, color], i) => (
                  <span key={i} className="flex h-3.5 items-center gap-1.5">
                    <span aria-hidden className="block h-full flex-none rounded-r-[4px]" style={{ ...w(v, max), backgroundColor: color }} />
                    <span className="text-small leading-none tabular-nums">{n(v)}</span>
                  </span>
                ))}
              </span>
              <span data-testid={`region-gap-${r.key}`} className="text-right text-body font-semibold tabular-nums">
                {regionGap(r)}
              </span>
            </Mark>
          </li>
        ))}
      </ul>
    </ChartFigure>
  );
}

/** The measured pooling test: share within 31 days per scenario, one small panel per capacity level on a shared 0 to 50% axis. */
export function PoolingChart({ runs }: { runs: PoolRun[] }) {
  const max = 50;
  return (
    <ChartFigure
      caption={
        <>
          <span className="font-medium text-ink">Sharing slots across regions helps only when there are enough slots overall.</span> Share of FIT+ patients with a
          colonoscopy within 31 days, with capacity placed in the wrong regions.
        </>
      }
      table={{
        caption: "Share within 31 days, by scenario",
        head: ["Scenario", "Slots per FIT+", "Within 31 days"],
        rows: runs.flatMap((run) => run.rows.map((r) => [`${r.label} (${run.slotsPerCase} slots)`, run.slotsPerCase, r.display])),
      }}
    >
      <div className="grid gap-x-6 gap-y-4 sm:grid-cols-2">
        {runs.map((run) => (
          <div key={run.slotsPerCase} className="min-w-0">
            <h3 className="m-0 text-small font-semibold text-ink">
              {run.slotsPerCase} slots per FIT+ · {run.enough ? "enough overall" : "short overall"}
            </h3>
            <ul className="m-0 mt-1 list-none p-0">
              {run.rows.map((r) => {
                const pooled = r.id.endsWith("skewed_pool_60");
                return (
                  <li key={r.id}>
                    <Mark
                      className="flex flex-col justify-center"
                      tip={<><strong className="font-medium">{r.label}</strong>: {r.display} within 31 days at {run.slotsPerCase} slots per FIT+.</>}
                    >
                      <span className={cn("text-small leading-4", pooled ? "font-semibold" : "text-ink-soft")}>{r.label}</span>
                      <span className="mt-1 flex h-3 items-center gap-1.5 border-l border-limestone-400">
                        <span aria-hidden className="block h-full flex-none rounded-r-[4px]" style={{ ...w(r.share, max), backgroundColor: pooled ? SLOTS : CHROME.deemphasis }} />
                        <span className={cn("text-small leading-none tabular-nums", pooled && "font-semibold")}>{r.display}</span>
                      </span>
                    </Mark>
                  </li>
                );
              })}
            </ul>
            <div aria-hidden className="relative ml-0 mr-[20%] mt-1 h-4 text-small text-muted tabular-nums">
              <span data-axis className="absolute left-0">0%</span>
              <span data-axis className="absolute left-1/2 -translate-x-1/2">25%</span>
              <span data-axis className="absolute left-full -translate-x-1/2">50%</span>
            </div>
          </div>
        ))}
      </div>
      <div className="mt-3">
        <Key items={[{ color: SLOTS, label: "Sharing across regions (60% willing)" }, { color: CHROME.deemphasis, label: "Other scenarios" }]} />
      </div>
    </ChartFigure>
  );
}
