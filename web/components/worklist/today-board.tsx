"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import NumberFlow from "@number-flow/react";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { Avatar } from "@/components/avatar/avatar";
import { RecordAvatar } from "@/components/wasl/record-avatar";
import { SealedChain, type ChainNode } from "@/components/wasl/sealed-chain";
import { dayToDate, formatDay } from "@/lib/domain/day";
import { caseHref } from "@/lib/domain/ids";
import { cn } from "@/lib/utils";
import { byUrgency, STANDARD_DAYS } from "./days";
import type { View, WorkItem } from "./model";
import { SearchField } from "./search-field";
import { TAB_FOCUS, useRovingTabs } from "./use-roving-tabs";

/*
 * Gap G6: the API has no write endpoint for call outcomes, so every outcome here is session state.
 * `confirm_contact` needs a review item of kind contact; the pipeline raises only link and evidence items.
 */

type Contact = { what: string; when: string };
type Patch = { view?: View; status?: string; snoozed?: boolean; noAnswer?: number; log: Contact[] };
type Stage = "reached" | "callback";
type Logged = { id: string; head: string; move: string; undo: Pick<State, "over" | "stage" | "last"> };
type State = {
  tab: View;
  /** Below lg the list and the open patient take turns; from lg up both show. */
  mobile: "list" | "detail";
  sel: string | null;
  q: string;
  calling: string | null;
  stage: Stage | null;
  last: Logged | null;
  over: Record<string, Patch>;
};

type Row = WorkItem & { snoozed?: boolean; noAnswer?: number; log: Contact[] };

const TABS: [View, string][] = [
  ["call", "To call"],
  ["waiting", "Waiting"],
  ["none", "No owner"],
];

const WEEKDAY = new Intl.DateTimeFormat("en-GB", { weekday: "short", day: "numeric", month: "short", timeZone: "UTC" });
const clock = () => new Date().toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" });

export function TodayBoard({ items, now, aside }: { items: WorkItem[]; now: number; aside: React.ReactNode }) {
  const [s, setS] = useState<State>({ tab: "call", mobile: "detail", sel: null, q: "", calling: null, stage: null, last: null, over: {} });

  const reduce = useReducedMotion();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const rowRef = useRef<HTMLButtonElement>(null);
  const prevPane = useRef(s.mobile);
  useEffect(() => {
    if (prevPane.current === s.mobile) return;
    prevPane.current = s.mobile;
    if (window.matchMedia("(min-width: 1024px)").matches) return;
    (s.mobile === "detail" ? headingRef : rowRef).current?.focus();
  }, [s.mobile]);

  /* Where focus goes after the control that had it is removed. Resolved after the render that removes it. */
  const root = useRef<HTMLDivElement>(null);
  const [want, setWant] = useState<{ k: string } | null>(null);
  useEffect(() => {
    if (want) root.current?.querySelector<HTMLElement>(`[data-focus="${want.k}"]`)?.focus();
  }, [want]);

  const roving = useRovingTabs({
    ids: TABS.map(([id]) => id),
    selected: s.tab,
    onSelect: (id) => setS((prev) => ({ ...prev, tab: id, sel: null, calling: null, stage: null, last: null })),
    prefix: "today",
  });

  const all: Row[] = items.map((it) => {
    const o = s.over[it.id];
    return { ...it, ...o, log: o?.log ?? [] };
  });
  const inView = (v: View) => all.filter((p) => p.view === v).sort(byUrgency);
  const q = s.q.trim().toLowerCase();
  const list = inView(s.tab).filter((p) => !q || p.name.toLowerCase().includes(q) || p.short.toLowerCase().includes(q));
  const toCall = inView("call");
  /* The open patient is always one of the visible results, so Call, Snooze and Hand-off never act on someone filtered or moved out. */
  const p = list.find((x) => x.id === s.sel) ?? list[0];
  const fresh = { calling: null, stage: null, last: null };

  const top = toCall[0];
  const finding = !top
    ? { name: "Nobody", rest: "is left to call." }
    : {
        name: top.name,
        rest:
          (top.days > STANDARD_DAYS
            ? `is ${top.days - STANDARD_DAYS} days over ${STANDARD_DAYS}: ${top.status.toLowerCase()}.`
            : `has ${STANDARD_DAYS - top.days} days left of ${STANDARD_DAYS}: ${top.status.toLowerCase()}.`) +
          ` ${toCall.length - 1} more to call.`,
      };

  const log = (row: Row, what: string, patch: Omit<Patch, "log">, head: string, move: string, stage: Stage | null = null) => {
    setWant({ k: stage ? "chips" : "undo" });
    setS((prev) => {
      const before = prev.over[row.id] ?? { log: [] };
      return {
        ...prev,
        over: { ...prev.over, [row.id]: { ...before, ...patch, log: [...before.log, { what, when: `This session · ${clock()}` }] } },
        sel: row.id,
        calling: null,
        stage,
        last: {
          id: row.id,
          head,
          move,
          undo: { over: prev.over, stage: prev.stage === "callback" ? "reached" : prev.stage, last: prev.last },
        },
      };
    });
  };

  const last = s.last && p && s.last.id === p.id ? s.last : null;
  /* The last action moved its patient out of this result set: they are not shown, only the way back is. */
  const removed = s.last && !list.some((x) => x.id === s.last!.id) ? s.last : null;
  const undoLast = (l: Logged) => {
    setWant({ k: "heading" });
    setS((prev) => ({ ...prev, sel: l.id, calling: null, ...l.undo }));
  };
  const removedNotice = removed && (
    <div role="status" className="mb-4 flex min-h-11 flex-wrap items-center gap-x-gap" data-testid="removed-notice">
      <span>
        <span className="font-medium">{removed.head}</span> · {removed.move} ·{" "}
        <button data-focus="undo" onClick={() => undoLast(removed)} className="inline-flex min-h-11 cursor-pointer items-center bg-transparent px-1 text-body text-hue hover:text-ink">
          Undo
        </button>
      </span>
    </div>
  );
  const stage = last ? s.stage : null;
  const first = p?.name.split(" ")[0] ?? "";
  const toEnd = `${first} moved to the end`;

  type Chip = [string, () => void];
  let chipsTitle = "";
  let chips: Chip[] = [];
  if (p) {
    if (!last) {
      chipsTitle = s.calling === p.id ? "How did it go?" : "After the call";
      chips = [
        ["Reached", () => log(p, "Reached on the phone", { status: `Reached · ${p.next}`, snoozed: false }, "Logged: Reached", "Next: book or set a call back", "reached")],
        [
          "No answer",
          () => {
            const na = (p.noAnswer ?? 0) + 1;
            log(p, "Called, no answer", { noAnswer: na, status: `No answer ×${na} · retry tomorrow`, snoozed: true }, "Logged: No answer", toEnd);
          },
        ],
      ];
    } else if (stage === "reached") {
      chipsTitle = "What next?";
      chips = [
        ["Book", () => log(p, "Booked the procedure", { view: "waiting", status: "Booked · date to confirm", snoozed: false }, "Logged: Booked", `${first} moved to Waiting`)],
        [
          "Call back on…",
          () => {
            setWant({ k: "chips" });
            setS((prev) => ({ ...prev, stage: "callback" }));
          },
        ],
        ["Declined", () => log(p, "Declined the procedure", { view: "waiting", status: "Declined · GP to sign", snoozed: false }, "Logged: Declined", `${first} moved to Waiting for the GP`)],
      ];
    } else if (stage === "callback") {
      chipsTitle = "Call back on";
      chips = [1, 2, 3].map((k) => {
        const d = WEEKDAY.format(dayToDate(Math.floor(now) + k));
        return [d, () => log(p, `Asked for a call back on ${d}`, { status: `Call back ${d}`, snoozed: true }, `Logged: Call back ${d}`, toEnd)] as Chip;
      });
    }
  }
  const chipsLive = !!p && (s.calling === p.id || !!stage);
  const nextUp = p ? list.find((x) => x.id !== p.id) : undefined;
  const callingNow = !!p && s.calling === p.id;

  const trail: ChainNode[] = p
    ? [
        ...p.log.slice(-3).map((c, i) => ({ id: `log-${i}`, kind: "human" as const, who: "Hessa Al Ali", what: c.what, when: c.when })),
        { id: "next", kind: "current", who: "Next", what: p.next, when: p.log.length ? "Now" : "No contact logged this session" },
      ]
    : [];

  const showList = s.mobile === "list";

  return (
    <div ref={root}>
      <header className="mb-6 flex flex-wrap items-end justify-between gap-x-6 gap-y-2">
        <div>
          <h1 className="m-0 text-title font-semibold">Today</h1>
          <p className="m-0 mt-1 text-muted" data-testid="finding">
            <span className="font-medium text-ink">{finding.name}</span> {finding.rest}
          </p>
        </div>
        <div className="flex flex-col gap-1.5 sm:items-end">
          <span className="text-small text-muted">
            Al Mushrif Healthcare Centre · run day {Math.floor(now)}, {formatDay(now)}
          </span>
          {aside}
        </div>
      </header>

      <div className="flex flex-wrap items-stretch gap-gap">
        <section
          aria-label="Your list"
          className={cn("min-w-0 flex-[1_1_300px] flex-col rounded-card border border-line bg-surface lg:flex lg:flex-[0_1_384px]", showList ? "flex" : "hidden")}
        >
          <div {...roving.listProps} aria-label="Saved views" className="flex gap-4 overflow-x-auto border-b border-line px-4">
            {TABS.map(([id, label]) => {
              const on = s.tab === id;
              return (
                <button
                  key={id}
                  {...roving.tabProps(id)}
                  onClick={() => setS((prev) => ({ ...prev, tab: id, sel: null, ...fresh }))}
                  className={cn(
                    "-mb-px h-11 flex-none cursor-pointer whitespace-nowrap border-b-2 bg-transparent text-body",
                    TAB_FOCUS,
                    on ? "border-ink font-medium text-ink" : "border-transparent text-muted",
                  )}
                >
                  {label} <span className="text-small text-muted">{inView(id).length}</span>
                </button>
              );
            })}
          </div>
          <div className="px-4 pb-2 pt-3">
            <SearchField label="Search" placeholder="Name or case id" value={s.q} onChange={(e) => setS((prev) => ({ ...prev, q: e.target.value }))} />
          </div>
          <div role="tabpanel" id={roving.panelId} aria-labelledby={roving.tabId(s.tab)} tabIndex={list.length ? undefined : 0} className="flex flex-1 flex-col gap-0.5 px-2">
            {list.map((r) => {
              const on = p?.id === r.id;
              const over = r.days > STANDARD_DAYS;
              return (
                <div key={r.id} className={cn("relative flex min-h-[68px] items-start gap-1 rounded-control px-1", on ? "bg-active" : "hover:bg-ground")}>
                  <div className="flex size-11 flex-none items-center justify-center">
                    <RecordAvatar id={r.id} name={r.name} linked={r.hasRecord} className="relative z-10 size-11 items-center justify-center">
                      <Avatar person={r.seed} size={24} label="" />
                    </RecordAvatar>
                  </div>
                  <button
                    ref={on ? rowRef : undefined}
                    aria-pressed={on}
                    onClick={() => setS((prev) => ({ ...prev, sel: r.id, mobile: "detail", ...fresh }))}
                    className="min-w-0 flex-1 cursor-pointer bg-transparent py-2.5 pr-2 text-left after:absolute after:inset-0 after:rounded-control"
                  >
                    <span className="flex min-w-0 flex-wrap items-baseline gap-x-2">
                      <span className="font-semibold">{r.name}</span>
                      {r.nameAr && (
                        <span lang="ar" dir="rtl" className="text-muted">
                          {r.nameAr}
                        </span>
                      )}
                      <span className={cn("ml-auto whitespace-nowrap text-small", over ? "font-medium text-negative" : "text-muted")}>
                        {r.days} days · {over ? `${r.days - STANDARD_DAYS} over` : `${STANDARD_DAYS - r.days} left`}
                      </span>
                    </span>
                    <span className="mt-0.5 block text-muted">{r.status}</span>
                  </button>
                </div>
              );
            })}
            {list.length === 0 && <p className="m-2 rounded-card border border-dashed border-control px-3 py-2.5 text-muted">Nobody left in this view.</p>}
          </div>
          <div className="border-t border-rule px-4 py-3 text-small text-muted">
            {s.tab === "call"
              ? `${list.length} to call · ${list.filter((r) => r.days > STANDARD_DAYS).length} over ${STANDARD_DAYS} days`
              : s.tab === "waiting"
                ? `${list.length} waiting on someone else`
                : `${list.length} with no owner`}
          </div>
        </section>

        {p ? (
          <section
            aria-label="Open patient"
            className={cn("min-w-0 flex-[1_1_520px] flex-col rounded-card border border-line bg-surface p-4 sm:p-pad lg:flex", showList ? "hidden" : "flex")}
          >
            {removedNotice}
            <button
              onClick={() => setS((prev) => ({ ...prev, mobile: "list" }))}
              className="-mt-1 mb-3 flex min-h-11 cursor-pointer items-center gap-2 self-start rounded-control bg-transparent pr-3 text-body text-ink lg:hidden"
            >
              <span aria-hidden>‹</span>
              Your list · {list.length}
            </button>
            <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between sm:gap-6">
              <div className="flex min-w-0 items-start gap-4">
                <RecordAvatar id={p.id} name={p.name} linked={p.hasRecord}>
                  <Avatar person={p.seed} size={56} label="" />
                </RecordAvatar>
                <div className="min-w-0">
                  <div className="flex flex-wrap items-baseline gap-x-3">
                    <h2 ref={headingRef} data-focus="heading" tabIndex={-1} className="m-0 text-title font-semibold outline-none">
                      {p.name}
                    </h2>
                    {p.nameAr && (
                      <span lang="ar" dir="rtl" className="text-section text-muted">
                        {p.nameAr}
                      </span>
                    )}
                  </div>
                  <p className="m-0 mt-1 text-muted">{[p.age, p.named ? p.short : null, `referred ${formatDay(p.referralDay)}`].filter(Boolean).join(" · ")}</p>
                  {p.hasRecord && (
                    <Link href={caseHref("/app/patients", p.id)} className="inline-flex min-h-11 items-center underline">
                      Open record
                    </Link>
                  )}
                </div>
              </div>
              <div className="sm:flex-none sm:text-right">
                <div className="flex items-baseline gap-1.5 sm:justify-end">
                  <span data-testid="lead-days" data-value={p.days} className={cn("text-lead font-bold", p.days > STANDARD_DAYS ? "text-negative" : "text-ink")}>
                    <NumberFlow value={p.days} />
                  </span>
                  <span className="text-muted">days since referral</span>
                </div>
                <div className={cn("mt-0.5 text-body sm:text-small", p.days > STANDARD_DAYS ? "text-negative" : "text-muted")}>
                  {p.days > STANDARD_DAYS ? `${p.days - STANDARD_DAYS} days over ${STANDARD_DAYS}` : `${STANDARD_DAYS - p.days} days left of ${STANDARD_DAYS}`}
                </div>
              </div>
            </div>

            <div className="mt-4 flex flex-wrap items-center gap-gap">
              <button
                onClick={() => setS((prev) => ({ ...prev, calling: p.id, stage: null, last: null }))}
                className={cn(
                  "flex h-12 w-full cursor-pointer items-center justify-center gap-2 rounded-control px-5 text-section font-medium text-white sm:h-11 sm:w-auto",
                  callingNow ? "bg-ink-soft" : "bg-cta",
                )}
              >
                <svg aria-hidden width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M5.5 2.5l1.5 3-1.5 1c.8 1.6 2 2.8 3.5 3.5l1-1.5 3 1.5-.5 2c-.2.6-.8 1-1.4.9C6.6 12.4 3.6 9.4 3.1 4.9c-.1-.6.3-1.2.9-1.4z" />
                </svg>
                <span>{callingNow ? `On the call · ${first}` : `Call ${first}`}</span>
              </button>
              <span className="hidden flex-1 sm:block" />
              <Secondary className="max-sm:flex-1" onClick={() => log(p, "Snoozed to tomorrow", { status: "Snoozed to tomorrow", snoozed: true }, "Logged: Snoozed to tomorrow", toEnd)}>
                Snooze to tomorrow
              </Secondary>
              {p.view === "none" ? (
                <Secondary className="max-sm:flex-1" onClick={() => log(p, "Took this patient", { view: "call", status: "Yours now" }, "Logged: Yours now", `${first} moved to To call`)}>
                  Take this patient
                </Secondary>
              ) : (
                <Secondary className="max-sm:flex-1" onClick={() => log(p, "Handed to Dr Al Kaabi", { view: "waiting", status: "With Dr Al Kaabi" }, "Logged: Handed to Dr Al Kaabi", `${first} moved to Waiting`)}>
                  Hand to Dr Al Kaabi
                </Secondary>
              )}
            </div>

            <AnimatePresence initial={false}>
              {last && (
                <motion.div
                  key="logged"
                  role="status"
                  initial={reduce ? false : { opacity: 0, y: -4 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ duration: reduce ? 0 : 0.15 }}
                  className="mt-4 flex min-h-11 flex-wrap items-center gap-x-gap"
                >
                  <span>
                    <span className="font-medium">{last.head}</span> · {last.move} ·{" "}
                    <button
                      data-focus="undo"
                      onClick={() => undoLast(last)}
                      className="inline-flex min-h-11 cursor-pointer items-center bg-transparent px-1 text-body text-hue hover:text-ink"
                    >
                      Undo
                    </button>
                  </span>
                  {!stage && nextUp && (
                    <Secondary
                      className="sm:ml-auto"
                      onClick={() => {
                        setWant({ k: "heading" });
                        setS((prev) => ({ ...prev, sel: nextUp.id, ...fresh }));
                      }}
                    >
                      Next patient: {nextUp.name}
                    </Secondary>
                  )}
                </motion.div>
              )}
            </AnimatePresence>

            {chips.length > 0 && (
              <div className="mt-4">
                <h3 data-focus="chips" tabIndex={-1} className="m-0 text-section font-medium">
                  {chipsTitle}
                </h3>
                <div role="group" aria-label={chipsTitle} className="mt-2 flex flex-wrap gap-2">
                  {chips.map(([label, pick]) => (
                    <button
                      key={label}
                      onClick={pick}
                      className={cn("h-11 cursor-pointer rounded-chip border bg-surface px-4 text-body text-ink hover:bg-ground", chipsLive ? "border-ink" : "border-control")}
                    >
                      {label}
                    </button>
                  ))}
                </div>
              </div>
            )}

            <Track days={p.days} referralDay={p.referralDay} />

            <dl className="m-0 mt-6 grid grid-cols-1 gap-gap sm:grid-cols-3">
              <Field label="Texts" value={p.textsAllowed ? "Allowed" : "Not allowed yet"} warn={!p.textsAllowed} />
              <Field label="Owner" value={p.owner} />
              <Field label="Next" value={p.next} />
            </dl>

            <div className="mt-7">
              <h3 className="m-0 mb-3 text-section font-medium">Contacts</h3>
              <div aria-live="polite">
                <SealedChain nodes={trail} />
              </div>
            </div>
          </section>
        ) : (
          <section aria-label="Open patient" className={cn("min-w-0 flex-[1_1_520px] rounded-card border border-line bg-surface p-4 sm:p-pad lg:block", showList ? "hidden" : "block")}>
            <button onClick={() => setS((prev) => ({ ...prev, mobile: "list" }))} className="mb-3 flex min-h-11 cursor-pointer items-center gap-2 bg-transparent text-body text-ink lg:hidden">
              <span aria-hidden>‹</span>
              Your list
            </button>
            {removedNotice}
            <h2 className="m-0 text-section font-medium">Nobody to open</h2>
            <p className="m-0 mt-2 text-muted">This view is empty. Choose another tab.</p>
          </section>
        )}
      </div>
    </div>
  );
}

function Track({ days, referralDay }: { days: number; referralDay: number }) {
  const SCALE = 40;
  const fill = (Math.min(days, STANDARD_DAYS) / SCALE) * 100;
  const over = (Math.min(Math.max(0, days - STANDARD_DAYS), SCALE - STANDARD_DAYS) / SCALE) * 100;
  const tick = (STANDARD_DAYS / SCALE) * 100;
  return (
    <div role="img" aria-label={`Day ${days} of ${STANDARD_DAYS} since referral${days > STANDARD_DAYS ? `, ${days - STANDARD_DAYS} over` : ""}`} className="mt-5">
      <div className="relative h-4">
        <span className="absolute inset-x-0 top-1 h-2 rounded-chip bg-raised" />
        <span className="absolute left-0 top-1 h-2 rounded-l-chip bg-hue" style={{ width: `${fill}%` }} />
        <span className="absolute top-1 h-2 rounded-r-chip bg-negative" style={{ left: `${tick}%`, width: `${over}%` }} />
        <span className="absolute top-0 h-4 w-0.5 bg-ink" style={{ left: `${tick}%` }} />
      </div>
      <div className="relative mt-1.5 h-4 text-small text-muted">
        <span className="absolute left-0">Referred {formatDay(referralDay)}</span>
        <span className="absolute -translate-x-1/2 whitespace-nowrap" style={{ left: `${tick}%` }}>
          Day {STANDARD_DAYS} · {formatDay(referralDay + STANDARD_DAYS)}
        </span>
      </div>
    </div>
  );
}

function Field({ label, value, warn = false }: { label: string; value: string; warn?: boolean }) {
  return (
    <div>
      <dt className="text-small text-muted">{label}</dt>
      <dd className={cn("m-0 mt-1 font-medium", warn ? "text-warning" : "text-ink")}>{value}</dd>
    </div>
  );
}

function Secondary({ className, ...props }: React.ComponentProps<"button">) {
  return <button {...props} className={cn("h-11 cursor-pointer rounded-control bg-active px-4 text-body text-ink hover:bg-control", className)} />;
}
