import AxeBuilder from "@axe-core/playwright";
import { expect as baseExpect, test, type Page } from "@playwright/test";

const expect = baseExpect.configure({ timeout: 20_000 });
// A missing name or case must fail within seconds, not after the 2-minute test budget: actions (click, fill) get the same 20s as expect.
test.describe.configure({ timeout: 60_000 });
test.use({ actionTimeout: 20_000, navigationTimeout: 30_000 });

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

    await page.goto("/app/patients?consent=denied");
    await expect(showing(page)).toHaveText("9 cases: consent denied");

    await page.goto("/app/patients?review=signed");
    await expect(showing(page)).toHaveText("31 cases: review signed");

    await page.goto("/app/patients?owner=none");
    await expect(showing(page)).toHaveText("136 cases: no owner");
    // The month's case files carry the owner on every folder.
    await page.getByRole("link", { name: /^Month’s files for / }).click();
    await expect(page.locator("[data-pf-case]").first()).toContainText("No owner");

    await page.goto("/app/patients?owner=Practitioner%2Fahs-mushrif-nurse-4");
    await expect(showing(page)).toHaveText("9 cases: owner Nurse 4");
  });

  test("filters compose with each other and with the chips and search", async ({ page }) => {
    await page.goto("/app/patients?programme=crc&owner=none&consent=granted");
    await expect(showing(page)).toContainText("colorectal screening · consent granted · no owner");
    await page.getByRole("navigation", { name: "Filters" }).getByRole("link", { name: "Overdue" }).click();
    await expect(page).toHaveURL(/programme=crc/);
    await expect(page).toHaveURL(/owner=none/);
    await expect(page).toHaveURL(/consent=granted/);
    await expect(page).toHaveURL(/overdue=true/);
    // Search keeps them too.
    await page.getByRole("searchbox", { name: "Find a case, task or owner" }).fill("10");
    await page.getByRole("button", { name: "Search", exact: true }).click();
    await expect(page).toHaveURL(/owner=none/);
    await expect(page).toHaveURL(/q=10/);
  });

  test("the filter menus list what the data holds, with counts that match the result", async ({ page }) => {
    await page.goto("/app/patients");
    const menu = (name: string) => page.getByRole("button", { name, exact: true });
    await menu("Programme").click();
    await expect(page.getByRole("listbox", { name: "Programme" }).getByRole("option")).toHaveCount(7);
    await page.keyboard.press("Escape");
    await expect(menu("Programme")).toBeFocused();
    await menu("Provider").click();
    const provider = page.getByRole("listbox", { name: "Provider" });
    await expect(provider.getByRole("option", { name: "Al Mushrif 51" })).toBeVisible();
    await expect(provider.getByRole("option", { name: "rad-imaging 25" })).toBeVisible();
    await page.keyboard.press("Escape");

    await menu("Owner").click();
    await page.getByRole("listbox", { name: "Owner" }).getByRole("option", { name: /^No owner / }).click();
    await expect(page).toHaveURL(/owner=none/);
    await expect(showing(page)).toHaveText("136 cases: no owner");
    await expect(menu("Owner: No owner")).toBeVisible();
    // Counts now reflect the other active filters: the programme choices split the 136 unowned cases.
    await menu("Programme").click();
    const counts = await page.getByRole("listbox", { name: "Programme" }).getByRole("option").evaluateAll((os) => os.map((o) => Number(o.lastElementChild!.textContent)));
    expect(counts.reduce((a, b) => a + b, 0)).toBe(136);
  });

  test("set filters show as chips that can be removed or cleared", async ({ page }) => {
    await page.goto("/app/patients?programme=lung_nodule&consent=granted&review=none");
    const filters = page.getByRole("group", { name: "Filter patients" });
    await expect(filters.getByRole("button", { name: "Programme: Lung nodule follow-up" })).toBeVisible();
    await expect(filters.getByRole("button", { name: "Consent: Granted" })).toBeVisible();
    await filters.getByRole("link", { name: "Remove Consent filter" }).click();
    await expect(page).not.toHaveURL(/consent=/);
    await expect(page).toHaveURL(/programme=lung_nodule/);
    await filters.getByRole("link", { name: "Clear all" }).click();
    await expect(page).toHaveURL(/\/app\/patients$/);
    await expect(showing(page)).toHaveText("177 cases: all cases");
    await expect(filters.getByRole("link", { name: "Clear all" })).toHaveCount(0);
  });

  test("an empty combination says so and a stale value is not silently dropped", async ({ page }) => {
    await page.goto("/app/patients?provider=no-such-unit");
    await expect(showing(page)).toHaveText("0 cases: provider no-such-unit");
    await expect(page.getByText("No cases match these filters. Try removing one.")).toBeVisible();
    await expect(page.getByRole("region", { name: "Referrals by date received" })).toHaveCount(0);
  });

  test("390px: no side scroll, 44px controls, accessible", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/app/patients?programme=crc&owner=none");
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    expect(overflow).toBeLessThanOrEqual(0);
    const small = await page.evaluate(() => {
      const out: string[] = [];
      const root = document.querySelector('[role=group][aria-label="Filter patients"]')!;
      for (const el of root.querySelectorAll("input, button, a")) {
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
