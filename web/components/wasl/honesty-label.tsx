import { cn } from "@/lib/utils";
import { HONESTY, type HonestyKind } from "@/lib/honesty";

const CHIP_BOX =
  "inline-flex h-6 items-center gap-1.5 self-start whitespace-nowrap rounded-chip bg-raised px-2 text-small font-medium text-ink-soft";

/** An honesty label read as plain text: no chip box, no status dot. */
export const HONESTY_PLAIN = "inline h-auto whitespace-normal bg-transparent px-0 text-small font-normal text-muted [&>[data-dot]]:hidden";

const dotClass = (kind: HonestyKind) =>
  cn(
    "size-1.5 flex-none rounded-full",
    kind === "built" || kind === "published" ? "bg-positive" : kind === "designed" ? "border border-current" : "bg-warning",
  );

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
