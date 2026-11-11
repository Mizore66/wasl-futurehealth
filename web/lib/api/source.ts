import { readFile, readdir } from "node:fs/promises";
import path from "node:path";
import type { z } from "zod";
import {
  ActionError,
  ActionResponse,
  ApiError,
  CaseAction,
  Audit,
  CaseDetail,
  CaseList,
  CompensationResponse,
  ContestRequest,
  ContestResolutionRequest,
  Dashboard,
  Fixtures,
  Healthz,
  ReviewRequest,
  ReviewResponse,
  UndoProjection,
  UndoRequest,
  type CaseSummary,
  type CompensationResponse as CompensationResponseBody,
  type ContestRequest as ContestRequestBody,
  type ContestResolutionRequest as ContestResolutionRequestBody,
  type Role,
  type UndoRequest as UndoRequestBody,
  type UndoProjection as UndoProjectionBody,
} from "./schema";

/*
 * The one data source for every screen.
 * WASL_API_URL set: the live Go API (scripts/demo.sh serve).
 * Unset: the snapshot in web/data/api (copied from docs/api-samples by scripts/sync-samples.mjs).
 */

export type ApiMode = "live" | "fixtures";

export function apiMode(): ApiMode {
  return process.env.WASL_API_URL ? "live" : "fixtures";
}

const FIXTURE_DIR = path.join(process.cwd(), "data", "api");

export type GovernanceCredential = Exclude<Role, "auditor"> | "reviewer_2" | "reviewer_3";

function apiToken(credential: Role | GovernanceCredential): string {
  const name = `WASL_API_${credential.toUpperCase()}_TOKEN`;
  const token = process.env[name]?.trim();
  if (!token) throw new Error(`${name} is required for the live synthetic API`);
  return token;
}

function authenticated(init: RequestInit | undefined, credential: Role | GovernanceCredential): RequestInit {
  const headers = new Headers(init?.headers);
  headers.set("authorization", `Bearer ${apiToken(credential)}`);
  return { cache: "no-store", ...init, headers };
}

function liveBase(): string {
  return process.env.WASL_API_URL!.replace(/\/$/, "");
}

async function live<S extends z.ZodType>(schema: S, route: string, init?: RequestInit, role: Role = "reviewer"): Promise<z.infer<S> | null> {
  const res = await fetch(liveBase() + route, authenticated(init, role));
  if (res.status === 404) return null;
  if (!res.ok) throw new Error(`${route}: HTTP ${res.status}`);
  return schema.parse(await res.json());
}

async function fixture<S extends z.ZodType>(schema: S, file: string): Promise<z.infer<S>> {
  return schema.parse(JSON.parse(await readFile(path.join(FIXTURE_DIR, file), "utf8")));
}

async function fixtureCaseFiles(suffix: "" | "_audit"): Promise<string[]> {
  const files = await readdir(FIXTURE_DIR);
  return files.filter((f) => f.startsWith("case_") && f.endsWith(`${suffix}.json`) && (suffix || !f.endsWith("_audit.json")));
}

export async function getDashboard(): Promise<Dashboard> {
  if (apiMode() === "live") return (await live(Dashboard, "/api/dashboard"))!;
  return fixture(Dashboard, "dashboard.json");
}

export async function getFixtures(): Promise<Fixtures> {
  if (apiMode() === "live") return (await live(Fixtures, "/api/fixtures"))!;
  return fixture(Fixtures, "fixtures.json");
}

export type CaseQuery = Partial<{
  programme: CaseSummary["programme"];
  state: CaseSummary["state"];
  dropoff: CaseSummary["dropoff"];
  owner: string;
  /** Cases with no owner. The API's `owner` filter cannot ask for an empty owner, so this is applied here. */
  unowned: boolean;
  provider: string;
  consent: CaseSummary["consent"];
  review: CaseSummary["review"];
  overdue: boolean;
  fixture: boolean;
  q: string;
  limit: number;
  offset: number;
}>;

function toSearch(q: CaseQuery): string {
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(q)) {
    if (v === undefined || k === "unowned") continue;
    params.set(k, k === "fixture" ? (v ? "1" : "") : String(v));
  }
  const s = params.toString();
  return s ? `?${s}` : "";
}

export function matches(c: CaseSummary, q: CaseQuery): boolean {
  return (
    (q.programme === undefined || c.programme === q.programme) &&
    (q.state === undefined || c.state === q.state) &&
    (q.dropoff === undefined || c.dropoff === q.dropoff) &&
    (q.owner === undefined || c.owner === q.owner) &&
    (!q.unowned || c.owner === "") &&
    (q.provider === undefined || c.provider === q.provider) &&
    (q.consent === undefined || c.consent === q.consent) &&
    (q.review === undefined || c.review === q.review) &&
    (q.overdue === undefined || c.overdue === q.overdue) &&
    (!q.fixture || c.fixture !== "") &&
    (q.q === undefined || c.id.includes(q.q))
  );
}

/** Fixtures mode filters each programme's 25-case sample page plus the fixtures; the live API filters all cases. */
export async function getCases(query: CaseQuery = {}): Promise<CaseList> {
  if (apiMode() === "live") {
    if (!query.unowned) return (await live(CaseList, `/api/cases${toSearch(query)}`))!;
    // The API cannot filter on an empty owner: fetch every other match (500 a page) and keep the unowned ones.
    const { limit: want = 100, offset: skip = 0, ...rest } = query;
    const cases: CaseSummary[] = [];
    let first: CaseList | undefined;
    for (let offset = 0; ; offset += 500) {
      const page = (await live(CaseList, `/api/cases${toSearch({ ...rest, limit: 500, offset })}`))!;
      first ??= page;
      cases.push(...page.cases.filter((c) => c.owner === ""));
      if (offset + page.cases.length >= page.total || page.cases.length === 0) break;
    }
    const limit = Math.min(want, 500);
    return { ...first, cases: cases.slice(skip, skip + limit), total: cases.length, limit, offset: skip };
  }
  const pageFiles = (await readdir(FIXTURE_DIR)).filter((f) => /^cases_[a-z_]+_page1\.json$/.test(f)).sort();
  const [pages, fx] = await Promise.all([Promise.all(pageFiles.map((f) => fixture(CaseList, f))), getFixtures()]);
  const page = pages.find((p) => p.cases[0]?.programme === "crc") ?? pages[0];
  const byId = new Map<string, CaseSummary>();
  for (const c of [...Object.values(fx.fixtures), ...pages.flatMap((p) => p.cases)]) if (c && !byId.has(c.id)) byId.set(c.id, c);
  const all = [...byId.values()].filter((c) => matches(c, query));
  const limit = Math.min(query.limit ?? 100, 500);
  const offset = query.offset ?? 0;
  return { meta: page.meta, cases: all.slice(offset, offset + limit), total: all.length, limit, offset };
}

export async function getCase(id: string): Promise<CaseDetail | null> {
  if (apiMode() === "live") return live(CaseDetail, `/api/cases/${encodeURIComponent(id)}`);
  for (const f of await fixtureCaseFiles("")) {
    const detail = await fixture(CaseDetail, f);
    if (detail.case.id === id) return detail;
  }
  return null;
}

export async function getAudit(id: string): Promise<Audit | null> {
  if (apiMode() === "live") return live(Audit, `/api/cases/${encodeURIComponent(id)}/audit`, undefined, "auditor");
  for (const f of await fixtureCaseFiles("_audit")) {
    const audit = await fixture(Audit, f);
    if (audit.case === id) return audit;
  }
  return null;
}

export type ReviewResult =
  | { ok: true; status: 201; response: ReviewResponse }
  | { ok: false; status: 400 | 403 | 404; error: string };

export async function postReview(
  id: string,
  body: ReviewRequest,
  who: { role: Role; actor: string },
): Promise<ReviewResult> {
  const request = ReviewRequest.safeParse(body);
  if (!request.success) return { ok: false, status: 400, error: request.error.message };

  if (apiMode() === "live") {
    const res = await fetch(
      `${liveBase()}/api/cases/${encodeURIComponent(id)}/review`,
      authenticated({ method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(request.data) }, who.role),
    );
    const json = await res.json();
    if (res.status === 201) return { ok: true, status: 201, response: ReviewResponse.parse(json) };
    if (res.status === 400 || res.status === 403 || res.status === 404) {
      return { ok: false, status: res.status, error: ApiError.parse(json).error };
    }
    throw new Error(`POST review: HTTP ${res.status}`);
  }

  // Fixtures mode has no log to append to. Fabricating an event with a made-up hash would present a click as a saved review.
  throw new Error("postReview needs WASL_API_URL: fixtures mode cannot append review events");
}

export type GovernanceResult =
  | { ok: true; status: 201; response: CompensationResponseBody }
  | { ok: false; status: 400 | 403 | 404 | 409 | 500 | 503; error: string };

/** The API binds the selected credential to its configured role and immutable actor; the browser supplies neither. */
async function postGovernance(id: string, body: unknown, credential: GovernanceCredential): Promise<GovernanceResult> {
  if (apiMode() !== "live") throw new Error("governance actions need WASL_API_URL: fixtures mode cannot append governance events");
  const res = await fetch(
    `${liveBase()}/api/cases/${encodeURIComponent(id)}/undo`,
    authenticated({ method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(body) }, credential),
  );
  const json = await res.json();
  if (res.status === 201) return { ok: true, status: 201, response: CompensationResponse.parse(json) };
  if ([400, 403, 404, 409, 500, 503].includes(res.status)) {
    return { ok: false, status: res.status as 400 | 403 | 404 | 409 | 500 | 503, error: ApiError.parse(json).error };
  }
  throw new Error(`POST undo: HTTP ${res.status}`);
}

export async function postUndo(id: string, body: UndoRequestBody, credential: GovernanceCredential): Promise<GovernanceResult> {
  const request = UndoRequest.safeParse(body);
  if (!request.success) return { ok: false, status: 400, error: request.error.message };
  return postGovernance(id, request.data, credential);
}

export async function postContest(id: string, body: ContestRequestBody, credential: GovernanceCredential): Promise<GovernanceResult> {
  const request = ContestRequest.safeParse(body);
  if (!request.success) return { ok: false, status: 400, error: request.error.message };
  return postGovernance(id, request.data, credential);
}

export async function postContestResolution(
  id: string,
  body: ContestResolutionRequestBody,
  credential: GovernanceCredential,
): Promise<GovernanceResult> {
  const request = ContestResolutionRequest.safeParse(body);
  if (!request.success) return { ok: false, status: 400, error: request.error.message };
  return postGovernance(id, request.data, credential);
}

export async function getUndoProjection(id: string): Promise<UndoProjectionBody | null> {
  if (apiMode() !== "live") throw new Error("getUndoProjection needs WASL_API_URL: fixtures do not include a live governance projection");
  return live(UndoProjection, `/api/cases/${encodeURIComponent(id)}/undo`);
}

export type ActionResult =
  | { ok: true; status: 201; response: ActionResponse }
  | { ok: false; status: 400 | 403 | 404 | 409 | 500; error: string; conflict?: string; currentHead?: string };

/**
 * A coordinator action (assign owner, snooze, contact outcome, note), appended to the API's hash-chained ledger.
 * Sent with the server-side coordinator credential (WASL_API_COORDINATOR_TOKEN); the API binds that token to its role
 * and actor and records that actor, so no identity header is sent from here. `expectedHead` is the case's
 * `api_overlay.head`; a stale head returns 409 with the current one. A 500 that the API explains (its ledger failed
 * validation, or an append failed and nothing was recorded) is returned as a refusal, not thrown.
 */
export async function postAction(id: string, action: CaseAction, expectedHead: string): Promise<ActionResult> {
  const body = CaseAction.safeParse(action);
  if (!body.success) return { ok: false, status: 400, error: body.error.message };
  if (apiMode() !== "live") throw new Error("postAction needs WASL_API_URL: fixtures mode cannot append coordinator events");

  const res = await fetch(
    `${liveBase()}/api/cases/${encodeURIComponent(id)}/actions`,
    authenticated(
      { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ ...body.data, expected_head: expectedHead }) },
      "coordinator",
    ),
  );
  const json = await res.json();
  if (res.status === 201) return { ok: true, status: 201, response: ActionResponse.parse(json) };
  if (res.status === 400 || res.status === 403 || res.status === 404 || res.status === 409) {
    const e = ActionError.parse(json);
    return { ok: false, status: res.status, error: e.error, conflict: e.conflict, currentHead: e.current_head };
  }
  const refused = res.status === 500 ? ActionError.safeParse(json) : null;
  if (refused?.success) return { ok: false, status: 500, error: refused.data.error };
  throw new Error(`POST actions: HTTP ${res.status}`);
}

/**
 * The backend's `/healthz`. Live mode fetches it (short timeout, never cached); fixtures mode reads the published snapshot.
 * Throws on any fetch, status or parse failure: callers show an unavailable state, never a guessed value.
 */
export async function getHealthz(): Promise<Healthz> {
  if (apiMode() === "live") {
    const health = await live(Healthz, "/healthz", { signal: AbortSignal.timeout(3000) });
    if (!health) throw new Error("/healthz: not found");
    return health;
  }
  return fixture(Healthz, "healthz.json");
}
