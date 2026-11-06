import { HonestyLabel } from "@/components/wasl/honesty-label";
import { apiMode, getDashboard } from "@/lib/api/source";

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
 * Which run, policy and reader model the numbers on this screen come from, and whether the data is
 * live or a published snapshot. Built: reads `meta` from the same source every screen uses.
 */
export async function RunMeta() {
  const source = SOURCE[apiMode()];
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

  const rows: [string, string][] = [
    ["Data source", source],
    ["Policy", meta.policy],
    ["Reader model", meta.reader_model],
    ["Loaded", meta.loaded_at],
    ["API", meta.api],
  ];

  return (
    <section
      aria-label="Run metadata"
      className="mb-6 rounded-card border border-line bg-surface text-small"
    >
      <details className="group">
        <summary className="flex min-h-11 cursor-pointer list-none items-center gap-3 rounded-card px-3 md:min-h-9 [&::-webkit-details-marker]:hidden">
          <HonestyLabel kind="simulated">
            data_mode: {meta.data_mode}
          </HonestyLabel>
          <span className="min-w-0 flex-1 truncate text-ink-soft">
            <span className="text-muted">Run </span>
            <span className="font-mono">{meta.run}</span>
          </span>
          <span className="flex-none text-muted group-open:hidden">
            Details
          </span>
          <span className="hidden flex-none text-muted group-open:inline">
            Hide
          </span>
        </summary>
        <dl className="m-0 grid gap-x-6 gap-y-2 border-t border-line px-3 py-3 sm:grid-cols-[max-content_1fr]">
          {rows.map(([k, v]) => (
            <div key={k} className="contents">
              <dt className="text-muted">{k}</dt>
              <dd className="m-0 break-all font-mono text-ink-soft">{v}</dd>
            </div>
          ))}
          <dt className="text-muted">Note</dt>
          <dd className="m-0 text-ink-soft">{meta.note}</dd>
        </dl>
      </details>
    </section>
  );
}
