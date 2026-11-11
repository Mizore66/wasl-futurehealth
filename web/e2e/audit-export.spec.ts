import { readFileSync } from "node:fs";
import { createServer, type Server } from "node:http";
import path from "node:path";
import { expect, test } from "@playwright/test";

/*
 * The redacted evidence export on the case audit page (issue #46).
 *
 * Fixtures mode (the default run): there is no auditor API, so the action is disabled with the reason and the
 * download route serves no file.
 *
 * Live mode against a mock of the Go API: start `next start` yourself with WASL_API_URL=http://127.0.0.1:3295 and the
 * four role tokens (WASL_API_REVIEWER_TOKEN, WASL_API_CLINICIAN_TOKEN, WASL_API_AUDITOR_TOKEN,
 * WASL_API_COORDINATOR_TOKEN), then run this spec with AUDIT_EXPORT_LIVE_MOCK=1, PLAYWRIGHT_BASE_URL pointing at that
 * server and the same four tokens. The mock serves export bytes that the real Go handler produced
 * (web/tests/fixtures/export_redacted.json, pinned by pipeline/api_export_contract_test.go) and, like the Go API,
 * answers the export only to the auditor token.
 */
const LIVE = !!process.env.AUDIT_EXPORT_LIVE_MOCK;
const MOCK_PORT = 3295;

const dir = path.join(process.cwd(), "data", "api");
const read = (f: string) => JSON.parse(readFileSync(path.join(dir, f), "utf8"));
const fx = read("fixtures.json");
const EXPORT_BYTES = readFileSync(path.join(process.cwd(), "tests", "fixtures", "export_redacted.json"));
const exported = JSON.parse(EXPORT_BYTES.toString("utf8"));

const ALLOWED = fx.fixtures.evidence_closure.id as string;
const REFUSED = fx.fixtures.review_hold.id as string;
const auditHref = (id: string) => `/app/patients/${encodeURIComponent(id)}/audit`;

test.describe("redacted evidence export, fixtures mode", () => {
  test.skip(LIVE, "fixtures-mode checks run against the default server without WASL_API_URL");

  test("the action is disabled with the reason, after the chain, and the route serves no file", async ({ page, request }) => {
    const res = await page.goto(auditHref(ALLOWED));
    expect(res?.status()).toBe(200);
    const card = page.getByRole("region", { name: "Redacted evidence export" });
    await expect(card).toBeVisible();
    await expect(card.locator("[data-honesty]")).toHaveText("Synthetic data, redacted");
    await expect(card.getByRole("status")).toContainText("no auditor API");
    await expect(card.getByRole("button", { name: "Download redacted export (JSON)" })).toBeDisabled();
    await expect(card.getByRole("link")).toHaveCount(0);
    await expect(card.getByTestId("export-content-hash")).toHaveCount(0);
    // Secondary to chain inspection: it comes after the chain cards in the page.
    const after = await page.evaluate(() => {
      const chain = document.getElementById("chain");
      const exp = document.querySelector("[data-testid=audit-export]");
      return !!chain && !!exp && !!(chain.compareDocumentPosition(exp) & Node.DOCUMENT_POSITION_FOLLOWING);
    });
    expect(after).toBe(true);

    const direct = await request.get(`${auditHref(ALLOWED)}/export`);
    expect(direct.status()).toBe(404);
    expect(direct.headers()["content-disposition"]).toBeUndefined();
    expect((await direct.json()).error).toContain("no auditor API");
  });
});

test.describe("redacted evidence export, live mode against a mock API", () => {
  test.skip(!LIVE, "set AUDIT_EXPORT_LIVE_MOCK=1 with the server pointed at the mock");
  test.describe.configure({ mode: "serial" });

  const principals = new Map<string, string>();
  const exportCalls: string[] = [];
  let server: Server;

  test.beforeAll(async () => {
    for (const role of ["reviewer", "clinician", "auditor", "coordinator"]) {
      const name = `WASL_API_${role.toUpperCase()}_TOKEN`;
      const token = process.env[name]?.trim();
      if (!token) throw new Error(`${name} must be set for the live mock, with the same value the server sends`);
      principals.set(token, role);
    }
    const details = new Map<string, unknown>();
    const audits = new Map<string, unknown>();
    for (const name of ["evidence_closure", "false_closure_trap", "review_hold", "dr_exam_closure"]) {
      details.set(fx.fixtures[name].id, read(`case_${name}.json`));
      audits.set(fx.fixtures[name].id, read(`case_${name}_audit.json`));
    }
    const meta = fx.meta;
    server = createServer((req, res) => {
      const url = new URL(req.url ?? "/", "http://mock");
      const send = (status: number, body: unknown) => {
        res.writeHead(status, { "content-type": "application/json" });
        res.end(typeof body === "string" || body instanceof Buffer ? body : JSON.stringify(body));
      };
      const p = decodeURIComponent(url.pathname);
      if (p === "/healthz") return send(200, read("healthz.json"));
      const [scheme, token] = (req.headers.authorization ?? "").split(" ");
      const role = scheme === "Bearer" && token ? principals.get(token) : undefined;
      if (!role) return send(401, { error: "authentication required" });
      const auditor = role === "auditor";
      const leaky = { error: "rbac: role may not read_audit", meta: { ...meta, run: "CANARY-META-RUN", policy: "CANARY-META-POLICY" } };
      if (p.endsWith("/export")) {
        const id = p.replace(/^\/api\/cases\//, "").replace(/\/export$/, "");
        exportCalls.push(`${role} ${id}`);
        if (!auditor || id === REFUSED) return send(403, leaky);
        if (!details.has(id)) return send(404, { error: "unknown case", id, meta });
        return send(200, EXPORT_BYTES);
      }
      if (p.endsWith("/audit")) {
        if (!auditor) return send(403, leaky);
        const a = audits.get(p.replace(/^\/api\/cases\//, "").replace(/\/audit$/, ""));
        return a ? send(200, a) : send(404, { error: "unknown case", meta });
      }
      if (auditor) return send(403, leaky);
      if (p === "/api/dashboard") return send(200, read("dashboard.json"));
      if (p === "/api/fixtures") return send(200, fx);
      if (p === "/api/cases") {
        const cases = Object.values(fx.fixtures);
        return send(200, { meta, cases, total: cases.length, limit: 500, offset: 0 });
      }
      const d = details.get(p.replace(/^\/api\/cases\//, ""));
      return d ? send(200, d) : send(404, { error: "unknown case", meta });
    });
    await new Promise<void>((resolve) => server.listen(MOCK_PORT, "127.0.0.1", resolve));
  });

  test.afterAll(async () => {
    await new Promise((resolve) => server.close(resolve));
  });

  test.beforeEach(() => {
    exportCalls.length = 0;
  });

  test("an auditor downloads the backend's bytes unchanged, with the hashes shown on the page", async ({ page }) => {
    await page.goto(auditHref(ALLOWED));
    const card = page.getByRole("region", { name: "Redacted evidence export" });
    await expect(card.getByTestId("export-content-hash")).toHaveText(exported.content_sha256);
    await expect(card.getByTestId("export-comparison-hash")).toHaveText(exported.comparison_sha256);
    await expect(card.locator("[data-honesty]")).toHaveText("Synthetic data, redacted");

    const [download] = await Promise.all([page.waitForEvent("download"), card.getByRole("link", { name: "Download redacted export (JSON)" }).click()]);
    expect(download.suggestedFilename()).toBe(`wasl-synthetic-redacted-export_${ALLOWED.replaceAll("/", "_")}.json`);
    const saved = readFileSync(await download.path());
    expect(saved.equals(EXPORT_BYTES)).toBe(true);
    expect(JSON.parse(saved.toString("utf8")).content_sha256).toBe(exported.content_sha256);
    // The page's hash read and the download both used the auditor credential, and only it.
    expect(exportCalls).toEqual([`auditor ${ALLOWED}`, `auditor ${ALLOWED}`]);
  });

  test("a refused export shows fixed wording, no backend metadata, and no download", async ({ page, request }) => {
    await page.goto(auditHref(REFUSED));
    const card = page.getByRole("region", { name: "Redacted evidence export" });
    await expect(card.getByRole("status")).toHaveText("The Wasl API refused the export. Only the auditor role may download it.");
    await expect(card.getByRole("button", { name: "Download redacted export (JSON)" })).toBeDisabled();
    await expect(page.locator("main")).not.toContainText("CANARY");
    await expect(page.locator("main")).not.toContainText("rbac");

    const direct = await request.get(`${auditHref(REFUSED)}/export`);
    expect(direct.status()).toBe(403);
    expect(direct.headers()["content-disposition"]).toBeUndefined();
    const text = await direct.text();
    expect(text).not.toMatch(/CANARY|meta|rbac/);
  });
});
