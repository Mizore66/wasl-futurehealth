import { describe, expect, it } from "vitest";
import { workItem } from "@/components/worklist/model";
import { openPatientTasks } from "@/components/worklist/patient-tasks";
import { CaseList, type PatientTask, type TeachBackCallback } from "@/lib/api/schema";
import { loadSample } from "./samples";

const teachBack: TeachBackCallback = {
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
const callback: PatientTask = { ...teachBack, kind: "teach_back_callback" };
const contest: PatientTask = {
  kind: "patient_contest",
  id: "f0e1d2c3b4a59687",
  owner: "senior_clinician",
  status: "open",
  escalation: "patient_contest_review",
  opened_at: "2026-10-01T08:00:00Z",
  due_at: "2026-10-08T08:00:00Z",
  overdue: false,
  closure_held: true,
};

function openRow() {
  return CaseList.parse(loadSample("cases_crc_page1.json")).cases.find((c) => !c.closed && !c.overdue)!;
}

describe("shared patient-request staff tasks", () => {
  it("parses the summary task list the API serves", () => {
    const row = CaseList.parse({ ...(loadSample("cases_crc_page1.json") as object) }).cases[0];
    const parsed = CaseList.shape.cases.element.parse({ ...row, patient_tasks: [callback, contest] });
    expect(parsed.patient_tasks?.map((t) => t.kind)).toEqual(["teach_back_callback", "patient_contest"]);
    expect(() => CaseList.shape.cases.element.parse({ ...row, patient_tasks: [{ ...contest, kind: "staff_contest" }] })).toThrow();
  });

  it("shows an open patient contest as independent review in Waiting, with due and overdue words", () => {
    const row = openRow();
    const item = workItem({ ...row, patient_tasks: [contest] }, 120, false);
    expect([item.view, item.status, item.overdue]).toEqual(["waiting", "Patient contest · independent review due 2026-10-08 08:00 UTC", false]);
    const late = workItem({ ...row, patient_tasks: [{ ...contest, overdue: true }] }, 120, false);
    expect([late.status, late.overdue]).toEqual(["Patient contest · independent review overdue since 2026-10-08 08:00 UTC", true]);
  });

  it("keeps a decision_revised hold as owned, due-dated revision work in Waiting", () => {
    const row = openRow();
    const resolved = { ...contest, status: "resolved" as const, resolved_at: "2026-10-02T09:00:00Z", resolved_by: "dr-independent", outcome: "decision_revised" as const };
    const revision: PatientTask = {
      kind: "patient_contest_revision", id: "9a8b7c6d5e4f3021", contest: contest.id, owner: "senior_clinician", status: "open",
      escalation: "patient_contest_revision", opened_at: "2026-10-02T09:00:00Z", due_at: "2026-10-09T09:00:00Z", overdue: false, closure_held: true,
    };
    const parsed = CaseList.shape.cases.element.parse({ ...row, patient_tasks: [revision, resolved] });
    const item = workItem(parsed, 120, false);
    expect([item.view, item.status]).toEqual(["waiting", "Patient contest · decision to be revised due 2026-10-09 09:00 UTC"]);
    expect(workItem({ ...parsed, patient_tasks: [{ ...revision, overdue: true }, resolved] }, 120, false).overdue).toBe(true);
  });

  it("restores the ordinary row once the contest is resolved", () => {
    const row = openRow();
    const resolved = { ...contest, status: "resolved" as const, resolved_at: "2026-10-02T09:00:00Z", resolved_by: "dr-independent", outcome: "decision_upheld" as const };
    const item = workItem({ ...row, patient_tasks: [resolved] }, 120, false);
    expect([item.status, item.overdue]).toEqual([workItem(row, 120, false).status, false]);
    expect(openPatientTasks({ ...row, patient_tasks: [resolved] })).toEqual([]);
  });

  it("keeps the coordinator's own failed teach-back call ahead of a contest it cannot resolve", () => {
    const row = openRow();
    const item = workItem({ ...row, teach_back_callback: teachBack, patient_tasks: [callback, contest] }, 120, false);
    expect([item.view, item.status]).toEqual(["call", "Teach-back failed · nurse callback due 2026-10-05 08:00 UTC"]);
    expect(workItem({ ...row, teach_back_callback: teachBack, patient_tasks: [callback, { ...contest, overdue: true }] }, 120, false).overdue).toBe(true);
  });
});
