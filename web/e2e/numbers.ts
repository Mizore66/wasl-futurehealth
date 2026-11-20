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
export const AXIS = new Set(["0%", "10%", "20%", "25%", "30%", "40%", "50%", "75%", "100%", "0.5%"]);

export async function numbersOn(page: Page): Promise<string[]> {
  // Axis ticks (data-axis) are layout, not data: left out like the AXIS labels below.
  const text = await page.locator("main").evaluate((main) => {
    const copy = main.cloneNode(true) as HTMLElement;
    copy.querySelectorAll("[data-axis]").forEach((el) => el.remove());
    document.body.append(copy);
    const t = copy.innerText;
    copy.remove();
    return t;
  });
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
 *  - the gaps in points between the true 31-day rate and the Wasl rate, the Emirates ID rate and the target = |a - b|, to one decimal;
 *  - the share of on-time colonoscopies each method sees = its rate / the truth's rate, as a whole percent, per tab;
 *  - on time by programme (metrics.on_time_by_programme): each window's share = on_time / eligible as a percent to one
 *    decimal, and per programme the summed eligible and on_time over its windows and their share, the same way.
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
  const target = Number(/acceptable >(\d+)%/.exec(kpi.definition)![1]);
  for (const [truth, wasl, eid] of [
    [demo(kpi.truth), demo(kpi.wasl_live), demo(kpi.eid_only)],
    [held("kpi31.heldout.truth"), held("kpi31.heldout.wasl"), held("kpi31.heldout.eid_only")],
  ]) {
    out.add(gap(truth, wasl));
    out.add(gap(truth, eid));
    out.add(gap(target, truth));
  }
  // What each method sees: its 31-day rate over the truth's, as a whole percent (seenShare), per source tab.
  const seen = (rate: number, truth: number) => String(Math.round((100 * rate) / truth));
  for (const a of [kpi.eid_only, kpi.wasl_live]) out.add(seen(demo(a), demo(kpi.truth)));
  for (const a of ["eid_only", "wasl"]) out.add(seen(held(`kpi31.heldout.${a}`), held("kpi31.heldout.truth")));
  const share = (on: number, of: number) => (Math.round((on / of) * 1000) / 10).toFixed(1);
  const sums = new Map<string, { eligible: number; onTime: number }>();
  for (const r of dashboard.metrics.on_time_by_programme ?? []) {
    if (r.eligible === 0) continue;
    out.add(share(r.on_time, r.eligible));
    const t = sums.get(r.programme) ?? { eligible: 0, onTime: 0 };
    sums.set(r.programme, { eligible: t.eligible + r.eligible, onTime: t.onTime + r.on_time });
  }
  for (const t of sums.values()) {
    out.add(t.eligible.toLocaleString("en-US"));
    out.add(t.onTime.toLocaleString("en-US"));
    out.add(share(t.onTime, t.eligible));
  }
  return out;
}

const DERIVED = derived();
for (const v of chartDerived()) DERIVED.add(v);

/**
 * Figures the chart-first Programme charts print, each computed here from the dashboard with the page's formula:
 *  - each programme's share of its cases finished, overdue and still open, as a whole percent (the strip's blocks);
 *  - each region's slots minus open cases needing one ("29 short", "86 spare");
 *  - each provider's evidenced closures over its cases, as a whole percent.
 */
function chartDerived(): string[] {
  const out: string[] = [];
  const whole = (a: number, b: number) => String(Math.round((100 * a) / b));
  for (const p of Object.values(dashboard.by_programme)) {
    const open = p.cases - p.closed - p.overdue;
    for (const v of [p.closed, p.overdue, open]) if (v >= 0 && p.cases > 0) out.push(whole(v, p.cases));
  }
  const regions = (dashboard.capacity as { condition?: { by_region?: Record<string, { slots: number; cases: number }> } }).condition?.by_region ?? {};
  for (const r of Object.values(regions)) {
    out.push(Math.abs(r.slots - r.cases).toLocaleString("en-US"));
  }
  for (const p of dashboard.providers as { loops: unknown; evidenced_closed: unknown }[]) {
    if (typeof p.loops === "number" && typeof p.evidenced_closed === "number" && p.loops > 0) out.push(whole(p.evidenced_closed, p.loops));
  }
  return out;
}

/** Numbers on the Programme page that are neither axis labels nor in a source file nor a named derivation. */
export async function unsourced(page: Page): Promise<string[]> {
  return (await numbersOn(page)).filter((n) => !AXIS.has(n) && !inSources(n) && !DERIVED.has(n.replace(/%$/, "")) && !DERIVED.has(n));
}
