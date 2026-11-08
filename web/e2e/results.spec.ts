import { expect, test } from "@playwright/test";
import dashboard from "../data/api/dashboard.json";
import { AXIS, axe, inSources, numbersOn } from "./numbers";

test.describe("Results", () => {
  test("shows the held-out figures, the demo cohort on its own line, and the review comparison", async ({ page }) => {
    await page.goto("/app/results");
    await expect(page.locator("h1")).toHaveText("Results");
    await expect(page.locator("[data-honesty]:visible").first()).toBeVisible();
    await expect(page.getByText("Frozen evaluation at code 075abc5")).toBeVisible();
    await expect(page.getByTestId("lead-figure")).toHaveText("0 of 3,035");
    await expect(page.getByRole("img", { name: /95% upper bound 0\.12%/ })).toBeVisible();
    await expect(page.getByText("Recall 0.785 (0.772–0.798).")).toBeVisible();
    await expect(page.getByText(/0 of 16,486 wrong-patient messages and 0 of 11,650 sent without consent/)).toBeVisible();
    await expect(page.getByTestId("kpi-error")).toHaveText("3.1 points off");
    await expect(page.getByText("Emirates ID only: 20.7 points off · mean of 5 seeds")).toBeVisible();
    await expect(page.getByRole("img", { name: "Within 31 days, held-out noise: Emirates ID only 12.2%, Wasl 29.8%, truth 32.9%." })).toBeVisible();
    await expect(page.getByTestId("demo-cohort")).toHaveText(
      "Demo cohort, one run on training-time noise: Emirates ID only 11.3%, Wasl 30.7%, truth 31.9%.",
    );
    await expect(page.getByTestId("kpi-heldout-h")).toHaveText("Five-seed held-out mean");
    await expect(page.getByTestId("kpi-why")).toBeVisible();
    await expect(page.getByTestId("kpi-why")).toContainText("single seed on noise the system was tuned on");
    await expect(page.getByTestId("kpi-why")).toContainText("Wasl reads 30.7% there and 29.8% on the five-seed held-out mean");
    await expect(page.getByText("3 of 7")).toBeVisible();
    await expect(page.getByTestId("double-review")).toHaveText("0in 8,422");
    await expect(page.getByTestId("single-review")).toHaveText("21");
    await expect(page.getByText(/On 12,600 calibration cases/)).toBeVisible();
    await page.screenshot({ path: "test-results/screens/results.png", fullPage: true });
    await axe(page);
  });

  test("consent lifecycle shows the dashboard's counts as simulated message intents beside the zero counters (#9)", async ({ page }) => {
    await page.goto("/app/results");
    const section = page.getByTestId("consent-lifecycle");
    await expect(section.getByRole("heading", { name: "Consent: held, released, cancelled" })).toBeVisible();
    await expect(section.locator('[data-honesty="simulated"]')).toBeVisible();
    await expect(section).toContainText("message intents, not of patients");
    await expect(section).toContainText("No message was sent to anyone");
    const t = dashboard.metrics.consent_timing;
    const shown = async (key: string) => (await section.getByTestId(`consent-${key}`).innerText()).replace(/,/g, "");
    for (const key of Object.keys(t)) expect(await shown(key), key).toContain(String(t[key as keyof typeof t]));
    expect(t.intents_ever_held_for_consent).toBeGreaterThan(0);
    await expect(section.getByTestId("consent-opted_out_loops")).toContainText("loops with an opt-out");
    await expect(section).toContainText("counts loops");
    // The table repeats the same figures for anyone not using the diagram.
    const row = section.getByRole("row", { name: /^Held/ });
    await expect(row).toContainText(t.intents_ever_held_for_consent.toLocaleString("en-US"));
    await expect(section.getByRole("row", { name: /Loops with an opt-out/ })).toContainText("loops");
  });

  test("every number on the screen is in results.json", async ({ page }) => {
    await page.goto("/app/results");
    expect((await numbersOn(page)).filter((n) => !AXIS.has(n) && !inSources(n))).toEqual([]);
  });
});

test.describe("Results on a phone", () => {
  test("fits 390px, shows the explanation line, and has no controls to mislead", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/app/results");
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
    await expect(page.getByTestId("kpi-why")).toBeVisible();
    await expect(page.locator("main button")).toHaveCount(0);
    await expect(page.getByTestId("consent-lifecycle")).toBeVisible();
    for (const el of await page.getByTestId("consent-lifecycle").locator("summary").all()) expect((await el.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await page.screenshot({ path: "test-results/screens/pipe2-results-390.png", fullPage: true });
    await expect(page.locator("main")).not.toContainText(/closure_decision|needs_human|not_closed|link_decision|review_hold/);
    await page.screenshot({ path: "test-results/screens/results-390.png", fullPage: true });
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.screenshot({ path: "test-results/screens/results-1440.png", fullPage: true });
    await page.setViewportSize({ width: 390, height: 844 });
    await axe(page);
  });
});
