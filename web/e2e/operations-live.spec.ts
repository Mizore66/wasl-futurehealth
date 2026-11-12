import { readFileSync } from "node:fs";
import { spawn, type ChildProcess } from "node:child_process";
import { createServer, type Server } from "node:http";
import path from "node:path";
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

/* Self-contained live API mock plus Next server; only synthetic values, fixed ports, Chromium only. */
const MOCK_PORT = 3299;
const NEXT_PORT = 3300;
const NEXT_URL = `http://127.0.0.1:${NEXT_PORT}`;
const SHARED = {
  WASL_API_REVIEWER_TOKEN: "synthetic-shared-reviewer-0001",
  WASL_API_WEB_REVIEWER_TOKEN: "synthetic-web-reviewer-00001",
  WASL_API_CLINICIAN_TOKEN: "synthetic-shared-clinician-0001",
  WASL_API_AUDITOR_TOKEN: "synthetic-shared-auditor-00001",
  WASL_API_COORDINATOR_TOKEN: "synthetic-shared-coord-000001",
};
const dir = path.join(process.cwd(), "data", "api");
const read = (file: string) => JSON.parse(readFileSync(path.join(dir, file), "utf8"));
const meta = { api: "wasl-api/1", data_mode: "simulated", run: "redacted-internal-audit", policy: "redacted-internal-audit", reader_model: "not-disclosed", loaded_at: "2026-11-13T11:00:00Z", note: "synthetic" };

const healthy = {
  schema: "wasl-operations/1", meta, observed_at: "2026-11-13T12:00:00Z",
  api: { status: "ok", source: "/healthz", dependencies: { materialised_run: "loaded", nats: "up", reader: "cached", ingest_adapter: "fhir", review_ledger: "ok" } },
  nats: { status: "reachable", source: "nats_jetstream_consumer_info" },
  queue: {
    status: "available", source: "nats_jetstream_consumer_info", coverage: "known_durable_consumers",
    consumers: [
      { stream: "FHIRIN", consumer: "notifications", status: "available", num_pending: 2, num_ack_pending: 1 },
      { stream: "FHIR", consumer: "prep", status: "available", num_pending: 3, num_ack_pending: 0 },
    ],
  },
  run: {
    status: "available", scope: "completed_synthetic_run", source: "report7.json",
    latency_ms: { n: 4, p50: 12.5, p90: 30.25, p99: 48.75, max: 50 },
    recovery: { generated_resources: 100, fhir_msgs: 120, expected_outputs: 80, live_outputs: 79, ingest_lost: 1, ingest_duplicates: 2, lost_outputs: 3, duplicate_outputs: 4 },
  },
  api_measurements: {
    latency: { status: "unmeasured", reason: "api_request_latency_not_recorded" },
    error_rate: { status: "unmeasured", reason: "api_request_outcomes_not_recorded" },
  },
  backup: { status: "available", source: "recovery_bundle_manifest", bundle_schema: "wasl-file-backup-v2-aesgcm", created_at: "2026-11-13T11:59:57Z", age_seconds: 3, verification_status: "not_verified_by_api" },
};

const degraded = {
  ...healthy,
  nats: { status: "unreachable", source: "nats_jetstream_consumer_info", reason: "jetstream_probe_failed" },
  queue: { status: "unavailable", source: "nats_jetstream_consumer_info", coverage: "known_durable_consumers", reason: "nats_unreachable", consumers: [] },
  run: { status: "unmeasured", scope: "completed_synthetic_run", source: "report7.json", reason: "run_metrics_not_recorded" },
  backup: { status: "unavailable", source: "recovery_bundle_manifest", reason: "backup_manifest_invalid" },
};

const missing = {
  ...healthy,
  queue: {
    status: "partial", source: "nats_jetstream_consumer_info", coverage: "known_durable_consumers", reason: "consumer_info_unavailable",
    consumers: [
      healthy.queue.consumers[0],
      { stream: "FHIR", consumer: "prep", status: "unavailable", reason: "consumer_not_found" },
    ],
  },
  run: { status: "unmeasured", scope: "completed_synthetic_run", source: "report7.json", reason: "run_metrics_not_recorded" },
  backup: { status: "unmeasured", source: "recovery_bundle_manifest", reason: "backup_manifest_not_configured" },
};

type Variant = "healthy" | "degraded" | "missing" | "invalid";
const payloads = { healthy, degraded, missing };

async function checkAccessibleAndNarrow(page: Page, name: string) {
  await page.setViewportSize({ width: 390, height: 844 });
  const panel = page.getByTestId("health-panel");
  await panel.scrollIntoViewIfNeeded();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  await page.screenshot({ path: `test-results/screens/operations-${name}-390.png`, fullPage: true });
  const axe = await new AxeBuilder({ page }).include('[data-testid="health-panel"]').analyze();
  expect(axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical").map((v) => v.id)).toEqual([]);
}

test.describe("synthetic operational readiness, live mock", () => {
  test.beforeEach(({}, testInfo) => test.skip(testInfo.project.name !== "chromium", "owns fixed ports: Chromium only"));
  test.describe.configure({ mode: "serial" });

  let server: Server;
  let nextServer: ChildProcess | undefined;
  let variant: Variant = "healthy";
  let operationsAuthorization = "";

  test.beforeAll(async ({}, testInfo) => {
    if (testInfo.project.name !== "chromium") return;
    server = createServer((req, res) => {
      const send = (status: number, body: unknown) => {
        res.writeHead(status, { "content-type": "application/json" });
        res.end(JSON.stringify(body));
      };
      const route = decodeURIComponent(new URL(req.url ?? "/", "http://mock").pathname);
      if (route.startsWith("/api/") && req.headers.authorization !== `Bearer ${SHARED.WASL_API_WEB_REVIEWER_TOKEN}` && !route.endsWith("/audit")) {
        return send(401, { error: "authentication required" });
      }
      if (route.endsWith("/audit")) {
        if (req.headers.authorization !== `Bearer ${SHARED.WASL_API_AUDITOR_TOKEN}`) return send(401, { error: "authentication required" });
        return send(200, read("case_evidence_closure_audit.json"));
      }
      if (route === "/api/operations") {
        operationsAuthorization = req.headers.authorization ?? "";
        if (variant === "invalid") return send(200, { schema: "wasl-operations/1", meta });
        return send(200, payloads[variant]);
      }
      if (route === "/api/fixtures") return send(200, read("fixtures.json"));
      if (route === "/api/dashboard") return send(200, read("dashboard.json"));
      if (route === "/api/cases") return send(200, { meta, cases: [], total: 0, limit: 50, offset: 0 });
      return send(404, { meta, error: "not found" });
    });
    await new Promise<void>((resolve) => server.listen(MOCK_PORT, "127.0.0.1", resolve));
    nextServer = spawn(path.join(process.cwd(), "node_modules", ".bin", "next"), ["start", "-p", String(NEXT_PORT)], {
      env: { ...process.env, ...SHARED, WASL_API_URL: `http://127.0.0.1:${MOCK_PORT}` }, stdio: "ignore",
    });
    for (let i = 0; i < 120; i++) {
      if (await fetch(`${NEXT_URL}/app`).then((r) => r.ok, () => false)) return;
      await new Promise((resolve) => setTimeout(resolve, 500));
    }
    throw new Error("next start did not become ready");
  });

  test.afterAll(async () => {
    nextServer?.kill();
    if (server) await new Promise((resolve) => server.close(resolve));
  });

  test.beforeEach(async ({ page }) => {
    variant = "healthy";
    operationsAuthorization = "";
    await page.emulateMedia({ reducedMotion: "reduce" });
  });

  test("healthy values, sources and timestamps match the API payload", async ({ page }) => {
    await page.goto(`${NEXT_URL}/app/pipeline`);
    const panel = page.getByTestId("health-panel");
    await expect(panel.getByRole("heading", { name: "Synthetic operational readiness" })).toBeVisible();
    await expect(panel.getByText("Live synthetic API check", { exact: true })).toBeVisible();
    await expect(panel.getByTestId("health-status")).toHaveText(healthy.api.status);
    await expect(panel.getByTestId("api-source")).toHaveText(healthy.api.source);
    await expect(panel.getByTestId("nats-status")).toHaveText(healthy.nats.status);
    await expect(panel.getByTestId("nats-source")).toHaveText(healthy.nats.source);
    await expect(panel.getByTestId("queue-FHIRIN-notifications")).toHaveText("2 pending, 1 unacknowledged");
    await expect(panel.getByTestId("queue-FHIR-prep")).toHaveText("3 pending, 0 unacknowledged");
    await expect(panel.getByTestId("api-latency")).toHaveText("Unmeasured");
    await expect(panel.getByTestId("api-error-rate")).toHaveText("Unmeasured");
    await expect(panel.getByTestId("run-latency-p50")).toHaveText("12.5 ms");
    await expect(panel.getByTestId("run-latency-p90")).toHaveText("30.25 ms");
    await expect(panel.getByTestId("run-source")).toHaveText(healthy.run.source);
    await expect(panel.getByTestId("run-ingest_lost")).toHaveText("1");
    await expect(panel.getByTestId("backup-created-at").locator("time")).toHaveAttribute("datetime", healthy.backup.created_at);
    await expect(panel.getByTestId("backup-age")).toHaveText("3 seconds");
    await expect(panel.getByTestId("backup-source")).toHaveText(healthy.backup.source);
    await expect(panel.getByTestId("observed-at").locator("time")).toHaveAttribute("datetime", healthy.observed_at);
    await expect(panel.getByTestId("loaded-at").locator("time")).toHaveAttribute("datetime", meta.loaded_at);
    expect(operationsAuthorization).toBe(`Bearer ${SHARED.WASL_API_WEB_REVIEWER_TOKEN}`);
    await checkAccessibleAndNarrow(page, "healthy");
  });

  test("degraded NATS and missing run/backup data render explicit states, not zeros", async ({ page }) => {
    variant = "degraded";
    await page.goto(`${NEXT_URL}/app/pipeline`);
    const panel = page.getByTestId("health-panel");
    await expect(panel.getByTestId("nats-status")).toHaveText("unreachable");
    await expect(panel.getByTestId("queue-status")).toHaveText("unavailable");
    await expect(panel.getByTestId("queue-unmeasured")).toHaveText("Unmeasured");
    await expect(panel.getByTestId("queue-FHIR-prep")).toHaveCount(0);
    await expect(panel.getByTestId("run-unmeasured")).toHaveText("Unmeasured");
    await expect(panel.getByTestId("run-latency-p50")).toHaveCount(0);
    await expect(panel.getByTestId("backup-status")).toHaveText("unavailable");
    await expect(panel.getByTestId("backup-age")).toHaveText("Unavailable");
    await expect(panel).not.toContainText("0 pending");
    await expect(panel).toContainText("not production monitoring");
    await expect(panel).toContainText("not production monitoring, a hospital live feed, proof of NATS cluster quorum, a verified restore, or an accumulated service-level objective");
    await expect(panel).not.toContainText(/SLO met|production monitoring active|hospital live feed active|cluster quorum verified|restore verified/i);
    await checkAccessibleAndNarrow(page, "degraded");
  });

  test("partial queue and unmeasured metadata preserve measured values without inventing missing ones", async ({ page }) => {
    variant = "missing";
    await page.goto(`${NEXT_URL}/app/pipeline`);
    const panel = page.getByTestId("health-panel");
    await expect(panel.getByTestId("queue-status")).toHaveText("partial");
    await expect(panel.getByTestId("queue-FHIRIN-notifications")).toHaveText("2 pending, 1 unacknowledged");
    await expect(panel.getByTestId("queue-FHIR-prep")).toHaveText("Unavailable");
    await expect(panel.getByTestId("queue-FHIR-prep")).not.toContainText("0 pending");
    await expect(panel.getByTestId("run-unmeasured")).toHaveText("Unmeasured");
    await expect(panel.getByTestId("backup-age")).toHaveText("Unmeasured");
    await checkAccessibleAndNarrow(page, "missing");
  });

  test("an invalid response replaces the whole panel and retains no healthy values", async ({ page }) => {
    variant = "invalid";
    await page.goto(`${NEXT_URL}/app/pipeline`);
    const panel = page.getByTestId("health-panel");
    await expect(panel.getByTestId("health-unavailable")).toContainText("Operational data unavailable");
    await expect(panel.getByTestId("health-status")).toHaveCount(0);
    await expect(panel.getByTestId("nats-status")).toHaveCount(0);
    await expect(panel.getByTestId("backup-age")).toHaveCount(0);
  });
});
