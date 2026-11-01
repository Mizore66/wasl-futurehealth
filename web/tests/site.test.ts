import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { QUOTED, exactlyOnce, missedByIdOnly, missedOfFive, published, upperBound } from "@/components/site/figures";
import { LTT_SOURCE, LTT_TIERS, OPERATING_TAU } from "@/components/site/ltt";
import { CAMERA_KEYS } from "@/components/story/rig";
import { JOIN_SAYS } from "@/components/story/copy";
import { BEATS, CAST, CUES, JOIN_BARS, TRACK_VH, WALK, beatAt, cueAt, cueOn, dayAt, pageAt, storyAt } from "@/components/story/script";
import { PEOPLE } from "@/components/site/name-joiner";
import { result } from "@/lib/results";

const repo = path.resolve(import.meta.dirname, "../..");
const read = (file: string) => readFileSync(path.join(repo, file), "utf8").replace(/\r\n/g, "\n");

describe("site figures", () => {
  it("rounds the ID-only miss rate (62%) to 3 in 5", () => {
    expect(Math.round(missedByIdOnly() * 100)).toBe(62);
    expect(missedOfFive()).toBe(3);
  });

  it.each(Object.entries(QUOTED))("%s is still quoted in its source file", (_, q) => {
    expect(read(q.file)).toContain(q.quote);
    expect(q.quote).toContain(q.value);
  });

  it("reads the published Abu Dhabi wait from results.json, labelled published and still in its source", () => {
    expect([published("crc_followup_median_wait_days"), published("crc_followup_standard_days")]).toEqual([72, 31]);
    const e = result("published.crc_followup_median_wait_days");
    expect(e.label as string).toBe("published");
    expect(read(e.source)).toContain("median **72 days**, against a 31-day standard (SEHA, BMC Cancer 2021)");
    expect(() => published("v2.kpi_wasl")).toThrow();
  });

  it("shows the held-out exactly-once count while the sources conflict", () => {
    expect(exactlyOnce()).toEqual({ display: "0 / 198,810", contested: true });
  });

  it("prints the false-closure upper bound as the brief does", () => {
    expect(upperBound("v2.safety.crc_false_closures")).toBe("0.12%");
  });
});

describe("Learn-then-Test tiers", () => {
  const md = read(LTT_SOURCE);
  const table = (heading: string) => {
    const block = md.slice(md.indexOf(heading)).split("\n\n")[1];
    return new Map(
      block
        .split("\n")
        .filter((l) => /^\| \d/.test(l))
        .map((l) => {
          const c = l.split("|").map((s) => s.trim());
          return [c[1], { fc: Number(c[3]), wp: Number(c[4]), recall: Number(c[5]), review: Number(c[6]), certified: c[9] === "yes" }];
        }),
    );
  };
  const withReview = table("## with review");
  const noReview = table("## no review");

  it.each(LTT_TIERS.map((t) => [t.tau, t] as const))("τ %s matches LTT.md", (tau, t) => {
    const w = withReview.get(tau)!;
    expect(w).toBeDefined();
    expect([t.falseClosures, t.wrongPatient, t.recall, t.reviewPer100, t.certified]).toEqual([w.fc, w.wp, w.recall, w.review, w.certified]);
    expect(t.recallNoReview).toBe(noReview.get(tau)!.recall);
  });

  it("defaults to the operating tier, the floor above the 0.989 cap", () => {
    expect(LTT_TIERS.some((t) => t.tau === OPERATING_TAU)).toBe(true);
    expect(md).toContain("the operating tier must be ≥ 0.99");
  });
});

describe("landing story", () => {
  it("splits the whole scroll track into the nine beats of the road, in order, with no gap", () => {
    expect(BEATS.map((b) => b.id)).toEqual(["clinic", "city", "door", "gap", "thread", "join", "proof", "home", "end"]);
    expect(BEATS[0].from).toBe(0);
    expect(BEATS.at(-1)!.to).toBe(1);
    BEATS.slice(1).forEach((b, i) => expect(b.from).toBe(BEATS[i].to));
    expect(beatAt(0.7)).toBe("join");
  });

  it("never shows two lines of story text at once, and every line gets its moment", () => {
    for (let i = 0; i <= 10000; i++) {
      const p = i / 10000;
      expect(CUES.filter((c) => cueAt(c, p).fade > 0).length, `p = ${p}`).toBeLessThanOrEqual(1);
    }
    for (const c of CUES) {
      expect(cueOn(c.from + (c.to - c.from) * 0.55)?.id).toBe(c.id);
    }
  });

  it("maps page scroll to story progress monotonically, with exact ends and an exact inverse", () => {
    expect(storyAt(0)).toBe(0);
    expect(storyAt(1)).toBe(1);
    expect(pageAt(0)).toBe(0);
    expect(pageAt(1)).toBe(1);
    let last = 0;
    for (let i = 1; i <= 10000; i++) {
      const q = i / 10000;
      const p = storyAt(q);
      expect(p, `q = ${q}`).toBeGreaterThan(last);
      expect(pageAt(p)).toBeCloseTo(q, 9);
      last = p;
    }
  });

  it("gives every cue's readable part at least 4 wheel notches of page on a desktop viewport", () => {
    const pxPerPage = ((TRACK_VH.desktop - 100) / 100) * 900;
    for (const c of CUES) {
      const len = c.to - c.from;
      const [from, to] = [c.enter === false ? c.from : c.from + len * 0.3, c.exit === false ? c.to : c.to - len * 0.15];
      expect(cueAt(c, from + 1e-9), c.id).toEqual({ reveal: 1, fade: 1 });
      expect(cueAt(c, to - 1e-9), c.id).toEqual({ reveal: 1, fade: 1 });
      expect((pageAt(to) - pageAt(from)) * pxPerPage, c.id).toBeGreaterThanOrEqual(400);
    }
  });

  it("reveals words with scroll, so scrolling back un-reveals them", () => {
    const c = CUES.find((x) => x.id === "booked")!;
    const len = c.to - c.from;
    expect(cueAt(c, c.from + len * 0.05).reveal).toBeLessThan(cueAt(c, c.from + len * 0.2).reveal);
    expect(cueAt(c, c.from + len * 0.05)).toEqual(cueAt(c, c.from + len * 0.05));
    expect(cueAt(c, c.to + 0.001)).toEqual({ reveal: 0, fade: 0 });
    expect(cueAt(CUES[0], 0)).toEqual({ reveal: 1, fade: 1 });
    expect(cueAt(CUES.at(-1)!, 1)).toEqual({ reveal: 1, fade: 1 });
  });

  it("counts the Day counter from 0 to the published wait over the walk", () => {
    const wait = published("crc_followup_median_wait_days");
    expect(dayAt(WALK.from, wait)).toBe(0);
    expect(dayAt(WALK.to, wait)).toBe(wait);
    expect(dayAt((WALK.from + WALK.to) / 2, wait)).toBeGreaterThan(published("crc_followup_standard_days") - 20);
  });

  it("keeps the camera keys in scroll order from 0 to 1", () => {
    const ps = CAMERA_KEYS.map((k) => k.p);
    expect(ps[0]).toBe(0);
    expect(ps.at(-1)).toBe(1);
    expect([...ps].sort((a, b) => a - b)).toEqual(ps);
  });

  it("joins on a matching birth date, holds a near match for a person, and keeps different people apart", () => {
    const born = (d: string) => d.match(/Born (\d+) (\w+) (\d+)\./)!.slice(1);
    for (const [k, c] of Object.entries(CAST)) {
      const p = PEOPLE[k as keyof typeof PEOPLE];
      const [d1, m1, y1] = born(p.clinicDetail);
      const [d2, m2, y2] = born(p.hospitalDetail);
      const months = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];
      const swapped = y1 === y2 && Number(d1) === months.indexOf(m2) + 1 && Number(d2) === months.indexOf(m1) + 1;
      const expected = d1 === d2 && m1 === m2 && y1 === y2 ? "joined" : swapped ? "review" : "apart";
      expect(c.outcome, k).toBe(expected);
      expect(p.outcome).toBe(c.outcome);
    }
  });
});

describe("the identity effect's record cards", () => {
  const birthWords = { agrees: "Same birth date", differs: "Different birth date", similar: "Birth dates almost match" } as const;

  it.each(Object.entries(JOIN_BARS))("%s: the birth-date bar says what the verdict says", (outcome, bars) => {
    const said = JOIN_SAYS[outcome as keyof typeof JOIN_BARS];
    expect(said).toContain(birthWords[bars[1] as keyof typeof birthWords]);
  });

  it.each(Object.entries(JOIN_BARS))("%s: the name is only similar and the ID number is missing", (_, bars) => {
    expect(bars[0]).toBe("similar");
    expect(JOIN_SAYS.name).toContain("Similar name");
    expect(bars[2]).toBe("missing");
  });
});
