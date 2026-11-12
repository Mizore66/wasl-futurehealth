import { readFileSync } from "node:fs";
import { createServer, type Server } from "node:http";
import path from "node:path";
import { expect, test, type Page } from "@playwright/test";
import fixtures from "../data/api/fixtures.json";

/*
 * Browser -> sign-in -> server action -> authenticated API consent contest/resolution, acting only as the signed-in
 * token. Start `next start` with WASL_API_URL=http://127.0.0.1:3295 and the server-held shared read credentials
 * (WASL_API_WEB_REVIEWER_TOKEN and WASL_API_AUDITOR_TOKEN, at least 16 characters each), then run this spec with the
 * same two variables, GOVERNANCE_LIVE_MOCK=1 and PLAYWRIGHT_BASE_URL. The clinicians' own tokens below are typed into
 * the sign-in form, as an operator would issue them to two people; the Next.js server never holds them.
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

/** Server-held shared read credentials: [role, actor]. Neither may append governance events. */
const sharedActors: Record<string, [string, string]> = {
  WASL_API_WEB_REVIEWER_TOKEN: ["reviewer", "web-reviewer"],
  WASL_API_AUDITOR_TOKEN: ["auditor", "auditor-one"],
};
/** Each person's own assigned token, issued by the operator and typed into the sign-in form. */
const people = {
  clinicianOne: { token: "governance-clinician-one-0001", role: "clinician", actor: "clinician-one" },
  clinicianTwo: { token: "governance-clinician-two-0002", role: "clinician", actor: "clinician-two" },
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
  let posted: { actor: string; identityHeaders: boolean; body: Record<string, unknown> }[];

  test.beforeAll(async () => {
    const principals = new Map<string, { role: string; actor: string }>();
    for (const [name, [role, actor]] of Object.entries(sharedActors)) {
      const token = process.env[name]?.trim();
      if (!token) throw new Error(`${name} must be set for the governance live mock`);
      principals.set(token, { role, actor });
    }
    for (const person of Object.values(people)) principals.set(person.token, { role: person.role, actor: person.actor });
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
          posted.push({ actor: principal.actor, identityHeaders: "x-wasl-role" in req.headers || "x-wasl-actor" in req.headers, body });
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

  async function signIn(page: Page, token: string) {
    const prompt = page.getByTestId("governance-signin");
    await expect(prompt).toBeVisible();
    await prompt.getByLabel("Your token").fill(token);
    await prompt.getByRole("button", { name: "Sign in" }).click();
    await expect(page.getByTestId("governance-identity")).toBeVisible();
  }

  test("signed out, the page offers a sign-in prompt and no governance control", async ({ page }) => {
    await page.goto(auditPath);
    await expect(page.getByTestId("governance-signin")).toContainText("signs in separately");
    await expect(page.getByText("Contest review")).toHaveCount(0);
    await expect(page.getByText("Append compensation")).toHaveCount(0);
    await expect(page.locator('select[name="credential"]')).toHaveCount(0);
    await page.getByTestId("governance-signin").getByLabel("Your token").fill("not-an-issued-token-0000");
    await page.getByTestId("governance-signin").getByRole("button", { name: "Sign in" }).click();
    await expect(page.getByTestId("governance-signin").getByRole("alert")).toContainText("Sign-in failed");
    expect(posted).toEqual([]);
  });

  test("a consent contest is resolved only by a separately signed-in, different actor", async ({ page }) => {
    await page.goto(auditPath);
    await signIn(page, people.clinicianOne.token);
    await expect(page.locator('select[name="credential"]')).toHaveCount(0);
    const original = page.locator(`#review-event-${originalHash}`);
    const contestForm = original.locator("details").filter({ hasText: "Contest review" });
    await contestForm.locator("summary").click();
    await contestForm.getByLabel("Reason").fill("Consent needs an independent check");
    await contestForm.getByRole("button", { name: "Contest review" }).click();
    await expect(page.locator(`#review-event-${contestHash}`)).toContainText("Contest status: open");

    /* The same sign-in cannot also resolve: no default resolution, and the API refuses the contester as resolver. */
    await page.reload();
    const contest = page.locator(`#review-event-${contestHash}`);
    let resolutionForm = contest.locator("details").filter({ hasText: "Resolve contest" });
    await expect(contest).toContainText("Contest status: open");
    await resolutionForm.locator("summary").click();
    await expect(resolutionForm.getByLabel("Resolution")).toHaveValue("");
    await resolutionForm.getByLabel("Reason").fill("Resolving my own contest");
    await resolutionForm.getByRole("button", { name: "Resolve contest" }).click();
    expect(await resolutionForm.getByLabel("Resolution").evaluate((el: HTMLSelectElement) => el.validity.valueMissing)).toBe(true);
    await resolutionForm.getByLabel("Resolution").selectOption("upheld");
    await resolutionForm.getByRole("button", { name: "Resolve contest" }).click();
    await expect(resolutionForm.getByRole("alert")).toContainText("actor must differ");
    await expect(page.locator(`#review-event-${resolutionHash}`)).toHaveCount(0);

    /* A different person signs in with their own token, in a fresh session on this browser. */
    await page.getByTestId("governance-identity").getByRole("button", { name: "Sign out" }).click();
    await signIn(page, people.clinicianTwo.token);
    resolutionForm = page.locator(`#review-event-${contestHash}`).locator("details").filter({ hasText: "Resolve contest" });
    await resolutionForm.locator("summary").click();
    await expect(resolutionForm.getByLabel("Resolution")).toHaveValue("");
    await resolutionForm.getByLabel("Resolution").selectOption("upheld");
    await resolutionForm.getByLabel("Reason").fill("Independent clinician upholds the contest");
    await resolutionForm.getByRole("button", { name: "Resolve contest" }).click();
    await expect(page.locator(`#review-event-${resolutionHash}`)).toContainText("Contest resolved: upheld");

    expect(posted).toEqual([
      {
        actor: "clinician-one",
        identityHeaders: false,
        body: { target: originalHash, contest: true, note: "Consent needs an independent check" },
      },
      {
        actor: "clinician-one",
        identityHeaders: false,
        body: { target: contestHash, resolution: "upheld", note: "Resolving my own contest" },
      },
      {
        actor: "clinician-two",
        identityHeaders: false,
        body: { target: contestHash, resolution: "upheld", note: "Independent clinician upholds the contest" },
      },
    ]);
    await page.reload();
    await expect(page.locator(`#review-event-${contestHash}`)).toContainText("Contest status: upheld");
    await expect(page.locator(`#review-event-${originalHash}`)).toContainText("Review status: compensated");
  });
});
