import { readFileSync } from "node:fs";
import path from "node:path";
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import fixtures from "../data/api/fixtures.json";

/*
 * Issue #44, fixtures mode: the DR clinical findings on the served dr_exam_closure detail, read against the same JSON
 * file, at desktop and phone widths. The snapshot holds no lung-nodule or breast case detail; those two programmes
 * (and DR pending states) are checked against a mock of the Go API in case-live.spec.ts, and against every served
 * summary shape in tests/programme-findings.test.ts.
 */
const dir = path.join(process.cwd(), "data", "api");
const detail = JSON.parse(readFileSync(path.join(dir, "case_dr_exam_closure.json"), "utf8"));
const dr = detail.case.dr;
const href = `/app/patients/${encodeURIComponent(fixtures.fixtures.dr_exam_closure!.id)}`;

const YEAR = new Intl.DateTimeFormat("en-GB", { day: "numeric", month: "short", year: "numeric", timeZone: "UTC" });
const day = (d: number) => YEAR.format(new Date(Date.UTC(2026, 4, 1) + Math.round(d * 86_400_000)));

async function noSeriousAxe(page: Page) {
  const axe = await new AxeBuilder({ page }).analyze();
  const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

for (const width of [1440, 390] as const) {
  test(`DR findings match the served JSON at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: width === 390 ? 844 : 900 });
    expect((await page.goto(href))?.status()).toBe(200);
    const card = page.getByRole("region", { name: "Diabetic retinopathy screening" });
    await expect(card).toBeVisible();
    const rows = card.getByRole("definition");
    const value = (label: string) => card.locator("dt", { hasText: new RegExp(`^${label}$`) }).locator("xpath=following-sibling::dd[1]");
    await expect(value("Grade")).toHaveText(dr.screen_label);
    await expect(value("Urgency")).toHaveText(dr.rec.urgency === "urgent" ? "Urgent" : "Routine");
    await expect(value("Deadline")).toHaveText(`${dr.rec.deadline_days} days, due ${day(dr.deadline_day)}`);
    await expect(value("Outcome")).toHaveText("Examined, follow-up");
    await expect(value("Within deadline")).toHaveText(dr.within_deadline ? "Yes" : "No");
    await expect(value("Next due")).toHaveText(`Clinic review ${day(dr.next_due.due_day)} (note review interval)`);
    expect(await rows.count()).toBeGreaterThanOrEqual(8);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(width);
    await page.screenshot({ path: `test-results/screens/case-findings-dr-${width}.png`, fullPage: true });
    await noSeriousAxe(page);
  });
}
