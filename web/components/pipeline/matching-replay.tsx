"use client";

import { useState } from "react";
import { motion, useReducedMotion } from "motion/react";
import Link from "next/link";
import { Avatar } from "@/components/avatar/avatar";
import { CATEGORICAL, CHROME } from "@/components/charts/palette";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import type { PersonSeed } from "@/lib/people/types";
import { cn } from "@/lib/utils";
import { MarkShape, PlaybackControls, Picker } from "./controls";
import { PipelineHeading, type PipelineHead } from "./heading";
import type { Mark } from "./flow";
import { usePlayback, type Track } from "./playback";

export type FieldCheck = { name: string; referral: string; report: string; verdict: "agrees" | "differs" | "missing" | "exact" };

export type MatchCase = {
  tab: string;
  referralSeed: PersonSeed;
  /** The patient record this referral and report belong to. */
  record: string;
  /** Whether the linker's decision alone makes the link trusted, before any person reviews it. */
  trustedByLinker: boolean;
  report: string;
  referralName: string;
  referralMeta: string;
  reportName: string;
  reportMeta: string;
  p: number;
  tau: number;
  basis: string;
  fields: FieldCheck[];
  /** The real referral first, then runner-ups that are illustrative (gap G7). */
  candidates: { ar: string; meta: string; similarity: number; seed: PersonSeed }[];
  outcome: { held: boolean; pill: string; text: string };
};

const FRAMES = 14;
const TICK_MS = 900;
const SCORED = 8;
const CALIBRATED = 9;
const DECIDED = 10;

const VERDICT_SHAPE: Record<FieldCheck["verdict"], { d: string; className: string }> = {
  agrees: { d: "M3 8.5l3 3 7-7", className: "text-positive" },
  exact: { d: "M3 8.5l3 3 7-7", className: "text-positive" },
  differs: { d: "M4 4l8 8M12 4l-8 8", className: "text-muted" },
  missing: { d: "M4 8h8", className: "text-warning" },
};

export function MatchingReplay({ heading, cases, referrals }: { heading: PipelineHead; cases: MatchCase[]; referrals: number }) {
  const total = cases.length * FRAMES;
  const [track] = useState<Track<number>>(() => ({ rest: DECIDED, advance: (f) => (f + 1) % total }));
  const play = usePlayback(track, TICK_MS);
  const [howOpen, setHowOpen] = useState(false);
  const reduceMotion = !!useReducedMotion();

  const ci = Math.floor(play.frame / FRAMES);
  const f = play.frame % FRAMES;
  const C = cases[ci];
  const searching = f >= 2 && f < 4;
  const shown = Math.max(0, Math.min(C.candidates.length, f - 3));
  const scored = f >= SCORED;
  const calibrated = f >= CALIBRATED;
  const decided = f >= DECIDED;
  const count = referrals.toLocaleString("en-US");

  const stageState = (doneAt: number, from: number): Mark => (f >= doneAt ? "done" : f >= from ? "working" : "queued");
  const stages: { label: string; mark: Mark; detail: string }[] = [
    { label: "Received", mark: "done", detail: C.report },
    { label: "Name read", mark: stageState(2, 1), detail: "Arabic or English spelling" },
    { label: "Similar names", mark: stageState(4, 2), detail: `${count} referrals` },
    { label: "Compared", mark: stageState(SCORED, 4), detail: `${C.candidates.length} possible matches` },
    { label: "Score", mark: stageState(CALIBRATED, SCORED), detail: C.p.toFixed(3) },
    { label: "Decision", mark: decided ? (C.outcome.held ? "held" : "done") : f >= CALIBRATED ? "working" : "queued", detail: decided ? C.outcome.pill : "" },
  ];
  const WORD: Record<Mark, string> = { queued: "queued", working: "working", done: "done", held: "held", failed: "failed" };

  const left = Math.max(0, Math.min(100, ((C.p - 0.95) / 0.05) * 100));
  const lineAt = ((C.tau - 0.95) / 0.05) * 100;

  return (
    <>
      <PipelineHeading
        {...heading}
        controls={
          <PlaybackControls
            paused={play.paused}
            onToggle={play.toggle}
            onStep={play.step}
            reduceMotion={play.reduceMotion}
            position={`Report ${ci + 1} of ${cases.length} · step ${f + 1} of ${FRAMES}`}
            speed="Slowed down so you can follow it. The real match takes under a second."
          >
            <Picker
              label="Case"
              options={cases.map((c, i) => ({ value: i, label: c.tab }))}
              value={ci}
              onPick={(i) => play.seek(i * FRAMES)}
            />
          </PlaybackControls>
        }
      >
        <section aria-label="Matching steps" className="border-t border-line px-4 py-4 sm:px-6">
          <ol className="m-0 grid list-none grid-cols-2 gap-x-3 gap-y-2 p-0 sm:grid-cols-3 lg:grid-cols-6">
            {stages.map((g, i) => (
              <li key={g.label} className="relative min-w-0 pt-5" data-mark={g.mark}>
                {i < stages.length - 1 && (
                  <span aria-hidden className={cn("absolute -right-2 left-3.5 top-1 hidden h-0.5 lg:block", stages[i + 1].mark === "queued" ? "bg-control" : "bg-ink")} />
                )}
                <MarkShape mark={g.mark} hue={g.mark === "working" || (i === 5 && decided && !C.outcome.held)} className="absolute left-0 top-0 size-2.5" />
                <div className="text-body font-medium">{g.label}</div>
                <div className="text-small text-muted">
                  {WORD[g.mark]}
                  {g.mark !== "queued" && g.detail ? ` · ${g.detail}` : ""}
                </div>
              </li>
            ))}
          </ol>
        </section>
      </PipelineHeading>

      <div className="mt-3 flex flex-wrap items-start gap-3">
        <div className="flex min-w-0 max-w-[400px] flex-[1_1_340px] flex-col gap-3">
          <section aria-labelledby="search-h" className="rounded-card border border-line bg-surface px-4 py-5 sm:px-6">
            <h2 id="search-h" className="m-0 text-section font-semibold text-ink">
              Closest referrals
            </h2>
            <p role="status" className="m-0 mt-0.5 flex h-6 items-center gap-2 text-small text-muted">
              {f < 4 && <MarkShape mark="working" />}
              {f < 1 ? "Report received" : f < 2 ? "Reading the name on the report…" : searching ? `Searching ${count} referrals…` : `${C.candidates.length} closest of ${count} referrals`}
            </p>
            <div className="mt-2 flex justify-between text-small text-muted">
              <span>Referral, in Arabic</span>
              <span>Name similarity</span>
            </div>
            <ol className="m-0 mt-1 list-none p-0">
              {C.candidates.map((k, i) => {
                const best = scored && i === 0;
                return (
                  <li key={`${ci}-${i}`} className={cn("-mx-2 min-h-[68px] rounded-control border-t border-rule p-2", best && "bg-active")}>
                    {i < shown ? (
                      <motion.div initial={reduceMotion ? false : { opacity: 0, y: -4 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: reduceMotion ? 0 : 0.3 }}>
                        <div className="flex items-center justify-between gap-3">
                          <span className="flex min-w-0 items-center gap-2">
                            <Avatar person={k.seed} size={22} label="" />
                            <span lang="ar" dir="rtl" className="min-w-0 break-words text-section font-medium text-ink">
                              {k.ar}
                            </span>
                          </span>
                          <span className="flex-none text-body font-medium tabular-nums text-ink">{k.similarity.toFixed(2)}</span>
                        </div>
                        <div aria-hidden className="mt-0.5 h-1 rounded-sm bg-raised">
                          <div className="h-1 rounded-sm" style={{ width: `${Math.round(k.similarity * 100)}%`, backgroundColor: i === 0 ? CATEGORICAL[0] : CHROME.deemphasis }} />
                        </div>
                        <div className="mt-1 break-words text-small text-muted">
                          {k.meta}
                          {i > 0 ? " · illustrative" : ""}
                        </div>
                      </motion.div>
                    ) : (
                      <div aria-hidden className="pt-1">
                        <div className="h-4 w-[110px] rounded-chip bg-raised" />
                        <div className="mt-2.5 h-1 rounded-sm bg-raised" />
                        <div className="mt-2 h-3 w-40 rounded-chip bg-raised" />
                      </div>
                    )}
                  </li>
                );
              })}
            </ol>
            <p className="m-0 mt-3 flex flex-wrap items-center gap-2 text-small text-ink-soft">
              <HonestyLabel plain kind="illustrative" />
              The other names, their ages and all similarity bars are examples. The matched pair, its score and the line are real.
            </p>
          </section>

          <section className="rounded-card border border-line bg-surface px-4 py-2">
            <button
              type="button"
              aria-expanded={howOpen}
              onClick={() => setHowOpen((v) => !v)}
              className="flex h-11 w-full cursor-pointer items-center justify-between rounded-control bg-transparent px-2 text-body font-medium text-ink"
            >
              How it decides
              <svg aria-hidden width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" className={cn(howOpen && "rotate-180")}>
                <path d="M3 4.5l3 3 3-3" />
              </svg>
            </button>
            {howOpen && (
              <ol className="m-0 mb-2 mt-1 pl-7 pr-2 text-small leading-[18px] text-ink-soft">
                <li className="mb-1.5">
                  <span className="font-medium text-ink">Reads the name.</span> It knows Arabic and English spellings. The right referral is in its top 10 for 93% of names; the best
                  ready-made tool manages 60%.
                </li>
                <li className="mb-1.5">
                  <span className="font-medium text-ink">Finds similar names.</span> It looks through every referral for the closest names.
                </li>
                <li className="mb-1.5">
                  <span className="font-medium text-ink">Weighs the details.</span> It checks the name against the other details on file.
                </li>
                <li>
                  <span className="font-medium text-ink">Gives a score.</span> A name alone never reaches the 0.99 line, so it always needs a second identifier or two reviewers.
                </li>
              </ol>
            )}
          </section>
        </div>

        <section aria-labelledby="bridge-h" className="min-w-0 flex-[999_1_560px] rounded-card border border-line bg-surface px-4 py-5 sm:px-6">
          <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
            <h2 id="bridge-h" className="m-0 text-section font-semibold text-ink">
              Report {C.report} against the closest referral
            </h2>
            <span className="whitespace-nowrap text-small text-muted">
              case {ci + 1} of {cases.length}
            </span>
          </div>

          <div className="@container mt-3">
          <div className="grid grid-cols-1 gap-3 @xl:grid-cols-[minmax(0,1fr)_172px_minmax(0,1fr)]">
            <div className={cn("rounded-control px-4", scored ? "border border-line" : "border border-dashed border-control")}>
              <div className="flex min-h-16 items-center gap-3 py-2">
                {scored ? (
                  <Link href={C.record} className="flex min-h-11 min-w-0 items-center gap-3 rounded-control text-ink no-underline hover:text-ink [&:hover_[data-name]]:underline">
                    <Avatar person={C.referralSeed} size={40} label="The referral" />
                    <div className="flex min-w-0 flex-col justify-center">
                      <div data-name lang="ar" dir="rtl" className="break-words text-left text-[20px] font-medium leading-7 text-ink">
                        {C.referralName}
                      </div>
                      <div className="break-words text-small text-muted">{C.referralMeta}</div>
                    </div>
                  </Link>
                ) : (
                  <>
                    <Avatar size={40} label="Referral not chosen yet" />
                    <div className="flex min-w-0 flex-col justify-center">
                      <div className="break-words text-left text-[20px] font-medium leading-7 text-ink">Searching…</div>
                      <div className="break-words text-small text-muted">Referral not chosen yet</div>
                    </div>
                  </>
                )}
              </div>
              {C.fields.map((fd) => (
                <div key={fd.name} className={cn("flex min-h-9 items-center gap-2 border-t border-rule py-1 text-body", !scored && "text-muted")}>
                  <span className="w-24 flex-none text-small text-muted @xl:hidden">{fd.name}</span>
                  <span className="min-w-0 break-words">{scored ? fd.referral : "—"}</span>
                </div>
              ))}
            </div>

            <div>
              <div className="flex items-center gap-2 py-1 text-small text-muted @xl:hidden">
                <MarkShape mark={decided ? (C.outcome.held ? "held" : "done") : "queued"} hue={decided && !C.outcome.held} className="size-2.5" />
                {decided ? (C.outcome.held ? C.outcome.pill.toLowerCase() : `matched · ${C.basis}`) : scored ? "possible match" : "no match yet"}
              </div>
              <div aria-hidden className="relative hidden h-16 @xl:block">
                <div className={cn("absolute -left-3 -right-3 top-[31px] h-0.5", scored ? "bg-ink" : "bg-control")} />
                <div className="absolute left-1/2 top-[27px] -ml-[5px]">
                  <MarkShape mark={decided ? (C.outcome.held ? "held" : "done") : "queued"} hue={decided && !C.outcome.held} className="size-2.5" />
                </div>
                <div className="absolute inset-x-0 top-[42px] text-center text-small text-muted">
                  {decided ? (C.outcome.held ? C.outcome.pill.toLowerCase() : `matched · ${C.basis}`) : scored ? "possible match" : "no match yet"}
                </div>
              </div>
              {C.fields.map((fd) => {
                const v = VERDICT_SHAPE[fd.verdict];
                return (
                  <div key={fd.name} className="flex min-h-9 items-center gap-1.5 border-t border-rule px-1 py-1">
                    <svg aria-hidden width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" className={cn("flex-none", scored ? v.className : "text-control")}>
                      <path d={scored ? v.d : "M4 8h8"} />
                    </svg>
                    <span className="flex-1 whitespace-nowrap text-body">{fd.name}</span>
                    <span className="whitespace-nowrap text-small text-muted">{scored ? fd.verdict : "waiting"}</span>
                  </div>
                );
              })}
            </div>

            <div className="rounded-control border border-line px-4">
              <div className="flex min-h-16 items-center gap-3 py-2">
                <Avatar
                  person={C.referralSeed}
                  state={decided && C.trustedByLinker ? "resolving" : "unknown"}
                  size={40}
                  label={decided && C.trustedByLinker ? "Same patient as the referral" : "Not yet confirmed as the referral's patient"}
                />
                <div className="flex min-w-0 flex-col justify-center">
                  <div className="break-words text-[20px] font-medium leading-7 text-ink">{C.reportName}</div>
                  <div className="break-words text-small text-muted">{C.reportMeta}</div>
                </div>
              </div>
              {C.fields.map((fd) => (
                <div key={fd.name} className={cn("flex min-h-9 items-center gap-2 border-t border-rule py-1 text-body", fd.verdict === "missing" && "text-muted")}>
                  <span className="w-24 flex-none text-small text-muted @xl:hidden">{fd.name}</span>
                  <span className="min-w-0 break-words">{fd.report}</span>
                </div>
              ))}
            </div>
          </div>
          </div>

          <div className="mt-5">
            <span className="text-body font-semibold text-ink">How sure we are it is the same woman</span>
            <div
              role="img"
              aria-label={
                calibrated
                  ? `Match score ${C.p.toFixed(3)} on a scale from 0.95 to 1.00. At ${C.tau} or above, a match closes by itself.`
                  : `No match score yet. At ${C.tau} or above, a match closes by itself.`
              }
              className="relative mt-2 h-16 tabular-nums"
            >
              {/* The zone at or above the line, where a link closes by itself. */}
              <div className="absolute top-3.5 h-6 bg-jade-50" style={{ left: `${lineAt}%`, right: 0 }} />
              <div className="absolute inset-x-0 top-6 h-0.5 rounded-sm bg-limestone-400" />
              <div className="absolute top-2.5 -ml-px h-8 w-0.5" style={{ left: `${lineAt}%`, backgroundColor: CHROME.reference }} />
              <div className="absolute -top-2 mr-1.5 whitespace-nowrap text-small text-ink" style={{ right: `${100 - lineAt}%` }}>
                auto-close line {C.tau}
              </div>
              {calibrated && (
                <motion.div
                  initial={reduceMotion ? false : { left: "0%", opacity: 0 }}
                  animate={{ left: `${left}%`, opacity: 1 }}
                  transition={{ duration: reduceMotion ? 0 : 0.6, ease: [0.2, 0.7, 0.2, 1] }}
                  className="absolute top-[19px] -ml-[7px] size-3.5 rounded-full"
                  style={{ backgroundColor: CATEGORICAL[0], boxShadow: "0 0 0 2px var(--color-surface)" }}
                />
              )}
              {/* The value sits under its dot; the 1.00 end label gives way when the dot is near it. */}
              <div
                data-testid="match-p"
                className="absolute top-[38px] whitespace-nowrap text-body font-semibold text-ink"
                style={calibrated ? (left > 88 ? { right: 0 } : { left: `${left}%`, transform: "translateX(-50%)" }) : { left: 0 }}
              >
                {calibrated ? C.p.toFixed(3) : "—"}
              </div>
              {(!calibrated || left > 10) && <div className="absolute left-0 top-[40px] text-small text-muted">0.95</div>}
              {(!calibrated || left <= 88) && <div className="absolute right-0 top-[40px] text-small text-muted">1.00</div>}
            </div>
            <p className="m-0 mt-1 flex items-center gap-1.5 text-small text-ink-soft">
              <span aria-hidden className="block h-3 w-4 rounded-[2px] bg-jade-50 shadow-[inset_0_0_0_1px_var(--color-jade-200)]" />
              Closes by itself at {C.tau} and above
            </p>
          </div>

          <div role="status" className="mt-2 flex min-h-11 items-center gap-3 border-t border-rule pt-3">
            {decided ? (
              <span
                className={cn(
                  "inline-flex h-[22px] flex-none items-center gap-1.5 whitespace-nowrap rounded-chip px-2 text-small font-medium",
                  C.outcome.held ? "bg-pill-warning-bg text-pill-warning-ink" : "bg-pill-positive-bg text-pill-positive-ink",
                )}
              >
                <MarkShape mark={C.outcome.held ? "held" : "done"} />
                {C.outcome.pill}
              </span>
            ) : (
              <span className="inline-flex h-[22px] flex-none items-center gap-1.5 whitespace-nowrap rounded-chip bg-pill-neutral-bg px-2 text-small font-medium text-pill-neutral-ink">
                <MarkShape mark="queued" />
                Matching
              </span>
            )}
            <span className="min-w-0 flex-1 text-body">
              {decided ? C.outcome.text : calibrated ? "Checking for a second identifier…" : scored ? "Scoring the closest match…" : "Looking for similar names…"}
            </span>
          </div>
        </section>
      </div>
    </>
  );
}
