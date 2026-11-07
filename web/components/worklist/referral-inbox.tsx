"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { Avatar } from "@/components/avatar/avatar";
import { RecordAvatar } from "@/components/wasl/record-avatar";
import { SealedChain, type ChainNode } from "@/components/wasl/sealed-chain";
import { caseHref } from "@/lib/domain/ids";
import { cn } from "@/lib/utils";
import type { Referral, ReferralState } from "./referrals";
import { rememberSearchFocus, SearchField } from "./search-field";
import { TAB_FOCUS, useRovingTabs } from "./use-roving-tabs";

/* Every action here is session state: the API has no referral-inbox endpoint. */

const ME = "Dr Layla Haddad";
const CONDITIONS = ["Hold blood thinners as advised", "Arabic interpreter on the day", "Female endoscopist", "Kidney function before booking"];
const REASONS = ["Needs a clinic visit first", "Blood thinner plan missing", "Wrong unit", "Not fit for colonoscopy"];
const GRADES: [string, string][] = [
  ["0", "Not seen"],
  ["1", "Partly seen"],
  ["2", "Mostly seen"],
  ["3", "All seen"],
];

type Tab = "needs" | "all" | "returned" | "accepted";
const TABS: [Tab, string, (r: Referral) => boolean][] = [
  ["needs", "Needs you", (r) => r.state === "new" || r.state === "grade"],
  ["all", "All", () => true],
  ["returned", "Waiting on GP", (r) => r.state === "returned"],
  ["accepted", "Accepted", (r) => r.state === "accepted" || r.state === "done"],
];
const RANK: Record<ReferralState, number> = { new: 0, grade: 1, returned: 2, accepted: 3, done: 4 };

type Override = { state: ReferralState; outcome: string; log: string[] };
type Mode = null | "cond" | "ret";

export function ReferralInbox({
  referrals,
  window,
  actBy,
  q,
  aside,
}: {
  referrals: Referral[];
  /** Days the unit has to answer (dashboard `sla.referral_response_days`). */
  window: number;
  actBy: string;
  q: string;
  aside: React.ReactNode;
}) {
  const [tab, setTab] = useState<Tab>("needs");
  const [sel, setSel] = useState<string | null>(null);
  const [mode, setMode] = useState<Mode>(null);
  const [picked, setPicked] = useState<string[]>([]);
  const [over, setOver] = useState<Record<string, Override>>({});
  /** Below lg the list and the open referral take turns; from lg up both show. */
  const [pane, setPane] = useState<"list" | "detail">("detail");
  const reduce = useReducedMotion();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const rowRef = useRef<HTMLButtonElement>(null);
  const prevPane = useRef(pane);
  useEffect(() => {
    if (prevPane.current === pane) return;
    prevPane.current = pane;
    if (globalThis.matchMedia("(min-width: 1024px)").matches) return;
    (pane === "detail" ? headingRef : rowRef).current?.focus();
  }, [pane]);

  /* Where focus goes after the control that had it is removed. Resolved after the render that removes it. */
  const root = useRef<HTMLDivElement>(null);
  const [want, setWant] = useState<{ k: string } | null>(null);
  useEffect(() => {
    if (want) root.current?.querySelector<HTMLElement>(`[data-focus="${want.k}"]`)?.focus();
  }, [want]);

  const selectTab = (id: Tab) => {
    setTab(id);
    setSel(null);
    setMode(null);
    setPicked([]);
  };
  const roving = useRovingTabs({ ids: TABS.map(([id]) => id), selected: tab, onSelect: selectTab, prefix: "referrals" });

  const all = referrals.map((r) => ({ ...r, state: over[r.id]?.state ?? r.state }));
  const sorted = [...all].sort((a, b) => RANK[a.state] - RANK[b.state] || b.waiting - a.waiting);
  const test = TABS.find(([id]) => id === tab)![2];
  const list = sorted.filter(test);
  const r = list.find((x) => x.id === sel) ?? (sel ? all.find((x) => x.id === sel) : undefined) ?? list[0];
  const needCount = all.filter(TABS[0][2]).length;

  const late = sorted.filter((x) => x.state === "new" && x.waiting > window);
  const grade = sorted.find((x) => x.state === "grade");
  const finding = [
    late[0] && `${late[0].name} has waited ${late[0].waiting} days, ${late[0].waiting - window} past the unit's ${window}.`,
    grade && `${grade.name}'s report needs your review.`,
  ]
    .filter(Boolean)
    .join(" ");

  const open = (id: string) => {
    setSel(id);
    setPane("detail");
    setMode(null);
    setPicked([]);
  };
  const act = (id: string, state: ReferralState, outcome: string, entry: string) => {
    setWant({ k: "undo" });
    setOver((o) => ({ ...o, [id]: { state, outcome, log: [...(o[id]?.log ?? []), entry] } }));
    open(id);
  };
  const accept = (x: Referral, conds: string[] = []) =>
    act(
      x.id,
      "accepted",
      conds.length ? `Accepted with conditions. ${conds.join("; ")}. Your unit books her now.` : "Accepted. Your unit books her now.",
      conds.length ? `Accepted with ${conds.length} ${conds.length === 1 ? "condition" : "conditions"}` : "Accepted the referral",
    );
  const undo = (id: string) => {
    setWant({ k: "heading" });
    setOver((o) => Object.fromEntries(Object.entries(o).filter(([k]) => k !== id)));
    open(id);
  };

  const chain: ChainNode[] = r
    ? [
        { id: "ref", kind: "human", who: `${r.clinic} GP`, what: "referred to SKMC endoscopy", when: r.sent },
        ...(over[r.id]?.log ?? []).map((what, i) => ({ id: `log-${i}`, kind: "human" as const, who: ME, what, when: "This session" })),
        ...(r.state === "done"
          ? []
          : [
              {
                id: "now",
                kind: "current" as const,
                who: r.state === "returned" ? `${r.clinic} GP` : r.state === "grade" ? "You" : "SKMC endoscopy",
                what: { new: "Your answer", grade: "Review the incomplete report", returned: "Act on the return", accepted: "Book and do the colonoscopy", done: "" }[r.state],
                when: r.state === "returned" ? `by ${actBy}` : `${r.waiting} days since referral`,
              },
            ]),
      ]
    : [];

  const choose = (m: Exclude<Mode, null>) => {
    setWant({ k: "mode" });
    setMode(m);
    setPicked([]);
  };
  const cancel = (id: string, opener: string) => {
    open(id);
    setWant({ k: opener });
  };

  const chip = (on: boolean) =>
    cn("h-11 cursor-pointer rounded-chip border px-4 text-body text-ink", on ? "border-ink bg-active" : "border-control bg-surface hover:bg-ground");
  const lateOpen = !!r && r.state === "new" && r.waiting > window;
  const outcome = r && over[r.id]?.outcome;

  return (
    <div ref={root}>
      <header className="mb-6 flex flex-wrap items-end justify-between gap-x-6 gap-y-2">
        <div>
          <h1 className="m-0 text-title font-semibold">Referrals</h1>
          <p className="m-0 mt-1 text-muted" data-testid="finding">
            {finding || "Nothing needs you right now."}
          </p>
        </div>
        {aside}
      </header>

      <div className="flex flex-wrap items-stretch gap-gap">
        <section aria-label="Referral list" className={cn("min-w-0 flex-[1_1_300px] flex-col rounded-card border border-line bg-surface lg:flex lg:flex-[0_1_420px]", pane === "list" ? "flex" : "hidden")}>
          <div {...roving.listProps} aria-label="Saved views" className="grid grid-cols-2 gap-x-4 border-b border-line px-4 sm:flex sm:overflow-x-auto">
            {TABS.map(([id, label, t]) => (
              <button
                key={id}
                {...roving.tabProps(id)}
                onClick={() => selectTab(id)}
                className={cn("-mb-px h-11 flex-none cursor-pointer whitespace-nowrap border-b-2 bg-transparent text-start text-body", TAB_FOCUS, tab === id ? "border-ink font-medium text-ink" : "border-transparent text-muted")}
              >
                {label} <span className="text-small text-muted">{all.filter(t).length}</span>
              </button>
            ))}
          </div>
          <form action="/app/referrals" onSubmit={() => rememberSearchFocus("referrals")} className="px-4 pb-2 pt-3">
            <SearchField label="Search case id" name="q" defaultValue={q} placeholder="Case id, then Enter" restoreKey="referrals" />
          </form>
          <div role="tabpanel" id={roving.panelId} aria-labelledby={roving.tabId(tab)} tabIndex={list.length ? undefined : 0} className="flex flex-1 flex-col gap-0.5 px-2">
            {list.map((x) => {
              const on = r?.id === x.id;
              const isLate = x.state === "new" && x.waiting > window;
              const inline = x.state === "new" && x.flags.length === 0;
              const review = (x.state === "new" && x.flags.length > 0) || x.state === "grade";
              return (
                <div key={x.id} className={cn("relative flex min-h-[68px] items-center gap-1 rounded-control px-1", on ? "bg-active" : "hover:bg-ground")}>
                  <div className="flex size-11 flex-none items-center justify-center self-start">
                    <RecordAvatar id={x.id} name={x.name} linked={x.hasRecord} className="relative z-10 size-11 items-center justify-center">
                      <Avatar person={x.seed} size={24} label="" />
                    </RecordAvatar>
                  </div>
                  <button
                    ref={on ? rowRef : undefined}
                    aria-pressed={on}
                    onClick={() => open(x.id)}
                    className="min-w-0 flex-1 cursor-pointer bg-transparent py-2.5 text-left after:absolute after:inset-0 after:rounded-control"
                  >
                    <span className="flex min-w-0 flex-wrap items-baseline gap-x-2">
                      <span className="font-semibold">{x.name}</span>
                      {x.nameAr && (
                        <span lang="ar" dir="rtl" className="text-muted">
                          {x.nameAr}
                        </span>
                      )}
                      {(x.state === "new" || x.state === "grade") && (
                        <span className={cn("whitespace-nowrap text-small", isLate ? "font-medium text-negative" : "text-muted")}>{x.waiting} days</span>
                      )}
                    </span>
                    <span className="mt-0.5 block text-muted">{lineFor(x, over[x.id])}</span>
                  </button>
                  <span className="flex-none pr-1">
                    {inline && (
                      <button onClick={() => accept(x)} aria-label={`Accept referral for ${x.name}`} className="relative z-10 h-11 cursor-pointer rounded-control bg-cta px-4 text-body font-medium text-white">
                        Accept
                      </button>
                    )}
                    {review && (
                      <button onClick={() => open(x.id)} aria-label={`Review the referral for ${x.name}`} className="relative z-10 h-11 cursor-pointer rounded-control bg-active px-4 text-body text-ink hover:bg-control">
                        Review
                      </button>
                    )}
                    {!inline && !review && <span className="pr-2 text-small text-muted">{{ returned: "Returned", accepted: "Accepted", done: "Closed", new: "", grade: "" }[x.state]}</span>}
                  </span>
                </div>
              );
            })}
            {list.length === 0 && <p className="m-2 rounded-card border border-dashed border-control px-3 py-2.5 text-muted">No referrals in this view.</p>}
          </div>
          <div className="border-t border-rule px-4 py-3 text-small text-muted">
            {list.length} {list.length === 1 ? "referral" : "referrals"} · {needCount} need you{q && ` · matching “${q}”`}
          </div>
        </section>

        {r ? (
          <section aria-label="Open referral" className={cn("min-w-0 flex-[1_1_520px] flex-col rounded-card border border-line bg-surface p-4 sm:p-pad lg:flex", pane === "list" ? "hidden" : "flex")}>
            <button onClick={() => setPane("list")} className="-mt-1 mb-3 flex min-h-11 cursor-pointer items-center gap-2 self-start rounded-control bg-transparent pr-3 text-body text-ink lg:hidden">
              <span aria-hidden>‹</span>
              All referrals · {list.length}
            </button>
            <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between sm:gap-6">
              <div className="flex min-w-0 items-start gap-4">
                <RecordAvatar id={r.id} name={r.name} linked={r.hasRecord}>
                  <Avatar person={r.seed} size={56} label="" />
                </RecordAvatar>
                <div className="min-w-0">
                  <div className="flex flex-wrap items-baseline gap-x-3">
                    <h2 ref={headingRef} data-focus="heading" tabIndex={-1} className="m-0 text-title font-semibold outline-none">
                      {r.name}
                    </h2>
                    {r.nameAr && (
                      <span lang="ar" dir="rtl" className="text-section text-muted">
                        {r.nameAr}
                      </span>
                    )}
                  </div>
                  <p className="m-0 mt-1 text-muted">{[r.age, `from ${r.clinic} Healthcare Centre`].filter(Boolean).join(" · ")}</p>
                  {r.hasRecord && (
                    <Link href={caseHref("/app/patients", r.id)} className="inline-flex min-h-11 items-center underline">
                      Open record
                    </Link>
                  )}
                </div>
              </div>
              <div className="sm:flex-none sm:text-right">
                <div className="flex items-baseline gap-1.5 sm:justify-end">
                  <span data-testid="lead-days" className={cn("text-lead font-bold", lateOpen ? "text-negative" : "text-ink")}>
                    {r.waiting}
                  </span>
                  <span className="text-muted">{r.state === "new" ? "days waiting" : "days since referral"}</span>
                </div>
                <div className={cn("mt-0.5 text-body sm:text-small", lateOpen ? "text-negative" : "text-muted")}>
                  {r.state === "new" ? (lateOpen ? `${r.waiting - window} past the unit's ${window}` : `unit answers within ${window}`) : `sent ${r.sent}`}
                </div>
              </div>
            </div>

            {r.flags.length > 0 && r.state !== "done" && (
              <p className="m-0 mt-3 text-body" data-testid="flags">
                <span className="font-medium">Check first:</span> <span className="text-muted">{r.flags.join(" · ")}</span>
              </p>
            )}

            <div className="mt-4">
              {r.state === "new" && mode === null && (
                <div className="flex flex-wrap gap-gap">
                  <button onClick={() => accept(r)} className="h-11 cursor-pointer rounded-control bg-cta px-4 text-body font-medium text-white">
                    Accept referral
                  </button>
                  <Secondary data-focus="open-cond" onClick={() => choose("cond")}>Accept with conditions</Secondary>
                  <Secondary data-focus="open-ret" onClick={() => choose("ret")}>Return to GP</Secondary>
                </div>
              )}
              {r.state === "new" && mode === "cond" && (
                <div>
                  <h3 data-focus="mode" tabIndex={-1} className="m-0 text-section font-medium">
                    Accept on these conditions
                  </h3>
                  <div role="group" aria-label="Conditions" className="mt-2 flex flex-wrap gap-2">
                    {CONDITIONS.map((c) => (
                      <button key={c} aria-pressed={picked.includes(c)} className={chip(picked.includes(c))} onClick={() => setPicked((p) => (p.includes(c) ? p.filter((x) => x !== c) : [...p, c]))}>
                        {c}
                      </button>
                    ))}
                  </div>
                  <div className="mt-3 flex gap-gap">
                    <button onClick={() => accept(r, picked)} className="h-11 cursor-pointer rounded-control bg-cta px-4 text-body font-medium text-white">
                      {picked.length ? `Accept with ${picked.length} ${picked.length === 1 ? "condition" : "conditions"}` : "Accept with no conditions"}
                    </button>
                    <Secondary onClick={() => cancel(r.id, "open-cond")}>Cancel</Secondary>
                  </div>
                </div>
              )}
              {r.state === "new" && mode === "ret" && (
                <div>
                  <h3 data-focus="mode" tabIndex={-1} className="m-0 text-section font-medium">
                    Why return this referral?
                  </h3>
                  <div role="group" aria-label="Reason" className="mt-2 flex flex-wrap gap-2">
                    {REASONS.map((c) => (
                      <button key={c} aria-pressed={picked[0] === c} className={chip(picked[0] === c)} onClick={() => setPicked([c])}>
                        {c}
                      </button>
                    ))}
                  </div>
                  <p className="m-0 mt-2 text-small text-muted">Returning hands the case back to the {r.clinic} GP, who must act by {actBy}.</p>
                  <div className="mt-3 flex items-center gap-gap">
                    {picked[0] ? (
                      <button
                        onClick={() => act(r.id, "returned", `Returned to the ${r.clinic} GP. Reason: ${picked[0].toLowerCase()}. Act by ${actBy}.`, `Returned to the GP: ${picked[0].toLowerCase()}`)}
                        className="h-11 cursor-pointer rounded-control bg-cta px-4 text-body font-medium text-white"
                      >
                        Return to GP
                      </button>
                    ) : (
                      <span className="text-muted">Choose a reason to return it.</span>
                    )}
                    <Secondary onClick={() => cancel(r.id, "open-ret")}>Cancel</Secondary>
                  </div>
                </div>
              )}
              {r.state === "grade" && (
                <div>
                  <p className="m-0">The report does not state the prep or that the caecum was reached. Grade the prep from your procedure record.</p>
                  <div role="group" aria-label="Boston grade" className="mt-2 flex flex-wrap gap-2">
                    {GRADES.map(([n, w]) => (
                      <button
                        key={n}
                        onClick={() => act(r.id, "accepted", `Graded ${n}, ${w.toLowerCase()}. The case stays held until the report is complete.`, `Graded the prep ${n}`)}
                        className="flex h-14 min-w-16 cursor-pointer flex-col items-center justify-center rounded-chip border border-control bg-surface px-3 hover:bg-ground"
                      >
                        <span className="font-medium">{n}</span>
                        <span className="text-small text-muted">{w}</span>
                      </button>
                    ))}
                  </div>
                  <p className="m-0 mt-2 text-small text-muted">A grade never closes a case. Only a complete report does.</p>
                </div>
              )}
              <AnimatePresence initial={false}>
                {outcome && (
                  <motion.div key="outcome" role="status" initial={reduce ? false : { opacity: 0, y: -4 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0 }} transition={{ duration: reduce ? 0 : 0.15 }} className="flex flex-wrap items-center gap-gap">
                    <p className="m-0 flex-1">{outcome}</p>
                    <Secondary data-focus="undo" onClick={() => undo(r.id)}>Undo</Secondary>
                  </motion.div>
                )}
              </AnimatePresence>
              {!outcome && (r.state === "accepted" || r.state === "done" || r.state === "returned") && (
                <p className="m-0" role="status">
                  {{ accepted: "Accepted. Nothing needed from you.", done: "Closed with evidence. Nothing needed from you.", returned: "Returned to the GP.", new: "", grade: "" }[r.state]}
                </p>
              )}
            </div>

            <h3 className="m-0 mt-7 text-section font-medium">Hand-off</h3>
            <div aria-live="polite" className="mt-3">
              <SealedChain nodes={chain} />
            </div>

            <h3 className="m-0 mt-6 text-section font-medium">Referral details</h3>
            <dl className="m-0 mt-2 grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2">
              {r.details.map(([k, v]) => (
                <div key={k}>
                  <dt className="text-small text-muted">{k}</dt>
                  <dd className={cn("m-0", k === "Case" && "font-mono text-small text-muted")}>{v}</dd>
                </div>
              ))}
            </dl>
          </section>
        ) : (
          <section aria-label="Open referral" className={cn("min-w-0 flex-[1_1_520px] rounded-card border border-line bg-surface p-4 sm:p-pad lg:block", pane === "list" ? "hidden" : "block")}>
            <button onClick={() => setPane("list")} className="mb-3 flex min-h-11 cursor-pointer items-center gap-2 bg-transparent text-body text-ink lg:hidden">
              <span aria-hidden>‹</span>
              All referrals
            </button>
            <h2 className="m-0 text-section font-medium">Nothing to open</h2>
            <p className="m-0 mt-2 text-muted">This view is empty. Choose another tab or clear the search.</p>
          </section>
        )}
      </div>
    </div>
  );
}

function lineFor(x: Referral, o: Override | undefined): string {
  if (o) return o.log.at(-1)!;
  switch (x.state) {
    case "new":
      return x.flags.length ? x.flags.join(" · ") : "Ready to accept";
    case "grade":
      return "Report incomplete · grade the prep";
    case "returned":
      return "Returned to GP · waiting on the GP";
    case "accepted":
      return "Accepted · in progress";
    case "done":
      return "Closed with evidence";
  }
}

function Secondary(props: React.ComponentProps<"button">) {
  return <button {...props} className="h-11 cursor-pointer rounded-control bg-active px-4 text-body text-ink hover:bg-control" />;
}
