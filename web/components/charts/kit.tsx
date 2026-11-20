"use client";

import { Tip } from "@/components/programme/chart-kit";
import { cn } from "@/lib/utils";
import { CHROME } from "./palette";

/*
 * Small HTML chart parts shared by the evidence and results charts. Positions are percentages of a plot area that
 * sits inside each row's track with fixed side room for direct labels, so nothing overflows at 320 px.
 * Mark specs follow the dataviz skill: 2px lines, >= 8px dots with a 2px surface ring, bars <= 24px with a 4px
 * rounded data end and a square baseline, solid hairline grid, text in ink tokens only.
 */

/** Room either side of the plot for labels that sit beside the first and last marks. */
const PLOT = "absolute inset-y-0 left-9 right-12";
const AXIS_PAD = "pl-9 pr-12";

/** A linear scale onto 0–100 (percent of the plot width), clamped. */
export function scale(min: number, max: number) {
  return (v: number) => Math.max(0, Math.min(100, ((v - min) / (max - min || 1)) * 100));
}

const pct = (x: number) => `${x}%`;

/** Label column width at md and up; the label sits above the track on a phone. */
const ROW_GRID = "md:grid-cols-[minmax(0,var(--label-w))_minmax(0,1fr)]";

/**
 * One row of a chart: its label, and a track holding the marks. The whole row is the hover and focus target
 * (much bigger than any mark), and its tooltip gives the exact values.
 */
export function ChartRow({
  label,
  sub,
  tip,
  children,
  labelWidth = "13rem",
  testId,
  track = "h-6",
}: {
  label: React.ReactNode;
  sub?: React.ReactNode;
  tip: React.ReactNode;
  children: React.ReactNode;
  labelWidth?: string;
  testId?: string;
  track?: string;
}) {
  return (
    <Tip tip={tip}>
      {(describedBy) => (
        <button
          type="button"
          aria-describedby={describedBy}
          data-testid={testId}
          style={{ ["--label-w" as string]: labelWidth }}
          className={cn(
            "grid min-h-11 w-full cursor-default items-center gap-x-4 gap-y-0.5 rounded-control border-0 bg-transparent px-0 py-1 text-left font-[inherit] text-ink",
            "hover:bg-jade-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-hue",
            ROW_GRID,
          )}
        >
          <span className="min-w-0 text-small leading-4 text-ink-soft [overflow-wrap:anywhere]">
            {label}
            {sub && <span className="block text-muted">{sub}</span>}
          </span>
          <span className={cn("relative block", track)}>
            <span className={PLOT}>{children}</span>
          </span>
        </button>
      )}
    </Tip>
  );
}

/** Solid hairline gridlines at the tick positions, behind the marks of a row. */
export function Grid({ ticks, x }: { ticks: number[]; x: (v: number) => number }) {
  return (
    <>
      {ticks.map((t, i) => (
        <span
          key={t}
          aria-hidden
          className="absolute inset-y-0 w-px"
          style={{ left: pct(x(t)), backgroundColor: i === 0 ? CHROME.baseline : CHROME.grid }}
        />
      ))}
    </>
  );
}

/** Tick labels under the plot, aligned with the rows' plot area. Layout, not data: marked data-axis. */
export function Axis({
  ticks,
  x,
  format,
  labelWidth = "13rem",
  title,
}: {
  ticks: number[];
  x: (v: number) => number;
  format: (v: number) => string;
  labelWidth?: string;
  title?: string;
}) {
  return (
    <div
      data-axis
      aria-hidden
      style={{ ["--label-w" as string]: labelWidth }}
      className={cn("grid gap-x-4 text-small text-muted tabular-nums", ROW_GRID)}
    >
      <span className="hidden md:block" />
      <span className={cn("block", AXIS_PAD)}>
        <span className="relative block h-5">
          {ticks.map((t, i) => (
            <span
              key={t}
              className={cn("absolute top-0 whitespace-nowrap", i === 0 ? "" : i === ticks.length - 1 ? "-translate-x-full" : "-translate-x-1/2")}
              style={{ left: pct(x(t)) }}
            >
              {format(t)}
            </span>
          ))}
        </span>
        {title && <span className="block text-center">{title}</span>}
      </span>
    </div>
  );
}

/** A dot (>= 10px) with a 2px surface ring so it stays legible where it overlaps a line or another dot. */
export function Dot({ x, color, y = 50, size = 10, hollow = false }: { x: number; color: string; y?: number; size?: number; hollow?: boolean }) {
  return (
    <span
      aria-hidden
      className="absolute z-10 -translate-x-1/2 -translate-y-1/2 rounded-full"
      style={{
        left: pct(x),
        top: pct(y),
        width: size,
        height: size,
        backgroundColor: hollow ? CHROME.surface : color,
        boxShadow: hollow ? `inset 0 0 0 2px ${color}, 0 0 0 2px ${CHROME.surface}` : `0 0 0 2px ${CHROME.surface}`,
      }}
    />
  );
}

/** A horizontal 2px line from one value to another (an interval, a whisker, the bar of a dumbbell). */
export function Seg({ from, to, color, y = 50, thick = 2 }: { from: number; to: number; color: string; y?: number; thick?: number }) {
  const [a, b] = from <= to ? [from, to] : [to, from];
  return (
    <span
      aria-hidden
      className="absolute -translate-y-1/2 rounded-full"
      style={{ left: pct(a), width: `calc(${b - a}% + 0px)`, top: pct(y), height: thick, backgroundColor: color }}
    />
  );
}

/** A vertical tick: the end cap of a whisker, the truth marker, a base case. */
export function Tick({ x, color, y = 50, h = 14, w = 2 }: { x: number; color: string; y?: number; h?: number; w?: number }) {
  return (
    <span
      aria-hidden
      className="absolute z-10 -translate-x-1/2 -translate-y-1/2"
      style={{ left: pct(x), top: pct(y), width: w, height: h, backgroundColor: color }}
    />
  );
}

/** A bar from the baseline: square at the baseline, 4px rounded data end, at most 24px thick. */
export function Bar({ from = 0, to, color, y = 50, thick = 10 }: { from?: number; to: number; color: string; y?: number; thick?: number }) {
  const [a, b] = from <= to ? [from, to] : [to, from];
  return (
    <span
      aria-hidden
      className="absolute -translate-y-1/2"
      style={{
        left: pct(a),
        width: pct(Math.max(b - a, 0.4)),
        top: pct(y),
        height: Math.min(thick, 24),
        backgroundColor: color,
        borderRadius: from <= to ? "0 4px 4px 0" : "4px 0 0 4px",
      }}
    />
  );
}

/** A direct label beside a mark, in ink (never the series colour). */
export function Label({
  x,
  side = "right",
  y = 50,
  strong = false,
  children,
}: {
  x: number;
  side?: "left" | "right" | "above";
  y?: number;
  strong?: boolean;
  children: React.ReactNode;
}) {
  const pos =
    side === "right"
      ? "translate-x-2 -translate-y-1/2"
      : side === "left"
        ? "-translate-x-[calc(100%+8px)] -translate-y-1/2"
        : "-translate-x-1/2 -translate-y-[calc(100%+6px)]";
  return (
    <span
      className={cn("absolute z-10 whitespace-nowrap text-small leading-4 tabular-nums", strong ? "font-medium text-ink" : "text-ink-soft", pos)}
      style={{ left: pct(x), top: pct(y) }}
    >
      {children}
    </span>
  );
}

export type LegendItem = { color: string; label: string; shape?: "dot" | "bar" | "tick" | "line" };

/** The key for two or more series. Swatches mirror the mark; the names stay in ink. */
export function Legend({ items, className }: { items: LegendItem[]; className?: string }) {
  return (
    <ul className={cn("m-0 flex list-none flex-wrap gap-x-4 gap-y-1 p-0 text-small text-ink-soft", className)}>
      {items.map((i) => (
        <li key={i.label} className="flex items-center gap-1.5">
          <span
            aria-hidden
            className={cn(
              "block flex-none",
              i.shape === "bar" && "h-2.5 w-4 rounded-r-[3px]",
              i.shape === "tick" && "h-3.5 w-0.5",
              i.shape === "line" && "h-0.5 w-4",
              (i.shape ?? "dot") === "dot" && "size-2.5 rounded-full",
            )}
            style={{ backgroundColor: i.color }}
          />
          {i.label}
        </li>
      ))}
    </ul>
  );
}

/** A tooltip body: values lead, names follow, each keyed by a short stroke of its series colour. */
export function TipRows({ title, rows, note }: { title?: React.ReactNode; rows: { color?: string; name: string; value: string }[]; note?: React.ReactNode }) {
  return (
    <span className="block max-w-[22rem] whitespace-normal">
      {title && <span className="block font-medium">{title}</span>}
      {rows.map((r) => (
        <span key={r.name} className="flex items-center gap-2">
          {r.color && <span aria-hidden className="block h-0.5 w-3 flex-none" style={{ backgroundColor: r.color }} />}
          <span className="font-medium tabular-nums">{r.value}</span>
          <span className="opacity-80">{r.name}</span>
        </span>
      ))}
      {note && <span className="mt-1 block opacity-80">{note}</span>}
    </span>
  );
}

/** A segmented toggle (one row, above the charts it scopes). */
export function Toggle<T extends string>({
  label,
  options,
  value,
  onChange,
}: {
  label: string;
  options: { value: T; label: string }[];
  value: T;
  onChange: (v: T) => void;
}) {
  return (
    <div role="group" aria-label={label} className="inline-flex max-w-full flex-wrap rounded-control border border-control bg-surface p-0.5">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          aria-pressed={o.value === value}
          onClick={() => onChange(o.value)}
          className={cn(
            "inline-flex min-h-11 cursor-pointer items-center rounded-[6px] border-0 px-3 text-small md:min-h-8",
            "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-hue",
            o.value === value ? "bg-active font-medium text-ink" : "bg-transparent text-ink-soft hover:bg-raised",
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

/** Even ticks from 0 to a round maximum. */
export function ticksTo(max: number, n = 4): number[] {
  return Array.from({ length: n + 1 }, (_, i) => Number(((max / n) * i).toPrecision(6)));
}
