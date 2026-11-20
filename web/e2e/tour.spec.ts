import { expect, test } from "@playwright/test";

test.describe("guided tour", () => {
  test("never opens on its own in an automated browser", async ({ page }) => {
    await page.goto("/app");
    await page.waitForTimeout(1200);
    await expect(page.getByRole("dialog", { name: "Welcome to Wasl" })).toHaveCount(0);
  });

  test("walks Today, Patients, Review desk and Programme by clicking the highlighted links", async ({ page }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto("/app/review");
    await page.getByRole("button", { name: "Show the guided tour" }).click();

    const tour = page.getByTestId("guided-tour");
    const visited = new Set<string>();
    for (let i = 0; i < 25; i++) {
      await expect(tour).toBeVisible();
      const goTo = await tour.getAttribute("data-goto");
      const step = await tour.getAttribute("data-step");
      if (goTo) {
        // A click step: the highlighted sidebar link is the only live control, so a real click must land on it.
        await page.locator(`#app-sidebar a[href="${goTo}"]`).click();
        await expect(page).toHaveURL(new RegExp(`${goTo.replace(/\//g, "\\/")}(\\?|$)`));
        visited.add(goTo);
      } else {
        const done = page.getByRole("button", { name: "Done" });
        if (await done.isVisible()) {
          await done.click();
          break;
        }
        await page.getByRole("dialog").getByRole("button", { name: "Next" }).click();
      }
      await expect(tour).not.toHaveAttribute("data-step", step!);
    }

    await expect(tour).toHaveCount(0);
    expect([...visited]).toEqual(["/app/today", "/app/patients", "/app/review", "/app"]);
    expect(await page.evaluate(() => localStorage.getItem("wasl.tour.seen"))).toBe("1");
  });
});
