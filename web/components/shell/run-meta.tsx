"use client";

import { createContext, Suspense, use } from "react";
import { ChevronDown } from "lucide-react";
import { DismissableDetails } from "@/components/ui/dismissable-details";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import type { RunInfo } from "./run-info";

const RunInfoContext = createContext<Promise<RunInfo> | null>(null);

/** The app layout starts the read; each page header's notice waits on the same promise. */
export function RunInfoProvider({ value, children }: { value: Promise<RunInfo>; children: React.ReactNode }) {
  return <RunInfoContext value={value}>{children}</RunInfoContext>;
}

// Not positioned: the drop-down anchors to the header's aside group, whose right edge is the header's.
const FRAME = "text-small";

/**
 * Whether the numbers on this screen are simulated and when they were taken, in plain words (issue #50), at the end of
 * the page header. The run, policy, reader model and API version open in a drop-down, one click away for reproducibility.
 */
export function RunMeta() {
  return (
    <Suspense fallback={<div aria-hidden className="h-11 w-96 max-w-full" />}>
      <Notice />
    </Suspense>
  );
}

function Notice() {
  const read = use(RunInfoContext);
  if (!read) return null;
  const info = use(read);
  if (!info.ok) {
    return (
      <section aria-label="Run metadata" className={`${FRAME} px-1 text-muted`}>
        Run details are unavailable. Source: {info.source}.
      </section>
    );
  }
  return (
    <section aria-label="Run metadata" className={`${FRAME} min-w-0 max-w-full`}>
      <DismissableDetails className="group">
        <summary className="flex min-h-11 cursor-pointer list-none items-center gap-3 rounded-control px-2 py-1 hover:bg-jade-50/70 [&::-webkit-details-marker]:hidden">
          <HonestyLabel kind="simulated" className="self-center">
            {info.data}
          </HonestyLabel>
          {/* Wraps to two lines at phone width rather than cutting the time off. */}
          <time dateTime={info.loadedAt} className="min-w-0 flex-1 leading-tight text-ink-soft">
            {info.when}
          </time>
          <span className="flex flex-none items-center gap-1 text-muted">
            <span className="max-sm:sr-only group-open:hidden">Details</span>
            <span className="hidden max-sm:sr-only group-open:inline">Hide</span>
            <ChevronDown aria-hidden className="size-4 group-open:rotate-180 motion-safe:transition-transform" />
          </span>
        </summary>
        <div className="mt-2 rounded-card border border-line bg-surface px-3 py-3 sm:absolute sm:right-0 sm:top-full sm:z-30 sm:mt-1 sm:w-[min(480px,calc(100vw-48px))] sm:shadow-[0_6px_20px_rgb(32_55_46/0.12)]">
          <p className="m-0 text-ink-soft">{info.note}</p>
          <p className="m-0 mt-3 font-medium text-muted">Technical detail</p>
          <dl className="m-0 mt-2 grid gap-x-6 gap-y-2 sm:grid-cols-[max-content_1fr]">
            {info.rows.map(([k, v]) => (
              <div key={k} className="contents">
                <dt className="text-muted">{k}</dt>
                <dd className="m-0 break-all font-mono text-ink-soft">{v}</dd>
              </div>
            ))}
          </dl>
        </div>
      </DismissableDetails>
    </section>
  );
}
