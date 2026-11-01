import { result, results } from "@/lib/results";

/**
 * Numbers the site needs that `results.json` does not carry yet, quoted from the repo file named
 * beside each. `tests/site.test.ts` checks every quote still appears in its file.
 */
export const QUOTED = {
  baselineRate: { value: "30.5%", file: "docs/BRIEF.md", quote: "only **30.5%** of FIT+ patients" },
  pilotStreak: { value: "300", file: "docs/BRIEF.md", quote: "until 300 in a row are correct" },
} as const;

/** A figure from a published study, recorded in results.json under `published.<name>`. */
export function published(name: string): number {
  const e = result(`published.${name}`);
  if ((e.label as string) !== "published") throw new Error(`results.json entry published.${name} is not labelled published`);
  return e.value;
}

/** Share of real completions that ID-only tracking misses, from the demo-run 31-day KPI. */
export function missedByIdOnly(): number {
  const eid = result("kpi31.demo.eid_only").value;
  const truth = result("kpi31.demo.truth").value;
  return 1 - eid / truth;
}

/** The landing's plain-words version of `missedByIdOnly`: how many in five, rounded. */
export function missedOfFive(): number {
  return Math.round(missedByIdOnly() * 5);
}

/**
 * The exactly-once denominator. `results.json` still lists it under `conflicts`; until the backend
 * owner picks one, the site shows the frozen held-out evaluation's figure.
 */
export function exactlyOnce(): { display: string; contested: boolean } {
  const contested = results.conflicts.some((c) => c.id === "exactly_once_outputs");
  return { display: result("v2.safety.duplicate_or_lost_outputs").display, contested };
}

/** "0.1760%" → "0.18%": the 95% upper bound as the expert page prints it. */
export function upperBound(id: string): string {
  const raw = result(id).upper95;
  if (!raw) throw new Error(`results.json entry "${id}" has no upper95`);
  return `${Number.parseFloat(raw).toFixed(2)}%`;
}

export function fixed(id: string, digits: number): string {
  return result(id).value.toFixed(digits);
}
