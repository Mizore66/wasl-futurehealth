import { describe, expect, it } from "vitest";
import replayJson from "@/data/replay.json";
import readerJson from "@/data/reader.json";
import { FEED_ROWS, ROWS, SLOTS, flowView, replayTrack } from "@/components/pipeline/flow";
import { playback, type Playback, type PlaybackAction } from "@/components/pipeline/playback";
import { readerFrames, readerStart, readerTrack, readerView } from "@/components/pipeline/reader-timeline";
import { NOT_ASKED, type ReaderData } from "@/components/pipeline/reader-types";
import type { ReplayState } from "@/lib/replay/reducer";
import type { Replay } from "@/lib/replay/types";

const replay = replayJson as Replay;
const reader = readerJson as ReaderData;
const track = replayTrack(replay);

function run(actions: PlaybackAction<ReplayState>[]): Playback<ReplayState>[] {
  const states: Playback<ReplayState>[] = [{ frame: track.rest, paused: false }];
  for (const a of actions) states.push(playback(track, states.at(-1)!, a));
  return states;
}

describe("pipeline playback", () => {
  it("rests on a frame that fills most stage rows and already has decisions on the feed", () => {
    const v = flowView(replay, track.rest);
    expect(v.rows.filter((r) => r.count > 0).length).toBeGreaterThanOrEqual(6);
    expect(v.feed.length).toBe(FEED_ROWS);
  });

  it("does not move on ticks while paused, and Step pauses then moves exactly one event", () => {
    const [start, paused, tick] = run([{ type: "toggle" }, { type: "tick" }]);
    expect(tick.frame).toBe(paused.frame);
    const [, stepped] = run([{ type: "step" }]);
    expect(stepped.paused).toBe(true);
    expect(stepped.frame.cursor).toBe(start.frame.cursor + 1);
  });

  it("over two full loops of ticks, totals never pass the run's totals and every view stays inside its slots", () => {
    const states = run(Array.from({ length: replay.events.length * 2 }, () => ({ type: "tick" as const })));
    let reached = false;
    for (const s of states) {
      const v = flowView(replay, s.frame);
      expect(v.decisions).toBeLessThanOrEqual(31_677);
      expect(v.chains).toBeLessThanOrEqual(3_163);
      if (v.decisions === replay.totals.outputs && v.chains === replay.totals.chainsVerified) reached = true;
      expect(v.rows).toHaveLength(ROWS.length);
      expect(v.feed.length).toBeLessThanOrEqual(FEED_ROWS);
      for (const r of v.rows) expect(r.chips.length + (r.more ? 1 : 0)).toBeLessThanOrEqual(SLOTS);
      const ledger = v.rows.find((r) => r.id === "ledger")!;
      expect(v.rows.filter((r) => r !== ledger).reduce((n, r) => n + r.count, 0)).toBe(s.frame.order.length);
      expect(ledger.count).toBeLessThanOrEqual(s.frame.order.length);
    }
    expect(reached).toBe(true);
  });

  it("marks only the case that just moved as active", () => {
    const v = flowView(replay, track.rest);
    const active = v.rows.flatMap((r) => r.chips).filter((c) => c.active);
    expect(active.length).toBeLessThanOrEqual(1);
    expect(v.activeCase).not.toBeNull();
  });
});

describe("reader playback", () => {
  it("never shows more elapsed reading time than the recorded latency, and plays it 5x faster", () => {
    const frames = readerFrames(reader);
    let s = readerTrack(reader).rest;
    for (let i = 0; i < frames.total * 2; i++) {
      const v = readerView(reader, s);
      expect(v.elapsedMs).toBeLessThanOrEqual(v.c.ai.latencyMs);
      s = readerTrack(reader).advance(s);
    }
    expect(frames.recordedMsPerFrame / frames.tickMs).toBe(5);
  });

  it("rests on the first report the moment its verdict lands", () => {
    const v = readerView(reader, readerTrack(reader).rest);
    expect(v.index).toBe(0);
    expect(v.gated).toBe(true);
  });

  it("carries the recorded answers: u38's readers disagree on the caecum and only u31 closes", () => {
    const [u31, u6, u38] = reader.cases;
    expect([u31.uid, u6.uid, u38.uid]).toEqual(["u31", "u6", "u38"]);
    expect(u31.ai).toMatchObject({ state: "closed", reason: "criteria_met", latencyMs: 8849 });
    expect(u6.ai).toMatchObject({ state: "not_closed", reason: "incomplete_exam", latencyMs: 11421 });
    expect(u38.ai).toMatchObject({ state: "needs_human", caecum: "not_stated", downgraded: ["caecum"], latencyMs: 10130 });
    expect(u38.rules.caecum).toBe("yes");
    expect(readerView(reader, readerStart(reader, 2)).disagree).toEqual(["caecum"]);
  });

  it("compares tissue only where the older rules reader answered it", () => {
    const u6 = reader.cases[1];
    expect(u6.ai.tissue).toBe("yes");
    expect(u6.rules.tissue).toBe(NOT_ASKED);
    const v = readerView(reader, readerStart(reader, 1));
    expect(v.compared).not.toContain("tissue");
    expect(v.disagree).toEqual([]);
  });
});
