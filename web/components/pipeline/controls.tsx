"use client";

import { Button } from "@/components/ui/button";
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
 * One control row: Pause / Play and Step first, so they never shift, then where the replay is and how fast it plays.
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
    <div role="group" aria-label="Replay controls" className="flex min-w-0 flex-col gap-3">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <div className="flex flex-none items-center gap-2">
          <Button type="button" variant="cta" size="touch" onClick={onToggle} className="min-w-[104px]">
            <svg aria-hidden width="12" height="12" viewBox="0 0 12 12" fill="currentColor" className="size-3">
              <path d={paused ? PLAY : PAUSE} />
            </svg>
            {paused ? "Play" : "Pause"}
          </Button>
          <Button type="button" variant="quiet" size="touch" onClick={onStep} className="min-w-[104px] border border-limestone-600 font-medium">
            Step
          </Button>
        </div>
        <div className="flex min-w-0 flex-[1_1_240px] flex-col">
          <span className="text-body font-medium tabular-nums text-ink" data-testid="replay-position">
            {position}
            {paused ? " · paused" : ""}
          </span>
          <span className="text-small text-ink-soft">{speed}</span>
        </div>
      </div>
      {reduceMotion && (
        <p data-testid="reduced-motion-note" className="m-0 max-w-[72ch] text-small text-ink-soft">
          Reduced motion is on, so the replay is paused. Press Play or Step to move it.
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
    <div role="group" aria-label={label} className="flex flex-wrap gap-2">
      {options.map((o) => {
        const on = o.value === value;
        return (
          <Button
            key={o.value}
            type="button"
            variant="chip"
            size="touch"
            aria-pressed={on}
            onClick={() => onPick(o.value)}
            className={cn("min-w-11 whitespace-nowrap px-3", on ? "border-jade-700 bg-active font-medium text-jade-900 hover:bg-active" : "border-line text-ink-soft")}
          >
            {o.label}
          </Button>
        );
      })}
    </div>
  );
}
