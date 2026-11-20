import { cn } from "@/lib/utils";
import { HONESTY, type HonestyKind } from "@/lib/honesty";
import { CHIP_BOX, dotClass } from "./honesty-chip";

/**
 * A data-mode chip: how true the thing beside it is. Children override the default word.
 * One height, padding and type everywhere, with a leading dot so it reads as a status, not as prose.
 * Plain-text call sites (footers, story captions) override the box and hide the dot via `[&>[data-dot]]:hidden`.
 * `plain` drops the chip for small muted text: for a content-specific distinction inside a card ("Modelled, not
 * measured"), where the top bar's one "Simulated data" chip already says the data is synthetic.
 */
export function HonestyLabel({
  kind,
  children,
  className,
  plain = false,
}: {
  kind: HonestyKind;
  children?: React.ReactNode;
  className?: string;
  plain?: boolean;
}) {
  if (plain)
    return (
      <span data-honesty={kind} className={cn("text-small text-muted", className)}>
        {children ?? HONESTY[kind]}
      </span>
    );
  return (
    <span data-honesty={kind} className={cn(CHIP_BOX, className)}>
      <span aria-hidden data-dot className={dotClass(kind)} />
      {children ?? HONESTY[kind]}
    </span>
  );
}
