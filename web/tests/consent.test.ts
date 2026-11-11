import { readFileSync } from "node:fs";
import path from "node:path";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ConsentCard, SignoffsCard } from "@/components/case/sections";
import { CaseDetail } from "@/lib/api/schema";
import * as apiSource from "@/lib/api/source";

const nextCache = vi.hoisted(() => ({ revalidatePath: vi.fn() }));
vi.mock("next/cache", () => ({ revalidatePath: nextCache.revalidatePath }));

const rawDetail = () => JSON.parse(readFileSync(path.join(__dirname, "fixtures", "ledger_case_detail.json"), "utf8"));
const meta = { api: "wasl-api/1", data_mode: "simulated", run: "r", policy: "p", reader_model: "m", loaded_at: "2026-11-10T00:00:00Z", note: "synthetic" };
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

type ConsentClient = {
  getConsent?: (id: string) => Promise<unknown>;
  postConsent?: (
    id: string,
    body: { event: "consent_granted" | "consent_denied" | "consent_opted_out"; expected_head: string; note?: string },
  ) => Promise<unknown>;
};
const consentClient = apiSource as unknown as ConsentClient;

function live() {
  vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088/");
  vi.stubEnv("WASL_API_REVIEWER_TOKEN", "reviewer-secret-1234567890");
  vi.stubEnv("WASL_API_COORDINATOR_TOKEN", "coordinator-secret-1234567890");
}

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
  nextCache.revalidatePath.mockClear();
});

describe("consent API schema", () => {
  it("keeps the demo overlay and its append-only history separate from canonical consent", () => {
    const result = CaseDetail.safeParse(rawDetail());
    expect(result.error?.issues ?? []).toEqual([]);
    expect(result.data!.api_contact?.overlay).toMatchObject({
      status: "granted",
      demo_pending_consent: "released",
      canonical_consent: "granted",
      canonical_unchanged: true,
    });
    expect(result.data!.api_contact?.history.map((event) => event.decision)).toEqual(["consent_granted"]);
    expect(result.data!.case.consent).toBe("granted");
  });

  it("rejects an overlay that claims the canonical simulation changed", () => {
    const detail = rawDetail();
    detail.api_contact.overlay.canonical_unchanged = false;
    expect(CaseDetail.safeParse(detail).success).toBe(false);
  });
});

describe("consent API client", () => {
  it("reads the overlay and history with the server-side reviewer token", async () => {
    live();
    const detail = rawDetail();
    const fetchMock = vi.fn().mockResolvedValue(json(200, { meta, case: detail.case.id, ...detail.api_contact }));
    vi.stubGlobal("fetch", fetchMock);
    expect(consentClient.getConsent).toBeTypeOf("function");
    if (!consentClient.getConsent) return;

    const result = await consentClient.getConsent("case/1");
    expect(result).toMatchObject({ case: detail.case.id, overlay: { status: "granted" }, history: [{ decision: "consent_granted" }] });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("http://127.0.0.1:8088/api/cases/case%2F1/consent");
    expect(new Headers(init.headers).get("authorization")).toBe("Bearer reviewer-secret-1234567890");
  });

  it("appends an event with the coordinator token and the canonical expected head", async () => {
    live();
    const detail = rawDetail();
    const fetchMock = vi.fn().mockResolvedValue(
      json(201, {
        meta,
        review: detail.api_contact.history[0],
        overlay: detail.api_contact.overlay,
        note: "appended to the review ledger; demo overlay only",
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    expect(consentClient.postConsent).toBeTypeOf("function");
    if (!consentClient.postConsent) return;

    const result = await consentClient.postConsent("case/1", {
      event: "consent_granted",
      expected_head: "canonical-head-signature",
      note: "Patient agreed to contact",
    });
    expect(result).toMatchObject({ ok: true, status: 201, response: { overlay: { demo_pending_consent: "released" } } });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("http://127.0.0.1:8088/api/cases/case%2F1/consent");
    const headers = new Headers(init.headers);
    expect(headers.get("authorization")).toBe("Bearer coordinator-secret-1234567890");
    expect(headers.has("x-wasl-role")).toBe(false);
    expect(headers.has("x-wasl-actor")).toBe(false);
    expect(JSON.parse(init.body)).toEqual({
      event: "consent_granted",
      expected_head: "canonical-head-signature",
      note: "Patient agreed to contact",
    });
  });

  it.each([400, 403, 404, 409])("returns HTTP %s in place instead of throwing", async (status) => {
    live();
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json(status, { meta, error: "consent refused", current_head: "current-head" })));
    expect(consentClient.postConsent).toBeTypeOf("function");
    if (!consentClient.postConsent) return;
    await expect(consentClient.postConsent("c", { event: "consent_denied", expected_head: "stale-head" })).resolves.toEqual({
      ok: false,
      status,
      error: "consent refused",
      currentHead: "current-head",
    });
  });

  it("rejects an overlong note before any request", async () => {
    live();
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    expect(consentClient.postConsent).toBeTypeOf("function");
    if (!consentClient.postConsent) return;
    const result = await consentClient.postConsent("c", { event: "consent_opted_out", expected_head: "h", note: "x".repeat(501) });
    expect(result).toMatchObject({ ok: false, status: 400 });
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe("recordConsent server action", () => {
  async function actionModule() {
    return import("../app/app/patients/[id]/actions").catch(() => ({ recordConsent: undefined }));
  }

  function deniedDetail() {
    const detail = rawDetail();
    const denied = {
      ...detail.api_contact.history[0],
      decision: "consent_denied",
      hash: "denied-event-hash",
      prev: detail.api_contact.history[0].hash,
      note: "Patient declined contact",
    };
    detail.api_contact.history = [...detail.api_contact.history, denied];
    detail.api_contact.overlay = {
      ...detail.api_contact.overlay,
      status: "denied",
      demo_pending_consent: "cancelled",
      events: 2,
      contact_blocked: true,
      last_event_hash: denied.hash,
    };
    return { detail, denied };
  }

  it("re-fetches the case after 201 and returns the refreshed overlay and history", async () => {
    live();
    const { detail, denied } = deniedDetail();
    const fetchMock = vi.fn().mockImplementation((url: string, init?: RequestInit) => {
      if (init?.method === "POST") {
        return Promise.resolve(json(201, { meta, review: denied, overlay: detail.api_contact.overlay, note: "demo overlay only" }));
      }
      expect(url).toBe(`http://127.0.0.1:8088/api/cases/${encodeURIComponent(detail.case.id)}`);
      return Promise.resolve(json(200, detail));
    });
    vi.stubGlobal("fetch", fetchMock);
    const { recordConsent } = await actionModule();
    expect(recordConsent).toBeTypeOf("function");
    if (!recordConsent) return;

    const result = await recordConsent(detail.case.id, detail.current.sig, "consent_denied", "Patient declined contact");
    expect(result).toMatchObject({
      ok: true,
      expectedHead: detail.current.sig,
      contact: { overlay: { status: "denied", demo_pending_consent: "cancelled" }, history: [{ decision: "consent_granted" }, { decision: "consent_denied" }] },
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(new Headers(fetchMock.mock.calls[0][1].headers).get("authorization")).toBe("Bearer coordinator-secret-1234567890");
    expect(new Headers(fetchMock.mock.calls[1][1].headers).get("authorization")).toBe("Bearer reviewer-secret-1234567890");
    expect(JSON.parse(fetchMock.mock.calls[0][1].body).expected_head).toBe(detail.current.sig);
    expect(nextCache.revalidatePath).toHaveBeenCalledWith(`/app/patients/${encodeURIComponent(detail.case.id)}`);
    expect(nextCache.revalidatePath).toHaveBeenCalledWith(`/app/patients/${encodeURIComponent(detail.case.id)}/audit`);
  });

  it("returns a stale-head conflict without refreshing or reporting a save", async () => {
    live();
    const fetchMock = vi.fn().mockResolvedValue(json(409, { meta, error: "stale expected_head", current_head: "new-head" }));
    vi.stubGlobal("fetch", fetchMock);
    const { recordConsent } = await actionModule();
    expect(recordConsent).toBeTypeOf("function");
    if (!recordConsent) return;

    await expect(recordConsent("c", "stale-head", "consent_granted", "")).resolves.toEqual({
      ok: false,
      status: 409,
      error: "stale expected_head",
      currentHead: "new-head",
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(nextCache.revalidatePath).not.toHaveBeenCalled();
  });

  it("says the event was saved even if the read-back cannot find the case", async () => {
    live();
    const { detail, denied } = deniedDetail();
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(json(201, { meta, review: denied, overlay: detail.api_contact.overlay, note: "demo overlay only" }))
      .mockResolvedValueOnce(json(404, { meta, error: "unknown case" }));
    vi.stubGlobal("fetch", fetchMock);
    const { recordConsent } = await actionModule();
    expect(recordConsent).toBeTypeOf("function");
    if (!recordConsent) return;

    const result = await recordConsent(detail.case.id, detail.current.sig, "consent_denied", "Patient declined contact");
    expect(result).toMatchObject({ ok: true, contact: null, warning: "Consent event saved, but the updated case could not be read. Reload before another action." });
    expect(result).not.toMatchObject({ ok: false });
  });
});

describe("consent card demo overlay", () => {
  function consentEvent(decision: string, hash: string, note: string) {
    return { ...rawDetail().api_contact.history[0], decision, hash, note };
  }

  function heldDetail(status: string, demoPendingConsent: string, history: unknown[]) {
    const raw = rawDetail();
    raw.case.consent = "unknown";
    raw.outreach = [{ kind: "patient_notification", ev: "held-message-1", released: false, suppress: "consent_unknown" }];
    raw.api_contact = {
      overlay: {
        ...raw.api_contact.overlay,
        status,
        demo_pending_consent: demoPendingConsent,
        canonical_consent: "unknown",
        canonical_unchanged: true,
        contact_blocked: status === "denied" || status === "opted_out",
        events: history.length,
      },
      history,
    };
    return CaseDetail.parse(raw);
  }

  it("shows a grant releasing held demo outreach without rewriting the canonical hold", () => {
    const detail = heldDetail("granted", "released", [consentEvent("consent_granted", "grant-hash", "Permission recorded")]);
    const html = renderToStaticMarkup(ConsentCard({ detail }));
    expect(html).toContain("Demo outreach released");
    expect(html).toContain("The demo consent hold is released. No message was sent.");
    expect(html).toContain("stopped: no consent on file yet");
    expect(html).toContain("No answer on messages yet");
    expect(html).toContain("Canonical consent and outreach are unchanged");
  });

  it.each(["denied", "opted_out"])("shows %s cancelling queued demo outreach without dispatch", (status) => {
    const eventName = status === "denied" ? "consent_denied" : "consent_opted_out";
    const detail = heldDetail(status, "cancelled", [consentEvent(eventName, `${status}-hash`, "Contact refused")]);
    const html = renderToStaticMarkup(ConsentCard({ detail }));
    expect(html).toContain("Demo outreach cancelled");
    expect(html).toContain("Queued demo outreach is cancelled. No message was sent.");
    expect(html).toContain("stopped: no consent on file yet");
    expect(html).toContain("No answer on messages yet");
  });

  it("keeps duplicate and reordered events append-only in the API's ledger order", () => {
    const history = [
      consentEvent("consent_granted", "hash-1", "first event"),
      consentEvent("consent_denied", "hash-2", "second event"),
      consentEvent("consent_granted", "hash-3", "third event"),
    ];
    const detail = heldDetail("granted", "released", history);
    const html = renderToStaticMarkup(ConsentCard({ detail }));
    expect(html.match(/Consent granted/g)).toHaveLength(2);
    expect(html.match(/Consent denied/g)).toHaveLength(1);
    expect(html.indexOf("first event")).toBeLessThan(html.indexOf("second event"));
    expect(html.indexOf("second event")).toBeLessThan(html.indexOf("third event"));
    expect(html).toContain("Demo outreach released");
    expect(html).toContain("stopped: no consent on file yet");
  });

  it("uses action-specific confirmation copy and never offers message dispatch", () => {
    live();
    const detail = heldDetail("none", "", []);
    const html = renderToStaticMarkup(ConsentCard({ detail }));
    expect(html).toContain("Record consent granted");
    expect(html).toContain("Releases the demo consent hold");
    expect(html).toContain("Record consent denied");
    expect(html).toContain("Cancels queued demo outreach");
    expect(html).toContain("Record opt-out");
    expect(html).toContain("blocks a later grant in this demo flow");
    expect(html).toContain("No message is sent and the canonical simulation does not change.");
  });

  it("does not present consent overlay events as clinical sign-offs", () => {
    const detail = heldDetail("granted", "released", [consentEvent("consent_granted", "grant-hash", "Permission recorded")]);
    const html = renderToStaticMarkup(SignoffsCard({ detail }));
    expect(html).not.toContain("Consent granted");
    expect(html).not.toContain("Permission recorded");
  });
});
