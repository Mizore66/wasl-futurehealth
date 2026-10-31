import { describe, expect, it } from "vitest";
import { people } from "@/lib/people";
import { result, results } from "@/lib/results";

describe("results.json pins the honest numbers", () => {
  it.each([
    ["v2.safety.crc_false_closures", "0 / 2,090"],
    ["showcase.decisions", "25,620"],
    ["showcase.chains_verified", "1,741 / 1,741"],
    ["kpi31.demo.eid_only", "12.4%"],
    ["kpi31.demo.wasl", "32.0%"],
    ["kpi31.demo.truth", "33.0%"],
    ["kpi31.heldout.eid_only", "13.0%"],
    ["kpi31.heldout.wasl", "29.4%"],
    ["kpi31.heldout.truth", "32.7%"],
    ["double_review.false_closures_denominator", "8,422"],
    ["single_review.false_closures", "21"],
    ["ltt.calibration_loops", "12,600"],
  ])("%s = %s", (id, display) => {
    expect(result(id).display).toBe(display);
  });

  it("names a source file and a label for every number", () => {
    for (const e of Object.values(results.entries)) {
      expect(e.source).toMatch(/^(docs|pipeline)\//);
      expect(e.label).toBe("simulated");
    }
  });

  it("flags the exactly-once conflict instead of choosing", () => {
    const c = results.conflicts.find((x) => x.id === "exactly_once_outputs")!;
    expect(c.values.map((v) => v.value)).toEqual(expect.arrayContaining([201285, 136669]));
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
    expect(people.people["ahs-mushrif|AHS-MRN001050"].display_name).toBe("Fatima Al Zaabi");
  });
});
