"use client";

import { useMemo, useState } from "react";
import NumberFlow from "@number-flow/react";
import { LayoutGroup, motion, useReducedMotion } from "motion/react";
import replayJson from "@/data/replay.json";
import { Avatar } from "@/components/avatar/avatar";
import { Button } from "@/components/ui/button";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { SealedChain, type ChainNode } from "@/components/wasl/sealed-chain";
import type { Replay } from "@/lib/replay/types";
import { cn } from "@/lib/utils";
import { MarkShape, PlaybackControls } from "./controls";
import { PipelineHeading, type PipelineHead } from "./heading";
import { FEED_ROWS, flowView, replayTrack, type Chip, type FlowRow, type RowId } from "./flow";
import { TechnicalDetail } from "./technical";
import { stepAnnouncement } from "./announce";
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

const CARD = "min-w-0 rounded-card border border-line bg-surface px-4 py-5 sm:px-6";
const CHIP_MOVE = { duration: 0.5, ease: [0.2, 0.7, 0.2, 1] } as const;
const FEED_PITCH = 72;

type Open = (caseId: string) => void;

export function PipelineReplay({ heading, tamper, rules }: { heading: PipelineHead; tamper: TamperDemo; rules: RulesBySha }) {
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
      <div role="status" aria-live="polite" aria-atomic="true" data-testid="step-announcer" className="sr-only">
        <span key={stepNote.n}>{stepNote.text}</span>
      </div>

      <PipelineHeading
        {...heading}
        controls={
          <PlaybackControls
            paused={play.paused}
            onToggle={play.toggle}
            onStep={onStep}
            reduceMotion={play.reduceMotion}
            position={`Step ${view.step} of ${view.steps}`}
            speed={`One step a second. The recorded run handled ${facts.throughputEvS} events a second.`}
          />
        }
      >
        <section aria-labelledby="flow-h" className="border-t border-line pt-5">
          <div className="px-4 sm:px-6">
            <h2 id="flow-h" className="m-0 text-section font-semibold text-ink">
              Case flow
            </h2>
            <p className="m-0 mt-1 max-w-[72ch] text-small text-muted">
              Left to right. A report shows the name as written, and &ldquo;?&rdquo;, until it is matched. The ringed one just moved; open anyone to
              see their steps.
            </p>
          </div>
          <LayoutGroup>
            <div className="mt-4 border-t border-rule">
              <ol
                aria-label={`Case flow, 8 stages. ${view.rows.map((r) => `${r.label} ${r.count}`).join(", ")}`}
                className="m-0 grid list-none grid-cols-1 p-0 md:grid-cols-2 lg:grid-cols-4"
              >
                {stages.map((r, i) => (
                  <StageColumn key={r.id} row={r} last={i === stages.length - 1} reduceMotion={reduceMotion} onOpen={profile.open} />
                ))}
                <li className="flex min-w-0 flex-col gap-2 px-4 py-4 sm:px-5 md:col-span-2 lg:col-span-4 lg:border-t lg:border-rule">
                  <span className="text-body font-semibold text-ink">Outcome</span>
                  <div className="grid gap-x-4 gap-y-2 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.6fr)_minmax(0,1fr)]">
                    {lanes.map((r) => (
                      <OutcomeLane key={r.id} row={r} reduceMotion={reduceMotion} onOpen={profile.open} />
                    ))}
                  </div>
                </li>
              </ol>
              <div data-stage="ledger" className="flex flex-wrap items-baseline gap-x-4 gap-y-1 border-t border-rule px-4 py-3 sm:px-6">
                <span className="text-body font-semibold text-ink">{ledger.label}</span>
                <span className="text-section font-semibold tabular-nums text-ink">{ledger.count}</span>
                <span className="text-small text-muted">{ledger.note}</span>
                <span className="min-w-0 flex-[1_1_240px] break-words text-small text-ink-soft">
                  {view.latestRecorded
                    ? `Latest: ${view.latestRecorded.who.name} · ${view.latestRecorded.closed ? "chain verified" : "decision sealed"}`
                    : "Nothing recorded yet in this loop"}
                </span>
              </div>
              <div className="border-t border-rule px-4 sm:px-6">
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
      </PipelineHeading>

      <div className="mt-3 grid grid-cols-1 items-start gap-3 lg:grid-cols-2">
        <section aria-labelledby="feed-h" className={cn(CARD, "lg:row-span-2")}>
          <h2 id="feed-h" className="m-0 text-section font-semibold text-ink">
            Decisions
          </h2>
          <p className="m-0 mt-1 max-w-[72ch] text-small text-muted">Newest first. Plain words; the system&apos;s own labels are under Technical detail.</p>
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
                    <div className="truncate text-body text-ink">{f.text}</div>
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
                  <span className="text-small tabular-nums text-muted">step {f.step}</span>
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

        <section aria-label="Run totals and the recorded failover result" className={CARD}>
          <div className="grid grid-cols-2 gap-4 xl:grid-cols-4">
            <Total label="Decisions recorded" note={`of ${fmt(totals.outputs)} in the run`}>
              <span data-testid="total-decisions" data-value={view.decisions}>
                <NumberFlow value={view.decisions} locales="en-US" />
              </span>
            </Total>
            <Total label="Lost" note="decisions">
              <span data-testid="total-lost">{facts.lost}</span>
            </Total>
            <Total label="Duplicated" note="decisions">
              <span data-testid="total-duplicated">{facts.duplicates}</span>
            </Total>
            <Total label="Records verified so far" note={`of ${fmt(totals.chainsVerified)} sealed chains`}>
              <span data-testid="total-chains" data-value={view.chains}>
                <NumberFlow value={view.chains} locales="en-US" />
              </span>
            </Total>
          </div>

          <div className="mt-5 flex flex-col gap-2 border-t border-rule pt-4">
            <p className="m-0 flex flex-wrap items-center gap-2 text-body font-semibold text-ink">
              Switching a server off during a run <HonestyLabel kind="designed" />
            </p>
            <p className="m-0 max-w-[72ch] text-small text-muted" data-testid="server-result">
              There is no switch on this screen because live failover is not built. This is the result of the recorded run, not the state of any server now. It shows {facts.lost} of {fmt(totals.outputs)} decisions
              lost and {facts.duplicates} duplicated.
            </p>
          </div>
        </section>

        <section aria-labelledby="tamper-h" className={CARD}>
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="flex min-w-0 flex-col items-start gap-1.5">
              <h2 id="tamper-h" className="m-0 text-section font-semibold text-ink">
                {tamper.title}
              </h2>
              <div className="flex flex-wrap items-center gap-2">
                <HonestyLabel kind="simulated">Simulated edit</HonestyLabel>
                <span
                  role="status"
                  className={cn(
                    "inline-flex h-[22px] items-center gap-1.5 whitespace-nowrap rounded-chip px-2 text-small font-medium",
                    tampered ? "bg-pill-negative-bg text-pill-negative-ink" : "bg-pill-positive-bg text-pill-positive-ink",
                  )}
                >
                  <svg aria-hidden width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                    <path d={tampered ? "M3 3l6 6M9 3l-6 6" : "M2.5 6.5l2.5 2.5 4.5-6"} />
                  </svg>
                  {tampered ? `Changed record detected at step ${tamper.step}` : "Records intact"}
                </span>
              </div>
            </div>
            <Button type="button" variant="quiet" size="touch" onClick={() => setTampered((v) => !v)} className="flex-none whitespace-nowrap border border-limestone-600 font-medium">
              {tampered ? "Undo the change" : "Tamper with a step"}
            </Button>
          </div>
          <SealedChain nodes={tampered ? tamper.tampered : tamper.intact} className="mt-4" />
          <p aria-live="polite" className="m-0 mt-3 max-w-[72ch] text-small text-muted">
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
      <PersonSheet profile={shown} onClose={profile.close} />
    </>
  );
}

function Total({ label, note, children }: { label: string; note: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <div className="text-small font-medium text-ink-soft">{label}</div>
      <div className="mt-1 text-title font-semibold tabular-nums text-ink">{children}</div>
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
        "inline-flex min-h-11 max-w-full cursor-pointer items-center gap-2 rounded-[26px] py-1 pl-1 pr-3 text-small no-underline hover:text-ink hover:shadow-[inset_0_0_0_1px_var(--color-control)]",
        tint,
        chip.active ? "font-medium text-ink ring-2 ring-hue" : "text-ink-soft",
      )}
    >
      <Avatar person={chip.who.seed} state={chip.who.face} size={chip.active ? 44 : 32} label="" />
      <span className="min-w-0 break-words" data-testid="chip-name">
        {chip.who.name}
      </span>
      {row === "matching" && chip.mark === "working" && <MarkShape mark="working" hue />}
      {chip.readers && <ReaderMarks state={chip.readers} reduceMotion={reduceMotion} />}
      {row === "gate" && <MarkShape mark={chip.mark} hue={chip.mark === "done"} />}
    </motion.a>
  );
}

/** A stage in the left-to-right row: a node on the connector line (filled while the stage holds cases), then its name and count. */
function StageColumn({ row, last, reduceMotion, onOpen }: { row: FlowRow; last: boolean; reduceMotion: boolean; onOpen: Open }) {
  return (
    <li data-stage={row.id} className="relative flex min-w-0 flex-col border-b border-rule px-4 pb-5 pt-4 sm:px-5 md:odd:border-r lg:border-b-0 lg:border-r-0 lg:odd:border-r-0">
      <span aria-hidden className="mb-3 hidden items-center lg:flex">
        <span
          className={cn("block size-3 flex-none rounded-full border-2 border-jade-700", row.count ? "bg-jade-700" : "bg-surface")}
        />
        {!last && <span className="-mr-5 block h-0.5 flex-1 bg-jade-400" />}
      </span>
      <LaneHead label={row.label} count={row.count} />
      <span className="text-small text-muted">{row.note}</span>
      <div className="mt-4 flex min-h-24 flex-row flex-wrap items-start gap-2 lg:min-h-[120px] lg:flex-col">
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
      <LaneHead label={row.label} count={row.count} mark={<span className={cn("self-center", lane.ink)}><OutcomeMark mark={lane.mark} /></span>} />
      <span className="block text-small text-muted">{row.note}</span>
      <div className="mt-2 grid min-h-10 grid-cols-[repeat(auto-fill,minmax(140px,1fr))] items-center justify-items-start gap-1.5">
        {row.chips.map((c) => (
          <CaseChip key={c.id} chip={c} row={row.id} reduceMotion={reduceMotion} onOpen={onOpen} tint={lane.bg} />
        ))}
        {row.more > 0 && <span className="self-center text-small text-muted">+{row.more} more</span>}
      </div>
    </div>
  );
}

/** Every lane's label and count share one baseline and one type scale, with tabular figures so counts do not jitter. */
function LaneHead({ label, count, mark }: { label: string; count: number; mark?: React.ReactNode }) {
  return (
    <div className="flex items-baseline gap-2">
      {mark}
      <span className="min-w-0 flex-1 text-body font-semibold text-ink">{label}</span>
      <span className={cn("text-section font-semibold tabular-nums", count ? "text-ink" : "text-muted")}>{count}</span>
    </div>
  );
}
