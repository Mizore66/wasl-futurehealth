import { describe, expect, it } from "vitest";
import replayJson from "@/data/replay.json";
import { initialState, next, type ReplayState } from "@/lib/replay/reducer";
import { COUNTERS, type Replay } from "@/lib/replay/types";

const replay = replayJson as Replay;

function walk(): ReplayState[] {
  const states = [initialState];
  for (let i = 0; i < replay.events.length; i++) states.push(next(replay, states.at(-1)!));
  return states;
}

describe("replay loop", () => {
  const states = walk();

  it("never exceeds the source totals", () => {
    for (const s of states) for (const k of COUNTERS) expect(s.counters[k]).toBeLessThanOrEqual(replay.totals[k]);
  });

  it("reaches the real totals exactly before the loop ends", () => {
    expect(states.at(-2)!.counters).toEqual(replay.totals);
  });

  it("returns to state 0 after one full loop", () => {
    expect(states.at(-1)).toEqual(initialState);
  });

  it("is a loop: a second pass replays the first", () => {
    let s = states.at(-1)!;
    for (let i = 0; i < 10; i++) s = next(replay, s);
    expect(s).toEqual(states[10]);
  });

  it("ends every case in its scripted outcome, in phase order", () => {
    const end = states.at(-2)!;
    for (const c of replay.cases) expect(end.cases[c.id]).toEqual({ phase: "recorded", gatePassed: c.gatePassed, outcome: c.outcome });
  });

  it("passes the gate only for reports the reader closed", () => {
    for (const c of replay.cases) expect(c.gatePassed).toBe(c.read.state === "closed");
  });

  it("carries the real totals of the showcase run", () => {
    expect(replay.totals).toEqual({ fhirMessages: 40983, readerReports: 245, outputs: 31677, chainsVerified: 3163 });
    expect(replay.facts).toMatchObject({ duplicates: 0, lost: 0, throughputEvS: 895, tamperAttacks: 17, tamperDetected: 17 });
  });
});
