import { expect as baseExpect, test, type Page } from "@playwright/test";

/*
 * Today's programme/provider selection (issue #114), fixture mode. The choice is a GET form, so it lives in the URL,
 * survives reload and history, and the queue shows exactly the selected open cases. CRC at AHS Mushrif stays the default.
 */
const expect = baseExpect.configure({ timeout: 20_000 });
test.describe.configure({ timeout: 60_000 });
test.use({ actionTimeout: 20_000, navigationTimeout: 30_000 });

const PROGRAMMES = {
  crc: "Colorectal screening",
  cervical: "Cervical screening",
  lung_nodule: "Lung nodule follow-up",
  diabetic_retinopathy: "Diabetic eye screening",
  breast: "Breast screening",
  lung_ldct: "Lung cancer screening (LDCT)",
  ifhas: "IFHAS cardiometabolic follow-up",
} as const;

const scope = (page: Page) => page.getByTestId("today-scope");
const programmeSelect = (page: Page) => scope(page).getByLabel("Programme");
const providerSelect = (page: Page) => scope(page).getByLabel("Provider");
const list = (page: Page) => page.getByRole("region", { name: "Your list" });

/** Total of the three saved views (To call, Waiting, No owner) = the queue the selection produced. */
async function queueSize(page: Page): Promise<number> {
  let n = 0;
  for (const tab of await list(page).getByRole("tab").all()) n += Number((await tab.locator("span").innerText()).trim());
  return n;
}

test.describe("Today programme and provider selection", () => {
  test("opens on CRC at Al Mushrif by default", async ({ page }) => {
    await page.goto("/app/today");
    await expect(programmeSelect(page)).toHaveValue("crc");
    await expect(providerSelect(page)).toHaveValue("ahs-mushrif");
    await expect(page.getByText(/Colorectal screening · Al Mushrif · run day \d+/)).toBeVisible();
    await expect(list(page).getByTestId("row-programme")).toHaveCount(0);
  });

  test("every supported programme can be selected; the queue and its count match the choice", async ({ page }) => {
    for (const [value, name] of Object.entries(PROGRAMMES)) {
      await page.goto(`/app/today?programme=${value}&provider=`);
      await expect(programmeSelect(page)).toHaveValue(value);
      await expect(providerSelect(page)).toHaveValue("");
      await expect(page.getByText(new RegExp(`${name.replace(/[()]/g, "\\$&")} · all providers · run day`))).toBeVisible();
      const option = programmeSelect(page).locator(`option[value="${value}"]`);
      const offered = Number((await option.innerText()).match(/\((\d+)\)$/)?.[1]);
      expect(offered, `${value} is offered with an open-case count`).toBeGreaterThan(0);
      expect(await queueSize(page), `${value}: queue size equals the offered count`).toBe(offered);
    }
  });

  test("the form puts the choice in the URL; reload and history keep it", async ({ page }) => {
    await page.goto("/app/today");
    await programmeSelect(page).selectOption("lung_nodule");
    await providerSelect(page).selectOption("");
    await scope(page).getByRole("button", { name: "Show queue" }).click();
    await expect(page).toHaveURL(/programme=lung_nodule/);
    await expect(page.getByText(/Lung nodule follow-up · all providers/)).toBeVisible();
    const open = page.getByRole("region", { name: "Open patient" });
    await expect(open.getByRole("heading", { level: 2 })).toBeVisible();

    await page.reload();
    await expect(programmeSelect(page)).toHaveValue("lung_nodule");
    await expect(page.getByText(/Lung nodule follow-up · all providers/)).toBeVisible();

    await page.goto("/app/patients");
    await page.goBack();
    await expect(page).toHaveURL(/programme=lung_nodule/);
    await expect(programmeSelect(page)).toHaveValue("lung_nodule");
  });

  test("a mixed queue labels each row with its programme and provider", async ({ page }) => {
    await page.goto("/app/today?programme=&provider=");
    await expect(page.getByText(/All programmes · all providers/)).toBeVisible();
    const labels = await list(page).getByTestId("row-programme").allInnerTexts();
    expect(labels.length).toBeGreaterThan(0);
    const names = new Set(labels.map((l) => l.split(" · ")[0]));
    expect(names.size, `several programmes in view: ${[...names].join(", ")}`).toBeGreaterThan(1);
    for (const n of names) expect(Object.values(PROGRAMMES) as string[]).toContain(n);
  });

  test("a selection with no open cases says so honestly", async ({ page }) => {
    await page.goto("/app/today?programme=breast&provider=ahs-mushrif");
    await expect(page.getByTestId("today-scope-empty")).toHaveText(
      "No open cases for Breast screening · Al Mushrif. Choose another programme or provider.",
    );
    expect(await queueSize(page)).toBe(0);
  });
});
