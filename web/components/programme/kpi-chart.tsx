"use client";

import { useState } from "react";
import { SegmentedToggle } from "@/components/ui/segmented-toggle";
import { CATEGORICAL } from "@/components/charts/palette";
import { ChartFigure, Mark } from "./chart-kit";
import { pct, seenShare, type KpiSet } from "./model";

type Source = "demo" | "heldout";
type Arm = "eid" | "wasl" | "truth";

/** Every method, in table order. */
const ARMS: { arm: Arm; name: string }[] = [
  { arm: "eid", name: "Emirates ID only" },
  { arm: "wasl", name: "Wasl" },
  { arm: "truth", name: "Simulated truth" },
];

/** The two measured methods, in the shared palette's first two slots (Wasl jade, the comparator ochre). */
const BARS: { arm: Exclude<Arm, "truth">; name: string; fill: string; emphasis?: boolean }[] = [
  { arm: "wasl", name: "Wasl", fill: CATEGORICAL[0], emphasis: true },
  { arm: "eid", name: "Emirates ID only", fill: CATEGORICAL[1] },
];

const TICKS = [0, 25, 50, 75, 100];

const SOURCES: Record<Source, { button: string; table: string }> = {
  demo: { button: "This demo run", table: "This demo run" },
  heldout: { button: "Average of 5 test runs", table: "Average of 5 test runs" },
};

/** Label, track, value: the three columns every bar row and the axis share. */
const COLS = "grid grid-cols-[minmax(87px,.8fr)_minmax(0,2fr)_minmax(42px,.38fr)] gap-[7px] @min-[701px]:grid-cols-[minmax(110px,.9fr)_minmax(0,4fr)_minmax(47px,.35fr)] @min-[701px]:gap-3";

/**
 * What each method can see: the share of the real (simulated-truth) on-time colonoscopies it counts, on a 0 to 100% scale,
 * for one demo run or the five-seed held-out mean. The programme's own on-time rate and the DoH target are a plain line
 * under it: they describe the programme, not the measurement.
 */
export function KpiChart({ kpi, target }: { kpi: Record<Source, KpiSet>; target: number | null }) {
  const [source, setSource] = useState<Source>("demo");
  const set = kpi[source];
  const { truth } = set.rates;
  const seen = { eid: seenShare(set.rates.eid, truth), wasl: seenShare(set.rates.wasl, truth) };
  const title = `Wasl sees ${seen.wasl}% of on-time colonoscopies; Emirates ID alone sees ${seen.eid}%`;
  const label = `Share of on-time colonoscopies each method sees, on a 0 to 100% scale: ${BARS.map(({ arm, name }) => `${name} ${seen[arm]}%`).join(", ")}.`;

  return (
    <>
      <p data-testid="kpi-title" className="m-0 mt-4 text-[17px] font-semibold leading-snug text-ink @min-[701px]:text-[20px]">
        {title}
      </p>
      <ChartFigure
        captionBelow
        caption={
          <>
            <span data-testid="kpi-why">
              The demo run uses data Wasl was tuned on; the 5-run average uses new data, so it is the fairer figure.
            </span>
          </>
        }
        table={{
          caption: `Colonoscopy within 31 days, ${SOURCES[source].table}`,
          head: ["Method", "Sees", "31-day rate", "Detail"],
          rows: [
            ...ARMS.map(({ arm, name }) => [name, arm === "truth" ? "100%" : `${seen[arm]}%`, pct(set.rates[arm]), set.detail[arm]]),
            ...(target === null ? [] : [["Target", "", `${target}%`, "DoH 31-day standard"]]),
          ],
        }}
      >
        <div role="group" aria-label={label} className={`${COLS} mt-4`}>
          <ul className="col-span-full m-0 grid list-none p-0">
            {BARS.map(({ arm, name, fill, emphasis }) => {
              const v = seen[arm];
              return (
                <li key={arm} data-method={arm} className="border-b border-rule last:border-b-0">
                  <Mark
                    tip={
                      <>
                        <strong className="font-medium">
                          {name}: sees {v}%
                        </strong>
                        <br />
                        31-day rate {pct(set.rates[arm])} of the simulated truth&apos;s {pct(truth)}
                        <br />
                        {set.detail[arm]}
                      </>
                    }
                    className={`${COLS} min-h-14 items-center @min-[701px]:min-h-[clamp(56px,6vw,84px)]`}
                  >
                    <span className={`text-[12px] leading-[1.35] @min-[701px]:text-body ${emphasis ? "font-semibold text-ink" : "font-medium text-ink-soft"}`}>{name}</span>
                    <span aria-hidden className="relative flex h-[13px] min-w-0 items-center @min-[701px]:h-5">
                      <span className="block h-full rounded-r-[4px] motion-safe:transition-[width] motion-safe:duration-300" style={{ width: `${Math.min(v, 100)}%`, minWidth: 4, backgroundColor: fill }} />
                    </span>
                    <span data-testid={`kpi-${arm}`} className="whitespace-nowrap text-[12px] font-semibold tabular-nums text-ink @min-[701px]:text-body">
                      {v}%
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
                data-axis
                className={`absolute tabular-nums ${i === 0 ? "" : i === TICKS.length - 1 ? "-translate-x-full" : "-translate-x-1/2"} ${i % 2 === 1 ? "hidden @min-[701px]:inline" : ""}`}
                style={{ left: `${t}%` }}
              >
                {t}%
              </span>
            ))}
          </div>
        </div>
      </ChartFigure>

      <p data-testid="kpi-programme" className="m-0 mt-4 text-body text-ink-soft">
        Programme on-time rate: <span className="font-semibold tabular-nums text-ink">{pct(truth)}</span>
        {target !== null && <> (DoH target {target}%)</>} — Wasl shows who still needs a colonoscopy.
      </p>
      <p data-testid="kpi-message" className="m-0 mt-1 text-body text-ink-soft">
        Wasl doesn&apos;t create colonoscopy slots. It makes sure no completed colonoscopy is missed and no lost patient stays invisible.
      </p>

      <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-2">
        <SegmentedToggle
          label="31-day rate source"
          value={source}
          onChange={setSource}
          items={[
            { value: "demo", label: SOURCES.demo.button },
            { value: "heldout", label: SOURCES.heldout.button },
          ]}
        />
      </div>
    </>
  );
}
