"use client";

import Link from "next/link";
import { useId, useState } from "react";
import { cn } from "@/lib/utils";

/*
 * The small parts the Programme charts share: a tooltip that opens on hover and on focus, a focusable mark,
 * a figure with a "Show as table" toggle, a segmented toggle and the fills that tell series apart without colour.
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

export type TableData = { caption: string; head: string[]; rows: string[][] };

/** A chart in a figure with its caption, and the same numbers as a table behind a toggle. */
export function ChartFigure({ caption, table, children, className }: { caption: React.ReactNode; table: TableData; children: React.ReactNode; className?: string }) {
  const [asTable, setAsTable] = useState(false);
  return (
    <figure className={cn("m-0", className)}>
      <div className="flex flex-wrap items-start justify-between gap-x-3 gap-y-1">
        <figcaption className="min-w-0 flex-1 basis-48 text-small leading-4 text-muted">{caption}</figcaption>
        <button
          type="button"
          aria-pressed={asTable}
          onClick={() => setAsTable(!asTable)}
          className={cn(
            "inline-flex min-h-11 flex-none cursor-pointer items-center rounded-control border border-control px-3 text-small text-ink md:min-h-8",
            "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-hue",
            asTable ? "bg-active font-medium" : "bg-surface hover:bg-raised",
          )}
        >
          Show as table
        </button>
      </div>
      <div className="mt-3 tabular-nums">
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
              {table.rows.map((row) => (
                <tr key={row[0]}>
                  {row.map((cell, i) =>
                    i === 0 ? (
                      <th key={i} scope="row" className="border-b border-rule px-1 py-2 text-left font-normal">
                        {cell}
                      </th>
                    ) : (
                      <td key={i} className="border-b border-rule px-1 py-2 text-right">
                        {cell}
                      </td>
                    ),
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          children
        )}
      </div>
    </figure>
  );
}

/** Two or three choices where exactly one applies; real buttons with aria-pressed. */
export function Segmented<T extends string>({ label, value, options, onChange }: { label: string; value: T; options: { value: T; label: string }[]; onChange: (v: T) => void }) {
  return (
    <div role="group" aria-label={label} className="inline-flex max-w-full flex-wrap gap-1 rounded-control bg-raised p-1">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          aria-pressed={value === o.value}
          onClick={() => onChange(o.value)}
          className={cn(
            "min-h-11 cursor-pointer rounded-[4px] border-0 px-3 text-body md:min-h-8",
            "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-hue",
            value === o.value ? "bg-surface font-medium text-ink shadow-sm" : "bg-transparent text-ink-soft hover:text-ink",
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
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
