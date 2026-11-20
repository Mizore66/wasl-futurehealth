"use client";

import { useLayoutEffect, useRef, useState } from "react";
import { flushSync } from "react-dom";
import {
  Activity,
  ArrowDownWideNarrow,
  BadgeCheck,
  CalendarDays,
  Check,
  ChevronRight,
  ClipboardList,
  FileText,
  History,
  Inbox,
  Lock,
  MapPin,
  MessageSquareText,
  Phone,
  Send,
  Undo2,
  User,
  UserRound,
  X,
  type LucideIcon,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { SegmentedToggle } from "@/components/ui/segmented-toggle";
import { dayLabel, groupByMonth, type AuditEvent, type AuditEventKind, type DecisionView } from "./audit-model";

const ICON: Record<AuditEventKind, LucideIcon> = {
  referral: Send,
  intake: Inbox,
  consent: MessageSquareText,
  booked: CalendarDays,
  procedure: Activity,
  report: FileText,
  visit: MapPin,
  source: FileText,
  review: BadgeCheck,
  call: Phone,
  owner: User,
  booking: CalendarDays,
  coordinator: ClipboardList,
  patient: UserRound,
  compensation: Undo2,
  claim: Lock,
  unknown: History,
};

export type WorkspaceEvent = AuditEvent & { anchor?: string };

type Props = {
  /** The page header, drawn as the card's top band above both panes. */
  head: React.ReactNode;
  events: WorkspaceEvent[];
  /** Server-rendered detail and controls of ledger events, keyed by event key. */
  details: Record<string, React.ReactNode>;
  controls: Record<string, React.ReactNode>;
  /** Above the months: governance status, honesty label, or why history is withheld. */
  historyIntro?: React.ReactNode;
  historyEmpty: string;
  snapshotLabel: string;
  decisions: DecisionView[];
  /** Under the score: chain warnings and the honesty label. */
  verifyNote?: React.ReactNode;
  auditDetails: React.ReactNode;
};

const orderWord = (oldest: boolean) => (oldest ? "Oldest first" : "Newest first");

/**
 * Whether every month of the history fits in the space the pane takes with only the first month open. Flips the
 * months' `open` in the DOM to measure both layouts and puts them back, all before the browser paints.
 */
function allMonthsFit(pane: HTMLElement, chronology: HTMLElement): boolean | undefined {
  const months = [...chronology.querySelectorAll<HTMLDetailsElement>(":scope > details")];
  if (months.length < 2) return undefined;
  const was = months.map((d) => d.open);
  months.forEach((d, i) => (d.open = i === 0));
  const room = pane.getBoundingClientRect().height;
  months.forEach((d) => (d.open = true));
  const top = chronology.getBoundingClientRect().top - pane.getBoundingClientRect().top;
  const need = top + months.reduce((n, d) => n + d.getBoundingClientRect().height, 0) + Number.parseFloat(getComputedStyle(pane).paddingBottom || "0");
  months.forEach((d, i) => (d.open = was[i]));
  // A hidden pane (the phone's other tab) has no room to measure; decide when it is shown.
  return room > 0 ? need <= room + 1 : undefined;
}

function EventRow({ e, detail, controls }: { e: WorkspaceEvent; detail?: React.ReactNode; controls?: React.ReactNode }) {
  const Icon = ICON[e.kind];
  const row = (
    <>
      <span aria-hidden className="col-start-1 row-start-1 grid size-[27px] place-items-center justify-self-center rounded-full bg-jade-50 text-jade-700 max-md:row-span-2">
        <Icon className="size-4" strokeWidth={1.6} />
      </span>
      {e.date && (
        <time dateTime={e.date} className="col-start-2 row-start-1 whitespace-nowrap text-[12px] leading-tight tabular-nums text-muted max-md:text-[11px]">
          {dayLabel(e.date)}
        </time>
      )}
      <span
        className={cn(
          "row-start-1 flex min-w-0 flex-wrap items-baseline justify-between gap-x-3 gap-y-px max-md:grid",
          e.date ? "col-start-3 max-md:col-start-2 max-md:row-start-2" : "col-span-2 col-start-2 max-md:col-span-1",
        )}
      >
        <strong className="min-w-0 break-words text-[14px] font-[550] text-ink">{e.label}</strong>
        <small className="text-[13px] text-muted">{e.source}</small>
      </span>
    </>
  );
  const grid = "grid flex-1 grid-cols-[28px_85px_minmax(0,1fr)_14px] items-center gap-x-2 py-1.5 max-md:grid-cols-[28px_minmax(0,1fr)_14px] max-md:gap-y-0.5";
  return (
    <li
      id={e.anchor}
      className="relative flex min-h-[clamp(46px,6.2vh,80px)] scroll-mt-4 flex-col before:pointer-events-none before:absolute before:inset-y-0 before:left-[14px] before:w-px before:bg-jade-200 first:before:top-1/2 last:before:bottom-1/2 only:before:hidden"
    >
      {detail ? (
        <details className="group/ev flex flex-1 flex-col">
          <summary className={cn(grid, "min-h-11 cursor-pointer list-none rounded-sm [&::-webkit-details-marker]:hidden")}>
            {row}
            <ChevronRight aria-hidden className="col-start-4 row-start-1 size-3 text-muted group-open/ev:rotate-90 max-md:col-start-3 max-md:row-span-2" strokeWidth={1.8} />
          </summary>
          <div className="mb-2.5 ml-[129px] max-w-[54ch] text-[13px] text-muted max-md:ml-9">{detail}</div>
        </details>
      ) : (
        <div className={grid}>{row}</div>
      )}
      {controls && <div className="mb-2 ml-[129px] max-md:ml-9">{controls}</div>}
    </li>
  );
}

function Receipt({ d }: { d: DecisionView }) {
  return (
    <>
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h3 className="staff-display m-0 text-[22px] tracking-[-0.025em]">{d.label}</h3>
        {d.matched ? (
          <span className="inline-flex items-center gap-1 text-[12px] text-jade-700" data-match="true">
            <Check aria-hidden className="size-[13px]" strokeWidth={1.8} />
            Unchanged since signing
          </span>
        ) : (
          <span className="inline-flex items-center gap-1 text-[12px] font-semibold text-clay-ink" data-match="false">
            <X aria-hidden className="size-[13px]" strokeWidth={2} />
            Changed since signing
          </span>
        )}
      </div>
      <p className="m-0 mb-3 mt-2 max-w-[55ch] leading-normal">{d.explanation}</p>
    </>
  );
}

/** The approved audit layout: event history beside verification on desktop, two tabs on a phone. */
export function AuditWorkspace({ head, events, details, controls, historyIntro, historyEmpty, snapshotLabel, decisions, verifyNote, auditDetails }: Props) {
  const [oldest, setOldest] = useState(false);
  const groups = groupByMonth(events, oldest);
  // Server render and first paint without layout: only the newest month open. When every month fits in the room
  // the pane already takes, all open instead, decided before paint and again on resize until the reader toggles one.
  const [open, setOpen] = useState<Set<string>>(() => new Set(groups[0] ? [groups[0].name] : []));
  const [fits, setFits] = useState(false);
  const touched = useRef(false);
  const paneRef = useRef<HTMLElement>(null);
  const chronologyRef = useRef<HTMLDivElement>(null);
  const [view, setView] = useState<"history" | "verification">("history");
  const [picked, setPicked] = useState(0);
  const matched = decisions.filter((d) => d.matched).length;

  const monthKey = groups.map((g) => g.name).join("|");
  useLayoutEffect(() => {
    const pane = paneRef.current;
    const chronology = chronologyRef.current;
    if (!pane || !chronology) return;
    const names = monthKey ? monthKey.split("|") : [];
    let last = { w: -1, h: -1 };
    const decide = (sync: boolean) => {
      if (touched.current) return;
      const w = pane.getBoundingClientRect().width;
      if (w === last.w && window.innerHeight === last.h) return;
      last = { w, h: window.innerHeight };
      const fit = allMonthsFit(pane, chronology);
      if (fit === undefined) return;
      const apply = () => {
        setFits(fit);
        setOpen((s) => (fit ? new Set(names) : s.size === names.length && names.length > 1 ? new Set(names.slice(0, 1)) : s));
      };
      if (sync) flushSync(apply);
      else apply();
    };
    decide(false);
    const ro = new ResizeObserver(() => decide(true));
    ro.observe(pane);
    return () => ro.disconnect();
  }, [monthKey]);

  function toggleOrder() {
    const next = !oldest;
    setOldest(next);
    const shown = groupByMonth(events, next);
    setOpen(new Set(fits && !touched.current ? shown.map((g) => g.name) : shown[0] ? [shown[0].name] : []));
  }

  const paneHead = "flex min-h-[clamp(58px,8vh,90px)] items-center justify-between gap-2 border-b border-line max-md:min-h-0 max-md:border-0";
  const paneTitle = "staff-display m-0 text-[clamp(21px,2vw,31px)] leading-[1.1] max-md:sr-only";
  const corner = "max-md:absolute max-md:right-[15px] max-md:top-0.5";

  return (
    <div className="flex min-w-0 flex-1 flex-col rounded-[8px] border border-line bg-surface">
      {head}
      <div
        className="relative grid min-w-0 flex-1 grid-cols-[minmax(0,1.36fr)_minmax(300px,1fr)] max-md:flex max-md:flex-col"
        data-audit-view={view}
      >
        <SegmentedToggle
          label="Audit view"
          value={view}
          onChange={setView}
          items={[
            { value: "history", label: "History", ariaLabel: "Event history" },
            { value: "verification", label: "Verification" },
          ]}
          className="hidden px-[15px] max-md:flex"
        />

        <section
          ref={paneRef}
          aria-labelledby="au-history-h"
          className={cn("flex min-w-0 flex-col px-[clamp(15px,2.1vw,36px)] pb-3.5 max-md:flex-1", view !== "history" && "max-md:hidden")}
        >
          <header className={paneHead}>
            <h2 id="au-history-h" className={paneTitle}>
              Event history
            </h2>
            {events.some((e) => e.date) && (
              <button
                type="button"
                onClick={toggleOrder}
                aria-controls="audit-chronology"
                title={`Switch to ${orderWord(!oldest).toLowerCase()}`}
                data-oldest={oldest}
                className={cn("inline-flex min-h-10 items-center gap-1.5 rounded px-1.5 text-[12px] text-jade-700 hover:bg-jade-50 max-md:min-h-11", corner)}
              >
                <ArrowDownWideNarrow aria-hidden className={cn("size-[15px]", oldest && "-scale-y-100")} strokeWidth={1.6} />
                {orderWord(oldest)}
              </button>
            )}
          </header>
          {historyIntro}
          <div id="audit-chronology" ref={chronologyRef} className="flex flex-1 flex-col" data-all-fit={fits || undefined}>
            {groups.length === 0 ? (
              <p className="m-0 mb-3.5 mt-2 text-[13px] text-muted">{historyEmpty}</p>
            ) : (
              groups.map((g) => {
                const isOpen = open.has(g.name);
                return (
                  <details
                    key={g.name}
                    open={isOpen}
                    onToggle={(ev) => {
                      const now = (ev.currentTarget as HTMLDetailsElement).open;
                      if (now === isOpen) return;
                      touched.current = true;
                      setOpen((s) => {
                        const n = new Set(s);
                        if (now) n.add(g.name);
                        else n.delete(g.name);
                        return n;
                      });
                    }}
                    className="group/m border-b border-line last:border-b-0"
                    data-month={g.name}
                  >
                    <summary className="flex min-h-[clamp(44px,6.1vh,72px)] cursor-pointer list-none items-center gap-2 text-[14px] font-semibold max-md:min-h-11 [&::-webkit-details-marker]:hidden">
                      <span>{g.name}</span>
                      <small className="text-[11px] font-normal text-muted">
                        {g.events.length}
                        <span className="sr-only"> {g.events.length === 1 ? "event" : "events"}</span>
                      </small>
                      <ChevronRight aria-hidden className="ms-auto size-[13px] text-muted group-open/m:rotate-90" strokeWidth={1.8} />
                    </summary>
                    <ol className="m-0 mb-2 flex list-none flex-col p-0">
                      {g.events.map((e) => (
                        <EventRow key={e.key} e={e} detail={details[e.key]} controls={controls[e.key]} />
                      ))}
                    </ol>
                  </details>
                );
              })
            )}
          </div>
        </section>

        <section
          aria-labelledby="au-verify-h"
          className={cn("flex min-w-0 flex-col border-s border-line px-[clamp(15px,2.1vw,36px)] pb-3.5 max-md:flex-1 max-md:border-0", view !== "verification" && "max-md:hidden")}
        >
          <header className={paneHead}>
            <h2 id="au-verify-h" className={paneTitle}>
              Verification
            </h2>
            <span className={cn("text-[12px] text-muted max-md:flex max-md:h-11 max-md:items-center max-md:text-[11px]", corner)}>{snapshotLabel}</span>
          </header>
          {decisions.length === 0 ? (
            <p className="m-0 my-4 text-[13px] text-muted" role="status" data-testid="signature-score">
              No signed decisions to check for this case.
            </p>
          ) : (
            <>
              <div className="flex flex-wrap items-baseline gap-x-3.5 py-[clamp(14px,2.8vh,38px)] max-md:py-[18px]" role="status" data-testid="signature-score">
                <div className="staff-display text-[clamp(42px,4.3vw,74px)] leading-none tracking-[-0.05em] text-jade-900">
                  <b className="font-medium">{matched}</b>
                  <span className="text-[0.62em] text-jade-500"> / {decisions.length}</span>
                </div>
                <p className="m-0 text-[14px] text-ink">signed decisions unchanged</p>
              </div>
              {verifyNote}
              <fieldset className="@container m-0 border-0 p-0">
                <legend className="mb-2 p-0 text-[12px] text-muted">Inspect a decision</legend>
                <div className="grid grid-cols-3 gap-1.5 @max-[380px]:grid-cols-2">
                  {decisions.map((d, i) => (
                    <label key={d.key} className="relative block cursor-pointer">
                      <input
                        type="radio"
                        name="audit-decision"
                        value={i}
                        checked={picked === i}
                        onChange={() => setPicked(i)}
                        aria-label={`${d.label}, decision ${i + 1}, ${d.matched ? "unchanged since signing" : "changed since signing"}`}
                        className="peer absolute inset-0 m-0 size-full cursor-pointer opacity-0"
                      />
                      <span
                        className={cn(
                          "flex h-full min-h-[clamp(46px,5.5vh,65px)] items-center justify-between gap-1.5 rounded px-[9px] py-2 peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-jade-800 max-md:min-h-11",
                          picked === i ? "bg-jade-700 text-limestone-50" : d.matched ? "bg-jade-50 text-jade-700" : "bg-clay-wash text-clay-ink",
                        )}
                      >
                        {/* A long word ("acknowledgement") must break inside the chip, or it pushes the tick past the edge. */}
                        <b className="min-w-0 hyphens-auto text-[13px] font-[550] leading-[1.25] [overflow-wrap:anywhere]">{d.label}</b>
                        {d.matched ? <Check aria-hidden className="size-[13px] flex-none" strokeWidth={1.8} /> : <X aria-hidden className="size-[13px] flex-none" strokeWidth={2} />}
                      </span>
                    </label>
                  ))}
                </div>
              </fieldset>
              <div className="my-[clamp(12px,2vh,24px)] border-s-2 border-jade-400 bg-limestone-100 p-[clamp(14px,1.6vw,25px)]" aria-live="polite" aria-atomic="true" data-testid="decision-receipt">
                <Receipt d={decisions[picked]} />
              </div>
            </>
          )}
          <div className="mt-auto border-t border-line pb-[5px] max-md:pb-2">
            <details className="group/ad">
              <summary className="flex min-h-[42px] cursor-pointer list-none items-center text-[13px] font-[550] max-md:min-h-11 [&::-webkit-details-marker]:hidden">
                Audit details
                <ChevronRight aria-hidden className="ms-auto size-[13px] text-muted group-open/ad:rotate-90" strokeWidth={1.8} />
              </summary>
              <div className="flex flex-col gap-5 pb-3 pt-2 [&_section]:rounded-none [&_section]:border-0 [&_section]:bg-transparent [&_section]:p-0">{auditDetails}</div>
            </details>
          </div>
        </section>
      </div>
    </div>
  );
}
