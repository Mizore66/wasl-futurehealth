import { expect, test } from "@playwright/test";

test("lab renders three avatar systems and the theme demo", async ({ page }) => {
  await page.goto("/lab");
  await expect(page.getByText("Lab · not part of the product")).toBeVisible();
  await expect(page.getByRole("heading", { level: 3, name: /^[ABC] · / })).toHaveCount(3);
  await expect(page.getByRole("img", { name: "Fatima Al Zaabi" }).first()).toBeVisible();
  await page.waitForTimeout(1500);
  await page.screenshot({ path: "test-results/screens/lab-1440.png", fullPage: true });
  await page.getByLabel("Pipeline mock").screenshot({ path: "test-results/screens/lab-pipeline.png" });
  await page.getByRole("button", { name: "C" }).click();
  await page.waitForTimeout(400);
  await page.getByLabel("Pipeline mock").screenshot({ path: "test-results/screens/lab-pipeline-c.png" });
});

test("reduced motion skips the resolving animation", async ({ browser }) => {
  const ctx = await browser.newContext({ reducedMotion: "reduce", viewport: { width: 1440, height: 900 } });
  const page = await ctx.newPage();
  await page.goto("/lab");
  await expect(page.locator('[data-state="resolving"]').first()).toBeAttached({ timeout: 5000 });
  const anims = await page.evaluate(() => document.getAnimations().filter((a) => a instanceof CSSAnimation).length);
  expect(anims).toBe(0);
  await ctx.close();
});
