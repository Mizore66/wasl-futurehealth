import { describe, expect, it } from "vitest";
import { SKIN, signature, type PersonSeed } from "@/components/avatar/traits";
import { LAB_SALIENT as SALIENT, SYSTEMS, type SystemId } from "@/app/lab/systems";

const SEXES = ["female", "male", undefined] as const;
const people: PersonSeed[] = Array.from({ length: 200 }, (_, i) => ({ key: `person-${String(i).padStart(4, "0")}`, sex: SEXES[i % 3] }));

const NEAR_NAMES: [string, string][] = [
  ["Fatima Al Zaabi", "Fatma Al Zaabi"],
  ["Fatima Al Zaabi", "Fatima Alzaabi"],
  ["Mohammed Al Mansoori", "Mohamed Al Mansoori"],
  ["Aisha Al Ketbi", "Ayesha Al Ketbi"],
  ["Khalid Al Hammadi", "Khaled Al Hammadi"],
];

const ids = Object.keys(SYSTEMS) as SystemId[];

function differing(a: object, b: object): number {
  const bv = b as Record<string, unknown>;
  return Object.entries(a).filter(([k, v]) => bv[k] !== v).length;
}

describe.each(ids)("avatar system %s", (id) => {
  const features = SYSTEMS[id].features;

  it("gives 200 people 200 distinct feature signatures", () => {
    const sigs = people.map((p) => signature(features(p)));
    expect(new Set(sigs).size).toBe(200);
  });

  it("is deterministic for the same person key", () => {
    expect(signature(features({ key: "Fatima Al Zaabi", sex: "female" }))).toBe(
      signature(features({ key: "Fatima Al Zaabi", sex: "female" })),
    );
  });

  it.each(NEAR_NAMES)("separates %s from %s in at least 3 features", (a, b) => {
    for (const sex of SEXES) {
      expect(differing(features({ key: a, sex }), features({ key: b, sex }))).toBeGreaterThanOrEqual(3);
    }
  });

  it("covers every skin tone, each at least 8% of 200 people", () => {
    const counts = new Array(SKIN.length).fill(0);
    for (const p of people) counts[features(p).skin]++;
    for (const c of counts) expect(c).toBeGreaterThanOrEqual(16);
  });

  it("reports salient (24px) signature collisions", () => {
    const seen = new Map<string, string>();
    const clashes: string[] = [];
    for (const p of people) {
      const sig = signature(features(p), SALIENT[id]);
      const prior = seen.get(sig);
      if (prior) clashes.push(`${prior}~${p.key}`);
      else seen.set(sig, p.key);
    }
    console.log(`${id}: ${clashes.length} salient collisions in 200`, clashes.slice(0, 3));
    expect(clashes.length).toBeLessThanOrEqual(8);
  });
});
