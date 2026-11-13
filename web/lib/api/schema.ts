import { z } from "zod";

/*
 * The backend contract (docs/API.md), parsed once at the boundary.
 * Objects are loose: unknown extra fields pass through so a newer backend does not break the UI.
 * Enums are closed: a new state or decision value fails the contract tests first.
 */

/** `lung_ldct` (lung cancer screening LDCT) and `ifhas` (IFHAS cardiometabolic results) arrived with the seven-programme run (docs/API.md). */
export const Programme = z.enum(["crc", "cervical", "lung_nodule", "diabetic_retinopathy", "breast", "lung_ldct", "ifhas"]);

export const CaseState = z.enum([
  "open",
  "booked",
  "procedure_done",
  "awaiting_histology",
  "inadequate",
  "no_show",
  "referral_rejected",
  "closed",
  "no_followup_needed",
  "escalated",
  "not_assessable",
  "out_of_scope",
  "no_referral_needed",
  "no_recall_needed",
  /** Lung PET-avid nodule waiting for tissue (pipeline/lung.go). In the served data, not yet listed in docs/API.md. */
  "awaiting_tissue",
]);

export const Dropoff = z.enum([
  "",
  "never_notified",
  "no_acknowledgement",
  "not_booked",
  "referral_rejected",
  "referral_unanswered",
  "no_show",
  "inadequate",
  "awaiting_histology",
  "in_progress",
  "no_followup_ct",
  "unlinked",
  "escalated",
  "not_assessable",
  "out_of_scope",
  "no_eye_exam",
  "ungradable_no_repeat",
  "technical_repeat_not_done",
  "no_assessment_outcome",
  /** See CaseState: served, not yet listed in docs/API.md. */
  "awaiting_tissue",
  /** Lung LDCT (docs/API.md, programme lung_ldct). */
  "incomplete_scan_not_completed",
  "followup_ldct_not_done",
  "no_workup_outcome",
  /** IFHAS (docs/API.md, programme ifhas). */
  "no_followup_documented",
]);

export const Consent = z.enum(["unknown", "granted", "denied", "opted_out"]);
export const ReviewStatus = z.enum(["none", "open", "signed"]);
export const FixtureName = z.enum(["evidence_closure", "false_closure_trap", "review_hold", "dr_exam_closure"]);
/** The three CRC fixtures every run has; `dr_exam_closure` exists only in runs with DR loops. */
export const CRC_FIXTURES = ["evidence_closure", "false_closure_trap", "review_hold"] as const;
export const Role = z.enum(["coordinator", "clinician", "reviewer", "auditor"]);

/** Small-cell suppression: counts 1 to 4 arrive as the string "<5". */
export const Count = z.union([z.number(), z.string().regex(/^<\d+$/)]);

export const Meta = z.looseObject({
  api: z.string(),
  data_mode: z.string(),
  run: z.string(),
  policy: z.string(),
  reader_model: z.string(),
  loaded_at: z.string(),
  note: z.string(),
});

const withMeta = <T extends z.ZodRawShape>(shape: T) => z.looseObject({ meta: Meta, ...shape });

export const LungFinding = z.looseObject({
  finding_report: z.string().optional(),
  finding_label: z.string().optional(),
  risk: z.string().optional(),
  /** False when no risk was recorded and pipeline/lung.go assumed "high" for the Fleischner rule. */
  risk_recorded: z.boolean().optional(),
  scope: z.string().optional(),
  window_start_day: z.number().optional(),
  window_end_day: z.number().optional(),
  outcome: z.enum(["stable", "resolved", "growing", "suspicious_solid", "not_assessable"]).optional(),
  rec: z
    .looseObject({
      action: z.enum(["none", "ct", "ct_or_pet_or_tissue"]),
      win_lo_days: z.number().optional(),
      win_hi_days: z.number().optional(),
      next_step: z.string().optional(),
      cell: z.string().optional(),
      optional: z.boolean().optional(),
    })
    .optional(),
});

const DR_OUTCOMES = ["examined_treated", "examined_followup", "examined_discharged", "not_examined", "dna", "not_assessable"] as const;

export const DrFinding = z.looseObject({
  screen_report: z.string().optional(),
  screen_label: z.string().optional(),
  coded_grade: z.boolean().optional(),
  rec: z
    .looseObject({
      action: z.enum(["refer", "rescreen", "repeat_photo"]),
      deadline_days: z.number().optional(),
      urgency: z.enum(["urgent", "routine"]).optional(),
      cell: z.string().optional(),
    })
    .optional(),
  grade_day: z.number().optional(),
  deadline_day: z.number().optional(),
  clinic_note: z.string().optional(),
  outcome: z.enum(DR_OUTCOMES).optional(),
  examined: z.boolean().optional(),
  within_deadline: z.boolean().optional(),
  next_due: z
    .looseObject({
      kind: z.enum(["rescreen", "clinic_review"]),
      basis: z.enum([
        "doh2024dr_annual_rescreen",
        "discharged_to_annual_screening",
        "note_review_interval",
        "assumed_followup_91d",
        "assumed_treated_28d",
      ]),
      due_day: z.number(),
    })
    .optional(),
});

const BREAST_OUTCOMES = ["return_routine", "early_recall", "to_treatment", "incomplete", "dna"] as const;

export const BreastFinding = z.looseObject({
  screen_report: z.string().optional(),
  screen_label: z.string().optional(),
  coded_birads: z.boolean().optional(),
  rec: z
    .looseObject({
      action: z.enum(["routine", "recall", "repeat_mammogram"]),
      deadline_days: z.number().optional(),
      urgency: z.string().optional(),
      cell: z.string().optional(),
    })
    .optional(),
  screen_day: z.number().optional(),
  deadline_day: z.number().optional(),
  assessment_document: z.string().optional(),
  outcome: z.enum(BREAST_OUTCOMES).optional(),
  within_deadline: z.boolean().optional(),
});

const LDCT_LABELS = ["LR0", "LR1", "LR2", "LR3", "LR4A", "LR4B", "LR4X"] as const;
const LDCT_WORKUP_LABELS = ["wk_treatment", "wk_benign", "wk_surveillance", "wk_incomplete", "wk_dna"] as const;
const IFHAS_DOMAINS = ["bp", "glyc", "lipid", "weight"] as const;
const IFHAS_SCREEN_LABEL = /^bp=(N|H|S);glyc=(N|P|D|S);lipid=(N|H);weight=(N|O)$/;
const IFHAS_FOLLOWUP_PART = /^(bp|glyc|lipid|weight)=(managed|not_confirmed|pending)$/;
const IfhasDomain = z.enum(IFHAS_DOMAINS);
const IfhasFollowupLabel = z
  .string()
  .regex(/^(bp|glyc|lipid|weight)=(managed|not_confirmed|pending)(;(bp|glyc|lipid|weight)=(managed|not_confirmed|pending)){0,3}$/)
  .refine((label) => {
    const domains = label.split(";").map((part) => IFHAS_FOLLOWUP_PART.exec(part)?.[1]);
    return domains.every(Boolean) && new Set(domains).size === domains.length;
  }, "IFHAS follow-up labels cannot repeat a domain");

export const LdctFinding = z.looseObject({
  scan_report: z.string().optional(),
  scan_label: z.enum(LDCT_LABELS).optional(),
  coded_lungrads: z.boolean().optional(),
  rec: z
    .looseObject({
      action: z.enum(["annual", "complete_ldct", "followup_ldct", "workup"]),
      due_days: z.number(),
      deadline_days: z.number(),
      urgency: z.enum(["routine", "suspicious", "urgent"]).optional(),
      cell: z.enum(LDCT_LABELS),
      next_step: z.string(),
    })
    .optional(),
  scan_day: z.number().optional(),
  due_day: z.number().optional(),
  deadline_day: z.number().optional(),
  followup_report: z.string().optional(),
  followup_label: z.enum(LDCT_LABELS).optional(),
  followup_next_step: z.string().optional(),
  workup_document: z.string().optional(),
  outcome: z.enum(["followup_done", ...LDCT_WORKUP_LABELS]).optional(),
  within_deadline: z.boolean().optional(),
});

export const IfhasFinding = z.looseObject({
  screen_report: z.string().optional(),
  screen_label: z.string().regex(IFHAS_SCREEN_LABEL).optional(),
  coded_results_agree: z.boolean().optional(),
  rec: z
    .looseObject({
      action: z.enum(["routine", "follow_up"]),
      deadline_days: z.number(),
      urgency: z.enum(["routine", "urgent"]).optional(),
      cell: z.string().regex(IFHAS_SCREEN_LABEL),
      abnormal_domains: z.array(IfhasDomain).optional(),
      domain_deadline_days: z.partialRecord(IfhasDomain, z.number()).optional(),
      prediabetes: z.boolean().optional(),
    })
    .optional(),
  screen_day: z.number().optional(),
  deadline_day: z.number().optional(),
  domain_status: z.partialRecord(IfhasDomain, z.enum(["", "managed", "not_confirmed", "pending"])).optional(),
  resolved_day: z.partialRecord(IfhasDomain, z.number()).optional(),
  followup_documents: z.array(z.string()).optional(),
  outcome: z.union([z.literal("dna"), IfhasFollowupLabel]).optional(),
  within_deadline: z.boolean().optional(),
  next_step: z.string().optional(),
});

/** One persisted contact outcome in the overlay. The list summary's `last_contact` carries no actor or note. */
export const OverlayContact = z.looseObject({
  at: z.string(),
  event: z.string().default(""),
  outcome: z.string(),
  channel: z.string().default(""),
  actor: z.string().optional(),
  note: z.string().optional(),
});
export const OverlayNote = z.looseObject({ at: z.string(), event: z.string().default(""), actor: z.string().default(""), text: z.string() });

export const BookingSummarySlot = z.looseObject({
  slot_id: z.string(),
  session_id: z.number(),
  unit: z.string(),
  day: z.number(),
  procedure: z.string(),
  deadline: z.number(),
});

/** Server-replayed simulated reservation state. No provider or patient system is involved. */
export const BookingSummary = z.looseObject({
  status: z.enum(["unreserved", "reserved", "cancelled"]),
  mode: z.literal("simulated"),
  revision: z.number().int().nonnegative(),
  reservation_id: z.string().nullable(),
  reservation_version: z.string().nullable(),
  slot: BookingSummarySlot.nullable(),
  updated_at: z.string().nullable(),
  updated_event: z.string().nullable(),
});

export const EligibleBookingSlot = z.looseObject({
  slot_id: z.string(),
  session_id: z.number().int().nonnegative(),
  unit: z.string(),
  day: z.number(),
  region: z.string(),
  deadline: z.number(),
  within_deadline: z.boolean(),
  endoscopist_female: z.boolean(),
  arabic: z.boolean(),
  after_hours: z.boolean(),
  prep_send_day: z.number(),
  predicted_no_show: z.number().min(0).max(1),
  nurse_call: z.boolean(),
  cross_region: z.boolean(),
  version: z.string().regex(/^[0-9a-f]{64}$/).optional(),
  availability: z.enum(["available", "reserved"]).optional(),
});

/**
 * Coordinator overlay (POST /actions). Absent in snapshots taken before the actions endpoint existed. The list carries
 * the summary form; case detail adds `contact_outcomes`, `notes` and `canonical_owner`. `ledger_status: invalid` means
 * replay stopped at an invalid ledger line, so the overlay holds only the events before it and is not authoritative.
 */
export const ApiOverlay = z.looseObject({
  head: z.string(),
  actions: z.number().default(0),
  owner: z.string().default(""),
  last_action_at: z.string().default(""),
  ledger_status: z.enum(["ok", "invalid"]).default("ok"),
  snooze: z.looseObject({ until: z.string(), reason: z.string().optional(), at: z.string().optional(), actor: z.string().optional() }).optional(),
  last_contact: OverlayContact.optional(),
  last_appointment_agreement: z.looseObject({ at: z.string(), event: z.string().default("") }).optional(),
  booking: BookingSummary.optional(),
  no_answer_streak: z.number().default(0),
  contact_outcomes: z.array(OverlayContact).optional(),
  notes: z.array(OverlayNote).optional(),
  canonical_owner: z.string().optional(),
});

/** Case detail `review_ledger`: whether the shared ledger replayed completely. */
export const ReviewLedger = z.looseObject({
  status: z.enum(["ok", "invalid"]),
  detail: z.string().optional(),
  events_applied: z.number().optional(),
  note: z.string().optional(),
});

export const RecoveryCase = z.looseObject({
  backfill: z.looseObject({
    slot_id: z.string(),
    status: z.literal("simulated_backfill_confirmed"),
    day: z.number(),
    event: z.string(),
    outreach_status: z.enum(["none", "recorded_not_dispatched", "cancelled_consent"]),
    dispatched: z.literal(false).optional(),
  }).optional(),
  source_releases: z.array(z.looseObject({ slot_id: z.string(), status: z.string(), day: z.number() })).default([]),
});

export const CaseSummary = z.looseObject({
  id: z.string(),
  programme: Programme,
  state: CaseState,
  dropoff: Dropoff,
  owner: z.string().default(""),
  provider: z.string(),
  referral_day: z.number(),
  deadline_day: z.number().default(0),
  overdue: z.boolean(),
  priority: z.string().default(""),
  consent: Consent,
  review: ReviewStatus,
  closed: z.boolean(),
  closed_day: z.number().default(0),
  fixture: z.union([FixtureName, z.literal("")]).default(""),
  decisions: z.number().optional(),
  lung: LungFinding.optional(),
  dr: DrFinding.optional(),
  breast: BreastFinding.optional(),
  ldct: LdctFinding.optional(),
  ifhas: IfhasFinding.optional(),
  api_overlay: ApiOverlay.optional(),
  recovery: RecoveryCase.optional(),
});

export const ChainEntry = z.looseObject({
  type: z.string(),
  body: z.record(z.string(), z.unknown()),
  prev: z.string(),
  hash: z.string(),
});

export const WeakestEdge = z.looseObject({ a: z.string(), b: z.string(), basis: z.string(), p: z.number() });

export const Link = z.looseObject({
  loop: z.string(),
  downstream_event: z.string(),
  referral_patient: z.string(),
  hops: z.number(),
  linker: z.string(),
  near_miss: z.boolean(),
  path_p_cal: z.number(),
  path_strength: z.number(),
  tau: z.number(),
  weakest_edge: WeakestEdge,
});

export const Evidence = z.looseObject({
  report: z.string(),
  agree: z.boolean(),
  label: z.string(),
  /** CRC and cervical evidence carries one quote; lung, DR and breast evidence carry one per reader in `reader_*_fields`. */
  quote: z.string().default(""),
  human_override: z.string().default(""),
  reader_model: z.string().optional(),
  rule: z.string().optional(),
  regex: z.string().optional(),
  gemma: z.string().optional(),
  fields: z.record(z.string(), z.unknown()).optional(),
  reader_a_regex: z.string().optional(),
  /** IFHAS calls the first reader's label `reader_a`; the other programmes use `reader_a_regex`. */
  reader_a: z.string().optional(),
  reader_b: z.string().optional(),
  /** IFHAS reader provenance is a note string; LDCT and the other programmes send field objects. */
  reader_a_fields: z.union([z.string(), z.looseObject({ quote: z.string().optional() })]).optional(),
  reader_b_fields: z.union([z.string(), z.looseObject({ quote: z.string().optional() })]).optional(),
});

export const Identity = z.looseObject({ provider: z.string(), registration: z.string(), role: z.string() });

export const TimelineEvent = z.looseObject({
  id: z.string(),
  type: z.string(),
  provider: z.string(),
  day: z.number(),
  arrival_day: z.number().optional(),
  seq: z.number().optional(),
  source: z.string().optional(),
  status: z.string().optional(),
  category: z.string().optional(),
  raw_sha: z.string().optional(),
});

export const Hold = z.looseObject({
  loop: z.string(),
  /** Absent when the hold waits for a report that has not arrived (DR, breast). */
  report: z.string().optional(),
  reason: z.string().optional(),
  rule: z.string(),
  policy: z.string().optional(),
  programme: z.string().optional(),
  state: z.string(),
});

export const ConsentLogEntry = z.looseObject({
  ev: z.string().optional(),
  kind: z.string().optional(),
  to: z.string(),
  contact: z.string().optional(),
});

export const Decision = z.looseObject({
  decision_id: z.string(),
  state: CaseState,
  dropoff: Dropoff,
  in_seq: z.number(),
  overdue: z.boolean(),
  owner: z.string().default(""),
  policy: z.string(),
  sig: z.string(),
  escalations: z.array(z.string()),
  consent_log: z.array(ConsentLogEntry),
});

export const ReviewItem = z.looseObject({
  item: z.string(),
  loop: z.string(),
  kinds: z.array(z.string()),
  pair: z.array(z.string()),
  p: z.number(),
  report: z.string(),
  verdict: z.string(),
  conf: z.number().optional(),
  quote: z.string(),
  reason: z.string().optional(),
  chain: z.array(ChainEntry),
  head: z.string().optional(),
  in_seq: z.number().optional(),
  overdue: z.boolean().optional(),
});

export const Signoff = z.looseObject({
  item: z.string(),
  loop: z.string(),
  kinds: z.array(z.string()),
  role: z.string(),
  reviewer: z.string(),
  reads: z.array(z.string()),
  link: z.string(),
  evidence: z.string(),
  verdict: z.string(),
  wrong: z.boolean().optional(),
  t: z.number().optional(),
  ms: z.number().optional(),
  sim_s: z.number().optional(),
});

/** One appended demo review action (POST /review response and case detail api_reviews). */
export const ApiReview = z.looseObject({
  case: z.string(),
  item: z.string(),
  decision: z.string(),
  note: z.string(),
  role: z.string(),
  actor: z.string(),
  action: z.string(),
  at: z.string(),
  mode: z.string(),
  prev: z.string(),
  hash: z.string(),
});

/** Consent events and the demo-only contact overlay (pipeline/api_consent.go). */
export const ConsentEventName = z.enum(["consent_granted", "consent_denied", "consent_opted_out"]);
/** `prev` is absent when it would repeat the hash of a first independent read still concealed from the viewer. */
export const ConsentEvent = ApiReview.extend({
  prev: z.string().optional(),
  decision: ConsentEventName,
  action: z.literal("record_consent"),
  head_signed: z.string(),
});
export const ConsentOverlay = z.looseObject({
  status: z.enum(["none", "granted", "denied", "opted_out"]),
  demo_pending_consent: z.enum(["", "released", "cancelled"]),
  events: z.number(),
  contact_blocked: z.boolean(),
  mode: z.literal("demo"),
  canonical_consent: Consent,
  canonical_unchanged: z.literal(true),
  last_event_hash: z.string(),
  revision: z.string().min(1),
  note: z.string(),
});
export const ApiContact = z.looseObject({ overlay: ConsentOverlay, history: z.array(ConsentEvent) });
export const ConsentRequest = z.object({
  event: ConsentEventName,
  expected_head: z.string().min(1),
  note: z.string().max(500).default(""),
});
export const ConsentResponse = withMeta({ case: z.string(), overlay: ConsentOverlay, history: z.array(ConsentEvent) });
export const ConsentAppendResponse = withMeta({ review: ConsentEvent, overlay: ConsentOverlay, note: z.string() });
export const ConsentError = withMeta({ error: z.string(), current_head: z.string().optional(), api_contact: ConsentOverlay.optional() });

/**
 * One review action as GET /audit serves it to the auditor. The redacted projection (exportReviewEventAllow in
 * pipeline/api_export.go) never carries actor or note, and drops an empty or out-of-pattern value, such as the empty
 * item of a review not tied to a queue item. Pinned by tests/fixtures/audit_signed_reviews.json.
 */
export const AuditReview = z.looseObject({
  case: z.string(),
  item: z.string().optional(),
  decision: z.string().optional(),
  note: z.string().optional(),
  role: z.string(),
  actor: z.string().optional(),
  action: z.string(),
  at: z.string(),
  mode: z.string(),
  prev: z.string(),
  hash: z.string(),
});

/*
 * The shared ledger (`<run>/api_reviews.ndjson`) holds more than reviews: coordinator actions (pipeline/api.go
 * caseAction) and undo/contest compensation events (pipeline/api_undo.go) share its hash chain, so case detail
 * `api_reviews` and audit `history` list all of them in order. Each entry is parsed by its own shape and tagged with
 * `ledger`. A review keeps every required field; anything that matches no known shape becomes an explicit
 * `unknown` entry instead of failing the whole response, so one new event kind cannot take down every page that reads
 * a case. Pinned by tests/fixtures/ledger_*.json (pipeline/api_ledger_contract_test.go).
 */
export const COORDINATOR_KINDS = ["assign_owner", "snooze", "contact_outcome", "note", "booking_accept", "booking_choose_slot", "booking_reschedule", "booking_cancel"] as const;
export const COMPENSATION_KINDS = ["undo", "contest", "contest_resolution"] as const;

const ledgerCore = {
  case: z.string(),
  role: z.string(),
  action: z.string(),
  at: z.string(),
  mode: z.string(),
  prev: z.string(),
  hash: z.string(),
};

/** `data` of a coordinator action. The redacted audit keeps only `until`, `outcome` and `channel`. */
export const CoordinatorData = z.looseObject({
  owner: z.string().optional(),
  until: z.string().optional(),
  reason: z.string().optional(),
  outcome: z.string().optional(),
  channel: z.string().optional(),
  note: z.string().optional(),
  text: z.string().optional(),
  expected_suggestion_version: z.string().optional(),
  slot_id: z.string().optional(),
  expected_slot_version: z.string().optional(),
  reservation_id: z.string().optional(),
  expected_reservation_version: z.string().optional(),
});

const coordinatorShape = {
  ...ledgerCore,
  type: z.literal("coordinator_action"),
  kind: z.enum(COORDINATOR_KINDS),
  data: CoordinatorData,
  head: z.string(),
  seq: z.number(),
};
const compensationShape = {
  ...ledgerCore,
  kind: z.enum(COMPENSATION_KINDS),
  target: z.string(),
  target_action: z.string(),
  action_head: z.string(),
  head_signed: z.string().optional(),
  resolution: z.enum(["upheld", "rejected"]).optional(),
};
const claimShape = {
  ...ledgerCore,
  type: z.literal("review_claim"),
  kind: z.enum(["claim", "renew", "release", "takeover"]),
  item: z.string(),
  question: z.enum(["link", "evidence"]),
  read: z.enum(["r1", "r2", "r3"]),
  claimed_at: z.string(),
  heartbeat_at: z.string(),
  expires_at: z.string(),
  expected_claim_head: z.string(),
};

/** A coordinator action as case detail serves it (actor and free text included). */
export const CoordinatorEvent = z.looseObject({ ...coordinatorShape, actor: z.string() });
/** An undo, contest or contest resolution as case detail serves it. */
export const CompensationEvent = z.looseObject({ ...compensationShape, actor: z.string(), target_actor: z.string(), note: z.string() });
/** A lease operation in case detail; audit/export use the actor-redacted variant below. */
export const ReviewClaimEvent = z.looseObject({ ...claimShape, actor: z.string() });
export const AuditReviewClaimEvent = z.looseObject({ ...claimShape, actor: z.string().optional() });
/** The auditor's redacted forms: no actor, note or target actor (exportReviewEventAllow in pipeline/api_export.go). */
export const AuditCoordinatorEvent = z.looseObject({ ...coordinatorShape, actor: z.string().optional() });
export const AuditCompensationEvent = z.looseObject({ ...compensationShape, actor: z.string().optional(), note: z.string().optional() });

const str = (v: unknown) => (typeof v === "string" ? v : "");

/** A ledger entry that matches no known shape: shown as unrecognised, never dropped and never guessed at. */
export const UnknownLedgerEvent = z.unknown().transform((raw) => {
  const o = raw !== null && typeof raw === "object" && !Array.isArray(raw) ? (raw as Record<string, unknown>) : {};
  return {
    ledger: "unknown" as const,
    hash: str(o.hash),
    prev: str(o.prev),
    at: str(o.at),
    actor: str(o.actor),
    role: str(o.role),
    /** `type/kind` as the API wrote them, or "" when absent. */
    label: [str(o.type), str(o.kind)].filter(Boolean).join("/"),
  };
});

const tag =
  <L extends string>(ledger: L) =>
  <T extends object>(e: T) => ({ ...e, ledger });

/*
 * What makes a ledger entry a review in Go: no `type` key (replay treats any typed event as a coordinator action,
 * pipeline/api.go replayLedgerLine) and no `kind`, or an empty one (pipeline/api_undo.go evKind). Review writers (POST review,
 * consent, independent reads) set neither. Without this, an event of a type or kind the frontend does not know that
 * happens to carry the core fields (case, role, action, at, mode, prev, hash) would be shown as a review.
 * Deliberately stricter than Go: evKind also treats a non-string `kind` and a literal `kind: "review"` as a review, while
 * this schema shows both as `unknown`. No Go writer produces either, and showing an event as unknown is the safe side of
 * that disagreement.
 */
const reviewOnly = { type: z.never().optional(), kind: z.literal("").optional() };

/** One case-detail `api_reviews` entry. */
export const LedgerEvent = z.union([
  CoordinatorEvent.transform(tag("coordinator")),
  CompensationEvent.transform(tag("compensation")),
  ReviewClaimEvent.transform(tag("claim")),
  ApiReview.extend(reviewOnly).transform(tag("review")),
  UnknownLedgerEvent,
]);
/** One audit `history` entry (redacted). */
export const AuditLedgerEvent = z.union([
  AuditCoordinatorEvent.transform(tag("coordinator")),
  AuditCompensationEvent.transform(tag("compensation")),
  AuditReviewClaimEvent.transform(tag("claim")),
  AuditReview.extend(reviewOnly).transform(tag("review")),
  UnknownLedgerEvent,
]);

export const Booking = z.looseObject({
  status: z.enum(["suggested", "no_slot_within_horizon", "not_applicable", "not_generated"]),
  data_mode: z.string().optional(),
  reason: z.string().optional(),
  suggestion_version: z.string().regex(/^[0-9a-f]{64}$/).optional(),
  suggested_slot_id: z.string().optional(),
  calendar_version: z.string().regex(/^[0-9a-f]{64}$/).optional(),
  eligible_slots: z.array(EligibleBookingSlot).optional(),
  reservable: z.boolean().optional(),
  reservation_explanation: z.string().optional(),
});

export const CurrentDecision = z.looseObject({
  id: z.string(),
  loop: z.string(),
  state: CaseState,
  chain: z.array(ChainEntry).default([]),
  sig: z.string().optional(),
  lung: LungFinding.optional(),
  dr: DrFinding.optional(),
  breast: BreastFinding.optional(),
  ldct: LdctFinding.optional(),
  ifhas: IfhasFinding.optional(),
});

export const ReviewSlot = z.enum(["r1", "r2", "r3"]);

/** One read slot of an independent review. Concealed initial answers carry no decision or hash. */
export const ReviewRead = z.looseObject({
  read: ReviewSlot,
  submitted: z.boolean(),
  decision: z.string().optional(),
  hash: z.string().optional(),
});

/** A server-owned five-minute lease for one independent-review question. */
export const ReviewClaim = z.looseObject({
  owner: z.string(),
  role: z.string(),
  read: ReviewSlot,
  claimed_at: z.string(),
  heartbeat_at: z.string(),
  expires_at: z.string(),
  status: z.enum(["active", "expired"]),
});
export const ReviewClaimActions = z.looseObject({ claim: z.boolean(), renew: z.boolean(), release: z.boolean(), takeover: z.boolean() });
export const ReviewClaimOperation = z.enum(["claim", "renew", "release", "takeover"]);
export const ReviewClaimRequest = z.object({
  item: z.string().min(1),
  question: z.enum(["link", "evidence"]),
  operation: ReviewClaimOperation,
  expected_claim_head: z.string().regex(/^(?:[0-9a-f]{64})?$/),
});

/** One (item, question) independent-review state machine as the API projects it for the calling credential. */
export const ReviewState = z.looseObject({
  item: z.string(),
  question: z.enum(["link", "evidence"]),
  state: z.enum(["awaiting_r1", "awaiting_r2", "awaiting_r3", "resolved_agreed", "resolved_senior"]),
  head: z.string(),
  next_read: ReviewSlot.nullable(),
  may_submit: z.boolean(),
  /** Optional for old fixture snapshots. A live desk treats their absence as no claim support, never ownership. */
  claim_head: z.string().optional(),
  claim: ReviewClaim.nullable().optional(),
  claim_actions: ReviewClaimActions.optional(),
  frozen: z.boolean().optional(),
  reads: z.array(ReviewRead),
  disagreement: z.boolean(),
  final_decision: z.string().optional(),
  resolution_basis: z.string().optional(),
  canonical_unchanged: z.literal(true),
});

export const CaseDetail = withMeta({
  case: CaseSummary,
  identities: z.array(Identity),
  timeline: z.array(TimelineEvent),
  links: z.array(Link),
  evidence: z.array(Evidence),
  holds: z.array(Hold),
  outreach: z.array(z.looseObject({})),
  decisions: z.array(Decision),
  review_items: z.array(ReviewItem),
  signoffs: z.array(Signoff),
  /** Every ledger event of the case, in ledger order: reviews, consent, coordinator actions, undo/contest. */
  api_reviews: z.array(LedgerEvent),
  /** Separate demo contact overlay; canonical case consent and outreach stay untouched. */
  api_contact: ApiContact.optional(),
  /** Full coordinator overlay; absent in snapshots taken before the actions endpoint existed. */
  api_overlay: ApiOverlay.optional(),
  review_ledger: ReviewLedger.optional(),
  /** Independent-review projection for the calling credential; absent from snapshots and older APIs. */
  api_review_states: z.array(ReviewState).optional(),
  current: CurrentDecision,
  booking: Booking,
});

export const Audit = withMeta({
  case: z.string(),
  chain: z.array(ChainEntry),
  chain_length: z.number(),
  chain_verified: z.boolean(),
  decision_signatures: z.array(
    z.looseObject({ decision_id: z.string(), policy: z.string(), sig: z.string(), recomputed: z.string() }),
  ),
  /** The redacted audit omits anchors when the run recorded none (anchor_note then says so). */
  anchors: z.record(z.string(), z.string()).default({}),
  anchor_note: z.string().optional(),
  history: z.array(AuditLedgerEvent),
});

export const CaseList = withMeta({
  cases: z.array(CaseSummary),
  total: z.number(),
  limit: z.number(),
  offset: z.number(),
});

export const Fixtures = withMeta({
  fixtures: z.object({
    evidence_closure: CaseSummary,
    false_closure_trap: CaseSummary,
    review_hold: CaseSummary,
    dr_exam_closure: CaseSummary.optional(),
  }),
});

const ClosureMetrics = z.looseObject({
  closed_pred: z.number(),
  tp: z.number(),
  false_closures: z.number(),
  missed: z.number(),
  precision: z.number(),
  recall: z.number(),
  false_closure_rate_upper95: z.number(),
});

const KpiArm = z.looseObject({ kpi_all_referrals: z.number(), eligible_referrals: z.number(), within_31d: z.number() });

export const Dashboard = withMeta({
  cases: z.number(),
  closed: z.number(),
  overdue: z.number(),
  states: z.record(z.string(), Count),
  dropoff: z.record(z.string(), Count),
  review: z.record(z.string(), Count),
  by_programme: z.record(z.string(), z.looseObject({ cases: z.number(), closed: z.number(), overdue: z.number() })),
  sla: z.record(z.string(), z.number()),
  small_cell_rule: z.string().optional(),
  fixtures: z.object({
    evidence_closure: z.string(),
    false_closure_trap: z.string(),
    review_hold: z.string(),
    dr_exam_closure: z.string().optional(),
  }),
  // --- Provider/origin breakdowns (issue #45) ---
  // The API copies these from the run's dashboard.json and serves null when the run has none (pipeline/api.go getDashboard),
  // so a missing or null array reads as empty rather than failing the whole dashboard. Rows stay loose; the Programme model
  // (components/programme/model.ts) parses the counts it shows, small-cell "<5" strings included.
  providers: z
    .array(z.looseObject({ provider: z.string(), loops: Count }))
    .nullish()
    .transform((v) => v ?? []),
  origins: z
    .array(z.looseObject({ origin: z.string(), loops: Count }))
    .nullish()
    .transform((v) => v ?? []),
  // --- end issue #45 ---
  metrics: z.looseObject({
    closure: z.record(
      z.string(),
      z.looseObject({
        loops: z.number(),
        wasl_live_with_review: ClosureMetrics,
        wasl_no_review: ClosureMetrics,
        baseline_eid_only: ClosureMetrics,
        baseline_any_proc_120d: ClosureMetrics,
      }),
    ),
    crc_kpi_31d: z.looseObject({
      definition: z.string(),
      truth: KpiArm,
      eid_only: KpiArm,
      wasl_live: KpiArm,
      wasl_no_review: KpiArm,
    }),
    exactly_once: z.record(z.string(), z.number()),
    forged: z.record(z.string(), z.number()),
    provenance: z.looseObject({
      closed_chains: z.number(),
      hash_verified: z.number(),
      tamper_detected: z.boolean(),
      with_human_signoff: z.number(),
    }),
    consent_timing: z.record(z.string(), z.number()),
  }),
  capacity: z.looseObject({ status: z.string(), data_mode: z.string() }),
});

export const Healthz = z.looseObject({
  status: z.string(),
  cases: z.number(),
  dependencies: z.record(z.string(), z.string()),
  meta: Meta,
});

const OperationsInstant = z.iso.datetime();
const OperationsDependencies = z.strictObject({
  materialised_run: z.string(),
  nats: z.string(),
  reader: z.string(),
  ingest_adapter: z.string(),
  review_ledger: z.string(),
});
const OperationsNATS = z.discriminatedUnion("status", [
  z.strictObject({ status: z.literal("reachable"), source: z.literal("nats_jetstream_consumer_info") }),
  z.strictObject({ status: z.literal("unreachable"), source: z.literal("nats_jetstream_consumer_info"), reason: z.literal("jetstream_probe_failed") }),
]);
const OperationsConsumer = z.discriminatedUnion("status", [
  z.strictObject({
    stream: z.enum(["FHIRIN", "FHIR", "OUT"]), consumer: z.enum(["notifications", "prep", "reviewer"]), status: z.literal("available"),
    num_pending: z.number().int().nonnegative(), num_ack_pending: z.number().int().nonnegative(),
  }),
  z.strictObject({
    stream: z.enum(["FHIRIN", "FHIR", "OUT"]), consumer: z.enum(["notifications", "prep", "reviewer"]), status: z.literal("unavailable"),
    reason: z.enum(["consumer_not_found", "consumer_info_failed"]),
  }),
]);
const OperationsQueue = z.discriminatedUnion("status", [
  z.strictObject({
    status: z.literal("available"), source: z.literal("nats_jetstream_consumer_info"), coverage: z.literal("known_durable_consumers"),
    consumers: z.array(OperationsConsumer),
  }),
  z.strictObject({
    status: z.literal("partial"), source: z.literal("nats_jetstream_consumer_info"), coverage: z.literal("known_durable_consumers"),
    reason: z.literal("consumer_info_unavailable"), consumers: z.array(OperationsConsumer),
  }),
  z.strictObject({
    status: z.literal("unavailable"), source: z.literal("nats_jetstream_consumer_info"), coverage: z.literal("known_durable_consumers"),
    reason: z.enum(["nats_unreachable", "consumer_info_unavailable"]), consumers: z.array(OperationsConsumer),
  }),
]);
const OperationsLatency = z.strictObject({ n: z.number().nonnegative(), p50: z.number().nonnegative(), p90: z.number().nonnegative(), p99: z.number().nonnegative(), max: z.number().nonnegative() });
const OperationsRecovery = z.strictObject({
  generated_resources: z.number().int().nonnegative(), fhir_msgs: z.number().int().nonnegative(),
  expected_outputs: z.number().int().nonnegative(), live_outputs: z.number().int().nonnegative(),
  ingest_lost: z.number().int().nonnegative(), ingest_duplicates: z.number().int().nonnegative(),
  lost_outputs: z.number().int().nonnegative(), duplicate_outputs: z.number().int().nonnegative(),
});
const OperationsRun = z.discriminatedUnion("status", [
  z.strictObject({
    status: z.literal("available"), scope: z.literal("completed_synthetic_run"), source: z.literal("report7.json"),
    latency_ms: OperationsLatency, recovery: OperationsRecovery,
  }),
  z.strictObject({
    status: z.literal("unmeasured"), scope: z.literal("completed_synthetic_run"), source: z.literal("report7.json"),
    reason: z.literal("run_metrics_not_recorded"),
  }),
]);
const OperationsUnmeasured = z.strictObject({ status: z.literal("unmeasured"), reason: z.string() });
const OperationsBackup = z.discriminatedUnion("status", [
  z.strictObject({
    status: z.literal("available"), source: z.literal("recovery_bundle_manifest"),
    bundle_schema: z.enum(["wasl-file-backup-v1", "wasl-file-backup-v2-aesgcm"]),
    created_at: OperationsInstant, age_seconds: z.number().int().nonnegative(), verification_status: z.literal("not_verified_by_api"),
  }),
  z.strictObject({
    status: z.literal("unavailable"), source: z.literal("recovery_bundle_manifest"),
    bundle_schema: z.enum(["wasl-file-backup-v1", "wasl-file-backup-v2-aesgcm"]).optional(), created_at: OperationsInstant.optional(),
    reason: z.enum(["backup_manifest_unreadable", "backup_manifest_invalid", "backup_created_at_invalid", "backup_created_at_in_future"]),
  }),
  z.strictObject({
    status: z.literal("unmeasured"), source: z.literal("recovery_bundle_manifest"), reason: z.literal("backup_manifest_not_configured"),
  }),
]);

/** Authenticated point-in-time operational observations, with no fallback from failed probes to zero or run history. */
export const Operations = z.strictObject({
  schema: z.literal("wasl-operations/1"), meta: Meta, observed_at: OperationsInstant,
  api: z.strictObject({ status: z.string(), source: z.literal("/healthz"), dependencies: OperationsDependencies }),
  nats: OperationsNATS,
  queue: OperationsQueue,
  run: OperationsRun,
  api_measurements: z.strictObject({
    latency: OperationsUnmeasured.extend({ reason: z.literal("api_request_latency_not_recorded") }),
    error_rate: OperationsUnmeasured.extend({ reason: z.literal("api_request_outcomes_not_recorded") }),
  }),
  backup: OperationsBackup,
});

const CRC_DECISIONS = ["same", "different", "adequate", "inadequate", "confirm_contact"] as const;
const LUNG_OUTCOMES = [
  "stable",
  "resolved",
  "growing",
  "suspicious_solid",
  "not_assessable",
  "pet_low",
  "pet_avid",
  "benign",
  "nondiagnostic",
  "malignant",
] as const;
/**
 * The size bands pipeline/fleischner.go sizeBand produces for each nodule type: solid nodules band as <6, 6-8 or >8 mm;
 * ground-glass and part-solid nodules as <6 or >=6 mm. The review desk offers only these bands.
 */
const LUNG_SIZE_BANDS = {
  solid: ["<6", "6-8", ">8"],
  ground_glass: ["<6", ">=6"],
  part_solid: ["<6", ">=6"],
} as const;
const LUNG_COUNTS = ["single", "multiple"] as const;
/**
 * Index-CT labels such as `solid/6-8/single` or `ground_glass/<6/multiple`. The API boundary keeps every band the
 * endpoint parses (pipeline/lungread.go parseFindingLabel), including ones sizeBand never writes.
 */
const LUNG_INDEX_LABEL = /^(solid|ground_glass|part_solid)\/(<6|6-8|>8|>=6)\/(single|multiple)$/;

/** DR screening grades as the review endpoint accepts them: `R{0|1|2|3A|3S}M{0|1}`, optional `/urgent`, or `U`. */
const DR_GRADE = /^(R(0|1|2|3A|3S)M[01](\/urgent)?|U)$/;
const BREAST_LABELS = ["B0", "B1", "B2", "B3", "B4", "B5", "TR", ...BREAST_OUTCOMES] as const;

const lungIndexLabels = Object.entries(LUNG_SIZE_BANDS).flatMap(([type, sizes]) => sizes.flatMap((size) => LUNG_COUNTS.map((count) => `${type}/${size}/${count}`)));
const drGradeLabels = ["0", "1", "2", "3A", "3S"].flatMap((r) => ["0", "1"].flatMap((m) => [`R${r}M${m}`, `R${r}M${m}/urgent`]));
const ifhasScreenLabels = ["N", "H", "S"].flatMap((bp) =>
  ["N", "P", "D", "S"].flatMap((glyc) => ["N", "H"].flatMap((lipid) => ["N", "O"].map((weight) => `bp=${bp};glyc=${glyc};lipid=${lipid};weight=${weight}`))),
);
function ifhasFollowupLabels(): string[] {
  const labels = ["dna"];
  const statuses = ["managed", "not_confirmed", "pending"];
  const visit = (index: number, parts: string[]) => {
    if (index === IFHAS_DOMAINS.length) {
      if (parts.length > 0) labels.push(parts.join(";"));
      return;
    }
    visit(index + 1, parts);
    for (const status of statuses) visit(index + 1, [...parts, `${IFHAS_DOMAINS[index]}=${status}`]);
  };
  visit(0, []);
  return labels;
}

/** Every evidence label the corresponding backend programme accepts, including procedure and structured-result labels. */
export const GUIDED_EVIDENCE_LABELS: Record<"crc" | "lung_nodule" | "diabetic_retinopathy" | "breast" | "lung_ldct" | "ifhas", string[]> = {
  crc: ["adequate", "inadequate"],
  lung_nodule: [...LUNG_OUTCOMES, ...lungIndexLabels],
  diabetic_retinopathy: [...drGradeLabels, "U", ...DR_OUTCOMES],
  breast: [...BREAST_LABELS],
  lung_ldct: [...LDCT_LABELS, ...LDCT_WORKUP_LABELS],
  ifhas: [...ifhasScreenLabels, ...ifhasFollowupLabels()],
};

/** Programmes the review desk can sign: each has evidence labels. Cervical has none, so its cases cannot be answered there. */
export const isGuided = (programme: string): programme is keyof typeof GUIDED_EVIDENCE_LABELS => Object.hasOwn(GUIDED_EVIDENCE_LABELS, programme);

export const ReviewDecision = z.union([
  z.enum(CRC_DECISIONS),
  z.enum(LUNG_OUTCOMES),
  z.string().regex(LUNG_INDEX_LABEL),
  z.enum(DR_OUTCOMES),
  z.string().regex(DR_GRADE),
  z.enum(BREAST_LABELS),
  z.enum(LDCT_LABELS),
  z.enum(LDCT_WORKUP_LABELS),
  z.string().regex(IFHAS_SCREEN_LABEL),
  IfhasFollowupLabel,
]);

/** `read` and `expected_head` are additive: a request without them is the legacy append-only review event. */
export const ReviewRequest = z.object({
  item: z.string(),
  decision: ReviewDecision,
  note: z.string().default(""),
  read: ReviewSlot.optional(),
  expected_head: z.string().optional(),
  claim_head: z.string().regex(/^[0-9a-f]{64}$/).optional(),
});

export const ReviewResponse = withMeta({
  review: ApiReview,
  review_state: ReviewState.optional(),
  note: z.string().optional(),
  api_review_states: z.array(ReviewState).optional(),
});

export const ReviewClaimResponse = withMeta({
  claim_event: ReviewClaimEvent,
  review_state: ReviewState.optional(),
  api_review_states: z.array(ReviewState),
  note: z.string(),
});

/** Error body of a protocol POST: a 409 names the current head so the desk can show server truth. */
export const ReviewError = withMeta({
  error: z.string(),
  conflict: z.enum(["claim_active", "claim_required", "claim_expired", "claim_head", "review_state", "head"]).optional(),
  current_head: z.string().optional(),
  current_claim_head: z.string().optional(),
  review_state: ReviewState.optional(),
  api_review_states: z.array(ReviewState).optional(),
});

/** GET /api/cases/{id}/undo: a projection only; canonical_state is reported for comparison, never rewritten. */
export const UndoStatus = z.enum(["", "active", "contested", "compensated", "open", "upheld", "rejected", "applied"]);
export const UndoEventProjection = z.looseObject({
  hash: z.string(),
  kind: z.string(),
  actor: z.string(),
  action: z.string(),
  target: z.string(),
  status: UndoStatus,
  at: z.string(),
});
const undoProjectionShape = {
  case: z.string(),
  case_status: z.enum(["none", "active", "contested", "compensated"]),
  canonical_state: CaseState,
  note: z.string(),
  events: z.array(UndoEventProjection),
};
export const UndoProjectionData = z.looseObject(undoProjectionShape);
export const UndoProjection = withMeta(undoProjectionShape);

const governanceTarget = z.string().min(1);
const governanceReason = z.string().trim().min(1).max(1000);
export const UndoRequest = z.object({ target: governanceTarget, contest: z.literal(false).default(false), note: governanceReason });
export const ContestRequest = z.object({ target: governanceTarget, contest: z.literal(true).default(true), note: governanceReason });
export const ContestResolutionRequest = z.object({
  target: governanceTarget,
  resolution: z.enum(["upheld", "rejected"]),
  note: governanceReason,
});
/** The compensation event as a viewer receives it: `prev` and `action_head` are omitted when they would repeat a concealed first-read hash. */
export const ViewerCompensationEvent = z.looseObject({
  ...compensationShape,
  prev: z.string().optional(),
  action_head: z.string().optional(),
  actor: z.string(),
  target_actor: z.string(),
  note: z.string(),
});
export const CompensationResponse = withMeta({
  event: ViewerCompensationEvent,
  canonical_reopened: z.literal(false),
  auto_closure: z.literal(false),
  canonical_state: CaseState,
  requires_resolution: z.literal(true).optional(),
  projection: UndoProjectionData,
  note: z.string(),
});

/** POST /api/cases/{id}/actions body (docs/API.md, "POST actions"). `expected_head` is added by the caller. */
const hash = z.string().regex(/^[0-9a-f]{64}$/);
const bookingReason = z.string().trim().min(1).max(200);
export const BookingAction = z.discriminatedUnion("action", [
  z.object({ action: z.literal("booking_accept"), expected_suggestion_version: hash, slot_id: z.string().min(1).max(160), expected_slot_version: hash }),
  z.object({
    action: z.literal("booking_choose_slot"), expected_suggestion_version: hash, slot_id: z.string().min(1).max(160), expected_slot_version: hash,
    reason: bookingReason,
  }),
  z.object({
    action: z.literal("booking_reschedule"), reservation_id: hash, expected_reservation_version: hash,
    expected_suggestion_version: hash, slot_id: z.string().min(1).max(160), expected_slot_version: hash, reason: bookingReason,
  }),
  z.object({ action: z.literal("booking_cancel"), reservation_id: hash, expected_reservation_version: hash, reason: bookingReason }),
]);
export const CaseAction = z.discriminatedUnion("action", [
  z.object({ action: z.literal("assign_owner"), owner: z.string().min(1).max(64) }),
  z.object({ action: z.literal("snooze"), until: z.string(), reason: z.string().max(200).optional() }),
  z.object({
    action: z.literal("contact_outcome"),
    outcome: z.enum(["reached", "no_answer", "voicemail", "wrong_number", "declined", "appointment_agreed"]),
    channel: z.enum(["phone", "sms", "email", "in_person"]),
    note: z.string().max(1000).optional(),
  }),
  z.object({ action: z.literal("note"), text: z.string().min(1).max(1000) }),
  ...BookingAction.options,
]);
export const ActionResponse = withMeta({
  /** The appended event. `actor` is the one the API bound to the coordinator token, not one the frontend asserted. */
  event: z.looseObject({ hash: z.string(), at: z.string(), actor: z.string() }),
  api_overlay: ApiOverlay,
  booking: Booking.optional(),
});
export const ActionError = withMeta({
  error: z.string(), conflict: z.string().optional(), current_head: z.string().optional(),
  current_suggestion_version: z.string().optional(), current_reservation_version: z.string().nullable().optional(),
  slot_id: z.string().optional(), current_slot_version: z.string().optional(), availability: z.enum(["available", "reserved"]).optional(),
  booking: BookingSummary.optional(), consent: z.string().optional(),
});
export const ApiError = withMeta({ error: z.string() });

export type Programme = z.infer<typeof Programme>;
export type CaseState = z.infer<typeof CaseState>;
export type Dropoff = z.infer<typeof Dropoff>;
export type FixtureName = z.infer<typeof FixtureName>;
export type Role = z.infer<typeof Role>;
export type Meta = z.infer<typeof Meta>;
export type CaseSummary = z.infer<typeof CaseSummary>;
export type ChainEntry = z.infer<typeof ChainEntry>;
export type CaseDetail = z.infer<typeof CaseDetail>;
export type Audit = z.infer<typeof Audit>;
export type CaseList = z.infer<typeof CaseList>;
export type Fixtures = z.infer<typeof Fixtures>;
export type Dashboard = z.infer<typeof Dashboard>;
export type Healthz = z.infer<typeof Healthz>;
export type Operations = z.infer<typeof Operations>;
export type ReviewRequest = z.input<typeof ReviewRequest>;
export type ReviewResponse = z.infer<typeof ReviewResponse>;
export type ReviewRead = z.infer<typeof ReviewRead>;
export type ReviewState = z.infer<typeof ReviewState>;
export type ReviewSlot = z.infer<typeof ReviewSlot>;
export type ReviewReadInput = { item: string; question: "link" | "evidence"; decision: string; expectedHead: string; claimHead: string };
export type ReviewClaimInput = { item: string; question: "link" | "evidence"; operation: "claim" | "renew" | "release" | "takeover"; expectedClaimHead: string };
export type ReviewClaim = z.infer<typeof ReviewClaim>;
export type ReviewClaimActions = z.infer<typeof ReviewClaimActions>;
export type ReviewClaimRequest = z.input<typeof ReviewClaimRequest>;
export type ReviewClaimResponse = z.infer<typeof ReviewClaimResponse>;
export type ReviewClaimEvent = z.infer<typeof ReviewClaimEvent>;
export type UndoRequest = z.input<typeof UndoRequest>;
export type ContestRequest = z.input<typeof ContestRequest>;
export type ContestResolutionRequest = z.input<typeof ContestResolutionRequest>;
export type CompensationResponse = z.infer<typeof CompensationResponse>;
export type UndoProjection = z.infer<typeof UndoProjection>;
export type ApiReview = z.infer<typeof ApiReview>;
export type ConsentEventName = z.infer<typeof ConsentEventName>;
export type ConsentEvent = z.infer<typeof ConsentEvent>;
export type ConsentOverlay = z.infer<typeof ConsentOverlay>;
export type ApiContact = z.infer<typeof ApiContact>;
export type ConsentRequest = z.input<typeof ConsentRequest>;
export type ConsentResponse = z.infer<typeof ConsentResponse>;
export type ConsentAppendResponse = z.infer<typeof ConsentAppendResponse>;
export type AuditReview = z.infer<typeof AuditReview>;
export type LedgerEvent = z.infer<typeof LedgerEvent>;
export type AuditLedgerEvent = z.infer<typeof AuditLedgerEvent>;
export type ApiOverlay = z.infer<typeof ApiOverlay>;
export type CaseAction = z.infer<typeof CaseAction>;
export type BookingAction = z.input<typeof BookingAction>;
export type Booking = z.infer<typeof Booking>;
export type BookingSummary = z.infer<typeof BookingSummary>;
export type EligibleBookingSlot = z.infer<typeof EligibleBookingSlot>;
export type ActionResponse = z.infer<typeof ActionResponse>;
