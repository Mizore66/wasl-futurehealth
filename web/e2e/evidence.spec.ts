import { expect, test, type Page } from "@playwright/test";
import evidence from "../data/evidence.json";
import { axe } from "./numbers";

/*
 * The evidence page for judges who never open GitHub. Every number it prints must trace to data/evidence.json, either
 * as written in the file (labels such as "2015–16" included) or as a file value rounded for display (0.00117 as 0.12%).
 * Axis ticks (data-axis) are layout and code spans are commands, so both are left out of the check.
 */
const SOURCE_TEXT = JSON.stringify(evidence);
const VALUES: number[] = [];
(function walk(x: unknown) {
  if (typeof x === "number") VALUES.push(x);
  else if (Array.isArray(x)) x.forEach(walk);
  else if (x && typeof x === "object") Object.values(x).forEach(walk);
})(evidence);

function sourced(shown: string): boolean {
  const bare = shown.replace(/,/g, "").replace(/%$/, "");
  if (SOURCE_TEXT.includes(shown) || SOURCE_TEXT.includes(bare)) return true;
  const n = Number(bare);
  const decimals = (bare.split(".")[1] ?? "").length;
  const tol = 0.5 * 10 ** -decimals + 1e-9;
  // A file value as printed, as a percent of a share, or on the per-1,000 scale of a percentage (30.5% → 305).
  // Interval ends below zero are printed with their minus sign, which the number scan drops: compare magnitudes.
  return VALUES.some((v) => [v, v * 100, v * 10].some((f) => Math.abs(Math.abs(f) - n) <= tol));
}

async function pageNumbers(page: Page): Promise<string[]> {
  const text = await page.locator("main").evaluate((main) => {
    const copy = main.cloneNode(true) as HTMLElement;
    copy.querySelectorAll("[data-axis], code, [aria-label='Run metadata'], [data-testid='verify-run']").forEach((el) => el.remove());
    document.body.append(copy);
    const t = copy.innerText;
    copy.remove();
    return t;
  });
  return [...new Set(text.match(/\d[\d,]*(\.\d+)?%?/g) ?? [])];
}

test.describe("Evidence", () => {
  test("renders six chart-first sections with takeaway titles, and every number is in evidence.json", async ({ page }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto("/app/evidence");
    await expect(page.locator("h1")).toHaveText("Evidence");
    const h2 = await page.locator("main h2").allInnerTexts();
    const scen = evidence.impact.modelled.scenarios;
    const cancers = scen.map((x) => x.cancers_found_per_1000);
    expect(h2).toEqual([
      "Wasl raises the measured rate toward the true rate",
      "No false closures in any programme; misses stay visible",
      "Ahead of the published baseline on public data",
      `If navigators act on the list: ${Math.min(...cancers)} to ${Math.max(...cancers)} more cancers found per 1,000`,
      `${evidence.claims.length} claims, each with its evidence and a rerun`,
      "Verify it yourself in two minutes",
    ]);
    await expect(page.locator("main")).not.toContainText(/from 11% to 3\d%/);
    await expect(page.locator("main")).not.toContainText("100% accurate");

    // The measured gap: the hero follows the run toggle, the dumbbell plots the run's per-1,000 rows, and the
    // SEHA calibration point is a reference with its source. All figures come from evidence.json.
    const gap = page.getByTestId("evidence-gap");
    const runRows = (run: RegExp) => evidence.impact.per_1000.filter((r) => run.test(r.row));
    const flagged = (run: RegExp) => runRows(run).find((r) => /flag|not followed/i.test(r.row))!;
    const r = (v: number | null) => Math.round(v!).toLocaleString("en-US");
    // The arrow is aria-hidden with a screen-reader "to" beside it, so innerText reads "207 →to 31".
    const hero = (run: RegExp) => new RegExp(`${r(flagged(run).eid_only)} →(?:to)? ${r(flagged(run).wasl)}`);
    await expect(page.getByTestId("flagged")).toContainText(hero(/v2/));
    await expect(page.getByTestId("flagged")).toContainText(/207 →(?:to)? 31/);
    const v2per1000 = runRows(/v2/).filter((x) => /1,000/.test(x.unit));
    await expect(page.getByTestId("gap-chart").getByTestId("gap-row")).toHaveCount(v2per1000.length);
    for (const [i, row] of v2per1000.entries()) {
      const mark = page.getByTestId("gap-chart").getByTestId("gap-row").nth(i);
      await expect(mark).toContainText(r(row.wasl));
      await expect(mark).toContainText(r(row.eid_only));
    }
    // Hover a mark: its tooltip gives the exact values, including the truth.
    const first = page.getByTestId("gap-chart").getByTestId("gap-row").first();
    await first.hover();
    const tip = page.locator(`[id="${await first.getAttribute("aria-describedby")}"]`);
    await expect(tip).toBeVisible();
    await expect(tip).toContainText(r(v2per1000[0].truth));
    await expect(tip).toContainText("SEHA 30.5%");
    await gap.getByRole("button", { name: "Tuned-on (v1)" }).click();
    await expect(page.getByTestId("flagged")).toContainText(hero(/v1/));
    await gap.getByRole("button", { name: "Held-out (v2)" }).click();
    await expect(page.getByTestId("calibration")).toContainText("30.5%");
    await expect(page.getByTestId("calibration").getByRole("link", { name: "BMC Cancer 2021" })).toHaveAttribute("href", evidence.impact.calibration.url);
    // The table alternative gives the exact numbers.
    await gap.getByRole("button", { name: "Show as table" }).click();
    const gapTable = gap.getByRole("table", { name: /Measured gap/ });
    for (const row of runRows(/v2/)) await expect(gapTable).toContainText(row.wasl!.toLocaleString("en-US"));
    await gap.getByRole("button", { name: "Show as table" }).click();

    // Accuracy: one whisker row per false-closure counter, the CRC row first, as a count with a bound (never a score).
    const safety = page.getByTestId("safety");
    const closureRows = evidence.accuracy.safety.filter((x) => x.metric.startsWith("[v2]") && /false closures/i.test(x.metric));
    await expect(safety.getByTestId("safety-row").first()).toContainText("Colorectal (CRC)");
    await expect(safety.getByTestId("safety-row").first()).toContainText("0 of 3,035");
    await expect(safety.getByTestId("safety-row").first()).toContainText("≤ 0.12%");
    expect(await safety.getByTestId("safety-row").count()).toBeGreaterThanOrEqual(closureRows.length);
    const accuracy = page.getByTestId("evidence-accuracy");
    await accuracy.getByRole("button", { name: "Show as table" }).first().click();
    await expect(accuracy.getByRole("table", { name: "Safety events and 95% upper bounds" })).toContainText("0 of 3,161");
    // Linkage rates: two panels (false match, missed match), each programme with v2 and v1 intervals.
    await expect(page.getByTestId("rates-missed").getByTestId("rate-row").first()).toContainText("Colorectal (CRC)");
    await expect(page.getByTestId("rates-missed")).toContainText("21.5%");
    await accuracy.getByRole("button", { name: "Show as table" }).nth(1).click();
    await expect(accuracy.getByRole("table", { name: "Linkage error rates by noise model" })).toContainText(/\d\.\d+%\s*\(\d/);
    // Worst cases as a strip: the worst v2 seed labelled.
    await expect(page.getByTestId("worst")).toContainText(/seed \d+/);
    await expect(page.getByTestId("worst-tiles")).toContainText("3.4 points");
    // Definitions behind a disclosure.
    await expect(page.getByTestId("terms").locator("dl")).toBeHidden();
    await page.getByTestId("terms").locator("summary").click();
    await expect(page.getByTestId("terms")).toContainText(evidence.accuracy.definitions[0].term);

    // Benchmarks: paired bars for rows with a baseline, bullets for the encoder; sources linked.
    const paired = evidence.benchmarks.filter((b) => b.baseline !== null);
    await expect(page.getByTestId("bench-pairs").getByTestId("bench-row")).toHaveCount(paired.length);
    await expect(page.getByTestId("bench-pairs")).toContainText("97.9%");
    await expect(page.getByTestId("bench-bullets").getByTestId("bench-row")).toHaveCount(evidence.benchmarks.length - paired.length);
    await expect(page.getByTestId("bench-sources").getByRole("link", { name: "FEBRL4" })).toHaveAttribute("href", /recordlinkage/);

    // Modelled: labelled modelled and conditional, a range per measure and the tornado with its table.
    const modelled = page.getByTestId("evidence-modelled");
    await expect(modelled.locator('[data-honesty="illustrative"]')).toHaveText(/Modelled, not measured/);
    await expect(page.getByTestId("modelled-label")).toContainText("Conditional on navigation");
    await expect(page.getByTestId("scenarios").getByTestId("scenario-row").first()).toContainText(String(scen.find((x) => x.name === "best")!.cancers_found_per_1000));
    await modelled.getByRole("button", { name: "Show as table" }).nth(1).click();
    await expect(modelled.getByRole("table", { name: /Sensitivity/ })).toContainText("Navigation uplift");

    // Ledger: status counts as chips, then the full ledger behind a disclosure.
    for (const s of ["built", "simulated", "designed"] as const) {
      const n = evidence.claims.filter((c) => c.status === s).length;
      if (n) await expect(page.getByTestId("claim-counts").locator(`[data-status="${s}"]`)).toContainText(String(n));
    }
    const ledger = page.getByTestId("claims-table");
    await expect(ledger).toBeHidden();
    await page.getByTestId("claims").locator("summary").click();
    await expect(ledger.locator("tbody tr")).toHaveCount(evidence.claims.length);
    for (const [i, c] of evidence.claims.entries()) await expect(ledger.locator("tbody tr").nth(i).locator(`[data-honesty="${c.status}"]`)).toBeVisible();
    await expect(ledger.getByRole("link", { name: /^docs\// }).first()).toHaveAttribute("href", /^https:\/\/github\.com\/Mizore66\/wasl-futurehealth\/blob\/main\/docs\//);
    for (const a of await ledger.getByRole("link", { name: /^\/app\// }).all()) expect(await a.getAttribute("href")).toMatch(/^\/app\//);
    await expect(ledger.locator("code").first()).toBeVisible();

    // Verify: two command lines and the repository links.
    const verify = page.getByTestId("evidence-verify");
    await expect(page.getByTestId("verify-cmd")).toHaveText("git clone https://github.com/Mizore66/wasl-futurehealth && cd wasl-futurehealth && make verify");
    await expect(verify.locator("code")).toHaveCount(2);
    await expect(verify.locator("code").last()).toHaveText("make verify SEED=<n>");
    await expect(verify.getByRole("link", { name: "GitHub repository" })).toHaveAttribute("href", "https://github.com/Mizore66/wasl-futurehealth");
    await expect(verify.getByRole("link", { name: "docs/EVIDENCE.md" })).toHaveAttribute("href", /docs\/EVIDENCE\.md$/);

    // Every number on the page (charts and the tables opened above) traces to evidence.json.
    expect((await pageNumbers(page)).filter((n) => !sourced(n))).toEqual([]);
    await page.screenshot({ path: "test-results/screens/evidence-1440.png", fullPage: true });
    await axe(page);
  });

  test("every chart has a working table view whose numbers are all sourced", async ({ page }) => {
    await page.goto("/app/evidence");
    const toggles = page.locator("main").getByRole("button", { name: "Show as table" });
    const n = await toggles.count();
    expect(n).toBeGreaterThanOrEqual(8);
    for (let i = 0; i < n; i++) await toggles.nth(i).click();
    await expect(page.locator("main figure table")).toHaveCount(n);
    expect((await pageNumbers(page)).filter((x) => !sourced(x))).toEqual([]);
    await axe(page);
  });

  test("the sidebar lists Evidence first in its group and marks it current", async ({ page }) => {
    await page.goto("/app/evidence");
    const nav = page.locator("aside#app-sidebar").getByRole("navigation", { name: "Main" });
    const link = nav.getByRole("link", { name: "Evidence", exact: true });
    await expect(link).toHaveAttribute("href", "/app/evidence");
    await expect(link).toHaveAttribute("aria-current", "page");
    const names = (await nav.getByRole("link").allInnerTexts()).map((t) => t.replace(/\s*\d+$/, "").trim());
    expect(names.slice(names.indexOf("Evidence"), names.indexOf("Evidence") + 3)).toEqual(["Evidence", "Pipeline", "Results"]);
    await nav.getByRole("link", { name: "Results" }).click();
    await expect(page).toHaveURL(/\/app\/results$/);
  });
});

for (const vp of [
  { w: 390, h: 844 },
  { w: 320, h: 700 },
]) {
  test(`fits ${vp.w}px with the ledger as a list and passes axe`, async ({ page }) => {
    await page.setViewportSize({ width: vp.w, height: vp.h });
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto("/app/evidence");
    await expect(page.locator("h1")).toHaveText("Evidence");
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(vp.w);
    await expect(page.getByTestId("claims-table")).toBeHidden();
    await page.getByTestId("claims").locator("summary").click();
    await expect(page.getByTestId("claims-table")).toBeHidden();
    await expect(page.getByTestId("claims-list").locator("li")).toHaveCount(evidence.claims.length);
    for (const b of await page.locator("main").getByRole("button", { name: "Show as table" }).all()) expect((await b.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    for (const el of await page.locator("main summary").all()) expect((await el.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    for (const b of await page.locator("main").getByRole("group", { name: "Evaluation run" }).getByRole("button").all()) expect((await b.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await page.screenshot({ path: `test-results/screens/evidence-${vp.w}.png`, fullPage: true });
    await axe(page);
  });
}

test("the breadcrumb names the Evidence page", async ({ page }) => {
  await page.goto("/app/evidence");
  await expect(page.getByRole("navigation", { name: "Breadcrumb" }).locator('[aria-current="page"]')).toHaveText("Evidence");
});
