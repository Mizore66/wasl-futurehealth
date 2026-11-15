import { z } from "zod";

export const PatientTeachBack = z.strictObject({
  question_id: z.literal("crc-next-step-v1"),
  status: z.enum(["pending", "completed"]),
  completed_at: z.string(),
  event: z.string(),
});
export const PatientContactPermission = z.strictObject({
  status: z.enum(["unknown", "granted", "denied", "opted_out"]),
  updated_at: z.string(),
  event: z.string(),
});
export const PatientSlotSelection = z.strictObject({
  status: z.enum(["unselected", "selected"]),
  offers_version: z.string(),
  slot_id: z.string(),
  selected_at: z.string(),
  event: z.string(),
});
export const PatientPreparationItem = z.strictObject({
  item_id: z.enum(["clear_fluids_and_prep", "escort_home"]),
  completed: z.boolean(),
  updated_at: z.string(),
  event: z.string(),
});
export const PatientPreparation = z.strictObject({
  items: z.array(PatientPreparationItem),
  completed: z.number().int().min(0).max(2),
  total: z.literal(2),
});
export const PatientJourney = z.strictObject({
  schema: z.literal("wasl-patient-journey/1"),
  head: z.string().min(1),
  seq: z.number().int().nonnegative(),
  ledger_status: z.enum(["ok", "invalid"]),
  teach_back: PatientTeachBack,
  contact_permission: PatientContactPermission,
  slot_selection: PatientSlotSelection,
  preparation: PatientPreparation,
  last_action_at: z.string(),
});
export const PatientOffer = z.strictObject({ slot_id: z.string(), unit: z.string(), starts_at: z.string() });
export const PatientOffers = z.strictObject({ version: z.string(), slots: z.array(PatientOffer) });
export const PatientJourneyResponse = z.looseObject({
  meta: z.looseObject({ api: z.string(), data_mode: z.string() }),
  journey: PatientJourney,
  offers: PatientOffers,
  expires_at: z.string(),
});
export const PatientPublicEvent = z.looseObject({
  hash: z.string(),
  kind: z.enum(["teach_back_completed", "contact_permission_changed", "slot_selected", "preparation_changed"]),
  at: z.string(),
  seq: z.number().int().positive(),
  data: z.record(z.string(), z.unknown()),
});
export const PatientMutationResponse = z.looseObject({
  event: PatientPublicEvent.nullable(),
  journey: PatientJourney,
  replayed: z.boolean(),
});
export const PatientError = z.looseObject({
  error: z.string(),
  conflict: z.enum(["head", "request", "offers", "consent"]).optional(),
  current_head: z.string().optional(),
  journey: PatientJourney.optional(),
});

const mutationBase = { expected_head: z.string().min(1).max(128), request_id: z.uuid() };
export const PatientMutation = z.discriminatedUnion("kind", [
  z.strictObject({
    ...mutationBase,
    kind: z.literal("teach_back_completed"),
    data: z.strictObject({ question_id: z.literal("crc-next-step-v1"), answer_id: z.literal("colonoscopy") }),
  }),
  z.strictObject({
    ...mutationBase,
    kind: z.literal("contact_permission_changed"),
    data: z.strictObject({ permission: z.enum(["granted", "denied", "opted_out"]) }),
  }),
  z.strictObject({
    ...mutationBase,
    kind: z.literal("slot_selected"),
    data: z.strictObject({ slot_id: z.string().min(1), offers_version: z.string().min(1) }),
  }),
  z.strictObject({
    ...mutationBase,
    kind: z.literal("preparation_changed"),
    data: z.strictObject({ item_id: z.enum(["clear_fluids_and_prep", "escort_home"]), completed: z.boolean() }),
  }),
]);

export type PatientJourney = z.infer<typeof PatientJourney>;
export type PatientOffers = z.infer<typeof PatientOffers>;
export type PatientOffer = z.infer<typeof PatientOffer>;
export type PatientJourneyResponse = z.infer<typeof PatientJourneyResponse>;
export type PatientMutationResponse = z.infer<typeof PatientMutationResponse>;
export type PatientError = z.infer<typeof PatientError>;
export type PatientMutation = z.input<typeof PatientMutation>;
export type PatientMutationKind = PatientMutation["kind"];
export type PatientMutationData<K extends PatientMutationKind> = Extract<PatientMutation, { kind: K }>["data"];

/** One explanation request or contest as the patient sees it (pipeline/api_patient_request.go patientRequestRowLocked). */
export const PatientRightsRequest = z.looseObject({
  receipt: z.string().min(1),
  kind: z.enum(["explanation_requested", "contest_submitted"]),
  decision: z.string(),
  submitted_at: z.string(),
  reason: z.string(),
  status: z.enum(["answered", "open", "resolved"]),
  owner: z.string().optional(),
  due_at: z.string().optional(),
  overdue: z.boolean().optional(),
  outcome: z.enum(["decision_upheld", "decision_revised"]).optional(),
  resolved_at: z.string().optional(),
  closure_held: z.boolean().optional(),
});
export const PatientRightsList = z.looseObject({ requests: z.array(PatientRightsRequest), current_decision: z.string() });
/** The case's own provenance, returned only after a verified explanation request. */
export const PatientExplanation = z.looseObject({
  decision: z.string(),
  state: z.string(),
  closed: z.boolean(),
  closing_report: z.string(),
  matched_records: z.array(z.strictObject({ provider: z.string(), role: z.string() })),
  closure_held: z.boolean(),
});
export const PatientRightsReceipt = z.looseObject({
  request: PatientRightsRequest,
  replayed: z.boolean(),
  explanation: PatientExplanation.optional(),
});
export const PatientVerification = z.looseObject({ delivery: z.literal("simulated"), code: z.string().regex(/^\d{6}$/), expires_at: z.string() });
export const PatientRightsSubmission = z.strictObject({
  kind: z.enum(["explanation_requested", "contest_submitted"]),
  request_id: z.uuid(),
  otp: z.string().max(6),
  decision: z.string().min(1).max(128),
  reason: z.string().max(500),
});

export type PatientRightsRequest = z.infer<typeof PatientRightsRequest>;
export type PatientRightsList = z.infer<typeof PatientRightsList>;
export type PatientRightsReceipt = z.infer<typeof PatientRightsReceipt>;
export type PatientExplanation = z.infer<typeof PatientExplanation>;
export type PatientRightsSubmission = z.infer<typeof PatientRightsSubmission>;
