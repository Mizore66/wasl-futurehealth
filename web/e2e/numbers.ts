/** Shared checks for the Programme and Results screens: every number traces to a source file, no serious axe violations. */
import AxeBuilder from "@axe-core/playwright";
import { expect, type Page } from "@playwright/test";
import dashboard from "../data/api/dashboard.json";
import results from "../data/results.json";

export const SOURCE_TEXT = JSON.stringify(results) + JSON.stringify(dashboard);

/** Numbers a source file states, as printed or as a share (32.0% or 3.4 points for 0.32 or 0.034). */
export function inSources(n: string): boolean {
  const bare = n.replace(/,/g, "").replace(/%$/, "");
  const v = Number(bare);
  const forms = new Set([n, bare, n.replace(/%$/, "")]);
  forms.add(String(v / 100)).add((v / 100).toFixed(3)).add((v / 100).toFixed(4));
  return [...forms].some((f) => SOURCE_TEXT.includes(f));
}

/** Axis labels: layout, not data. */
export const AXIS = new Set(["0%", "10%", "20%", "30%", "40%", "100%", "0.5%"]);

export async function numbersOn(page: Page): Promise<string[]> {
  const text = await page.locator("main").innerText();
  return [...new Set(text.match(/\d[\d,]*(\.\d+)?%?/g) ?? [])];
}

export async function axe(page: Page) {
  const report = await new AxeBuilder({ page }).analyze();
  const serious = report.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

/**
 * Derived figures the Programme page prints that no source file states, each computed here from the sources with the page's formula:
 *  - "still open" per programme = cases - closed - overdue (shown only when it is not negative);
 *  - the gap in points between the true and Wasl 31-day rates = |a - b| of two sourced rates, to one decimal.
 * Only the Programme checks allow these; nothing else gets through.
 */
function derived(): Set<string> {
  const out = new Set<string>();
  for (const p of Object.values(dashboard.by_programme)) {
    const open = p.cases - p.closed - p.overdue;
    if (open >= 0) out.add(open.toLocaleString("en-US"));
  }
  const kpi = dashboard.metrics.crc_kpi_31d;
  const demo = (a: { kpi_all_referrals: number }) => Math.round(a.kpi_all_referrals * 1000) / 10;
  const held = (id: string) => Math.round((results.entries as Record<string, { value: number }>)[id].value * 1000) / 10;
  const gap = (a: number, b: number) => (Math.round(Math.abs(a - b) * 10) / 10).toString();
  out.add(gap(demo(kpi.truth), demo(kpi.wasl_live)));
  out.add(gap(held("kpi31.heldout.truth"), held("kpi31.heldout.wasl")));
  return out;
}

const DERIVED = derived();

/** Numbers on the Programme page that are neither axis labels nor in a source file nor a named derivation. */
export async function unsourced(page: Page): Promise<string[]> {
  return (await numbersOn(page)).filter((n) => !AXIS.has(n) && !inSources(n) && !DERIVED.has(n.replace(/%$/, "")) && !DERIVED.has(n));
}
