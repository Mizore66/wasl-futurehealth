"use client";

import { motion } from "motion/react";
import readerJson from "@/data/reader.json";
import { cn } from "@/lib/utils";
import { MarkShape, PlaybackControls, Picker } from "./controls";
import type { Mark } from "./flow";
import { usePlayback } from "./playback";
import { FIELDS, READER_SPEEDUP, READER_TICK_MS, readerFrames, readerStart, readerTrack, readerView } from "./reader-timeline";
import type { ReaderAnswer, ReaderCase, ReaderData, ReaderField } from "./reader-types";

const reader = readerJson as ReaderData;
const track = readerTrack(reader);
const frames = readerFrames(reader);

const WORDS: Record<Exclude<ReaderField, "histology">, Record<string, string>> = {
  caecum: { yes: "reached", no: "not reached", not_stated: "not stated" },
  prep: { adequate: "adequate", inadequate: "inadequate", not_stated: "not graded" },
  polyps: { yes: "polyp removed", no: "none removed" },
};

function valueWord(a: ReaderAnswer, k: ReaderField): string {
  if (k === "histology") return a.histology === "yes" ? "back" : a.polyps === "yes" ? "not back" : "not needed";
  return WORDS[k][a[k]] ?? a[k].replaceAll("_", " ");
}

const REASON_WORDS: Record<string, string> = {
  incomplete_exam: "the scope didn't reach the end, so this colonoscopy doesn't count yet",
  inadequate_prep: "the bowel prep was not adequate",
  awaiting_histology: "tissue was taken and histology is not back",
  missing_documentation: "the report does not document every check",
};

function verdict(c: ReaderCase, disagree: ReaderField[]): { kind: "pass" | "held" | "review"; pill: string; text: string; tech: string } {
  if (disagree.length > 0) {
    const names = disagree.map((k) => FIELDS.find((f) => f.k === k)!.name.toLowerCase()).join(", ");
    return {
      kind: "review",
      pill: "Two reviewers",
      text: `The readers disagree on ${names}, so nothing closes by itself. Waiting for two reviewers.`,
      tech: `AI ${c.ai.state} · ${c.ai.reason} · rules ${c.rules.state} · ${c.rules.reason}`,
    };
  }
  if (c.ai.state === "closed") {
    return { kind: "pass", pill: "Passed", text: "Both readers agree on all four checks. Closed with evidence.", tech: `${c.ai.state} · ${c.ai.reason} · per-field agreement` };
  }
  return {
    kind: "held",
    pill: "Held",
    text: `Both readers agree: ${REASON_WORDS[c.ai.reason] ?? c.ai.reason}.`,
    tech: `${c.ai.state} · ${c.ai.reason}`,
  };
}

const s1 = (ms: number) => (ms / 1000).toFixed(1);

export function ReadingReplay({ heading, standsFor }: { heading: React.ReactNode; standsFor: Record<string, string> }) {
  const play = usePlayback(track, READER_TICK_MS);
  const v = readerView(reader, play.frame);
  const { c, f } = v;
  const t = frames.perCase[v.index];
  const result = verdict(c, v.disagree);

  const stageState = (done: boolean, started: boolean): Mark => (done ? "done" : started ? "working" : "queued");
  const gateMark: Mark = v.gated ? (result.kind === "pass" ? "done" : "held") : v.agreed ? "working" : "queued";
  const stages: { label: string; mark: Mark; detail: string }[] = [
    { label: "Received", mark: "done", detail: `report ${c.uid}` },
    { label: "Rules reader", mark: stageState(v.rulesDone, f >= 1), detail: "DoH checklist" },
    { label: "AI reader", mark: stageState(v.aiDone, f >= 1), detail: `${s1(v.elapsedMs)} s` },
    { label: "Agreement", mark: stageState(v.agreed, v.aiDone), detail: v.agreed ? `${4 - v.disagree.length} of 4` : "" },
    { label: "Safety gate", mark: gateMark, detail: v.gated ? result.pill : "" },
  ];
  const WORD: Record<Mark, string> = { queued: "queued", working: "working", done: "done", held: "held", failed: "failed" };

  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-6">
        {heading}
        <PlaybackControls
          paused={play.paused}
          onToggle={play.toggle}
          onStep={play.step}
          position={`Report ${v.index + 1} of ${reader.cases.length} · step ${f + 1} of ${t.frames}`}
          speed={`AI reading times are the recorded ones, played ${READER_SPEEDUP}× faster.`}
        >
          <Picker
            label="Report"
            options={reader.cases.map((x, i) => ({ value: i, label: `Report ${x.uid}` }))}
            value={v.index}
            onPick={(i) => play.seek(readerStart(reader, i))}
          />
        </PlaybackControls>
      </header>

      <section aria-label="Engine steps" className="mt-4 rounded-card border border-line bg-surface px-6 py-4">
        <ol className="m-0 grid list-none grid-cols-5 gap-x-3 p-0">
          {stages.map((g, i) => (
            <li key={g.label} className="relative min-w-0 pt-5" data-mark={g.mark}>
              {i < stages.length - 1 && (
                <span aria-hidden className={cn("absolute left-3.5 -right-2 top-1 h-0.5", stages[i + 1].mark === "queued" ? "bg-control" : "bg-ink")} />
              )}
              <MarkShape mark={g.mark} hue={g.mark === "working"} className="absolute left-0 top-0 size-2.5" />
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
        <section aria-labelledby="report-h" className="min-w-0 flex-[0_1_420px] rounded-card border border-line bg-surface px-6 py-5">
          <h2 id="report-h" className="m-0 text-section font-medium">
            Colonoscopy report {c.uid}
          </h2>
          <p className="m-0 truncate text-small text-muted">{standsFor[c.uid] ?? "Not tied to a demo patient"}</p>
          <div className="mt-3 h-[268px] overflow-hidden rounded-control bg-ground px-4 py-3">
            <p className="m-0 text-body leading-[22px]" data-testid="report-text">
              {c.segments.map((s, i) => {
                const lit = !!s.field && v.aiDone;
                const disputed = lit && v.agreed && v.disagree.includes(s.field!);
                return (
                  <span key={`${c.uid}-${i}`}>
                    {lit && (
                      <span className={cn("mr-1 text-small font-medium", disputed ? "text-warning" : "text-hue")}>
                        {s.field}
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
          <p className="m-0 mt-2 text-small text-muted">
            Excerpt of MTSamples {c.uid} (&ldquo;{c.title}&rdquo;), a real US dictation, attached to a synthetic patient. Highlights are the AI reader&apos;s own quotes. Ledger row{" "}
            {c.sha.slice(0, 8)}.
          </p>
        </section>

        <section aria-labelledby="checks-h" className="min-w-0 flex-[999_1_560px] rounded-card border border-line bg-surface px-6 py-5">
          <div className="flex items-baseline justify-between gap-3">
            <h2 id="checks-h" className="m-0 text-section font-medium">
              The four checks for a complete colonoscopy
            </h2>
            <span className="whitespace-nowrap text-small text-muted">
              report {v.index + 1} of {reader.cases.length}
            </span>
          </div>

          <div className="mt-3 grid grid-cols-[minmax(0,24fr)_minmax(0,22fr)_minmax(0,34fr)_minmax(0,20fr)] items-end gap-x-3 pb-2">
            <span className="text-small text-muted">Check</span>
            <span className="flex flex-col">
              <span className="text-body font-medium">Rules reader</span>
              <span className="text-small text-muted">DoH checklist, word for word</span>
            </span>
            <span className="flex min-w-0 flex-col">
              <span className="truncate text-body font-medium">AI reader · Gemma 3 4B</span>
              <span className="flex items-center gap-2 text-small text-muted">
                <span className="h-1 w-16 flex-none rounded-sm bg-raised">
                  <span className="block h-1 rounded-sm bg-hue" style={{ width: `${Math.round((v.elapsedMs / c.ai.latencyMs) * 100)}%` }} />
                </span>
                <span className="truncate" data-testid="ai-timer">
                  {f < 1 ? "queued" : v.aiDone ? `${s1(c.ai.latencyMs)} s recorded` : `reading · ${s1(v.elapsedMs)} s`}
                </span>
              </span>
            </span>
            <span className="text-small text-muted">Agreement</span>
          </div>

          {FIELDS.map((fd) => {
            const same = c.rules[fd.k] === c.ai[fd.k];
            const downgraded = c.ai.downgraded.includes(fd.k);
            const quoted = c.segments.some((s) => s.field === fd.k);
            return (
              <div key={fd.k} data-field={fd.k} className="grid h-16 grid-cols-[minmax(0,24fr)_minmax(0,22fr)_minmax(0,34fr)_minmax(0,20fr)] items-center gap-x-3 border-t border-rule">
                <span className="flex min-w-0 flex-col">
                  <span className="text-body font-medium">{fd.name}</span>
                  <span className="font-mono text-small text-muted">{fd.key}</span>
                </span>
                <Cell mark={stageState(v.rulesDone, f >= 1)} word={v.rulesDone ? valueWord(c.rules, fd.k) : f >= 1 ? "reading" : "queued"} />
                <Cell
                  mark={stageState(v.aiDone, f >= 1)}
                  word={v.aiDone ? valueWord(c.ai, fd.k) : f >= 1 ? "reading" : "queued"}
                  sub={v.aiDone ? (downgraded ? "downgraded by the reader's own check" : quoted ? "quote found in report" : "") : ""}
                />
                <span className="inline-flex items-center gap-1.5 whitespace-nowrap">
                  <svg aria-hidden width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" className={cn("flex-none", !v.agreed ? "text-control" : same ? "text-positive" : "text-warning")}>
                    <path d={!v.agreed ? "M4 8h8" : same ? "M3 8.5l3 3 7-7" : "M4 4l8 8M12 4l-8 8"} />
                  </svg>
                  <span className={cn("text-body", !v.agreed ? "text-muted" : same ? "text-ink" : "font-medium text-warning")}>
                    {!v.agreed ? "waiting" : same ? "agree" : "disagree"}
                  </span>
                </span>
              </div>
            );
          })}

          <div role="status" className="mt-2 min-h-[104px] rounded-control bg-ground p-4">
            <div className="flex items-center gap-3">
              <span className="text-body font-medium">Safety gate</span>
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
            <motion.p key={`${c.uid}-${v.gated}`} initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="m-0 mt-2 text-body">
              {v.gated ? result.text : v.agreed ? "Comparing the two readers field by field…" : "Nothing closes until both readers have answered and agree on every check."}
            </motion.p>
            <p className="m-0 mt-1 font-mono text-small text-muted">{v.gated ? result.tech : "policy · two-reader agreement per field"}</p>
          </div>
          <p className="m-0 mt-2 font-mono text-small text-muted">model_version {c.ai.modelVersion}</p>
        </section>
      </div>
    </>
  );
}

function Cell({ mark, word, sub }: { mark: Mark; word: string; sub?: string }) {
  return (
    <span className="flex min-w-0 items-center gap-2">
      <MarkShape mark={mark} hue={mark === "working"} className="size-2.5" />
      <span className="flex min-w-0 flex-col">
        <span className={cn("truncate text-body", mark === "done" ? "text-ink" : "text-muted")}>{word}</span>
        {sub ? <span className="truncate text-small text-muted">{sub}</span> : null}
      </span>
    </span>
  );
}
