import { readFileSync } from "node:fs";
import path from "node:path";
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { ROUTES } from "./routes";

const meta = JSON.parse(
  readFileSync(path.join(process.cwd(), "data/api/dashboard.json"), "utf8"),
).meta as { run: string; loaded_at: string; reader_model: string };

const sidebar = (page: Page) => page.locator("aside#app-sidebar");
const toggle = (page: Page) =>
  page
    .locator("aside#app-sidebar")
    .getByRole("button", { name: /(Collapse|Expand) sidebar/ });

test.describe("sidebar (desktop)", () => {
  test.beforeEach(async ({ context }) => {
    await context.clearCookies();
  });

  test("collapses and expands by mouse and keyboard, and persists across reload", async ({
    page,
  }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto("/app");
    const aside = sidebar(page);
    await expect(toggle(page)).toHaveAttribute("aria-expanded", "true");
    await expect(toggle(page)).toHaveAttribute("aria-controls", "app-sidebar");
    expect((await aside.boundingBox())?.width).toBe(240);

    await toggle(page).click();
    await expect(toggle(page)).toHaveAccessibleName("Expand sidebar");
    await expect(toggle(page)).toHaveAttribute("aria-expanded", "false");
    await expect.poll(async () => (await aside.boundingBox())?.width).toBe(64);

    // Keyboard: focus the button, press Enter, then Space.
    await toggle(page).focus();
    await page.keyboard.press("Enter");
    await expect(toggle(page)).toHaveAttribute("aria-expanded", "true");
    await page.keyboard.press("Space");
    await expect(toggle(page)).toHaveAttribute("aria-expanded", "false");

    await page.reload();
    await expect(toggle(page)).toHaveAttribute("aria-expanded", "false");
    expect((await aside.boundingBox())?.width).toBe(64);

    await toggle(page).click();
    await page.reload();
    await expect(toggle(page)).toHaveAttribute("aria-expanded", "true");
  });

  test("the [ shortcut toggles, except while typing", async ({ page }) => {
    await page.goto("/app/patients");
    await expect(toggle(page)).toHaveAttribute("aria-expanded", "true");
    await page.keyboard.press("[");
    await expect(toggle(page)).toHaveAttribute("aria-expanded", "false");
    await page.evaluate(() => {
      const input = document.createElement("input");
      input.id = "probe";
      document.body.append(input);
      input.focus();
    });
    await page.keyboard.press("[");
    await expect(toggle(page)).toHaveAttribute("aria-expanded", "false");
  });

  test("collapsed links keep their names, show a tooltip on focus, and pass axe", async ({
    page,
  }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto("/app");
    await toggle(page).click();
    const nav = sidebar(page).getByRole("navigation", { name: "Main" });
    for (const name of [
      "Programme",
      "Today",
      "Review",
      "Patients",
      "Referrals",
      "Pipeline",
      "Results",
      "Agent review",
    ]) {
      await expect(nav.getByRole("link", { name, exact: name !== "Agent review" })).toBeVisible();
    }

    await nav.getByRole("link", { name: "Review", exact: true }).focus();
    await page.keyboard.press("Tab");
    await expect(nav.getByRole("link", { name: "Patients" })).toBeFocused();
    await expect(
      page
        .locator('[data-slot="tooltip-content"]:not([data-state="closed"])')
        .first(),
    ).toContainText("Patients");

    const axe = await new AxeBuilder({ page }).analyze();
    const serious = axe.violations.filter(
      (v) => v.impact === "serious" || v.impact === "critical",
    );
    expect(
      serious.map(
        (v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`,
      ),
    ).toEqual([]);
  });

  test("the metadata strip shows plain words first and keeps run internals in its technical panel, and adds no h1", async ({
    page,
  }) => {
    await page.goto("/app/pipeline");
    const strip = page.getByRole("region", { name: "Run metadata" });
    const summary = strip.locator("summary");
    // Collapsed: only the simulation label and a readable snapshot time are shown, no identifiers or ISO timestamps.
    await expect(strip.locator("time")).toBeVisible();
    expect(await summary.innerText()).toMatch(/^Simulated data\s*Snapshot \d{1,2} [A-Z][a-z]{2} \d{4}, \d{2}:\d{2} UTC\s*Details$/);
    await expect(strip.locator("time")).toHaveAttribute("datetime", meta.loaded_at);
    await expect(strip.getByText(meta.run)).toBeHidden();
    await expect(strip).not.toContainText("data_mode");
    await expect(summary).not.toContainText(/\d{4}-\d{2}-\d{2}T/);

    const chevron = summary.locator("svg");
    await expect(chevron).toBeVisible();
    // Tailwind's rotate-* sets the CSS `rotate` property, so the open state shows as 180deg there, not in `transform`.
    const turn = () => chevron.evaluate((el) => getComputedStyle(el).rotate);
    expect(await turn()).toBe("none");
    await strip.getByText("Details").click();
    await expect(strip.locator("details")).toHaveAttribute("open", "");
    await expect(summary).toContainText("Hide");
    await expect(strip.getByText(meta.run)).toBeVisible();
    await expect(strip).toContainText("Published snapshot of the API");
    await expect(strip).toContainText("Reader model");
    await expect(strip).toContainText(meta.reader_model);
    await expect.poll(turn).toBe("180deg");
    await expect(page.locator("h1")).toHaveCount(1);
  });
});

test.describe("mobile (390px)", () => {
  test.use({ viewport: { width: 390, height: 844 }, hasTouch: true });

  test("the menu opens, Escape closes it and returns focus, targets are 44px", async ({
    page,
  }) => {
    await page.goto("/app");
    const trigger = page.getByRole("button", { name: /Menu/ });
    await expect(trigger).toHaveAttribute("aria-expanded", "false");
    await trigger.focus();
    await page.keyboard.press("Enter");
    await expect(page.getByRole("button", { name: /Close/ })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
    const nav = page.getByRole("navigation", { name: "Main" });
    await expect(nav).toBeVisible();
    await expect(nav.getByRole("link").first()).toBeFocused();
    await expect(
      page.locator("#mobile-menu").getByText("Synthetic data · demo"),
    ).toBeVisible();
    await expect(
      page.locator("#mobile-menu").getByText("Dr Salama Al Tamimi"),
    ).toBeVisible();

    for (const l of await nav.getByRole("link").all()) {
      expect((await l.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    }
    expect(
      (await page.getByRole("button", { name: /Close/ }).boundingBox())!.height,
    ).toBeGreaterThanOrEqual(44);

    await page.keyboard.press("Escape");
    await expect(nav).toBeHidden();
    await expect(page.getByRole("button", { name: /Menu/ })).toBeFocused();
    await expect(page.getByRole("button", { name: /Menu/ })).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  });

  test("tapping outside closes it, and navigating closes it", async ({
    page,
  }) => {
    await page.goto("/app");
    const nav = page.getByRole("navigation", { name: "Main" });
    await page.getByRole("button", { name: /Menu/ }).click();
    await expect(nav).toBeVisible();
    await page.mouse.click(200, 800);
    await expect(nav).toBeHidden();

    await page.getByRole("button", { name: /Menu/ }).click();
    await nav.getByRole("link", { name: "Pipeline" }).click();
    await expect(page).toHaveURL(/\/app\/pipeline$/);
    await expect(nav).toBeHidden();
  });

  test("/app has no horizontal overflow", async ({ page }) => {
    await page.goto("/app");
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBeLessThanOrEqual(390);
  });

  test("the run strip fits, shows its time in full, and opens by touch and keyboard", async ({ page }) => {
    await page.goto("/app");
    const strip = page.getByRole("region", { name: "Run metadata" });
    const summary = strip.locator("summary");
    const details = strip.locator("details");
    expect((await summary.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    const time = strip.locator("time");
    await expect(time).toBeVisible();
    expect(await time.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
    // The time wraps inside the 44px row, so the strip keeps the height its loading placeholder reserves.
    expect((await summary.boundingBox())!.height).toBe(44);
    await expect(summary.locator("svg")).toBeVisible();

    await summary.tap();
    await expect(details).toHaveAttribute("open", "");
    await expect(strip.getByText(meta.run)).toBeVisible();
    await summary.focus();
    await page.keyboard.press("Enter");
    await expect(details).not.toHaveAttribute("open", "");
    await page.keyboard.press("Space");
    await expect(details).toHaveAttribute("open", "");
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  });
});

// /patient/* is a separate surface (no shared layout), so the shortcut is checked on the site and the dashboard.
const SHORTCUT_ROUTES = ROUTES.filter((r) => !r.startsWith("/patient"));
for (const vp of [
  { w: 1440, h: 900 },
  { w: 390, h: 844 },
]) {
  test.describe(`skip link at ${vp.w}px`, () => {
    test.use({ viewport: { width: vp.w, height: vp.h } });
    for (const route of SHORTCUT_ROUTES) {
      test(`${route}: first Tab is "Skip to main content", Enter focuses main#main, next Tab is inside it`, async ({ page }) => {
        await page.emulateMedia({ reducedMotion: "reduce" });
        await page.goto(route);
        await expect(page.locator("main#main")).toHaveCount(1);
        await page.keyboard.press("Tab");
        const skip = page.getByRole("link", { name: "Skip to main content" });
        await expect(skip).toBeFocused();
        await expect(skip).toBeVisible();
        await page.keyboard.press("Enter");
        await expect(page.locator("main#main")).toBeFocused();
        await page.keyboard.press("Tab");
        expect(await page.evaluate(() => document.querySelector("main#main")!.contains(document.activeElement) && document.activeElement !== document.querySelector("main#main"))).toBe(true);
      });
    }
  });
}

test.describe("unknown URLs", () => {
  for (const url of ["/nope", "/app/nope", "/app/nope/deeper"]) {
    test(`${url} is a 404 with one h1 in main, recovery links and no serious axe issues`, async ({ page }) => {
      const res = await page.goto(url);
      expect(res?.status()).toBe(404);
      await expect(page.locator("main#main")).toHaveCount(1);
      await expect(page.locator("h1")).toHaveCount(1);
      await expect(page.locator("main#main h1")).toHaveText("This page could not be found");
      const main = page.locator("main#main");
      const home = main.getByRole("link", { name: "Home", exact: true });
      const dash = main.getByRole("link", { name: "Dashboard", exact: true });
      await expect(home).toHaveAttribute("href", "/");
      await expect(dash).toHaveAttribute("href", "/app");
      // Both are plain links in the tab order, whichever comes first on the page.
      const [first, second] = (await main.getByRole("link").all()).slice(-2);
      await first.focus();
      await page.keyboard.press("Tab");
      await expect(second).toBeFocused();
      const axe = await new AxeBuilder({ page }).analyze();
      const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
      expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
    });
  }

  test("/app/nope keeps the app shell", async ({ page }) => {
    await page.goto("/app/nope");
    await expect(page.locator("aside#app-sidebar")).toBeVisible();
  });
});

test("the nav lists Agent review with a text Experimental marker, and the brand subtitle is not CRC-only", async ({ page }) => {
  await page.goto("/app");
  const link = sidebar(page).getByRole("link", { name: /Agent review/ });
  await expect(link).toHaveAttribute("href", "/app/agent-review");
  await expect(link).toContainText("Experimental");
  await expect(sidebar(page)).toContainText("ADPHC · screening follow-up");
  await expect(sidebar(page)).not.toContainText("CRC programme");
});

test("the run strip says Agent review is a separate run, and is unchanged elsewhere", async ({ page }) => {
  await page.goto("/app/agent-review");
  const strip = page.getByRole("region", { name: "Run metadata" });
  await expect(strip).toHaveCount(1);
  await expect(strip).toContainText("separate experimental run");
  await expect(strip).not.toContainText(meta.run);
  await page.goto("/app/pipeline");
  await expect(page.getByRole("region", { name: "Run metadata" })).toContainText("Simulated data");
  await expect(page.getByRole("region", { name: "Run metadata" })).toContainText(meta.run);
});

// Landmarks (issue #20): the desktop aside's nav is "Main" and the mobile menu's is "Main (mobile)", so the DOM never holds
// two navs with one name; only one of them is in the accessibility tree at a time (the other is display:none).
const APP_ROUTES = ROUTES.filter((r) => r === "/app" || r.startsWith("/app/"));
const navNames = async (page: Page) => {
  const snap = await page.locator("body").ariaSnapshot();
  return [...snap.matchAll(/^\s*- navigation(?: "([^"]*)")?/gm)].map((m) => m[1] ?? "");
};
const main = (page: Page, name: "Main" | "Main (mobile)" = "Main") => page.getByRole("navigation", { name, exact: true });
const FOCUS_TEXT = ["Skip to main content", "WaslADPHC · screening follow-up", "Programme", "Today", "Recovery", "Review", "Patients", "Referrals", "Pipeline", "Results", "Agent reviewExperimental"];

const tabStop = (page: Page) =>
  page.evaluate(() => {
    const el = document.activeElement as HTMLElement;
    const cs = getComputedStyle(el);
    return { text: (el.textContent ?? "").replace(/\s+/g, " ").trim(), outline: cs.outlineStyle, width: cs.outlineWidth };
  });

async function expectOneMain(page: Page, name: "Main" | "Main (mobile)" = "Main") {
  await expect(main(page, name)).toHaveCount(1);
  await expect(main(page, name)).toBeVisible();
  const names = await navNames(page);
  expect(names.filter((n) => n.startsWith("Main"))).toEqual([name]);
  expect(await page.locator('nav[aria-label="Main"]').count(), "one DOM nav named Main").toBe(1);
  expect(new Set(names).size, `landmark names must be unique: ${names.join(" | ")}`).toBe(names.length);
  expect(names.every(Boolean), "every navigation landmark is named").toBe(true);
}

for (const route of APP_ROUTES) {
  test.describe(`landmarks ${route}`, () => {
    test("1440px: one Main navigation, unique names, desktop focus order and rings", async ({ page }) => {
      await page.setViewportSize({ width: 1440, height: 900 });
      await page.goto(route);
      await expectOneMain(page);
      await expect(sidebar(page).getByRole("navigation", { name: "Main", exact: true })).toBeVisible();
      // The header (and the mobile menu inside it) is display:none, so it is not in the tab order either.
      await expect(page.locator("header #mobile-menu")).toBeHidden();
      const seen: string[] = [];
      for (let i = 0; i < FOCUS_TEXT.length; i++) {
        await page.keyboard.press("Tab");
        const f = await tabStop(page);
        seen.push(f.text);
        expect(f.outline, `focus ring on "${f.text}"`).not.toBe("none");
        expect(f.width).toBe("2px");
      }
      expect(seen).toEqual(FOCUS_TEXT);
    });

    test("390px: one Main navigation with the menu closed and open, unique names, focus order and rings", async ({ page }) => {
      await page.setViewportSize({ width: 390, height: 844 });
      await page.goto(route);
      // Closed: the desktop aside and the closed menu panel are both out of the accessibility tree.
      await expect(page.locator("aside#app-sidebar")).toBeHidden();
      await expect(main(page)).toHaveCount(0);
      expect((await navNames(page)).filter((n) => n.startsWith("Main"))).toHaveLength(0);
      const closed: string[] = [];
      for (let i = 0; i < 3; i++) {
        await page.keyboard.press("Tab");
        const f = await tabStop(page);
        closed.push(f.text);
        expect(f.outline).not.toBe("none");
        expect(f.width).toBe("2px");
      }
      expect(closed).toEqual(["Skip to main content", "WaslADPHC · screening follow-up", "Menu navigation"]);

      await page.getByRole("button", { name: /Menu/ }).click();
      await expectOneMain(page, "Main (mobile)");
      await expect(page.locator("#mobile-menu").getByRole("navigation", { name: "Main (mobile)", exact: true })).toBeVisible();
      await expect(page.locator("aside#app-sidebar")).toBeHidden();
      // Opening the menu moves focus to its first link, and Tab continues down the menu in order.
      await expect(main(page, "Main (mobile)").getByRole("link").first()).toBeFocused();
      await page.keyboard.press("Tab");
      await expect(main(page, "Main (mobile)").getByRole("link").nth(1)).toBeFocused();
    });
  });
}
