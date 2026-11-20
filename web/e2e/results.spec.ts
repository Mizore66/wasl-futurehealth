import { expect, test } from "@playwright/test";
import dashboard from "../data/api/dashboard.json";
import { AXIS, axe, inSources, numbersOn } from "./numbers";

test.describe("Results", () => {
  test("shows the held-out figures as charts with table views, the demo run as its own bar, and the review comparison", async ({ page }) => {
    await page.goto("/app/results");
    await expect(page.locator("h1")).toHaveText("Results");
    await expect(page.locator("[data-honesty]:visible").first()).toBeVisible();
    await expect(page.getByText("Measured on unseen test data across 5 runs")).toBeVisible();
    await expect(page.getByTestId("lead-figure")).toHaveText("0 of 3,035");
    // False closures: a "cases checked" bar per zero-event counter, "0 of N" with its 95% upper bound beside it.
    await expect(page.getByTestId("bound-crc")).toContainText("0 of 3,035");
    await expect(page.getByTestId("bound-crc")).toContainText("≤ 0.12%");
    await expect(page.getByTestId("bound-wrong-patient")).toContainText("0 of 16,486");
    await expect(page.getByTestId("bound-consent")).toContainText("0 of 11,650");
    await expect(page.getByTestId("recall-chart")).toContainText("0.785");
    // Hover shows the exact interval.
    const recall = page.getByTestId("recall-chart").getByRole("button");
    await recall.hover();
    await expect(page.locator(`[id="${await recall.getAttribute("aria-describedby")}"]`)).toContainText("0.785 (0.772–0.798)");
    // The 31-day number: held-out and demo run as bar groups against the truth.
    await expect(page.getByTestId("kpi-error")).toHaveText("3.1 points off");
    await expect(page.getByTestId("kpi-error-eid")).toHaveText("Emirates ID only: 20.7 points off");
    const kpi = page.getByTestId("kpi-row");
    await expect(kpi).toHaveCount(2);
    await expect(kpi.nth(0)).toContainText("Unseen test data, average of 5 runs");
    await expect(kpi.nth(0)).toContainText("12.2%");
    await expect(kpi.nth(0)).toContainText("29.8%");
    await expect(kpi.nth(1)).toContainText("11.3%");
    await expect(kpi.nth(1)).toContainText("30.7%");
    await expect(page.getByTestId("kpi-why")).toHaveText("The demo is one run on data used for tuning. Quote the unseen test average.");
    const kpiSection = page.locator(`section[aria-labelledby="kpi-h"]`);
    await kpiSection.getByRole("button", { name: "Show as table" }).click();
    const kpiTable = kpiSection.getByRole("table", { name: "31-day completion by method" });
    for (const v of ["12.2%", "29.8%", "32.9%", "11.3%", "30.7%", "31.9%"]) await expect(kpiTable).toContainText(v);
    await expect(page.getByRole("img", { name: "3 of 7 report sets exposed an unsafe reading" })).toBeVisible();
    await expect(page.getByTestId("double-review")).toContainText("in 8,422");
    await expect(page.getByTestId("double-review")).toContainText("0");
    await expect(page.getByTestId("single-review")).toContainText("21");
    await expect(page.getByTestId("ltt")).toContainText("12,600 test cases");
    await page.screenshot({ path: "test-results/screens/results.png", fullPage: true });
    await axe(page);
  });

  test("consent lifecycle shows the dashboard's counts as simulated message intents beside the zero counters (#9)", async ({ page }) => {
    await page.goto("/app/results");
    const section = page.getByTestId("consent-lifecycle");
    await expect(section.getByRole("heading", { name: "Consent: held, released, cancelled" })).toBeVisible();
    // One short muted line, no chips; the labels are one or two words and their definitions sit in the tooltip and table.
    await expect(section).toContainText("planned messages, not patients");
    await expect(section).toContainText("No message was sent");
    await expect(section.locator("[data-dot], p .rounded-chip")).toHaveCount(0);
    for (const label of ["Held", "Released", "Cancelled", "No consent", "After opt-out", "Before opt-out"])
      await expect(section.getByRole("button", { name: new RegExp(`^${label}`) })).toBeVisible();
    const t = dashboard.metrics.consent_timing;
    const shown = async (key: string) => (await section.getByTestId(`consent-${key}`).innerText()).replace(/,/g, "");
    for (const key of Object.keys(t)) expect(await shown(key), key).toContain(String(t[key as keyof typeof t]));
    expect(t.intents_ever_held_for_consent).toBeGreaterThan(0);
    await expect(section.getByTestId("consent-opted_out_loops")).toContainText("cases had an opt-out");
    // The safety counters are drawn as 0 against the released total, like the false-closure chart.
    for (const key of ["released_without_granted_consent", "released_after_opt_out_on_file", "released_before_opt_out_recorded"])
      await expect(section.getByTestId(`consent-${key}`)).toContainText(`${t[key as keyof typeof t]} of ${t.released_total.toLocaleString("en-US")}`);
    const afterOptOut = section.getByTestId("consent-released_after_opt_out_on_file");
    await afterOptOut.hover();
    await expect(page.locator(`[id="${await afterOptOut.getAttribute("aria-describedby")}"]`)).toContainText("This is the violation");
    await expect(section).toContainText("counts cases");
    await expect(section.getByTestId("consent-intents_ever_held_for_consent")).toContainText(t.intents_ever_held_for_consent.toLocaleString("en-US"));
  });

  test("every number on the screen is in results.json, tables open or closed", async ({ page }) => {
    await page.goto("/app/results");
    expect((await numbersOn(page)).filter((n) => !AXIS.has(n) && !inSources(n))).toEqual([]);
    const toggles = page.locator("main").getByRole("button", { name: "Show as table" });
    const n = await toggles.count();
    expect(n).toBeGreaterThanOrEqual(6);
    for (let i = 0; i < n; i++) await toggles.nth(i).click();
    await expect(page.locator("main table")).toHaveCount(n);
    expect((await numbersOn(page)).filter((n) => !AXIS.has(n) && !inSources(n))).toEqual([]);
  });
});

test.describe("Results on a phone", () => {
  test("fits 390px, shows the explanation line, and its only controls are chart marks and table toggles", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/app/results");
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
    await expect(page.getByTestId("kpi-why")).toBeVisible();
    // Buttons are chart marks (tooltips) and "Show as table" toggles, nothing that changes data.
    for (const b of await page.locator("main button").all()) {
      const describedBy = await b.getAttribute("aria-describedby");
      if (!describedBy) await expect(b).toHaveAccessibleName("Show as table");
      expect((await b.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    }
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
