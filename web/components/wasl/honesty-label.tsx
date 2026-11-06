import { cn } from "@/lib/utils";
import { HONESTY, type HonestyKind } from "@/lib/honesty";

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
    <span
      data-honesty={kind}
      className={cn(
        "inline-flex h-6 items-center gap-1.5 self-start whitespace-nowrap rounded-chip bg-raised px-2 text-small font-medium text-ink-soft",
        className,
      )}
    >
      <span
        aria-hidden
        data-dot
        className={cn(
          "size-1.5 flex-none rounded-full",
          kind === "built" || kind === "published" ? "bg-positive" : kind === "designed" ? "border border-current" : "bg-warning",
        )}
      />
      {children ?? HONESTY[kind]}
    </span>
  );
}
