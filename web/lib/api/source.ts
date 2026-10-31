import { createHash } from "node:crypto";
import { readFile, readdir } from "node:fs/promises";
import path from "node:path";
import type { z } from "zod";
import {
  ApiError,
  Audit,
  CaseDetail,
  CaseList,
  Dashboard,
  Fixtures,
  ReviewRequest,
  ReviewResponse,
  type CaseSummary,
  type Role,
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

async function live<S extends z.ZodType>(schema: S, route: string, init?: RequestInit): Promise<z.infer<S> | null> {
  const base = process.env.WASL_API_URL!.replace(/\/$/, "");
  const res = await fetch(base + route, { cache: "no-store", ...init });
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
    if (v === undefined) continue;
    params.set(k, k === "fixture" ? (v ? "1" : "") : String(v));
  }
  const s = params.toString();
  return s ? `?${s}` : "";
}

function matches(c: CaseSummary, q: CaseQuery): boolean {
  return (
    (q.programme === undefined || c.programme === q.programme) &&
    (q.state === undefined || c.state === q.state) &&
    (q.dropoff === undefined || c.dropoff === q.dropoff) &&
    (q.owner === undefined || c.owner === q.owner) &&
    (q.provider === undefined || c.provider === q.provider) &&
    (q.consent === undefined || c.consent === q.consent) &&
    (q.review === undefined || c.review === q.review) &&
    (q.overdue === undefined || c.overdue === q.overdue) &&
    (!q.fixture || c.fixture !== "") &&
    (q.q === undefined || c.id.includes(q.q))
  );
}

/** Fixtures mode filters the 25-case sample page plus the three fixtures; the live API filters all cases. */
export async function getCases(query: CaseQuery = {}): Promise<CaseList> {
  if (apiMode() === "live") return (await live(CaseList, `/api/cases${toSearch(query)}`))!;
  const [page, fx] = await Promise.all([fixture(CaseList, "cases_crc_page1.json"), getFixtures()]);
  const byId = new Map<string, CaseSummary>();
  for (const c of [...Object.values(fx.fixtures), ...page.cases]) if (!byId.has(c.id)) byId.set(c.id, c);
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
  if (apiMode() === "live") return live(Audit, `/api/cases/${encodeURIComponent(id)}/audit`);
  for (const f of await fixtureCaseFiles("_audit")) {
    const audit = await fixture(Audit, f);
    if (audit.case === id) return audit;
  }
  return null;
}

export type ReviewResult =
  | { ok: true; status: 201; response: ReviewResponse }
  | { ok: false; status: 400 | 403 | 404; error: string };

const ACTION_BY_DECISION: Record<string, string> = {
  same: "signoff_link",
  different: "signoff_link",
  adequate: "signoff_evidence",
  inadequate: "signoff_evidence",
  confirm_contact: "record_consent",
};

/** Mirror of pipeline/governance.go `rbac`, used only in fixtures mode. */
const RBAC: Record<Role, readonly string[]> = {
  coordinator: ["record_consent"],
  clinician: ["signoff_evidence", "record_consent"],
  reviewer: ["signoff_link"],
  auditor: [],
};

export async function postReview(
  id: string,
  body: ReviewRequest,
  who: { role: Role; actor: string },
): Promise<ReviewResult> {
  const request = ReviewRequest.safeParse(body);
  if (!request.success) return { ok: false, status: 400, error: request.error.message };

  if (apiMode() === "live") {
    const base = process.env.WASL_API_URL!.replace(/\/$/, "");
    const res = await fetch(`${base}/api/cases/${encodeURIComponent(id)}/review`, {
      method: "POST",
      cache: "no-store",
      headers: { "content-type": "application/json", "X-Wasl-Role": who.role, "X-Wasl-Actor": who.actor },
      body: JSON.stringify(request.data),
    });
    const json = await res.json();
    if (res.status === 201) return { ok: true, status: 201, response: ReviewResponse.parse(json) };
    if (res.status === 400 || res.status === 403 || res.status === 404) {
      return { ok: false, status: res.status, error: ApiError.parse(json).error };
    }
    throw new Error(`POST review: HTTP ${res.status}`);
  }

  const detail = await getCase(id);
  if (!detail) return { ok: false, status: 404, error: "case not found" };
  const lungEvidence = detail.case.programme === "lung_nodule" && !(request.data.decision in ACTION_BY_DECISION);
  const action = ACTION_BY_DECISION[request.data.decision] ?? (lungEvidence ? "signoff_evidence" : undefined);
  if (!action) return { ok: false, status: 400, error: "decision must be same|different|adequate|inadequate|confirm_contact" };
  if (!RBAC[who.role].includes(action)) return { ok: false, status: 403, error: `rbac: role "${who.role}" may not ${action}` };
  if (!who.actor) return { ok: false, status: 400, error: "X-Wasl-Actor header required (who is signing)" };
  if (request.data.item && !detail.review_items.some((r) => r.item === request.data.item)) {
    return { ok: false, status: 404, error: "review item not raised for this case" };
  }
  const event = {
    case: id,
    item: request.data.item,
    decision: request.data.decision,
    note: request.data.note,
    role: who.role,
    actor: who.actor,
    action,
    at: new Date().toISOString(),
    mode: "fixtures",
    prev: "genesis",
  };
  const hash = createHash("sha256").update(JSON.stringify(event)).digest("hex");
  return {
    ok: true,
    status: 201,
    response: { meta: detail.meta, review: { ...event, hash }, note: "fixtures mode: not persisted" },
  };
}
