import { cn } from "@/lib/utils";

export type Tone = "positive" | "negative" | "warning" | "muted";

const PATH = { tick: "M3 8.5l3 3 7-7", cross: "M4 4l8 8M12 4l-8 8", dash: "M4 8h8" } as const;
export type Shape = keyof typeof PATH;

const TONE: Record<Tone, string> = {
  positive: "text-positive",
  negative: "text-negative",
  warning: "text-warning",
  muted: "text-muted",
};

/** A word with its shape: status is never colour alone. */
export function Mark({ shape, tone, children, className }: { shape: Shape; tone: Tone; children: React.ReactNode; className?: string }) {
  return (
    <span className={cn("inline-flex items-center gap-1.5 whitespace-nowrap", className)}>
      <svg aria-hidden width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className={cn("flex-none", TONE[tone])}>
        <path d={PATH[shape]} />
      </svg>
      <span>{children}</span>
    </span>
  );
}

const PILL: Record<Tone, string> = {
  positive: "bg-pill-positive-bg text-pill-positive-ink",
  negative: "bg-pill-negative-bg text-pill-negative-ink",
  warning: "bg-pill-warning-bg text-pill-warning-ink",
  muted: "bg-pill-neutral-bg text-pill-neutral-ink",
};

export function Pill({ tone, shape, children }: { tone: Tone; shape?: Shape; children: React.ReactNode }) {
  return (
    <span className={cn("inline-flex h-[22px] items-center gap-1.5 whitespace-nowrap rounded-chip px-2 text-small font-medium", PILL[tone])}>
      {shape && (
        <svg aria-hidden width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="flex-none">
          <path d={PATH[shape]} />
        </svg>
      )}
      {children}
    </span>
  );
}
