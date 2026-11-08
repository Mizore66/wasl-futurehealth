import { cx } from "@/lib/cx";
import { HONESTY, type HonestyKind } from "@/lib/honesty";

export const CHIP_BOX =
  "inline-flex h-6 items-center gap-1.5 self-start whitespace-nowrap rounded-chip bg-raised px-2 text-small font-medium text-ink-soft";

export const dotClass = (kind: HonestyKind) =>
  cx(
    "size-1.5 flex-none rounded-full",
    kind === "built" || kind === "published" ? "bg-positive" : kind === "designed" ? "border border-current" : "bg-warning",
  );

/**
 * The chip without class merging, for client components (the app sidebar). `compact` swaps the horizontal padding
 * instead of overriding it, which is the one override the sidebar needs. Server pages use HonestyLabel.
 */
export function HonestyChip({ kind, children, compact = false }: { kind: HonestyKind; children?: React.ReactNode; compact?: boolean }) {
  return (
    <span data-honesty={kind} className={compact ? CHIP_BOX.replace("px-2", "px-1.5") : CHIP_BOX}>
      <span aria-hidden data-dot className={dotClass(kind)} />
      {children ?? HONESTY[kind]}
    </span>
  );
}
