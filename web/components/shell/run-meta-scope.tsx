"use client";

import { usePathname } from "next/navigation";

/**
 * The strip describes the main dashboard run. The Agent review page shows a separate experimental run, so
 * there the strip says so instead of presenting the main run as that page's source.
 */
export function RunMetaScope({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  if (pathname !== "/app/agent-review" && !pathname.startsWith("/app/agent-review/")) return <>{children}</>;
  return (
    <section aria-label="Run metadata" className="mb-6 rounded-card border border-line bg-surface px-3 py-2 text-small text-ink-soft">
      The main dashboard run is not the source of this page. Agent review shows a separate experimental run, described in the banner below.
    </section>
  );
}
