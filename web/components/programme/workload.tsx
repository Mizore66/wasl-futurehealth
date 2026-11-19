import Link from "next/link";
import { ArrowRight } from "lucide-react";
import type { ProgrammeRow } from "./model";

const n = (v: number) => v.toLocaleString("en-US");

/** The approved case-status tones: ochre for overdue, sand for other open, jade for closed. */
const TONE = { overdue: "bg-[#a9824b]", open: "bg-[#c4b088]", closed: "bg-jade-600" } as const;

/**
 * One programme's cases under the target rail: the total, then overdue, other open and closed as a proportion bar
 * with their counts, and the way into today's list. "Other open" is cases minus closed minus overdue; when that is
 * negative the run's counts overlap and the row says so instead of a number.
 */
export function Workload({ row, note }: { row: ProgrammeRow | undefined; note?: React.ReactNode }) {
  const parts = row
    ? ([
        ["overdue", "Overdue", row.overdue],
        ["open", "Other open", row.open],
        ["closed", "Closed", row.closed],
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
          <div aria-hidden className="flex h-[clamp(11px,1.5cqi,21px)] gap-0.5 overflow-hidden rounded-[4px] bg-surface">
            {parts.map(([key, , count]) =>
              count ? <span key={key} data-case-tone={key} className={`block min-w-0 shrink ${TONE[key]}`} style={{ flexBasis: `${(100 * count) / total}%` }} /> : null,
            )}
          </div>
          <dl className="m-0 grid grid-cols-3 gap-x-3.5 gap-y-2">
            {parts.map(([key, label, count]) => (
              <div key={key} data-testid={`programme-${key}`} className="grid min-w-0 gap-[3px]">
                <dt className="flex items-center gap-1 text-[11px] font-semibold leading-[1.3] text-muted @min-[701px]:gap-1.5 @min-[701px]:text-small">
                  <span aria-hidden className={`inline-block size-[7px] flex-none rounded-full @min-[701px]:size-2 ${TONE[key]}`} />
                  {label}
                </dt>
                <dd className="m-0 text-[clamp(21px,2.85cqi,34px)] font-semibold leading-[1.15] tracking-[-0.025em] text-ink tabular-nums">
                  {count === null ? <span className="text-small font-medium tracking-normal text-muted">Not available</span> : n(count)}
                </dd>
              </div>
            ))}
          </dl>
          {note}
        </div>
      )}
    </section>
  );
}
