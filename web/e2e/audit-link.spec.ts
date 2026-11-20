import { expect, test } from "@playwright/test";

const ID = "ahs-mushrif/ServiceRequest/AHS-sr-1011";

test("the sidebar's Audit trail opens an audit, then the last case opened", async ({ page }) => {
  await page.goto("/app");
  const nav = page.locator("aside#app-sidebar").getByRole("navigation", { name: "Main" });
  await expect(nav.getByRole("link", { name: "Audit trail" })).toHaveAttribute("href", "/app/audit");
  await expect(page.getByRole("button", { name: "Appearance" })).toHaveCount(0);

  // Before any case is opened: the first open case's audit.
  await nav.getByRole("link", { name: "Audit trail" }).click();
  await expect(page).toHaveURL(/\/app\/patients\/[^/]+\/audit$/);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("Audit trail");
  await expect(nav.getByRole("link", { name: "Audit trail" })).toHaveAttribute("aria-current", "page");

  // After opening a case: that case's audit.
  await page.goto(`/app/patients/${encodeURIComponent(ID)}`);
  await nav.getByRole("link", { name: "Audit trail" }).click();
  await expect(page).toHaveURL(`/app/patients/${encodeURIComponent(ID)}/audit`);
});

test("a tampered last-case cookie cannot redirect off the audit pages", async ({ page, context }) => {
  await context.addCookies([{ name: "wasl-last-case", value: encodeURIComponent("//evil.example/x"), url: "http://localhost" }]);
  await page.goto("/app/audit");
  await expect(page).toHaveURL(/\/app\/patients\/[^/]+\/audit$/);
});

test("on Today, a sidebar queue link switches queue in the browser without a server render", async ({ page }) => {
  await page.goto("/app/today");
  const nav = page.locator("aside#app-sidebar").getByRole("navigation", { name: "Main" });
  await page.waitForLoadState("networkidle");
  const documents: string[] = [];
  page.on("request", (r) => {
    // A prefetch (next-router-prefetch) may run on hover; only a navigation fetch would re-render the page.
    if (r.resourceType() === "document" || (r.headers()["rsc"] === "1" && !r.headers()["next-router-prefetch"])) documents.push(r.url());
  });
  await nav.getByRole("link", { name: /^Waiting/ }).click();
  await expect(page).toHaveURL(/queue=waiting/);
  await expect(page.getByRole("heading", { level: 2, name: /^Waiting/ })).toBeVisible();
  expect(documents.filter((u) => u.includes("/app/today"))).toEqual([]);
});
