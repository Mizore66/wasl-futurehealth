import { readFileSync } from "node:fs";
import path from "node:path";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ConsentCard, SignoffsCard } from "@/components/case/sections";
import { CONSENT_ACTIONS, availability, failureView, historyRows, overlayWords } from "@/components/case/consent-model";
import { apiContactOf, consentHead, getConsent, postConsent } from "@/lib/api/consent";
import { CaseDetail, ConsentError, ConsentResponse, ConsentState } from "@/lib/api/schema";
import { loadSample } from "./samples";

/*
 * fixtures/consent_*.json are GET/POST /api/cases/{id}/consent and GET /api/cases/{id} as the Go API serves them,
 * captured from the authenticated handler with role-bound tokens (pipeline/api_consent_contract_test.go,
 * WASL_WRITE_CONSENT_FIXTURES=1): grant, duplicate grant, deny, grant, opt-out, then the refusals. Real handler output,
 * not hand-written JSON.
 */
const load = (f: string) => JSON.parse(readFileSync(path.join(__dirname, "fixtures", f), "utf8"));

const revalidatePath = vi.fn();
vi.mock("next/cache", () => ({ revalidatePath: (...args: unknown[]) => revalidatePath(...args) }));

const TOKEN = "coordinator-secret-1234567890";
const live = () => {
  vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088/");
  vi.stubEnv("WASL_API_COORDINATOR_TOKEN", TOKEN);
};
const respond = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

beforeEach(() => revalidatePath.mockClear());
afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("consent contract (real Go handler output)", () => {
  it("GET /consent parses before any event and after the sequence, in ledger order", () => {
    const empty = ConsentState.parse(load("consent_get_empty.json"));
    expect([empty.overlay.status, empty.overlay.demo_pending_consent, empty.history.length]).toEqual(["none", "", 0]);
    const after = ConsentState.parse(load("consent_get.json"));
    expect(after.history.map((e) => e.decision)).toEqual(["consent_granted", "consent_granted", "consent_denied", "consent_granted", "consent_opted_out"]);
    expect(after.history.every((e) => e.actor === "coordinator-bound" && e.role === "coordinator")).toBe(true);
    expect(after.history.slice(1).every((e, i) => e.prev === after.history[i].hash)).toBe(true);
    expect(after.overlay).toMatchObject({ status: "opted_out", demo_pending_consent: "cancelled", contact_blocked: true, events: 5, canonical_unchanged: true, mode: "demo" });
  });

  it("grant releases the demo hold; denial and opt-out cancel demo outreach and block contact", () => {
    const granted = ConsentResponse.parse(load("consent_post_granted.json"));
    expect(granted.overlay).toMatchObject({ status: "granted", demo_pending_consent: "released", contact_blocked: false });
    for (const f of ["consent_post_denied.json", "consent_post_opted_out.json"]) {
      expect(ConsentResponse.parse(load(f)).overlay).toMatchObject({ demo_pending_consent: "cancelled", contact_blocked: true });
    }
    // The overlay never claims to change the simulation's consent.
    expect(granted.overlay.canonical_consent).toBe("granted");
    expect(granted.overlay.canonical_unchanged).toBe(true);
  });

  it("every refusal parses with its error, the stale head and the opt-out overlay", () => {
    for (const f of ["consent_bad_request.json", "consent_forbidden.json", "consent_unknown_case.json"]) expect(ConsentError.parse(load(f)).error).not.toBe("");
    expect(ConsentError.parse(load("consent_conflict_stale.json")).current_head).toMatch(/^[0-9a-f]{64}$/);
    expect(ConsentError.parse(load("consent_conflict_opted_out.json")).api_contact?.status).toBe("opted_out");
  });

  it("case detail carries api_contact; the write head is current.sig, which each event signed", () => {
    const detail = CaseDetail.parse(load("consent_case_detail.json"));
    const view = apiContactOf(detail);
    expect(view.kind).toBe("ok");
    if (view.kind !== "ok") return;
    expect(view.contact.history).toHaveLength(5);
    expect(view.contact.history.every((e) => e.head_signed === consentHead(detail))).toBe(true);
    expect(detail.case.consent).toBe("granted");
  });
});

describe("api_contact on snapshots and off-contract data", () => {
  it("is absent from the committed fixtures snapshot, and unreadable data is reported, not guessed", () => {
    const snap = CaseDetail.parse(loadSample("case_review_hold.json"));
    expect(apiContactOf(snap)).toEqual({ kind: "absent" });
    const bad = CaseDetail.parse({ ...snap, api_contact: { overlay: { status: "maybe" }, history: [] } });
    expect(apiContactOf(bad).kind).toBe("unreadable");
  });
});

describe("postConsent and getConsent", () => {
  it("posts with the coordinator token, the current head and no identity headers", async () => {
    live();
    const fetchMock = vi.fn().mockResolvedValue(respond(201, load("consent_post_granted.json")));
    vi.stubGlobal("fetch", fetchMock);
    const res = await postConsent("exp-prov/ServiceRequest/EXP-sr-1", "consent_granted", "head-1");
    expect(res.ok).toBe(true);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("http://127.0.0.1:8088/api/cases/exp-prov%2FServiceRequest%2FEXP-sr-1/consent");
    const headers = new Headers(init.headers);
    expect(headers.get("authorization")).toBe(`Bearer ${TOKEN}`);
    expect(headers.get("x-wasl-role")).toBeNull();
    expect(headers.get("x-wasl-actor")).toBeNull();
    expect(JSON.parse(init.body)).toEqual({ event: "consent_granted", expected_head: "head-1", note: "" });
  });

  it.each([
    [400, "consent_bad_request.json", {}],
    [403, "consent_forbidden.json", {}],
    [404, "consent_unknown_case.json", {}],
    [409, "consent_conflict_stale.json", { currentHead: expect.any(String) }],
    [409, "consent_conflict_opted_out.json", { overlay: expect.objectContaining({ status: "opted_out" }) }],
  ] as const)("returns a %i (%s) as a refusal", async (status, file, extra) => {
    live();
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(respond(status, load(file))));
    const res = await postConsent("c", "consent_granted", "h");
    expect(res).toMatchObject({ ok: false, status, error: load(file).error, ...extra });
  });

  it("throws on an unexplained status and on fixtures mode; rejects a bad event without a request", async () => {
    live();
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(respond(502, {})));
    await expect(postConsent("c", "consent_granted", "h")).rejects.toThrow("HTTP 502");
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    expect(await postConsent("c", "consent_maybe" as never, "h")).toMatchObject({ ok: false, status: 400 });
    expect(await postConsent("c", "consent_granted", "")).toMatchObject({ ok: false, status: 400 });
    expect(fetchMock).not.toHaveBeenCalled();
    vi.stubEnv("WASL_API_URL", "");
    await expect(postConsent("c", "consent_granted", "h")).rejects.toThrow("fixtures mode cannot append");
    await expect(getConsent("c")).rejects.toThrow("fixtures mode has no consent ledger");
  });

  it("getConsent parses the served state and returns null for an unknown case", async () => {
    live();
    vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(respond(200, load("consent_get.json"))).mockResolvedValueOnce(respond(404, load("consent_unknown_case.json"))));
    expect((await getConsent("c"))?.history).toHaveLength(5);
    expect(await getConsent("nowhere")).toBeNull();
  });
});

describe("recordConsent server action", () => {
  it("refuses in fixtures mode without a request", async () => {
    const { recordConsent } = await import("@/app/app/patients/[id]/actions");
    vi.stubEnv("WASL_API_URL", "");
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    expect(await recordConsent("c", "h", "consent_granted")).toMatchObject({ ok: false, status: 503 });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("returns the API's actor and re-reads the case pages after a 201", async () => {
    const { recordConsent } = await import("@/app/app/patients/[id]/actions");
    live();
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(respond(201, load("consent_post_denied.json"))));
    const res = await recordConsent("exp-prov/ServiceRequest/EXP-sr-1", "head-1", "consent_denied");
    expect(res).toMatchObject({ ok: true, actor: "coordinator-bound", overlay: { status: "denied", demo_pending_consent: "cancelled" } });
    expect(revalidatePath).toHaveBeenCalledWith("/app/patients/[id]", "page");
  });

  it("passes a stale head back with the current one and re-reads nothing", async () => {
    const { recordConsent } = await import("@/app/app/patients/[id]/actions");
    live();
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(respond(409, load("consent_conflict_stale.json"))));
    const res = await recordConsent("c", "old-head", "consent_granted");
    expect(res).toMatchObject({ ok: false, status: 409, currentHead: load("consent_conflict_stale.json").current_head });
    expect(revalidatePath).not.toHaveBeenCalled();
    expect(await recordConsent("c", "h", "consent_maybe")).toMatchObject({ ok: false, status: 400 });
  });
});

describe("consent action logic", () => {
  const ctx = { live: true, head: "h", status: null };

  it("offers all three actions only with the live API and a head", () => {
    for (const a of CONSENT_ACTIONS) {
      expect(availability(a.event, ctx).enabled).toBe(true);
      expect(availability(a.event, { ...ctx, live: false })).toMatchObject({ enabled: false, reason: expect.stringContaining("Needs the live API") });
      expect(availability(a.event, { ...ctx, head: null }).enabled).toBe(false);
    }
  });

  it("after an opt-out only a repeated opt-out is offered, as the API enforces", () => {
    const after = { ...ctx, status: "opted_out" as const };
    expect(CONSENT_ACTIONS.filter((a) => availability(a.event, after).enabled).map((a) => a.event)).toEqual(["consent_opted_out"]);
    expect(availability("consent_granted", { ...ctx, status: "denied" }).enabled).toBe(true);
  });

  it("confirmation copy names contact permission and the demo overlay without implying a change to the simulation", () => {
    expect(CONSENT_ACTIONS.map((a) => a.label)).toEqual(["Consent granted", "Consent denied", "Opted out"]);
    for (const a of CONSENT_ACTIONS) expect(a.confirmTitle).toMatch(/contact/);
    expect(CONSENT_ACTIONS[0].confirmBody).toContain("demo then shows the held message as released");
  });

  it("maps each refusal to an in-place message; only a stale head or unknown case offers a reload", () => {
    expect(failureView({ status: 409, error: "stale", currentHead: "h2" })).toMatchObject({ reload: true, title: "This case changed since the page loaded" });
    expect(failureView({ status: 409, error: "patient has opted out" })).toMatchObject({ reload: false, detail: expect.stringContaining("patient has opted out") });
    expect(failureView({ status: 400, error: "bad" }).reload).toBe(false);
    expect(failureView({ status: 403, error: "rbac" }).title).toBe("This role may not record consent");
    expect(failureView({ status: 404, error: "unknown case" }).reload).toBe(true);
    expect(failureView({ status: 500, error: "ledger" }).title).toBe("Nothing was recorded");
  });

  it("history rows keep every event in ledger order, duplicates included", () => {
    const state = ConsentState.parse(load("consent_get.json"));
    const rows = historyRows(state.history);
    expect(rows.map((r) => `${r.what}: ${r.effect}`)).toEqual([
      "Consent granted: demo hold released",
      "Consent granted: demo hold released",
      "Consent denied: demo outreach cancelled",
      "Consent granted: demo hold released",
      "Opted out: demo outreach cancelled",
    ]);
    expect(overlayWords(state.overlay)).toBe("Opted out: demo outreach cancelled; contact confirmation blocked.");
    expect(overlayWords(ConsentState.parse(load("consent_get_empty.json")).overlay)).toBe("No demo consent event yet: nothing held or released by the demo yet.");
  });
});

describe("ConsentCard", () => {
  it("renders the overlay and its events below the unchanged canonical history, and actions in live mode", () => {
    const detail = CaseDetail.parse(load("consent_case_detail.json"));
    const html = renderToStaticMarkup(ConsentCard({ detail, live: true }));
    expect(html).toContain("Demo consent overlay");
    expect(html).toContain("never changes the simulation&#x27;s consent, outreach or decisions, and no message is sent");
    expect(html).toContain("Opted out: demo outreach cancelled; contact confirmation blocked.");
    expect(html).toContain("Simulation consent unchanged: allowed text messages.");
    expect(html.match(/<li /g)?.length).toBeGreaterThanOrEqual(5);
    expect(html).toContain("coordinator-bound");
    expect(html).toContain("The patient opted out: a later grant or denial needs a new, separately recorded consent flow.");
    expect(html).not.toContain("Needs the live API");
  });

  it("in fixtures mode says the overlay is absent and disables every action with the reason", () => {
    const detail = CaseDetail.parse(loadSample("case_review_hold.json"));
    const html = renderToStaticMarkup(ConsentCard({ detail }));
    expect(html).toContain("This case data carries no demo consent overlay.");
    expect(html).toContain("Needs the live API: fixtures mode has no ledger to record consent on.");
    expect(html.match(/<button[^>]*disabled=""/g)).toHaveLength(3);
  });

  it("consent events are not listed as sign-offs", () => {
    const detail = CaseDetail.parse(load("consent_case_detail.json"));
    const html = renderToStaticMarkup(SignoffsCard({ detail }));
    expect(html).not.toMatch(/consent/i);
    expect(html).toContain("No human sign-offs recorded for this case.");
  });
});
