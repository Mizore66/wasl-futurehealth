import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import dashboard from "../data/api/dashboard.json";
import results from "../data/results.json";

const SOURCE_TEXT = JSON.stringify(results) + JSON.stringify(dashboard);

/** Numbers a source file states, as printed or as a share (32.0% or 3.4 points for 0.32 or 0.034). */
function inSources(n: string): boolean {
  const bare = n.replace(/,/g, "").replace(/%$/, "");
  const v = Number(bare);
  const forms = new Set([n, bare, n.replace(/%$/, "")]);
  forms.add(String(v / 100)).add((v / 100).toFixed(3)).add((v / 100).toFixed(4));
  return [...forms].some((f) => SOURCE_TEXT.includes(f));
}

/** Axis labels: layout, not data. */
const AXIS = new Set(["0%", "10%", "20%", "30%", "40%", "100%", "0.5%"]);

async function numbersOn(page: Page): Promise<string[]> {
  const text = await page.locator("main").innerText();
  return [...new Set(text.match(/\d[\d,]*(\.\d+)?%?/g) ?? [])];
}

async function axe(page: Page) {
  const report = await new AxeBuilder({ page }).analyze();
  const serious = report.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

test.describe("Programme", () => {
  test("leads with reported vs measured, one track, the target and the truth line", async ({ page }) => {
    await page.goto("/app");
    await expect(page.locator("h1")).toHaveText("Programme");
    await expect(page.locator("[data-honesty]:visible").first()).toBeVisible();
    await expect(page.getByTestId("kpi-eid")).toContainText("12.6%");
    await expect(page.getByTestId("kpi-wasl")).toContainText("31.9%");
    await expect(page.getByRole("img", { name: "On a 0 to 100% scale: reported 12.6%, measured by Wasl 31.9%, target 90%." })).toBeVisible();
    await expect(page.getByTestId("kpi-truth")).toHaveText("Within 1 point of the true rate (32.9%) in testing");
    await expect(page.getByText("1 in 6 on your chase list is already done.")).toBeVisible();
    await page.screenshot({ path: "test-results/screens/programme.png", fullPage: true });
    await axe(page);
  });

  test("both Source disclosures open and close", async ({ page }) => {
    await page.goto("/app");
    for (const [card, note] of [
      ["kpi-h", "The target is the DoH 31-day standard."],
      ["stall-h", "Simulated run of 2,100 cases"],
    ] as const) {
      const section = page.locator(`section[aria-labelledby="${card}"]`);
      const button = section.getByRole("button", { name: "Source" });
      await expect(section.getByText(note)).toBeHidden();
      await button.click();
      await expect(section.getByRole("button", { name: "Hide source" })).toHaveAttribute("aria-expanded", "true");
      await expect(section.getByText(note)).toBeVisible();
      if (card === "stall-h") await page.screenshot({ path: "test-results/screens/programme-source.png", fullPage: true });
      await section.getByRole("button", { name: "Hide source" }).click();
      await expect(section.getByText(note)).toBeHidden();
    }
  });

  test("the top three stalls each link to the filtered patient list", async ({ page }) => {
    await page.goto("/app");
    const rows = page.locator('section[aria-labelledby="stall-h"] ol a');
    await expect(rows).toHaveCount(3);
    const expected = [
      ["No acknowledgement", "390", "no_acknowledgement"],
      ["Not booked", "252", "not_booked"],
      ["Never notified", "183", "never_notified"],
    ];
    for (const [i, [label, count, dropoff]] of expected.entries()) {
      await expect(rows.nth(i)).toContainText(label);
      await expect(rows.nth(i)).toContainText(count);
      await expect(rows.nth(i)).toHaveAttribute("href", `/app/patients?dropoff=${dropoff}`);
    }
    await rows.first().click();
    await expect(page).toHaveURL(/\/app\/patients\?dropoff=no_acknowledgement$/);
  });

  test("Open today's list and See proof go where the board says", async ({ page }) => {
    await page.goto("/app");
    await expect(page.getByLabel("Safety")).toContainText("0 false closures in 3,025 tested.");
    await page.getByRole("link", { name: "See proof" }).click();
    await expect(page).toHaveURL(/\/app\/results$/);
    await page.goto("/app");
    await page.getByRole("link", { name: "Open today's list" }).click();
    await expect(page).toHaveURL(/\/app\/today$/);
  });

  test("every number on the screen is in results.json or the dashboard", async ({ page }) => {
    await page.goto("/app");
    expect((await numbersOn(page)).filter((n) => !AXIS.has(n) && !inSources(n))).toEqual([]);
  });
});

test.describe("Results", () => {
  test("shows the held-out figures, the demo cohort on its own line, and the review comparison", async ({ page }) => {
    await page.goto("/app/results");
    await expect(page.locator("h1")).toHaveText("Results");
    await expect(page.locator("[data-honesty]:visible").first()).toBeVisible();
    await expect(page.getByText("Frozen evaluation at code 5496563")).toBeVisible();
    await expect(page.getByTestId("lead-figure")).toHaveText("0 of 3,025");
    await expect(page.getByRole("img", { name: /95% upper bound 0\.12%/ })).toBeVisible();
    await expect(page.getByText("Recall 0.782 (0.769–0.795).")).toBeVisible();
    await expect(page.getByText(/0 of 14,451 wrong-patient messages and 0 of 9,602 sent without consent/)).toBeVisible();
    await expect(page.getByTestId("kpi-error")).toHaveText("3.1 points off");
    await expect(page.getByText("Emirates ID only: 20.7 points off · mean of 5 seeds")).toBeVisible();
    await expect(page.getByRole("img", { name: "Within 31 days, held-out noise: Emirates ID only 12.2%, Wasl 29.8%, truth 32.9%." })).toBeVisible();
    await expect(page.getByTestId("demo-cohort")).toHaveText(
      "Demo cohort, one run on training-time noise: Emirates ID only 12.6%, Wasl 31.9%, truth 32.9%.",
    );
    await expect(page.getByText("3 of 7")).toBeVisible();
    await expect(page.getByTestId("double-review")).toHaveText("0in 8,422");
    await expect(page.getByTestId("single-review")).toHaveText("21");
    await expect(page.getByText(/On 12,600 calibration cases/)).toBeVisible();
    await page.screenshot({ path: "test-results/screens/results.png", fullPage: true });
    await axe(page);
  });

  test("every number on the screen is in results.json", async ({ page }) => {
    await page.goto("/app/results");
    expect((await numbersOn(page)).filter((n) => !AXIS.has(n) && !inSources(n))).toEqual([]);
  });
});
