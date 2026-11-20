"use client";

import type { ReactNode } from "react";
import { ToggleGroup } from "radix-ui";
import { cn } from "@/lib/utils";

export type ToggleSegment<T extends string> = { value: T; label: ReactNode; ariaLabel?: string };

/**
 * SegmentedFilter's underlined tabs for a view held in client state rather than the URL: a Radix single-select
 * ToggleGroup, so it reads as a radio group and arrow keys move between choices. Built on the primitive, not
 * ui/toggle-group, because that wrapper's shadcn styling removes the app's focus outline.
 */
export function SegmentedToggle<T extends string>({
  label,
  value,
  onChange,
  items,
  className,
}: {
  label: string;
  value: T;
  onChange: (value: T) => void;
  items: ToggleSegment<T>[];
  className?: string;
}) {
  return (
    <ToggleGroup.Root
      type="single"
      aria-label={label}
      value={value}
      onValueChange={(next) => next && onChange(next as T)}
      className={cn("flex min-w-0 gap-1 overflow-x-auto border-b border-line [scrollbar-width:thin]", className)}
    >
      {items.map((s) => (
        <ToggleGroup.Item
          key={s.value}
          value={s.value}
          aria-label={s.ariaLabel}
          className="inline-flex min-h-11 shrink-0 cursor-pointer items-center gap-1.5 border-b-2 border-transparent px-2.5 text-body whitespace-nowrap text-ink hover:bg-ground data-[state=on]:border-jade-600 data-[state=on]:font-medium"
        >
          {s.label}
        </ToggleGroup.Item>
      ))}
    </ToggleGroup.Root>
  );
}
