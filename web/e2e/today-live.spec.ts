import { readFileSync } from "node:fs";
import { createServer, type Server } from "node:http";
import path from "node:path";
import { expect, test } from "@playwright/test";

/*
 * Today against a mock of the Go API (issue #47): what a reload shows comes from the ledger, not from browser state.
 * Start `next start` yourself with WASL_API_URL=http://127.0.0.1:3294 (or TODAY_LIVE_MOCK_PORT) and the four role tokens (WASL_API_REVIEWER_TOKEN,
 * WASL_API_CLINICIAN_TOKEN, WASL_API_AUDITOR_TOKEN, WASL_API_COORDINATOR_TOKEN), then run this spec with
 * TODAY_LIVE_MOCK=1, PLAYWRIGHT_BASE_URL pointing at that server and the same four tokens.
 *
 * The mock follows the Go API's contract for what Today uses (pipeline/api.go caseAction, overlayViewLocked; docs/API.md
 * "POST actions"): identity only from the bearer token, expected_head checks, the summary overlay on the list (with
 * last_contact and no_answer_streak), the full overlay and review_ledger on detail, and 500 on writes once replay failed.
 * It is a mock: the real handler's output is pinned separately by pipeline/api_ledger_contract_test.go.
 */
const LIVE = !!process.env.TODAY_LIVE_MOCK;
/** Today's queues are links under Today in the sidebar, each with its live count. */
const queues = (page: import("@playwright/test").Page) => page.getByRole("navigation", { name: "Main", exact: true });
// Overridable so parallel checkouts can run this spec at once; the server must point WASL_API_URL at the same port.
const MOCK_PORT = Number(process.env.TODAY_LIVE_MOCK_PORT ?? 3294);

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
  // Every programme's first page, so a non-CRC queue (issue #114) can be selected and written through the same action.
  const pages = ["crc", "cervical", "lung_nodule", "diabetic_retinopathy", "breast", "lung_ldct", "ifhas"].flatMap((p) => read(`cases_${p}_page1.json`).cases);
  for (const c of [...Object.values(fx.fixtures), ...pages] as Record<string, unknown>[]) if (!byId.has(c.id as string)) byId.set(c.id as string, c);

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
    const notification = evs.find((e) => e.kind === "initial_notification");
    const receipt = evs.find((e) => e.kind === "receipt_acknowledgement");
    if (notification || receipt) {
      const canonical = (byId.get(id)?.dropoff as string) ?? "";
      const fact = (e: Ev, keys: string[]) => ({ at: e.at, event: e.hash, ...Object.fromEntries(keys.map((k) => [k, e.data[k]])), ...(full ? { actor: e.actor, identity_check: e.data.identity_check } : {}) });
      m.communication = {
        canonical_dropoff: canonical,
        obligation: servedDropoff(id, canonical),
        understanding: "none",
        ...(notification ? { notification: { ...fact(notification, ["occurred_at", "channel", "template"]), language: "en" } } : {}),
        ...(receipt ? { receipt: { ...fact(receipt, ["occurred_at", "channel"]), notification_event: notification?.hash ?? "canonical" } } : {}),
      };
    }
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
  /**
   * The Go API's communication projection (pipeline/api_communication.go): a staff notification moves never_notified to
   * no_acknowledgement, a receipt moves that to referral_unanswered; the approved wording is served while a notification
   * is owed. Only those two actions move it: a contact outcome or note never does.
   */
  const recorded = (id: string, kind: string) => (ledger.get(id) ?? []).some((e) => e.kind === kind);
  function servedDropoff(id: string, canonical: string): string {
    if (canonical === "never_notified" && recorded(id, "initial_notification")) return recorded(id, "receipt_acknowledgement") ? "referral_unanswered" : "no_acknowledgement";
    if (canonical === "no_acknowledgement" && recorded(id, "receipt_acknowledgement")) return "referral_unanswered";
    return canonical;
  }
  function served(c: Record<string, unknown>): Record<string, unknown> {
    const id = c.id as string;
    const dropoff = servedDropoff(id, c.dropoff as string);
    const out: Record<string, unknown> = { ...c, dropoff, api_overlay: overlay(id, false) };
    if (c.programme === "crc" && dropoff === "never_notified" && c.consent === "granted") out.notification_template = "fitfu-notify-v2/en";
    return out;
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
          if (action === "initial_notification" || action === "receipt_acknowledgement") {
            const c = byId.get(id);
            const owed = action === "initial_notification" ? "never_notified" : "no_acknowledgement";
            if (c?.consent !== "granted") return send(409, { meta, error: "recording a patient communication needs canonical contact permission granted", conflict: "consent" });
            if (servedDropoff(id, (c?.dropoff as string) ?? "") !== owed) return send(409, { meta, error: "no such communication is outstanding on this case", conflict: "communication" });
          }
          if (action === "assign_owner" && data.owner === undefined) data.owner = principal.actor;
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
          .filter((c) => !q.has("closed") || String(c.closed) === q.get("closed"))
          .map(served);
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
      case: served({ ...(base.case as object), ...(summary ?? {}) }),
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
    const open = page.getByRole("region", { name: "Selected case" });
    await expect(open).toHaveAttribute("data-case", "AHS-sr-1002");
    await expect(page.getByTestId("today-open")).toContainText("No contact recorded yet");
    await expect(queues(page).getByRole("link", { name: "To call 8" })).toBeVisible();

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
    await expect(queues(page).getByRole("link", { name: "To call 7" })).toBeVisible();
    await queues(page).getByRole("link", { name: /^Waiting/ }).click();
    const row = page.locator("button[data-case-row]").filter({ hasText: /AHS-sr-1002/ });
    await expect(row).toContainText("Declined · GP to sign");
    await row.click();
    const trail = page.getByRole("region", { name: "Case history" }).getByRole("list");
    await expect(page.getByTestId("today-open")).toContainText("Reached (phone)");
    await expect(page.getByTestId("today-open")).toContainText("Declined the procedure (phone)");
    await expect(page.getByTestId("today-open")).toContainText("mock-coordinator");
    await expect(page.getByTestId("today-open")).not.toContainText("earlier action");
    await expect(trail).not.toContainText("This browser");

    // Another case's earlier note and outcome, written before this browser ever opened it.
    await queues(page).getByRole("link", { name: /^To call/ }).click();
    await page.locator("button[data-case-row]").filter({ hasText: /AHS-sr-1023/ }).click();
    await expect(page.getByTestId("today-open")).toContainText("Note: “Prefers a morning call”");
    await expect(page.getByTestId("today-open")).toContainText("Reached (phone): “Will check her diary”");
    await expect(page.getByTestId("today-open")).toContainText("seed-coordinator");
  });

  test("a saved Reached outcome keeps its follow-up choices after a reload and after switching patients (issue #131)", async ({ page }) => {
    const choices = ["Appointment agreed", "Call back on…", "Done elsewhere…", "Declined"];
    const outcomes = (id: string) => ledger.get(id)?.map((e) => e.data.outcome);
    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Selected case" });
    const list = page.getByTestId("today-sheet");
    await open.getByRole("button", { name: "Reached" }).click();
    await expect(open.getByRole("group", { name: "What next?" }).getByRole("button")).toHaveText(choices);

    await page.reload();
    await expect(open).toHaveAttribute("data-case", "AHS-sr-1002");
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
    await queues(page).getByRole("link", { name: /^Waiting/ }).click();
    await list.getByRole("button", { name: /AHS-sr-1002/ }).click();
    await expect(open.getByRole("group", { name: "What next?" })).toHaveCount(0);
    await expect(open.getByRole("group", { name: "After the call" })).toBeVisible();
  });

  test("a restored Reached follow-up saves a call back or a decline without another Reached (issue #131)", async ({ page }) => {
    append(first, "contact_outcome", { outcome: "reached", channel: "phone" }, "seed-coordinator", "2026-11-09T08:00:00.000Z");
    append(second, "contact_outcome", { outcome: "reached", channel: "phone" }, "seed-coordinator", "2026-11-09T08:00:00.000Z");
    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Selected case" });
    await open.getByRole("button", { name: "Call back on…" }).click();
    await open.getByRole("group", { name: "Call back on" }).getByRole("button").first().click();
    await expect(open.getByRole("status")).toContainText("Logged: Call back");
    expect(ledger.get(first)?.map((e) => e.kind)).toEqual(["contact_outcome", "snooze"]);

    await page.reload();
    await page.locator("button[data-case-row]").filter({ hasText: /AHS-sr-1023/ }).click();
    await open.getByRole("button", { name: "Declined" }).click();
    await expect(open.getByTestId("removed-notice")).toContainText("Logged: Declined");
    expect(ledger.get(second)?.map((e) => e.data.outcome)).toEqual(["reached", "declined"]);
  });

  for (const [label, outcome, status, trail] of [
    ["Voicemail", "voicemail", "Left a voicemail · retry", "Left a voicemail (phone)"],
    ["Wrong number", "wrong_number", "Wrong number · check the record", "Wrong number (phone)"],
  ]) {
    test(`${label} is logged as ${outcome} on the phone and survives a reload (issue #134)`, async ({ page }) => {
      append(first, "contact_outcome", { outcome: "no_answer", channel: "phone" }, "seed-coordinator", "2026-11-08T08:00:00.000Z");
      append(first, "contact_outcome", { outcome: "no_answer", channel: "phone" }, "seed-coordinator", "2026-11-08T09:00:00.000Z");
      await page.goto("/app/today");
      const open = page.getByRole("region", { name: "Selected case" });
      const row = page.locator("button[data-case-row]").filter({ hasText: /AHS-sr-1002/ });
      await row.click();
      await open.getByRole("button", { name: "Log call · AHS-sr-1002" }).click();
      await expect(open.getByRole("group", { name: "How did it go?" }).getByRole("button")).toHaveText(["Reached", "No answer", "Voicemail", "Wrong number"]);

      await open.getByRole("button", { name: label, exact: true }).click();
      await expect(open.getByRole("status")).toContainText(`Logged: ${label}`);
      await expect(row).toContainText(status);
      await expect(page.getByTestId("today-open")).toContainText(trail);
      expect(ledger.get(first)?.at(-1)).toMatchObject({ kind: "contact_outcome", data: { outcome, channel: "phone" }, actor: "mock-coordinator" });

      await page.reload();
      await expect(row).toContainText(status);
      await row.click();
      await expect(page.getByTestId("today-open")).toContainText(trail);
      await expect(open.getByText(trail)).toHaveCount(1);
      await expect(open.getByText("No answer (phone)")).toHaveCount(2);

      await open.getByRole("button", { name: "Log call · AHS-sr-1002" }).click();
      await open.getByRole("button", { name: "No answer", exact: true }).click();
      await expect(row).toContainText("No answer ×1 · retry tomorrow");
      expect(ledger.get(first)?.map((e) => e.data.outcome)).toEqual(["no_answer", "no_answer", outcome, "no_answer"]);

      await page.reload();
      await expect(row).toContainText("No answer ×1 · retry tomorrow");
      await row.click();
      await expect(open.getByText(trail)).toHaveCount(1);
      await expect(open.getByText("No answer (phone)")).toHaveCount(3);
    });
  }

  test("Take this patient assigns the verified coordinator, not a fixed name (issue #148)", async ({ page }) => {
    await page.goto("/app/today");
    await queues(page).getByRole("link", { name: /^No owner/ }).click();
    const open = page.getByRole("region", { name: "Selected case" });
    const heading = (await open.getAttribute("data-case"))!;
    await open.getByRole("button", { name: "Take this patient" }).click();
    await expect(open.getByRole("status")).toContainText("Logged: Yours now");
    const events = [...ledger.values()].flat();
    expect(events.map((e) => [e.kind, e.data.owner, e.actor])).toEqual([["assign_owner", "mock-coordinator", "mock-coordinator"]]);
    await expect(page.getByTestId("today-open")).not.toContainText("Hessa");

    await page.reload();
    await queues(page).getByRole("link", { name: /^To call/ }).click();
    await page.locator("button[data-case-row]").filter({ hasText: new RegExp(heading) }).click();
    await expect(page.getByTestId("today-open")).not.toContainText("Hessa");
  });

  test("a non-CRC case takes an owner, a contact outcome and a snooze through the same ledger action, and a reload restores them (issue #114)", async ({ page }) => {
    const breast = "bsu-screen/ServiceRequest/BSU-sr-15";
    await page.goto("/app/today?programme=breast&provider=bsu-screen");
    await expect(page.getByText(/Breast screening · .* · run day/)).toBeVisible();
    await queues(page).getByRole("link", { name: /^No owner/ }).click();
    const open = page.getByRole("region", { name: "Selected case" });
    await expect(open).toHaveAttribute("data-case", "BSU-sr-15");
    await expect(page.getByTestId("today-open")).toContainText("Breast screening");

    await open.getByRole("button", { name: "Take this patient" }).click();
    await expect(open.getByRole("status")).toContainText("Logged: Yours now");
    await queues(page).getByRole("link", { name: /^To call/ }).click();
    await page.locator("button[data-case-row]").filter({ hasText: /BSU-sr-15/ }).click();
    await open.getByRole("button", { name: /^Log call/ }).click();
    await open.getByRole("button", { name: "Reached" }).click();
    await expect(open.getByRole("status")).toContainText("Logged: Reached");
    await open.getByRole("button", { name: "Snooze to tomorrow" }).click();
    await expect(open.getByRole("status")).toContainText("Logged: Snoozed to tomorrow");
    // The breast clock is the programme's 14-day assessment standard, not CRC's 31 days.
    await expect(page.getByTestId("today-open")).toContainText("113 days over 14");

    // One chained ledger, written by the verified coordinator; each write named the head the one before it left.
    const evs = ledger.get(breast) ?? [];
    expect(evs.map((e) => [e.kind, e.actor])).toEqual([
      ["assign_owner", "mock-coordinator"],
      ["contact_outcome", "mock-coordinator"],
      ["snooze", "mock-coordinator"],
    ]);
    expect(evs.map((e) => e.head)).toEqual([`sig-${breast}`, evs[0].hash, evs[1].hash]);
    expect(evs[1].data.outcome).toBe("reached");

    await page.reload();
    await expect(page).toHaveURL(/programme=breast&provider=bsu-screen/);
    // The persisted snooze parks the patient until tomorrow; the owner keeps them off No owner.
    await expect(queues(page).getByRole("link", { name: "No owner 0" })).toBeVisible();
    await queues(page).getByRole("link", { name: /^Waiting/ }).click();
    const row = page.locator("button[data-case-row]").filter({ hasText: /BSU-sr-15/ });
    await expect(row).toContainText("Snoozed to tomorrow");
    await row.click();
    await expect(page.getByTestId("today-open")).toContainText("Reached (phone)");
    await expect(page.getByTestId("today-open")).toContainText("mock-coordinator");
    await expect(page.getByTestId("today-open")).not.toContainText("Colorectal");
  });

  test("result notification and receipt are separate, labelled actions; Reached records neither (issue #197)", async ({ page }) => {
    const id = "ahs-mushrif/ServiceRequest/AHS-sr-1003";
    await page.goto("/app/today");
    const list = page.getByTestId("today-sheet");
    await list.getByRole("searchbox", { name: "Search" }).fill("AHS-sr-1003");
    await list.getByRole("button", { name: /AHS-sr-1003/ }).click();
    const open = page.getByRole("region", { name: "Selected case" });
    await expect(open).toHaveAttribute("data-case", "AHS-sr-1003");
    const notifyForm = open.getByRole("region", { name: "Result notification" });
    const receiptForm = open.getByRole("region", { name: "Receipt of the result" });
    const notify = notifyForm.getByRole("button", { name: "Record result notification" });
    const receipt = receiptForm.getByRole("button", { name: "Record receipt acknowledged" });
    await expect(notifyForm).toContainText("fitfu-notify-v2/en");
    await expect(receiptForm).toHaveCount(0);

    // A generic Reached is a contact outcome only: the notification is still owed.
    await open.getByRole("button", { name: /^Log call/ }).click();
    await open.getByRole("button", { name: "Reached" }).click();
    await expect(open.getByRole("status")).toContainText("Logged: Reached");
    await expect(notify).toBeVisible();
    expect(ledger.get(id)?.map((e) => e.kind)).toEqual(["contact_outcome"]);

    // Nothing is recorded until two identifiers are confirmed.
    await expect(notify).toBeDisabled();
    await notifyForm.getByLabel("Identity confirmed with").selectOption("name_and_dob");
    await notify.click();
    await expect(open.getByRole("status")).toContainText("Logged: Result notification");
    await expect(notifyForm).toHaveCount(0);
    await expect(receiptForm).toContainText("This is not a check that they understood the next step.");
    await receiptForm.getByLabel("How").selectOption("video");
    await receiptForm.getByLabel("Identity confirmed with").selectOption("name_and_mrn");
    await receipt.click();
    await expect(open.getByRole("status")).toContainText("Logged: Receipt acknowledged");
    await expect(receiptForm).toHaveCount(0);

    const evs = ledger.get(id) ?? [];
    expect(evs.map((e) => [e.kind, e.data.channel, e.data.identity_check, e.actor])).toEqual([
      ["contact_outcome", "phone", undefined, "mock-coordinator"],
      ["initial_notification", "phone", "name_and_dob", "mock-coordinator"],
      ["receipt_acknowledgement", "video", "name_and_mrn", "mock-coordinator"],
    ]);
    expect(evs[1].data.template).toBe("fitfu-notify-v2/en");

    // After a reload the board follows the API's obligation: neither step is offered again.
    await page.reload();
    await list.getByRole("searchbox", { name: "Search" }).fill("AHS-sr-1003");
    await list.getByRole("button", { name: /AHS-sr-1003/ }).click();
    await expect(open).toHaveAttribute("data-case", "AHS-sr-1003");
    await expect(notifyForm).toHaveCount(0);
    await expect(receiptForm).toHaveCount(0);
  });

  test("a non-CRC case without contact permission refuses the outcome and records nothing (issue #114)", async ({ page }) => {
    const ifhas = "ifh-screen/ServiceRequest/IFH-sr-101";
    await page.goto("/app/today?programme=ifhas&provider=ifh-screen");
    const list = page.getByTestId("today-sheet");
    await list.getByRole("searchbox", { name: "Search" }).fill("IFH-sr-101");
    const row = list.getByRole("button", { name: /IFH-sr-101/ });
    for (const tab of ["To call", "Waiting", "No owner"]) {
      await queues(page).getByRole("link", { name: new RegExp(`^${tab}`) }).click();
      if (await row.count()) break;
    }
    await row.click();
    const open = page.getByRole("region", { name: "Selected case" });
    await expect(open).toHaveAttribute("data-case", "IFH-sr-101");
    // The API refuses a contact outcome without contact permission; the board says so and records nothing.
    await open.getByRole("button", { name: "Reached" }).click();
    await expect(open.getByTestId("action-refused")).toContainText("Not saved: this patient has not given contact permission");
    expect(ledger.get(ifhas)).toBeUndefined();
  });

  test("a partial ledger replay shows only the valid prefix under a warning and disables writes", async ({ page }) => {
    append(first, "contact_outcome", { outcome: "no_answer", channel: "phone" }, "seed-coordinator", "2026-11-09T08:00:00.000Z");
    append(first, "note", { text: "Left a message with her son" }, "seed-coordinator", "2026-11-09T08:01:00.000Z");
    invalid = "line 3: head does not follow the case's previous event";

    await page.goto("/app/today");
    // Replay failure is server-wide: the board says so before any patient's detail is read.
    const board = page.getByTestId("ledger-invalid-board");
    await expect(board).toHaveAttribute("role", "alert");
    await expect(board).toContainText("this board may be out of date");
    const open = page.getByRole("region", { name: "Selected case" });
    await page.locator("button[data-case-row]").filter({ hasText: /AHS-sr-1002/ }).click();
    const warning = open.getByTestId("ledger-invalid");
    await expect(warning).toBeVisible();
    // The board banner is the one alert; the per-patient box adds detail without a second announcement.
    await expect(warning).not.toHaveAttribute("role", "alert");
    // (Next.js keeps an empty route-announcer alert on every page, so count only alerts about the ledger.)
    await expect(page.getByRole("alert").filter({ hasText: /case history/i })).toHaveCount(1);
    await expect(warning).toContainText("may be out of date");
    await expect(page.getByTestId("today-open")).toContainText("No answer (phone)");
    await expect(page.getByTestId("today-open")).toContainText("Note: “Left a message with her son”");
    await expect(open.getByRole("button", { name: "Snooze to tomorrow" })).toBeDisabled();
    await expect(open.getByRole("button", { name: "Reached" })).toBeDisabled();
    await expect(open.getByRole("button", { name: /^Hand to/ })).toBeDisabled();

    // A row the open patient's detail never touched is no more authoritative, and the board still says so.
    await page.locator("button[data-case-row]").filter({ hasText: /AHS-sr-1023/ }).click();
    await expect(board).toBeVisible();
    await expect(open.getByRole("button", { name: "Snooze to tomorrow" })).toBeDisabled();

    await page.goto("/app/patients");
    await expect(page.getByTestId("ledger-invalid-board")).toContainText("this list may be out of date");
  });

  test("a failed ledger read is not kept: Retry and reselecting the case read it again", async ({ page }) => {
    append(first, "note", { text: "Prefers a morning call" }, "seed-coordinator", "2026-11-09T08:00:00.000Z");
    unavailable.add(first);

    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Selected case" });
    const failed = open.getByTestId("ledger-read-failed");
    await expect(failed).toContainText("Could not load the contact history");
    await expect(page.getByTestId("today-open")).not.toContainText("Prefers a morning call");

    // Retry while the API is still down fails again, and says so again.
    await failed.getByRole("button", { name: "Retry" }).click();
    await expect(failed).toBeVisible();

    unavailable.delete(first);
    await failed.getByRole("button", { name: "Retry" }).click();
    await expect(page.getByTestId("today-open")).toContainText("Note: “Prefers a morning call”");
    await expect(failed).toHaveCount(0);

    // Reselection reads a failed case again, without Retry.
    unavailable.add(second);
    const list = page.getByTestId("today-sheet");
    await list.getByRole("button", { name: /AHS-sr-1023/ }).click();
    await expect(failed).toBeVisible();
    unavailable.delete(second);
    await list.getByRole("button", { name: /AHS-sr-1002/ }).click();
    await list.getByRole("button", { name: /AHS-sr-1023/ }).click();
    await expect(failed).toHaveCount(0);
    await expect(page.getByTestId("today-open")).toContainText("No contact recorded yet");
  });

  test("reselecting a failed case by search shows 'Reading…' while it is read again, not the old error", async ({ page }) => {
    append(first, "note", { text: "Prefers a morning call" }, "seed-coordinator", "2026-11-09T08:00:00.000Z");
    unavailable.add(first);

    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Selected case" });
    const failed = open.getByTestId("ledger-read-failed");
    await expect(failed).toContainText("Could not load the contact history");

    // The API is back but slow for this case. Search away from it and back: no row click, no Retry.
    unavailable.delete(first);
    hold(first);
    const search = page.getByRole("searchbox", { name: "Search" });
    await search.fill("1023");
    await expect(open).toHaveAttribute("data-case", "AHS-sr-1023");
    await search.fill("");
    await expect(open).toHaveAttribute("data-case", "AHS-sr-1002");
    await expect(page.getByTestId("today-open")).toContainText("Loading…");
    await expect(failed).toHaveCount(0);
    await expect(open.getByRole("button", { name: "Retry" })).toHaveCount(0);

    release();
    await expect(page.getByTestId("today-open")).toContainText("Note: “Prefers a morning call”");
    await expect(failed).toHaveCount(0);
  });

  test("the board warning follows the latest read: a restored ledger clears it, a failed one sets it again", async ({ page }) => {
    invalid = "append failed and the ledger file state is unknown: disk full";
    await page.goto("/app/today");
    const board = page.getByTestId("ledger-invalid-board");
    const open = page.getByRole("region", { name: "Selected case" });
    await expect(board).toBeVisible();
    await expect(open.getByTestId("ledger-invalid")).toContainText("Case history needs checking");
    await expect(open.getByRole("button", { name: "Snooze to tomorrow" })).toBeDisabled();

    // An operator restores the ledger. The next read (selecting another case) clears the warning without a reload.
    invalid = null;
    const list = page.getByTestId("today-sheet");
    await list.getByRole("button", { name: /AHS-sr-1023/ }).click();
    await expect(open).toHaveAttribute("data-case", "AHS-sr-1023");
    await expect(board).toHaveCount(0);
    await expect(open.getByTestId("ledger-invalid")).toHaveCount(0);
    await expect(open.getByRole("button", { name: "Snooze to tomorrow" })).toBeEnabled();

    // Reselecting a case reads it again: a ledger that fails again brings the warning back.
    invalid = "ledger changed under another writer: hash mismatch";
    await list.getByRole("button", { name: /AHS-sr-1002/ }).click();
    await expect(board).toBeVisible();
    await expect(open.getByTestId("ledger-invalid")).toContainText("Case history needs checking");
  });

  test("a 401 on a write is a refusal that names the credential, not 'the API did not answer'", async ({ page }) => {
    refuseWrites = true;
    await page.goto("/app/today");
    const open = page.getByRole("region", { name: "Selected case" });
    await expect(page.getByTestId("today-open")).toContainText("No contact recorded yet");
    await open.getByRole("button", { name: "Snooze to tomorrow" }).click();
    const refused = open.getByTestId("action-refused");
    await expect(refused).toContainText("Not saved: the API did not accept this server's coordinator credential");
    await expect(refused).not.toContainText("did not answer");
    expect(ledger.get(first)).toBeUndefined();
  });

  test("a valid ledger shows no board-level warning", async ({ page }) => {
    await page.goto("/app/today");
    await expect(page.getByRole("region", { name: "Selected case" }).getByRole("heading", { level: 2 })).toBeVisible();
    await expect(page.getByTestId("ledger-invalid-board")).toHaveCount(0);
  });
});
