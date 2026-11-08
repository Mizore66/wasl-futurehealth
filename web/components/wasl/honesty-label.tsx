import { cn } from "@/lib/utils";
import { HONESTY, type HonestyKind } from "@/lib/honesty";
import { CHIP_BOX, dotClass } from "./honesty-chip";

/**
 * A data-mode chip: how true the thing beside it is. Children override the default word.
 * One height, padding and type everywhere, with a leading dot so it reads as a status, not as prose.
 * Plain-text call sites (footers, story captions) override the box and hide the dot via `[&>[data-dot]]:hidden`.
 */
export function HonestyLabel({
  kind,
  children,
  className,
}: {
  kind: HonestyKind;
  children?: React.ReactNode;
  className?: string;
}) {
  return (
    <span data-honesty={kind} className={cn(CHIP_BOX, className)}>
      <span aria-hidden data-dot className={dotClass(kind)} />
      {children ?? HONESTY[kind]}
    </span>
  );
}
