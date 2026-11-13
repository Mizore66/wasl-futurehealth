import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import fixtures from "../data/api/fixtures.json";

/*
 * Fixtures mode: run against a dev server without WASL_API_URL. The live, token-authenticated independent-review
 * workflow is covered by review-independent.spec.ts against its own stateful mock API.
 */
const LIVE = !!process.env.REVIEW_LIVE_MOCK;

const holdId = fixtures.fixtures.review_hold.id;
const desk = `/app/review/${encodeURIComponent(holdId)}`;

async function noSeriousAxe(page: Page) {
  const axe = await new AxeBuilder({ page }).analyze();
  const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

const rail = (page: Page) => page.getByRole("complementary", { name: "Your answers" });
const prepRow = (page: Page) => page.locator('[data-check="Prep graded"]');
/** The one evidence item covers every check the readers could not settle: bowel prep and tissue taken. */
const tissueRow = (page: Page) => page.locator('[data-check="Tissue taken"]');
const disputedRows = (page: Page) => [prepRow(page), tissueRow(page)];
const signedNodes = (page: Page) => rail(page).locator('li[data-kind="human"]');
const previewNodes = (page: Page) => rail(page).locator('li[data-kind="current"]');

test.describe("review, fixtures mode", () => {
  test.skip(LIVE, "fixtures mode only");

  test("the queue is rebuilt from the review_hold case and says so", async ({ page }) => {
    await page.goto("/app/review");
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Review");
    await expect(page.getByText("Simulated queue: rebuilt from a held demo case")).toBeVisible();
    await expect(page.getByText(/The sample run has no open review items/)).toContainText("2 review items");
    const row = page.getByRole("link", { name: /^Mariam Al Shamsi/ });
    await expect(row).toContainText("Identity · Colonoscopy report");
    await expect(row).toContainText("SKM-dr-217");
    await page.screenshot({ path: "test-results/screens/review-queue.png", fullPage: true });
    await noSeriousAxe(page);
    await row.click();
    await expect(page).toHaveURL(/\/app\/review\/ahs-mushrif(%2F|\/)ServiceRequest(%2F|\/)AHS-sr-1079$/);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Mariam Al Shamsi");
  });

  test("the patient's avatar opens her record from the queue and the desk", async ({ page }) => {
    const record = /\/app\/patients\/ahs-mushrif(%2F|\/)ServiceRequest(%2F|\/)AHS-sr-1079$/;
    await page.goto("/app/review");
    await page.getByRole("link", { name: "Open Mariam Al Shamsi's record" }).click();
    await expect(page).toHaveURL(record);
    await page.goto(desk);
    await page.getByRole("link", { name: "Open Mariam Al Shamsi's record" }).click();
    await expect(page).toHaveURL(record);
  });

  test("the desk shows both records, both readers and the quote", async ({ page }) => {
    await page.goto(desk);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Mariam Al Shamsi");
    await expect(page.getByText("Case 1 of 1 waiting")).toBeVisible();
    await expect(page.getByText("Held: no second identifier is confirmed for the two records, and the two readers could not settle the bowel prep grade and whether tissue was taken.")).toBeVisible();
    await expect(page.getByText("not recorded", { exact: true })).toHaveCount(1);
    await expect(page.getByText("1965-03-14", { exact: true })).toBeVisible();
    await expect(page.getByText("Date of birth is not recorded on one or both records.")).toBeVisible();
    await expect(page.locator("mark")).toHaveText("the level of the cecum");
    await expect(prepRow(page)).toContainText("not stated");
    await expect(prepRow(page)).toContainText("reviewers decide");
    // Tissue: the rules reader found none, the second reader found it. Histology agrees (none).
    await expect(tissueRow(page)).toContainText("none");
    await expect(tissueRow(page)).toContainText("yes");
    await expect(tissueRow(page)).toContainText("reviewers decide");
    await expect(page.locator('[data-check="Histology back"]')).toContainText("agreed, none");
    await expect(page.locator('[data-check="Caecum reached"]')).toContainText("agreed");

    const why = page.getByRole("button", { name: "Why it was held" });
    await why.click();
    await expect(page.getByRole("button", { name: "Hide why" })).toHaveAttribute("aria-expanded", "true");
    await expect(page.getByRole("img", { name: /Link score 1\.000/ })).toBeVisible();
    await expect(page.getByText(/second identifier \(exact non-placeholder DOB, or a matching Emirates ID with agreeing name, sex and DOB\) or human confirmation/)).toBeVisible();
    await page.screenshot({ path: "test-results/screens/review-desk.png", fullPage: true });
    await noSeriousAxe(page);
  });

  test("identity first, then the report; Close the case previews the outcome and says nothing was saved", async ({ page }) => {
    await page.goto(desk);
    const r = rail(page);
    await expect(r.getByText("Answer both questions to sign.")).toBeVisible();
    await expect(r.getByRole("group", { name: /colonoscopy report/ })).toHaveCount(0);
    await expect(signedNodes(page)).toHaveCount(0);
    await expect(r.locator('li[data-kind="current"]')).toContainText("your answers");
    await expect(r).toContainText("nothing here is appended or saved");
    const bridge = page.getByRole("region", { name: "Is this the same person?" });
    await expect(bridge.getByRole("img", { name: "Not yet confirmed as this patient" })).toHaveAttribute("data-state", "unknown");

    await r.getByRole("radio", { name: "Same patient", exact: true }).click();
    await expect(bridge.getByRole("img", { name: "Same patient as Mariam Al Shamsi" })).toHaveAttribute("data-state", "resolving");
    await expect(r.getByRole("radio", { name: "Same patient", exact: true })).toBeChecked();
    await expect(r.getByText("Answer the colonoscopy report question to sign.")).toBeVisible();
    // The one evidence question names every check it covers.
    const q2 = r.getByRole("group", { name: "2. Is the colonoscopy report adequate?" });
    await expect(q2).toContainText("neither reader found the bowel prep grade");
    await expect(q2).toContainText("the readers differ on whether tissue was taken: rules reader none, second reader yes");
    await expect(q2).toContainText("One answer covers all of them");
    await expect(r.getByRole("button", { name: /Close the case/ })).toHaveCount(0);

    await r.getByRole("radio", { name: "Adequate", exact: true }).click();
    await r.getByRole("button", { name: "Preview: Close the case" }).click();

    const result = r.getByRole("status");
    await expect(result).toContainText("Would close the case");
    await expect(result).not.toContainText("Closed with");
    await expect(result.getByTestId("signature-squares")).toHaveCount(0);
    await expect(result.getByText("Reconstruction: nothing was saved")).toBeVisible();
    await expect(result).toContainText("No event was appended and no hash exists");
    await expect(signedNodes(page)).toHaveCount(0);
    await expect(previewNodes(page).nth(0)).toContainText("preview, not saved: same patient (as reviewer)");
    await expect(previewNodes(page).nth(1)).toContainText("preview, not saved: report adequate (as clinician)");
    for (const row of disputedRows(page)) await expect(row).toContainText("adequate (preview, not saved)");
    await expect(result.getByRole("link", { name: "Back to Review" })).toHaveAttribute("href", "/app/review");
    await expect(result.getByText("That was the last case waiting.")).toBeVisible();
    await page.screenshot({ path: "test-results/screens/review-closed.png", fullPage: true });
    await noSeriousAxe(page);

    await r.getByRole("button", { name: "Undo" }).click();
    await expect(r.getByText("Answer both questions to sign.")).toBeVisible();
    await expect(previewNodes(page)).toHaveCount(1);
    for (const row of disputedRows(page)) await expect(row).toContainText("reviewers decide");
  });

  test("keyboard only: arrows choose, the second question is announced, Close the case moves focus on", async ({ page }) => {
    await page.goto(desk);
    const r = rail(page);
    const active = () =>
      page.evaluate(() => {
        const el = document.activeElement as HTMLElement | null;
        return { tag: el?.tagName, text: (el?.textContent ?? "").trim(), label: el?.closest("label")?.textContent?.trim() ?? "" };
      });
    // Each group is named by its visible question.
    await expect(r.getByRole("group", { name: "1. Is it the same patient?" })).toBeVisible();

    await r.getByRole("radio", { name: "Same patient", exact: true }).focus();
    expect((await active()).label).toBe("Same patient");
    await page.keyboard.press("ArrowDown");
    expect((await active()).label).toBe("Not the same patient");
    await expect(r.getByRole("radio", { name: "Not the same patient" })).toBeChecked();
    await page.keyboard.press("ArrowUp");
    await expect(r.getByRole("radio", { name: "Same patient", exact: true })).toBeChecked();

    // The second question is announced in a polite live region, and the next Tab lands in it.
    await expect(r.locator("[aria-live=polite]")).toHaveText(/Question 2 of 2 is now showing/);
    await expect(r.getByRole("group", { name: "2. Is the colonoscopy report adequate?" })).toBeVisible();
    await page.keyboard.press("Tab");
    expect((await active()).tag).toBe("INPUT");
    expect((await active()).label).toBe("Adequate");
    await page.keyboard.press("Space");
    await expect(r.getByRole("radio", { name: "Adequate", exact: true })).toBeChecked();
    await page.keyboard.press("Tab");
    expect((await active()).text).toBe("Preview: Close the case");

    await page.keyboard.press("Enter");
    const heading = r.getByRole("heading", { name: "Would close the case" });
    await expect(heading).toBeVisible();
    await expect(heading).toBeFocused();
    await page.keyboard.press("Tab");
    expect((await active()).text).toBe("Back to Review");
    await page.keyboard.press("Tab");
    expect((await active()).text).toBe("Undo");
    await page.keyboard.press("Enter");
    // Undo brings the questions back and focus goes to the first of them.
    await expect(r.getByRole("radio", { name: "Same patient", exact: true })).toBeFocused();
  });

  test("the other three answers name what they sign", async ({ page }) => {
    await page.goto(desk);
    const r = rail(page);

    await r.getByRole("radio", { name: "Same patient", exact: true }).click();
    await r.getByRole("radio", { name: "Not adequate" }).click();
    await r.getByRole("button", { name: "Preview: Sign: report not adequate" }).click();
    await expect(r.getByRole("status")).toContainText("Would not close: repeat colonoscopy");
    for (const row of disputedRows(page)) await expect(row).toContainText("not adequate (preview, not saved)");
    await r.getByRole("button", { name: "Undo" }).click();

    await r.getByRole("radio", { name: "Same patient", exact: true }).click();
    await r.getByRole("radio", { name: "Ask the unit" }).click();
    await r.getByRole("button", { name: "Preview: Sign and ask the unit to complete the report" }).click();
    await expect(r.getByRole("status")).toContainText("Would ask the unit");
    await expect(r.locator('li[data-kind="current"]').last()).toContainText("SKMC endoscopy");
    await r.getByRole("button", { name: "Undo" }).click();

    await r.getByRole("radio", { name: "Same patient", exact: true }).click();
    await r.getByRole("radio", { name: "Adequate", exact: true }).click();
    await r.getByRole("radio", { name: "Not the same patient" }).click();
    await expect(r.getByRole("group", { name: /colonoscopy report/ })).toHaveCount(0);
    await r.getByRole("button", { name: "Preview: Sign: not the same patient" }).click();
    await expect(r.getByRole("status")).toContainText("Would reject the link");
    for (const row of disputedRows(page)) await expect(row).toContainText("not applied");
    await expect(signedNodes(page)).toHaveCount(0);
  });
});
