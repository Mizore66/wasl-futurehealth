import AxeBuilder from "@axe-core/playwright";
import { expect as baseExpect, test, type Page } from "@playwright/test";

/*
 * #155 acceptance 5 in the browser (fixtures mode): the lung work-up and diabetic-eye views of /app/referrals are
 * chosen by URL, and the choice survives search, reload and the round trip to the referrer's view. The packets are the
 * committed synthetic canonical controls (web/data/referral-handoff-synthetic.json); nothing here writes anything.
 */
const expect = baseExpect.configure({ timeout: 20_000 });
test.describe.configure({ timeout: 60_000 });
test.use({ actionTimeout: 20_000, navigationTimeout: 30_000 });

const LUNG = "programme=lung_nodule&provider=rad-imaging";
const DR = "programme=diabetic_retinopathy&provider=phc-retina";
const CRC_COPY = /FIT|Day 31|SKMC|colonoscopy|endoscopy|\bGP\b/;

async function noSeriousAxe(page: Page) {
  const axe = await new AxeBuilder({ page }).analyze();
  const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

const views = (page: Page) => page.getByRole("navigation", { name: "Referral programme" });

test("the lung view is chosen by URL and survives search, reload and the referrer's view", async ({ page }) => {
  await page.goto("/app/referrals");
  await expect(views(page).getByRole("link", { name: "Colorectal, Al Mushrif" })).toHaveAttribute("aria-current", "true");

  await views(page).getByRole("link", { name: "Lung nodule work-up" }).click();
  await expect(page).toHaveURL(`/app/referrals?${LUNG}`);
  await expect(views(page).getByRole("link", { name: "Lung nodule work-up" })).toHaveAttribute("aria-current", "true");
  await expect(page.getByRole("region", { name: /current$/ })).toContainText("Waiting on referrer");
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("Nodule work-up referral");

  await page.getByRole("link", { name: /^View all/ }).click();
  const box = page.getByRole("searchbox", { name: "Search case id" });
  await box.fill("9101");
  await box.press("Enter");
  await expect(page).toHaveURL(`/app/referrals?${LUNG}&q=9101&view=inbox`);
  await expect(box).toBeFocused();
  await page.reload();
  await expect(views(page).getByRole("link", { name: "Lung nodule work-up" })).toHaveAttribute("aria-current", "true");
  await page.locator("tbody a").first().click();
  const pane = page.getByRole("region", { name: "Open referral" });
  await pane.getByRole("button", { name: "Referral details" }).click();
  await expect(pane).toContainText("PriorityUrgent");
  await expect(pane).toContainText("PET/CT or tissue sampling for a solid nodule > 8 mm");
  await expect(pane).toContainText("Returned byRadiology department, regional hospital");
  await expect(pane).toContainText("Responsible clinicianReferring clinician, Outpatient imaging centre");
  await expect(pane).not.toContainText(CRC_COPY);
  await noSeriousAxe(page);

  await page.getByRole("link", { name: "Returned referrals, referring clinician view" }).click();
  await expect(page).toHaveURL(`/app/referrals/returned?${LUNG}`);
  await expect(page.getByRole("navigation", { name: "Referring provider" }).getByRole("link", { name: "Lung nodule work-up" })).toHaveAttribute("aria-current", "true");
  await expect(page.getByRole("article")).toContainText("PET/CT or tissue sampling");
  await expect(page.getByRole("article")).not.toContainText(CRC_COPY);
  await page.getByRole("link", { name: "Receiving unit inbox" }).click();
  await expect(page).toHaveURL(`/app/referrals?${LUNG}`);
  await expect(views(page).getByRole("link", { name: "Lung nodule work-up" })).toHaveAttribute("aria-current", "true");
});

test("an unknown programme and provider pair and an empty search are honest, not CRC fallbacks", async ({ page }) => {
  await page.goto("/app/referrals?programme=lung_nodule&provider=ahs-mushrif");
  await expect(page.getByText("No referral workflow for this programme and provider")).toBeVisible();
  await expect(page.locator("main")).not.toContainText(/AHS-sr|Al Mansoori/);
  await page.goto(`/app/referrals?${DR}&q=nothing-matches`);
  await expect(views(page).getByRole("link", { name: "Diabetic eye" })).toHaveAttribute("aria-current", "true");
  await expect(page.getByRole("heading", { name: "Nothing to open" })).toBeVisible();
});

test("phone width: a DR packet opens without horizontal scroll and keeps its view", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`/app/referrals?${DR}`);
  await expect(views(page).getByRole("link", { name: "Diabetic eye" })).toHaveAttribute("aria-current", "true");
  await page.getByRole("link", { name: /^View all/ }).click();
  await page.locator("tbody a").first().click();
  await page.getByRole("button", { name: "Referral details" }).click();
  const pane = page.getByRole("region", { name: "Open referral" });
  await expect(pane).toContainText("Other unitNone suggested");
  await expect(pane).not.toContainText(CRC_COPY);
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  await page.reload();
  await expect(page).toHaveURL(new RegExp(`/app/referrals\\?${DR}&from=inbox&case=`));
  await expect(views(page).getByRole("link", { name: "Diabetic eye" })).toHaveAttribute("aria-current", "true");
});
