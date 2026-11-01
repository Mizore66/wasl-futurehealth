import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { QUOTED, exactlyOnce, missedByIdOnly, missedInFive, upperBound } from "@/components/site/figures";
import { LTT_SOURCE, LTT_TIERS, OPERATING_TAU } from "@/components/site/ltt";

const repo = path.resolve(import.meta.dirname, "../..");
const read = (file: string) => readFileSync(path.join(repo, file), "utf8").replace(/\r\n/g, "\n");

describe("site figures", () => {
  it("rounds the ID-only miss rate (62%) to 3 in 5", () => {
    expect(Math.round(missedByIdOnly() * 100)).toBe(62);
    expect(missedInFive()).toBe("3 in 5");
  });

  it.each(Object.entries(QUOTED))("%s is still quoted in its source file", (_, q) => {
    expect(read(q.file)).toContain(q.quote);
    expect(q.quote).toContain(q.value);
  });

  it("shows the held-out exactly-once count while the sources conflict", () => {
    expect(exactlyOnce()).toEqual({ display: "0 / 136,669", contested: true });
  });

  it("prints the false-closure upper bound as the brief does", () => {
    expect(upperBound("v2.safety.crc_false_closures")).toBe("0.18%");
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
