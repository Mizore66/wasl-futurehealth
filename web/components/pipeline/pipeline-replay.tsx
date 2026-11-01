"use client";

import { useMemo, useState } from "react";
import NumberFlow from "@number-flow/react";
import { AnimatePresence, LayoutGroup, motion, useReducedMotion } from "motion/react";
import replayJson from "@/data/replay.json";
import { SealedChain, type ChainNode } from "@/components/wasl/sealed-chain";
import type { Replay } from "@/lib/replay/types";
import { cn } from "@/lib/utils";
import { MarkShape, PlaybackControls } from "./controls";
import { FEED_ROWS, flowView, replayTrack, type FlowRow } from "./flow";
import { usePlayback } from "./playback";

const replay = replayJson as Replay;
const track = replayTrack(replay);
const TICK_MS = 1000;
const fmt = (n: number) => n.toLocaleString("en-US");

export type TamperDemo = {
  title: string;
  intact: ChainNode[];
  tampered: ChainNode[];
  /** 1-based index of the changed step. */
  step: number;
  changed: string;
};

export function PipelineReplay({ heading, tamper }: { heading: React.ReactNode; tamper: TamperDemo }) {
  const play = usePlayback(track, TICK_MS);
  const reduceMotion = useReducedMotion();
  const view = useMemo(() => flowView(replay, play.frame), [play.frame]);
  const [serverOff, setServerOff] = useState(false);
  const [tampered, setTampered] = useState(false);
  const { facts, totals } = replay;

  return (
    <>
      <header className="flex flex-wrap items-end justify-between gap-6">
        {heading}
        <PlaybackControls
          paused={play.paused}
          onToggle={play.toggle}
          onStep={play.step}
          position={`Step ${view.step} of ${view.steps}`}
          speed={`One step a second. The recorded run handled ${facts.throughputEvS} events a second.`}
        />
      </header>

      <div className="mt-6 grid grid-cols-1 gap-3 xl:grid-cols-[minmax(0,7fr)_minmax(0,5fr)]">
        <section aria-labelledby="flow-h" className="rounded-card border border-line bg-surface px-6 pb-3 pt-5">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <h2 id="flow-h" className="m-0 text-section font-medium">
                Case flow
              </h2>
              <p className="m-0 text-small text-muted">One chip per case. The blue chip is the case that just moved.</p>
            </div>
            <ul aria-label="Chip marks" className="m-0 mt-1 flex list-none gap-3 whitespace-nowrap p-0 text-small text-muted">
              {(["queued", "working", "done", "held"] as const).map((m) => (
                <li key={m} className="flex items-center gap-1.5">
                  <MarkShape mark={m} />
                  {m === "queued" ? "waiting" : m}
                </li>
              ))}
            </ul>
          </div>
          <LayoutGroup>
            <ol
              aria-label={`Case flow, 8 stages. ${view.rows.map((r) => `${r.label} ${r.count}`).join(", ")}`}
              className="m-0 mt-3 list-none p-0"
            >
              {view.rows.map((r) => (
                <StageRow key={r.id} row={r} latest={r.id === "ledger" ? view.latestRecorded : null} />
              ))}
            </ol>
          </LayoutGroup>
        </section>

        <section aria-labelledby="feed-h" className="rounded-card border border-line bg-surface px-6 pb-3 pt-5">
          <h2 id="feed-h" className="m-0 text-section font-medium">
            Decisions
          </h2>
          <p className="m-0 text-small text-muted">Newest first. Plain words, then the system&apos;s own label.</p>
          <ol aria-label="Decisions, newest first" className="m-0 mt-3 list-none overflow-hidden p-0" style={{ height: FEED_ROWS * 56 }}>
            <AnimatePresence initial={false} mode="popLayout">
              {view.feed.map((f) => (
                <motion.li
                  key={f.key}
                  layout
                  initial={reduceMotion ? false : { opacity: 0, y: -8 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, transition: { duration: 0 } }}
                  transition={{ duration: 0.32, ease: "easeOut" }}
                  className="grid h-14 grid-cols-[8px_minmax(0,1fr)_auto] items-start gap-x-3 border-t border-rule py-2"
                >
                  <MarkShape mark={f.mark} hue={f.key === view.step - 1} className="mt-1.5" />
                  <div className="min-w-0">
                    <div className="truncate text-body">{f.text}</div>
                    <div className="truncate text-small text-muted">
                      <span className={cn(f.key === view.step - 1 && "font-medium text-hue")}>{f.caseLabel}</span> · {f.tech}
                    </div>
                  </div>
                  <span className="text-small text-muted">step {f.step}</span>
                </motion.li>
              ))}
            </AnimatePresence>
          </ol>
        </section>
      </div>

      <div className="mt-3 grid grid-cols-1 gap-3 xl:grid-cols-[minmax(0,7fr)_minmax(0,5fr)]">
        <section
          aria-label="Run totals and a server switched off"
          className="grid grid-cols-3 gap-x-6 gap-y-5 rounded-card border border-line bg-surface p-6"
        >
          <Total label="Decisions recorded" note={`of ${fmt(totals.outputs)} in the run`}>
            <span data-testid="total-decisions" data-value={view.decisions}>
              <NumberFlow value={view.decisions} locales="en-US" />
            </span>
          </Total>
          <Total label="Lost · duplicated" note="every decision exactly once">
            {facts.lost} · {facts.duplicates}
          </Total>
          <Total label="Records verified" note={`of ${fmt(totals.chainsVerified)} sealed chains`}>
            <span data-testid="total-chains" data-value={view.chains}>
              <NumberFlow value={view.chains} locales="en-US" />
            </span>
          </Total>

          <div className="col-span-3 flex flex-col gap-2 border-t border-rule pt-4">
            <div className="flex items-center justify-between gap-3">
              <div
                role="img"
                aria-label={serverOff ? "Three servers: server 2 off, servers 1 and 3 carry every event." : "Three servers, all on."}
                className="flex items-center gap-1.5"
              >
                {[0, 1, 2].map((i) =>
                  serverOff && i === 1 ? (
                    <MarkShape key={i} mark="failed" className="size-2.5" />
                  ) : (
                    <MarkShape key={i} mark="done" className="size-2.5" />
                  ),
                )}
                <span className="ml-1 text-small text-muted">{serverOff ? "Server 2 of 3 off" : "3 servers on"}</span>
              </div>
              <button
                type="button"
                onClick={() => setServerOff((v) => !v)}
                className="h-9 flex-none cursor-pointer whitespace-nowrap rounded-control bg-active px-3.5 text-body font-medium text-ink"
              >
                {serverOff ? "Switch it back on" : "Switch off a server"}
              </button>
            </div>
            <p aria-live="polite" className="m-0 text-small text-muted" data-testid="server-result">
              {serverOff
                ? `Server 2 off: still ${facts.lost} lost, ${facts.duplicates} duplicated. The switch is simulated here; the figures are the recorded run's.`
                : `In the recorded run, ${facts.lost} of ${fmt(totals.outputs)} decisions were lost and ${facts.duplicates} duplicated.`}
            </p>
          </div>
        </section>

        <section aria-labelledby="tamper-h" className="rounded-card border border-line bg-surface p-6">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <h2 id="tamper-h" className="m-0 text-section font-medium">
                {tamper.title}
              </h2>
              <span
                role="status"
                className={cn(
                  "mt-1 inline-flex h-[22px] items-center gap-1.5 whitespace-nowrap rounded-chip px-2 text-small font-medium",
                  tampered ? "bg-pill-negative-bg text-pill-negative-ink" : "bg-pill-positive-bg text-pill-positive-ink",
                )}
              >
                <svg aria-hidden width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                  <path d={tampered ? "M3 3l6 6M9 3l-6 6" : "M2.5 6.5l2.5 2.5 4.5-6"} />
                </svg>
                {tampered ? `Changed record detected at step ${tamper.step}` : "Records intact"}
              </span>
            </div>
            <button
              type="button"
              onClick={() => setTampered((v) => !v)}
              className="h-9 flex-none cursor-pointer whitespace-nowrap rounded-control bg-active px-3.5 text-body font-medium text-ink"
            >
              {tampered ? "Undo the change" : "Tamper with a step"}
            </button>
          </div>
          <SealedChain nodes={tampered ? tamper.tampered : tamper.intact} className="mt-4" />
          <p aria-live="polite" className="m-0 mt-3 text-small text-muted">
            {tampered
              ? `${tamper.changed} Simulated here; in the recorded run ${facts.tamperDetected} of ${facts.tamperAttacks} tamper attacks were caught.`
              : `In the recorded run, ${facts.tamperDetected} of ${facts.tamperAttacks} tamper attacks were caught: deleted, reordered, truncated and rewritten records.`}
          </p>
        </section>
      </div>
    </>
  );
}

function Total({ label, note, children }: { label: string; note: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <div className="text-small text-muted">{label}</div>
      <div className="mt-1 text-title font-medium tabular-nums">{children}</div>
      <div className="text-small text-muted">{note}</div>
    </div>
  );
}

function StageRow({ row, latest }: { row: FlowRow; latest: ReturnType<typeof flowView>["latestRecorded"] }) {
  return (
    <li data-stage={row.id} className="grid h-12 grid-cols-[204px_minmax(0,1fr)_32px] items-center gap-x-3 border-t border-rule">
      <div className={cn("flex min-w-0 items-center", row.branch && "pl-1")}>
        {row.branch && (
          <svg aria-hidden width="10" height="12" viewBox="0 0 10 12" fill="none" stroke="var(--color-control)" strokeWidth="1.5" className="mr-1.5 flex-none">
            <path d="M1.5 0v6.5h7" />
          </svg>
        )}
        <span className="flex min-w-0 flex-col">
          <span className="truncate text-body">{row.label}</span>
          <span className="truncate text-small text-muted">{row.tech}</span>
        </span>
      </div>
      <div className="flex min-w-0 items-center gap-1.5">
        {row.id === "ledger" ? (
          <span className="truncate text-small text-muted">
            {latest ? `Latest: ${latest.caseLabel} · ${latest.closed ? "chain verified" : "decision sealed"}` : "Nothing recorded yet in this loop"}
          </span>
        ) : (
          <>
            {row.chips.map((c) => (
              <motion.span
                key={c.id}
                layoutId={c.id}
                transition={{ duration: 0.45, ease: [0.2, 0.7, 0.2, 1] }}
                className={cn(
                  "inline-flex h-6 w-[84px] flex-none items-center gap-1.5 whitespace-nowrap rounded-chip border bg-surface px-2 text-small",
                  c.active ? "border-hue font-medium text-ink" : "border-line text-muted",
                )}
              >
                <MarkShape mark={c.mark} hue={c.active} />
                {c.label}
              </motion.span>
            ))}
            {row.more > 0 && <span className="w-[84px] flex-none text-small text-muted">+{row.more} more</span>}
          </>
        )}
      </div>
      <span className={cn("text-right text-body tabular-nums", row.count ? "text-ink" : "text-muted")}>{row.count}</span>
    </li>
  );
}
