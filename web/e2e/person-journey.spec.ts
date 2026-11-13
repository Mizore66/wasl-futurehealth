import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import journey from "../data/api/person_journey.json";

const caseURL = `/app/patients/${encodeURIComponent(journey.person.anchor_case_id)}`;
const personURL = `/app/people/${journey.person.id}`;

async function axeClean(page: Page) {
  const report = await new AxeBuilder({ page }).analyze();
  expect(report.violations.filter((v) => v.impact === "serious" || v.impact === "critical").map((v) => v.id)).toEqual([]);
}

test("case navigation resolves an opaque person and renders the read-only journey", async ({ page }) => {
  await page.goto(caseURL);
  await page.getByRole("link", { name: "View all programmes" }).click();
  await expect(page).toHaveURL(personURL);
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  await expect(page.getByText("Trusted identity links only", { exact: true })).toBeVisible();
  await expect(page.getByTestId("next-action")).toContainText("wasl-person-next-action/1");
  await expect(page.getByTestId("next-action")).toContainText("No open coordinator action was found");
  await expect(page.getByText("Programme contact permission", { exact: true })).toBeVisible();
  await expect(page.getByText("Timeline and raw event hashes", { exact: true })).toBeVisible();
  await expect(page.getByText("Decision history and signatures", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "Open full case and evidence" })).toHaveAttribute("href", caseURL);
  await expect(page.getByRole("button", { name: /send|suppress|dismiss/i })).toHaveCount(0);
  await axeClean(page);
});

test("programme anchors are keyboard accessible", async ({ page }) => {
  await page.goto(personURL);
  const obligations = page.getByRole("link", { name: "Obligations", exact: true });
  await obligations.focus();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(`${personURL}#upcoming-obligations`);
  await expect(page.locator("#upcoming-obligations")).toBeVisible();
});

test("unknown opaque person shows the safe not-found state", async ({ page }) => {
  const response = await page.goto(`/app/people/person_${"0".repeat(64)}`);
  expect(response?.status()).toBe(404);
  await expect(page.getByRole("heading", { name: "Person journey not found" })).toBeVisible();
  await expect(page.getByText(/No similar-name records were substituted/)).toBeVisible();
});

for (const width of [390, 1440]) {
  test(`person journey has no horizontal overflow or serious axe violations at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(personURL);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
    await axeClean(page);
    await page.screenshot({ path: `test-results/screens/person-journey-${width}.png`, fullPage: true });
  });
}
