import { cn } from "@/lib/utils";

/** A figure with its label and where it came from. `lead` is the screen's one 36/700 figure. */
export function Stat({
  value,
  label,
  source,
  lead = false,
  hue = false,
  className,
}: {
  value: React.ReactNode;
  label: string;
  /** Provenance line, e.g. "pipeline/results/final/SUMMARY.md · held-out v2 noise". */
  source: string;
  lead?: boolean;
  hue?: boolean;
  className?: string;
}) {
  return (
    <div className={cn("flex flex-col", className)}>
      <span className="text-small text-muted">{label}</span>
      <span
        className={cn(
          "mt-1 tabular-nums tracking-[-0.02em]",
          lead ? "text-lead font-bold" : "text-[30px] leading-[38px] font-medium",
          hue ? "text-hue" : "text-ink",
        )}
      >
        {value}
      </span>
      <span className="mt-1 text-small text-muted">{source}</span>
    </div>
  );
}
