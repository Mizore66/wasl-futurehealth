"use client";

import { useSearchParams } from "next/navigation";
import { isTodayQueue } from "@/components/shell/today-counts";
import { FilterMenu } from "@/components/ui/filters/filter-menu";
import { FilterSheet } from "@/components/ui/filters/filter-sheet";
import { Toolbar } from "@/components/ui/filters/toolbar";
import type { FilterSpec } from "@/components/ui/filters/types";

function withQueue(href: string, queue: string): string {
  const [path, query = ""] = href.split("?");
  const params = new URLSearchParams(query);
  params.set("queue", queue);
  return `${path}?${params}`;
}

/**
 * Today's scope menus. The sidebar switches queue in the browser without a server render, so the server-built links
 * may name a stale queue: each one is re-pointed at the queue in the live URL.
 */
export function ScopeMenus({ menus, clearHref, total }: { menus: FilterSpec[]; clearHref: string; total: number }) {
  const live = useSearchParams()?.get("queue");
  const q = (href: string) => (isTodayQueue(live) ? withQueue(href, live) : href);
  const filters = menus.map((m) => ({ ...m, clearHref: q(m.clearHref), options: m.options.map((o) => ({ ...o, href: q(o.href) })) }));

  return (
    <Toolbar label="Today scope" data-testid="today-scope" className="flex-row flex-wrap items-center justify-end gap-2">
      <FilterSheet filters={filters} clearHref={q(clearHref)} total={total} one="open case" className="sm:hidden" />
      {filters.map((m) => (
        <FilterMenu key={m.label} {...m} className={m.value ? undefined : "max-sm:hidden"} />
      ))}
    </Toolbar>
  );
}
