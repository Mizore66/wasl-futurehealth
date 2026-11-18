import { afterAll, beforeAll, describe, expect, it, vi } from "vitest";

// Due times carry a relative hint ("in 2 days"), so the wall clock is pinned (Dubai 16:00, Sat 3 Oct 2026).
beforeAll(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-10-03T12:00:00Z"));
});
afterAll(() => {
  vi.useRealTimers();
});
import { LANGS } from "@/components/patient/i18n";
import { ALL_DICTS as DICTS } from "@/components/patient/i18n/all";
import { callbackStatus } from "@/components/case/teach-back-callback-card";
import { workItem } from "@/components/worklist/model";
import { PatientJourney, PatientMutation } from "@/lib/api/patient-schema";
import { CaseList, CaseDetail, type TeachBackCallback } from "@/lib/api/schema";
import { patientFixture } from "./patient-fixture";
import { loadSample } from "./samples";

const open: TeachBackCallback = {
  id: "a1b2c3d4e5f60718",
  source: "patient_link",
  state: "misunderstood",
  owner: "coordinator",
  status: "open",
  escalation: "teachback_misunderstood_callback",
  opened_at: "2026-10-01T08:00:00Z",
  due_at: "2026-10-05T08:00:00Z",
  overdue: false,
};

describe("failed teach-back callback", () => {
  it("parses the patient receipt and refuses a malformed answer id; the API owns which answers are offered", () => {
    const journey = patientFixture().journey;
    const receipt = { id: open.id, source: "patient_link", state: "misunderstood", owner: "coordinator", status: "open", opened_at: open.opened_at, due_at: open.due_at };
    const failed = PatientJourney.parse({
      ...journey,
      teach_back: { ...journey.teach_back, status: "callback", understanding: "misunderstood", attempts: 1, callback: receipt },
    });
    expect(failed.teach_back.callback?.due_at).toBe("2026-10-05T08:00:00Z");
    const base = { kind: "teach_back_answered", expected_head: "h", request_id: crypto.randomUUID() } as const;
    expect(PatientMutation.safeParse({ ...base, data: { question_id: "crc-next-step-v1", answer_id: "blood_test" } }).success).toBe(true);
    expect(PatientMutation.safeParse({ ...base, data: { question_id: "crc-next-step-v1", answer_id: "../colonoscopy" } }).success).toBe(false);
    expect(PatientMutation.safeParse({ ...base, data: { question_id: "crc-next-step-v1", answer_id: "blood_test" }, case: "LOOP-1" }).success).toBe(false);
  });

  it("has receipt words in every patient language", () => {
    for (const lang of LANGS) {
      const c = DICTS[lang].check;
      expect(c.callbackOpen).toContain("{date}");
      expect(c.letters).toHaveLength(3);
      for (const words of [c.notSure, c.callbackLate, c.callbackResolved, c.callbackRef]) expect(words.length).toBeGreaterThan(0);
    }
  });

  it("shows open, overdue and resolved staff states", () => {
    expect(callbackStatus(open)).toBe("Open · coordinator callback due Mon 5 Oct, 12:00 Dubai time (in 2 days)");
    expect(callbackStatus({ ...open, overdue: true })).toMatch(/^Overdue/);
    expect(callbackStatus({ ...open, status: "resolved", resolved_by: "coordinator-one", resolved_at: "2026-10-02T09:00:00Z" })).toBe(
      "Resolved by coordinator-one 2026-10-02 09:00 UTC",
    );
  });

  it("keeps an open callback on the To call tab until it is resolved, whatever contact was logged", () => {
    const row = CaseList.parse(loadSample("cases_crc_page1.json")).cases.find((c) => !c.closed)!;
    const withTask = { ...row, teach_back_callback: open };
    const item = workItem(withTask, 120, false);
    expect([item.view, item.status]).toEqual(["call", "Teach-back failed · nurse callback due Mon 5 Oct, 12:00 Dubai time (in 2 days)"]);
    expect(workItem({ ...withTask, teach_back_callback: { ...open, overdue: true } }, 120, false).status).toBe(
      "Teach-back failed · nurse callback overdue since Mon 5 Oct, 12:00 Dubai time (in 2 days)",
    );
    expect(workItem({ ...withTask, teach_back_callback: { ...open, status: "resolved" } }, 120, false).status).toBe(workItem(row, 120, false).status);
  });

  it("flags the row overdue while an open callback is late, whatever the programme deadline says", () => {
    const row = CaseList.parse(loadSample("cases_crc_page1.json")).cases.find((c) => !c.closed && !c.overdue)!;
    expect(workItem({ ...row, teach_back_callback: open }, 120, false).overdue).toBe(false);
    expect(workItem({ ...row, teach_back_callback: { ...open, overdue: true } }, 120, false).overdue).toBe(true);
    expect(workItem({ ...row, teach_back_callback: { ...open, overdue: true, status: "resolved" } }, 120, false).overdue).toBe(false);
  });

  it("parses consultation outcomes on case detail", () => {
    const detail = CaseDetail.parse({
      ...(loadSample("case_evidence_closure.json") as object),
      teach_back_callbacks: [{ ...open, consultations: [{ outcome: "another_explanation", actor: "coordinator-one", role: "coordinator", at: "2026-10-02T09:00:00Z", event: "e1" }] }],
    });
    expect(detail.teach_back_callbacks?.[0].consultations?.[0].outcome).toBe("another_explanation");
  });
});
