import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

const ASSETS = [
  ["/favicon.ico", /image\/(x-icon|vnd\.microsoft\.icon)/],
  ["/icon.svg", /image\/svg\+xml/],
  ["/apple-icon.png", /image\/png/],
  ["/manifest.webmanifest", /application\/manifest\+json/],
  ["/opengraph-image", /image\/png/],
  ["/twitter-image", /image\/png/],
  ["/icon-192.png", /image\/png/],
  ["/icon-512.png", /image\/png/],
  ["/icon-maskable-512.png", /image\/png/],
] as const;

for (const [path, type] of ASSETS) {
  test(`${path} is served as ${type.source}`, async ({ request }) => {
    const res = await request.get(path);
    expect(res.status()).toBe(200);
    expect(res.headers()["content-type"]).toMatch(type);
    expect((await res.body()).length).toBeGreaterThan(100);
  });
}

test("the manifest names Wasl and lists a maskable icon", async ({ request }) => {
  const m = await (await request.get("/manifest.webmanifest")).json();
  expect(m).toMatchObject({ name: "Wasl", short_name: "Wasl", theme_color: "#0f4f4c", background_color: "#f6f3ee" });
  expect(m.icons.map((i: { purpose: string }) => i.purpose)).toContain("maskable");
});

test("the head links the icons, manifest and an absolute share image", async ({ page }) => {
  await page.goto("/app");
  await expect(page.locator('link[rel="icon"][href*="/favicon.ico"]')).toHaveCount(1);
  await expect(page.locator('link[rel="icon"][href*="/icon.svg"]')).toHaveCount(1);
  await expect(page.locator('link[rel="apple-touch-icon"]')).toHaveCount(1);
  await expect(page.locator('link[rel="manifest"]')).toHaveAttribute("href", "/manifest.webmanifest");
  await expect(page.locator('meta[property="og:image"]')).toHaveAttribute("content", /^https?:\/\/.+\/opengraph-image/);
});

async function expectBrandAndAxe(page: Page, scope: string, shot: string) {
  const region = page.locator(scope).first();
  await expect(region.locator('[data-brand="mark"]').first()).toBeVisible();
  await expect(region.getByText("و", { exact: true })).toHaveCount(0);
  await region.screenshot({ path: `test-results/screens/brand-${shot}.png` });
  const axe = await new AxeBuilder({ page }).analyze();
  const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

test("the app sidebar shows the mark and links home", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/app");
  await expect(page.getByRole("link", { name: /Wasl/ }).first()).toHaveAttribute("href", "/");
  await expectBrandAndAxe(page, "aside", "sidebar");
});

test("the site header shows the logo without the typed Arabic name", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/how-it-works");
  const header = page.getByRole("navigation", { name: "Primary" });
  await expect(header.locator('[data-brand="logo"]')).toContainText("Wasl");
  await expect(header.getByText("وصل")).toHaveCount(0);
  await expectBrandAndAxe(page, 'nav[aria-label="Primary"]', "header");
});

for (const lang of ["en"] as const) {
  test(`the patient phone header shows the tile (${lang})`, async ({ page }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto(`/patient/notice?lang=${lang}`);
    await expectBrandAndAxe(page, "header", `phone-${lang}`);
  });
}
