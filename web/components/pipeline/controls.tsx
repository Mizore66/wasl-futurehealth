"use client";

import { cn } from "@/lib/utils";
import type { Mark } from "./flow";

/** Status as shape: hollow queued, pulsing working, filled done, square held, cross failed. */
export function MarkShape({ mark, hue = false, className }: { mark: Mark; hue?: boolean; className?: string }) {
  const color = hue ? "var(--color-hue)" : "currentColor";
  return (
    <svg width="8" height="8" viewBox="0 0 8 8" aria-hidden className={cn("flex-none", mark === "working" && "wasl-pulse", className)}>
      {mark === "queued" && <circle cx="4" cy="4" r="3.25" fill="var(--color-surface)" stroke={color} strokeWidth="1.5" />}
      {mark === "working" && <circle cx="4" cy="4" r="3.25" fill="var(--color-surface)" stroke={color} strokeWidth="1.5" />}
      {mark === "done" && <circle cx="4" cy="4" r="4" fill={color} />}
      {mark === "held" && <rect x="0.5" y="0.5" width="7" height="7" rx="1" fill={color} />}
      {mark === "failed" && <path d="M1 1l6 6M7 1l-6 6" stroke="var(--color-negative)" strokeWidth="1.6" strokeLinecap="round" />}
    </svg>
  );
}

const PAUSE = "M2.5 1.5h2.5v9H2.5zM7 1.5h2.5v9H7z";
const PLAY = "M3 1.5v9l7.5-4.5z";

/**
 * Pause / Play, Step and a one-line note on where the replay is and how fast it plays.
 * These drive the recorded replay only; they are labelled as replay controls and are 44px tall for touch.
 */
export function PlaybackControls({
  paused,
  onToggle,
  onStep,
  position,
  speed,
  reduceMotion = false,
  children,
}: {
  paused: boolean;
  onToggle: () => void;
  onStep: () => void;
  position: string;
  speed: string;
  reduceMotion?: boolean;
  children?: React.ReactNode;
}) {
  return (
    <div role="group" aria-label="Replay controls" className="flex w-full min-w-0 flex-col gap-2 sm:w-auto sm:items-end">
      <div className="flex flex-wrap items-center gap-3 sm:justify-end">
        <span className="flex min-w-0 flex-col sm:items-end">
          <span className="text-body font-medium" data-testid="replay-position">
            {position}
            {paused ? " · paused" : ""}
          </span>
          <span className="text-small text-muted">{speed}</span>
        </span>
        <div className="flex items-center gap-3">
          <button
            type="button"
            onClick={onToggle}
            className="inline-flex h-11 min-w-[104px] flex-none cursor-pointer items-center justify-center gap-2 rounded-control bg-cta px-4 text-body font-medium text-white"
          >
            <svg aria-hidden width="12" height="12" viewBox="0 0 12 12" fill="currentColor">
              <path d={paused ? PLAY : PAUSE} />
            </svg>
            {paused ? "Play" : "Pause"}
          </button>
          <button
            type="button"
            onClick={onStep}
            className="h-11 min-w-[72px] flex-none cursor-pointer rounded-control bg-active px-4 text-body font-medium text-ink"
          >
            Step
          </button>
        </div>
      </div>
      {reduceMotion && (
        <p data-testid="reduced-motion-note" className="m-0 text-small text-muted sm:text-right">
          Reduced motion is on, so the replay waits at a settled frame. Press Play or Step to move it.
        </p>
      )}
      {children}
    </div>
  );
}

/** Report or case picker under the controls. */
export function Picker<T extends string | number>({
  label,
  options,
  value,
  onPick,
}: {
  label: string;
  options: { value: T; label: string }[];
  value: T;
  onPick: (v: T) => void;
}) {
  return (
    <div role="group" aria-label={label} className="flex flex-wrap gap-1">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          aria-pressed={o.value === value}
          onClick={() => onPick(o.value)}
          className={cn(
            "min-h-11 min-w-11 cursor-pointer whitespace-nowrap rounded-chip px-3 text-small",
            o.value === value ? "bg-active font-medium text-ink" : "bg-transparent text-muted",
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}
