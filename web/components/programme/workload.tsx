import Link from "next/link";
import { ArrowRight } from "lucide-react";
import { CATEGORICAL, CHROME } from "@/components/charts/palette";
import { cn } from "@/lib/utils";
import { FINISHED_MEANS, STATUS_PART, type ProgrammeRow } from "./model";

const n = (v: number) => v.toLocaleString("en-US");

/** Case-status fills from the shared palette: ochre overdue, a neutral for still open, jade finished; the label ink each needs. */
const TONE = {
  overdue: { fill: CATEGORICAL[1], ink: "text-[#111]" },
  open: { fill: CHROME.grid, ink: "text-ink" },
  closed: { fill: CATEGORICAL[0], ink: "text-white" },
} as const;

/** A whole-number share, as the block prints it. */
const share = (v: number, total: number) => `${Math.round((100 * v) / total)}%`;

/**
 * One programme's cases under the target rail: the total, then overdue, still open and finished as one segmented bar
 * with each count printed where it fits, and the way into today's list. "Still open" is cases minus finished minus overdue; when that is
 * negative the run's counts overlap and the row says so instead of a number.
 */
export function Workload({ row, note }: { row: ProgrammeRow | undefined; note?: React.ReactNode }) {
  const parts = row
    ? ([
        ["overdue", STATUS_PART.overdue, row.overdue],
        ["open", STATUS_PART.open, row.open],
        ["closed", STATUS_PART.closed, row.closed],
      ] as const)
    : [];
  const total = parts.reduce((t, [, , c]) => t + (c ?? 0), 0);

  return (
    <section
      aria-label="Case status"
      data-testid="programme-workload"
      className="grid border-t border-rule bg-limestone-100 @min-[701px]:grid-cols-[minmax(180px,.85fr)_minmax(0,2.5fr)]"
    >
      <div className="flex min-w-0 items-center justify-between gap-3 px-4 py-2.5 @min-[701px]:flex-col @min-[701px]:items-start @min-[701px]:justify-center @min-[701px]:border-r @min-[701px]:border-rule @min-[701px]:px-[clamp(19px,2.65cqi,42px)] @min-[701px]:py-[clamp(16px,2.2cqi,31px)]">
        <p className="m-0 whitespace-nowrap font-medium text-muted">
          {row ? (
            <>
              <strong data-testid="programme-cases" className="text-[clamp(24px,3.45cqi,43px)] font-semibold leading-[1.05] tracking-[-0.04em] text-jade-900 tabular-nums">
                {n(row.cases)}
              </strong>{" "}
              cases
            </>
          ) : (
            "Case counts not available"
          )}
        </p>
        <Link
          href="/app/today"
          className="inline-flex min-h-11 flex-none items-center gap-1.5 rounded-control bg-cta px-4 font-medium text-white no-underline hover:bg-jade-800 hover:text-white"
        >
          Open Today
          <ArrowRight aria-hidden className="size-4" strokeWidth={1.75} />
        </Link>
      </div>

      {row && total > 0 && (
        <div className="flex min-w-0 flex-col justify-center gap-[11px] px-4 pb-4 @min-[701px]:gap-3.5 @min-[701px]:px-[clamp(19px,2.65cqi,42px)] @min-[701px]:py-[clamp(16px,2.2cqi,31px)]">
          <p className="m-0 text-body font-semibold text-ink">
            {n(row.overdue)} of {n(row.cases)} {row.name.toLowerCase()} cases are overdue
          </p>
          <div
            role="img"
            aria-label={`${n(row.cases)} ${row.name.toLowerCase()} cases: ${parts
              .filter(([, , c]) => c)
              .map(([, label, c]) => `${n(c!)} ${label.toLowerCase()} (${share(c!, total)})`)
              .join(", ")}.`}
            className="@container flex h-7 gap-0.5"
          >
            {parts.map(([key, label, count]) => {
              if (!count) return null;
              const part = count / total;
              return (
                <span
                  key={key}
                  data-case-tone={key}
                  data-testid={`programme-${key}`}
                  title={`${label}: ${n(count)}`}
                  className={cn("flex min-w-0 shrink items-center overflow-visible rounded-[4px] px-2 text-small font-semibold tabular-nums", TONE[key].ink)}
                  style={{ flexBasis: `${100 * part}%`, backgroundColor: TONE[key].fill }}
                >
                  {/* A label only where it fits: the full figure on wide blocks, the count on mid ones, nothing on slivers. */}
                  {part >= 0.27 ? (
                    <span className="whitespace-nowrap">
                      {n(count)} · {share(count, total)}
                    </span>
                  ) : part >= 0.12 ? (
                    <span className="whitespace-nowrap">{n(count)}</span>
                  ) : part >= 0.06 ? (
                    <span className="hidden whitespace-nowrap @min-[480px]:inline">{n(count)}</span>
                  ) : null}
                </span>
              );
            })}
          </div>
          <ul className="m-0 flex list-none flex-wrap gap-x-4 gap-y-1 p-0 text-small text-ink-soft">
            {parts.map(([key, label, count]) =>
              count === null ? null : (
                <li key={key} className="flex items-center gap-1.5">
                  <span aria-hidden className="block size-2.5 flex-none rounded-[2px]" style={{ backgroundColor: TONE[key].fill, boxShadow: key === "open" ? "inset 0 0 0 1px var(--color-control)" : undefined }} />
                  {key === "closed" ? `${label} (${FINISHED_MEANS})` : label}
                </li>
              ),
            )}
          </ul>
          {note}
        </div>
      )}
    </section>
  );
}
