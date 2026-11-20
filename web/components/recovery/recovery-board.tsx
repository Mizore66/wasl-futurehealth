"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { PageHeader } from "@/components/shell/page-header";
import { providerLabel } from "@/components/worklist/model";
import { Pager } from "@/components/worklist/pager";
import { pageOfId, pagerFocusAfter, pageStatusText, paginate, type PagerControl } from "@/components/worklist/paging";
import { TAB_FOCUS, useRovingTabs } from "@/components/worklist/use-roving-tabs";
import type { RecoveryDetail, RecoveryList } from "@/lib/api/recovery-schema";
import { cx } from "@/lib/cx";
import { shortId } from "@/lib/domain/ids";
import {
  leadWords,
  NO_VIEW,
  procedureLabel,
  recoveryHref,
  replaceRecoveryParams,
  revealIn,
  SLOT_FILTERS,
  slotRows,
  words,
  type RecoveryView,
  type SlotFilter,
  type SlotRow,
} from "./candidates";
import { SlotDetail, SlotStatusWord } from "./slot-detail";

const FILTER_WORDS: Record<SlotFilter, string> = { open: "Open", filled: "Filled", stale: "Stale", all: "All" };

/** Consecutive rows that share a slot day, so a page reads as a short agenda. */
function byDay(rows: readonly SlotRow[]) {
  const groups: { day: number; lead: number; rows: SlotRow[] }[] = [];
  for (const row of rows) {
    const last = groups.at(-1);
    if (last && last.day === row.day) last.rows.push(row);
    else groups.push({ day: row.day, lead: row.lead, rows: [row] });
  }
  return groups;
}

/**
 * Master and detail: released slots on the left, the selected slot's candidates on the right. From lg up the board
 * fills the screen under the top bar and each column scrolls on its own, as the Today board does.
 */
export function RecoveryBoard({ list, detail, readOnly, view = NO_VIEW }: { list: RecoveryList; detail: RecoveryDetail | null; readOnly: boolean; view?: RecoveryView }) {
  const rows = slotRows(list);
  const slot = detail?.slot ?? null;
  const selectedId = slot?.slot_id ?? view.slot;
  const selectedRow = rows.find((row) => row.id === selectedId);
  const count = (filter: SlotFilter) => (filter === "all" ? rows.length : rows.filter((row) => row.status === filter).length);
  const [filterChoice, setFilterChoice] = useState(view.filter);
  const [pageChoice, setPageChoice] = useState(view.page);
  const filter = filterChoice ?? (count("open") > 0 && (!selectedRow || selectedRow.status === "open") ? "open" : "all");
  const shown = filter === "all" ? rows : rows.filter((row) => row.status === filter);
  const paging = paginate(shown, pageChoice ?? pageOfId(shown, selectedId) ?? 1);
  const scroller = useRef<HTMLDivElement>(null);
  const pager = useRef<HTMLDivElement>(null);
  // 32: the sticky day heading that sits over the top of the list.
  useEffect(() => revealIn(scroller.current, "[aria-current=page]", 32), []);

  const chooseFilter = (next: SlotFilter) => {
    setFilterChoice(next);
    setPageChoice(null);
    replaceRecoveryParams({ status: next, page: null });
    scroller.current?.scrollTo({ top: 0 });
  };
  const tabs = useRovingTabs({ ids: SLOT_FILTERS, selected: filter, onSelect: chooseFilter, prefix: "rc-slots" });

  const goPage = (to: number, via: PagerControl) => {
    const target = paginate(shown, to);
    setPageChoice(target.page);
    replaceRecoveryParams({ page: target.page });
    scroller.current?.scrollTo({ top: 0 });
    const focus = pagerFocusAfter(via, target);
    if (focus) requestAnimationFrame(() => pager.current?.querySelector<HTMLElement>(`[data-focus="pager-${focus}"]`)?.focus());
  };

  const href = (row: SlotRow) =>
    recoveryHref({ ...NO_VIEW, slot: row.id, filter: filterChoice, page: paging.page > 1 ? paging.page : null });

  return (
    <div className="lg:flex lg:h-[calc(100dvh_-_54px_-_2*var(--main-py))] lg:min-h-[480px] lg:flex-col">
      <article aria-labelledby="rc-page" className="flex flex-col rounded-card border border-line bg-surface text-ink lg:min-h-0 lg:flex-1 lg:overflow-clip">
        <PageHeader
          attached
          compact
          title={<span id="rc-page">Recovery</span>}
          sub={`Released capacity from missed appointments, as of run day ${Math.floor(list.as_of_day)}`}
        />
        <div className="grid lg:min-h-0 lg:flex-1 lg:grid-cols-[clamp(280px,30%,400px)_minmax(0,1fr)]">
          <nav aria-labelledby="rc-slots" className="flex min-h-0 min-w-0 flex-col border-b border-line bg-limestone-100 lg:border-r lg:border-b-0">
            <div className="flex-none border-b border-line px-4 pt-3">
              <h2 id="rc-slots" className="staff-display m-0 text-[22px] font-medium text-jade-900">
                Released slots
              </h2>
              <div {...tabs.listProps} aria-label="Slot status" className="-mb-px flex gap-4 overflow-x-auto">
                {SLOT_FILTERS.map((key) => (
                  <button
                    key={key}
                    {...tabs.tabProps(key)}
                    onClick={() => chooseFilter(key)}
                    className={cx(
                      "inline-flex min-h-11 flex-none cursor-pointer items-center gap-1.5 border-b-2 bg-transparent text-body",
                      TAB_FOCUS,
                      filter === key ? "border-jade-700 font-semibold text-jade-900" : "border-transparent font-medium text-muted hover:text-jade-800",
                    )}
                  >
                    {FILTER_WORDS[key]}
                    <span className="text-small tabular-nums text-muted">{count(key)}</span>
                  </button>
                ))}
              </div>
            </div>
            <div
              ref={scroller}
              id={tabs.panelId}
              role="tabpanel"
              aria-labelledby={tabs.tabId(filter)}
              className="relative min-h-0 flex-1 overflow-y-auto overscroll-contain max-lg:max-h-[50dvh]"
            >
              {shown.length === 0 && (
                <p className="m-0 px-4 py-3 text-muted">
                  {list.status !== "ready" ? "Released slots are not available yet." : `No ${filter === "all" ? "" : `${filter} `}slots.`}
                </p>
              )}
              {byDay(paging.rows).map((group) => (
                <section key={`${group.day}-${group.rows[0].id}`} aria-label={`Day ${Math.floor(group.day)}`}>
                  <h3 className="sticky top-0 z-[1] m-0 flex justify-between gap-2 border-b border-rule bg-limestone-100 px-4 py-1.5 text-small font-semibold text-ink-soft">
                    <span>Day {Math.floor(group.day)}</span>
                    <span className="font-medium text-muted">{leadWords(group.lead)}</span>
                  </h3>
                  <ul className="m-0 list-none p-0">
                    {group.rows.map((row) => {
                      const on = row.id === selectedId;
                      return (
                        <li key={row.id} className="border-b border-rule last:border-b-0">
                          <Link
                            href={href(row)}
                            scroll={false}
                            data-slot-row={row.unit}
                            aria-current={on ? "page" : undefined}
                            className={cx(
                              "flex min-h-[52px] flex-col justify-center gap-0.5 px-4 py-2 text-ink no-underline hover:bg-jade-50",
                              on && "bg-jade-100 shadow-[inset_3px_0_var(--color-jade-600)] hover:bg-jade-100",
                            )}
                          >
                            <span className="flex min-w-0 items-baseline justify-between gap-3">
                              <span className="min-w-0 truncate font-semibold">
                                {row.unit} · {procedureLabel(row.procedure)}
                              </span>
                              <SlotStatusWord status={row.status} />
                            </span>
                            <span className="truncate text-small text-muted">
                              {providerLabel(row.provider)}
                              {row.status === "filled" && row.filled_case ? ` · ${shortId(row.filled_case)}` : ""}
                              {row.status === "stale" && row.stale_reason_codes.length ? ` · ${row.stale_reason_codes.map(words).join(", ")}` : ""}
                            </span>
                          </Link>
                        </li>
                      );
                    })}
                  </ul>
                </section>
              ))}
            </div>
            <div ref={pager} className="flex-none border-t border-line">
              <Pager paging={paging} label="Slot pages" testId="slot-page-range" onPage={goPage} className="px-4 py-1.5" />
              {paging.pages > 1 && (
                <p role="status" aria-live="polite" aria-atomic="true" className="sr-only">
                  {pageStatusText(paging)}
                </p>
              )}
            </div>
          </nav>

          {slot ? (
            <SlotDetail key={slot.slot_id} slot={slot} asOfDay={list.as_of_day} readOnly={readOnly} view={view} />
          ) : (
            <p role="status" className="m-0 p-4 lg:px-[clamp(20px,2.4vw,40px)] lg:py-8">
              {list.status !== "ready"
                ? "Released slots are not available yet."
                : list.slots.length === 0
                  ? "No released slots right now (simulated)."
                  : "Select a released slot to see its candidates."}
            </p>
          )}
        </div>
      </article>
    </div>
  );
}
