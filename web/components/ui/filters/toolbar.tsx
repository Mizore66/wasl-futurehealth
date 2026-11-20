import Link from "next/link";
import type { ComponentProps, ReactNode } from "react";
import { cn } from "@/lib/utils";
import { segmentOn, segmentRow, segmentTab, type FilterSpec } from "./types";

/**
 * The filter bar of a list page, one per page. A labelled group, not role="toolbar": a toolbar promises arrow-key
 * focus between its controls, which a text field and a row of links cannot honour, so Tab moves through them instead.
 */
export function Toolbar({ label, className, ...props }: ComponentProps<"div"> & { label: string }) {
  return <div role="group" aria-label={label} className={cn("flex min-w-0 flex-col gap-3", className)} {...props} />;
}

export type Segment = { label: string; href: string; current: boolean; count?: number };

/** Single-select quick filters as underlined tabs. Links with aria-current, so each view is a URL that works without JS. */
export function SegmentedFilter({ label, items, className }: { label: string; items: Segment[]; className?: string }) {
  return (
    <nav aria-label={label} className={cn(segmentRow, className)}>
      {items.map((s) => (
        <Link
          key={s.label}
          href={s.href}
          aria-current={s.current ? "true" : undefined}
          className={cn(segmentTab, s.current ? segmentOn : "border-transparent")}
        >
          {s.label}
          {s.count !== undefined && <> <span className="font-normal text-muted tabular-nums">{s.count}</span></>}
        </Link>
      ))}
    </nav>
  );
}

/** Appears only while at least one of `filters` is set. */
export function ClearAll({ filters, href, className }: { filters: FilterSpec[]; href: string; className?: string }) {
  if (!filters.some((f) => f.value)) return null;
  return (
    <Link href={href} className={cn("inline-flex min-h-11 shrink-0 items-center px-2 text-body whitespace-nowrap text-jade-700 underline-offset-4 hover:underline", className)}>
      Clear all
    </Link>
  );
}

/** How many results the filters leave, announced politely when it changes. */
export function ResultCount({
  count,
  one = "result",
  many = `${one}s`,
  children,
  className,
  ...props
}: Omit<ComponentProps<"span">, "children"> & { count: number; one?: string; many?: string; children?: ReactNode }) {
  return (
    <span aria-live="polite" className={cn("tabular-nums", className)} {...props}>
      {count} {count === 1 ? one : many}
      {children}
    </span>
  );
}

