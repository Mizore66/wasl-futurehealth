import AxeBuilder from "@axe-core/playwright";
import { expect as baseExpect, test, type Page } from "@playwright/test";

const expect = baseExpect.configure({ timeout: 20_000 });
test.describe.configure({ timeout: 120_000 });

const shot = (page: Page, name: string) => page.screenshot({ path: `test-results/screens/patients-${name}.png`, fullPage: true });

async function noSeriousAxe(page: Page) {
  const axe = await new AxeBuilder({ page }).analyze();
  const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

const showing = (page: Page) => page.getByTestId("showing");

test.describe("Patients: programme, provider, consent, review and owner filters", () => {
  test("each filter is read from the URL, never ignored, and says what is in view", async ({ page }) => {
    await page.goto("/app/patients?provider=phc-retina");
    await expect(showing(page)).toHaveText("26 cases: provider phc-retina");
    await expect(page.getByTestId("programmes-in-view")).toHaveText("Programme in view: Diabetic eye screening (26).");
    await expect(page.getByTestId("count-basis")).toContainText("published sample of 128 cases");

    await page.goto("/app/patients?consent=denied");
    await expect(showing(page)).toHaveText("6 cases: consent denied");

    await page.goto("/app/patients?review=signed");
    await expect(showing(page)).toHaveText("29 cases: review signed");

    await page.goto("/app/patients?owner=none");
    await expect(showing(page)).toHaveText("86 cases: no owner");
    await expect(page.locator("tbody tr").first()).toContainText("No owner");

    await page.goto("/app/patients?owner=Practitioner%2Fahs-mushrif-nurse-4");
    await expect(showing(page)).toHaveText("10 cases: owner Nurse 4");
  });

  test("filters compose with each other and with the chips and search", async ({ page }) => {
    await page.goto("/app/patients?programme=crc&owner=none&consent=granted");
    await expect(showing(page)).toContainText("colorectal screening · consent granted · no owner");
    await expect(page.getByTestId("programmes-in-view")).toContainText("Colorectal screening");
    await page.getByRole("navigation", { name: "Filters" }).getByRole("link", { name: "Overdue" }).click();
    await expect(page).toHaveURL(/programme=crc/);
    await expect(page).toHaveURL(/owner=none/);
    await expect(page).toHaveURL(/consent=granted/);
    await expect(page).toHaveURL(/overdue=true/);
    // Search keeps them too.
    await page.getByRole("searchbox", { name: "Search case id" }).fill("10");
    await page.getByRole("button", { name: "Search" }).click();
    await expect(page).toHaveURL(/owner=none/);
    await expect(page).toHaveURL(/q=10/);
  });

  test("the selects list what the data holds, with counts that match the result", async ({ page }) => {
    await page.goto("/app/patients");
    await page.getByText("Filter by programme, provider, consent, review or owner").click();
    const programme = page.getByRole("combobox", { name: /^Programme/ });
    await expect(programme.locator("option")).toHaveCount(6);
    await expect(programme.locator("option").first()).toHaveText("All programmes (128)");
    const provider = page.getByRole("combobox", { name: /^Provider/ });
    await expect(provider).toContainText("Al Mushrif (52)");
    await expect(provider).toContainText("rad-imaging (25)");

    await page.getByRole("combobox", { name: /^Owner/ }).selectOption("none");
    await page.getByRole("button", { name: "Apply filters" }).click();
    await expect(page).toHaveURL(/owner=none/);
    await expect(showing(page)).toHaveText("86 cases: no owner");
    // Counts now reflect the other active filters: the programme choices split the 86 unowned cases.
    await expect(page.getByRole("combobox", { name: /^Programme/ })).toContainText("All programmes (86)");
  });

  test("active filters are summarised and can be removed or cleared", async ({ page }) => {
    await page.goto("/app/patients?programme=lung_nodule&consent=granted&review=none");
    const summary = page.getByTestId("active-filters");
    await expect(summary).toContainText("Lung nodule follow-up");
    await expect(summary).toContainText("Consent: Granted");
    await summary.getByRole("link", { name: /Consent: Granted/ }).click();
    await expect(page).not.toHaveURL(/consent=/);
    await expect(page).toHaveURL(/programme=lung_nodule/);
    await page.getByRole("link", { name: "Clear these filters" }).click();
    await expect(page).toHaveURL(/\/app\/patients$/);
    await expect(showing(page)).toHaveText("128 cases: all cases");
    await expect(page.getByTestId("active-filters")).toHaveCount(0);
  });

  test("an empty combination says so and a stale value is not silently dropped", async ({ page }) => {
    await page.goto("/app/patients?provider=no-such-unit");
    await expect(showing(page)).toHaveText("0 cases: provider no-such-unit");
    await expect(page.getByText("No cases match these filters. Try removing one.")).toBeVisible();
    await expect(page.getByTestId("range")).toHaveText("Showing 0–0 of 0");
    await expect(page.getByTestId("programmes-in-view")).toHaveText("No programme in view.");
  });

  test("390px: no side scroll, 44px controls, accessible", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/app/patients?programme=crc&owner=none");
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    expect(overflow).toBeLessThanOrEqual(0);
    const small = await page.evaluate(() => {
      const out: string[] = [];
      const root = document.querySelector("[data-testid=refine]")!;
      for (const el of root.querySelectorAll("summary, select, button, a")) {
        const r = el.getBoundingClientRect();
        if (r.width > 0 && r.height < 43.5) out.push(`${el.tagName} ${el.textContent?.slice(0, 30)} ${Math.round(r.height)}`);
      }
      return out;
    });
    expect(small).toEqual([]);
    await shot(page, "filters-390");
    await noSeriousAxe(page);
    await page.setViewportSize({ width: 1440, height: 900 });
    await shot(page, "filters-1440");
    await noSeriousAxe(page);
  });
});
