import AxeBuilder from "@axe-core/playwright";
import { expect, test } from "@playwright/test";

async function noSeriousAxe(page: import("@playwright/test").Page) {
  const result = await new AxeBuilder({ page }).analyze();
  expect(result.violations.filter((violation) => violation.impact === "serious" || violation.impact === "critical").map((violation) => violation.id)).toEqual([]);
}

test.describe("Recovery fixture mode", () => {
  test("shows synthetic provenance, ranking and exclusions while remaining read-only", async ({ page }) => {
    await page.goto("/app/recovery");
    await expect(page.getByRole("heading", { name: "No-show recovery" })).toBeVisible();
    await expect(page.getByText("Simulated released-capacity projection", { exact: true })).toBeVisible();
    await expect(page.getByRole("heading", { name: /Rank 1/ })).toContainText("AHS-sr-910");
    await expect(page.getByRole("heading", { name: /Excluded/ })).toContainText("AHS-sr-900");
    await expect(page.getByText("Slot eligible; outreach not permitted.")).toBeVisible();
    await page.getByText("Checks and explanations").first().click();
    await expect(page.getByText(/programme_match: pass/)).toBeVisible();
    await expect(page.getByRole("button", { name: "Select case" })).toHaveCount(0);
    await expect(page.getByText(/Fixture mode is read-only/)).toBeVisible();
    await noSeriousAxe(page);
  });

  test("supports keyboard navigation and a narrow viewport", async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/app/recovery");
    await page.keyboard.press("Tab");
    await expect(page.locator(":focus")).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
    await noSeriousAxe(page);
  });
});

/*
 * Opt-in live browser tests use the mock below and a separately started Next
 * server with WASL_API_URL=http://127.0.0.1:3297 plus WEB_REVIEWER and
 * COORDINATOR tokens. The Go suite separately pins the real atomic handler.
 */
const LIVE = !!process.env.RECOVERY_LIVE_MOCK;
test.describe("Recovery live confirmation", () => {
  test.skip(!LIVE, "set RECOVERY_LIVE_MOCK=1 with Next pointed at the recovery mock");

  test("confirms once, persists after reload, and recovers a competing stale dialog", async ({ page, browser }) => {
    test.setTimeout(60_000);
    const { createServer } = await import("node:http");
    const { readFileSync } = await import("node:fs");
    const path = await import("node:path");
    const list = JSON.parse(readFileSync(path.join(process.cwd(), "data/api/recovery_slots.json"), "utf8"));
    const detail = JSON.parse(readFileSync(path.join(process.cwd(), "data/api/recovery_slot.json"), "utf8"));
    const meta = list.meta ?? { api: "wasl-api/1", data_mode: "simulated", run: "fixture", policy: "synthetic", reader_model: "unknown", loaded_at: "2026-11-13T00:00:00Z", note: "synthetic" };
    let filled = "";
    let eventHash = "";
    let requests = 0;
    const server = createServer((req, res) => {
      const send = (status: number, body: unknown) => {
        res.writeHead(status, { "content-type": "application/json" });
        res.end(JSON.stringify(body));
      };
      const url = new URL(req.url ?? "/", "http://mock");
      if (req.headers.authorization !== `Bearer ${process.env.WASL_API_WEB_REVIEWER_TOKEN}` && req.headers.authorization !== `Bearer ${process.env.WASL_API_COORDINATOR_TOKEN}`) return send(401, { error: "authentication required" });
      if (url.pathname === "/healthz") return send(200, JSON.parse(readFileSync(path.join(process.cwd(), "data/api/healthz.json"), "utf8")));
      if (url.pathname === "/api/dashboard") return send(200, JSON.parse(readFileSync(path.join(process.cwd(), "data/api/dashboard.json"), "utf8")));
      if (url.pathname === "/api/fixtures") return send(200, JSON.parse(readFileSync(path.join(process.cwd(), "data/api/fixtures.json"), "utf8")));
      if (url.pathname === "/api/cases") return send(200, { meta, cases: [], total: 0, limit: 500, offset: 0 });
      if (url.pathname === "/api/recovery/slots") {
        const slots = list.slots.map((slot: Record<string, unknown>) => filled ? { ...slot, status: "filled", filled_case: filled, head: eventHash } : slot);
        return send(200, { ...list, meta, slots });
      }
      if (url.pathname === `/api/recovery/slots/${detail.slot.slot_id}`) {
        const slot = filled ? { ...detail.slot, status: "filled", filled_case: filled, head: eventHash, backfill: { case_id: filled, event: eventHash, status: "simulated_backfill_confirmed" } } : detail.slot;
        return send(200, { ...detail, meta, slot });
      }
      if (req.method === "POST" && url.pathname.endsWith("/confirm")) {
        let raw = "";
        req.on("data", (chunk) => (raw += chunk));
        req.on("end", () => {
          requests++;
          const input = JSON.parse(raw);
          if (filled || input.expected_slot_head !== detail.slot.head) return send(409, { meta, error: "stale expected_slot_head; reload the slot projection", conflict: "slot_head" });
          if (input.outreach_intent) return send(409, { meta, error: "outreach intent requires effective consent granted", conflict: "consent" });
          filled = input.case_id;
          eventHash = "ledger-confirmed-hash";
          send(201, { meta, event: { hash: eventHash, at: "2026-11-13T00:00:00Z", type: "waitlist_recovery" }, case_head: eventHash, canonical_unchanged: true, dispatched: false, duplicate: false });
        });
        return;
      }
      send(404, { meta, error: "not found" });
    });
    await new Promise<void>((resolve) => server.listen(3297, "127.0.0.1", resolve));
    try {
      page.on("pageerror", (error) => console.error("recovery browser error:", error.message));
      page.on("console", (message) => { if (message.type() === "error") console.error("recovery console:", message.text()); });
      page.on("response", (response) => { if (response.status() >= 400) console.error("recovery response:", response.status(), response.url()); });
      await page.goto("/app/recovery");
      await page.waitForLoadState("networkidle");
      const otherContext = await browser.newContext({ baseURL: new URL(page.url()).origin });
      const competing = await otherContext.newPage();
      competing.on("pageerror", (error) => console.error("competing recovery browser error:", error.message));
      competing.on("console", (message) => { if (message.type() === "error") console.error("competing recovery console:", message.text()); });
      competing.on("response", (response) => { if (response.status() >= 400) console.error("competing recovery response:", response.status(), response.url()); });
      await competing.goto("/app/recovery");
      await competing.waitForLoadState("networkidle");
      for (const current of [page, competing]) {
        await expect(current.getByRole("button", { name: "Select case" })).toBeVisible();
        await current.getByRole("button", { name: "Select case" }).click();
        await expect(current.getByRole("dialog")).toBeVisible();
      }
      await Promise.all([
        page.getByRole("button", { name: "Confirm simulated backfill", exact: true }).click(),
        competing.getByRole("button", { name: "Confirm simulated backfill", exact: true }).click(),
      ]);
      const messages = [page, competing];
      await expect.poll(async () => (await Promise.all(messages.map((current) => current.locator("[role=status]").allTextContents()))).flat().filter((text) => text.includes("backfill") || text.includes("refused")).length).toBeGreaterThan(0);
      const outcomeText = (await Promise.all(messages.map((current) => current.locator("[role=status]").allTextContents()))).flat().join("\n");
      expect(outcomeText).toContain("Simulated backfill confirmed; no provider booking or message was made.");
      expect(outcomeText).toContain("Confirmation refused:");
      await page.reload();
      await expect(page.getByText("filled", { exact: true }).first()).toBeVisible();
      expect(requests).toBe(2);
      expect(filled).toBe("ahs-mushrif/ServiceRequest/AHS-sr-910");
      await otherContext.close();
    } finally {
      await new Promise((resolve) => server.close(resolve));
    }
  });
});
