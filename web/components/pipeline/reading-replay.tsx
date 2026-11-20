"use client";

import { motion, useReducedMotion } from "motion/react";
import Link from "next/link";
import readerJson from "@/data/reader.json";
import { Avatar } from "@/components/avatar/avatar";
import type { PersonSeed } from "@/lib/people/types";
import { cn } from "@/lib/utils";
import { plain } from "@/lib/domain/terms";
import { MarkShape, PlaybackControls, Picker } from "./controls";
import { PipelineHeading, type PipelineHead } from "./heading";
import type { Mark } from "./flow";
import { usePlayback } from "./playback";
import { FIELDS, READER_SPEEDUP, READER_TICK_MS, readerFrames, readerStart, readerTrack, readerView } from "./reader-timeline";
import { NOT_ASKED, type ReaderAnswer, type ReaderCase, type ReaderData, type ReaderField } from "./reader-types";

const reader = readerJson as ReaderData;
const track = readerTrack(reader);
const frames = readerFrames(reader);

const WORDS: Record<Exclude<ReaderField, "histology">, Record<string, string>> = {
  caecum: { yes: "reached", no: "not reached", not_stated: "not stated" },
  prep: { adequate: "adequate", inadequate: "inadequate", not_stated: "not graded" },
  tissue: { yes: "tissue taken", no: "none taken" },
};

function valueWord(a: ReaderAnswer, k: ReaderField): string {
  if (a[k] === NOT_ASKED) return "not asked";
  if (k === "histology") return a.histology === "yes" ? "back" : a.tissue === "yes" ? "not back" : "not needed";
  return WORDS[k][a[k]] ?? a[k].replaceAll("_", " ");
}

const REASON_WORDS: Record<string, string> = {
  incomplete_exam: "the scope didn't reach the end, so this colonoscopy doesn't count yet",
  inadequate_prep: "the bowel prep was not adequate",
  awaiting_histology: "tissue was taken and histology is not back",
  missing_documentation: "the report does not document every check",
};

function verdict(c: ReaderCase, disagree: ReaderField[]): { kind: "pass" | "held" | "review"; pill: string; text: string } {
  if (disagree.length > 0) {
    const names = disagree.map((k) => FIELDS.find((f) => f.k === k)!.name.toLowerCase()).join(", ");
    return {
      kind: "review",
      pill: "Two reviewers",
      text: `The readers disagree on ${names}, so nothing closes by itself. Waiting for two reviewers.`,
    };
  }
  if (c.ai.state === "closed") {
    return { kind: "pass", pill: "Passed", text: "Both readers agree on every check they both read. Closed with evidence." };
  }
  return {
    kind: "held",
    pill: "Held",
    text: `Both readers agree: ${REASON_WORDS[c.ai.reason] ?? plain(c.ai.reason).toLowerCase()}.`,
  };
}

const s1 = (ms: number) => (ms / 1000).toFixed(1);

/** The demo patient a report stands for: the line under its title and the face beside it. */
export type StandsFor = { line: string; seed: PersonSeed; record: string };

export function ReadingReplay({ heading, standsFor }: { heading: PipelineHead; standsFor: Record<string, StandsFor> }) {
  const play = usePlayback(track, READER_TICK_MS);
  const v = readerView(reader, play.frame);
  const { c, f } = v;
  const t = frames.perCase[v.index];
  const result = verdict(c, v.disagree);
  const reduceMotion = !!useReducedMotion();

  const stageState = (done: boolean, started: boolean): Mark => (done ? "done" : started ? "working" : "queued");
  const gateMark: Mark = v.gated ? (result.kind === "pass" ? "done" : "held") : v.agreed ? "working" : "queued";
  const stages: { label: string; mark: Mark; detail: string }[] = [
    { label: "Received", mark: "done", detail: `report ${c.uid}` },
    { label: "Checklist reader", mark: stageState(v.rulesDone, f >= 1), detail: "DoH checklist" },
    { label: "AI reader", mark: stageState(v.aiDone, f >= 1), detail: `${s1(v.elapsedMs)} s` },
    { label: "Agreement", mark: stageState(v.agreed, v.aiDone), detail: v.agreed ? `${v.compared.length - v.disagree.length} of ${v.compared.length}` : "" },
    { label: "Safety check", mark: gateMark, detail: v.gated ? result.pill : "" },
  ];
  const WORD: Record<Mark, string> = { queued: "queued", working: "working", done: "done", held: "held", failed: "failed" };

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
            position={`Report ${v.index + 1} of ${reader.cases.length} · step ${f + 1} of ${t.frames}`}
            speed={`Real AI reading times, played ${READER_SPEEDUP}× faster.`}
          >
            <Picker
              label="Report"
              options={reader.cases.map((x, i) => ({ value: i, label: `Report ${x.uid}` }))}
              value={v.index}
              onPick={(i) => play.seek(readerStart(reader, i))}
            />
          </PlaybackControls>
        }
      >
        <section aria-label="Reading steps" className="border-t border-line px-4 py-4 sm:px-6">
          <ol className="m-0 grid list-none grid-cols-2 gap-x-3 gap-y-2 p-0 sm:grid-cols-3 lg:grid-cols-5">
            {stages.map((g, i) => (
              <li key={g.label} className="relative min-w-0 pt-5" data-mark={g.mark}>
                {i < stages.length - 1 && (
                  <span aria-hidden className={cn("absolute -right-2 left-3.5 top-1 hidden h-0.5 lg:block", stages[i + 1].mark === "queued" ? "bg-control" : "bg-ink")} />
                )}
                <MarkShape mark={g.mark} hue={g.mark === "working"} className="absolute left-0 top-0 size-2.5" />
                <div className="text-body font-medium">{g.label}</div>
                <div className="text-small text-muted tabular-nums">
                  {WORD[g.mark]}
                  {g.mark !== "queued" && g.detail ? ` · ${g.detail}` : ""}
                </div>
              </li>
            ))}
          </ol>
        </section>
      </PipelineHeading>

      <div className="mt-3 flex flex-wrap items-start gap-3">
        <section aria-labelledby="report-h" className="min-w-0 flex-[0_1_420px] rounded-card border border-line bg-surface px-4 py-5 sm:px-6">
          <h2 id="report-h" className="m-0 text-section font-semibold text-ink">
            Colonoscopy report {c.uid}
          </h2>
          {standsFor[c.uid] ? (
            <Link
              href={standsFor[c.uid].record}
              className="mt-1 flex min-h-11 min-w-0 items-center gap-2 self-start rounded-chip text-small text-ink-soft no-underline hover:text-ink hover:underline"
            >
              <Avatar person={standsFor[c.uid].seed} size={20} label="" />
              <span className="min-w-0 break-words">{standsFor[c.uid].line}</span>
            </Link>
          ) : (
            <p className="m-0 mt-1 flex min-w-0 items-center gap-2 text-small text-muted">
              <Avatar size={20} label="No demo patient" />
              <span className="min-w-0 break-words">Not tied to a demo patient</span>
            </p>
          )}
          <div role="region" aria-label={`Report ${c.uid} excerpt`} tabIndex={0} className="mt-3 h-[268px] overflow-y-auto rounded-control border border-rule bg-limestone-100 px-4 py-3">
            <p className="m-0 text-body leading-[22px]" data-testid="report-text">
              {c.segments.map((s, i) => {
                const lit = !!s.field && v.aiDone;
                const disputed = lit && v.agreed && v.disagree.includes(s.field!);
                return (
                  <span key={`${c.uid}-${i}`}>
                    {lit && (
                      <span className={cn("mr-1 text-small font-medium", disputed ? "text-warning" : "text-hue")}>
                        {FIELDS.find((x) => x.k === s.field)?.name ?? s.field}
                        {disputed ? " · disputed" : ""}
                      </span>
                    )}
                    <span
                      className={cn(
                        lit && "bg-raised underline decoration-2 underline-offset-4",
                        lit && (disputed ? "bg-pill-warning-bg decoration-dashed decoration-warning" : "decoration-hue"),
                      )}
                    >
                      {s.text}
                    </span>
                  </span>
                );
              })}
            </p>
          </div>
          <p className="m-0 mt-2 max-w-[72ch] text-small text-muted">
            Excerpt of a real US colonoscopy report (&ldquo;{c.title}&rdquo;, MTSamples), given to a simulated patient. Highlights show what the AI reader quoted.
          </p>
        </section>

        <section aria-labelledby="checks-h" className="min-w-0 flex-[999_1_560px] rounded-card border border-line bg-surface px-4 py-5 sm:px-6">
          <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
            <h2 id="checks-h" className="m-0 text-section font-semibold text-ink">
              The four checks for a complete colonoscopy
            </h2>
            <span className="whitespace-nowrap text-small text-muted">
              report {v.index + 1} of {reader.cases.length}
            </span>
          </div>
          <p data-testid="checks-takeaway" className="m-0 mt-1 text-body font-medium text-ink">
            {!v.agreed
              ? "Waiting for both readers"
              : v.disagree.length === 0
                ? `Both readers agree on all ${v.compared.length} checks they both read`
                : `The readers disagree on ${v.disagree.length} of ${v.compared.length} checks they both read`}
          </p>

          <div className="@container">
          <div className="mt-3 hidden grid-cols-[minmax(0,24fr)_minmax(0,22fr)_minmax(0,34fr)_minmax(0,20fr)] items-end gap-x-3 pb-2 @xl:grid">
            <span className="text-body font-medium text-ink">Check</span>
            <span className="flex flex-col">
              <span className="text-body font-medium text-ink">Checklist reader</span>
              <span className="text-small text-muted">Follows the DoH checklist word for word</span>
            </span>
            <span className="flex min-w-0 flex-col">
              <span className="text-body font-medium">AI reader</span>
              <span className="flex items-center gap-2 text-small text-muted">
                <span className="h-1 w-16 flex-none rounded-sm bg-raised">
                  <span className="block h-1 rounded-sm bg-hue" style={{ width: `${Math.round((v.elapsedMs / c.ai.latencyMs) * 100)}%` }} />
                </span>
                <span className="tabular-nums" data-testid="ai-timer">
                  {f < 1 ? "queued" : v.aiDone ? `${s1(c.ai.latencyMs)} s recorded` : `reading · ${s1(v.elapsedMs)} s`}
                </span>
              </span>
            </span>
            <span className="text-body font-medium text-ink">Agreement</span>
          </div>

          {FIELDS.map((fd) => {
            const compared = v.compared.includes(fd.k);
            const settled = v.agreed && compared;
            const same = c.rules[fd.k] === c.ai[fd.k];
            const downgraded = c.ai.downgraded.includes(fd.k);
            const quoted = c.segments.some((s) => s.field === fd.k);
            return (
              <div key={fd.k} data-field={fd.k} className="mt-0 grid grid-cols-1 items-center gap-x-3 gap-y-1 border-t border-rule py-2 @lg:grid-cols-2 @xl:min-h-14 @xl:grid-cols-[minmax(0,24fr)_minmax(0,22fr)_minmax(0,34fr)_minmax(0,20fr)] @xl:py-2">
                <span className="flex min-w-0 flex-col @lg:col-span-2 @xl:col-span-1">
                  <span className="text-body font-medium">{fd.name}</span>
                </span>
                <Cell
                  who="Checklist reader"
                  mark={stageState(v.rulesDone, f >= 1)}
                  word={v.rulesDone ? valueWord(c.rules, fd.k) : f >= 1 ? "reading" : "queued"}
                />
                <Cell
                  who="AI reader"
                  mark={stageState(v.aiDone, f >= 1)}
                  word={v.aiDone ? valueWord(c.ai, fd.k) : f >= 1 ? "reading" : "queued"}
                  sub={v.aiDone ? (downgraded ? "marked down after its own double-check" : quoted ? "quote found in report" : "") : ""}
                />
                <span className="inline-flex items-center gap-1.5 whitespace-nowrap @lg:col-span-2 @xl:col-span-1">
                  <span className="w-24 text-small text-muted @xl:hidden">Agreement</span>
                  <svg aria-hidden width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" className={cn("flex-none", !settled ? "text-control" : same ? "text-positive" : "text-warning")}>
                    <path d={!settled ? "M4 8h8" : same ? "M3 8.5l3 3 7-7" : "M4 4l8 8M12 4l-8 8"} />
                  </svg>
                  <span className={cn("text-body", !settled ? "text-muted" : same ? "text-ink" : "font-medium text-warning")}>
                    {!v.agreed ? "waiting" : !compared ? "not compared" : same ? "agree" : "disagree"}
                  </span>
                </span>
              </div>
            );
          })}

          </div>

          <div role="status" className="mt-2 min-h-[104px] rounded-control border border-rule bg-limestone-100 p-4">
            <div className="flex items-center gap-3">
              <span className="text-body font-semibold text-ink">Safety check</span>
              <span
                data-testid="gate-pill"
                className={cn(
                  "inline-flex h-[22px] items-center gap-1.5 whitespace-nowrap rounded-chip px-2 text-small font-medium",
                  !v.gated ? "bg-pill-neutral-bg text-pill-neutral-ink" : result.kind === "pass" ? "bg-pill-positive-bg text-pill-positive-ink" : "bg-pill-warning-bg text-pill-warning-ink",
                )}
              >
                <MarkShape mark={!v.gated ? "queued" : result.kind === "pass" ? "done" : "held"} />
                {v.gated ? result.pill : "Waiting"}
              </span>
            </div>
            <motion.p key={`${c.uid}-${v.gated}`} initial={reduceMotion ? false : { opacity: 0 }} animate={{ opacity: 1 }} className="m-0 mt-2 text-body">
              {v.gated ? result.text : v.agreed ? "Comparing the two readers check by check…" : "Nothing closes until both readers have answered and agree on every check."}
            </motion.p>
          </div>
        </section>
      </div>
    </>
  );
}

function Cell({ who, mark, word, sub }: { who: string; mark: Mark; word: string; sub?: string }) {
  return (
    <span className="flex min-w-0 items-center gap-2">
      <span className="w-24 flex-none text-small text-muted @xl:hidden">{who}</span>
      {/* A settled answer needs no mark; queued and working keep theirs, so the replay's progress stays visible. */}
      {mark !== "done" && <MarkShape mark={mark} hue={mark === "working"} className="size-2.5" />}
      <span className="flex min-w-0 flex-col">
        <span className={cn("break-words text-body", mark === "done" ? "text-ink" : "text-muted")}>{word}</span>
        {sub ? <span className="break-words text-small text-muted">{sub}</span> : null}
      </span>
    </span>
  );
}
