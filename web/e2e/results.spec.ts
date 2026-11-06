import { expect, test } from "@playwright/test";
import { AXIS, axe, inSources, numbersOn } from "./numbers";

test.describe("Results", () => {
  test("shows the held-out figures, the demo cohort on its own line, and the review comparison", async ({ page }) => {
    await page.goto("/app/results");
    await expect(page.locator("h1")).toHaveText("Results");
    await expect(page.locator("[data-honesty]:visible").first()).toBeVisible();
    await expect(page.getByText("Frozen evaluation at code 5496563")).toBeVisible();
    await expect(page.getByTestId("lead-figure")).toHaveText("0 of 3,025");
    await expect(page.getByRole("img", { name: /95% upper bound 0\.12%/ })).toBeVisible();
    await expect(page.getByText("Recall 0.782 (0.769–0.795).")).toBeVisible();
    await expect(page.getByText(/0 of 14,451 wrong-patient messages and 0 of 9,602 sent without consent/)).toBeVisible();
    await expect(page.getByTestId("kpi-error")).toHaveText("3.1 points off");
    await expect(page.getByText("Emirates ID only: 20.7 points off · mean of 5 seeds")).toBeVisible();
    await expect(page.getByRole("img", { name: "Within 31 days, held-out noise: Emirates ID only 12.2%, Wasl 29.8%, truth 32.9%." })).toBeVisible();
    await expect(page.getByTestId("demo-cohort")).toHaveText(
      "Demo cohort, one run on training-time noise: Emirates ID only 12.6%, Wasl 31.9%, truth 32.9%.",
    );
    await expect(page.getByText("3 of 7")).toBeVisible();
    await expect(page.getByTestId("double-review")).toHaveText("0in 8,422");
    await expect(page.getByTestId("single-review")).toHaveText("21");
    await expect(page.getByText(/On 12,600 calibration cases/)).toBeVisible();
    await page.screenshot({ path: "test-results/screens/results.png", fullPage: true });
    await axe(page);
  });

  test("every number on the screen is in results.json", async ({ page }) => {
    await page.goto("/app/results");
    expect((await numbersOn(page)).filter((n) => !AXIS.has(n) && !inSources(n))).toEqual([]);
  });
});
