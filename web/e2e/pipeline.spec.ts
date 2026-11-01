import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import replay from "../data/replay.json";

const LINE = "Replaying the recorded showcase run · synthetic patients · real report texts";
const SHOTS = "test-results/screens/pipeline";

test.describe.configure({ timeout: 120_000 });

const asNumber = (s: string) => Number(s.replace(/[^\d]/g, ""));

/** Call with the replay paused: axe needs real timers. */
async function expectNoSeriousAxe(page: Page) {
  await page.clock.resume();
  // Endless status pulses never finish; wait only for one-shot fades and slides.
  await page.waitForFunction(() => document.getAnimations().every((a) => a.playState !== "running" || a.effect?.getTiming().iterations === Infinity));
  const axe = await new AxeBuilder({ page }).analyze();
  const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

async function totals(page: Page) {
  const read = async (id: string) => Number(await page.getByTestId(id).getAttribute("data-value"));
  return { decisions: await read("total-decisions"), chains: await read("total-chains") };
}

/** Timers fire only on clock.runFor, so a slow machine cannot drift the replay between assertions. */
async function freezeClock(page: Page) {
  await page.clock.install({ time: new Date("2026-10-31T09:00:00Z") });
  await page.clock.pauseAt(new Date("2026-10-31T09:00:01Z"));
}

async function position(page: Page) {
  return asNumber((await page.getByTestId("replay-position").innerText()).split(" of ")[0]);
}

test("pipeline: pause, step, play, switch off a server and tamper, totals within the run", async ({ page }) => {
  await freezeClock(page);
  await page.goto("/app/pipeline");
  await expect(page.locator("h1")).toHaveText("Pipeline");
  await expect(page.getByText(LINE)).toBeVisible();
  await expect(page.locator("[data-honesty]").first()).toBeVisible();
  await expect(page.locator("[data-stage]")).toHaveCount(8);

  await page.clock.runFor(500);
  await page.screenshot({ path: `${SHOTS}-rest.png`, fullPage: true });
  await page.getByRole("button", { name: "Pause" }).click();
  await expect(page.getByTestId("replay-position")).toContainText("paused");

  const before = await position(page);
  await page.clock.runFor(3000);
  expect(await position(page), "paused: no ticks").toBe(before);

  for (let i = 0; i < 12; i++) await page.getByRole("button", { name: "Step", exact: true }).click();
  await expect.poll(() => position(page)).toBe(before + 12);
  const stepped = await totals(page);
  expect(stepped.decisions).toBeLessThanOrEqual(replay.totals.outputs);
  expect(stepped.chains).toBeLessThanOrEqual(replay.totals.chainsVerified);

  await page.getByRole("button", { name: "Play" }).click();
  await page.clock.runFor(2500);
  await expect.poll(() => position(page)).toBe(before + 14);

  await page.getByRole("button", { name: "Switch off a server" }).click();
  await expect(page.getByRole("img", { name: /server 2 off/ })).toBeVisible();
  await expect(page.getByTestId("server-result")).toContainText("still 0 lost, 0 duplicated");
  const moving = await position(page);
  await page.clock.runFor(2000);
  await expect.poll(() => position(page)).toBe(moving + 2);

  await page.getByRole("button", { name: "Tamper with a step" }).click();
  await expect(page.getByRole("status").filter({ hasText: "Changed record detected at step 2" })).toBeVisible();
  await expect(page.locator('[aria-label="Sealed chain"] [data-broken]')).toHaveCount(2);
  await expect(page.getByText(/no longer matches a81e5d7bd109/).first()).toBeVisible();

  await page.getByRole("button", { name: "Pause" }).click();
  await page.clock.runFor(1000);
  await page.screenshot({ path: `${SHOTS}-mid-run.png`, fullPage: true });
  await page.getByRole("button", { name: "Play" }).click();

  for (let i = 0; i < 5; i++) {
    const t = await totals(page);
    expect(t.decisions).toBeLessThanOrEqual(replay.totals.outputs);
    expect(t.chains).toBeLessThanOrEqual(replay.totals.chainsVerified);
    await page.clock.runFor(3000);
  }

  await page.getByRole("button", { name: "Undo the change" }).click();
  await expect(page.getByText("Records intact")).toBeVisible();
  await page.getByRole("button", { name: "Switch it back on" }).click();
  await expect(page.getByRole("img", { name: "Three servers, all on." })).toBeVisible();
  await page.getByRole("button", { name: "Pause" }).click();
  await page.clock.runFor(1000);
  await expectNoSeriousAxe(page);
});

test("matching: the score lands against the 0.99 line, runner-ups labelled illustrative", async ({ page }) => {
  await freezeClock(page);
  await page.goto("/app/pipeline/matching");
  await expect(page.locator("h1")).toHaveText("Identity matching");
  await expect(page.getByText(LINE)).toBeVisible();
  await page.clock.runFor(800);
  await page.getByRole("button", { name: "Pause" }).click();
  await expect(page.getByTestId("match-p")).toHaveText("0.949");
  await expect(page.getByText("auto-close line 0.99")).toBeVisible();
  await expect(page.getByText(/illustrative/i).first()).toBeVisible();
  await page.screenshot({ path: `${SHOTS}-matching-rest.png`, fullPage: true });

  await page.getByRole("button", { name: /^Aisha/ }).click();
  await expect(page.getByTestId("match-p")).toHaveText("—");
  for (let i = 0; i < 9; i++) await page.getByRole("button", { name: "Step", exact: true }).click();
  await expect(page.getByTestId("match-p")).toHaveText("1.000");
  await page.getByRole("button", { name: "Step", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Matched on the Emirates ID" })).toBeVisible();

  await page.getByRole("button", { name: "How it decides" }).click();
  await expect(page.getByText(/R@10 0\.932/)).toBeVisible();
  await page.getByRole("button", { name: "Play" }).click();
  await page.clock.runFor(6 * 900);
  await page.screenshot({ path: `${SHOTS}-matching-mid-run.png`, fullPage: true });
  await page.getByRole("button", { name: "Pause" }).click();
  await page.clock.runFor(1000);
  await expectNoSeriousAxe(page);
});

test("reading: real report, recorded Gemma answer and latency, played 5x faster", async ({ page }) => {
  await freezeClock(page);
  await page.goto("/app/pipeline/reading");
  await expect(page.locator("h1")).toHaveText("Report reading");
  await expect(page.getByText(/played 5× faster/).first()).toBeVisible();
  await page.clock.runFor(800);
  await page.getByRole("button", { name: "Pause" }).click();
  await expect(page.getByTestId("ai-timer")).toHaveText("8.8 s recorded");
  await expect(page.getByTestId("gate-pill")).toHaveText("Passed");
  await expect(page.getByTestId("report-text")).toContainText("The preparation was excellent and all surfaces were well seen.");
  await expect(page.getByText("Fatima Al Zaabi's report SKM-dr-226")).toBeVisible();
  await page.screenshot({ path: `${SHOTS}-reading-rest.png`, fullPage: true });

  await page.getByRole("button", { name: "Report u38" }).click();
  await expect(page.getByTestId("gate-pill")).toHaveText("Waiting");
  for (let i = 0; i < 3; i++) await page.getByRole("button", { name: "Step", exact: true }).click();
  await expect(page.getByTestId("ai-timer")).toHaveText("reading · 9.0 s");
  await page.getByRole("button", { name: "Step", exact: true }).click();
  await expect(page.getByTestId("ai-timer")).toHaveText("10.1 s recorded");
  await page.getByRole("button", { name: "Step", exact: true }).click();
  await expect(page.locator('[data-field="caecum"]')).toContainText("disagree");
  await expect(page.locator('[data-field="tissue"]')).toContainText("not compared");
  await page.getByRole("button", { name: "Step", exact: true }).click();
  await expect(page.getByTestId("gate-pill")).toHaveText("Two reviewers");
  await page.screenshot({ path: `${SHOTS}-reading-u38.png`, fullPage: true });

  await page.getByRole("button", { name: "Report u6" }).click();
  await page.getByRole("button", { name: "Play" }).click();
  await page.clock.runFor(3 * 900);
  await expect(page.getByTestId("ai-timer")).toHaveText("reading · 9.0 s");
  await page.screenshot({ path: `${SHOTS}-reading-mid-run.png`, fullPage: true });
  await page.getByRole("button", { name: "Pause" }).click();
  await page.clock.runFor(1000);
  await expectNoSeriousAxe(page);
});
