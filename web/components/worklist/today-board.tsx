"use client";

import { useState } from "react";
import Link from "next/link";
import NumberFlow from "@number-flow/react";
import { AnimatePresence, motion } from "motion/react";
import { Avatar } from "@/components/avatar/avatar";
import { SealedChain, type ChainNode } from "@/components/wasl/sealed-chain";
import { dayToDate, formatDay } from "@/lib/domain/day";
import { caseHref } from "@/lib/domain/ids";
import { cn } from "@/lib/utils";
import { byUrgency, STANDARD_DAYS } from "./days";
import type { View, WorkItem } from "./model";

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
  const [s, setS] = useState<State>({ tab: "call", sel: null, q: "", calling: null, stage: null, last: null, over: {} });

  const all: Row[] = items.map((it) => {
    const o = s.over[it.id];
    return { ...it, ...o, log: o?.log ?? [] };
  });
  const inView = (v: View) => all.filter((p) => p.view === v).sort(byUrgency);
  const q = s.q.trim().toLowerCase();
  const list = inView(s.tab).filter((p) => !q || p.name.toLowerCase().includes(q) || p.short.toLowerCase().includes(q));
  const toCall = inView("call");
  const p = list.find((x) => x.id === s.sel) ?? (s.sel ? all.find((x) => x.id === s.sel) : undefined) ?? list[0];
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
        ["Call back on…", () => setS((prev) => ({ ...prev, stage: "callback" }))],
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

  return (
    <>
      <header className="mb-6 flex flex-wrap items-end justify-between gap-6">
        <div>
          <h1 className="m-0 text-title font-semibold">Today</h1>
          <p className="m-0 mt-1 text-muted" data-testid="finding">
            <span className="font-medium text-ink">{finding.name}</span> {finding.rest}
          </p>
        </div>
        <div className="flex flex-col items-end gap-1.5">
          <span className="text-small text-muted">
            Al Mushrif Healthcare Centre · run day {Math.floor(now)}, {formatDay(now)}
          </span>
          {aside}
        </div>
      </header>

      <div className="flex flex-wrap items-stretch gap-gap">
        <section aria-label="Your list" className="flex min-w-[300px] flex-[0_1_384px] flex-col rounded-card border border-line bg-surface">
          <div role="tablist" aria-label="Saved views" className="flex gap-5 border-b border-line px-4">
            {TABS.map(([id, label]) => {
              const on = s.tab === id;
              return (
                <button
                  key={id}
                  role="tab"
                  aria-selected={on}
                  onClick={() => setS((prev) => ({ ...prev, tab: id, sel: null, ...fresh }))}
                  className={cn(
                    "-mb-px h-11 cursor-pointer border-b-2 bg-transparent text-body",
                    on ? "border-ink font-medium text-ink" : "border-transparent text-muted",
                  )}
                >
                  {label} <span className="text-small text-muted">{inView(id).length}</span>
                </button>
              );
            })}
          </div>
          <div className="px-4 pb-2 pt-3">
            <label className="flex h-control items-center gap-2 rounded-control border border-control bg-surface px-2.5">
              <svg aria-hidden width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" className="text-muted">
                <circle cx="7" cy="7" r="4.5" />
                <path d="M10.5 10.5l3 3" />
              </svg>
              <span className="sr-only">Search</span>
              <input
                type="search"
                placeholder="Name or case id"
                value={s.q}
                onChange={(e) => setS((prev) => ({ ...prev, q: e.target.value }))}
                className="min-w-0 flex-1 border-0 bg-transparent text-body text-ink outline-none"
              />
            </label>
          </div>
          <div className="flex flex-1 flex-col gap-0.5 px-2">
            {list.map((r) => {
              const on = p?.id === r.id;
              const over = r.days > STANDARD_DAYS;
              return (
                <button
                  key={r.id}
                  aria-pressed={on}
                  onClick={() => setS((prev) => ({ ...prev, sel: r.id, ...fresh }))}
                  className={cn("block h-[68px] w-full cursor-pointer rounded-control px-2.5 py-3 text-left", on ? "bg-active" : "bg-transparent hover:bg-ground")}
                >
                  <span className="flex min-w-0 items-center gap-2">
                    <Avatar person={r.seed} size={24} label="" />
                    <span className="whitespace-nowrap font-semibold">{r.name}</span>
                    {r.nameAr && (
                      <span lang="ar" dir="rtl" className="truncate text-muted">
                        {r.nameAr}
                      </span>
                    )}
                    <span className={cn("ml-auto flex-none whitespace-nowrap text-small", over ? "font-medium text-negative" : "text-muted")}>
                      {r.days} days · {over ? `${r.days - STANDARD_DAYS} over` : `${STANDARD_DAYS - r.days} left`}
                    </span>
                  </span>
                  <span className="mt-1 block truncate pl-8 text-muted">{r.status}</span>
                </button>
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
          <section aria-label="Open patient" className="flex min-w-0 flex-[1_1_520px] flex-col rounded-card border border-line bg-surface p-pad">
            <div className="flex items-start justify-between gap-6">
              <div className="flex min-w-0 items-start gap-4">
                <Avatar person={p.seed} size={56} label="" />
                <div className="min-w-0">
                  <div className="flex flex-wrap items-baseline gap-3">
                    <h2 className="m-0 text-title font-semibold">{p.name}</h2>
                    {p.nameAr && (
                      <span lang="ar" dir="rtl" className="text-section text-muted">
                        {p.nameAr}
                      </span>
                    )}
                  </div>
                  <p className="m-0 mt-1 text-muted">
                    {[p.age, p.named ? p.short : null, `referred ${formatDay(p.referralDay)}`].filter(Boolean).join(" · ")}
                    {p.hasRecord && (
                      <>
                        {" · "}
                        <Link href={caseHref("/app/patients", p.id)} className="no-underline">
                          Open record
                        </Link>
                      </>
                    )}
                  </p>
                </div>
              </div>
              <div className="flex-none text-right">
                <div className="flex items-baseline justify-end gap-1.5">
                  <span data-testid="lead-days" data-value={p.days} className={cn("text-lead font-bold", p.days > STANDARD_DAYS ? "text-negative" : "text-ink")}>
                    <NumberFlow value={p.days} />
                  </span>
                  <span className="text-muted">days since referral</span>
                </div>
                <div className={cn("mt-0.5 text-small", p.days > STANDARD_DAYS ? "text-negative" : "text-muted")}>
                  {p.days > STANDARD_DAYS ? `${p.days - STANDARD_DAYS} days over ${STANDARD_DAYS}` : `${STANDARD_DAYS - p.days} days left of ${STANDARD_DAYS}`}
                </div>
              </div>
            </div>

            <Track days={p.days} referralDay={p.referralDay} />

            <dl className="m-0 mt-6 grid grid-cols-3 gap-gap">
              <Field label="Texts" value={p.textsAllowed ? "Allowed" : "Not allowed yet"} warn={!p.textsAllowed} />
              <Field label="Owner" value={p.owner} />
              <Field label="Next" value={p.next} />
            </dl>

            <div className="mt-6 flex flex-wrap items-center gap-gap">
              <button
                onClick={() => setS((prev) => ({ ...prev, calling: p.id, stage: null, last: null }))}
                className={cn("flex h-11 cursor-pointer items-center gap-2 rounded-control px-4 text-section font-medium text-white", callingNow ? "bg-ink-soft" : "bg-cta")}
              >
                <svg aria-hidden width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M5.5 2.5l1.5 3-1.5 1c.8 1.6 2 2.8 3.5 3.5l1-1.5 3 1.5-.5 2c-.2.6-.8 1-1.4.9C6.6 12.4 3.6 9.4 3.1 4.9c-.1-.6.3-1.2.9-1.4z" />
                </svg>
                <span>{callingNow ? `On the call · ${first}` : `Call ${first}`}</span>
              </button>
              <span className="flex-1" />
              <Secondary onClick={() => log(p, "Snoozed to tomorrow", { status: "Snoozed to tomorrow", snoozed: true }, "Logged: Snoozed to tomorrow", toEnd)}>
                Snooze to tomorrow
              </Secondary>
              {p.view === "none" ? (
                <Secondary onClick={() => log(p, "Took this patient", { view: "call", status: "Yours now" }, "Logged: Yours now", `${first} moved to To call`)}>
                  Take this patient
                </Secondary>
              ) : (
                <Secondary onClick={() => log(p, "Handed to Dr Al Kaabi", { view: "waiting", status: "With Dr Al Kaabi" }, "Logged: Handed to Dr Al Kaabi", `${first} moved to Waiting`)}>
                  Hand to Dr Al Kaabi
                </Secondary>
              )}
            </div>

            <AnimatePresence initial={false}>
              {last && (
                <motion.div
                  key="logged"
                  role="status"
                  initial={{ opacity: 0, y: -4 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0 }}
                  transition={{ duration: 0.15 }}
                  className="mt-6 flex min-h-control flex-wrap items-center gap-gap"
                >
                  <span>
                    <span className="font-medium">{last.head}</span> · {last.move} ·{" "}
                    <button onClick={() => setS((prev) => ({ ...prev, sel: last.id, calling: null, ...last.undo }))} className="cursor-pointer bg-transparent p-0 text-body text-hue hover:text-ink">
                      Undo
                    </button>
                  </span>
                  {!stage && nextUp && (
                    <Secondary className="ml-auto" onClick={() => setS((prev) => ({ ...prev, sel: nextUp.id, ...fresh }))}>
                      Next patient: {nextUp.name}
                    </Secondary>
                  )}
                </motion.div>
              )}
            </AnimatePresence>

            {chips.length > 0 && (
              <div className="mt-6">
                <h3 className="m-0 text-section font-medium">{chipsTitle}</h3>
                <div role="group" aria-label={chipsTitle} className="mt-2 flex flex-wrap gap-2">
                  {chips.map(([label, pick]) => (
                    <button
                      key={label}
                      onClick={pick}
                      className={cn("h-control cursor-pointer rounded-chip border bg-surface px-3 text-body text-ink hover:bg-ground", chipsLive ? "border-ink" : "border-control")}
                    >
                      {label}
                    </button>
                  ))}
                </div>
              </div>
            )}

            <div className="mt-7">
              <h3 className="m-0 mb-3 text-section font-medium">Contacts</h3>
              <div aria-live="polite">
                <SealedChain nodes={trail} />
              </div>
            </div>
          </section>
        ) : (
          <section aria-label="Open patient" className="min-w-0 flex-[1_1_520px] rounded-card border border-line bg-surface p-pad">
            <h2 className="m-0 text-section font-medium">Nobody to open</h2>
            <p className="m-0 mt-2 text-muted">This view is empty. Choose another tab.</p>
          </section>
        )}
      </div>
    </>
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
  return <button {...props} className={cn("h-control cursor-pointer rounded-control bg-active px-3.5 text-body text-ink hover:bg-control", className)} />;
}
