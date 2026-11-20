import type { Option } from "@/components/patients/facets";
import { Button } from "@/components/ui/button";
import type { FilterSpec } from "@/components/ui/filters/types";
import type { TodaySelection } from "./filters";
import { PROGRAMME_NAMES, providerLabel } from "./model";
import { ScopeMenus } from "./scope-menus";

export function scopeWords(s: TodaySelection): string {
  return `${s.programme ? PROGRAMME_NAMES[s.programme] : "All programmes"} · ${s.provider ? providerLabel(s.provider) : "all providers"}`;
}

const FIELDS = [
  ["programme", "Programme", "All programmes"],
  ["provider", "Provider", "All providers"],
] as const;

/** Both keys always, so an empty value reads as "all" and never falls back to Today's default scope. */
function scopeHref(s: TodaySelection, queue: string): string {
  return `/app/today?${new URLSearchParams({ programme: s.programme ?? "", provider: s.provider ?? "", queue })}`;
}

/**
 * Programme and provider as filter menus. The choice lives in the URL, so it survives reload, and a new scope keeps
 * the queue being viewed. Counts are open cases with the other choice applied. Without JavaScript the menus cannot
 * open, so the same choices come as a GET form.
 */
export function TodayScope({
  selection,
  options,
  count,
  queue,
}: {
  selection: TodaySelection;
  options: Record<keyof TodaySelection, Option[]>;
  count: number;
  /** The queue being viewed: a new scope keeps it. */
  queue: string;
}) {
  const menus: FilterSpec[] = FIELDS.map(([name, label]) => ({
    label,
    value: selection[name],
    options: options[name]
      .filter((o) => o.value !== "")
      .map((o) => ({ ...o, href: scopeHref({ ...selection, [name]: o.value }, queue) })),
    clearHref: scopeHref({ ...selection, [name]: undefined }, queue),
  }));

  return (
    <>
      <ScopeMenus menus={menus} clearHref={scopeHref({}, queue)} total={count} />
      <noscript>
        <form action="/app/today" method="get" className="flex w-[min(460px,calc(100vw-48px))] flex-wrap items-end gap-3">
          <input type="hidden" name="queue" value={queue} />
          {FIELDS.map(([name, label, all]) => (
            <label key={name} className="flex min-w-0 flex-[1_1_180px] flex-col gap-1 text-small text-muted">
              {label}
              <select name={name} defaultValue={selection[name] ?? ""} className="h-11 w-full min-w-0 rounded-control border border-input bg-surface px-2 text-body text-ink">
                <option value="">{all}</option>
                {options[name].filter((o) => o.value !== "").map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label} ({o.count})
                  </option>
                ))}
              </select>
            </label>
          ))}
          <Button variant="quiet" size="touch">
            Show queue
          </Button>
        </form>
      </noscript>
    </>
  );
}
