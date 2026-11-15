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
 * the other choice applied.
 */
export function TodayScope({ selection, options, count }: { selection: TodaySelection; options: Record<keyof TodaySelection, Option[]>; count: number }) {
  return (
    <div className="mb-6" data-testid="today-scope">
      <form action="/app/today" method="get" className="flex flex-wrap items-end gap-3">
        {FIELDS.map(([name, label, all]) => (
          <label key={name} className="flex min-w-0 flex-[1_1_200px] flex-col gap-1 text-small text-muted sm:flex-none">
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
      {count === 0 && (
        <p className="m-0 mt-3 text-body" role="status" data-testid="today-scope-empty">
          No open cases for {scopeWords(selection)}. Choose another programme or provider.
        </p>
      )}
    </div>
  );
}
