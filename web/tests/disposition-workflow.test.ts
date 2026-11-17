import { readFileSync } from "node:fs";
import path from "node:path";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { revalidatePath } from "next/cache";
import { recordDisposition } from "@/app/app/patients/[id]/disposition/actions";
import { DispositionCard } from "@/components/disposition/disposition-card";
import { DispositionInput, dispositionContext } from "@/components/disposition/model";
import { reviewerToken } from "@/lib/api/reviewer-session";
import { CaseDetail } from "@/lib/api/schema";

vi.mock("server-only", () => ({}));
vi.mock("next/cache", () => ({ revalidatePath: vi.fn(), refresh: vi.fn() }));
vi.mock("@/lib/api/reviewer-session", () => ({ reviewerToken: vi.fn(), signInReviewer: vi.fn() }));

/* Issues #180 and #115: one clinician disposition workflow, typed by kind, reachable without a raised review item. */
const raw = JSON.parse(readFileSync(path.join(process.cwd(), "data", "api", "case_dr_exam_closure.json"), "utf8"));
const HEAD = "h".repeat(64);
const DECLINE = "d".repeat(64);

function detail(opts: { programme?: string; escalations?: string[]; overlay?: Record<string, unknown> } = {}) {
  const programme = opts.programme ?? "diabetic_retinopathy";
  const state = programme === "lung_nodule" ? "out_of_scope" : programme === "crc" ? "open" : "not_assessable";
  return CaseDetail.parse({
    ...raw,
    review_items: [],
    case: { ...raw.case, programme, state, dropoff: programme === "crc" ? "not_booked" : state, closed: false, escalations: opts.escalations ?? [] },
    api_overlay: { head: HEAD, actions: 1, owner: "", ledger_status: "ok", ...opts.overlay },
  });
}

const holdPlan = { action: "clinician_disposition", reason: "No fundal view twice", plan: "alternative_diagnostic", details: "Slit-lamp exam", owner: "Dr Synthetic", review_date: "2026-12-01" };
const decline = {
  action: "decline_disposition", reason: "procedure_anxiety", counselling: "Explained the risks", safety_net: "Return with bleeding",
  plan: "follow_up", follow_up_owner: "Dr Al Kaabi", next_review: "2027-02-20", fit_reoffer: "offered_declined",
};

describe("dispositionContext", () => {
  it("opens a hold plan for the DR no-view and lung outside-Fleischner holds with no review item raised", () => {
    const dr = dispositionContext(detail({ escalations: ["clinician_decision_no_fundal_view"] }));
    expect([dr.hold, dr.open, dr.expectedHead]).toEqual(["clinician_decision_no_fundal_view", ["clinician_disposition"], HEAD]);
    const lung = dispositionContext(detail({ programme: "lung_nodule", escalations: ["clinician_decision_outside_fleischner"] }));
    expect([lung.hold, lung.open]).toEqual(["clinician_decision_outside_fleischner", ["clinician_disposition"]]);
  });

  it("ignores a hold code on the wrong programme and a case with no hold", () => {
    expect(dispositionContext(detail({ programme: "crc", escalations: ["clinician_decision_no_fundal_view"] })).open).toEqual([]);
    expect(dispositionContext(detail()).open).toEqual([]);
  });

  it("opens an informed-refusal disposition only for the latest, unanswered decline", () => {
    const declined = { contact_outcomes: [{ at: "2026-11-10T09:00:00Z", event: DECLINE, outcome: "declined", channel: "phone" }] };
    expect(dispositionContext(detail({ programme: "crc", overlay: declined })).open).toEqual(["decline_disposition"]);
    const answered = { ...declined, decline_disposition: { at: "x", event: "e", role: "clinician", decline_event: DECLINE, plan: "closed_declined", reason: "other" } };
    const ctx = dispositionContext(detail({ programme: "crc", overlay: answered }));
    expect([ctx.open, ctx.decline?.answered]).toEqual([[], true]);
    const reversed = { contact_outcomes: [...declined.contact_outcomes, { at: "y", event: "f".repeat(64), outcome: "appointment_agreed", channel: "phone" }] };
    expect(dispositionContext(detail({ programme: "crc", overlay: reversed })).open).toEqual([]);
  });
});

describe("DispositionInput", () => {
  it("accepts complete dispositions of both kinds", () => {
    expect(DispositionInput.safeParse(holdPlan).success).toBe(true);
    expect(DispositionInput.safeParse(decline).success).toBe(true);
    expect(DispositionInput.safeParse({ ...decline, plan: "closed_declined", follow_up_owner: undefined, next_review: undefined }).success).toBe(true);
  });

  it("refuses missing fields, unsupported plans and client-chosen holds", () => {
    for (const bad of [
      { ...holdPlan, reason: "" }, { ...holdPlan, plan: "procedure_done" }, { ...holdPlan, owner: undefined }, { ...holdPlan, review_date: "next week" },
      { ...holdPlan, hold: "clinician_decision_no_fundal_view" },
      { ...decline, counselling: " " }, { ...decline, safety_net: undefined }, { ...decline, reason: "bored" },
      { ...decline, follow_up_owner: undefined }, { ...decline, plan: "closed_declined" }, { ...decline, plan: "procedure_done" },
    ]) expect(DispositionInput.safeParse(bad).success, JSON.stringify(bad)).toBe(false);
  });
});

describe("recordDisposition server action", () => {
  const fetchMock = vi.fn();
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    vi.stubEnv("WASL_API_CLINICIAN_TOKEN", "server-clinician-token-1234567890");
    vi.stubEnv("WASL_API_COORDINATOR_TOKEN", "server-coordinator-token-1234567890");
    vi.stubGlobal("fetch", fetchMock);
    vi.mocked(reviewerToken).mockResolvedValue("signed-in-clinician-token-0001");
  });
  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });
  const reply = (status: number, body: unknown) => fetchMock.mockResolvedValueOnce(new Response(JSON.stringify(body), { status }));

  it("posts the typed kind with the signed-in token and the read head, then revalidates the case and Today", async () => {
    reply(201, { meta: {}, event: { hash: "a".repeat(64), at: "2026-11-17T00:00:00Z", actor: "clinician-one" }, api_overlay: { head: "a".repeat(64) } });
    const res = await recordDisposition({ caseId: "case/1", expectedHead: HEAD, disposition: holdPlan });
    expect(res.ok).toBe(true);
    expect(res.ok && res.message).toMatch(/hold stays open/);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("http://127.0.0.1:8088/api/cases/case%2F1/actions");
    expect(new Headers(init.headers).get("authorization")).toBe("Bearer signed-in-clinician-token-0001");
    expect(JSON.parse(init.body)).toEqual({ ...holdPlan, expected_head: HEAD });
    expect(revalidatePath).toHaveBeenCalledWith("/app/patients/case%2F1");
    expect(revalidatePath).toHaveBeenCalledWith("/app/today");
  });

  it("never falls back to a server-held credential and never sends identity, hold or decline fields", async () => {
    vi.mocked(reviewerToken).mockResolvedValue(undefined);
    expect(await recordDisposition({ caseId: "c", expectedHead: HEAD, disposition: holdPlan })).toMatchObject({ ok: false, signIn: true });
    for (const field of ["role", "actor", "token", "hold", "decline_event"]) {
      expect(await recordDisposition({ caseId: "c", expectedHead: HEAD, disposition: { ...decline, [field]: "x" } })).toMatchObject({ ok: false });
    }
    expect(await recordDisposition({ caseId: "c", expectedHead: HEAD, disposition: { ...holdPlan, details: "" } })).toMatchObject({ ok: false });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("reports a role refusal and a stale head without claiming a save", async () => {
    reply(403, { meta: {}, error: "role coordinator may not record_clinician_disposition" });
    expect(await recordDisposition({ caseId: "c", expectedHead: HEAD, disposition: holdPlan })).toMatchObject({ ok: false, error: expect.stringMatching(/Only a clinician/) });
    reply(409, { meta: {}, error: "stale expected_head", conflict: "head", current_head: "z".repeat(64) });
    expect(await recordDisposition({ caseId: "c", expectedHead: HEAD, disposition: decline })).toMatchObject({ ok: false, stale: true });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("refuses in fixtures mode", async () => {
    vi.stubEnv("WASL_API_URL", "");
    expect(await recordDisposition({ caseId: "c", expectedHead: HEAD, disposition: holdPlan })).toMatchObject({ ok: false, error: expect.stringMatching(/live API/) });
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe("DispositionCard copy", () => {
  it("shows the clinician, rationale, plan, owner and review date as a plan, not completed care", () => {
    const html = renderToStaticMarkup(createElement(DispositionCard, {
      ctx: dispositionContext(detail({
        escalations: ["clinician_decision_no_fundal_view"],
        overlay: { clinician_disposition: { at: "t", event: "e".repeat(64), hold: "clinician_decision_no_fundal_view", ...holdPlan, actor: "clinician-one" } },
      })),
    }));
    for (const want of ["clinician-one", "No fundal view twice", "A different diagnostic test", "Slit-lamp exam", "Dr Synthetic", "2026-12-01", "not completed care"]) expect(html).toContain(want);
    expect(html).toContain("/disposition");
  });

  it("links the refusal to the decline and names the clinician role; the end state never says closed or done", () => {
    const overlay = {
      contact_outcomes: [{ at: "2026-11-10T09:00:00Z", event: DECLINE, outcome: "declined", channel: "phone" }],
      decline_disposition: { at: "t", event: "e".repeat(64), role: "clinician", actor: "dr-hamdan", decline_event: DECLINE, plan: "closed_declined", reason: "procedure_anxiety", counselling: "Explained the risks", safety_net: "Return with bleeding", fit_reoffer: "offered_declined" },
    };
    const html = renderToStaticMarkup(createElement(DispositionCard, { ctx: dispositionContext(detail({ programme: "crc", overlay })) }));
    for (const want of ["dr-hamdan (clinician)", `event ${DECLINE.slice(0, 12)}`, "Anxiety about the procedure", "Explained the risks", "Return with bleeding", "FIT re-offered and declined", "no procedure was done"]) {
      expect(html).toContain(want);
    }
    expect(html).not.toMatch(/GP to sign|loop closed|procedure done|care completed|\bclosed\b/i);
    expect(html).not.toContain("/disposition\"");
  });
});
