"use client";

import { useState } from "react";
import { cn } from "@/lib/utils";
import type { ReviewView, Verdict } from "./case-view";
import { Mark, type Shape, type Tone } from "./marks";

const VERDICT: Record<Verdict, { shape: Shape; tone: Tone }> = {
  agrees: { shape: "tick", tone: "positive" },
  matches: { shape: "tick", tone: "positive" },
  missing: { shape: "dash", tone: "warning" },
  differs: { shape: "cross", tone: "muted" },
};

const ARABIC = /[؀-ۿ]/;
const CELL = "flex h-9 items-center overflow-hidden text-ellipsis whitespace-nowrap border-t border-rule";

function RecordCard({ name, source, values, arabic }: { name: string; source: string; values: string[]; arabic?: boolean }) {
  return (
    <div className="min-w-0 rounded-control border border-line px-4">
      <div className="flex h-16 flex-col justify-center">
        <div lang={arabic ? "ar" : undefined} dir={arabic ? "rtl" : undefined} className={cn("font-medium", arabic ? "pb-1 text-left font-arabic text-[22px] leading-[30px]" : "text-title")}>
          {name}
        </div>
        <div className="text-small text-muted">{source}</div>
      </div>
      {values.map((v, i) => (
        <div key={i} className={cn(CELL, v === "not on report" ? "text-muted" : "text-ink")}>
          <span lang={ARABIC.test(v) ? "ar" : undefined} dir={ARABIC.test(v) ? "rtl" : undefined} className={cn(ARABIC.test(v) && "font-arabic")}>
            {v}
          </span>
        </div>
      ))}
    </div>
  );
}

/** Link score on a 0.95 to 1.00 scale, with the auto-close line. */
function ScoreTrack({ score, line }: { score: number; line: number }) {
  const at = (v: number) => `${Math.min(100, Math.max(0, ((v - 0.95) / 0.05) * 100))}%`;
  const above = score >= line;
  return (
    <div
      role="img"
      aria-label={`Link score ${score.toFixed(3)} on a scale from 0.95 to 1.00, ${above ? "at or above" : "below"} the ${line} auto-close line.`}
      className="relative mt-2 h-[52px]"
    >
      <div className="absolute inset-x-0 top-[22px] h-1 rounded-sm bg-control" />
      <div className="absolute top-3.5 h-5 w-0.5 -ml-px bg-ink" style={{ left: at(line) }} />
      <div className="absolute -top-0.5 ml-1.5 whitespace-nowrap text-small" style={{ left: at(line) }}>
        auto-close line {line}
      </div>
      <div className="absolute top-[19px] -ml-[5px] size-2.5 rounded-full bg-hue" style={{ left: at(score) }} />
      <div className="absolute left-0 top-[34px] text-small text-muted">0.95</div>
      <div className="absolute right-0 top-[34px] text-small text-muted">1.00</div>
    </div>
  );
}

export function IdentityBridge({ view }: { view: ReviewView }) {
  const [why, setWhy] = useState(false);
  return (
    <section aria-labelledby="bridge-h" className="rounded-card border border-line bg-surface p-6">
      <div className="flex items-center justify-between gap-3">
        <h2 id="bridge-h" className="m-0 text-section font-medium">
          Is this the same person?
        </h2>
        <button
          type="button"
          aria-expanded={why}
          aria-controls="why-held"
          onClick={() => setWhy(!why)}
          className="inline-flex h-8 items-center gap-1.5 rounded-control px-2 hover:bg-active"
        >
          <span>{why ? "Hide why" : "Why it was held"}</span>
          <svg aria-hidden width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" className={cn(why && "rotate-180")}>
            <path d="M3 4.5l3 3 3-3" />
          </svg>
        </button>
      </div>

      <div className="mt-3 grid grid-cols-[minmax(0,1fr)_176px_minmax(0,1fr)] gap-3">
        <RecordCard name={view.referralName} source={view.referralSource} values={view.identity.map((r) => r.referral)} arabic />
        <div>
          <div aria-hidden className="relative h-16">
            <div className="absolute -inset-x-3 top-[31px] h-0.5 bg-ink" />
            <div className="absolute left-1/2 top-[27px] -ml-[5px] size-2.5 rounded-full border-2 border-ink bg-surface" />
            <div className="absolute inset-x-0 top-[42px] text-center text-small text-muted">proposed link</div>
          </div>
          {view.identity.map((r) => (
            <div key={r.field} className="flex h-9 items-center gap-1.5 border-t border-rule px-1">
              <span className="flex-1 whitespace-nowrap">{r.field}</span>
              <Mark {...VERDICT[r.verdict]} className={cn("text-small", r.verdict === "missing" && "text-warning")}>
                {r.verdict}
              </Mark>
            </div>
          ))}
        </div>
        <RecordCard name={view.reportName} source={view.reportSource} values={view.identity.map((r) => r.report)} />
      </div>

      <p className="m-0 mt-4 font-medium">{view.identitySummary}</p>

      {why && (
        <div id="why-held" className="mt-3 rounded-control bg-ground p-4">
          <div className="flex items-baseline justify-between">
            <span className="font-medium">Link score</span>
            <span>
              {view.link.score.toFixed(3)} <span className="text-small text-muted">on {view.link.basis}, auto-close line {view.link.line}</span>
            </span>
          </div>
          <ScoreTrack score={view.link.score} line={view.link.line} />
          <p className="m-0 mt-2 text-small leading-[18px] text-muted">
            Held anyway. Before Wasl messages her about this appointment, the rule asks for a {view.link.rule}.
          </p>
          <p className="m-0 mt-1 text-small leading-[18px] text-muted">
            Readers: the rules reader applies the DoH closure checklist word for word; the second reader is a language model ({view.readerModel}). Both must
            agree for a report to close a case.
          </p>
        </div>
      )}
    </section>
  );
}
