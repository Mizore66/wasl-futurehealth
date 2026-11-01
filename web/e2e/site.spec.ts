import { readFileSync } from "node:fs";
import path from "node:path";
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { ROUTES as APP_ROUTES } from "./routes";

const ROUTES = [
  { path: "/", name: "home" },
  { path: "/how-it-works", name: "how-it-works" },
  { path: "/experts", name: "experts" },
] as const;

const WIDTHS = [
  { w: 1440, h: 900 },
  { w: 390, h: 844 },
] as const;

/** Every file the site may quote a number from. */
const CORPUS_FILES = [
  "web/data/results.json",
  "web/data/api/dashboard.json",
  "docs/BRIEF.md",
  "docs/PITCH.md",
  "docs/WRITEUP.md",
  "pipeline/results/final/SUMMARY.md",
  "pipeline/results/ltt/LTT.md",
];

/**
 * Birth years of the illustrative name-picker cast, the only numbers that are not measurements.
 * Codes glued to letters (v2.5.1, ORU^R01, e4c7553) and the HL7 sample are skipped by the scan.
 * Anything else on a page must appear in the corpus.
 */
const NOT_MEASUREMENTS = new Set(["1959", "1971", "1972", "1983"]);

const repo = path.resolve(__dirname, "../..");
const corpus = CORPUS_FILES.map((f) => readFileSync(path.join(repo, f), "utf8")).join("\n");
const corpusNumbers = new Set([...corpus.matchAll(/\d[\d,]*(?:\.\d+)?/g)].map((m) => m[0].replaceAll(",", "")));
const corpusValues = [...corpusNumbers].map(Number);

/** A shown number is sourced when the corpus has it, or (for decimals) a value that rounds to it. */
function sourced(token: string): boolean {
  const t = token.replaceAll(",", "");
  if (NOT_MEASUREMENTS.has(t) || corpusNumbers.has(t)) return true;
  const decimals = t.split(".")[1]?.length ?? 0;
  if (decimals === 0) return false;
  return corpusValues.some((v) => v.toFixed(decimals) === t || (v * 100).toFixed(decimals) === t);
}

async function visitAll(page: Page) {
  const height = await page.evaluate(() => document.body.scrollHeight);
  for (let y = 0; y < height; y += 600) await page.evaluate((top) => window.scrollTo(0, top), y);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.waitForTimeout(600);
}

for (const vp of WIDTHS) {
  for (const route of ROUTES) {
    test(`${route.path} at ${vp.w}px: renders, fits, labels its honesty, passes axe`, async ({ page }) => {
      await page.setViewportSize({ width: vp.w, height: vp.h });
      const res = await page.goto(route.path);
      expect(res?.status()).toBe(200);
      await expect(page.locator("h1")).toHaveCount(1);
      await expect(page.locator("[data-honesty]").first()).toBeVisible();
      await expect(page.locator('a[href^="mailto:"]')).toHaveCount(0);

      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      expect(overflow, "no horizontal scroll").toBeLessThanOrEqual(0);

      await visitAll(page);
      await page.screenshot({ path: `test-results/screens/site-${route.name}-${vp.w}.png`, fullPage: true });

      const axe = await new AxeBuilder({ page }).analyze();
      const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
      expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
    });
  }
}

for (const route of ROUTES) {
  test(`${route.path}: every number on the page is in a source file`, async ({ page }) => {
    await page.goto(route.path);
    const text = await page.locator("main").evaluate((main) => {
      const copy = main.cloneNode(true) as HTMLElement;
      copy.querySelectorAll("pre").forEach((pre) => pre.remove());
      document.body.append(copy);
      const t = copy.innerText;
      copy.remove();
      return t;
    });
    const tokens = [...new Set([...text.matchAll(/(?<![\w^])\d[\d,]*(?:\.\d+)?/g)].map((m) => m[0].replace(/[.,]$/, "")))];
    expect(tokens.filter((t) => !sourced(t))).toEqual([]);
  });
}

test("the header's primary action opens the dashboard from every site page", async ({ page }) => {
  for (const route of ROUTES) {
    await page.goto(route.path);
    const nav = page.getByRole("navigation", { name: "Primary" });
    await nav.getByRole("link", { name: "Open the dashboard" }).click();
    await expect(page).toHaveURL(/\/app$/);
    await expect(page.locator("h1")).toHaveCount(1);
  }
});

test("the pilot page is gone and no page links to it", async ({ page }) => {
  test.setTimeout(120_000);
  const res = await page.goto("/pilot");
  expect(res?.status()).toBe(404);
  for (const route of APP_ROUTES) {
    await page.goto(route);
    await expect(page.locator('a[href="/pilot"], a[href^="/pilot#"], a[href^="/pilot?"]'), route).toHaveCount(0);
  }
});

test("landing: the closing beat opens the dashboard and the pipeline", async ({ page }) => {
  await page.goto("/?story=stills");
  const start = page.locator('[data-beat="start"]');
  await expect(start.getByRole("link", { name: "See the pipeline" })).toHaveAttribute("href", "/app/pipeline");
  await start.getByRole("link", { name: "Open the dashboard" }).click();
  await expect(page).toHaveURL(/\/app$/);
});

const results = JSON.parse(readFileSync(path.join(repo, "web/data/results.json"), "utf8"));
const closures = results.entries["v2.safety.crc_false_closures"];

/** Each beat of the landing story and a line of its copy, in scroll order. */
const BEATS = [
  ["lost", "Nobody lost"],
  ["unknown", "Done, and nobody knew"],
  ["wait", "The typical wait for the follow-up check"],
  ["join", "One person, two spellings"],
  ["proof", "Done only when it is proven"],
  ["receipt", "It is also a word for a receipt"],
  ["who", "Who notices the difference"],
  ["start", "Try it at one clinic first"],
] as const;

for (const vp of WIDTHS) {
  test(`landing at ${vp.w}px: the 3D room scrolls through every beat with its copy`, async ({ page }) => {
    await page.setViewportSize({ width: vp.w, height: vp.h });
    await page.goto("/?story=3d");
    await expect(page.locator('[data-story="3d"] canvas')).toHaveCount(1);
    await page.waitForFunction(() => typeof window.__wasl3d === "function");
    for (const [id, line] of BEATS) {
      const section = page.locator(`[data-beat="${id}"]`);
      await section.evaluate((el: HTMLElement) => window.scrollTo(0, el.getBoundingClientRect().top + window.scrollY + el.offsetHeight * 0.45 - window.innerHeight * 0.3));
      await page.waitForTimeout(900);
      await expect(section.getByText(line, { exact: false }).first()).toBeInViewport();
      await page.screenshot({ path: `test-results/screens/story-${id}-${vp.w}.png` });
    }
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow, "no horizontal scroll").toBeLessThanOrEqual(0);
    const axe = await new AxeBuilder({ page }).analyze();
    const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
    expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
  });
}

test("landing: the live join follows the chips, and the dot wall counts results.json", async ({ page }) => {
  await page.goto("/?story=3d");
  await expect(page.locator("[data-dots]")).toHaveAttribute("data-dots", String(closures.denominator));
  await page.locator('[data-beat="join"]').scrollIntoViewIfNeeded();
  const group = page.getByRole("group", { name: "Choose a person" });
  const verdict = page.getByTestId("verdict");
  await expect(group.getByRole("button", { name: "Fatima" })).toHaveAttribute("aria-pressed", "true");
  await expect(verdict).toHaveText("Same person");
  await expect(page.getByTestId("hospital-name")).toHaveText("F. Alzabi");

  await group.getByRole("button", { name: "Khalid" }).click();
  await expect(group.getByRole("button", { name: "Fatima" })).toHaveAttribute("aria-pressed", "false");
  await expect(verdict).toHaveText("Same person");
  await expect(page.getByTestId("hospital-name")).toHaveText("Khaled Alkitbi");

  await group.getByRole("button", { name: "Aisha" }).click();
  await expect(verdict).toHaveText("Not joined. A person decides.");
  await expect(page.getByTestId("hospital-name")).toHaveText("Aysha Al Mazroui");
});

test("landing with reduced motion: stills with the same copy and the 2D name picker, no canvas", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/");
  await expect(page.locator('[data-story="stills"]')).toHaveCount(1);
  await expect(page.locator("canvas")).toHaveCount(0);
  for (const [id, line] of BEATS) {
    const section = page.locator(`[data-beat="${id}"]`);
    await expect(section.getByText(line, { exact: false }).first()).toBeVisible();
    await expect(section.locator('svg[role="img"]')).toHaveCount(1);
  }
  const group = page.getByRole("group", { name: "Choose a person" });
  await group.getByRole("button", { name: "Aisha" }).click();
  await expect(page.getByTestId("verdict")).toHaveText("Not joined yet");
  await expect(page.locator('[data-thread="broken"]')).toHaveCount(1);
  await page.screenshot({ path: "test-results/screens/story-stills-1440.png", fullPage: true });
});

test("landing: the figures carry their honesty words", async ({ page }) => {
  await page.goto("/?story=stills");
  await expect(page.locator('[data-figure="missed"]')).toHaveText("About 3 in 5");
  await expect(page.getByText("from testing on simulated patients", { exact: false }).first()).toBeVisible();
  await expect(page.locator('[data-figure="wait"]')).toHaveText(`${results.entries["published.crc_followup_median_wait_days"].display} days`);
  await expect(page.locator('[data-honesty="published"]')).toHaveText("From a published study, 2021.");
  await expect(page.locator('[data-figure="closures"]')).toHaveText(`of ${closures.denominator.toLocaleString("en-US")} cases Wasl marked done. In testing on simulated patients.`);
});

test("how it works: six moments, and the tracker's tick against Wasl's open case", async ({ page }) => {
  await page.goto("/how-it-works");
  const nav = page.getByRole("navigation", { name: "Primary" });
  await expect(nav.getByRole("link", { name: "How it works" })).toHaveAttribute("aria-current", "page");
  await expect(page.locator("#path-title + ol > li")).toHaveCount(6);
  await expect(page.locator('[data-side="tracker"]')).toContainText("Ticked done");
  await expect(page.locator('[data-side="wasl"]')).toContainText("Kept open");
  await expect(nav.getByRole("link", { name: /pilot/i })).toHaveCount(0);
  await expect(nav.getByRole("link", { name: "Open the dashboard" })).toHaveAttribute("href", "/app");
});

test("experts: KPI bars, outcome table, exactly-once figure and the τ picker read the results", async ({ page }) => {
  await page.goto("/experts");
  const kpi = page.locator("figure");
  await expect(kpi).toContainText("Simulated, n = 1,337");
  for (const v of ["12.6%", "31.9%", "32.9%"]) await expect(kpi).toContainText(v);
  await expect(page.getByTestId("outcomes").locator("tbody tr")).toHaveCount(6);
  await expect(page.locator('[data-figure="false-closures"]')).toHaveText("0 / 3,025");
  await expect(page.locator('[data-figure="exactly-once"]')).toContainText("0 / 198,810");

  const tiers = page.getByRole("group", { name: "Closure tier" });
  const panel = page.getByTestId("tau-panel");
  const cell = (label: string) => panel.locator(`[data-cell="${label}"] [data-value]`);
  await expect(tiers.getByRole("button", { name: "τ 0.99", exact: true })).toHaveAttribute("aria-pressed", "true");
  await expect(panel).toContainText("τ 0.99. The operating tier");
  await expect(cell("Recall, with review")).toHaveText("0.816");
  await expect(cell("Review items per 100 loops")).toHaveText("58.8");

  await tiers.getByRole("button", { name: "τ 0.7", exact: true }).click();
  await expect(panel).toContainText("τ 0.7. Not certified");
  await expect(cell("False closures, with review")).toHaveText("8");
  await expect(cell("Recall, with review")).toHaveText("0.829");
  await expect(cell("Recall, no review")).toHaveText("0.565");

  await tiers.getByRole("button", { name: "τ 0.999", exact: true }).click();
  await expect(cell("Review items per 100 loops")).toHaveText("83.8");
  await expect(cell("False closures, with review")).toHaveText("0");
  await expect(tiers.locator('[aria-pressed="true"]')).toHaveCount(1);
});
