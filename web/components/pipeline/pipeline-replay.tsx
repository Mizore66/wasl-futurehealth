"use client";

import { useMemo, useState } from "react";
import NumberFlow from "@number-flow/react";
import { LayoutGroup, motion, useReducedMotion } from "motion/react";
import replayJson from "@/data/replay.json";
import { Avatar } from "@/components/avatar/avatar";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { SealedChain, type ChainNode } from "@/components/wasl/sealed-chain";
import type { Replay } from "@/lib/replay/types";
import { cn } from "@/lib/utils";
import { MarkShape, PlaybackControls } from "./controls";
import { FEED_ROWS, flowView, replayTrack, type Chip, type FlowRow, type RowId } from "./flow";
import { TechnicalDetail } from "./technical";
import { stepAnnouncement } from "./health";
import { PersonSheet, profileLink, useProfileParam } from "./person-sheet";
import { usePlayback } from "./playback";
import { profileOf, type RulesBySha } from "./profile";
import "./pipeline.css";

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
  /** The backend's own entry types and hashes, for the Technical detail disclosure. */
  technical: string[];
};

type LaneMark = "closed" | "review" | "open";

/** The outcome lanes: the theme's status colour, a shape and a word, so no lane depends on colour alone. */
const LANE: Partial<Record<RowId, { mark: LaneMark; bg: string; ink: string; edge: string }>> = {
  closed: { mark: "closed", bg: "bg-pill-positive-bg", ink: "text-positive", edge: "border-positive" },
  review: { mark: "review", bg: "bg-pill-warning-bg", ink: "text-warning", edge: "border-warning" },
  open: { mark: "open", bg: "bg-pill-negative-bg", ink: "text-negative", edge: "border-negative" },
};

const CHIP_MOVE = { duration: 0.5, ease: [0.2, 0.7, 0.2, 1] } as const;
const FEED_PITCH = 72;

type Open = (caseId: string) => void;

export function PipelineReplay({ heading, tamper, rules, health }: { heading: React.ReactNode; tamper: TamperDemo; rules: RulesBySha; health?: React.ReactNode }) {
  const profile = useProfileParam();
  const play = usePlayback(track, TICK_MS, profile.id !== null);
  const shown = useMemo(() => (profile.id ? profileOf(replay, play.frame, profile.id, rules) : null), [profile.id, play.frame, rules]);
  const reduceMotion = !!useReducedMotion();
  const view = useMemo(() => flowView(replay, play.frame), [play.frame]);
  const [tampered, setTampered] = useState(false);
  // Announced only when the user presses Step (issue #16). The counter re-mounts the text so an identical message is read again.
  const [stepNote, setStepNote] = useState({ n: 0, text: "" });
  const onStep = () => {
    setStepNote(({ n }) => ({ n: n + 1, text: stepAnnouncement(flowView(replay, track.advance(play.frame))) }));
    play.step();
  };
  const { facts, totals } = replay;
  const stages = view.rows.filter((r) => !LANE[r.id] && r.id !== "ledger");
  const lanes = view.rows.filter((r) => LANE[r.id]);
  const ledger = view.rows.find((r) => r.id === "ledger")!;

  return (
    <>
      <header className="flex flex-wrap items-end justify-between gap-6">
        {heading}
        <PlaybackControls
          paused={play.paused}
          onToggle={play.toggle}
          onStep={onStep}
          reduceMotion={play.reduceMotion}
          position={`Step ${view.step} of ${view.steps}`}
          speed={`One step a second. The recorded run handled ${facts.throughputEvS} events a second.`}
        />
      </header>

      <div role="status" aria-live="polite" aria-atomic="true" data-testid="step-announcer" className="sr-only">
        <span key={stepNote.n}>{stepNote.text}</span>
      </div>

      <section aria-labelledby="flow-h" className="mt-8">
        <div className="flex flex-wrap items-baseline justify-between gap-x-6 gap-y-1">
          <h2 id="flow-h" className="m-0 text-section font-medium">
            Case flow
          </h2>
          <p className="m-0 text-small text-muted">
            A report shows the name as it is written on it, and &ldquo;?&rdquo;, until it is matched to a referral. The ringed one just moved. Open
            anyone to see their steps.
          </p>
        </div>
        <LayoutGroup>
          <div className="mt-3 rounded-card border border-line bg-surface">
            <ol
              aria-label={`Case flow, 8 stages. ${view.rows.map((r) => `${r.label} ${r.count}`).join(", ")}`}
              className="m-0 grid list-none grid-cols-1 p-0 lg:grid-cols-[repeat(4,minmax(0,1fr))_minmax(0,1.5fr)]"
            >
              {stages.map((r) => (
                <StageColumn key={r.id} row={r} reduceMotion={reduceMotion} onOpen={profile.open} />
              ))}
              <li className="flex min-w-0 flex-col gap-1 px-4 pb-4 pt-4">
                <span className="px-1 text-small text-muted">Outcome</span>
                {lanes.map((r) => (
                  <OutcomeLane key={r.id} row={r} reduceMotion={reduceMotion} onOpen={profile.open} />
                ))}
              </li>
            </ol>
            <div data-stage="ledger" className="flex flex-wrap items-baseline gap-x-4 gap-y-1 border-t border-rule px-5 py-3">
              <span className="text-body font-medium">{ledger.label}</span>
              <span className="text-small text-muted">{ledger.note}</span>
              <span className="min-w-0 flex-1 truncate text-small text-muted">
                {view.latestRecorded
                  ? `Latest: ${view.latestRecorded.who.name} · ${view.latestRecorded.closed ? "chain verified" : "decision sealed"}`
                  : "Nothing recorded yet in this loop"}
              </span>
              <span className="text-body tabular-nums">{ledger.count}</span>
            </div>
            <div className="border-t border-rule px-5">
              <TechnicalDetail label="Technical detail: the system's names for these stages">
                {stages.concat(lanes, [ledger]).map((r) => (
                  <div key={r.id}>
                    {r.label} = {r.tech}
                  </div>
                ))}
              </TechnicalDetail>
            </div>
          </div>
        </LayoutGroup>
      </section>

      <div className="mt-10 grid grid-cols-1 gap-x-10 gap-y-8 xl:grid-cols-[minmax(0,4fr)_minmax(0,5fr)_minmax(0,5fr)]">
        <section aria-labelledby="feed-h" className="min-w-0 border-t border-line pt-5">
          <h2 id="feed-h" className="m-0 text-body font-medium">
            Decisions
          </h2>
          <p className="m-0 text-small text-muted">Newest first. Plain words; the system&apos;s own labels are under Technical detail.</p>
          <ol aria-label="Decisions, newest first" className="relative m-0 mt-3 list-none overflow-hidden p-0" style={{ height: FEED_ROWS * FEED_PITCH }}>
            {view.feed.map((f, i) => {
              const latest = f.key === view.step - 1;
              return (
                <li
                  key={f.key}
                  className={cn("pipe-feed-row grid grid-cols-[8px_minmax(0,1fr)_auto] items-start gap-x-3 pt-1", latest && "pipe-feed-new")}
                  style={{ top: i * FEED_PITCH, height: FEED_PITCH }}
                >
                  <MarkShape mark={f.mark} hue={latest} className="mt-1.5" />
                  <div className="min-w-0">
                    <div className="truncate text-body">{f.text}</div>
                    <div className="mt-0.5 flex min-w-0 items-center gap-1.5 text-small text-muted">
                      <a
                        {...profileLink(f.caseId, profile.open)}
                        className="flex min-h-11 min-w-0 flex-none items-center gap-1.5 rounded-chip no-underline hover:underline"
                      >
                        <Avatar person={f.who.seed} state={f.who.face === "unknown" ? "unknown" : "known"} size={20} label="" />
                        <span className={cn("truncate", latest ? "font-medium text-hue" : "text-ink-soft")}>{f.who.name}</span>
                      </a>
                      <span className="min-w-0 truncate">· {f.note}</span>
                    </div>
                  </div>
                  <span className="text-small text-muted">step {f.step}</span>
                </li>
              );
            })}
          </ol>
          <TechnicalDetail label="Technical detail: the system's labels for these lines">
            {view.feed.map((f) => (
              <div key={f.key}>
                step {f.step} · {f.tech}
              </div>
            ))}
          </TechnicalDetail>
        </section>

        <section aria-label="Run totals and the recorded failover result" className="min-w-0 border-t border-line pt-5">
          <div className="grid grid-cols-3 gap-4">
            <Total label="Decisions recorded" note={`of ${fmt(totals.outputs)} in the run`}>
              <span data-testid="total-decisions" data-value={view.decisions}>
                <NumberFlow value={view.decisions} locales="en-US" />
              </span>
            </Total>
            <Total label="Lost · duplicated" note="each decision exactly once">
              {facts.lost} · {facts.duplicates}
            </Total>
            <Total label="Records verified" note={`of ${fmt(totals.chainsVerified)} sealed chains`}>
              <span data-testid="total-chains" data-value={view.chains}>
                <NumberFlow value={view.chains} locales="en-US" />
              </span>
            </Total>
          </div>

          <div className="mt-6 flex flex-col gap-2">
            <p className="m-0 flex flex-wrap items-center gap-2 text-body font-medium">
              Switching a server off during a run <HonestyLabel kind="designed" />
            </p>
            <p className="m-0 text-small text-muted" data-testid="server-result">
              There is no switch on this screen because live failover is not built. This is the result of the recorded run, not the state of any server now. It shows {facts.lost} of {fmt(totals.outputs)} decisions
              lost and {facts.duplicates} duplicated.
            </p>
          </div>
        </section>

        <section aria-labelledby="tamper-h" className="min-w-0 border-t border-line pt-5">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <h2 id="tamper-h" className="m-0 text-body font-medium">
                {tamper.title}
              </h2>
              <HonestyLabel kind="simulated" className="mt-1">
                Simulated edit
              </HonestyLabel>
              <span
                role="status"
                className={cn(
                  "ml-2 mt-1 inline-flex h-[22px] items-center gap-1.5 whitespace-nowrap rounded-chip px-2 text-small font-medium",
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
              className="h-11 flex-none cursor-pointer whitespace-nowrap rounded-control bg-active px-3.5 text-body font-medium text-ink hover:bg-control"
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
          <TechnicalDetail label="Technical detail: the system's record types">
            {tamper.technical.map((t) => (
              <div key={t}>{t}</div>
            ))}
          </TechnicalDetail>
        </section>
      </div>
      {health}
      <PersonSheet profile={shown} onClose={profile.close} />
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

function OutcomeMark({ mark }: { mark: LaneMark }) {
  return (
    <svg aria-hidden width="14" height="14" viewBox="0 0 14 14" className="flex-none">
      {mark === "closed" && (
        <>
          <circle cx="7" cy="7" r="6.5" fill="currentColor" />
          <path d="M4 7.2l2 2 4-4.4" fill="none" stroke="var(--color-surface)" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
        </>
      )}
      {mark === "review" && <rect x="1.5" y="1.5" width="11" height="11" rx="1.5" fill="currentColor" />}
      {mark === "open" && <circle cx="7" cy="7" r="5.5" fill="none" stroke="currentColor" strokeWidth="2" />}
    </svg>
  );
}

/** Two reader marks: both at work while the report is read, then each ticks as its answer lands. */
function ReaderMarks({ state, reduceMotion }: { state: "reading" | "read"; reduceMotion: boolean }) {
  return (
    <span role="img" aria-label={state === "read" ? "Both readers answered" : "Two readers reading"} className="flex flex-none gap-1">
      {[0, 1].map((i) => (
        <svg
          key={i}
          width="14"
          height="14"
          viewBox="0 0 14 14"
          className={cn("flex-none", state === "reading" && "wasl-pulse")}
          style={{ animationDelay: `${i * 0.3}s` }}
        >
          <circle cx="7" cy="7" r="6" fill={state === "read" ? "var(--color-hue)" : "var(--color-surface)"} stroke="var(--color-hue)" strokeWidth="1.5" />
          {state === "read" && (
            <motion.path
              d="M4.2 7.3l1.9 1.9 3.8-4.2"
              fill="none"
              stroke="var(--color-surface)"
              strokeWidth="1.6"
              strokeLinecap="round"
              strokeLinejoin="round"
              initial={reduceMotion ? false : { pathLength: 0 }}
              animate={{ pathLength: 1 }}
              transition={{ duration: 0.3, delay: i * 0.25 }}
            />
          )}
        </svg>
      ))}
    </span>
  );
}

function CaseChip({ chip, row, reduceMotion, onOpen, tint = "bg-ground" }: { chip: Chip; row: RowId; reduceMotion: boolean; onOpen: Open; tint?: string }) {
  return (
    <motion.a
      layoutId={chip.id}
      transition={reduceMotion ? { duration: 0 } : CHIP_MOVE}
      data-case={chip.id}
      {...profileLink(chip.id, onOpen)}
      className={cn(
        "inline-flex min-h-11 max-w-full cursor-pointer items-center gap-2 rounded-[26px] py-1 pl-1 pr-3 text-small leading-[15px] no-underline hover:text-ink hover:shadow-[inset_0_0_0_1px_var(--color-control)]",
        tint,
        chip.active ? "font-medium text-ink ring-2 ring-hue" : "text-ink-soft",
      )}
    >
      <Avatar person={chip.who.seed} state={chip.who.face} size={chip.active ? 44 : 32} label="" />
      <span className="line-clamp-2 min-w-0 break-words" data-testid="chip-name">
        {chip.who.name}
      </span>
      {row === "matching" && chip.mark === "working" && <MarkShape mark="working" hue />}
      {chip.readers && <ReaderMarks state={chip.readers} reduceMotion={reduceMotion} />}
      {row === "gate" && <MarkShape mark={chip.mark} hue={chip.mark === "done"} />}
    </motion.a>
  );
}

function StageColumn({ row, reduceMotion, onOpen }: { row: FlowRow; reduceMotion: boolean; onOpen: Open }) {
  return (
    <li data-stage={row.id} className="relative flex min-w-0 flex-col border-b border-rule px-5 pb-5 pt-4 lg:border-b-0 lg:border-r">
      <svg aria-hidden width="10" height="16" viewBox="0 0 10 16" className="absolute -right-[5px] top-[20px] z-10 hidden bg-surface text-control lg:block">
        <path d="M2 2l6 6-6 6" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
      <div className="flex items-baseline justify-between gap-2">
        <span className="text-body font-medium">{row.label}</span>
        <span className={cn("text-title tabular-nums", row.count ? "text-ink" : "text-muted")}>{row.count}</span>
      </div>
      <span className="text-small text-muted">{row.note}</span>
      <div className="mt-4 flex min-h-24 flex-row flex-wrap items-start gap-2 lg:min-h-[196px] lg:flex-col">
        {row.chips.map((c) => (
          <CaseChip key={c.id} chip={c} row={row.id} reduceMotion={reduceMotion} onOpen={onOpen} />
        ))}
        {row.more > 0 && <span className="pl-2 text-small text-muted">+{row.more} more</span>}
      </div>
    </li>
  );
}

function OutcomeLane({ row, reduceMotion, onOpen }: { row: FlowRow; reduceMotion: boolean; onOpen: Open }) {
  const lane = LANE[row.id]!;
  return (
    <div data-stage={row.id} className={cn("border-l-[3px] py-2 pl-3 pr-1", lane.edge)}>
      <div className="flex items-center gap-2">
        <span className={lane.ink}>
          <OutcomeMark mark={lane.mark} />
        </span>
        <span className="min-w-0 flex-1 text-body font-medium">{row.label}</span>
        <span className="text-body tabular-nums">{row.count}</span>
      </div>
      <span className="block pl-[22px] text-small text-muted">{row.note}</span>
      <div className="mt-2 grid min-h-10 grid-cols-2 items-center justify-items-start gap-1.5 pl-[22px]">
        {row.chips.map((c) => (
          <CaseChip key={c.id} chip={c} row={row.id} reduceMotion={reduceMotion} onOpen={onOpen} tint={lane.bg} />
        ))}
        {row.more > 0 && <span className="self-center text-small text-muted">+{row.more} more</span>}
      </div>
    </div>
  );
}
