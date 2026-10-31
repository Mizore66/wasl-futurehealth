import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";
import { ROUTES, screenName } from "./routes";

for (const route of ROUTES) {
  test(`${route} renders its heading and an honesty label`, async ({ page }) => {
    // Axe must see the settled screen, not a replay row mid-fade.
    await page.emulateMedia({ reducedMotion: "reduce" });
    const res = await page.goto(route);
    expect(res?.status()).toBe(200);
    await expect(page.locator("h1")).toHaveCount(1);
    await expect(page.locator("h1")).toBeVisible();
    await expect(page.locator("[data-honesty]:visible").first()).toBeVisible();
    await page.screenshot({ path: `test-results/screens/${screenName(route)}.png`, fullPage: true });

    const axe = await new AxeBuilder({ page }).analyze();
    const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
    expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
  });
}

test("the shell marks the current page and carries the synthetic-data chip", async ({ page }) => {
  await page.goto("/app/pipeline/matching");
  const nav = page.getByRole("navigation", { name: "Main" });
  await expect(nav.getByRole("link", { name: "Pipeline" })).toHaveAttribute("aria-current", "page");
  await expect(nav.locator("[aria-current]")).toHaveCount(1);
  await expect(page.getByText("Synthetic data · demo").first()).toBeVisible();
  await nav.getByRole("link", { name: "Review" }).click();
  await expect(page).toHaveURL(/\/app\/review$/);
  await expect(nav.getByRole("link", { name: "Review" })).toHaveAttribute("aria-current", "page");
});

test("an unknown case is a 404, not a blank page", async ({ page }) => {
  const res = await page.goto(`/app/patients/${encodeURIComponent("nowhere/ServiceRequest/none")}`);
  expect(res?.status()).toBe(404);
});
