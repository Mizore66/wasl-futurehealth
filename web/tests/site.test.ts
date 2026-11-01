import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { QUOTED, exactlyOnce, missedByIdOnly, missedOfFive, published, upperBound } from "@/components/site/figures";
import { LTT_SOURCE, LTT_TIERS, OPERATING_TAU } from "@/components/site/ltt";
import { BEATS, CAMERA_KEYS, CAST, beatAt, local } from "@/components/story/beats";
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
  it("splits the whole scroll track into the eight beats, in order, with no gap", () => {
    expect(BEATS.map((b) => b.id)).toEqual(["lost", "unknown", "wait", "join", "proof", "receipt", "who", "start"]);
    expect(BEATS[0].from).toBe(0);
    expect(BEATS.at(-1)!.to).toBe(1);
    BEATS.slice(1).forEach((b, i) => expect(b.from).toBe(BEATS[i].to));
    expect(beatAt(0.4)).toBe("join");
    expect(local(0.41, "join")).toBeCloseTo(0.5);
  });

  it("keeps the camera keys in scroll order from 0 to 1", () => {
    const ps = CAMERA_KEYS.map((k) => k.p);
    expect(ps[0]).toBe(0);
    expect(ps.at(-1)).toBe(1);
    expect([...ps].sort((a, b) => a - b)).toEqual(ps);
  });

  it("joins the two records only when the birth dates agree, in 3D and in the 2D picker alike", () => {
    for (const [k, c] of Object.entries(CAST)) {
      const p = PEOPLE[k as keyof typeof PEOPLE];
      const born = (d: string) => d.match(/Born ([^.]+)\./)![1];
      expect(c.joined).toBe(born(p.clinicDetail) === born(p.hospitalDetail));
      expect(p.joined).toBe(c.joined);
    }
  });
});
