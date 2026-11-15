import { expect, test } from "@playwright/test";

/*
 * Issues #104/#125, web and Go together. The Patients "No owner" filter is one `unowned=true` request that the Go API
 * answers before it counts and pages (the web no longer fetches every page and filters client-side), so a web server
 * pointed at an older API would silently ignore it. This spec runs against the REAL Go API (`wsi7 serve`, for example
 * scripts/demo.sh serve) and a `next start` whose WASL_API_URL points at it; it uses no mock.
 *
 * Run with OWNER_LIVE_API=<the Go API base URL>, PLAYWRIGHT_BASE_URL=<that next start>, and the web server's
 * WASL_API_WEB_REVIEWER_TOKEN plus WASL_API_COORDINATOR_TOKEN. It saves one owner through the API's own coordinator
 * action, so run it against a disposable lane only. Not part of the default CI projects: the parent runs it as a gate.
 */
const API = process.env.OWNER_LIVE_API?.replace(/\/$/, "");
const SAVED = "Hessa Al Ali";

test.describe("Patients owner filter, live web with the real Go API", () => {
  test.skip(!API, "set OWNER_LIVE_API with the server pointed at the real Go API");

  const token = (name: string) => {
    const v = process.env[name]?.trim();
    if (!v) throw new Error(`${name} must be set, with the value the servers use`);
    return v;
  };
  const noun = (n: number) => (n === 1 ? "case" : "cases");

  test("an assigned case leaves No owner, appears under its saved owner, and the counts match the API", async ({ page, request }) => {
    const read = { headers: { authorization: `Bearer ${token("WASL_API_WEB_REVIEWER_TOKEN")}` } };
    const unowned = async () => (await (await request.get(`${API}/api/cases?unowned=true&limit=500`, read)).json()) as { total: number; cases: { id: string; owner: string; api_overlay: { owner: string; head: string } }[] };

    const before = await unowned();
    expect(before.total).toBeGreaterThan(0);
    for (const c of before.cases) expect(c.owner === "" && c.api_overlay.owner === "").toBe(true);
    const target = before.cases[0];

    await page.goto("/app/patients?owner=none");
    await expect(page.getByTestId("showing")).toHaveText(`${before.total} ${noun(before.total)}: no owner`);

    const assigned = await request.post(`${API}/api/cases/${target.id}/actions`, {
      headers: { authorization: `Bearer ${token("WASL_API_COORDINATOR_TOKEN")}`, "content-type": "application/json" },
      data: { action: "assign_owner", owner: SAVED, expected_head: target.api_overlay.head },
    });
    expect(assigned.status()).toBe(201);

    const after = await unowned();
    expect(after.total).toBe(before.total - 1);
    expect(after.cases.map((c) => c.id)).not.toContain(target.id);

    // A reload reads the same effective owner from the API: the page count is the API's count, not a client-side filter's.
    await page.goto("/app/patients?owner=none");
    await expect(page.getByTestId("showing")).toHaveText(`${after.total} ${noun(after.total)}: no owner`);
    await page.goto(`/app/patients?owner=${encodeURIComponent(SAVED)}`);
    await expect(page.getByTestId("showing")).toContainText(`: owner ${SAVED}`);
    await expect(page.locator("tbody")).toContainText(SAVED);

    await page.goto(`/app/patients/${encodeURIComponent(target.id)}`);
    await expect(page.getByText(`owner ${SAVED}`)).toBeVisible();
  });
});
