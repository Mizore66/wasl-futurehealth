import { renderToStaticMarkup } from "react-dom/server";
import { createElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { revalidatePath } from "next/cache";
import { approveOutcomeHandoff, releaseOutcomeHandoff } from "@/lib/api/outcome-handoff-actions";
import { reviewerToken } from "@/lib/api/reviewer-session";
import { OutcomeHandoffCard } from "@/components/case/outcome-handoff-card";
import { outcomeHandoffActions, type HandoffSession } from "@/components/case/outcome-handoff-review";
import { OutcomeHandoff } from "@/lib/api/schema";

vi.mock("next/cache", () => ({ revalidatePath: vi.fn(), refresh: vi.fn() }));
vi.mock("@/lib/api/reviewer-session", () => ({ reviewerToken: vi.fn(), resolveReviewer: vi.fn(), signInReviewer: vi.fn(), signOutReviewer: vi.fn() }));

/** The signed-in clinician's own token, held server-side in the reviewer-session cookie. Synthetic. */
const SESSION = "signed-in-clinician-token-0123456789";
const COORDINATOR = "server-coordinator-token-1234567890";
const CLINICIAN = "server-clinician-token-1234567890";
const DECISION = "d".repeat(64);
const VERSION = "a".repeat(64);
const fetchMock = vi.fn();

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
  vi.stubEnv("WASL_API_WEB_REVIEWER_TOKEN", "server-web-reviewer-token-1234567890");
  vi.stubEnv("WASL_API_CLINICIAN_TOKEN", CLINICIAN);
  vi.stubEnv("WASL_API_COORDINATOR_TOKEN", COORDINATOR);
  vi.stubGlobal("fetch", fetchMock);
  vi.mocked(reviewerToken).mockResolvedValue(SESSION);
});

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
const approve = (extra: object = {}) => approveOutcomeHandoff({ caseId: "LOOP/1", decision: DECISION, nextStep: "surveillance", version: "", ...extra });

describe("approveOutcomeHandoff acts only as the signed-in credential", () => {
  it("posts content_approved with the session token, never a server-held credential", async () => {
    fetchMock.mockResolvedValue(json(201, { event: { hash: "approval-hash-0123456789" }, replayed: false }));
    const result = await approve();
    expect(result).toMatchObject({ ok: true, hash: "approval-hash-0123456789" });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("http://127.0.0.1:8088/api/cases/LOOP%2F1/outcome-handoff");
    const auth = new Headers(init.headers).get("authorization");
    expect(auth).toBe(`Bearer ${SESSION}`);
    expect(auth).not.toContain(CLINICIAN);
    expect(new Headers(init.headers).has("x-wasl-role")).toBe(false);
    expect(JSON.parse(init.body)).toEqual({ kind: "content_approved", decision: DECISION, next_step: "surveillance", version: "" });
    expect(revalidatePath).toHaveBeenCalledWith("/app/patients/LOOP%2F1");
  });

  it.each([["credential", { credential: "clinician" }], ["token", { token: "x".repeat(30) }], ["role", { role: "clinician" }], ["actor", { actor: "dr-gp" }]])(
    "refuses a client-supplied %s and sends nothing",
    async (_name, identity) => {
      const result = await approve(identity);
      expect(result.ok).toBe(false);
      expect(fetchMock).not.toHaveBeenCalled();
    },
  );

  it.each([undefined, "", "start_chemotherapy"])("has no default next step: %j is refused before any request", async (nextStep) => {
    expect((await approve({ nextStep })).ok).toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("asks for a sign-in and sends nothing without a session", async () => {
    vi.mocked(reviewerToken).mockResolvedValue(undefined);
    expect(await approve()).toMatchObject({ ok: false, signIn: true });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("shows the server's refusal and does not revalidate", async () => {
    fetchMock.mockResolvedValue(json(403, { error: "role coordinator may not approve_outcome_handoff" }));
    const result = await approve();
    expect(result).toEqual({ ok: false, error: "Not recorded: role coordinator may not approve_outcome_handoff" });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("shows a stale-approval conflict verbatim", async () => {
    fetchMock.mockResolvedValue(json(409, { error: "the outcome hand-off changed since it was loaded; review the current approval" }));
    const result = await approve({ version: "b".repeat(64) });
    expect(result.ok === false && result.error).toContain("changed since it was loaded");
  });

  it("treats a rejected session as a sign-in prompt", async () => {
    fetchMock.mockResolvedValue(json(401, { error: "authentication required" }));
    expect(await approve()).toMatchObject({ ok: false, signIn: true });
  });

  it("says when the same content was already approved", async () => {
    fetchMock.mockResolvedValue(json(200, { event: null, replayed: true }));
    const result = await approve();
    expect(result.ok && result.message).toContain("already approved");
  });

  it("refuses in fixtures mode without sending", async () => {
    vi.stubEnv("WASL_API_URL", "");
    expect((await approve()).ok).toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe("releaseOutcomeHandoff uses the server's coordinator credential", () => {
  it("posts release_recorded for the approved version with the coordinator token, not the session", async () => {
    fetchMock.mockResolvedValue(json(201, { event: { hash: "release-hash-0123456789" }, replayed: false }));
    const result = await releaseOutcomeHandoff({ caseId: "c1", version: VERSION });
    expect(result).toMatchObject({ ok: true, hash: "release-hash-0123456789" });
    const [, init] = fetchMock.mock.calls[0];
    expect(new Headers(init.headers).get("authorization")).toBe(`Bearer ${COORDINATOR}`);
    expect(JSON.parse(init.body)).toEqual({ kind: "release_recorded", version: VERSION });
    expect(reviewerToken).not.toHaveBeenCalled();
  });

  it("refuses a client-supplied identity and sends nothing", async () => {
    expect((await releaseOutcomeHandoff({ caseId: "c1", version: VERSION, role: "coordinator" })).ok).toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("shows the server's refusal", async () => {
    fetchMock.mockResolvedValue(json(409, { error: "version is not the current approved content" }));
    expect(await releaseOutcomeHandoff({ caseId: "c1", version: VERSION })).toEqual({ ok: false, error: "Not recorded: version is not the current approved content" });
  });

  it("sends nothing when the server has no coordinator credential", async () => {
    vi.stubEnv("WASL_API_COORDINATOR_TOKEN", "");
    const result = await releaseOutcomeHandoff({ caseId: "c1", version: VERSION });
    expect(result.ok === false && result.error).toContain("no coordinator credential");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("says when the same gate is already recorded", async () => {
    fetchMock.mockResolvedValue(json(200, { event: null, replayed: true }));
    const result = await releaseOutcomeHandoff({ caseId: "c1", version: VERSION });
    expect(result.ok && result.message).toContain("already recorded");
  });

  it("needs an approved version", async () => {
    expect((await releaseOutcomeHandoff({ caseId: "c1", version: "" })).ok).toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

const base = {
  referrer_status: "pending",
  responsible_referrer: "referrer/ahs",
  consent: "granted",
  delivery: "simulated",
  draft: { decision: DECISION, programme: "crc", outcome: "closed_negative", safety_net: "safety_net_v1" },
  next_steps: ["routine_screening", "surveillance", "specialist_follow_up", "discuss_with_doctor"],
  history: [],
};
const approved = {
  version: VERSION,
  approved_at: "2026-10-01T08:00:00Z",
  approved_by: "dr-gp",
  patient_informed: false,
  content: { outcome: "follow_up_complete", next_step: "surveillance", safety_net: "safety_net_v1", decision: DECISION, programme: "crc" },
  history: [{ kind: "content_approved", version: VERSION, role: "clinician", actor: "dr-gp", at: "2026-10-01T08:00:00Z", event: "e1" }],
};
const signedIn: HandoffSession = { live: true, signedIn: true, mode: "demo" };
const signedOut: HandoffSession = { live: true, signedIn: false, mode: "demo" };

function render(raw: object, session: HandoffSession) {
  const handoff = OutcomeHandoff.parse(raw);
  return renderToStaticMarkup(createElement(OutcomeHandoffCard, { handoff, actions: outcomeHandoffActions("c1", handoff, session) }));
}

describe("outcome hand-off card states", () => {
  it("offers no staff step for an ineligible case", () => {
    const out = render({ ...base, status: "not_eligible", draft: undefined }, signedIn);
    expect(out).toContain("No staff step");
    expect(out).not.toContain("outcome-handoff-approve");
    expect(out).not.toContain("outcome-handoff-release");
  });

  it("says fixtures cannot record approval or release", () => {
    const out = render({ ...base, status: "pending_approval" }, { live: false, signedIn: false });
    expect(out).toContain("need the live API");
    expect(out).not.toContain("<form");
  });

  it("asks a signed-out viewer to sign in before approving, with no credential picker", () => {
    const out = render({ ...base, status: "pending_approval" }, signedOut);
    expect(out).toContain("governance-signin");
    expect(out).toContain("Sign in to approve the outcome hand-off as a clinician");
    expect(out).not.toContain("outcome-handoff-approve");
    expect(out).toContain("available once a clinician has approved");
    expect(out).not.toMatch(/name="(role|actor|credential)"/);
  });

  it("offers the signed-in viewer the closed next-step list with no default", () => {
    const out = render({ ...base, status: "pending_approval" }, signedIn);
    expect(out).toContain("outcome-handoff-approve");
    expect(out).toContain('<option value="" selected="">Choose a next step</option>');
    for (const step of base.next_steps) expect(out).toContain(`value="${step}"`);
    expect(out).not.toContain("outcome-handoff-release");
  });

  it("offers the release once a version is approved", () => {
    const out = render({ ...base, ...approved, status: "pending_release" }, signedOut);
    expect(out).toContain("outcome-handoff-release");
    expect(out).toContain("Record simulated release");
    expect(out).toContain("approved by dr-gp");
    expect(out).toContain("version aaaaaaaa");
  });

  it("shows a held release and offers a re-check", () => {
    const out = render(
      { ...base, ...approved, consent: "unknown", status: "held_consent", release: { gate: "held_consent", at: "2026-10-02T09:00:00Z", actor: "coordinator-one", event: "e2" } },
      signedIn,
    );
    expect(out).toContain("held: consent unknown · coordinator-one");
    expect(out).toContain("Record release again (re-check consent)");
  });

  it("shows the referrer acknowledgement read-only, with no control for it", () => {
    const out = render({ ...base, ...approved, status: "released", referrer_status: "acknowledged" }, signedIn);
    expect(out).toContain("Acknowledged (does not mark the patient informed)");
    const buttons = [...out.matchAll(/<button[^>]*>([^<]*)<\/button>/g)].map((m) => m[1]);
    expect(buttons).toEqual(["Sign out", "Record approval", "Record simulated release"]);
    expect(out).toContain("Approval acts as the token you signed in with");
  });
});
