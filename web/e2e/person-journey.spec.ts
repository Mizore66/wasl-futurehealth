import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import published from "../data/api/person_journey.json";
import { PersonJourney } from "../lib/api/person-schema";
import fixtures from "../data/api/fixtures.json";

const journey = PersonJourney.parse(published);

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
  await expect(page.getByText("Programme contact permission", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("Timeline and raw event hashes", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("Decision history and signatures", { exact: true }).first()).toBeVisible();
  await expect(page.getByRole("link", { name: "Open full case and evidence" }).first()).toHaveAttribute("href", caseURL);
  await expect(page.getByRole("button", { name: /send|suppress|dismiss/i })).toHaveCount(0);
  await expect(page).toHaveTitle("Person journey · Wasl");
  await axeClean(page);
});

// #161: the published journey is an actionable cross-programme person (docs/api-samples/README.md, -breastshowcase).
test("published journey shows linked programmes, an open obligation, an actionable outreach group and its next action", async ({ page }) => {
  await page.goto(caseURL);
  await page.getByRole("link", { name: "View all programmes" }).click();
  await expect(page).toHaveURL(personURL);
  await expect(page.getByText(`${journey.cases.length} linked cases`, { exact: false })).toBeVisible();
  const sections = page.getByRole("navigation", { name: "Person journey sections" });
  for (const programme of journey.person.programmes) await expect(sections.locator(`a[href="#programme-${journey.person.programmes.indexOf(programme)}"]`)).toBeVisible();
  expect(journey.person.programmes.length).toBeGreaterThanOrEqual(3);

  const open = journey.obligations.filter((o) => o.status === "open" || o.status === "overdue");
  expect(open.length).toBeGreaterThan(0);
  await expect(page.locator("#upcoming-obligations").locator("..")).toContainText(open[0].status);

  const flag = journey.outreach_flags.find((f) => f.actionable)!;
  expect(flag).toBeTruthy();
  const group = page.locator(`[id="flag-${flag.id}"]`);
  await expect(group).toContainText("review actionable");
  for (const intent of flag.intents) await expect(group.locator(`a[href="/app/patients/${encodeURIComponent(intent.case_id)}"]`).first()).toBeVisible();

  const action = page.getByTestId("next-action");
  expect(journey.next_action.kind).not.toBe("none");
  await expect(action).not.toContainText("No open coordinator action was found");
  const source = action.getByRole("link", { name: /^Open source case / });
  await expect(source).toHaveAttribute("href", `/app/patients/${encodeURIComponent(journey.next_action.case_id!)}`);
  if (journey.next_action.outreach_flag_id) {
    await action.getByRole("link", { name: "Open the related outreach review group" }).click();
    await expect(page).toHaveURL(`${personURL}#flag-${journey.next_action.outreach_flag_id}`);
  }
  if (journey.next_action.case_id === journey.person.anchor_case_id) {
    await source.click();
    await expect(page).toHaveURL(caseURL);
  }
  await expect(page.getByRole("button", { name: /send|suppress|dismiss/i })).toHaveCount(0);
});

for (const [fixture, item] of Object.entries(fixtures.fixtures)) {
  if (item.id === journey.person.anchor_case_id) continue;
  test(`${fixture} does not navigate to an unpublished person journey`, async ({ page }) => {
    await page.goto(`/app/patients/${encodeURIComponent(item.id)}`);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await expect(page.getByRole("link", { name: "View all programmes" })).toHaveCount(0);
    const response = await page.goto(`/app/people?case_id=${encodeURIComponent(item.id)}`);
    expect(response?.status()).toBe(404);
    await expect(page).not.toHaveURL(/\/app\/people\/person_/);
  });
}

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
