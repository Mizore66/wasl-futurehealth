import { expect, test, type Page } from "@playwright/test";
import fixtures from "../data/api/fixtures.json";

/*
 * Event history months on the audit page: all open when the whole list fits in the room the pane already takes with
 * only the newest month open, otherwise only the newest month. The card never grows to fit them.
 */

const auditHref = (name: keyof typeof fixtures.fixtures) => `/app/patients/${encodeURIComponent(fixtures.fixtures[name].id)}/audit`;
const months = (page: Page) => page.getByRole("region", { name: "Event history" }).locator("#audit-chronology > details");

/** The workspace card's height now, and with only the first month open (the collapsed layout), measured in place. */
const cardHeights = (page: Page) =>
  page.evaluate(() => {
    const pane = document.querySelector("section[aria-labelledby=au-history-h]")!;
    const card = pane.parentElement!.parentElement!;
    const ds = [...document.querySelectorAll<HTMLDetailsElement>("#audit-chronology > details")];
    const now = card.getBoundingClientRect().height;
    const was = ds.map((d) => d.open);
    ds.forEach((d, i) => (d.open = i === 0));
    const collapsed = card.getBoundingClientRect().height;
    ds.forEach((d, i) => (d.open = was[i]));
    return { now, collapsed };
  });

test("at 1440 a case with few events opens every month without the card growing", async ({ page }) => {
  await page.goto(auditHref("dr_exam_closure"));
  const all = months(page);
  expect(await all.count()).toBeGreaterThan(1);
  await expect(page.locator("#audit-chronology")).toHaveAttribute("data-all-fit", "true");
  for (const d of await all.all()) await expect(d).toHaveJSProperty("open", true);
  const h = await cardHeights(page);
  expect(Math.abs(h.now - h.collapsed)).toBeLessThanOrEqual(1);

  // A month the reader closes stays closed, through a resize too.
  await all.last().locator("> summary").click();
  await expect(all.last()).toHaveJSProperty("open", false);
  await page.setViewportSize({ width: 1280, height: 860 });
  await page.waitForTimeout(150);
  await expect(all.last()).toHaveJSProperty("open", false);
  await expect(all.first()).toHaveJSProperty("open", true);
});

test("months that would make the card taller keep only the newest open", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(auditHref("false_closure_trap"));
  const all = months(page);
  expect(await all.count()).toBeGreaterThan(1);
  await expect(all.first()).toHaveJSProperty("open", true);
  for (const d of (await all.all()).slice(1)) await expect(d).toHaveJSProperty("open", false);
  await expect(page.locator("#audit-chronology")).not.toHaveAttribute("data-all-fit", /.*/);
});
