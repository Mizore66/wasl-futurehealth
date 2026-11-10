import { expect, test, type Locator, type Page } from "@playwright/test";

// The shared Button's pressed feedback (#53), on the Patients search button: a product secondary action.

const search = (page: Page) => page.getByRole("button", { name: "Search", exact: true });

/** Rendered width now, as a share of the width at rest: what the eye sees, whichever transform property carries the squeeze. */
const width = (b: Locator) => b.evaluate((el) => el.getBoundingClientRect().width);
let rest = 0;
const ratio = async (b: Locator) => (await width(b)) / rest;
const scale = (b: Locator) => b.evaluate((el) => getComputedStyle(el).scale);

async function pressOn(page: Page, b: Locator) {
  const box = await b.boundingBox();
  if (!box) throw new Error("button has no box");
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
}

test.describe("shared Button press", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/app/patients");
    await expect(search(page)).toHaveAttribute("data-slot", "button");
    rest = await width(search(page));
  });

  test("squeezes to exactly 0.96 while held and returns to 1 on release, over 150ms of transform only", async ({ page }) => {
    const b = search(page);
    expect(await b.evaluate((el) => {
      const cs = getComputedStyle(el);
      return [cs.transitionProperty, cs.transitionDuration];
    })).toEqual(["transform, scale", "0.15s"]);

    await pressOn(page, b);
    await expect.poll(() => scale(b)).toBe("0.96");
    await expect.poll(() => ratio(b)).toBeCloseTo(0.96, 3);
    // Layout does not move: the squeeze is paint only.
    expect(await b.evaluate((el) => (el as HTMLElement).offsetHeight)).toBe(44);

    await page.mouse.up();
    await expect.poll(() => ratio(b)).toBeCloseTo(1, 3);
    expect(await scale(b)).toBe("none");
  });

  test("keeps the keyboard focus outline", async ({ page }) => {
    const b = search(page);
    await page.getByRole("searchbox", { name: "Search case id" }).focus();
    await page.keyboard.press("Tab");
    await expect(b).toBeFocused();
    expect(await b.evaluate((el) => {
      const cs = getComputedStyle(el);
      return [el.matches(":focus-visible"), cs.outlineStyle, cs.outlineWidth];
    })).toEqual([true, "solid", "2px"]);
  });

  for (const [what, attr] of [
    ["disabled", "disabled"],
    ["aria-disabled", "aria-disabled"],
    ["aria-busy", "aria-busy"],
  ] as const) {
    test(`does not squeeze when ${what}`, async ({ page }) => {
      const b = search(page);
      await b.evaluate((el, a) => el.setAttribute(a, a === "disabled" ? "" : "true"), attr);
      await pressOn(page, b);
      await page.waitForTimeout(300);
      expect(await scale(b)).toBe("none");
      expect(await ratio(b)).toBeCloseTo(1, 3);
      await page.mouse.up();
    });
  }

  test("does not squeeze under reduced motion", async ({ page }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    const b = search(page);
    await pressOn(page, b);
    await page.waitForTimeout(300);
    expect(await scale(b)).toBe("none");
    expect(await ratio(b)).toBeCloseTo(1, 3);
    await page.mouse.up();
  });
});
