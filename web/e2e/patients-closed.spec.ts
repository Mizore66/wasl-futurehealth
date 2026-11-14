import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { expect, test, type Page } from "@playwright/test";

// Ordinary browser journeys through the app's committed synthetic sample mode.
// No routes are intercepted and these files are not represented as live API captures.
type Sample = { id: string; programme: string; provider: string; state: string; closed: boolean };
const dir = path.join(process.cwd(), "data/api");
const read = (file: string) => JSON.parse(readFileSync(path.join(dir, file), "utf8"));
const samples = new Map<string, Sample>();
for (const c of Object.values(read("fixtures.json").fixtures) as Sample[]) samples.set(c.id, c);
for (const file of readdirSync(dir).filter((name) => /^cases_[a-z_]+_page1\.json$/.test(name)))
  for (const c of read(file).cases as Sample[]) samples.set(c.id, c);
const all = [...samples.values()];

async function view(page: Page, rows: Sample[], closed: boolean, offset = 0) {
  await expect(page.getByTestId("showing")).toContainText(`${rows.length} ${rows.length === 1 ? "case" : "cases"}: `);
  await expect(page.locator("tbody tr")).toHaveCount(Math.min(25, Math.max(0, rows.length - offset)));
  for (const row of await page.locator("tbody tr").all()) {
    if (closed) await expect(row).toContainText("Closed day");
    else await expect(row).not.toContainText("Closed day");
  }
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
    for (const c of rows.filter((c) => c.state !== "closed"))
      await expect(page.locator("tbody")).toContainText(c.id.split("/").at(-1)!);
  });
}

for (const closed of [true, false]) {
  test(`native Search and Refine retain closed=${closed}, composed filters and paging`, async ({ page }) => {
    const rows = all.filter((c) => c.closed === closed);
    expect(rows.length).toBeGreaterThan(25);
    await page.goto(`/app/patients?closed=${closed}`);
    await view(page, rows, closed);
    await page.getByRole("navigation", { name: "Pages" }).getByRole("link", { name: "Next", exact: true }).click();
    await expect(page).toHaveURL((url) => url.searchParams.get("closed") === String(closed) && url.searchParams.get("page") === "2");
    await view(page, rows, closed, 25);

    const search = page.getByRole("searchbox", { name: "Search case id" });
    await search.fill("sr-1");
    await page.getByRole("button", { name: "Search", exact: true }).click();
    await expect(page).toHaveURL((url) => url.searchParams.get("closed") === String(closed) && url.searchParams.get("q") === "sr-1" && !url.searchParams.has("page"));
    await view(page, rows.filter((c) => c.id.includes("sr-1")), closed);

    const refine = page.getByTestId("refine");
    await refine.locator("summary").click();
    await refine.getByLabel("Programme", { exact: true }).selectOption("lung_nodule");
    await refine.getByLabel("Provider", { exact: true }).selectOption("rad-imaging");
    await refine.getByRole("button", { name: "Apply filters", exact: true }).click();
    await expect(page).toHaveURL((url) => url.searchParams.get("closed") === String(closed) && url.searchParams.get("q") === "sr-1" && url.searchParams.get("programme") === "lung_nodule" && url.searchParams.get("provider") === "rad-imaging" && !url.searchParams.has("page"));
    await view(page, rows.filter((c) => c.id.includes("sr-1") && c.programme === "lung_nodule" && c.provider === "rad-imaging"), closed);
    await expect(refine.getByLabel("Programme", { exact: true })).toHaveValue("lung_nodule");
    await expect(refine.getByLabel("Provider", { exact: true })).toHaveValue("rad-imaging");
    if (closed) await expect(page.getByRole("navigation", { name: "Filters" }).getByRole("link", { name: "Closed", exact: true })).toHaveAttribute("aria-current", "true");
  });
}
