import { expect as baseExpect, test, type Locator, type Page } from "@playwright/test";

/*
 * Today's programme/provider selection (issue #114), fixture mode. Each choice is a filter menu that opens a URL, so it
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
/** A filter chip in the page header: "Programme" when unset, "Programme: Colorectal screening" when set. */
const chip = (page: Page, label: "Programme" | "Provider") => scope(page).getByRole("button", { name: new RegExp(`^${label}`) });
const menu = (page: Page, label: "Programme" | "Provider") => page.getByRole("listbox", { name: label });
/** An option's name is its label then its count. */
const option = (page: Page, label: "Programme" | "Provider", name: string) =>
  menu(page, label).getByRole("option", { name: new RegExp(`^${name.replace(/[()]/g, "\\$&")} \\d+$`) });
async function choose(page: Page, label: "Programme" | "Provider", name: string) {
  await chip(page, label).click();
  await option(page, label, name).click();
}
const list = (page: Page) => page.getByTestId("today-sheet");

/** Total of the three queues (To call, Waiting, No owner) the sidebar counts for this board = the queue the selection produced. */
async function queueSize(page: Page): Promise<number> {
  const nav = page.getByRole("navigation", { name: "Main", exact: true });
  let n = 0;
  for (const name of [/^To call/, /^Waiting/, /^No owner/]) n += Number((await nav.getByRole("link", { name }).innerText()).match(/(\d+)\s*$/)?.[1] ?? NaN);
  return n;
}

test.describe("Today programme and provider selection", () => {
  test("opens on CRC at Al Mushrif by default", async ({ page }) => {
    await page.goto("/app/today");
    await expect(chip(page, "Programme")).toHaveText("Programme: Colorectal screening");
    await expect(chip(page, "Provider")).toHaveText("Provider: Al Mushrif");
    await expect(page.getByText(/Colorectal screening · Al Mushrif · run day \d+/)).toBeVisible();
    await expect(list(page).getByTestId("row-programme")).toHaveCount(0);
  });

  test("every supported programme can be selected; the queue and its count match the choice", async ({ page }) => {
    for (const [value, name] of Object.entries(PROGRAMMES)) {
      await page.goto(`/app/today?programme=${value}&provider=`);
      await expect(chip(page, "Programme")).toHaveText(`Programme: ${name}`);
      await expect(chip(page, "Provider")).toHaveText("Provider");
      await expect(page.getByText(new RegExp(`${name.replace(/[()]/g, "\\$&")} · all providers · run day`))).toBeVisible();
      await chip(page, "Programme").click();
      const offered = Number((await menu(page, "Programme").getByRole("option", { selected: true }).innerText()).match(/(\d+)\s*$/)?.[1]);
      await page.keyboard.press("Escape");
      expect(offered, `${value} is offered with an open-case count`).toBeGreaterThan(0);
      await expect.poll(() => queueSize(page), { message: `${value}: queue size equals the offered count` }).toBe(offered);
    }
  });

  test("the menus put the choice in the URL; reload and history keep it", async ({ page }) => {
    await page.goto("/app/today");
    await scope(page).getByRole("link", { name: "Remove Provider filter" }).click();
    await expect(page).toHaveURL(/programme=crc&provider=&/);
    await expect(chip(page, "Provider")).toHaveText("Provider");
    await choose(page, "Programme", "Lung nodule follow-up");
    await expect(page).toHaveURL(/programme=lung_nodule&provider=&/);
    await expect(page.getByText(/Lung nodule follow-up · all providers/)).toBeVisible();
    await expect(list(page)).toBeVisible();
    // A sidebar queue link keeps the chosen scope.
    await page.getByRole("navigation", { name: "Main", exact: true }).getByRole("link", { name: /^Waiting/ }).click();
    await expect(page).toHaveURL(/programme=lung_nodule&provider=&queue=waiting/);
    await expect(page.getByText(/Lung nodule follow-up · all providers/)).toBeVisible();

    await page.reload();
    await expect(chip(page, "Programme")).toHaveText("Programme: Lung nodule follow-up");
    await expect(page.getByText(/Lung nodule follow-up · all providers/)).toBeVisible();

    await page.goto("/app/patients");
    await page.goBack();
    await expect(page).toHaveURL(/programme=lung_nodule/);
    await expect(chip(page, "Programme")).toHaveText("Programme: Lung nodule follow-up");
  });

  test("a new scope keeps the queue the sidebar switched to in the browser", async ({ page }) => {
    await page.goto("/app/today");
    // The sidebar switches queue without a server render, so the menus must follow the live URL, not the rendered one.
    await page.getByRole("navigation", { name: "Main", exact: true }).getByRole("link", { name: /^Waiting/ }).click();
    await expect(page).toHaveURL(/queue=waiting/);
    await choose(page, "Programme", "Cervical screening");
    await expect(page).toHaveURL(/programme=cervical&provider=ahs-mushrif&queue=waiting/);
    await expect(page.getByRole("heading", { level: 2, name: /^Waiting/ })).toBeVisible();
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
    await expect.poll(() => queueSize(page)).toBe(0);
  });
});

test.describe("Today scope menu is usable", () => {
  for (const [label, viewport] of [["desktop", { width: 1440, height: 900 }], ["phone", { width: 390, height: 844 }]] as const) {
    test(`its controls are not clipped by the header and a new scope keeps the queue (${label})`, async ({ page }) => {
      await page.setViewportSize(viewport);
      await page.goto("/app/today?queue=waiting");
      // Every control is the element a click at its centre reaches (nothing covers or clips it): each chip, then an
      // option in the open menu.
      const reached = async (control: Locator) => {
        await control.scrollIntoViewIfNeeded();
        return control.evaluate((el) => {
          const r = el.getBoundingClientRect();
          const hit = document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2);
          return hit === el || el.contains(hit);
        });
      };
      for (const control of [chip(page, "Programme"), chip(page, "Provider")]) expect(await reached(control)).toBe(true);
      await chip(page, "Programme").click();
      expect(await reached(option(page, "Programme", "Cervical screening"))).toBe(true);
      await option(page, "Programme", "Cervical screening").click();
      await expect(page).toHaveURL(/programme=cervical/);
      await expect(page).toHaveURL(/queue=waiting/);
    });
  }
});
