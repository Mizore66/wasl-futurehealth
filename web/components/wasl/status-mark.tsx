import { cn } from "@/lib/utils";

export const STATUS_WORDS = {
  queued: "Queued",
  working: "Working",
  done: "Done",
  held: "Held",
  failed: "Failed",
} as const;

export type Status = keyof typeof STATUS_WORDS;

function Shape({ status }: { status: Status }) {
  const common = { width: 10, height: 10, viewBox: "0 0 10 10", "aria-hidden": true } as const;
  switch (status) {
    case "queued":
      return (
        <svg {...common}>
          <circle cx="5" cy="5" r="4" fill="none" stroke="currentColor" strokeWidth="1.5" />
        </svg>
      );
    case "working":
      return (
        <svg {...common} className="wasl-pulse">
          <circle cx="5" cy="5" r="4" fill="none" stroke="currentColor" strokeWidth="1.5" />
          <circle cx="5" cy="5" r="1.75" fill="currentColor" />
        </svg>
      );
    case "done":
      return (
        <svg {...common}>
          <circle cx="5" cy="5" r="4.5" fill="currentColor" />
        </svg>
      );
    case "held":
      return (
        <svg {...common}>
          <rect x="1" y="1" width="8" height="8" fill="currentColor" />
        </svg>
      );
    case "failed":
      return (
        <svg {...common}>
          <path d="M1.5 1.5l7 7M8.5 1.5l-7 7" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" />
        </svg>
      );
  }
}

/** Status as word plus shape, never colour alone. */
export function StatusMark({
  status,
  label,
  className,
}: {
  status: Status;
  /** Overrides the default word, e.g. "Closed with evidence". */
  label?: string;
  className?: string;
}) {
  return (
    <span
      data-status={status}
      className={cn(
        "inline-flex items-center gap-1.5 text-small",
        status === "failed" ? "text-negative" : status === "queued" ? "text-muted" : "text-ink",
        className,
      )}
    >
      <Shape status={status} />
      <span>{label ?? STATUS_WORDS[status]}</span>
    </span>
  );
}
