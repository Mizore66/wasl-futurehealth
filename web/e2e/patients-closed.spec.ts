import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { expect, test, type Page } from "@playwright/test";

// Ordinary browser journeys through the app's committed synthetic sample mode.
// No routes are intercepted and these files are not represented as live API captures.
type Sample = { id: string; programme: string; provider: string; state: string; closed: boolean; referral_day: number };
const dir = path.join(process.cwd(), "data/api");
const read = (file: string) => JSON.parse(readFileSync(path.join(dir, file), "utf8"));
const samples = new Map<string, Sample>();
for (const c of Object.values(read("fixtures.json").fixtures) as Sample[]) samples.set(c.id, c);
for (const file of readdirSync(dir).filter((name) => /^cases_[a-z_]+_page1\.json$/.test(name)))
  for (const c of read(file).cases as Sample[]) samples.set(c.id, c);
const all = [...samples.values()];

/** The UTC date a referral was received (the cohort epoch is 1 May 2026), as the Referral calendar groups it. */
const referred = (c: Sample) => new Date(Date.UTC(2026, 4, 1) + Math.round(c.referral_day * 86_400_000)).toISOString().slice(0, 10);
const months = (rows: Sample[]) => [...new Set(rows.map((c) => referred(c).slice(0, 7)))].sort();

/** The count says how many match; the latest month's case files list exactly that month's matches, each closed or open. */
async function view(page: Page, rows: Sample[], closed: boolean) {
  await expect(page.getByTestId("showing")).toContainText(`${rows.length} ${rows.length === 1 ? "case" : "cases"}: `);
  if (rows.length === 0) return;
  const month = months(rows).at(-1)!;
  const inMonth = rows.filter((c) => referred(c).startsWith(month));
  await expect(page.getByTestId("month-total")).toHaveText(`${inMonth.length} ${inMonth.length === 1 ? "referral" : "referrals"}`);
  await page.getByRole("link", { name: /^Month’s files for / }).click();
  await expect(page).toHaveURL((url) => url.searchParams.get("files") === month);
  await expect(page.locator("[data-pf-case]")).toHaveCount(Math.min(25, inMonth.length));
  for (const folder of await page.locator("[data-pf-case]").all()) {
    if (closed) await expect(folder).toContainText("Closed day");
    else await expect(folder).not.toContainText("Closed day");
  }
  await page.goBack();
}

for (const programme of ["lung_nodule", "diabetic_retinopathy", "breast", "lung_ldct", "ifhas"]) {
  test(`Patients Closed chip includes ${programme} no-action resolutions`, async ({ page }) => {
    const rows = all.filter((c) => c.programme === programme && c.closed);
    expect(rows.some((c) => c.state !== "closed")).toBe(true);
    await page.goto(`/app/patients?programme=${programme}`);
    const chip = page.getByRole("navigation", { name: "Filters" }).getByRole("link", { name: "Closed", exact: true });
    await chip.click();
    await expect(page).toHaveURL((url) => url.searchParams.get("programme") === programme && url.searchParams.get("closed") === "true" && !url.searchParams.has("state"));
    await expect(chip).toHaveAttribute("aria-current", "true");
    await view(page, rows, true);
    // Every no-action resolution is in the closed view: each one's date opens a folder for it.
    for (const c of rows.filter((c) => c.state !== "closed").slice(0, 3)) {
      await page.goto(`/app/patients?programme=${programme}&closed=true&files=${referred(c)}`);
      await expect(page.locator(`[data-pf-case="${c.id}"]`)).toContainText("Closed day");
    }
  });
}

for (const closed of [true, false]) {
  test(`native Search and Refine retain closed=${closed}, composed filters and paging`, async ({ page }) => {
    const rows = all.filter((c) => c.closed === closed);
    expect(rows.length).toBeGreaterThan(25);
    await page.goto(`/app/patients?closed=${closed}`);
    await view(page, rows, closed);
    // A month with more than 25 files pages through them and keeps the filter.
    const busy = months(rows).find((m) => rows.filter((c) => referred(c).startsWith(m)).length > 25);
    if (busy) {
      await page.goto(`/app/patients?closed=${closed}&files=${busy}`);
      await page.getByRole("navigation", { name: "Pages" }).getByRole("link", { name: "Next", exact: true }).click();
      await expect(page).toHaveURL((url) => url.searchParams.get("closed") === String(closed) && url.searchParams.get("page") === "2");
      await page.goto(`/app/patients?closed=${closed}`);
    }

    const search = page.getByRole("searchbox", { name: "Find a case, task or owner" });
    await search.fill("sr-1");
    await page.getByRole("button", { name: "Search", exact: true }).click();
    await expect(page).toHaveURL((url) => url.searchParams.get("closed") === String(closed) && url.searchParams.get("q") === "sr-1" && !url.searchParams.has("page"));
    await view(page, rows.filter((c) => c.id.toLowerCase().includes("sr-1")), closed);

    const refine = page.getByTestId("refine");
    await refine.locator("summary").click();
    await refine.getByRole("combobox", { name: "Programme", exact: true }).selectOption("lung_nodule");
    await refine.getByRole("combobox", { name: "Provider", exact: true }).selectOption("rad-imaging");
    await refine.getByRole("button", { name: "Apply filters", exact: true }).click();
    await expect(page).toHaveURL((url) => url.searchParams.get("closed") === String(closed) && url.searchParams.get("q") === "sr-1" && url.searchParams.get("programme") === "lung_nodule" && url.searchParams.get("provider") === "rad-imaging" && !url.searchParams.has("page"));
    await view(page, rows.filter((c) => c.id.includes("sr-1") && c.programme === "lung_nodule" && c.provider === "rad-imaging"), closed);
    await expect(refine.getByRole("combobox", { name: "Programme", exact: true })).toHaveValue("lung_nodule");
    await expect(refine.getByRole("combobox", { name: "Provider", exact: true })).toHaveValue("rad-imaging");
    if (closed) await expect(page.getByRole("navigation", { name: "Filters" }).getByRole("link", { name: "Closed", exact: true })).toHaveAttribute("aria-current", "true");
  });
}
