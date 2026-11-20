"use client";

import { CATEGORICAL, CHROME } from "@/components/charts/palette";
import { cn } from "@/lib/utils";
import { ChartFigure, Mark } from "./chart-kit";
import { byOverdueShare, FINISHED_MEANS, pct, STATUS_PART, type OnTime, type ProgrammeRow } from "./model";

const n = (v: number) => v.toLocaleString("en-US");

const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

/** "30.7% (408 of 1,331)". */
const onTimeText = (o: { share: number; onTime: number; eligible: number }) => `${pct(o.share)} (${n(o.onTime)} of ${n(o.eligible)})`;

/** Each window's own figure, when the deadline depends on the referral: "14 days: 7.5% (4 of 53); 28 days: …". */
const windowsText = (o: OnTime) => o.windows.map((w) => `${n(w.days)} days: ${onTimeText(w)}`).join("; ");

/** Below this many referrals past their deadline, the on-time dot is hollow and its count is printed beside it. */
export const SMALL_ON_TIME = 20;

/** The status fills, shared with the strip under the target rail. */
const FILLS = { closed: CATEGORICAL[0], overdue: CATEGORICAL[1], open: CHROME.grid } as const;

function segments(r: ProgrammeRow): { key: keyof typeof FILLS; count: number }[] {
  const all = [
    { key: "closed" as const, count: r.closed },
    { key: "overdue" as const, count: r.overdue },
    ...(r.open === null ? [] : [{ key: "open" as const, count: r.open }]),
  ];
  return all.filter((s) => s.count > 0);
}

/** Name | status bar | on-time dot, sharing one grid so every bar and every dot starts on the same line. */
const ROW = "grid grid-cols-[minmax(0,1fr)_minmax(104px,40%)] gap-x-4 sm:grid-cols-[minmax(120px,190px)_minmax(0,1fr)_minmax(150px,220px)]";

/** Finished, overdue and still open cases for each programme beside its on-time share; a row opens the patient list for that programme. */
export function ProgrammeChart({ rows, onTimeMeasured }: { rows: ProgrammeRow[]; onTimeMeasured: boolean }) {
  const showOpen = rows.every((r) => r.open !== null);
  const sorted = byOverdueShare(rows);
  return (
    <ChartFigure
      caption={<>Rows sorted by overdue share. Select a row to open its patients.</>}
      table={{
        caption: "Cases by programme",
        head: ["Programme", "Cases", STATUS_PART.closed, STATUS_PART.overdue, ...(showOpen ? [STATUS_PART.open] : [])],
        rows: sorted.map((r) => [r.name, n(r.cases), n(r.closed), n(r.overdue), ...(showOpen ? [n(r.open ?? 0)] : [])]),
        details: onTimeMeasured
          ? sorted.map((r) =>
              r.onTime
                ? `On time: ${onTimeText(r.onTime)}, ${r.onTime.step} ${r.onTime.within}${r.onTime.windows.length > 1 ? `. By window, ${windowsText(r.onTime)}` : ""}.`
                : "On time: no referral's deadline has passed yet.",
            )
          : undefined,
      }}
    >
      <ul aria-hidden className="m-0 mb-2 flex list-none flex-wrap gap-x-4 gap-y-1 p-0 text-small text-ink-soft">
        {(["closed", "overdue", ...(showOpen ? ["open" as const] : [])] as const).map((k) => (
          <li key={k} className="flex items-center gap-1.5">
            <span className="block size-2.5 flex-none rounded-[2px]" style={{ backgroundColor: FILLS[k], boxShadow: k === "open" ? "inset 0 0 0 1px var(--color-control)" : undefined }} />
            {k === "closed" ? `${STATUS_PART.closed} (${FINISHED_MEANS})` : STATUS_PART[k]}
          </li>
        ))}
        {onTimeMeasured && (
          <>
            <li className="flex items-center gap-1.5">
              <span className="block size-2.5 flex-none rounded-full" style={{ backgroundColor: CATEGORICAL[0] }} />
              On time
            </li>
            <li className="flex items-center gap-1.5">
              <span className="block size-2.5 flex-none rounded-full" style={{ boxShadow: `inset 0 0 0 2px ${CATEGORICAL[0]}` }} />
              On time, fewer than {SMALL_ON_TIME} referrals
            </li>
          </>
        )}
      </ul>
      <div aria-hidden className={cn(ROW, "hidden pb-1 text-small text-muted sm:grid")}>
        <span />
        <span>Share of the programme&apos;s cases</span>
        {onTimeMeasured && <span>On time</span>}
      </div>
      <ul className="m-0 list-none p-0">
        {sorted.map((r) => {
          const segs = segments(r);
          const total = segs.reduce((t, s) => t + s.count, 0);
          const small = r.onTime !== null && r.onTime.eligible < SMALL_ON_TIME;
          return (
            <li key={r.key} data-programme={r.key}>
              <Mark
                href={r.href}
                className={cn(ROW, "items-center py-2 [&:hover_.row-name]:underline")}
                tip={
                  <>
                    <strong className="font-medium">{r.name}</strong>: {n(r.closed)} finished, {n(r.overdue)} overdue
                    {r.open !== null && <>, {n(r.open)} still open</>}, of {n(r.cases)} cases.
                    {r.onTime && (
                      <>
                        {" "}
                        {cap(r.onTime.step)} {r.onTime.within} for {pct(r.onTime.share)} of referrals past their deadline ({n(r.onTime.onTime)} of{" "}
                        {n(r.onTime.eligible)}){r.onTime.windows.length > 1 && <>; by window, {windowsText(r.onTime)}</>}.
                      </>
                    )}{" "}
                    Opens the patient list.
                  </>
                }
              >
                <span className="col-span-2 flex min-w-0 items-baseline justify-between gap-3 sm:col-span-1 sm:flex-col sm:items-start sm:gap-0">
                  <span className="row-name text-body font-medium">{r.name}</span>
                  <span className="text-small text-muted tabular-nums">{n(r.cases)} cases</span>
                </span>
                <span
                  data-testid={`status-${r.key}`}
                  className="mt-1.5 flex h-2.5 min-w-0 gap-0.5 sm:mt-0"
                  title={`${n(r.closed)} finished · ${n(r.overdue)} overdue${r.open !== null ? ` · ${n(r.open)} still open` : ""}`}
                >
                  {segs.map((s) => (
                    <span key={s.key} className="block h-full rounded-[4px]" style={{ width: `${(s.count / total) * 100}%`, minWidth: 3, backgroundColor: FILLS[s.key] }} />
                  ))}
                </span>
                {onTimeMeasured && (
                  <span data-testid={`on-time-${r.key}`} className="mt-1.5 flex min-w-0 items-center gap-2.5 sm:mt-0">
                    {r.onTime ? (
                      <>
                        <span className="relative block h-1 min-w-0 flex-1 rounded-full" style={{ backgroundColor: CHROME.track }}>
                          <span
                            className="absolute top-1/2 block size-3 -translate-x-1/2 -translate-y-1/2 rounded-full"
                            style={{
                              left: `${r.onTime.share}%`,
                              backgroundColor: small ? "var(--color-surface)" : CATEGORICAL[0],
                              boxShadow: small ? `inset 0 0 0 2px ${CATEGORICAL[0]}` : "0 0 0 2px var(--color-surface)",
                            }}
                          />
                        </span>
                        <span className="flex w-[4.25rem] flex-none flex-col text-right leading-tight">
                          <span className="text-body font-semibold text-ink tabular-nums">{pct(r.onTime.share)}</span>
                          {small && (
                            <span className="text-small text-muted tabular-nums">
                              {n(r.onTime.onTime)} of {n(r.onTime.eligible)}
                            </span>
                          )}
                        </span>
                      </>
                    ) : (
                      <span className="text-small text-muted">No deadline passed yet</span>
                    )}
                  </span>
                )}
              </Mark>
            </li>
          );
        })}
      </ul>
      {onTimeMeasured && (
        <div aria-hidden className={cn(ROW, "mt-1 text-small text-muted")}>
          <span className="hidden sm:block" />
          <span className="hidden sm:block" />
          <span className="relative col-start-2 mr-[calc(4.25rem+10px)] h-4 tabular-nums sm:col-start-3">
            <span data-axis className="absolute left-0 -translate-x-1/2">0%</span>
            <span data-axis className="absolute left-1/2 hidden -translate-x-1/2 sm:block">50%</span>
            <span data-axis className="absolute left-full -translate-x-1/2">100%</span>
          </span>
        </div>
      )}
    </ChartFigure>
  );
}
