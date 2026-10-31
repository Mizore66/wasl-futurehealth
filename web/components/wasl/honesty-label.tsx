import { cn } from "@/lib/utils";
import { HONESTY, type HonestyKind } from "@/lib/honesty";

/** A data-mode chip: how true the thing beside it is. Children override the default word. */
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
        "inline-flex h-[22px] items-center self-start whitespace-nowrap rounded-chip bg-raised px-2 text-small text-ink-soft",
        className,
      )}
    >
      {children ?? HONESTY[kind]}
    </span>
  );
}
