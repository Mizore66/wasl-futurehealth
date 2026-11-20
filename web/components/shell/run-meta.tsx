"use client";

import { createContext, Suspense, use } from "react";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import type { RunInfo } from "./run-info";

const RunInfoContext = createContext<Promise<RunInfo> | null>(null);

/** The app layout starts the read; the shell's one data notice waits on the same promise. */
export function RunInfoProvider({ value, children }: { value: Promise<RunInfo>; children: React.ReactNode }) {
  return <RunInfoContext value={value}>{children}</RunInfoContext>;
}

/**
 * The one global notice of how real the numbers are (issue #50): a single "Simulated data" chip in the top bar (the
 * phone header below md). Pages carry no generic synthetic label of their own. The run, policy, reader model and API
 * version are on the Evidence page's verify section.
 */
export function RunMeta() {
  return (
    <Suspense fallback={<div aria-hidden className="h-6 w-28" />}>
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
      <section aria-label="Run metadata" className="text-small text-muted">
        Run details are unavailable. Source: {info.source}.
      </section>
    );
  }
  return (
    <section aria-label="Run metadata" className="flex min-w-0 items-center">
      <HonestyLabel kind="simulated">{info.data}</HonestyLabel>
    </section>
  );
}
