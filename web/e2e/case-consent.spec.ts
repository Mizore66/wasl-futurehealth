import { expect, test } from "@playwright/test";
import fixtures from "../data/api/fixtures.json";

/*
 * Issue #42, fixtures mode (no WASL_API_URL): there is no ledger to append to, so the consent actions are disabled and
 * say why, and nothing pretends to be saved. The live flows run against a mock API in case-live.spec.ts.
 */
for (const width of [1440, 390] as const) {
  test(`without the live API the consent actions are disabled and say so (${width}px)`, async ({ page }) => {
    await page.setViewportSize({ width, height: width === 390 ? 844 : 900 });
    await page.goto(`/app/patients/${encodeURIComponent(fixtures.fixtures.review_hold.id)}`);
    const region = page.getByRole("region", { name: "Demo consent overlay" });
    await expect(region).toContainText("never changes the simulation's consent, outreach or decisions, and no message is sent");
    await expect(region).toContainText("This case data carries no demo consent overlay.");
    for (const name of ["Consent granted", "Consent denied", "Opted out"]) await expect(region.getByRole("button", { name, exact: true })).toBeDisabled();
    await expect(region.getByTestId("consent-disabled-reason")).toHaveText("Needs the live API: fixtures mode has no ledger to record consent on.");
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
  });
}
