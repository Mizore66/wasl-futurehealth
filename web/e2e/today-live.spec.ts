import { readFileSync } from "node:fs";
import { createServer, type Server } from "node:http";
import path from "node:path";
import { expect, test } from "@playwright/test";

/*
 * Today against a mock of the Go API (issue #47): what a reload shows comes from the ledger, not from browser state.
 * Start `next start` yourself with WASL_API_URL=http://127.0.0.1:3294 and the four role tokens (WASL_API_REVIEWER_TOKEN,
 * WASL_API_CLINICIAN_TOKEN, WASL_API_AUDITOR_TOKEN, WASL_API_COORDINATOR_TOKEN), then run this spec with
 * TODAY_LIVE_MOCK=1, PLAYWRIGHT_BASE_URL pointing at that server and the same four tokens.
 *
 * The mock follows the Go API's contract for what Today uses (pipeline/api.go caseAction, overlayViewLocked; docs/API.md
 * "POST actions"): identity only from the bearer token, expected_head checks, the summary overlay on the list (with
 * last_contact and no_answer_streak), the full overlay and review_ledger on detail, and 500 on writes once replay failed.
 * It is a mock: the real handler's output is pinned separately by pipeline/api_ledger_contract_test.go.
 */
const LIVE = !!process.env.TODAY_LIVE_MOCK;
const MOCK_PORT = 3294;

type Ev = { kind: string; data: Record<string, string>; actor: string; at: string; hash: string; head: string; seq: number; prev: string };

test.describe("Today, live mode against a mock API", () => {
  test.skip(!LIVE, "set TODAY_LIVE_MOCK=1 with the server pointed at the mock");
  test.describe.configure({ mode: "serial" });

  const dir = path.join(process.cwd(), "data", "api");
  const read = (f: string) => JSON.parse(readFileSync(path.join(dir, f), "utf8"));
  const fx = read("fixtures.json");
  const meta = fx.meta;
  const details: Record<string, unknown>[] = ["case_evidence_closure.json", "case_false_closure_trap.json", "case_review_hold.json", "case_dr_exam_closure.json"].map(read);
  const template = details[0] as { case: Record<string, unknown> };
  const byId = new Map<string, Record<string, unknown>>();
  for (const c of [...Object.values(fx.fixtures), ...read("cases_crc_page1.json").cases] as Record<string, unknown>[]) if (!byId.has(c.id as string)) byId.set(c.id as string, c);

  const ledger = new Map<string, Ev[]>();
  let chain: Ev[] = [];
  let invalid: string | null = null;
  /** Case details the mock answers with 503, as if the API were briefly down for them. */
  const unavailable = new Set<string>();
  /** Case details the mock holds back until `release` runs, so a test can see what Today shows while a read is running. */
  const held = new Map<string, Promise<void>>();
  let release = () => {};
  function hold(id: string) {
    held.set(id, new Promise<void>((resolve) => (release = () => (held.delete(id), resolve()))));
  }
  /** When set, the mock answers coordinator writes with the Go API's 401, as if the server's token were not accepted. */
  let refuseWrites = false;
  let n = 0;
  const principals = new Map<string, { role: string; actor: string }>();
  let server: Server;

  const head = (id: string) => ledger.get(id)?.at(-1)?.hash ?? `sig-${id}`;
  function append(id: string, kind: string, data: Record<string, string>, actor: string, at = new Date().toISOString()): Ev {
    const evs = ledger.get(id) ?? [];
    const ev = { kind, data, actor, at, hash: `${(++n).toString(16).padStart(8, "0")}${"ab".repeat(28)}`, head: head(id), seq: evs.length + 1, prev: chain.at(-1)?.hash ?? "genesis" };
    ledger.set(id, [...evs, ev]);
    chain.push(ev);
    return ev;
  }
  const entry = (e: Ev): Record<string, string> => ({ at: e.at, actor: e.actor, event: e.hash, ...e.data });
  function overlay(id: string, full: boolean) {
    const evs = ledger.get(id) ?? [];
    const contacts = evs.filter((e) => e.kind === "contact_outcome").map(entry);
    const m: Record<string, unknown> = {
      head: head(id),
      actions: evs.length,
      owner: [...evs].reverse().find((e) => e.kind === "assign_owner")?.data.owner ?? "",
      last_action_at: evs.at(-1)?.at ?? "",
      ledger_status: invalid ? "invalid" : "ok",
    };
    const snooze = [...evs].reverse().find((e) => e.kind === "snooze");
    if (snooze) m.snooze = entry(snooze);
    const last = contacts.at(-1);
    if (last) {
      m.last_contact = { at: last.at, event: last.event, outcome: last.outcome, channel: last.channel };
      let streak = 0;
      for (let i = contacts.length - 1; i >= 0 && contacts[i].outcome === "no_answer"; i--) streak++;
      m.no_answer_streak = streak;
    }
    if (full) {
      m.contact_outcomes = contacts;
      m.notes = evs.filter((e) => e.kind === "note").map(entry);
      m.canonical_owner = byId.get(id)?.owner ?? "";
    }
    return m;
  }
  const apiEvent = (id: string, e: Ev) => ({
    type: "coordinator_action", kind: e.kind, case: id, data: e.data, role: "coordinator", actor: e.actor, action: "coordinate_case",
    at: e.at, head: e.head, seq: e.seq, mode: "demo", prev: e.prev, hash: e.hash,
  });

  test.beforeAll(async () => {
    for (const role of ["reviewer", "clinician", "auditor", "coordinator"]) {
      const name = `WASL_API_${role.toUpperCase()}_TOKEN`;
      const token = process.env[name]?.trim();
      if (!token) throw new Error(`${name} must be set for the live mock, with the same value the server sends`);
      principals.set(token, { role, actor: `mock-${role}` });
    }
    server = createServer((req, res) => {
      const url = new URL(req.url ?? "/", "http://mock");
      const send = (status: number, body: unknown) => {
        res.writeHead(status, { "content-type": "application/json" });
        res.end(JSON.stringify(body));
      };
      const p = decodeURIComponent(url.pathname);
      if (p === "/healthz") return send(200, read("healthz.json"));
      const [scheme, token] = (req.headers.authorization ?? "").split(" ");
      const principal = scheme === "Bearer" && token ? principals.get(token) : undefined;
      if (!principal) return send(401, { error: "authentication required" });
      if (req.method === "POST" && p.endsWith("/actions")) {
        if (refuseWrites) return send(401, { error: "authentication required" });
        const id = p.replace(/^\/api\/cases\//, "").replace(/\/actions$/, "");
        let raw = "";
        req.on("data", (c) => (raw += c));
        req.on("end", () => {
          const { action, expected_head, ...data } = JSON.parse(raw);
          if (principal.role !== "coordinator") return send(403, { meta, error: "rbac: role may not coordinate_case" });
          if (invalid) return send(500, { meta, error: "review ledger failed validation; writes refused", detail: invalid });
          if (expected_head !== head(id)) return send(409, { meta, error: "stale expected_head", conflict: "head", current_head: head(id) });
          if (action === "contact_outcome" && byId.get(id)?.consent !== "granted") return send(409, { meta, error: "contact outcomes need contact permission granted", conflict: "consent" });
          const ev = append(id, action, data, principal.actor);
          send(201, { meta, event: apiEvent(id, ev), api_overlay: overlay(id, true) });
        });
        return;
      }
      if (p === "/api/dashboard") return send(200, read("dashboard.json"));
      if (p === "/api/fixtures") return send(200, fx);
      if (p === "/api/cases") {
        const q = url.searchParams;
        const cases = [...byId.values()]
          .filter((c) => (!q.get("provider") || c.provider === q.get("provider")) && (!q.get("programme") || c.programme === q.get("programme")))
          .map((c) => ({ ...c, api_overlay: overlay(c.id as string, false) }));
        return send(200, { meta, cases, total: cases.length, limit: 500, offset: 0 });
      }
      const id = p.replace(/^\/api\/cases\//, "");
      const summary = byId.get(id);
      const known = details.find((d) => (d.case as { id: string }).id === id);
      if (!summary && !known) return send(404, { meta, error: "unknown case" });
      const gate = held.get(id);
      if (gate) return void gate.then(() => answerDetail(id, summary, known, send));
      return answerDetail(id, summary, known, send);
    });
    await new Promise<void>((resolve) => server.listen(MOCK_PORT, "127.0.0.1", resolve));
  });

  /** The case-detail answer, sent now or once a held read is released (it reads the ledger state at that moment). */
  function answerDetail(id: string, summary: Record<string, unknown> | undefined, known: Record<string, unknown> | undefined, send: (status: number, body: unknown) => void) {
    if (unavailable.has(id)) return send(503, { error: "unavailable" });
    const base = known ?? { ...template, case: summary };
    return send(200, {
      ...base,
      case: { ...(base.case as object), api_overlay: overlay(id, false) },
      api_reviews: (ledger.get(id) ?? []).map((e) => apiEvent(id, e)),
      api_overlay: overlay(id, true),
      review_ledger: invalid ? { status: "invalid", detail: invalid, events_applied: chain.length, note: "api_overlay reflects only the ledger events before the failure; coordinator writes are refused" } : { status: "ok", events_applied: chain.length },
    });
  }

  test.afterAll(async () => {
    // A read still held by a failed test would keep its connection open, and close() waits for it.
    release();
    await new Promise((resolve) => server.close(resolve));
  });

  test.beforeEach(() => {
    ledger.clear();
    chain = [];
    invalid = null;
    unavailable.clear();
    release();
    held.clear();
    refuseWrites = false;
  });

  const first = "ahs-mushrif/ServiceRequest/AHS-sr-1002";
  const second = "ahs-mushrif/ServiceRequest/AHS-sr-1023";

  test("a reload shows the saved outcomes and notes, by the API's actor, and the board position they set", async ({ page }) => {
    append(second, "note", { text: "Prefers a morning call" }, "seed-coordinator", "2026-11-09T08:00:00.000Z");
    append(second, "contact_outcome", { outcome: "reached", channel: "phone", note: "Will check her diary" }, "seed-coordinator", "2026-11-09T08:05:00.000Z");

    await page.goto("/app/today");
    await expect(page.locator("[data-honesty=built]")).toHaveText("Call log: saved to the case ledger");
    const open = page.getByRole("region", { name: "Open patient" });
    await expect(open.getByRole("heading", { level: 2 })).toHaveText("AHS-sr-1002");
    await expect(open).toContainText("No contact on the case ledger yet");
    await expect(page.getByRole("tab", { name: "To call 8" })).toBeVisible();

    await open.getByRole("button", { name: "Reached" }).click();
    await expect(open.getByRole("status")).toContainText("Logged: Reached");
    await open.getByRole("button", { name: "Declined" }).click();
    await expect(open.getByTestId("removed-notice")).toContainText("Logged: Declined · AHS-sr-1002 moved to Waiting for the GP");
    expect(ledger.get(first)?.map((e) => [e.kind, e.data.outcome, e.actor])).toEqual([
      ["contact_outcome", "reached", "mock-coordinator"],
      ["contact_outcome", "declined", "mock-coordinator"],
    ]);

    await page.reload();
    // Placement after the reload comes from the persisted outcome: the declined patient is not back in To call.
    await expect(page.getByRole("tab", { name: "To call 7" })).toBeVisible();
    await page.getByRole("tab", { name: /^Waiting/ }).click();
    const row = page.getByRole("region", { name: "Your list" }).getByRole("button", { name: /AHS-sr-1002/ });
    await expect(row).toContainText("Declined · GP to sign");
    await row.click();
    const trail = open.getByRole("list").last();
    await expect(open).toContainText("Reached (phone)");
    await expect(open).toContainText("Declined the procedure (phone)");
    await expect(open).toContainText("mock-coordinator");
    await expect(open).not.toContainText("earlier action");
    await expect(trail).not.toContainText("This browser");

    // Another case's earlier note and outcome, written before this browser ever opened it.
    await page.getByRole("tab", { name: /^To call/ }).click();
    await page.getByRole("region", { name: "Your list" }).getByRole("button", { name: /AHS-sr-1023/ }).click();
    await expect(open).toContainText("Note: “Prefers a morning call”");
    await expect(open).toContainText("Reached (phone): “Will check her diary”");
    await expect(open).toContainText("seed-coordinator");
  });

  test("a saved Reached outcome keeps its follow-up choices after a reload and after switching patients (issue #131)", async ({ page }) => {
    const choices = ["Appointment agreed", "Call back on…", "Declined"];
    const outcomes = (id: string) => ledger.get(id)?.map((e) => e.data.outcome);
    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Open patient" });
    const list = page.getByRole("region", { name: "Your list" });
    await open.getByRole("button", { name: "Reached" }).click();
    await expect(open.getByRole("group", { name: "What next?" }).getByRole("button")).toHaveText(choices);

    await page.reload();
    await expect(open.getByRole("heading", { level: 2 })).toHaveText("AHS-sr-1002");
    await expect(open.getByRole("group", { name: "What next?" }).getByRole("button")).toHaveText(choices);
    await expect(open.getByText("Reached (phone)")).toHaveCount(1);

    await list.getByRole("button", { name: /AHS-sr-1023/ }).click();
    await expect(open.getByRole("group", { name: "After the call" })).toBeVisible();
    await list.getByRole("button", { name: /AHS-sr-1002/ }).click();
    await expect(open.getByRole("group", { name: "What next?" }).getByRole("button")).toHaveText(choices);

    await open.getByRole("button", { name: "Appointment agreed" }).click();
    await expect(open.getByTestId("removed-notice")).toContainText("Logged: Appointment agreed");
    expect(outcomes(first)).toEqual(["reached", "appointment_agreed"]);

    await page.reload();
    await page.getByRole("tab", { name: /^Waiting/ }).click();
    await list.getByRole("button", { name: /AHS-sr-1002/ }).click();
    await expect(open.getByRole("group", { name: "What next?" })).toHaveCount(0);
    await expect(open.getByRole("group", { name: "After the call" })).toBeVisible();
  });

  test("a restored Reached follow-up saves a call back or a decline without another Reached (issue #131)", async ({ page }) => {
    append(first, "contact_outcome", { outcome: "reached", channel: "phone" }, "seed-coordinator", "2026-11-09T08:00:00.000Z");
    append(second, "contact_outcome", { outcome: "reached", channel: "phone" }, "seed-coordinator", "2026-11-09T08:00:00.000Z");
    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Open patient" });
    await open.getByRole("button", { name: "Call back on…" }).click();
    await open.getByRole("group", { name: "Call back on" }).getByRole("button").first().click();
    await expect(open.getByRole("status")).toContainText("Logged: Call back");
    expect(ledger.get(first)?.map((e) => e.kind)).toEqual(["contact_outcome", "snooze"]);

    await page.reload();
    await page.getByRole("region", { name: "Your list" }).getByRole("button", { name: /AHS-sr-1023/ }).click();
    await open.getByRole("button", { name: "Declined" }).click();
    await expect(open.getByTestId("removed-notice")).toContainText("Logged: Declined");
    expect(ledger.get(second)?.map((e) => e.data.outcome)).toEqual(["reached", "declined"]);
  });

  test("a partial ledger replay shows only the valid prefix under a warning and disables writes", async ({ page }) => {
    append(first, "contact_outcome", { outcome: "no_answer", channel: "phone" }, "seed-coordinator", "2026-11-09T08:00:00.000Z");
    append(first, "note", { text: "Left a message with her son" }, "seed-coordinator", "2026-11-09T08:01:00.000Z");
    invalid = "line 3: head does not follow the case's previous event";

    await page.goto("/app/today");
    // Replay failure is server-wide: the board says so before any patient's detail is read.
    const board = page.getByTestId("ledger-invalid-board");
    await expect(board).toHaveAttribute("role", "alert");
    await expect(board).toContainText("nothing on this board is authoritative");
    const open = page.getByRole("region", { name: "Open patient" });
    await page.getByRole("region", { name: "Your list" }).getByRole("button", { name: /AHS-sr-1002/ }).click();
    const warning = open.getByTestId("ledger-invalid");
    await expect(warning).toBeVisible();
    // The board banner is the one alert; the per-patient box adds detail without a second announcement.
    await expect(warning).not.toHaveAttribute("role", "alert");
    // (Next.js keeps an empty route-announcer alert on every page, so count only alerts about the ledger.)
    await expect(page.getByRole("alert").filter({ hasText: /ledger/i })).toHaveCount(1);
    await expect(warning).toContainText("not authoritative");
    await expect(warning).toContainText("API: line 3: head does not follow the case's previous event");
    await expect(open).toContainText("No answer (phone)");
    await expect(open).toContainText("Note: “Left a message with her son”");
    await expect(open.getByRole("button", { name: "Snooze to tomorrow" })).toBeDisabled();
    await expect(open.getByRole("button", { name: "Reached" })).toBeDisabled();
    await expect(open.getByRole("button", { name: /^Hand to/ })).toBeDisabled();

    // A row the open patient's detail never touched is no more authoritative, and the board still says so.
    await page.getByRole("region", { name: "Your list" }).getByRole("button", { name: /AHS-sr-1023/ }).click();
    await expect(board).toBeVisible();
    await expect(open.getByRole("button", { name: "Snooze to tomorrow" })).toBeDisabled();

    await page.goto("/app/patients");
    await expect(page.getByTestId("ledger-invalid-board")).toContainText("nothing on this list is authoritative");
  });

  test("a failed ledger read is not kept: Retry and reselecting the case read it again", async ({ page }) => {
    append(first, "note", { text: "Prefers a morning call" }, "seed-coordinator", "2026-11-09T08:00:00.000Z");
    unavailable.add(first);

    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Open patient" });
    const failed = open.getByTestId("ledger-read-failed");
    await expect(failed).toContainText("Could not read the case ledger");
    await expect(open).not.toContainText("Prefers a morning call");

    // Retry while the API is still down fails again, and says so again.
    await failed.getByRole("button", { name: "Retry" }).click();
    await expect(failed).toBeVisible();

    unavailable.delete(first);
    await failed.getByRole("button", { name: "Retry" }).click();
    await expect(open).toContainText("Note: “Prefers a morning call”");
    await expect(failed).toHaveCount(0);

    // Reselection reads a failed case again, without Retry.
    unavailable.add(second);
    const list = page.getByRole("region", { name: "Your list" });
    await list.getByRole("button", { name: /AHS-sr-1023/ }).click();
    await expect(failed).toBeVisible();
    unavailable.delete(second);
    await list.getByRole("button", { name: /AHS-sr-1002/ }).click();
    await list.getByRole("button", { name: /AHS-sr-1023/ }).click();
    await expect(failed).toHaveCount(0);
    await expect(open).toContainText("No contact on the case ledger yet");
  });

  test("reselecting a failed case by search shows 'Reading…' while it is read again, not the old error", async ({ page }) => {
    append(first, "note", { text: "Prefers a morning call" }, "seed-coordinator", "2026-11-09T08:00:00.000Z");
    unavailable.add(first);

    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Open patient" });
    const failed = open.getByTestId("ledger-read-failed");
    await expect(failed).toContainText("Could not read the case ledger");

    // The API is back but slow for this case. Search away from it and back: no row click, no Retry.
    unavailable.delete(first);
    hold(first);
    const search = page.getByRole("searchbox", { name: "Search" });
    await search.fill("1023");
    await expect(open.getByRole("heading", { level: 2 })).toHaveText("AHS-sr-1023");
    await search.fill("");
    await expect(open.getByRole("heading", { level: 2 })).toHaveText("AHS-sr-1002");
    await expect(open).toContainText("Reading the case ledger…");
    await expect(failed).toHaveCount(0);
    await expect(open.getByRole("button", { name: "Retry" })).toHaveCount(0);

    release();
    await expect(open).toContainText("Note: “Prefers a morning call”");
    await expect(failed).toHaveCount(0);
  });

  test("the board warning follows the latest read: a restored ledger clears it, a failed one sets it again", async ({ page }) => {
    invalid = "append failed and the ledger file state is unknown: disk full";
    await page.goto("/app/today");
    const board = page.getByTestId("ledger-invalid-board");
    const open = page.getByRole("region", { name: "Open patient" });
    await expect(board).toBeVisible();
    await expect(open.getByTestId("ledger-invalid")).toContainText("API: append failed and the ledger file state is unknown");
    await expect(open.getByRole("button", { name: "Snooze to tomorrow" })).toBeDisabled();

    // An operator restores the ledger. The next read (selecting another case) clears the warning without a reload.
    invalid = null;
    const list = page.getByRole("region", { name: "Your list" });
    await list.getByRole("button", { name: /AHS-sr-1023/ }).click();
    await expect(open.getByRole("heading", { level: 2 })).toHaveText("AHS-sr-1023");
    await expect(board).toHaveCount(0);
    await expect(open.getByTestId("ledger-invalid")).toHaveCount(0);
    await expect(open.getByRole("button", { name: "Snooze to tomorrow" })).toBeEnabled();

    // Reselecting a case reads it again: a ledger that fails again brings the warning back.
    invalid = "ledger changed under another writer: hash mismatch";
    await list.getByRole("button", { name: /AHS-sr-1002/ }).click();
    await expect(board).toBeVisible();
    await expect(open.getByTestId("ledger-invalid")).toContainText("API: ledger changed under another writer");
  });

  test("a 401 on a write is a refusal that names the credential, not 'the API did not answer'", async ({ page }) => {
    refuseWrites = true;
    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Open patient" });
    await expect(open).toContainText("No contact on the case ledger yet");
    await open.getByRole("button", { name: "Snooze to tomorrow" }).click();
    const refused = open.getByTestId("action-refused");
    await expect(refused).toContainText("Not saved: the API did not accept this server's coordinator credential");
    await expect(refused).not.toContainText("did not answer");
    expect(ledger.get(first)).toBeUndefined();
  });

  test("a valid ledger shows no board-level warning", async ({ page }) => {
    await page.goto("/app/today");
    await expect(page.getByRole("region", { name: "Open patient" }).getByRole("heading", { level: 2 })).toBeVisible();
    await expect(page.getByTestId("ledger-invalid-board")).toHaveCount(0);
  });
});
