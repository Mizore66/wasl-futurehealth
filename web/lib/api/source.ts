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
  ConsentAppendResponse,
  ConsentError,
  ConsentRequest,
  ConsentResponse,
  ContestRequest,
  ContestResolutionRequest,
  Dashboard,
  Fixtures,
  Operations,
  Healthz,
  ReviewError,
  ReviewRequest,
  ReviewResponse,
  UndoProjection,
  UndoRequest,
  type CaseSummary,
  type CompensationResponse as CompensationResponseBody,
  type ConsentRequest as ConsentRequestInput,
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

/**
 * `web_reviewer` is the credential for generic shared reads (dashboard, lists, case detail on shared pages). It is bound
 * to its own non-reader actor, so a first read concealed from everyone but its author is concealed from these pages too.
 * It never falls back to the assigned reviewer tokens, which are initial readers.
 */
export type WebCredential = "web_reviewer";

/** The web server has no token for this credential: a configuration fault here, not an API outage. */
export class MissingCredentialError extends Error {
  constructor(readonly variable: string) {
    super(`${variable} is required for the live synthetic API`);
    this.name = "MissingCredentialError";
  }
}

/** The API answered a read with a status other than 2xx or 404. */
export class ApiStatusError extends Error {
  constructor(
    route: string,
    readonly status: number,
  ) {
    super(`${route}: HTTP ${status}`);
    this.name = "ApiStatusError";
  }
}

function apiToken(credential: Role | WebCredential): string {
  const name = `WASL_API_${credential.toUpperCase()}_TOKEN`;
  const token = process.env[name]?.trim();
  if (!token) throw new MissingCredentialError(name);
  return token;
}

export function authenticated(init: RequestInit | undefined, credential: Role | WebCredential): RequestInit {
  const headers = new Headers(init?.headers);
  headers.set("authorization", `Bearer ${apiToken(credential)}`);
  return { cache: "no-store", ...init, headers };
}

export function liveBase(): string {
  return process.env.WASL_API_URL!.replace(/\/$/, "");
}

async function live<S extends z.ZodType>(schema: S, route: string, init?: RequestInit, role: Role | WebCredential = "web_reviewer"): Promise<z.infer<S> | null> {
  const res = await fetch(liveBase() + route, authenticated(init, role));
  if (res.status === 404) return null;
  if (!res.ok) throw new ApiStatusError(route, res.status);
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

export async function getConsent(id: string): Promise<ConsentResponse | null> {
  if (apiMode() === "live") return live(ConsentResponse, `/api/cases/${encodeURIComponent(id)}/consent`);
  const detail = await getCase(id);
  if (!detail?.api_contact) return null;
  return ConsentResponse.parse({ meta: detail.meta, case: detail.case.id, ...detail.api_contact });
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

export type ConsentResult =
  /** `response` is null when the event was appended (201) but its body could not be read. */
  | { ok: true; status: 201; response: ConsentAppendResponse | null }
  | { ok: false; status: 400 | 403 | 404 | 409 | 500; error: string; currentHead?: string };

/** Append one demo consent event. The API binds the coordinator token to its role and actor; browsers never supply either. */
export async function postConsent(id: string, body: ConsentRequestInput): Promise<ConsentResult> {
  const request = ConsentRequest.safeParse(body);
  if (!request.success) return { ok: false, status: 400, error: request.error.message };
  if (apiMode() !== "live") throw new Error("postConsent needs WASL_API_URL: fixtures mode cannot append consent events");

  const res = await fetch(
    `${liveBase()}/api/cases/${encodeURIComponent(id)}/consent`,
    authenticated(
      { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(request.data) },
      "coordinator",
    ),
  );
  if (res.status === 201) {
    /* The append already happened: an unreadable body must not be reported as a failure. */
    const saved = ConsentAppendResponse.safeParse(await res.json().catch(() => undefined));
    return { ok: true, status: 201, response: saved.success ? saved.data : null };
  }
  const json = await res.json();
  if (res.status === 400 || res.status === 403 || res.status === 404 || res.status === 409) {
    const e = ConsentError.parse(json);
    return { ok: false, status: res.status, error: e.error, currentHead: e.current_head };
  }
  const refused = res.status === 500 ? ConsentError.safeParse(json) : null;
  if (refused?.success) return { ok: false, status: 500, error: refused.data.error };
  throw new Error(`POST consent: HTTP ${res.status}`);
}

export type GovernanceResult =
  | { ok: true; status: 201; response: CompensationResponseBody }
  | { ok: false; status: 400 | 401 | 403 | 404 | 409 | 500 | 503; error: string };

/**
 * Undo, contest and contest resolution are sent with the signed-in person's own assigned token (the reviewer-session
 * cookie), never with a server-held role credential. The API binds that token to its configured role and immutable
 * actor and checks independence; no identity header is sent.
 */
async function postGovernanceWithToken(id: string, body: unknown, token: string): Promise<GovernanceResult> {
  if (apiMode() !== "live") throw new Error("governance actions need WASL_API_URL: fixtures mode cannot append governance events");
  const res = await fetch(
    `${liveBase()}/api/cases/${encodeURIComponent(id)}/undo`,
    bearer({ method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(body) }, token),
  );
  const json = await res.json().catch(() => undefined);
  if (res.status === 201) return { ok: true, status: 201, response: CompensationResponse.parse(json) };
  if ([400, 401, 403, 404, 409, 500, 503].includes(res.status)) {
    const e = ApiError.safeParse(json);
    return { ok: false, status: res.status as 400 | 401 | 403 | 404 | 409 | 500 | 503, error: e.success ? e.data.error : `HTTP ${res.status}` };
  }
  throw new Error(`POST undo: HTTP ${res.status}`);
}

export async function postUndoWithToken(id: string, body: UndoRequestBody, token: string): Promise<GovernanceResult> {
  const request = UndoRequest.safeParse(body);
  if (!request.success) return { ok: false, status: 400, error: request.error.message };
  return postGovernanceWithToken(id, request.data, token);
}

export async function postContestWithToken(id: string, body: ContestRequestBody, token: string): Promise<GovernanceResult> {
  const request = ContestRequest.safeParse(body);
  if (!request.success) return { ok: false, status: 400, error: request.error.message };
  return postGovernanceWithToken(id, request.data, token);
}

export async function postContestResolutionWithToken(
  id: string,
  body: ContestResolutionRequestBody,
  token: string,
): Promise<GovernanceResult> {
  const request = ContestResolutionRequest.safeParse(body);
  if (!request.success) return { ok: false, status: 400, error: request.error.message };
  return postGovernanceWithToken(id, request.data, token);
}

export async function getUndoProjection(id: string): Promise<UndoProjectionBody | null> {
  if (apiMode() !== "live") throw new Error("getUndoProjection needs WASL_API_URL: fixtures do not include a live governance projection");
  return live(UndoProjection, `/api/cases/${encodeURIComponent(id)}/undo`);
}

export type ActionResult =
  | { ok: true; status: 201; response: ActionResponse }
  | { ok: false; status: 400 | 401 | 403 | 404 | 409 | 500; error: string; conflict?: string; currentHead?: string };

/** Today's words for a 401 on a coordinator action: the API did not accept the server-side coordinator token. */
export const COORDINATOR_TOKEN_REFUSED = "the API did not accept this server's coordinator credential; an operator must fix the server configuration";
/** Today's words when this server has no coordinator token at all, so nothing was sent. */
export const COORDINATOR_TOKEN_MISSING = "this server has no coordinator credential configured, so nothing was sent; an operator must fix the server configuration";

/**
 * A coordinator action (assign owner, snooze, contact outcome, note), appended to the API's hash-chained ledger.
 * Sent with the server-side coordinator credential (WASL_API_COORDINATOR_TOKEN); the API binds that token to its role
 * and actor and records that actor, so no identity header is sent from here. `expectedHead` is the case's
 * `api_overlay.head`; a stale head returns 409 with the current one. A 500 that the API explains (its ledger failed
 * validation, or an append failed and nothing was recorded) is returned as a refusal, not thrown. So are a 401 (the API
 * did not accept the coordinator token) and a missing coordinator token (nothing is sent), both as status 401 with
 * words that say which.
 */
export async function postAction(id: string, action: CaseAction, expectedHead: string): Promise<ActionResult> {
  const body = CaseAction.safeParse(action);
  if (!body.success) return { ok: false, status: 400, error: body.error.message };
  if (apiMode() !== "live") throw new Error("postAction needs WASL_API_URL: fixtures mode cannot append coordinator events");

  let init: RequestInit;
  try {
    init = authenticated(
      { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ ...body.data, expected_head: expectedHead }) },
      "coordinator",
    );
  } catch (e) {
    if (e instanceof MissingCredentialError) return { ok: false, status: 401, error: COORDINATOR_TOKEN_MISSING };
    throw e;
  }
  const res = await fetch(`${liveBase()}/api/cases/${encodeURIComponent(id)}/actions`, init);
  // The Go API's 401 body ({"error": "authentication required"}, no meta) says only that the token was not accepted.
  if (res.status === 401) return { ok: false, status: 401, error: COORDINATOR_TOKEN_REFUSED };
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

/** Authenticated point-in-time operational data. Failures propagate so the whole panel becomes unavailable. */
export async function getOperations(): Promise<Operations> {
  if (apiMode() === "live") {
    const operations = await live(Operations, "/api/operations", { signal: AbortSignal.timeout(3000) });
    if (!operations) throw new Error("/api/operations: not found");
    return operations;
  }
  return fixture(Operations, "operations.json");
}

/**
 * Raw-token review helpers. The reviewer's own assigned API token is the credential, sent as-is on GET and POST;
 * the API binds it to a role and actor, so no identity header is ever sent. The shared role tokens above are untouched.
 */
function bearer(init: RequestInit | undefined, token: string): RequestInit {
  const headers = new Headers(init?.headers);
  headers.set("authorization", `Bearer ${token}`);
  return { cache: "no-store", ...init, headers };
}

export type ReviewerCaseResult =
  | { ok: true; detail: CaseDetail }
  | { ok: false; status: 401 | 403 | 404 };

/** One case as the holder of `token` may see it (concealed first reads stay concealed). Other statuses throw. */
export async function getCaseWithToken(id: string, token: string): Promise<ReviewerCaseResult> {
  const route = `/api/cases/${encodeURIComponent(id)}`;
  const res = await fetch(liveBase() + route, bearer(undefined, token));
  if (res.status === 401 || res.status === 403 || res.status === 404) return { ok: false, status: res.status };
  if (!res.ok) throw new Error(`${route}: HTTP ${res.status}`);
  return { ok: true, detail: CaseDetail.parse(await res.json()) };
}

export type ReviewReadResult =
  /** `response` is null when the event was appended (201) but its body could not be parsed. */
  | { ok: true; status: 201; response: ReviewResponse | null }
  | { ok: false; status: 400 | 401 | 403 | 404 | 409; error: string; currentHead?: string };

/** Appends one independent-review read with the reviewer's own token. 400/401/403/404/409 are refusals, never successes. */
export async function postReviewWithToken(id: string, body: ReviewRequest, token: string): Promise<ReviewReadResult> {
  const request = ReviewRequest.safeParse(body);
  if (!request.success) return { ok: false, status: 400, error: request.error.message };
  const route = `/api/cases/${encodeURIComponent(id)}/review`;
  const res = await fetch(
    liveBase() + route,
    bearer({ method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(request.data) }, token),
  );
  const json = await res.json().catch(() => undefined);
  if (res.status === 201) {
    const saved = ReviewResponse.safeParse(json);
    return { ok: true, status: 201, response: saved.success ? saved.data : null };
  }
  if (res.status === 400 || res.status === 401 || res.status === 403 || res.status === 404 || res.status === 409) {
    const e = ReviewError.safeParse(json);
    return { ok: false, status: res.status, error: e.success ? e.data.error : `HTTP ${res.status}`, currentHead: e.success ? e.data.current_head : undefined };
  }
  throw new Error(`POST review: HTTP ${res.status}`);
}
