import type { Track } from "./playback";
import type { ReaderCase, ReaderData, ReaderField } from "./reader-types";

/** One tick on screen stands for 4.5 s of the recorded run: the reader's real latency, played 5x faster. */
const TICK_MS = 900;
const SPEEDUP = 5;
const RECORDED_MS_PER_FRAME = TICK_MS * SPEEDUP;
/** Frames a finished verdict stays on screen before the next report. */
const HOLD = 4;

export const READER_TICK_MS = TICK_MS;
export const READER_SPEEDUP = SPEEDUP;

export function readerFrames(data: ReaderData) {
  const perCase = data.cases.map((c) => {
    const aiDone = 1 + Math.ceil(c.ai.latencyMs / RECORDED_MS_PER_FRAME);
    return { aiDone, agree: aiDone + 1, gate: aiDone + 2, frames: aiDone + 2 + HOLD };
  });
  const offsets = perCase.map((_, i) => perCase.slice(0, i).reduce((n, t) => n + t.frames, 0));
  const total = perCase.reduce((n, t) => n + t.frames, 0);
  return { perCase, offsets, total, tickMs: TICK_MS, recordedMsPerFrame: RECORDED_MS_PER_FRAME };
}

export const FIELDS: { k: ReaderField; name: string; key: string }[] = [
  { k: "caecum", name: "Caecum reached", key: "caecum_reached" },
  { k: "prep", name: "Bowel prep graded", key: "prep" },
  { k: "polyps", name: "Polyp removed", key: "polyps_removed" },
  { k: "histology", name: "Histology back", key: "histology_present" },
];

export type ReaderView = {
  index: number;
  c: ReaderCase;
  /** Frame within this report: 0 received, 1 both readers start. */
  f: number;
  rulesDone: boolean;
  aiDone: boolean;
  agreed: boolean;
  gated: boolean;
  elapsedMs: number;
  disagree: ReaderField[];
};

export function readerView(data: ReaderData, frame: number): ReaderView {
  const { perCase, offsets } = readerFrames(data);
  let index = 0;
  while (index + 1 < data.cases.length && frame >= offsets[index + 1]) index++;
  const c = data.cases[index];
  const t = perCase[index];
  const f = frame - offsets[index];
  return {
    index,
    c,
    f,
    rulesDone: f >= 2,
    aiDone: f >= t.aiDone,
    agreed: f >= t.agree,
    gated: f >= t.gate,
    elapsedMs: f < 1 ? 0 : Math.min((f - 1) * RECORDED_MS_PER_FRAME, c.ai.latencyMs),
    disagree: FIELDS.filter((x) => c.rules[x.k] !== c.ai[x.k]).map((x) => x.k),
  };
}

/** First frame of report i. */
export function readerStart(data: ReaderData, i: number): number {
  return readerFrames(data).offsets[i];
}

export function readerTrack(data: ReaderData): Track<number> {
  const { perCase, total } = readerFrames(data);
  return { rest: perCase[0].gate, advance: (f) => (f + 1) % total };
}
