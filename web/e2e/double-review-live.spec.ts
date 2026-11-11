import { readFileSync } from "node:fs";
import { createServer, type Server } from "node:http";
import path from "node:path";
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

/*
 * Independent double review (issue #41) against a mock of the Go API.
 * Start `next start` yourself with WASL_API_URL=http://127.0.0.1:3297 and these tokens: WASL_API_REVIEWER_TOKEN,
 * WASL_API_CLINICIAN_TOKEN, WASL_API_AUDITOR_TOKEN and WASL_API_COORDINATOR_TOKEN. Also set WASL_API_REVIEWER_2_TOKEN
 * and WASL_API_REVIEWER_3_TOKEN (other features use them): this spec proves double review never sends them. Then run it
 * with DOUBLE_REVIEW_LIVE_MOCK=1, PLAYWRIGHT_BASE_URL pointing at that server and the same tokens.
 *
 * The web has no per-person sign-in, so the deployment under test is ONE reader (mock-reviewer-1 for identity
 * questions). The other readers are simulated by POSTing straight to the mock API with their own tokens, as their own
 * deployments or clients would; the page never holds their credentials.
 *
 * The mock follows docs/API-DOUBLE-REVIEW.md (pipeline/api_double_review.go): identity only from the bearer token, the
 * question's role, per-round expected_head, one person per initial read, r3 only after a disagreement by a third
 * person, and blinding of an initial answer from everyone but its author until both are in. It is a mock: the real
 * handler's output is pinned separately by pipeline/api_double_review_test.go and web/tests/double-review.test.ts.
 */
const LIVE = !!process.env.DOUBLE_REVIEW_LIVE_MOCK;
const MOCK_PORT = 3297;
const MOCK = `http://127.0.0.1:${MOCK_PORT}`;
/** Credentials of other people. Only this spec and the mock know them; the web server never does. */
const OTHER_2 = "mock-other-reviewer-2-token";
const OTHER_3 = "mock-other-reviewer-3-token";
/** A note no page may ever show while its read is blinded. */
const CANARY = "CANARY-blinded-note-of-another-reader";

type Read = { read: string; decision: string; note: string; actor: string; role: string; at: string; hash: string; item: string; question: string; phase_after: string; final?: string; prev: string };

test.describe("independent double review, live mode against a mock API", () => {
  test.skip(!LIVE, "set DOUBLE_REVIEW_LIVE_MOCK=1 with the server pointed at the mock");
  test.describe.configure({ mode: "serial" });

  const dir = path.join(process.cwd(), "data", "api");
  const read = (f: string) => JSON.parse(readFileSync(path.join(dir, f), "utf8"));
  const fx = read("fixtures.json");
  const meta = fx.meta;
  const hold = read("case_review_hold.json");
  const holdAudit = read("case_review_hold_audit.json");
  const id: string = hold.case.id;
  const summary = { ...hold.case, review: "open", closed: false, state: "open" };
  const LINK = "rv-636161dc42837e08";
  const EVIDENCE = "rv-d55f7ccf9f12c409";
  const questionOf: Record<string, string> = { [LINK]: "link", [EVIDENCE]: "evidence" };
  const roleFor = (q: string) => (q === "link" ? "reviewer" : "clinician");

  let ledger: Read[] = [];
  let n = 0;
  /** Make GET wrongly offer r2 to the author of r1, so the POST refusal itself is exercised. */
  let claimMay = false;
  /** Make GET and case detail wrongly include every blinded answer, as a broken API would. */
  let leak = false;
  /** Every request the mock received: which token, method and path. */
  const seen: { token: string; method: string; path: string; body?: Record<string, string> }[] = [];
  const tokens = new Map<string, { role: string; actor: string }>([
    [OTHER_2, { role: "reviewer", actor: "mock-other-2" }],
    [OTHER_3, { role: "reviewer", actor: "mock-other-3" }],
  ]);
  /** Tokens the web server holds but must never send for double review. */
  const serverExtras = new Set<string>();
  let deskToken = "";
  let server: Server;

  const norm = (s: string) => s.trim().toLowerCase();
  const reads = (item: string) => ledger.filter((r) => r.item === item);
  const head = (item: string) => reads(item).at(-1)?.hash ?? "genesis";
  function phase(item: string) {
    const by = Object.fromEntries(reads(item).map((r) => [r.read, r]));
    if (!by.r1 || !by.r2) return "awaiting_reads";
    if (by.r3) return "resolved";
    return by.r1.decision === by.r2.decision ? "agreed" : "disagreement";
  }
  function refusal(item: string, rd: string, p: { role: string; actor: string }): [number, Record<string, string>] | null {
    if (roleFor(questionOf[item]) !== p.role) return [403, { error: "rbac: role may not sign this question", reason: "rbac" }];
    const by = Object.fromEntries(reads(item).map((r) => [r.read, r]));
    const ph = phase(item);
    if (rd === "r3") {
      if (ph !== "disagreement") return [409, { error: "r3 opens only after a disagreement", conflict: "phase", phase: ph }];
      if ([by.r1, by.r2].some((r) => r && norm(r.actor) === norm(p.actor))) return [403, { error: "the senior resolver must be a third actor", reason: "same_actor" }];
      return null;
    }
    if (by[rd]) return [409, { error: `${rd} has already been submitted`, conflict: "read", phase: ph }];
    const other = by[rd === "r1" ? "r2" : "r1"];
    if (other && norm(other.actor) === norm(p.actor)) {
      return [403, { error: `independent double review needs two different actors: this credential's actor already gave ${other.read}`, reason: "same_actor" }];
    }
    return null;
  }
  function round(item: string, p: { role: string; actor: string }) {
    const ph = phase(item);
    const view = (r: Read) => {
      const visible = leak || ph !== "awaiting_reads" || norm(r.actor) === norm(p.actor);
      return { read: r.read, actor: r.actor, role: r.role, at: r.at, hash: r.hash, blinded: !visible, ...(visible ? { decision: r.decision, note: r.note } : {}) };
    };
    const by = Object.fromEntries(reads(item).map((r) => [r.read, view(r)]));
    const may = ["r1", "r2", "r3"].filter((rd) => !refusal(item, rd, p) || (claimMay && rd === "r2" && phase(item) === "awaiting_reads" && !by.r2));
    const final = ph === "agreed" ? reads(item).find((r) => r.read === "r1")!.decision : ph === "resolved" ? reads(item).find((r) => r.read === "r3")!.decision : undefined;
    return {
      item, question: questionOf[item], action: questionOf[item] === "link" ? "signoff_link" : "signoff_evidence", phase: ph, head: head(item),
      seq: reads(item).length, r3_open: ph === "disagreement", viewer_may: may, reads: { r1: by.r1 ?? null, r2: by.r2 ?? null, r3: by.r3 ?? null },
      ...(final ? { final } : {}),
    };
  }
  /** Case detail `api_reviews`, blinded for everyone (the author included) while a round awaits its second initial read. */
  const apiReviews = () =>
    ledger.map((r) => {
      const blind = !leak && phase(r.item) === "awaiting_reads";
      const ev = { type: "double_review", kind: "read", case: id, item: r.item, question: r.question, read: r.read, role: r.role, actor: r.actor,
        action: r.question === "link" ? "signoff_link" : "signoff_evidence", at: r.at, mode: "demo", prev: r.prev, hash: r.hash, round_head: "", round_seq: 1,
        phase_after: r.phase_after, ...(r.final ? { final: r.final } : {}) };
      return blind ? { ...ev, blinded: true } : { ...ev, decision: r.decision, note: r.note, nonce: "0".repeat(32) };
    });

  test.beforeAll(async () => {
    const env: [string, string, string][] = [
      ["WASL_API_REVIEWER_TOKEN", "reviewer", "mock-reviewer-1"],
      ["WASL_API_CLINICIAN_TOKEN", "clinician", "mock-clinician-1"],
      ["WASL_API_AUDITOR_TOKEN", "auditor", "mock-auditor"],
      ["WASL_API_COORDINATOR_TOKEN", "coordinator", "mock-coordinator"],
    ];
    for (const [name, role, actor] of env) {
      const token = process.env[name]?.trim();
      if (!token) throw new Error(`${name} must be set for the live mock, with the same value the server sends`);
      tokens.set(token, { role, actor });
    }
    deskToken = process.env.WASL_API_REVIEWER_TOKEN!.trim();
    // Bound to other people. If double review ever sent one, the mock would record a second person through this page.
    for (const [name, actor] of [["WASL_API_REVIEWER_2_TOKEN", "mock-server-reviewer-2"], ["WASL_API_REVIEWER_3_TOKEN", "mock-server-reviewer-3"]]) {
      const token = process.env[name]?.trim();
      if (!token) throw new Error(`${name} must be set for the live mock, so the spec can prove double review never sends it`);
      tokens.set(token, { role: "reviewer", actor });
      serverExtras.add(token);
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
      const who = scheme === "Bearer" && token ? tokens.get(token) : undefined;
      if (!who) return send(401, { error: "authentication required" });
      const entry: (typeof seen)[number] = { token, method: req.method ?? "GET", path: p };
      seen.push(entry);
      if (p === "/api/dashboard") return send(200, read("dashboard.json"));
      if (p === "/api/fixtures") return send(200, fx);
      if (p === "/api/cases") return send(200, { meta, cases: [summary], total: 1, limit: 50, offset: 0 });
      if (p === `/api/cases/${id}/audit`) return send(200, { ...holdAudit, history: [] });
      if (p === `/api/cases/${id}/double-review`) {
        if (who.role === "auditor") return send(403, { meta, error: "read permission denied" });
        if (req.method === "GET") {
          return send(200, { meta, protocol: "wasl-double-review/1", case: id, viewer: who, rounds: [LINK, EVIDENCE].map((i) => round(i, who)), canonical_state: "open", canonical_review: "open" });
        }
        let raw = "";
        req.on("data", (c) => (raw += c));
        req.on("end", () => {
          const body = JSON.parse(raw);
          entry.body = body;
          if (!questionOf[body.item]) return send(404, { meta, error: "review item not raised for this case" });
          if (body.expected_head !== head(body.item)) return send(409, { meta, error: "stale expected_head", conflict: "head", current_head: head(body.item) });
          const no = refusal(body.item, body.read, who);
          if (no) return send(no[0], { meta, ...no[1] });
          const r: Read = { read: body.read, decision: body.decision, note: body.note ?? "", actor: who.actor, role: who.role, at: new Date().toISOString(), item: body.item,
            question: questionOf[body.item], hash: `${(++n).toString(16).padStart(8, "0")}${"cd".repeat(28)}`, prev: ledger.at(-1)?.hash ?? "genesis", phase_after: "" };
          ledger.push(r);
          r.phase_after = phase(body.item);
          const rd = round(body.item, who);
          if (rd.final) r.final = rd.final;
          send(201, { meta, event: { ...apiReviews().at(-1), decision: r.decision, note: r.note }, round: rd, canonical_unchanged: true });
        });
        return;
      }
      if (p === `/api/cases/${id}`) return send(200, { ...hold, case: summary, api_reviews: apiReviews(), review_ledger: { status: "ok", events_applied: ledger.length } });
      return send(404, { meta, error: "unknown case" });
    });
    await new Promise<void>((resolve) => server.listen(MOCK_PORT, "127.0.0.1", resolve));
  });

  test.afterAll(async () => {
    await new Promise((resolve) => server.close(resolve));
  });

  test.beforeEach(() => {
    ledger = [];
    seen.length = 0;
    claimMay = false;
    leak = false;
  });

  test.afterEach(() => {
    // Whatever happened, the web server never acted for double review through any credential but the deployment's own.
    const web = seen.filter((s) => s.path.endsWith("/double-review") && s.token !== OTHER_2 && s.token !== OTHER_3);
    expect(web.length).toBeGreaterThan(0);
    expect(web.filter((s) => s.token !== deskToken && tokens.get(s.token)?.role !== "clinician")).toEqual([]);
    expect(seen.filter((s) => serverExtras.has(s.token) && s.path.endsWith("/double-review"))).toEqual([]);
  });

  const desk = `/app/review/${encodeURIComponent(id)}`;
  const panel = (page: Page) => page.getByRole("region", { name: "Independent double review" });
  const linkRound = (page: Page) => panel(page).getByTestId(`double-round-${LINK}`);

  /** Another person's read, through their own credential, straight to the API: never through this page. */
  async function otherReads(request: APIRequestContext, token: string, rd: string, decision: string, note = "") {
    const res = await request.post(`${MOCK}/api/cases/${encodeURIComponent(id)}/double-review`, {
      headers: { authorization: `Bearer ${token}`, "content-type": "application/json" },
      data: { item: LINK, read: rd, decision, note, expected_head: head(LINK) },
    });
    expect(res.status()).toBe(201);
  }

  /** The review page's HTML and its RSC payload, fetched as a visitor would. */
  async function payloads(request: APIRequestContext, url: string) {
    const html = await (await request.get(url)).text();
    const rsc = await request.get(url, { headers: { RSC: "1" } });
    expect(rsc.headers()["content-type"]).toContain("text/x-component");
    return { html, rsc: await rsc.text() };
  }

  test("one deployment gives one read; the other people read through their own credentials; every read is kept", async ({ page, request }) => {
    // Another person gives r1 first, through their own credential, with a note the page must never show while blinded.
    await otherReads(request, OTHER_2, "r1", "different", CANARY);

    await page.goto(`${desk}#double-review`);
    await expect(panel(page)).toContainText("Identity questions: this deployment is recorded by the API as mock-reviewer-1");
    await expect(panel(page)).toContainText("This deployment is one reader");
    await expect(panel(page)).toContainText("designed, not built");
    // No seat switcher: nothing on the page offers another credential.
    await expect(panel(page).getByRole("navigation")).toHaveCount(0);
    await expect(page.locator('a[href*="seat="]')).toHaveCount(0);

    const hidden = linkRound(page).getByTestId("read-r1");
    await expect(hidden).toContainText("given by mock-other-2");
    await expect(hidden).toContainText("Answer hidden until both initial reads are in.");
    await expect(hidden).not.toContainText("Not the same patient");
    for (const body of Object.values(await payloads(request, desk))) {
      expect(body).toContain("mock-other-2");
      expect(body).not.toContain(CANARY);
    }
    await expect(page.getByRole("button", { name: "Record the senior read" })).toHaveCount(0);
    await new AxeBuilder({ page }).include("#double-review").analyze().then((a) => {
      expect(a.violations.filter((v) => v.impact === "serious" || v.impact === "critical").map((v) => v.id)).toEqual([]);
    });

    // This deployment answers; the API gives it the open slot (r2).
    await expect(linkRound(page).getByRole("group", { name: "Your read: Is it the same patient?" })).toBeVisible();
    await linkRound(page).getByRole("radio", { name: "Same patient", exact: true }).check();
    await linkRound(page).getByRole("button", { name: "Record your read" }).click();
    await expect(linkRound(page)).toContainText("Initial reads disagree: senior read open");
    await expect(linkRound(page).getByTestId("read-r1")).toContainText("Not the same patient · mock-other-2");
    await expect(linkRound(page).getByTestId("read-r2")).toContainText("Same patient · your read, as mock-reviewer-1");
    // It gave an initial read, so it cannot resolve: the senior read must come from a third person.
    await expect(linkRound(page).getByTestId("why")).toHaveText("This deployment gave r2 as mock-reviewer-1. The senior read must come from a third person.");
    await expect(page.getByRole("button", { name: "Record the senior read" })).toHaveCount(0);

    // A third person resolves through their own credential.
    await otherReads(request, OTHER_3, "r3", "same");
    await page.reload();
    await expect(linkRound(page)).toContainText("Resolved by a senior read");
    await expect(linkRound(page).getByTestId("settled")).toHaveText("Settled answer: Same patient");
    await expect(linkRound(page).getByTestId("read-r3")).toContainText("Same patient · mock-other-3");

    expect(ledger.map((r) => [r.read, r.decision, r.actor])).toEqual([
      ["r1", "different", "mock-other-2"],
      ["r2", "same", "mock-reviewer-1"],
      ["r3", "same", "mock-other-3"],
    ]);
    // Exactly one of the three reads came through this page, with the deployment's one credential and no identity header.
    const webPosts = seen.filter((s) => s.method === "POST" && s.token !== OTHER_2 && s.token !== OTHER_3);
    expect(webPosts.map((s) => [s.token === deskToken, s.body?.read])).toEqual([[true, "r2"]]);

    // The case page lists each read on its own line, by the person the API recorded.
    await page.goto(`/app/patients/${encodeURIComponent(id)}`);
    const card = page.getByRole("region", { name: "Double-review reads" });
    await expect(card.getByRole("listitem")).toHaveCount(3);
    await expect(card).toContainText("First read (r1): Not the same patient · mock-other-2");
    await expect(card).toContainText("Second read (r2): Same patient · mock-reviewer-1");
    await expect(card).toContainText("Senior read (r3): Same patient · mock-other-3");
  });

  test("one deployment cannot fill two seats", async ({ page }) => {
    await page.goto(`${desk}#double-review`);
    await linkRound(page).getByRole("radio", { name: "Same patient", exact: true }).check();
    await linkRound(page).getByRole("button", { name: "Record your read" }).click();
    await expect(linkRound(page).getByTestId("read-r1")).toContainText("Same patient · your read, as mock-reviewer-1");
    await expect(linkRound(page).getByTestId("why")).toHaveText("You gave r1 as mock-reviewer-1. The other initial read must come from a different person.");
    await expect(linkRound(page).getByRole("button", { name: "Record your read" })).toHaveCount(0);

    // A leftover ?seat= link changes nothing: there is no seat to switch to.
    await page.goto(`${desk}?seat=r2#double-review`);
    await expect(panel(page)).toContainText("recorded by the API as mock-reviewer-1");
    await expect(linkRound(page).getByRole("button", { name: "Record your read" })).toHaveCount(0);

    // Even if a response offered the second seat, the API's refusal is what the page shows, and nothing is appended.
    claimMay = true;
    await page.reload();
    await linkRound(page).getByRole("radio", { name: "Not the same patient" }).check();
    await linkRound(page).getByRole("button", { name: "Record your read" }).click();
    await expect(linkRound(page).getByRole("alert")).toContainText("Not saved: the API refused because this deployment's credential belongs to someone who already read this item.");
    expect(ledger.map((r) => [r.read, r.actor])).toEqual([["r1", "mock-reviewer-1"]]);
    const posts = seen.filter((s) => s.method === "POST");
    expect(posts.map((s) => [s.token === deskToken, s.body?.read])).toEqual([
      [true, "r1"],
      [true, "r2"],
    ]);
  });

  test("a blinded answer that is not the deployment's own never reaches the HTML or RSC payload, even if the API leaks it", async ({ page, request }) => {
    await otherReads(request, OTHER_2, "r1", "different", CANARY);
    leak = true; // the mock now serves r1's answer and note to everyone, as a broken API might
    const leaked = await (await request.get(`${MOCK}/api/cases/${encodeURIComponent(id)}/double-review`, { headers: { authorization: `Bearer ${deskToken}` } })).text();
    expect(leaked).toContain(CANARY);

    for (const url of [desk, `/app/patients/${encodeURIComponent(id)}`]) {
      for (const body of Object.values(await payloads(request, url))) {
        expect(body).toContain("mock-other-2");
        expect(body).not.toContain(CANARY);
      }
    }
    await page.goto(`${desk}#double-review`);
    await expect(linkRound(page).getByTestId("read-r1")).toContainText("Answer hidden until both initial reads are in.");
    await expect(linkRound(page).getByTestId("read-r1")).not.toContainText("Not the same patient");
    await page.goto(`/app/patients/${encodeURIComponent(id)}`);
    await expect(page.getByRole("region", { name: "Double-review reads" })).toContainText("answer hidden until both initial reads are in");
  });
});
