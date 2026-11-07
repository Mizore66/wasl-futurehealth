import { describe, expect, it, vi } from "vitest";
import replayJson from "@/data/replay.json";
import readerJson from "@/data/reader.json";
import { FEED_ROWS, ROWS, SLOTS, flowView, replayTrack } from "@/components/pipeline/flow";
import { DEPENDENCY_ORDER, loadHealth, stepAnnouncement } from "@/components/pipeline/health";
import { Healthz } from "@/lib/api/schema";
import { getHealthz } from "@/lib/api/source";
import { playback,type Playback, type PlaybackAction } from "@/components/pipeline/playback";
import { readerFrames, readerStart, readerTrack, readerView } from "@/components/pipeline/reader-timeline";
import { NOT_ASKED, type ReaderData } from "@/components/pipeline/reader-types";
import { profileOf } from "@/components/pipeline/profile";
import { initialState, next, type ReplayState } from "@/lib/replay/reducer";
import type { Replay } from "@/lib/replay/types";
import { personForLoop } from "@/lib/people";
import { referralSeed } from "@/lib/people/identity";

const replay = replayJson as Replay;
const reader = readerJson as ReaderData;
const track = replayTrack(replay);

function run(actions: PlaybackAction<ReplayState>[]): Playback<ReplayState>[] {
  const states: Playback<ReplayState>[] = [{ frame: track.rest, paused: false }];
  for (const a of actions) states.push(playback(track, states.at(-1)!, a));
  return states;
}

describe("pipeline plain language", () => {
  const RAW = /closure_decision|needs_human|not_closed|link_decision|evidence_closure|false_closure_trap|review_hold/;

  it("shows no backend identifier in a stage label or note, and keeps the raw term for the disclosure", () => {
    for (const r of ROWS) {
      expect(`${r.label} ${r.note}`).not.toMatch(RAW);
      expect(r.tech).toBeTruthy();
    }
    expect(ROWS.find((r) => r.id === "review")).toMatchObject({ label: "Waiting for two reviewers", tech: "needs_human" });
  });

  it("gives every decision line a plain note apart from its technical label", () => {
    const v = flowView(replay, track.rest);
    for (const f of v.feed) {
      expect(f.note).not.toMatch(RAW);
      expect(f.note).not.toMatch(/_/);
    }
  });

  it("pauses on request without moving the frame", () => {
    const [start, paused] = run([{ type: "pause" }]);
    expect(paused).toEqual({ frame: start.frame, paused: true });
    expect(run([{ type: "pause" }, { type: "tick" }])[2].frame).toBe(start.frame);
  });
});

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

describe("replay people", () => {
  const byId = new Map(replay.cases.map((c) => [c.id, c]));
  const views = run(Array.from({ length: replay.events.length }, () => ({ type: "tick" as const }))).map((s) => flowView(replay, s.frame));

  it("names every chip, feed line and ledger line after a person, never a case number", () => {
    for (const v of views) {
      const names = [...v.rows.flatMap((r) => r.chips.map((c) => c.who.name)), ...v.feed.map((f) => f.who.name), ...(v.latestRecorded ? [v.latestRecorded.who.name] : [])];
      for (const n of names) expect(n).toMatch(/^[A-Z][a-z]+ [A-Z][A-Za-z ]+$/);
    }
  });

  it("shows the report's own Latin spelling and \"?\" until the report is matched, then the person's name and face", () => {
    let unknown = 0;
    for (const v of views)
      for (const r of v.rows)
        for (const chip of r.chips) {
          const person = byId.get(chip.id)!.person;
          const matched = r.id !== "received" && !(r.id === "matching" && chip.mark === "working");
          expect(chip.who.face === "unknown").toBe(!matched);
          expect(chip.who.name).toBe(matched ? person.name : person.reportName);
          expect(chip.who.seed).toEqual({ key: person.key, sex: person.sex });
          if (!matched) unknown++;
        }
    expect(unknown).toBeGreaterThan(0);
    expect(replay.cases.some((c) => c.person.reportName !== c.person.name)).toBe(true);
  });

  it("gives each generated person a distinct name in Latin and Arabic, and keys them on the report, not the case slot", () => {
    const generated = replay.cases.filter((c) => c.person.source === "generated");
    expect(generated.length).toBeGreaterThan(15);
    expect(new Set(generated.map((c) => c.person.name)).size).toBe(generated.length);
    for (const c of generated) {
      expect(c.person.nameAr).toMatch(/^[؀-ۿ]+ ال[؀-ۿ]+$/);
      expect(c.person.key).toBe(`replay|${c.read.reportSha.slice(0, 16)}`);
    }
  });

  it("keeps the cast member, name and face, where the Report reading page tells a replayed report as theirs", () => {
    const told = reader.cases.filter((r) => r.standsFor && replay.cases.some((c) => c.read.reportSha === r.sha));
    expect(told.length).toBeGreaterThanOrEqual(2);
    for (const r of told) {
      const c = replay.cases.find((x) => x.read.reportSha === r.sha)!;
      const cast = personForLoop(r.standsFor!.loop)!;
      expect(c.person).toMatchObject({ source: "canonical-cast", name: cast.display_name, nameAr: cast.display_name_ar, key: referralSeed(r.standsFor!.loop).key });
    }
  });

  it("lists the feed newest first", () => {
    for (const v of views) for (let i = 1; i < v.feed.length; i++) expect(v.feed[i].key).toBeLessThan(v.feed[i - 1].key);
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

describe("pipeline person profile", () => {
  const rules = Object.fromEntries(reader.cases.map((c) => [c.sha, c.rules]));
  const fatima = replay.cases.find((c) => c.person.name === "Fatima Al Zaabi")!;
  const generated = replay.cases.find((c) => c.person.source === "generated")!;

  it("links a demo cast member to her patient record, with both readers on record", () => {
    const p = profileOf(replay, track.rest, fatima.id, rules)!;
    expect(p.record).toBe(`/app/patients/${encodeURIComponent(personForLoop("ahs-mushrif/ServiceRequest/AHS-sr-1037")!.loop)}`);
    expect(p.rulesOnRecord).toBe(true);
    expect(p.fields.find((f) => f.key === "caecum_reached")).toMatchObject({ value: "yes", rules: "yes" });
    expect(p.decision).toEqual({ word: "Closed with evidence", label: "closed · criteria_met", note: "Criteria met", gatePassed: true });
  });

  it("gives a generated person no record and only the AI reader's answer", () => {
    const p = profileOf(replay, track.rest, generated.id, rules)!;
    expect(p.record).toBeNull();
    expect(p.rulesOnRecord).toBe(false);
    expect(p.fields.every((f) => f.rules === undefined)).toBe(true);
  });

  it("lists every step of the person's report, marking which the current frame has played", () => {
    const own = replay.events.flatMap((e, i) => ("caseId" in e && e.caseId === fatima.id ? [i + 1] : []));
    expect(profileOf(replay, initialState, fatima.id, rules)!.history.map((h) => h.step)).toEqual(own);
    expect(profileOf(replay, initialState, fatima.id, rules)!.history.every((h) => !h.played)).toBe(true);
    expect(profileOf(replay, initialState, fatima.id, rules)!.now).toBe(`Not in this loop yet. The report arrives at step ${own[0]}.`);

    let s = initialState;
    while (s.cursor < own.at(-1)!) s = next(replay, s);
    const done = profileOf(replay, s, fatima.id, rules)!;
    expect(done.history.every((h) => h.played)).toBe(true);
    expect(done.now).toBe("Closed with evidence");
    expect(done.matched).toBe(true);
  });

  it("drops quote fragments that hold no words", () => {
    const p = profileOf(replay, track.rest, "showcase-01", rules)!;
    expect(p.fields.find((f) => f.key === "prep")!.quote).toBe("");
  });
});

describe("step announcement (issue #16)", () => {
  it("names the step and the newest decision, and only that decision", () => {
    const after = flowView(replay, track.advance(track.rest));
    const text = stepAnnouncement(after);
    expect(text).toContain(`Step ${after.step} of ${after.steps}.`);
    expect(text).toContain(after.feed[0].who.name);
    expect(text).toContain(after.feed[0].text);
    expect(text.split("Newest decision").length).toBe(2);
  });

  it("says so when nothing is recorded yet", () => {
    expect(stepAnnouncement({ step: 0, steps: 5, feed: [] })).toBe("Step 0 of 5. No decision recorded yet.");
  });
});

describe("health state (issue #11)", () => {
  const ok: Healthz = {
    status: "ok",
    cases: 7,
    dependencies: { materialised_run: "loaded", nats: "not_required_for_read_api", reader: "cached", ingest_adapter: "fhir" },
    meta: { api: "wasl-api/1", data_mode: "simulated", loaded_at: "2026-10-31T23:47:27Z", note: "", policy: "p", reader_model: "m", run: "r" },
  };

  it("is a published snapshot, dated by meta.loaded_at, in fixtures mode", async () => {
    expect(await loadHealth(async () => ok, "fixtures")).toEqual({ kind: "snapshot", health: ok, takenAt: "2026-10-31T23:47:27Z" });
  });

  it("is a live check, dated by the fetch, in live mode", async () => {
    const state = await loadHealth(async () => ok, "live", () => new Date("2026-11-07T10:00:00Z"));
    expect(state).toEqual({ kind: "live", health: ok, fetchedAt: "2026-11-07T10:00:00.000Z" });
  });

  it("is explicitly unavailable when the health call fails", async () => {
    const state = await loadHealth(() => Promise.reject(new Error("/healthz: HTTP 503")), "live");
    expect(state).toEqual({ kind: "unavailable", reason: "/healthz: HTTP 503" });
  });

  it("is unavailable when the payload does not parse", async () => {
    const state = await loadHealth(async () => Healthz.parse({ status: "ok" }), "fixtures");
    expect(state.kind).toBe("unavailable");
  });

  it("getHealthz fails in live mode when the API is unreachable, and the state is unavailable", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:1");
    try {
      expect((await loadHealth(getHealthz, "live")).kind).toBe("unavailable");
    } finally {
      vi.unstubAllEnvs();
    }
  });

  it("getHealthz reads the published snapshot, which carries the four dependencies", async () => {
    const h = await getHealthz();
    for (const k of DEPENDENCY_ORDER) expect(h.dependencies[k]).toBeTruthy();
  });
});
