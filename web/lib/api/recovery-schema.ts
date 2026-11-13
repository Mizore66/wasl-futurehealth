import { z } from "zod";

export const RecoveryCheck = z.looseObject({
  code: z.string(),
  kind: z.enum(["hard", "soft"]),
  result: z.enum(["pass", "fail", "warning"]),
  reason_code: z.string(),
});

export const RecoveryOutreach = z.looseObject({
  allowed: z.boolean(),
  consent: z.string(),
  second_identifier: z.string(),
  reason_codes: z.array(z.string()),
});

export const RecoveryCandidate = z.looseObject({
  case_id: z.string(),
  case_head: z.string(),
  eligible: z.boolean(),
  rank: z.number().nullable(),
  priority: z.string(),
  requested_day: z.number(),
  deadline_day: z.number(),
  constraints: z.record(z.string(), z.unknown()),
  checks: z.array(RecoveryCheck),
  ordering: z.looseObject({
    priority_rank: z.number(),
    deadline_day: z.number(),
    requested_day: z.number(),
    tie_break: z.string(),
  }),
  outreach: RecoveryOutreach,
  eligibility_hash: z.string(),
});

export const RecoverySlotSummary = z.looseObject({
  slot_id: z.string(),
  head: z.string(),
  status: z.enum(["open", "filled", "stale"]),
  programme: z.enum(["crc", "cervical"]),
  procedure: z.string(),
  provider: z.string(),
  unit: z.string(),
  region: z.string(),
  day: z.number(),
  source_case: z.string(),
  filled_case: z.string(),
  stale_reason_codes: z.array(z.string()),
});

export const RecoverySlot = RecoverySlotSummary.extend({
  session_id: z.string(),
  base_head: z.string(),
  capabilities: z.looseObject({ female: z.boolean(), arabic: z.boolean(), after_hours: z.boolean(), ramadan: z.boolean() }),
  release: z.looseObject({
    source_appointment: z.string(),
    source_decision_sig: z.string(),
    projection_basis: z.literal("no_show_plus_future_synthetic_capacity_window"),
    label: z.string(),
  }),
  candidates: z.array(RecoveryCandidate),
  history: z.array(z.looseObject({ hash: z.string(), at: z.string() })),
  backfill: z.looseObject({ case_id: z.string(), event: z.string(), status: z.string() }).nullable(),
  outreach: z.looseObject({ status: z.string(), dispatched: z.literal(false) }),
});

export const RecoveryList = z.looseObject({
  schema: z.literal("wasl-recovery/1"),
  status: z.enum(["ready", "not_generated", "invalid"]),
  data_mode: z.literal("simulated"),
  decision_support_only: z.literal(true),
  canonical_unchanged: z.literal(true),
  as_of_day: z.number(),
  ranking_policy: z.array(z.string()),
  slots: z.array(RecoverySlotSummary),
  label: z.string().optional(),
  reason: z.string().optional(),
});

export const RecoveryDetail = z.looseObject({
  schema: z.literal("wasl-recovery/1"),
  status: z.literal("ready"),
  data_mode: z.literal("simulated"),
  canonical_unchanged: z.literal(true),
  slot: RecoverySlot,
});

export const RecoveryConfirmRequest = z.object({
  case_id: z.string().min(1),
  expected_slot_head: z.string().min(1).max(128),
  expected_case_head: z.string().min(1).max(128),
  request_id: z.string().min(16).max(80),
  outreach_intent: z.boolean(),
  note: z.string().max(500).default(""),
});

export const RecoveryEvent = z.looseObject({ hash: z.string(), at: z.string(), type: z.literal("waitlist_recovery") });
export const RecoveryConfirmResponse = z.looseObject({
  event: RecoveryEvent,
  case_head: z.string(),
  canonical_unchanged: z.literal(true),
  dispatched: z.literal(false),
  duplicate: z.boolean(),
  message: z.string().optional(),
});
export const RecoveryError = z.looseObject({ error: z.string(), conflict: z.string().optional() });

export type RecoveryList = z.infer<typeof RecoveryList>;
export type RecoveryDetail = z.infer<typeof RecoveryDetail>;
export type RecoverySlot = z.infer<typeof RecoverySlot>;
export type RecoveryCandidate = z.infer<typeof RecoveryCandidate>;
export type RecoveryConfirmRequest = z.infer<typeof RecoveryConfirmRequest>;
export type RecoveryConfirmResponse = z.infer<typeof RecoveryConfirmResponse>;
