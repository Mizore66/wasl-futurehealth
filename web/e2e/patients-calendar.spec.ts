import AxeBuilder from "@axe-core/playwright";
import { expect as baseExpect, test, type Page } from "@playwright/test";

/*
 * user-flows.html "Find the record from the calendar", on the app's fixture case that plays that role: AHS-sr-1011
 * (Aisha, held: an outside colonoscopy report with no prep grade), referred on 27 May 2026. Fixtures mode.
 */
const expect = baseExpect.configure({ timeout: 20_000 });
test.describe.configure({ timeout: 90_000 });
test.use({ actionTimeout: 20_000, navigationTimeout: 30_000 });

const ID = "ahs-mushrif/ServiceRequest/AHS-sr-1011";
const RECORD = `/app/patients/${encodeURIComponent(ID)}`;
const shot = (page: Page, name: string) => page.screenshot({ path: `test-results/screens/patients-calendar-${name}.png`, fullPage: true });

async function noSeriousAxe(page: Page) {
  const axe = await new AxeBuilder({ page }).analyze();
  const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

for (const [label, viewport] of [
  ["desktop", { width: 1440, height: 900 }],
  ["phone", { width: 390, height: 844 }],
] as const) {
  test(`find the record from the calendar (${label})`, async ({ page }) => {
    await page.setViewportSize(viewport);
    await page.goto("/app/patients");
    await expect(page.getByRole("heading", { level: 2, name: "Referral calendar" })).toBeVisible();

    // `/` jumps to the search.
    await page.locator("body").click({ position: { x: 5, y: 5 } });
    await page.keyboard.press("/");
    const search = page.getByRole("searchbox", { name: "Find a case, task or owner" });
    await expect(search).toBeFocused();
    await search.fill("AHS-sr-1011");
    await search.press("Enter");
    await expect(page).toHaveURL(/q=AHS-sr-1011/);
    await expect(page.getByRole("heading", { level: 3, name: "May 2026" })).toBeVisible();
    await expect(page.getByTestId("month-total")).toHaveText("1 referral");
    await shot(page, `search-${label}`);
    await noSeriousAxe(page);

    // Its date tile opens the case files of 27 May.
    await page.getByRole("link", { name: /^27 May 2026, 1 referral\. View case files$/ }).click();
    await expect(page).toHaveURL(/files=2026-05-27/);
    await expect(page.getByRole("heading", { level: 2, name: "Case files" })).toBeVisible();
    const folder = page.locator(`[data-pf-case="${ID}"]`);
    await expect(folder).toContainText("Aisha Al Mansoori");
    await shot(page, `files-${label}`);

    // The folder opens on the Evidence File.
    await folder.click();
    await expect(page).toHaveURL(/open=ahs-mushrif%2FServiceRequest%2FAHS-sr-1011/);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Aisha Al Mansoori");
    const file = page.getByRole("region", { name: "Evidence file for AHS-sr-1011" });
    const sources = file.getByRole("group", { name: "Choose a source document" });
    await expect(sources.getByRole("button", { name: "Outside report" })).toHaveAttribute("aria-pressed", "true");
    await expect(file.getByRole("region", { name: "DoH closure checklist" })).toBeVisible();
    await expect(file.getByText("“until it reached the cecum”")).toBeVisible();
    await shot(page, `open-${label}`);
    await noSeriousAxe(page);

    // Switch to the referral and back.
    await sources.getByRole("button", { name: "Referral" }).click();
    await expect(sources.getByRole("button", { name: "Referral" })).toHaveAttribute("aria-pressed", "true");
    await expect(file.getByRole("article", { name: "Referral AHS-sr-1011" })).toBeVisible();
    await expect(file.getByRole("region", { name: "DoH closure checklist" })).toBeHidden();
    await sources.getByRole("button", { name: "Outside report" }).click();
    await expect(file.getByRole("region", { name: "DoH closure checklist" })).toBeVisible();

    // The Audit trail clock beside Activity, then back to the case.
    await expect(file.getByRole("list", { name: "Timeline" }).getByRole("listitem").first()).toBeVisible();
    await file.getByRole("link", { name: "Audit trail" }).click();
    await expect(page).toHaveURL(`${RECORD}/audit`);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Audit trail");
    await expect(page.getByRole("main")).toContainText("AHS-sr-1011");
    await page.getByRole("link", { name: /Back to the case/ }).click();
    await expect(page).toHaveURL(RECORD);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Aisha Al Mansoori");

    // Back to files returns to the folder's case files, search kept; Calendar restores the search.
    const back = page.getByRole("link", { name: "Back to files" });
    await expect(back).toHaveAttribute("href", /files=2026-05-27/);
    await expect(back).toHaveAttribute("href", /q=AHS-sr-1011/);
    await back.click();
    await expect(page).toHaveURL(/files=2026-05-27/);
    await expect(page).not.toHaveURL(/open=/);
    await expect(page.locator(`[data-pf-case="${ID}"]`)).toBeVisible();
    await page.getByRole("link", { name: "Calendar", exact: true }).click();
    await expect(page.getByRole("heading", { level: 2, name: "Referral calendar" })).toBeVisible();
    await expect(page.getByRole("searchbox", { name: "Find a case, task or owner" })).toHaveValue("AHS-sr-1011");
    await expect(page.getByRole("heading", { level: 3, name: "May 2026" })).toBeVisible();
  });
}

test("a record opened directly goes back to the files of its referral date", async ({ page }) => {
  await page.goto(RECORD);
  await expect(page.getByRole("link", { name: "Back to files" })).toHaveAttribute("href", "/app/patients?files=2026-05-27");
});

test("month navigation steps between months that hold referrals", async ({ page }) => {
  await page.goto("/app/patients?month=2026-05");
  await expect(page.getByRole("heading", { level: 3, name: "May 2026" })).toBeVisible();
  await expect(page.getByTestId("month-total")).toHaveText("39 referrals");
  await page.getByRole("link", { name: /^Next referral month/ }).click();
  await expect(page).toHaveURL(/month=2026-06/);
  await expect(page.getByRole("heading", { level: 3, name: "June 2026" })).toBeVisible();
  // A busy date is one tile with a count, never one chip per case.
  await page.goto("/app/patients?month=2026-05");
  const busy = page.getByRole("link", { name: /^18 May 2026, 4 referrals/ });
  await expect(busy).toContainText("+3 more");
  await busy.click();
  await expect(page.locator("[data-pf-case]")).toHaveCount(4);
});
