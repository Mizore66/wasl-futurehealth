import { readFileSync } from "node:fs";
import path from "node:path";
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

const ROUTE = "/app/agent-review";
const GUIDE = readFileSync(path.join(__dirname, "..", "..", "docs", "DEMO-RECORDING-GUIDE.md"), "utf8").split("\n");
const DISCLOSURE = /“([^”]+)”/.exec(GUIDE[42])![1];

async function noSeriousAxe(page: Page) {
  const axe = await new AxeBuilder({ page }).analyze();
  const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
  expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
}

test.describe("agent review lane", () => {
  test.beforeEach(async ({ page }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto(ROUTE);
  });

  test("has one h1, the banner and the exact disclosure from the guide", async ({ page }) => {
    await expect(page.locator("h1")).toHaveCount(1);
    expect(DISCLOSURE).toContain("Development passed;");
    await expect(page.getByTestId("disclosure-text")).toHaveText(DISCLOSURE);
    const banner = page.getByTestId("agent-banner");
    await expect(banner).toContainText("separate experimental run");
    await expect(banner).toContainText("r1agent10, seed 987");
    await expect(banner).toContainText("main");
    await expect(banner).toContainText("Colorectal (CRC) only");
    await expect(banner).toContainText("Simulated");
    await expect(banner).toContainText("auto-close line 0.99");
    await expect(banner).toContainText("published snapshot only");
  });

  test("provenance says cached and experimental, with model, prompt and schema", async ({ page }) => {
    const prov = page.getByTestId("agent-provenance");
    await expect(prov).toContainText("gpt-5.6-luna");
    await expect(prov).toContainText("agent-v4-json");
    await expect(prov).toContainText("agent-evidence-tools-v1");
    await expect(prov).toContainText("Cached");
    await expect(prov).toContainText("Experimental");
  });

  test("citations render with tool and record reference but no prompt text", async ({ page }) => {
    await expect(page.getByTestId("agent-label")).toContainText("Same person");
    const cites = page.locator("[data-citation]");
    await expect(cites).toHaveCount(4);
    await expect(cites.first()).toContainText("Looked up a registration record");
    await expect(cites.first()).toContainText("reg_533471be…");
    await expect(page.getByTestId("agent-citations")).not.toContainText(/prompt:|system:|you are/i);
  });

  test("shows the r1 and agent reads and that a human made the final sign-off", async ({ page }) => {
    await expect(page.getByTestId("human-link-decision")).toContainText("Signed by a person");
    await expect(page.getByTestId("read-r1")).toContainText("Human reviewer, first read");
    await expect(page.getByTestId("read-agent")).toContainText("Evidence agent (experimental)");
    await expect(page.getByTestId("reads-list")).toContainText("r1 + agent");
    await expect(page.getByTestId("final-signoff")).toContainText("Final sign-off by a person");
    await expect(page.getByTestId("final-signoff")).toContainText("Simulated clinician");
    await expect(page.getByTestId("agent-chain-verified")).toContainText("Verified · 4 of 4 intact");
    await expect(page.locator("[data-entry]")).toHaveCount(4);
  });

  test("never claims validated, autonomous or clinically effective", async ({ page }) => {
    const text = (await page.locator("main").innerText()).replace(DISCLOSURE, "");
    expect(text).not.toMatch(/validated|autonomous|clinically effective/i);
  });

  test("links back to the default review flow, which still works", async ({ page }) => {
    await page.getByRole("link", { name: "← Back to the default review flow" }).click();
    await expect(page).toHaveURL(/\/app\/review$/);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Review");
    await expect(page.getByText("Simulated queue: rebuilt from a held demo case")).toBeVisible();
    await expect(page.getByText(/Experimental|r1agent10/)).toHaveCount(0);
  });

  test("screenshot and axe at desktop width", async ({ page }) => {
    await page.screenshot({ path: "test-results/screens/agent-desktop.png", fullPage: true });
    await noSeriousAxe(page);
  });
});

test.describe("agent review at 390px", () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test("does not scroll sideways, keeps targets at 44px, and is axe clean", async ({ page }) => {
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.goto(ROUTE);
    const over = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    expect(over).toBeLessThanOrEqual(0);

    const small = await page.evaluate(() => {
      const bad: string[] = [];
      for (const el of document.querySelectorAll<HTMLElement>("main a, main summary, main button")) {
        const r = el.getBoundingClientRect();
        if (r.width === 0 || r.height === 0) continue;
        if (r.height < 43.5) bad.push(`${el.tagName} "${el.textContent?.trim().slice(0, 40)}" ${Math.round(r.height)}px`);
      }
      return bad;
    });
    expect(small).toEqual([]);

    await page.screenshot({ path: "test-results/screens/agent-390.png", fullPage: true });
    await noSeriousAxe(page);
  });
});
