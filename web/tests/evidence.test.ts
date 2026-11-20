import { describe, expect, it } from "vitest";
import data from "@/data/evidence.json";
import {
  asPercent,
  boundPct,
  claimCounts,
  gapRuns,
  ratePanels,
  safetyPairs,
  splitRun,
  worstOthers,
  worstRanges,
  calibrationPer1000,
  evidenceHref,
  frozenWords,
  headline,
  humanise,
  linkageRows,
  loadEvidence,
  niceMax,
  safetyOrder,
  safetyWords,
  scenarioOrder,
  tornado,
  withInterval,
} from "@/components/evidence/model";
import { Evidence } from "@/lib/api/evidence-schema";

const clone = () => JSON.parse(JSON.stringify(data)) as Record<string, unknown> & typeof data;

describe("evidence.json contract", () => {
  it("the committed file parses", () => {
    const ev = loadEvidence();
    expect(ev.generated.source).toBe("pipeline/results/final/summary.json");
    expect(ev.claims.length).toBeGreaterThan(0);
  });

  it("rejects an unknown claim status, scenario name, noise model and parameter scope", () => {
    for (const mutate of [
      (d: ReturnType<typeof clone>) => ((d.claims[0] as { status: string }).status = "proven"),
      (d: ReturnType<typeof clone>) => ((d.impact.modelled.scenarios[0] as { name: string }).name = "likely"),
      (d: ReturnType<typeof clone>) => ((d.accuracy.linkage[0] as { noise: string }).noise = "v3"),
      (d: ReturnType<typeof clone>) => ((d.impact.modelled.parameters[0] as { scope: string }).scope = "EU"),
    ]) {
      const d = clone();
      mutate(d);
      expect(Evidence.safeParse(d).success).toBe(false);
    }
  });

  it("rejects a missing section and a non-integer event count", () => {
    const d = clone();
    delete (d as Record<string, unknown>).benchmarks;
    expect(Evidence.safeParse(d).success).toBe(false);
    const e = clone();
    (e.accuracy.safety[0] as { events: number }).events = 0.5;
    expect(Evidence.safeParse(e).success).toBe(false);
  });

  it("accepts an empty benchmark list and a benchmark row with no baseline", () => {
    const d = clone();
    expect(Evidence.safeParse({ ...d, benchmarks: [] }).success).toBe(true);
    const row = { name: "FEBRL4", dataset: "recordlinkage FEBRL4", url: "", metric: "pair F1", wasl: 0.9, baseline: null, baseline_source: null, note: "" };
    expect(Evidence.safeParse({ ...d, benchmarks: [row] }).success).toBe(true);
  });
});

describe("evidence model", () => {
  it("states zero-event safety as a count and an upper bound, never a perfect score", () => {
    const w = safetyWords({ metric: "crc_false_closures", events: 0, denominator: 3161, upper_95: 0.00117 });
    expect(w).toEqual({ count: "0 of 3,161", bound: "95% upper bound 0.12%" });
    expect(safetyWords({ metric: "x", events: 2, denominator: null, upper_95: null })).toEqual({ count: "2", bound: null });
    expect(boundPct(0.00022)).toBe("0.022%");
  });

  it("puts false closures first, keeping file order otherwise", () => {
    const rows = [
      { metric: "Wrong-patient messages", events: 0, denominator: 1, upper_95: null },
      { metric: "crc_false_closures", events: 0, denominator: 1, upper_95: null },
      { metric: "Consent", events: 0, denominator: 1, upper_95: null },
      { metric: "Cervical false closures", events: 0, denominator: 1, upper_95: null },
    ];
    expect(safetyOrder(rows).map((r) => r.metric)).toEqual(["crc_false_closures", "Cervical false closures", "Wrong-patient messages", "Consent"]);
  });

  it("translates machine names and leaves human labels", () => {
    expect(humanise("crc_false_closures")).toBe("CRC false closures");
    expect(humanise("Missed-match rate")).toBe("Missed-match rate");
  });

  it("pairs v1 and v2 rows by metric and formats intervals in the value's precision", () => {
    const rows = linkageRows([
      { noise: "v2", metric: "False-match rate", value: 0.0092, low: 0.0074, high: 0.011 },
      { noise: "v1", metric: "False-match rate", value: 0.011, low: 0.0082, high: 0.0138 },
      { noise: "v2", metric: "Missed-match rate", value: 0.1014, low: 0.0953, high: 0.1075 },
    ]);
    expect(rows.map((r) => [r.metric, r.v1?.value ?? null, r.v2?.value ?? null])).toEqual([
      ["False-match rate", 0.011, 0.0092],
      ["Missed-match rate", null, 0.1014],
    ]);
    expect(withInterval(rows[0].v2!)).toMatch(/%/);
    expect(withInterval({ value: 36.9, low: 36.1, high: 37.6 }, "Review band: review items per 100 loops")).toBe("36.9 (36.1–37.6)");
  });

  it("finds the chart row and the wrongly-flagged row, or null", () => {
    const ev = loadEvidence();
    const h = headline(ev.impact.per_1000);
    expect(h.chart?.row).toMatch(/Completions/);
    // The first row whose label names the wrongly flagged patients, whatever the generator's numbers are.
    const expected = ev.impact.per_1000.find((r) => /flag|not followed/i.test(r.row));
    expect(h.flagged?.eid_only).toBe(expected?.eid_only);
    expect(h.flagged?.eid_only).toBeGreaterThan(h.flagged?.wasl ?? Infinity);
    expect(headline([])).toEqual({ chart: null, flagged: null });
  });

  it("reads the calibration figure as a percent or a share", () => {
    expect(asPercent(30.5)).toBe(30.5);
    expect(asPercent(0.305)).toBeCloseTo(30.5);
    expect(calibrationPer1000(30.5)).toBe(305);
    expect(calibrationPer1000(0.305)).toBe(305);
  });

  it("rounds axis ends up", () => {
    expect(niceMax(319)).toBe(400);
    expect(niceMax(6.05)).toBe(8);
    expect(niceMax(2.75)).toBe(4);
    expect(niceMax(0)).toBe(1);
  });

  it("orders scenarios worst, base, best and tornado bars by swing", () => {
    const s = (name: "worst" | "base" | "best") => ({ name, uplift_pp: 0, ppv: 0, extra_colonoscopies_per_1000: 0, cancers_found_per_1000: 0 });
    expect(scenarioOrder([s("best"), s("worst"), s("base")]).map((x) => x.name)).toEqual(["worst", "base", "best"]);
    const t = tornado([
      { input: "small", low: "a", high: "b", out_low: 1, out_high: 1.2 },
      { input: "big", low: "a", high: "b", out_low: 2.75, out_high: 0.75 },
    ]);
    expect(t.bars.map((b) => b.input)).toEqual(["big", "small"]);
    expect(t.bars[0]).toMatchObject({ lo: 0.75, hi: 2.75 });
    expect(t).toMatchObject({ min: 0, max: 4 });
  });

  it("links evidence to the web, the app or the repository", () => {
    expect(evidenceHref("https://example.org/x")).toEqual({ href: "https://example.org/x", external: true });
    expect(evidenceHref("/app/results")).toEqual({ href: "/app/results", external: false });
    expect(evidenceHref("docs/EVIDENCE.md")?.href).toBe("https://github.com/Mizore66/wasl-futurehealth/blob/main/docs/EVIDENCE.md");
    expect(evidenceHref("pipeline/")?.href).toBe("https://github.com/Mizore66/wasl-futurehealth/tree/main/pipeline");
    expect(evidenceHref("  ")).toBeNull();
  });

  it("words the frozen timestamp in UTC", () => {
    expect(frozenWords("2026-11-02T04:33:36.064230Z")).toBe("2 Nov 2026, 04:33 UTC");
    expect(frozenWords("not a date")).toBe("not a date");
  });
});

describe("evidence chart data", () => {
  const ev = loadEvidence();

  it("groups the per-1,000 rows by run, held-out v2 first, with the hero row in each", () => {
    expect(splitRun("Worklist size (patients to chase) (frozen v2 five-seed mean)")).toEqual({
      metric: "Worklist size (patients to chase)",
      run: "v2",
      runText: "frozen v2 five-seed mean",
    });
    const runs = gapRuns(ev.impact.per_1000);
    expect(runs.map((r) => r.run)).toEqual(["v2", "v1", "deployed"]);
    for (const r of runs) expect(r.rows.some((x) => /wrongly flagged/i.test(x.metric))).toBe(true);
    const flagged = runs[0].rows.find((x) => /wrongly flagged/i.test(x.metric))!;
    // The hero reads "207 → 31": the file's v2 values rounded.
    expect([Math.round(flagged.eid_only!), Math.round(flagged.wasl!)]).toEqual([207, 31]);
  });

  it("pairs every safety counter across noise models, false closures first", () => {
    const { closures, other } = safetyPairs(ev.accuracy.safety);
    expect(closures[0].name).toMatch(/CRC/);
    expect(closures[0].v2).toMatchObject({ events: 0, denominator: 3035, upper_95: 0.00121 });
    expect(closures[0].v1).toMatchObject({ events: 0, denominator: 3161 });
    expect(closures.length + other.length).toBe(ev.accuracy.safety.length / 2);
    expect(closures.every((p) => /false closures/i.test(p.name))).toBe(true);
  });

  it("splits linkage rates into false-match and missed-match panels with v1 and v2 per programme", () => {
    const p = ratePanels(ev.accuracy.linkage);
    expect(p.falseMatch.map((r) => r.name)).toEqual(p.missed.map((r) => r.name));
    expect(p.missed.find((r) => r.name === "Colorectal (CRC)")).toMatchObject({ v2: { value: 0.215 }, v1: { value: 0.156 } });
    expect(p.other.map((r) => r.metric)).toEqual(expect.arrayContaining([expect.stringMatching(/F1/), expect.stringMatching(/per 100 loops/)]));
  });

  it("reads worst seeds and strata as ranges, and the rest as tiles", () => {
    const r = worstRanges(ev.accuracy.worst);
    expect(r.find((x) => x.noise === "v2" && /CRC closure recall/.test(x.label))).toMatchObject({ low: 0.767, high: 0.792, lowWhat: "seed 41" });
    expect(r.find((x) => x.noise === "v2" && /age band/.test(x.label))).toMatchObject({ low: 0.766, high: 0.798, lowWhat: "<40" });
    const t = worstOthers(ev.accuracy.worst);
    expect(t.find((x) => x.noise === "v2" && /KPI gap/.test(x.name))?.lead).toBe("3.4 points");
    expect(t.find((x) => x.noise === "v2" && /Review load/.test(x.name))).toMatchObject({ lead: "35.4 items", who: "filipino" });
    expect(r.length + t.length).toBe(ev.accuracy.worst.length);
  });

  it("counts claims by status", () => {
    const counts = claimCounts(ev.claims);
    expect(counts.map((c) => c.status)).toEqual(["built", "simulated", "designed"]);
    expect(counts.reduce((a, c) => a + c.count, 0)).toBe(ev.claims.length);
  });
});
