import { afterEach, describe, expect, it, vi } from "vitest";
import { LANGS } from "@/components/patient/i18n";
import { ALL_DICTS as DICTS } from "@/components/patient/i18n/all";
import { requestStatusLine } from "@/components/patient/rights-requests";
import { forwardPatientRights } from "@/lib/api/patient-source";
import { PatientRightsList, PatientRightsReceipt, PatientRightsSubmission } from "@/lib/api/patient-schema";

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

const contest = {
  receipt: "a1b2c3d4e5f6", kind: "contest_submitted", decision: "sig", submitted_at: "2026-10-01T08:00:00Z", reason: "Not my record",
  status: "open", owner: "senior_clinician", due_at: "2026-10-08T08:00:00Z", overdue: false, closure_held: true,
} as const;

describe("patient rights requests", () => {
  it("parses the Go request list and an explanation receipt with the case provenance", () => {
    expect(PatientRightsList.parse({ meta: {}, requests: [contest], current_decision: "sig" }).requests[0].status).toBe("open");
    const receipt = PatientRightsReceipt.parse({
      request: { ...contest, kind: "explanation_requested", status: "answered" },
      replayed: false,
      explanation: { decision: "sig", state: "closed_normal", closed: true, closed_day: 41, closing_report: "ahs/DiagnosticReport/COLO-1", matched_records: [{ provider: "ahs", role: "referral" }], human_assisted: false, closure_held: false },
    });
    expect(receipt.explanation?.closing_report).toBe("ahs/DiagnosticReport/COLO-1");
  });

  it("refuses a client-supplied case, role or actor in a submission", () => {
    const base = { kind: "contest_submitted", request_id: crypto.randomUUID(), otp: "123456", decision: "sig", reason: "x" };
    expect(PatientRightsSubmission.safeParse(base).success).toBe(true);
    for (const extra of [{ case: "LOOP-1" }, { role: "senior_clinician" }, { actor: "dr" }]) {
      expect(PatientRightsSubmission.safeParse({ ...base, ...extra }).success).toBe(false);
    }
  });

  it("shows pending, overdue and resolved states in every supported language", () => {
    for (const lang of LANGS) {
      const t = DICTS[lang].rights;
      const pending = requestStatusLine(contest, t, lang);
      expect(pending).not.toContain("{due}");
      expect(pending).not.toBe(requestStatusLine({ ...contest, overdue: true }, t, lang));
      expect(requestStatusLine({ ...contest, status: "resolved", outcome: "decision_upheld" }, t, lang)).toBe(t.upheld);
      expect(requestStatusLine({ ...contest, status: "resolved", outcome: "decision_revised" }, t, lang)).toBe(t.revised);
      expect(t.codeShown).toContain("{code}");
    }
    expect(requestStatusLine(contest, DICTS.en.rights, "en")).toBe("A person on the care team will review this by Thursday, 8 October.");
  });

  it("forwards only the link credential and raw body to the Go patient endpoints", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: "a valid one-time code for this link is required" }), { status: 403 }));
    vi.stubGlobal("fetch", fetchMock);
    const raw = JSON.stringify({ kind: "explanation_requested", request_id: crypto.randomUUID(), otp: "", decision: "sig", reason: "" });
    const result = await forwardPatientRights("opaque-patient-token", "/api/patient/requests", raw);
    expect(result.status).toBe(403);
    expect(fetchMock.mock.calls[0][0]).toBe("http://127.0.0.1:8088/api/patient/requests");
    const headers = new Headers(fetchMock.mock.calls[0][1].headers);
    expect(headers.get("authorization")).toBe("Bearer opaque-patient-token");
    expect(headers.get("x-wasl-role")).toBeNull();
    expect(fetchMock.mock.calls[0][1].body).toBe(raw);
  });
});
