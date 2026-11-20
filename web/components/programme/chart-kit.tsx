"use client";

import { Table2 } from "lucide-react";
import Link from "next/link";
import { Fragment, useId, useState } from "react";
import { cn } from "@/lib/utils";

/*
 * The small parts the Programme charts share: a tooltip that opens on hover and on focus, a focusable mark,
 * a figure with an icon-only "Show as table" toggle, a segmented toggle and the fills that tell series apart without colour.
 */

/** Fills that differ in texture as well as colour, so no series is told apart by colour alone. */
export const FILL = {
  solid: { backgroundColor: "var(--color-hue)" },
  hatch: {
    backgroundColor: "var(--color-surface)",
    backgroundImage: "repeating-linear-gradient(135deg, var(--color-negative) 0 2px, transparent 2px 6px)",
    boxShadow: "inset 0 0 0 1px var(--color-negative)",
  },
  open: { backgroundColor: "var(--color-raised)", boxShadow: "inset 0 0 0 1px var(--color-control)" },
  ink: {
    backgroundColor: "var(--color-surface)",
    backgroundImage: "repeating-linear-gradient(45deg, var(--color-ink-soft) 0 2px, transparent 2px 6px)",
    boxShadow: "inset 0 0 0 1px var(--color-ink-soft)",
  },
  muted: {
    backgroundColor: "var(--color-surface)",
    backgroundImage: "repeating-linear-gradient(135deg, var(--color-muted) 0 2px, transparent 2px 5px)",
    boxShadow: "inset 0 0 0 1px var(--color-muted)",
  },
  outline: { backgroundColor: "var(--color-mint)", boxShadow: "inset 0 0 0 2px var(--color-hue)" },
} as const;

export type FillName = keyof typeof FILL;

/** Wraps one mark. The tip shows on hover and on keyboard focus, and Escape closes it. */
export function Tip({ tip, children, className }: { tip: React.ReactNode; children: (describedBy: string) => React.ReactNode; className?: string }) {
  const [hover, setHover] = useState(false);
  const [focus, setFocus] = useState(false);
  const [dismissed, setDismissed] = useState(false);
  const id = useId();
  const open = (hover || focus) && !dismissed;
  return (
    <span
      className={cn("relative block", className)}
      onMouseEnter={() => {
        setHover(true);
        setDismissed(false);
      }}
      onMouseLeave={() => setHover(false)}
      onFocus={() => {
        setFocus(true);
        setDismissed(false);
      }}
      onBlur={() => setFocus(false)}
      onKeyDown={(e) => {
        if (e.key === "Escape") setDismissed(true);
      }}
    >
      {children(id)}
      <span
        id={id}
        role="tooltip"
        hidden={!open}
        data-tip
        className="pointer-events-none absolute bottom-full left-0 z-20 mb-1 w-max max-w-full rounded-control bg-ink px-3 py-2 text-small text-surface shadow-md"
      >
        {tip}
      </span>
    </span>
  );
}

const MARK =
  "block min-h-11 w-full cursor-default rounded-control border-0 bg-transparent p-0 text-left font-[inherit] text-ink no-underline hover:text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-hue";

/** A focusable chart mark with a tooltip: a link when it goes somewhere, else a button. */
export function Mark({ tip, href, className, children }: { tip: React.ReactNode; href?: string; className?: string; children: React.ReactNode }) {
  return (
    <Tip tip={tip}>
      {(describedBy) =>
        href ? (
          <Link href={href} aria-describedby={describedBy} className={cn(MARK, "cursor-pointer", className)}>
            {children}
          </Link>
        ) : (
          <button type="button" aria-describedby={describedBy} className={cn(MARK, className)}>
            {children}
          </button>
        )
      }
    </Tip>
  );
}

export type TableData = {
  caption: string;
  head: string[];
  rows: string[][];
  /** Optional one line per row, shown under it across every column (so a narrow screen gets no extra column). */
  details?: (string | null)[];
};

/** A chart in a figure with its caption, and the same numbers as a table behind a toggle. */
export function ChartFigure({
  caption,
  table,
  children,
  className,
  captionBelow = false,
}: {
  caption: React.ReactNode;
  table: TableData;
  children: React.ReactNode;
  className?: string;
  /** Put the caption and the table toggle under the chart, so the chart sits straight under its heading. */
  captionBelow?: boolean;
}) {
  const [asTable, setAsTable] = useState(false);
  const toggle = <TableToggle pressed={asTable} onToggle={() => setAsTable(!asTable)} />;
  const captionLine = <figcaption className={cn("text-small leading-4 text-muted", captionBelow ? "mt-5" : "min-w-0 flex-1 basis-48")}>{caption}</figcaption>;
  const head = captionBelow ? (
    // The chart sits straight under its heading: the toggle floats in the chart's own top margin, the caption goes below.
    <div className={cn("flex justify-end", !asTable && "-mb-7")}>{toggle}</div>
  ) : (
    <div className="flex items-start justify-between gap-x-3">
      {captionLine}
      {toggle}
    </div>
  );
  return (
    <figure className={cn("m-0", className)}>
      {head}
      <div className={cn("tabular-nums", (!captionBelow || asTable) && "mt-3")}>
        {asTable ? (
          <table className="w-full border-collapse text-small">
            <caption className="sr-only">{table.caption}</caption>
            <thead>
              <tr>
                {table.head.map((h, i) => (
                  <th key={h} scope="col" className={cn("border-b border-line px-1 py-2 font-medium text-muted", i === 0 ? "text-left" : "text-right")}>
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {table.rows.map((row, r) => {
                const detail = table.details?.[r] ?? null;
                const edge = detail === null ? "border-b border-rule" : "";
                return (
                  <Fragment key={row[0]}>
                    <tr>
                      {row.map((cell, i) =>
                        i === 0 ? (
                          <th key={i} scope="row" className={cn(edge, "px-1 py-2 text-left font-normal")}>
                            {cell}
                          </th>
                        ) : (
                          <td key={i} className={cn(edge, "px-1 py-2 text-right")}>
                            {cell}
                          </td>
                        ),
                      )}
                    </tr>
                    {detail !== null && (
                      <tr>
                        <td colSpan={table.head.length} className="border-b border-rule px-1 pb-2 text-left text-ink-soft">
                          {detail}
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        ) : (
          children
        )}
      </div>
      {captionBelow && captionLine}
    </figure>
  );
}

/**
 * The table view toggle: a quiet icon-only button at the chart's top-right. The visual box is 28px; the button itself
 * is 44px (negative margins keep the layout at 28px), so the hit area meets the touch-target size everywhere. Its name
 * stays "Show as table" and aria-pressed carries the state; the hover title says what a click will do.
 */
export function TableToggle({ pressed, onToggle }: { pressed: boolean; onToggle: () => void }) {
  return (
    <button
      type="button"
      aria-label="Show as table"
      aria-pressed={pressed}
      title={pressed ? "Show as chart" : "Show as table"}
      onClick={onToggle}
      className="group -m-2 inline-flex size-11 flex-none cursor-pointer items-center justify-center rounded-control border-0 bg-transparent p-0 text-ink-soft focus-visible:outline-none"
    >
      <span
        aria-hidden
        className={cn(
          "inline-flex size-7 items-center justify-center rounded-control border transition-colors",
          "group-focus-visible:outline-2 group-focus-visible:outline-offset-2 group-focus-visible:outline-hue",
          pressed ? "border-control bg-active text-ink" : "border-transparent group-hover:border-control group-hover:bg-raised group-hover:text-ink",
        )}
      >
        <Table2 className="size-4" strokeWidth={1.75} />
      </span>
    </button>
  );
}

/** A key: a swatch in the series' own fill beside its name. */
export function Legend({ items }: { items: { fill: FillName; label: string }[] }) {
  return (
    <ul className="m-0 flex list-none flex-wrap gap-x-4 gap-y-1 p-0 text-small text-ink-soft">
      {items.map((i) => (
        <li key={i.label} className="flex items-center gap-1.5">
          <span aria-hidden className="block h-3 w-5 flex-none rounded-[3px]" style={FILL[i.fill]} />
          {i.label}
        </li>
      ))}
    </ul>
  );
}
