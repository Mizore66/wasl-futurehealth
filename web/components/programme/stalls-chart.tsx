"use client";

import { useId, useState } from "react";
import { CATEGORICAL } from "@/components/charts/palette";
import { ChartFigure, Mark } from "./chart-kit";
import type { Stall, StallGroup } from "./model";

const n = (v: number) => v.toLocaleString("en-US");

/** Label | bar with its value at the tip | chevron: every bar hangs from one baseline, so lengths compare directly. */
function Rows({ stalls, max }: { stalls: Stall[]; max: number }) {
  return (
    <ol className="m-0 list-none p-0">
      {stalls.map((s) => (
        <li key={s.dropoff}>
          <Mark
            href={`/app/patients?dropoff=${s.dropoff}`}
            tip={<><strong className="font-medium">{s.label}</strong>: {n(s.count)} cases. Opens the patient list.</>}
            className="grid min-h-12 grid-cols-[minmax(0,9.5rem)_minmax(0,1fr)_16px] items-center gap-x-3 py-1 [&:hover_.stall-label]:underline"
          >
            <span className="stall-label text-body leading-tight">{s.label}</span>
            <span className="flex min-w-0 items-center gap-2 self-stretch border-l border-limestone-400">
              <span aria-hidden className="block h-3 flex-none rounded-r-[4px]" style={{ width: `${((s.count / max) * 80).toFixed(2)}%`, minWidth: 4, backgroundColor: CATEGORICAL[0] }} />
              <span className="text-small font-semibold tabular-nums">{n(s.count)}</span>
            </span>
            <svg aria-hidden width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--color-muted)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
              <path d="M6 3.5l4.5 4.5L6 12.5" />
            </svg>
          </Mark>
        </li>
      ))}
    </ol>
  );
}

/** The three largest stalls, with a disclosure for every sourced stall, grouped by pathway. */
export function StallsChart({ top, groups }: { top: Stall[]; groups: StallGroup[] }) {
  const [all, setAll] = useState(false);
  const panel = useId();
  const allMax = Math.max(1, ...groups.flatMap((g) => g.stalls.map((s) => s.count)));
  const allCount = groups.reduce((t, g) => t + g.stalls.length, 0);
  return (
    <ChartFigure
      caption={all ? "Every stall, by programme, on one scale." : "Cases at each stall."}
      table={{
        caption: "Where patients stall, every stall",
        head: ["Stall", "Pathway", "Cases"],
        rows: groups.flatMap((g) => g.stalls.map((s) => [s.label, g.pathway, n(s.count)])),
      }}
    >
      <div id={panel}>
        {all ? (
          groups.map((g) => (
            <div key={g.pathway} className="mt-2 first:mt-0">
              <h3 className="m-0 border-b border-rule pb-1 text-small font-medium text-muted">{g.pathway}</h3>
              <Rows stalls={g.stalls} max={allMax} />
            </div>
          ))
        ) : (
          <Rows stalls={top} max={top[0]?.count ?? 1} />
        )}
      </div>
      <button
        type="button"
        aria-expanded={all}
        aria-controls={panel}
        onClick={() => setAll(!all)}
        className="mt-2 inline-flex min-h-11 cursor-pointer items-center rounded-control border border-control bg-surface px-3 text-body text-ink hover:bg-raised focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-hue md:min-h-9"
      >
        {all ? "Show only the top 3" : `Show all ${allCount} stalls, every programme`}
      </button>
    </ChartFigure>
  );
}
