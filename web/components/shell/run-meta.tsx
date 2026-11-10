import { ChevronDown } from "lucide-react";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { apiMode, getDashboard } from "@/lib/api/source";
import { runStripCopy } from "./run-meta-copy";

const SOURCE = {
  live: "Live Wasl API",
  fixtures: "Published snapshot of the API",
} as const;

/** Reserves the strip's height while the dashboard loads, so the page does not jump. */
export function RunMetaFallback() {
  return (
    <div
      aria-hidden
      className="mb-6 h-11 rounded-card border border-line bg-surface md:h-9"
    />
  );
}

/**
 * Whether the numbers on this screen are simulated and when they were taken, in plain words (issue #50). The run, policy,
 * reader model and API version behind them sit in the expanded technical panel, one click away for reproducibility.
 * Built: reads `meta` from the same source every screen uses.
 */
export async function RunMeta() {
  const mode = apiMode();
  const source = SOURCE[mode];
  let meta: Awaited<ReturnType<typeof getDashboard>>["meta"];
  try {
    meta = (await getDashboard()).meta;
  } catch {
    return (
      <section
        aria-label="Run metadata"
        className="mb-6 rounded-card border border-line bg-surface px-3 py-2 text-small text-muted"
      >
        Run details are unavailable. Source: {source}.
      </section>
    );
  }

  const copy = runStripCopy(meta, mode);
  const rows: [string, string][] = [
    ["Data source", source],
    ["Run", meta.run],
    ["Policy", meta.policy],
    ["Reader model", meta.reader_model],
    ["API", meta.api],
  ];

  return (
    <section
      aria-label="Run metadata"
      className="mb-6 rounded-card border border-line bg-surface text-small"
    >
      <details className="group">
        <summary className="flex min-h-11 cursor-pointer list-none items-center gap-3 rounded-card px-3 py-1 md:min-h-9 [&::-webkit-details-marker]:hidden">
          <HonestyLabel kind="simulated" className="self-center">
            {copy.data}
          </HonestyLabel>
          {/* Wraps to two lines at phone width rather than cutting the time off. */}
          <time
            dateTime={meta.loaded_at}
            className="min-w-0 flex-1 leading-tight text-ink-soft"
          >
            {copy.when}
          </time>
          <span className="flex flex-none items-center gap-1 text-muted">
            <span className="max-sm:sr-only group-open:hidden">Details</span>
            <span className="hidden max-sm:sr-only group-open:inline">
              Hide
            </span>
            <ChevronDown
              aria-hidden
              className="size-4 group-open:rotate-180 motion-safe:transition-transform"
            />
          </span>
        </summary>
        <div className="border-t border-line px-3 py-3">
          <p className="m-0 text-ink-soft">{meta.note}</p>
          <p className="m-0 mt-3 font-medium text-muted">Technical detail</p>
          <dl className="m-0 mt-2 grid gap-x-6 gap-y-2 sm:grid-cols-[max-content_1fr]">
            {rows.map(([k, v]) => (
              <div key={k} className="contents">
                <dt className="text-muted">{k}</dt>
                <dd className="m-0 break-all font-mono text-ink-soft">{v}</dd>
              </div>
            ))}
          </dl>
        </div>
      </details>
    </section>
  );
}
