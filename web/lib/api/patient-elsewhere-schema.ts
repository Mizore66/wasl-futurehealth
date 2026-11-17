import { z } from "zod";

/*
 * Contract for /api/patient/completed-elsewhere (pipeline/api_patient_elsewhere.go, issue #177). A patient says the
 * procedure was already done at another provider. It is a report to check, never a result: the server opens an owned
 * record-retrieval task and suspends chase and booking; only a retrieved report that passes the identity-link,
 * two-reader and human-review gates can close the case.
 */

export const PATIENT_ELSEWHERE_OUTCOMES = ["not_found", "not_matching", "non_closing"] as const;

export const PatientElsewhereReport = z.looseObject({
  receipt: z.string(),
  submitted_at: z.string(),
  status: z.enum(["pending", "resumed"]),
  reported_by: z.enum(["patient", "care_team"]),
  procedure: z.string(),
  performed: z.string(),
  provider: z.string(),
  source: z.string(),
  report_ref: z.string().optional(),
  retrieval_due: z.string(),
  retrieval_status: z.enum(["due", "overdue", "ended", "evidence_closed"]),
  outcome: z.enum(PATIENT_ELSEWHERE_OUTCOMES).optional(),
  ended_at: z.string().optional(),
});

export const PatientElsewhereView = z.looseObject({ report: PatientElsewhereReport.nullable(), can_report: z.boolean() });

export const PatientElsewhereReceipt = z.looseObject({ receipt: z.string(), replayed: z.boolean(), completed_elsewhere: PatientElsewhereView });

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

/** What the patient may send: the server sets the source, the retrieval owner and the due date. */
export const PatientElsewhereSubmission = z.strictObject({
  request_id: z.string().regex(UUID),
  procedure: z.string().min(1).max(64),
  performed: z.string().regex(/^\d{4}-\d{2}(-\d{2})?$/),
  provider: z.string().min(1).max(120),
  report_ref: z.string().min(1).max(64).optional(),
});

export type PatientElsewhereReport = z.infer<typeof PatientElsewhereReport>;
export type PatientElsewhereView = z.infer<typeof PatientElsewhereView>;
export type PatientElsewhereSubmission = z.infer<typeof PatientElsewhereSubmission>;
