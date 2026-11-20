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
    copy.querySelectorAll("[data-axis], code, [aria-label='Run metadata']").forEach((el) => el.remove());
    document.body.append(copy);
    const t = copy.innerText;
    copy.remove();
    return t;
  });
  return [...new Set(text.match(/\d[\d,]*(\.\d+)?%?/g) ?? [])];
}

test.describe("Evidence", () => {
  test("renders the six sections in order with honest wording, and every number is in evidence.json", async ({ page }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto("/app/evidence");
    await expect(page.locator("h1")).toHaveText("Evidence");
    const h2 = await page.locator("main h2").allInnerTexts();
    expect(h2).toEqual([
      "The measured gap",
      "How accurate it is",
      "On data the team did not generate",
      "Modelled benefit",
      "Claim ledger",
      "Verify it yourself in two minutes",
    ]);

    // Headline: the wrongly-flagged figure and the bar chart with the SEHA calibration point.
    // The figures come from evidence.json (the generator's output), not from numbers pinned here.
    const flaggedRow = evidence.impact.per_1000.find((r) => /flag|not followed/i.test(r.row))!;
    const chartRow = evidence.impact.per_1000.find((r) => r.eid_only != null && r.wasl != null && r.truth != null)!;
    await expect(page.getByTestId("flagged")).toContainText(String(flaggedRow.eid_only));
    await expect(page.getByTestId("flagged")).toContainText(String(flaggedRow.wasl));
    const esc = (x: unknown) => String(x).replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    await expect(
      page.getByRole("img", { name: new RegExp(`Emirates ID only ${esc(chartRow.eid_only)}, Wasl ${esc(chartRow.wasl)}, Simulated truth ${esc(chartRow.truth)}\\. SEHA 30\\.5%`) }),
    ).toBeVisible();
    await expect(page.getByTestId("calibration")).toContainText("30.5%");
    await expect(page.getByTestId("calibration").getByRole("link", { name: "BMC Cancer 2021" })).toHaveAttribute("href", evidence.impact.calibration.url);
    await expect(page.locator("main")).toContainText("raises the measured rate toward the true rate");
    await expect(page.locator("main")).not.toContainText(/from 11% to 3\d%/);

    // The chart's table alternative.
    const gap = page.getByTestId("evidence-gap");
    await gap.getByRole("button", { name: "Show as table" }).click();
    await expect(gap.getByRole("table", { name: /Completions seen within 31 days/ })).toContainText(String(chartRow.wasl));

    // Accuracy: false closures first, as a count with a bound, never a perfect score.
    const safety = page.getByTestId("safety");
    await expect(safety.locator("tbody tr").first()).toContainText(/CRC\) false closures|CRC false closures/);
    await expect(safety.locator("tbody tr").first()).toContainText("0 of 3,161");
    await expect(safety.locator("tbody tr").first()).toContainText("0.12%");
    await expect(page.locator("main")).not.toContainText("100% accurate");
    await expect(page.getByTestId("linkage")).toContainText(/\d\.\d+%\s*\(\d/);
    await expect(page.getByTestId("worst")).toContainText(/seed \d+/);

    // Benchmarks: the out-of-generator rows when the generator has them, else a plain "not run" note.
    if (evidence.benchmarks.length) await expect(page.locator("main")).toContainText("FEBRL4");
    else await expect(page.getByTestId("benchmarks-none")).toContainText("Not run yet");

    // Modelled: labelled modelled and conditional, with the tornado and its table.
    const modelled = page.getByTestId("evidence-modelled");
    await expect(modelled.locator('[data-honesty="illustrative"]')).toHaveText(/Modelled, not measured/);
    await expect(page.getByTestId("modelled-label")).toContainText("conditional on navigation");
    await expect(page.getByTestId("scenarios")).toContainText(String(evidence.impact.modelled.scenarios.find((x) => x.name === "best")!.cancers_found_per_1000));
    await modelled.getByRole("button", { name: "Show as table" }).click();
    await expect(modelled.getByRole("table", { name: /Sensitivity/ })).toContainText("Navigation uplift");

    // Ledger: one row per claim with a status chip, an evidence link and the reproduce step.
    const ledger = page.getByTestId("claims-table");
    await expect(ledger.locator("tbody tr")).toHaveCount(evidence.claims.length);
    for (const [i, c] of evidence.claims.entries()) await expect(ledger.locator("tbody tr").nth(i).locator(`[data-honesty="${c.status}"]`)).toBeVisible();
    // Repository paths link to GitHub; app paths stay in the app.
    await expect(ledger.getByRole("link", { name: /^docs\// }).first()).toHaveAttribute("href", /^https:\/\/github\.com\/Mizore66\/wasl-futurehealth\/blob\/main\/docs\//);
    for (const a of await ledger.getByRole("link", { name: /^\/app\// }).all()) expect(await a.getAttribute("href")).toMatch(/^\/app\//);
    await expect(ledger.locator("code").first()).toBeVisible();

    // Verify: the clone-and-verify command, the seed form and the repository links.
    const verify = page.getByTestId("evidence-verify");
    await expect(page.getByTestId("verify-cmd")).toHaveText("git clone https://github.com/Mizore66/wasl-futurehealth && cd wasl-futurehealth && make verify");
    await expect(verify.locator("code").last()).toHaveText("make verify SEED=<n>");
    await expect(verify.getByRole("link", { name: "GitHub repository" })).toHaveAttribute("href", "https://github.com/Mizore66/wasl-futurehealth");
    await expect(verify.getByRole("link", { name: "docs/EVIDENCE.md" })).toHaveAttribute("href", /docs\/EVIDENCE\.md$/);

    expect((await pageNumbers(page)).filter((n) => !sourced(n))).toEqual([]);
    await page.screenshot({ path: "test-results/screens/evidence-1440.png", fullPage: true });
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
    await expect(page.getByTestId("claims-list").locator("li")).toHaveCount(evidence.claims.length);
    for (const b of await page.locator("main").getByRole("button", { name: "Show as table" }).all()) expect((await b.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    expect((await page.getByTestId("evidence-modelled").locator("summary").boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await page.screenshot({ path: `test-results/screens/evidence-${vp.w}.png`, fullPage: true });
    await axe(page);
  });
}

test("the breadcrumb names the Evidence page", async ({ page }) => {
  await page.goto("/app/evidence");
  await expect(page.getByRole("navigation", { name: "Breadcrumb" }).locator('[aria-current="page"]')).toHaveText("Evidence");
});
