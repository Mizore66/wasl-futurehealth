import type { Page } from "@playwright/test";
import dashboard from "../data/api/dashboard.json";
import results from "../data/results.json";

/* Honesty check: every number on a screen traces to results.json or the dashboard sample, or to a derivation named below. */

const SOURCE_TEXT = JSON.stringify(results) + JSON.stringify(dashboard);

/**
 * Derived figures the Programme page prints that no source file states, each computed here from dashboard.json with the page's formula:
 *  - "still open" per programme = cases - closed - overdue (shown only when it is not negative);
 *  - the 31-day gap in points = |a - b| of two sourced rates, to one decimal.
 * Nothing else is allowed through.
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

/** Numbers a source file states, as printed or as a share (32.0% or 3.4 points for 0.32 or 0.034), or a named derivation. */
export function inSources(n: string): boolean {
  const bare = n.replace(/,/g, "").replace(/%$/, "");
  if (DERIVED.has(n) || DERIVED.has(bare)) return true;
  const v = Number(bare);
  const forms = new Set([n, bare, n.replace(/%$/, "")]);
  forms.add(String(v / 100)).add((v / 100).toFixed(3)).add((v / 100).toFixed(4));
  return [...forms].some((f) => SOURCE_TEXT.includes(f));
}

/** Axis labels: layout, not data. */
export const AXIS = new Set(["0%", "10%", "20%", "30%", "40%", "100%", "0.5%"]);

/** Every distinct number the visible text of `main` shows. Tooltips and tables count once they are open. */
export async function numbersOn(page: Page): Promise<string[]> {
  const text = await page.locator("main").innerText();
  return [...new Set(text.match(/\d[\d,]*(\.\d+)?%?/g) ?? [])];
}

export async function unsourced(page: Page): Promise<string[]> {
  return (await numbersOn(page)).filter((n) => !AXIS.has(n) && !inSources(n));
}
