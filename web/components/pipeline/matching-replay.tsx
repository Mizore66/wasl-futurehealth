"use client";

import { useState } from "react";
import { motion } from "motion/react";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { cn } from "@/lib/utils";
import { MarkShape, PlaybackControls, Picker } from "./controls";
import type { Mark } from "./flow";
import { usePlayback, type Track } from "./playback";

export type FieldCheck = { name: string; referral: string; report: string; verdict: "agrees" | "differs" | "missing" | "exact" };

export type MatchCase = {
  tab: string;
  report: string;
  referralName: string;
  referralMeta: string;
  reportName: string;
  reportMeta: string;
  p: number;
  tau: number;
  basis: string;
  linker: string;
  fields: FieldCheck[];
  /** The real referral first, then runner-ups that are illustrative (gap G7). */
  candidates: { ar: string; meta: string; similarity: number }[];
  outcome: { held: boolean; pill: string; text: string; tech: string };
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

export function MatchingReplay({ heading, cases, referrals }: { heading: React.ReactNode; cases: MatchCase[]; referrals: number }) {
  const total = cases.length * FRAMES;
  const [track] = useState<Track<number>>(() => ({ rest: DECIDED, advance: (f) => (f + 1) % total }));
  const play = usePlayback(track, TICK_MS);
  const [howOpen, setHowOpen] = useState(false);

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
    { label: "Name encoded", mark: stageState(2, 1), detail: "Arabic ↔ Latin encoder" },
    { label: "Nearest names", mark: stageState(4, 2), detail: `${count} referrals` },
    { label: "Scored", mark: stageState(SCORED, 4), detail: `${C.candidates.length} candidates` },
    { label: "Calibrated", mark: stageState(CALIBRATED, SCORED), detail: `p ${C.p.toFixed(3)}` },
    { label: "Decision", mark: decided ? (C.outcome.held ? "held" : "done") : f >= CALIBRATED ? "working" : "queued", detail: decided ? C.outcome.pill : "" },
  ];
  const WORD: Record<Mark, string> = { queued: "queued", working: "working", done: "done", held: "held", failed: "failed" };

  const left = Math.max(0, Math.min(100, ((C.p - 0.95) / 0.05) * 100));
  const lineAt = ((C.tau - 0.95) / 0.05) * 100;

  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-6">
        {heading}
        <PlaybackControls
          paused={play.paused}
          onToggle={play.toggle}
          onStep={play.step}
          position={`Report ${ci + 1} of ${cases.length} · step ${f + 1} of ${FRAMES}`}
          speed="Slowed down to watch: the linker decides in well under a second."
        >
          <Picker
            label="Case"
            options={cases.map((c, i) => ({ value: i, label: c.tab }))}
            value={ci}
            onPick={(i) => play.seek(i * FRAMES)}
          />
        </PlaybackControls>
      </header>

      <section aria-label="Engine steps" className="mt-4 rounded-card border border-line bg-surface px-6 py-4">
        <ol className="m-0 grid list-none grid-cols-6 gap-x-3 p-0">
          {stages.map((g, i) => (
            <li key={g.label} className="relative min-w-0 pt-5" data-mark={g.mark}>
              {i < stages.length - 1 && (
                <span aria-hidden className={cn("absolute left-3.5 -right-2 top-1 h-0.5", stages[i + 1].mark === "queued" ? "bg-control" : "bg-ink")} />
              )}
              <MarkShape mark={g.mark} hue={g.mark === "working" || (i === 5 && decided && !C.outcome.held)} className="absolute left-0 top-0 size-2.5" />
              <div className="truncate text-body font-medium">{g.label}</div>
              <div className="truncate text-small text-muted">
                {WORD[g.mark]}
                {g.mark !== "queued" && g.detail ? ` · ${g.detail}` : ""}
              </div>
            </li>
          ))}
        </ol>
      </section>

      <div className="mt-3 flex flex-wrap items-start gap-3">
        <div className="flex min-w-0 max-w-[400px] flex-[1_1_340px] flex-col gap-3">
          <section aria-labelledby="search-h" className="rounded-card border border-line bg-surface px-6 py-5">
            <h2 id="search-h" className="m-0 text-section font-medium">
              Nearest referrals
            </h2>
            <p role="status" className="m-0 mt-0.5 flex h-6 items-center gap-2 text-small text-muted">
              {f < 4 && <MarkShape mark="working" />}
              {f < 1 ? "Report received" : f < 2 ? "Encoding the report name…" : searching ? `Searching ${count} referrals…` : `${C.candidates.length} nearest of ${count} referrals`}
            </p>
            <div className="mt-2 flex justify-between text-small text-muted">
              <span>Referral, in Arabic</span>
              <span>Name similarity</span>
            </div>
            <ol className="m-0 mt-1 list-none p-0">
              {C.candidates.map((k, i) => {
                const best = scored && i === 0;
                return (
                  <li key={`${ci}-${i}`} className={cn("-mx-2 h-[68px] rounded-control border-t border-rule p-2", best && "bg-active")}>
                    {i < shown ? (
                      <motion.div initial={{ opacity: 0, y: -4 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.3 }}>
                        <div className="flex items-baseline justify-between gap-3">
                          <span lang="ar" dir="rtl" className="whitespace-nowrap text-section font-medium">
                            {k.ar}
                          </span>
                          <span className="text-body">{k.similarity.toFixed(2)}</span>
                        </div>
                        <div aria-hidden className="mt-0.5 h-1 rounded-sm bg-raised">
                          <div className={cn("h-1 rounded-sm", best ? "bg-hue" : "bg-ink")} style={{ width: `${Math.round(k.similarity * 100)}%` }} />
                        </div>
                        <div className="mt-1 truncate text-small text-muted">
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
            <p className="m-0 mt-3 flex flex-wrap items-center gap-2 text-small text-muted">
              <HonestyLabel kind="illustrative" />
              Runner-up names, ages and every similarity bar. The matched pair, its probability and the line are from the run&apos;s case files.
            </p>
          </section>

          <section className="rounded-card border border-line bg-surface px-4 py-2">
            <button
              type="button"
              aria-expanded={howOpen}
              onClick={() => setHowOpen((v) => !v)}
              className="flex h-9 w-full cursor-pointer items-center justify-between rounded-control bg-transparent px-2 text-body font-medium text-ink"
            >
              How it decides
              <svg aria-hidden width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" className={cn(howOpen && "rotate-180")}>
                <path d="M3 4.5l3 3 3-3" />
              </svg>
            </button>
            {howOpen && (
              <ol className="m-0 mb-2 mt-1 pl-7 pr-2 text-small leading-[18px] text-ink-soft">
                <li className="mb-1.5">
                  <span className="font-medium text-ink">Name encoder.</span> Self-trained on Arabic and Latin spellings, 2.3 MB. The right referral is in its top 10 for 93.2% of names
                  (R@10 0.932), against 0.604 for the best off-the-shelf model.
                </li>
                <li className="mb-1.5">
                  <span className="font-medium text-ink">Nearest-neighbour search.</span> A deterministic HNSW index over every referral returns the closest names.
                </li>
                <li className="mb-1.5">
                  <span className="font-medium text-ink">Scorer.</span> A LightGBM model weighs the name with the other identifiers.
                </li>
                <li>
                  <span className="font-medium text-ink">Calibration.</span> Stratified calibration turns the score into a probability. A name alone is capped at 0.989, under the
                  0.99 auto-close line, so it always needs a second identifier or two reviewers.
                </li>
              </ol>
            )}
          </section>
        </div>

        <section aria-labelledby="bridge-h" className="min-w-0 flex-[999_1_560px] rounded-card border border-line bg-surface px-6 py-5">
          <div className="flex items-baseline justify-between gap-3">
            <h2 id="bridge-h" className="m-0 text-section font-medium">
              Incoming report {C.report} against the best referral
            </h2>
            <span className="whitespace-nowrap text-small text-muted">
              case {ci + 1} of {cases.length}
            </span>
          </div>

          <div className="mt-3 grid grid-cols-[minmax(0,1fr)_172px_minmax(0,1fr)] gap-3">
            <div className={cn("rounded-control px-4", scored ? "border border-line" : "border border-dashed border-control")}>
              <div className="flex h-16 flex-col justify-center">
                <div lang="ar" dir="rtl" className="truncate text-left text-title font-medium">
                  {scored ? C.referralName : "Searching…"}
                </div>
                <div className="text-small text-muted">{scored ? C.referralMeta : "Referral not chosen yet"}</div>
              </div>
              {C.fields.map((fd) => (
                <div key={fd.name} className={cn("flex h-9 items-center truncate border-t border-rule text-body", !scored && "text-muted")}>
                  {scored ? fd.referral : "—"}
                </div>
              ))}
            </div>

            <div>
              <div aria-hidden className="relative h-16">
                <div className={cn("absolute -left-3 -right-3 top-[31px] h-0.5", scored ? "bg-ink" : "bg-control")} />
                <div className="absolute left-1/2 top-[27px] -ml-[5px]">
                  <MarkShape mark={decided ? (C.outcome.held ? "held" : "done") : "queued"} hue={decided && !C.outcome.held} className="size-2.5" />
                </div>
                <div className="absolute inset-x-0 top-[42px] text-center text-small text-muted">
                  {decided ? (C.outcome.held ? C.outcome.pill.toLowerCase() : `linked · ${C.basis}`) : scored ? "proposed link" : "no candidate yet"}
                </div>
              </div>
              {C.fields.map((fd) => {
                const v = VERDICT_SHAPE[fd.verdict];
                return (
                  <div key={fd.name} className="flex h-9 items-center gap-1.5 border-t border-rule px-1">
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
              <div className="flex h-16 flex-col justify-center">
                <div className="truncate text-title font-medium">{C.reportName}</div>
                <div className="text-small text-muted">{C.reportMeta}</div>
              </div>
              {C.fields.map((fd) => (
                <div key={fd.name} className={cn("flex h-9 items-center truncate border-t border-rule text-body", fd.verdict === "missing" && "text-muted")}>
                  {fd.report}
                </div>
              ))}
            </div>
          </div>

          <div className="mt-5">
            <div className="flex items-baseline justify-between">
              <span className="text-body font-medium">Calibrated probability it is the same woman</span>
              <span className="text-body" data-testid="match-p">
                {calibrated ? C.p.toFixed(3) : "—"}
              </span>
            </div>
            <div
              role="img"
              aria-label={
                calibrated
                  ? `Link probability ${C.p.toFixed(3)} on a scale from 0.95 to 1.00; the auto-close line is ${C.tau}.`
                  : `Link probability not calibrated yet; the auto-close line is ${C.tau}.`
              }
              className="relative mt-1 h-14"
            >
              <div className="absolute inset-x-0 top-6 h-1 rounded-sm bg-control" />
              <div className="absolute top-3.5 -ml-px h-6 w-0.5 bg-ink" style={{ left: `${lineAt}%` }} />
              <div className="absolute -top-0.5 ml-1.5 whitespace-nowrap text-small" style={{ left: `${lineAt}%` }}>
                auto-close line {C.tau}
              </div>
              {calibrated && (
                <motion.div
                  initial={{ left: "0%", opacity: 0 }}
                  animate={{ left: `${left}%`, opacity: 1 }}
                  transition={{ duration: 0.6, ease: [0.2, 0.7, 0.2, 1] }}
                  className="absolute top-[21px] -ml-[5px] size-2.5 rounded-full bg-hue"
                />
              )}
              <div className="absolute left-0 top-[38px] text-small text-muted">0.95</div>
              <div className="absolute right-0 top-[38px] text-small text-muted">1.00</div>
            </div>
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
                Matching identity
              </span>
            )}
            <span className="min-w-0 flex-1 text-body">
              {decided ? C.outcome.text : calibrated ? "Checking for a second identifier…" : scored ? "Calibrating the best candidate…" : "Waiting for the nearest names…"}
            </span>
          </div>
          <p className="m-0 mt-1 font-mono text-small text-muted">{decided ? C.outcome.tech : `linker ${C.linker}`}</p>
        </section>
      </div>
    </>
  );
}
