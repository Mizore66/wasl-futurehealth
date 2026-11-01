import { readFileSync } from "node:fs";
import path from "node:path";
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { BEATS, CUES } from "../components/story/script";
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

const results = JSON.parse(readFileSync(path.join(repo, "web/data/results.json"), "utf8"));
const closures = results.entries["v2.safety.crc_false_closures"];
const waitDays = results.entries["published.crc_followup_median_wait_days"].value;
const standardDays = results.entries["published.crc_followup_standard_days"].value;

/** Scrolls the 3D story to scroll progress `p` and waits for the damped camera and the copy to settle. */
async function scrollStory(page: Page, p: number) {
  await page.evaluate((p) => {
    const el = document.querySelector<HTMLElement>('[data-story="3d"]')!;
    const top = el.getBoundingClientRect().top + window.scrollY;
    window.scrollTo(0, top + p * (el.offsetHeight - window.innerHeight));
  }, p);
  await page.waitForTimeout(700);
}

const onCues = (page: Page) => page.locator("[data-cue][data-on]");

async function openStory(page: Page) {
  await page.goto("/?story=3d");
  await expect(page.locator('[data-story="3d"] canvas')).toHaveCount(1);
  await page.waitForFunction(() => typeof window.__wasl3d === "function");
}

for (const vp of WIDTHS) {
  test(`landing at ${vp.w}px: every line of the story arrives and leaves, one at a time`, async ({ page }) => {
    test.setTimeout(150_000);
    await page.setViewportSize({ width: vp.w, height: vp.h });
    await openStory(page);
    for (const c of CUES) {
      await scrollStory(page, c.from + (c.to - c.from) * 0.55);
      await expect(onCues(page), c.id).toHaveCount(1);
      await expect(onCues(page)).toHaveAttribute("data-cue", c.id);
      await page.screenshot({ path: `test-results/screens/story-${c.id}-${vp.w}.png` });
      if (c.exit !== false) {
        await scrollStory(page, Math.min(1, c.to + 0.002));
        await expect(page.locator(`[data-cue="${c.id}"]`), `${c.id} leaves`).not.toHaveAttribute("data-on");
      }
    }
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow, "no horizontal scroll").toBeLessThanOrEqual(0);
    const axe = await new AxeBuilder({ page }).analyze();
    const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
    expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
  });
}

test("landing: the Day counter runs to the published wait, with its tick at the standard", async ({ page }) => {
  await openStory(page);
  const counter = page.locator("[data-counter]");
  await expect(counter).toHaveAttribute("data-standard", String(standardDays));
  await expect(counter).toHaveAttribute("data-max", String(waitDays));
  await expect(counter).toContainText(`${standardDays} · standard`);
  await scrollStory(page, 0.05);
  await expect(counter).toHaveCSS("opacity", "0");
  await scrollStory(page, 0.24);
  const mid = Number(await counter.getAttribute("data-day"));
  expect(mid).toBeGreaterThan(0);
  expect(mid).toBeLessThan(waitDays);
  await scrollStory(page, 0.336);
  await expect(counter).toHaveAttribute("data-day", String(waitDays));
  await expect(counter.locator("[data-day-value]")).toHaveText(String(waitDays));
  await expect(counter).toContainText("published");
});

test("landing: the closures figure and the other numbers come from results.json", async ({ page }) => {
  await openStory(page);
  expect(closures.value).toBe(0);
  await scrollStory(page, 0.81);
  const cue = page.locator('[data-cue="none-wrong"]');
  await expect(cue).toHaveAttribute("data-on", "");
  await expect(cue).toContainText("None wrong");
  await expect(cue.locator('[data-figure="closures"]')).toContainText(`of ${closures.denominator.toLocaleString("en-US")} cases Wasl marked done.`);
  await expect(cue.locator('[data-honesty="simulated"]')).toHaveText("simulated");
  await expect(page.locator('[data-figure="wait"]')).toHaveText(`${waitDays} days`);
  await expect(page.locator('[data-cue="wait"] [data-honesty="published"]')).toHaveText("published study, 2021");
  await expect(page.locator('[data-figure="missed"]')).toHaveText("About 3 in 5");
  await expect(page.locator("main")).not.toContainText("2,090");
});

test("landing: the chips change the join, only in the join beat", async ({ page }) => {
  await openStory(page);
  const join = page.locator('[data-cue="join"]');
  await expect(join).toHaveCSS("visibility", "hidden");
  await scrollStory(page, 0.72);
  await expect(join).toHaveAttribute("data-on", "");
  const group = page.getByRole("group", { name: "Choose a person" });
  const verdict = page.getByTestId("verdict");
  await expect(group.getByRole("button", { name: "Fatima" })).toHaveAttribute("aria-pressed", "true");
  await expect(verdict).toHaveAttribute("data-outcome", "joined");
  await expect(verdict).toHaveText("Same birth date. Same person.");
  await expect(page.getByTestId("hospital-name")).toHaveText("F. Alzabi");

  await group.getByRole("button", { name: "Khalid" }).click();
  await expect(group.getByRole("button", { name: "Fatima" })).toHaveAttribute("aria-pressed", "false");
  await expect(verdict).toHaveAttribute("data-outcome", "apart");
  await expect(page.getByTestId("hospital-name")).toHaveText("Khaled Alkitbi");

  await group.getByRole("button", { name: "Aisha" }).click();
  await expect(verdict).toHaveAttribute("data-outcome", "review");
  await expect(verdict).toHaveText("Birth dates almost match. A person decides.");
  await page.waitForTimeout(1400);
  await page.screenshot({ path: "test-results/screens/story-join-aisha-1440.png" });

  await scrollStory(page, 0.8);
  await expect(join).toHaveCSS("visibility", "hidden");
});

test("landing: the hero and the close open the dashboard and the pipeline", async ({ page }) => {
  await openStory(page);
  for (const [id, p] of [
    ["hero", 0],
    ["close", 1],
  ] as const) {
    await scrollStory(page, p);
    const cue = page.locator(`[data-cue="${id}"]`);
    await expect(cue.getByRole("link", { name: "Open the dashboard" })).toHaveAttribute("href", "/app");
    await expect(cue.getByRole("link", { name: "See the pipeline" })).toHaveAttribute("href", "/app/pipeline");
  }
  const close = page.locator('[data-cue="close"]');
  await expect(close).toHaveAttribute("data-on", "");
  await expect(close.getByRole("link", { name: "Wasl, home" })).toBeVisible();
  await close.getByRole("link", { name: "Open the dashboard" }).click();
  await expect(page).toHaveURL(/\/app$/);
});

test("landing with reduced motion: stills with every line and the 2D name picker, no canvas", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/");
  await expect(page.locator('[data-story="stills"]')).toHaveCount(1);
  await expect(page.locator("canvas")).toHaveCount(0);
  for (const b of BEATS) await expect(page.locator(`[data-beat="${b.id}"] svg[role="img"]`)).toHaveCount(1);
  for (const c of CUES) await expect(page.locator(`[data-cue="${c.id}"]`)).toBeVisible();
  await expect(page.locator('[data-cue="hero"]').getByRole("link", { name: "See the pipeline" })).toHaveAttribute("href", "/app/pipeline");
  const group = page.getByRole("group", { name: "Choose a person" });
  await group.getByRole("button", { name: "Aisha" }).click();
  await expect(page.getByTestId("verdict")).toHaveText("A person decides");
  await group.getByRole("button", { name: "Khalid" }).click();
  await expect(page.getByTestId("verdict")).toHaveText("Two different people");
  await page.screenshot({ path: "test-results/screens/story-stills-1440.png", fullPage: true });
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
