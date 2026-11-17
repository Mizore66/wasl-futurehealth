import { describe, expect, it } from "vitest";
import { ELSEWHERE_COPY } from "@/components/patient/elsewhere-copy";
import { elsewhereStatusLine } from "@/components/patient/elsewhere-report";
import { PatientElsewhereReceipt, PatientElsewhereSubmission, PatientElsewhereView } from "@/lib/api/patient-elsewhere-schema";

// Issue #177: the patient's completed-elsewhere contract (pipeline/api_patient_elsewhere.go) and its status words.
const report = {
  receipt: "a".repeat(64), submitted_at: "2026-10-01T08:00:00Z", status: "pending", reported_by: "patient", source: "patient",
  procedure: "colonoscopy", performed: "2026-08", provider: "Synthetic Private Hospital", report_ref: "SYN-RPT-9",
  retrieval_due: "2026-10-15", retrieval_status: "due",
} as const;

describe("patient completed-elsewhere contract", () => {
  it("parses the served view and receipt", () => {
    expect(PatientElsewhereView.parse({ report: null, can_report: true })).toEqual({ report: null, can_report: true });
    const receipt = PatientElsewhereReceipt.parse({ receipt: report.receipt, replayed: false, completed_elsewhere: { report, can_report: false } });
    expect(receipt.completed_elsewhere.report?.retrieval_status).toBe("due");
  });

  it("lets the patient send only the report, never the source, owner, due date, case or a closure", () => {
    const ok = { request_id: "00000000-0000-4000-8000-000000000001", procedure: "colonoscopy", performed: "2026-08", provider: "Synthetic Private Hospital" };
    expect(PatientElsewhereSubmission.parse(ok)).toEqual(ok);
    for (const extra of [{ source: "patient" }, { retrieval_owner: "x" }, { retrieval_due: "2026-10-15" }, { case: "ahs/ServiceRequest/X" }, { closed: true }]) {
      expect(PatientElsewhereSubmission.safeParse({ ...ok, ...extra }).success).toBe(false);
    }
    expect(PatientElsewhereSubmission.safeParse({ ...ok, performed: "last summer" }).success).toBe(false);
    expect(PatientElsewhereSubmission.safeParse({ ...ok, request_id: "nope" }).success).toBe(false);
  });
});

describe("patient status line", () => {
  const t = ELSEWHERE_COPY.en;
  it("pending, overdue, resumed and evidence-closed read from the server status, never as a result", () => {
    expect(elsewhereStatusLine({ ...report }, t, "en")).toBe("We are asking Synthetic Private Hospital for the report. Your care team aims to have it by 15 October.");
    expect(elsewhereStatusLine({ ...report, retrieval_status: "overdue" }, t, "en")).toContain("still waiting");
    expect(elsewhereStatusLine({ ...report, status: "resumed", retrieval_status: "ended", outcome: "not_found" }, t, "en")).toBe(
      "Synthetic Private Hospital could not find the report, so your care team will contact you about the next step.",
    );
    expect(elsewhereStatusLine({ ...report, status: "resumed", retrieval_status: "ended", outcome: "non_closing" }, t, "en")).toContain("not enough");
    expect(elsewhereStatusLine({ ...report, retrieval_status: "evidence_closed" }, t, "en")).toBe(t.evidenceClosed);
  });

  it("every language has every string", () => {
    const keys = Object.keys(ELSEWHERE_COPY.en).sort();
    for (const copy of Object.values(ELSEWHERE_COPY)) {
      expect(Object.keys(copy).sort()).toEqual(keys);
      expect(Object.keys(copy.resumed).sort()).toEqual(["non_closing", "not_found", "not_matching"]);
      expect(copy.pending).toContain("{provider}");
      expect(copy.pending).toContain("{due}");
    }
  });
});

describe("coordinator retrieval words", () => {
  it("use the server's retrieval_status over the caller's date", async () => {
    const { reconciliationWords } = await import("@/components/case/words");
    const r = { procedure: "colonoscopy", provider: "Synthetic Private Hospital", retrieval_owner: "coordinator", retrieval_due: "2026-10-15" };
    expect(reconciliationWords({ ...r, retrieval_status: "overdue" }, "2026-10-01").overdue).toBe(true);
    expect(reconciliationWords({ ...r, retrieval_status: "due" }, "2099-01-01").overdue).toBe(false);
    expect(reconciliationWords(r, "2026-10-16").overdue).toBe(true);
  });
});
