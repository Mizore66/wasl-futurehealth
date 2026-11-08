import { describe, expect, it } from "vitest";
import { people } from "@/lib/people";
import { result, results } from "@/lib/results";

describe("results.json pins the honest numbers", () => {
  it.each([
    ["v2.safety.crc_false_closures", "0 / 3,035"],
    ["showcase.decisions", "31,677"],
    ["showcase.chains_verified", "3,163 / 3,163"],
    ["kpi31.demo.eid_only", "11.3%"],
    ["kpi31.demo.wasl", "30.7%"],
    ["kpi31.demo.truth", "31.9%"],
    ["kpi31.heldout.eid_only", "12.2%"],
    ["kpi31.heldout.wasl", "29.8%"],
    ["kpi31.heldout.truth", "32.9%"],
    ["double_review.false_closures_denominator", "8,422"],
    ["single_review.false_closures", "21"],
    ["ltt.calibration_loops", "12,600"],
  ])("%s = %s", (id, display) => {
    expect(result(id).display).toBe(display);
  });

  it("names a source file and a label for every number", () => {
    for (const e of Object.values(results.entries)) {
      expect(e.source).toMatch(/^(docs|pipeline|booking)\//);
      expect(e.label as string).toBe(e.id.startsWith("published.") ? "published" : e.id.startsWith("labelling.") ? "built" : "simulated");
    }
  });

  it("reads the labelling agreement from docs/labelling/agreement.json", () => {
    for (const f of ["caecum", "prep", "polyps", "histology", "state"]) {
      expect(result(`labelling.kappa.${f}`).display).toBe("1.000");
      expect(result(`labelling.kappa.${f}`).source).toBe("docs/labelling/agreement.json");
    }
    expect(result("labelling.reports").value).toBe(43);
  });

  it("flags the exactly-once conflict instead of choosing", () => {
    const c = results.conflicts.find((x) => x.id === "exactly_once_outputs")!;
    // Three sources, three totals: docs/PITCH.md, then the v2 and v1 tables of pipeline/results/final/SUMMARY.md.
    expect(c.values.map((v) => [v.result, v.value])).toEqual([
      ["exactly_once.pitch_outputs", 201285],
      ["v2.safety.duplicate_or_lost_outputs", 217009],
      ["v1.safety.duplicate_or_lost_outputs", 220789],
    ]);
    expect(new Set(c.values.map((v) => v.value)).size).toBe(3);
    expect(c.note).toContain("No two agree");
  });

  it("throws on an unknown id rather than rendering a blank", () => {
    expect(() => result("made.up")).toThrow();
  });
});

describe("people.json", () => {
  it("names both registrations of each of the three fixtures from the canonical cast", () => {
    const byFixture = Object.groupBy(Object.values(people.people), (p) => p.fixture);
    expect(Object.keys(byFixture).sort()).toEqual(["evidence_closure", "false_closure_trap", "review_hold"]);
    for (const list of Object.values(byFixture)) expect(list).toHaveLength(2);
    expect(Object.values(people.people).every((p) => p.source === "canonical-cast")).toBe(true);
    expect(people.people["ahs-mushrif|AHS-MRN000985"].display_name).toBe("Fatima Al Zaabi");
  });
});
