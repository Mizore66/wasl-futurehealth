import AxeBuilder from "@axe-core/playwright";
import { expect as baseExpect, test, type Page } from "@playwright/test";

/*
 * Fixtures mode (no live API): the receiving unit returns a referral with a reason and a note to the GP, the GP sees both
 * on their own view and acknowledges, and the unit's inbox then shows "Acknowledged by GP". The return and the
 * acknowledgement are kept in this browser's sessionStorage only and labelled so; nothing is written anywhere.
 * The live path (case ledger) is covered by referral-live.spec.ts.
 */
const expect = baseExpect.configure({ timeout: 20_000 });
test.describe.configure({ timeout: 60_000 });
test.use({ actionTimeout: 20_000, navigationTimeout: 30_000 });

const ID = "ahs-mushrif/ServiceRequest/AHS-sr-9002";
const NOTE = "Please attach the FIT report.";

async function noSeriousAxe(page: Page) {
  const axe = await new AxeBuilder({ page }).analyze();
  const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

async function noSidewaysScroll(page: Page) {
  expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(0);
}

for (const vp of [
  { name: "desktop", width: 1440, height: 900 },
  { name: "390 px", width: 390, height: 844 },
]) {
  test(`fixtures: Return with a note, the GP sees it and acknowledges, the inbox shows it (${vp.name})`, async ({ page, browser }) => {
    await page.setViewportSize({ width: vp.width, height: vp.height });
    await page.goto(`/app/referrals?case=${encodeURIComponent(ID)}`);
    const pane = page.getByRole("region", { name: "Open referral" });
    await expect(pane).toContainText("AHS-sr-9002");
    await expect(pane).toContainText("Needs response");

    // An empty submission stays blocked, even with a note written.
    await pane.getByRole("button", { name: "Return with reason" }).click();
    const note = pane.getByRole("textbox", { name: /Note to the GP/ });
    await note.fill(NOTE);
    await pane.getByRole("button", { name: "Return to GP" }).click();
    await expect(pane.getByRole("alert")).toHaveText("Choose a reason before you return it. Nothing was sent.");
    await expect(pane).toContainText("Needs response");
    expect(await page.evaluate(() => sessionStorage.getItem("wasl.referrals.session-returns.v1"))).toBeNull();

    await pane.getByRole("radio", { name: "Needs a clinic visit first" }).check();
    await pane.getByRole("button", { name: "Return to GP" }).click();
    const gp = page.getByRole("region", { name: /GP, current$/ });
    await expect(gp).toContainText("Waiting on GP");
    await expect(gp).toContainText(`Note to the GP: ${NOTE}`);
    await expect(gp).toContainText("Kept in this browser only");
    await expect(page.getByTestId("handoff-arrow")).toHaveAttribute("data-direction", "back");
    await noSidewaysScroll(page);

    // The GP's own view, in this browser.
    await gp.getByRole("link", { name: "Open the GP view" }).click();
    await expect(page).toHaveURL(/\/app\/referrals\/returned\?case=/);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Returned referrals");
    await expect(page.locator("[data-honesty=designed]").first()).toHaveText("Acknowledgement: kept in this browser only");
    const card = page.getByRole("article");
    await expect(card).toHaveCount(1);
    await expect(card).toHaveAttribute("aria-current", "true");
    await expect(card).toContainText("ReasonNeeds a clinic visit first");
    await expect(card).toContainText(`Note from the unit${NOTE}`);
    await expect(card).toContainText("AcknowledgedNot yet");
    await expect(card.getByRole("radio")).toHaveCount(0); // only acknowledgement is possible without the live API
    await noSidewaysScroll(page);
    await noSeriousAxe(page);
    await card.getByRole("button", { name: "Acknowledge the returned referral" }).click();
    await expect(card.getByRole("status")).toHaveText("Acknowledged in this browser. The return still waits on your answer.");
    await expect(card).toContainText("AcknowledgedYes");
    await page.reload();
    await expect(page.getByRole("article")).toContainText("AcknowledgedYes");

    // Back in the unit's inbox: the row reads "Acknowledged by GP".
    await page.getByRole("link", { name: "Back to inbox" }).click();
    await expect(page).toHaveURL(/view=inbox/);
    const row = page.getByRole("row", { name: /AHS-sr-9002/ });
    await expect(row).toContainText("Acknowledged by GP");
    await noSidewaysScroll(page);
    await row.getByRole("link", { name: "AHS-sr-9002" }).click();
    const back = page.getByRole("region", { name: /GP, current$/ });
    await expect(back).toContainText("Acknowledged by GP");
    await expect(back).toContainText("The GP acknowledged the return");
    await expect(back).toContainText(`Note to the GP: ${NOTE}`);
    await noSeriousAxe(page);

    // Kept in this browser only: another browser session sees nothing.
    const other = await browser.newContext({ viewport: { width: vp.width, height: vp.height } });
    const elsewhere = await other.newPage();
    await elsewhere.goto("/app/referrals/returned");
    await expect(elsewhere.getByText("No returns in this browser yet.", { exact: false })).toBeVisible();
    await other.close();
  });
}
