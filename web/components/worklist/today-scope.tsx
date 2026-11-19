import { ChevronDown, SlidersHorizontal } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { Option } from "@/components/patients/facets";
import type { TodaySelection } from "./filters";
import { PROGRAMME_NAMES, providerLabel } from "./model";

export function scopeWords(s: TodaySelection): string {
  return `${s.programme ? PROGRAMME_NAMES[s.programme] : "All programmes"} · ${s.provider ? providerLabel(s.provider) : "all providers"}`;
}

const FIELDS = [
  ["programme", "Programme", "All programmes"],
  ["provider", "Provider", "All providers"],
] as const;

/**
 * A GET form, so the choice lives in the URL, survives reload and works without hydration. Counts are open cases with
 * the other choice applied. It sits folded in the page header; the header's sub-line already names the current scope.
 */
export function TodayScope({ selection, options, count }: { selection: TodaySelection; options: Record<keyof TodaySelection, Option[]>; count: number }) {
  return (
    <details className="group relative" data-testid="today-scope">
      <summary className="flex min-h-11 cursor-pointer list-none items-center gap-1.5 rounded-control px-2 text-small font-medium text-jade-700 hover:bg-jade-50 [&::-webkit-details-marker]:hidden">
        <SlidersHorizontal aria-hidden className="size-4" />
        Change scope
        <span className="sr-only">
          {" "}
          ({count} open {count === 1 ? "case" : "cases"})
        </span>
        <ChevronDown aria-hidden className="size-3.5 transition-transform group-open:rotate-180" />
      </summary>
      <form
        action="/app/today"
        method="get"
        className="mt-2 flex w-[min(460px,calc(100vw-48px))] flex-wrap items-end gap-3 rounded-card border border-line bg-surface p-3 sm:absolute sm:right-0 sm:z-20 sm:shadow-[0_6px_20px_rgb(32_55_46/0.12)]"
      >
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
    </details>
  );
}
