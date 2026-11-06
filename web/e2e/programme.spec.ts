import { expect, test } from "@playwright/test";
import { AXIS, axe, inSources, numbersOn } from "./numbers";

test.describe("Programme", () => {
  test("leads with reported vs measured, one track, the target and the truth line", async ({ page }) => {
    await page.goto("/app");
    await expect(page.locator("h1")).toHaveText("Programme");
    await expect(page.locator("[data-honesty]:visible").first()).toBeVisible();
    await expect(page.getByTestId("kpi-eid")).toContainText("12.6%");
    await expect(page.getByTestId("kpi-wasl")).toContainText("31.9%");
    await expect(page.getByRole("img", { name: "On a 0 to 100% scale: reported 12.6%, measured by Wasl 31.9%, target 90%." })).toBeVisible();
    await expect(page.getByTestId("kpi-truth")).toHaveText("Within 1 point of the true rate (32.9%) in testing");
    await expect(page.getByText("1 in 6 on your chase list is already done.")).toBeVisible();
    await page.screenshot({ path: "test-results/screens/programme.png", fullPage: true });
    await axe(page);
  });

  test("both Source disclosures open and close", async ({ page }) => {
    await page.goto("/app");
    for (const [card, note] of [
      ["kpi-h", "The target is the DoH 31-day standard."],
      ["stall-h", "Simulated run of 2,100 cases"],
    ] as const) {
      const section = page.locator(`section[aria-labelledby="${card}"]`);
      const button = section.getByRole("button", { name: "Source" });
      await expect(section.getByText(note)).toBeHidden();
      await button.click();
      await expect(section.getByRole("button", { name: "Hide source" })).toHaveAttribute("aria-expanded", "true");
      await expect(section.getByText(note)).toBeVisible();
      if (card === "stall-h") await page.screenshot({ path: "test-results/screens/programme-source.png", fullPage: true });
      await section.getByRole("button", { name: "Hide source" }).click();
      await expect(section.getByText(note)).toBeHidden();
    }
  });

  test("the top three stalls each link to the filtered patient list", async ({ page }) => {
    await page.goto("/app");
    const rows = page.locator('section[aria-labelledby="stall-h"] ol a');
    await expect(rows).toHaveCount(3);
    const expected = [
      ["No acknowledgement", "390", "no_acknowledgement"],
      ["Not booked", "252", "not_booked"],
      ["Never notified", "183", "never_notified"],
    ];
    for (const [i, [label, count, dropoff]] of expected.entries()) {
      await expect(rows.nth(i)).toContainText(label);
      await expect(rows.nth(i)).toContainText(count);
      await expect(rows.nth(i)).toHaveAttribute("href", `/app/patients?dropoff=${dropoff}`);
    }
    await rows.first().click();
    await expect(page).toHaveURL(/\/app\/patients\?dropoff=no_acknowledgement$/);
  });

  test("Open today's list and See proof go where the board says", async ({ page }) => {
    await page.goto("/app");
    await expect(page.getByLabel("Safety")).toContainText("0 false closures in 3,025 tested.");
    await page.getByRole("link", { name: "See proof" }).click();
    await expect(page).toHaveURL(/\/app\/results$/);
    await page.goto("/app");
    await page.getByRole("link", { name: "Open today's list" }).click();
    await expect(page).toHaveURL(/\/app\/today$/);
  });

  test("every number on the screen is in results.json or the dashboard", async ({ page }) => {
    await page.goto("/app");
    expect((await numbersOn(page)).filter((n) => !AXIS.has(n) && !inSources(n))).toEqual([]);
  });
});
