"use client";

import { useState } from "react";
import { SegmentedToggle } from "@/components/ui/segmented-toggle";
import { CATEGORICAL, CHROME } from "@/components/charts/palette";
import { ChartFigure, Mark } from "./chart-kit";
import { pct, points, type KpiSet, type Rates } from "./model";

type Source = "demo" | "heldout";
type Arm = keyof Rates;

/**
 * One row per method on the full 0 to 100% scale, in table order. Wasl is jade and the comparator ochre (the shared
 * palette's first two slots); the simulated truth is the reference, so it is an ink ring, not a third series colour.
 */
const ROWS: { arm: Arm; name: string; color: string; emphasis?: boolean }[] = [
  { arm: "eid", name: "Emirates ID only", color: CATEGORICAL[1] },
  { arm: "wasl", name: "Wasl", color: CATEGORICAL[0], emphasis: true },
  { arm: "truth", name: "Simulated truth", color: CHROME.reference },
];

const TICKS = [0, 25, 50, 75, 100];

/** Opaque, so the gap label can sit over the row rules without them showing through. */
const GAP_FILL = "color-mix(in oklab, var(--color-limestone-300) 50%, var(--color-surface))";

const SOURCES: Record<Source, { button: string; scope: string }> = {
  demo: { button: "This demo run", scope: "in testing" },
  heldout: { button: "Five-seed held-out mean", scope: "on held-out noise, mean of 5 seeds" },
};

/** The line under each method's name: how far it sits from the simulated truth. */
function note(arm: Arm, r: Rates): string | null {
  if (arm === "truth") return null;
  if (arm === "wasl") return `Within ${points(r.truth, r.wasl)} of truth`;
  return `${r.eid < r.truth ? "Undercounts" : "Overcounts"} by ${points(r.truth, r.eid)}`;
}

/** Name and note | the rate | the track: the three columns every row, the overlay and the axis share. */
const COLS =
  "grid grid-cols-[minmax(84px,1fr)_46px_minmax(0,2.3fr)] gap-x-2.5 @min-[701px]:grid-cols-[minmax(150px,1fr)_92px_minmax(0,4.2fr)] @min-[701px]:gap-x-4";

/**
 * The 31-day rate by each method as dots on one full 0 to 100% scale, with the target line and the gap between the
 * simulated truth and the target shaded. Wasl's dot sits on the truth guide; Emirates ID's sits well left of it.
 */
export function KpiChart({ kpi, target }: { kpi: Record<Source, KpiSet>; target: number | null }) {
  const [source, setSource] = useState<Source>("demo");
  const set = kpi[source];
  const { wasl, truth } = set.rates;
  const gap = target !== null && truth < target ? points(target, truth) : null;
  const label = `On a 0 to 100% scale: ${ROWS.map(({ arm, name }) => `${arm === "truth" ? name.toLowerCase() : name} ${pct(set.rates[arm])}`).join(", ")}${
    target === null ? "" : `, target ${target}%`
  }.`;

  return (
    <>
      <ChartFigure
        captionBelow
        caption={
          <span data-testid="kpi-why">The demo run is one run on training-time noise. The five-seed mean averages 5 seeds on held-out noise.</span>
        }
        table={{
          caption: `31-day rate, ${SOURCES[source].button}`,
          head: ["Method", "Rate", "Detail"],
          rows: [
            ...ROWS.map(({ arm, name }) => [name, pct(set.rates[arm]), set.detail[arm]]),
            ...(target === null ? [] : [["Target", `${target}%`, "DoH 31-day standard"]]),
            ...(gap === null ? [] : [["Gap to target", gap, "Target minus the simulated truth"]]),
          ],
        }}
      >
        <div role="group" aria-label={label} className={`relative ${COLS} mt-8 @min-[701px]:mt-[clamp(31px,4cqi,53px)]`}>
          {gap !== null && (
            <div aria-hidden className="pointer-events-none relative col-start-3 row-start-1">
              <div className="absolute inset-y-0" style={{ left: `${truth}%`, width: `${target! - truth}%`, backgroundColor: GAP_FILL }} />
            </div>
          )}
          <ul className="relative col-span-full row-start-1 m-0 grid list-none p-0">
            {ROWS.map(({ arm, name, color, emphasis }) => {
              const v = set.rates[arm];
              const sub = note(arm, set.rates);
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
                    className={`${COLS} min-h-14 items-center @min-[701px]:min-h-[clamp(60px,5.5vw,76px)]`}
                  >
                    <span className="flex min-w-0 flex-col leading-[1.3]">
                      <span className={`text-[12px] @min-[701px]:text-body ${emphasis ? "font-semibold text-ink" : "font-medium text-ink-soft"}`}>{name}</span>
                      {sub && <span className="text-[11px] text-muted @min-[701px]:text-small">{sub}</span>}
                    </span>
                    <span
                      data-testid={arm === "truth" ? "kpi-true" : `kpi-${arm}`}
                      className={`text-right text-[13px] font-semibold tabular-nums @min-[701px]:text-[22px] @min-[701px]:tracking-[-0.02em] ${arm === "truth" ? "text-ink-soft" : "text-ink"}`}
                    >
                      {pct(v)}
                    </span>
                    <span aria-hidden data-testid={arm === "wasl" ? "kpi-track" : undefined} className="relative block h-4 min-w-0">
                      <span className="absolute inset-x-0 top-1/2 h-px -translate-y-1/2" style={{ backgroundColor: CHROME.grid }} />
                      <span className="absolute left-0 top-1/2 h-[3px] -translate-y-1/2 rounded-full motion-safe:transition-[width] motion-safe:duration-300" style={{ width: `${v}%`, backgroundColor: color }} />
                      <span
                        className="absolute top-1/2 block size-3.5 -translate-x-1/2 -translate-y-1/2 rounded-full motion-safe:transition-[left] motion-safe:duration-300 @min-[701px]:size-4"
                        style={
                          arm === "truth"
                            ? { left: `${v}%`, backgroundColor: "var(--color-surface)", boxShadow: `inset 0 0 0 2px ${color}` }
                            : { left: `${v}%`, backgroundColor: color, boxShadow: "0 0 0 2px var(--color-surface)" }
                        }
                      />
                    </span>
                  </Mark>
                </li>
              );
            })}
          </ul>
          <div aria-hidden className="pointer-events-none relative z-10 col-start-3 row-start-1">
            {gap !== null && (
              <div className="absolute inset-y-0 flex items-center justify-center" style={{ left: `${truth}%`, width: `${target! - truth}%` }}>
                <span data-testid="kpi-gap" className="flex flex-col items-center rounded-control px-2 py-1 text-center leading-[1.2]" style={{ backgroundColor: GAP_FILL }}>
                  <span className="text-[13px] font-semibold tracking-[-0.01em] whitespace-nowrap text-ink tabular-nums @min-[701px]:text-[22px]">{gap}</span>
                  <span className="text-[11px] text-ink-soft @min-[701px]:text-small">below target</span>
                </span>
              </div>
            )}
            <div data-testid="kpi-truth-guide" className="absolute inset-y-0 border-l border-dashed" style={{ left: `${truth}%`, borderColor: CHROME.reference }} />
            {target !== null && (
              <div data-testid="kpi-target" className="absolute inset-y-0 border-l-2" style={{ left: `${target}%`, borderColor: CHROME.reference }}>
                <span className="absolute right-[5px] top-[-22px] whitespace-nowrap text-small font-semibold leading-[1.4] text-ink">
                  {target}% DoH target
                </span>
              </div>
            )}
          </div>
        </div>
        <div aria-hidden className={`${COLS} mt-[7px]`}>
          <div className="relative col-start-3 min-h-4 text-[10px] text-muted @min-[701px]:text-[12px]">
            {TICKS.map((t, i) => (
              <span
                key={t}
                data-axis
                className={`absolute tabular-nums ${i === 0 ? "" : i === TICKS.length - 1 ? "-translate-x-full" : "-translate-x-1/2"} ${i % 2 === 1 ? "hidden @min-[701px]:inline" : ""}`}
                style={{ left: `${t}%` }}
              >
                {t}%
              </span>
            ))}
          </div>
        </div>
        <ul aria-hidden className="m-0 mt-4 flex list-none flex-wrap gap-x-4 gap-y-1 p-0 text-small text-ink-soft">
          {ROWS.map(({ arm, name, color }) => (
            <li key={arm} className="flex items-center gap-1.5">
              <span
                className="block size-2.5 rounded-full"
                style={arm === "truth" ? { boxShadow: `inset 0 0 0 2px ${color}` } : { backgroundColor: color }}
              />
              {name}
            </li>
          ))}
          {gap !== null && (
            <li className="flex items-center gap-1.5">
              <span className="block h-2.5 w-4 rounded-[2px]" style={{ backgroundColor: GAP_FILL, boxShadow: `inset 0 0 0 1px ${CHROME.grid}` }} />
              Gap from the simulated truth to the target
            </li>
          )}
        </ul>
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
