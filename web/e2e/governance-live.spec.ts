import { readFileSync } from "node:fs";
import { createServer, type Server } from "node:http";
import path from "node:path";
import { expect, test } from "@playwright/test";
import fixtures from "../data/api/fixtures.json";

/*
 * Browser -> server action -> authenticated API consent contest/resolution. Start next start with WASL_API_URL
 * pointing at 127.0.0.1:3295 and the credentials below, then run with GOVERNANCE_LIVE_MOCK=1 and PLAYWRIGHT_BASE_URL.
 * The mock mirrors the Go API's role binding and independent-actor checks; the real handler is covered in pipeline.
 */
const LIVE = !!process.env.GOVERNANCE_LIVE_MOCK;
const caseId = fixtures.fixtures.review_hold.id;
const casePath = `/api/cases/${caseId}`;
const auditPath = `/app/patients/${encodeURIComponent(caseId)}/audit`;
const originalHash = "a".repeat(64);
const contestHash = "b".repeat(64);
const resolutionHash = "c".repeat(64);
const dir = path.join(process.cwd(), "data", "api");
const read = (file: string) => JSON.parse(readFileSync(path.join(dir, file), "utf8"));
const detail = read("case_review_hold.json");
const audit = read("case_review_hold_audit.json");
const meta = read("fixtures.json").meta;

type Event = {
  hash: string;
  action: string;
  actor: string;
  role: string;
  kind?: string;
  target?: string;
  target_actor?: string;
  resolution?: string;
  [key: string]: unknown;
};

const credentialActors: Record<string, [string, string]> = {
  WASL_API_REVIEWER_TOKEN: ["reviewer", "reviewer-one"],
  WASL_API_AUDITOR_TOKEN: ["auditor", "auditor-one"],
  WASL_API_COORDINATOR_TOKEN: ["coordinator", "coordinator-one"],
  WASL_API_CLINICIAN_TOKEN: ["clinician", "clinician-one"],
  WASL_API_CLINICIAN_2_TOKEN: ["clinician", "clinician-two"],
  WASL_API_CLINICIAN_3_TOKEN: ["clinician", "clinician-three"],
};

function seedConsent(): Event {
  return {
    case: caseId,
    item: "",
    decision: "consent_granted",
    note: "Original consent",
    role: "coordinator",
    actor: "coordinator-one",
    action: "record_consent",
    at: "2026-11-10T09:00:00Z",
    head_signed: detail.current.sig,
    mode: "demo",
    prev: "genesis",
    hash: originalHash,
  };
}

function statuses(events: Event[]) {
  const result = new Map(events.map((event) => [event.hash, event.kind === "contest" ? "open" : event.kind ? "applied" : "active"]));
  for (const event of events) {
    if (event.kind === "contest") result.set(event.target!, "contested");
    if (event.kind === "contest_resolution") {
      result.set(event.hash, event.resolution!);
      result.set(event.target!, event.resolution!);
      const contest = events.find((candidate) => candidate.hash === event.target)!;
      result.set(contest.target!, event.resolution === "upheld" ? "compensated" : "active");
    }
  }
  return result;
}

function projection(events: Event[]) {
  const status = statuses(events);
  return {
    case: caseId,
    case_status: events.some((event) => event.kind === "contest" && status.get(event.hash) === "open")
      ? "contested"
      : events.some((event) => event.kind === "contest_resolution" && event.resolution === "upheld")
        ? "compensated"
        : "active",
    canonical_state: detail.case.state,
    note: "review-ledger projection only",
    events: events.map((event) => ({
      hash: event.hash,
      kind: event.kind ?? "review",
      actor: event.actor,
      action: event.action,
      target: event.target ?? "",
      status: status.get(event.hash),
      at: event.at,
    })),
  };
}

test.describe("consent governance through the UI", () => {
  test.skip(!LIVE, "set GOVERNANCE_LIVE_MOCK=1 with the server pointed at the mock");
  test.describe.configure({ mode: "serial" });

  let server: Server;
  let events: Event[];
  let posted: { credential: string; body: Record<string, unknown> }[];

  test.beforeAll(async () => {
    const principals = new Map<string, { credential: string; role: string; actor: string }>();
    for (const [name, [role, actor]] of Object.entries(credentialActors)) {
      const token = process.env[name]?.trim();
      if (!token) throw new Error(`${name} must be set for the governance live mock`);
      principals.set(token, { credential: name, role, actor });
    }
    server = createServer((req, res) => {
      const send = (status: number, body: unknown) => {
        res.writeHead(status, { "content-type": "application/json" });
        res.end(JSON.stringify(body));
      };
      const p = decodeURIComponent(new URL(req.url ?? "/", "http://mock").pathname);
      const token = (req.headers.authorization ?? "").split(" ")[1];
      const principal = token ? principals.get(token) : undefined;
      if (!principal) return send(401, { meta, error: "authentication required" });
      if (req.method === "GET" && p === "/api/fixtures") return send(200, read("fixtures.json"));
      if (req.method === "GET" && p === "/api/cases") return send(200, { meta, cases: [], total: 0, limit: 50, offset: 0 });
      if (req.method === "GET" && p === `${casePath}/audit`) {
        const history = events.map((event) => Object.fromEntries(Object.entries(event).filter(([key]) => !["actor", "target_actor", "note"].includes(key))));
        return send(200, { ...audit, history });
      }
      if (req.method === "GET" && p === `${casePath}/undo`) return send(200, { meta, ...projection(events) });
      if (req.method === "GET" && p === casePath) return send(200, detail);
      if (req.method === "POST" && p === `${casePath}/undo`) {
        let raw = "";
        req.on("data", (chunk) => (raw += chunk));
        req.on("end", () => {
          const body = JSON.parse(raw);
          posted.push({ credential: principal.credential, body });
          const target = events.find((event) => event.hash === body.target);
          if (!target) return send(404, { meta, error: "target is not a review event of this case" });
          const authorised = target.action === "record_consent"
            ? principal.role === "coordinator" || principal.role === "clinician"
            : target.action === "signoff_evidence" && principal.role === "clinician";
          if (!authorised) return send(403, { meta, error: `rbac: role "${principal.role}" may not ${target.action}` });
          const resolving = typeof body.resolution === "string";
          const originalActor = resolving ? target.target_actor : target.actor;
          if (principal.actor === originalActor || (resolving && principal.actor === target.actor)) {
            return send(403, { meta, error: "actor must differ from the actor(s) of the event being undone, contested or resolved" });
          }
          const event: Event = {
            kind: resolving ? "contest_resolution" : "contest",
            case: caseId,
            target: target.hash,
            target_actor: originalActor,
            target_action: target.action,
            action: target.action,
            role: principal.role,
            actor: principal.actor,
            note: body.note,
            head_signed: detail.current.sig,
            action_head: events.at(-1)!.hash,
            at: `2026-11-10T09:0${events.length}:00Z`,
            mode: "demo",
            prev: events.at(-1)!.hash,
            hash: resolving ? resolutionHash : contestHash,
            ...(resolving ? { resolution: body.resolution } : {}),
          };
          events.push(event);
          return send(201, {
            meta,
            event,
            canonical_reopened: false,
            auto_closure: false,
            canonical_state: detail.case.state,
            ...(resolving ? {} : { requires_resolution: true }),
            projection: projection(events),
            note: resolving ? "contest resolved in the review ledger only" : "contest recorded; independent resolution required",
          });
        });
        return;
      }
      return send(404, { meta, error: "not found" });
    });
    await new Promise<void>((resolve) => server.listen(3295, "127.0.0.1", resolve));
  });

  test.beforeEach(() => {
    events = [seedConsent()];
    posted = [];
  });

  test.afterAll(async () => {
    await new Promise((resolve) => server.close(resolve));
  });

  test("a consent contest can be resolved by a third independent authorised credential", async ({ page }) => {
    await page.goto(auditPath);
    const original = page.locator(`#review-event-${originalHash}`);
    const contestForm = original.locator("details").filter({ hasText: "Contest review" });
    await contestForm.locator("summary").click();
    await contestForm.getByLabel("Authorised credential").selectOption("clinician");
    await contestForm.getByLabel("Reason").fill("Consent needs an independent check");
    await contestForm.getByRole("button", { name: "Contest review" }).click();
    await expect(page.locator(`#review-event-${contestHash}`)).toContainText("Contest status: open");

    await page.reload();
    const contest = page.locator(`#review-event-${contestHash}`);
    const resolutionForm = contest.locator("details").filter({ hasText: "Resolve contest" });
    await expect(contest).toContainText("Contest status: open");
    await resolutionForm.locator("summary").click();
    await resolutionForm.getByLabel("Authorised credential").selectOption("clinician_2");
    await resolutionForm.getByLabel("Resolution").selectOption("upheld");
    await resolutionForm.getByLabel("Reason").fill("Independent clinician upholds the contest");
    await resolutionForm.getByRole("button", { name: "Resolve contest" }).click();
    await expect(page.locator(`#review-event-${resolutionHash}`)).toContainText("Contest resolved: upheld");

    expect(posted).toEqual([
      {
        credential: "WASL_API_CLINICIAN_TOKEN",
        body: { target: originalHash, contest: true, note: "Consent needs an independent check" },
      },
      {
        credential: "WASL_API_CLINICIAN_2_TOKEN",
        body: { target: contestHash, resolution: "upheld", note: "Independent clinician upholds the contest" },
      },
    ]);
    await page.reload();
    await expect(page.locator(`#review-event-${contestHash}`)).toContainText("Contest status: upheld");
    await expect(page.locator(`#review-event-${originalHash}`)).toContainText("Review status: compensated");
  });
});
