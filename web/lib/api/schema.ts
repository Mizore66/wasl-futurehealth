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
export const COORDINATOR_KINDS = ["assign_owner", "snooze", "contact_outcome", "note"] as const;
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

/** A coordinator action as case detail serves it (actor and free text included). */
export const CoordinatorEvent = z.looseObject({ ...coordinatorShape, actor: z.string() });
/** An undo, contest or contest resolution as case detail serves it. */
export const CompensationEvent = z.looseObject({ ...compensationShape, actor: z.string(), target_actor: z.string(), note: z.string() });
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

/** One case-detail `api_reviews` entry. */
export const LedgerEvent = z.union([
  CoordinatorEvent.transform(tag("coordinator")),
  CompensationEvent.transform(tag("compensation")),
  ApiReview.transform(tag("review")),
  UnknownLedgerEvent,
]);
/** One audit `history` entry (redacted). */
export const AuditLedgerEvent = z.union([
  AuditCoordinatorEvent.transform(tag("coordinator")),
  AuditCompensationEvent.transform(tag("compensation")),
  AuditReview.transform(tag("review")),
  UnknownLedgerEvent,
]);

export const Booking = z.looseObject({
  status: z.enum(["suggested", "no_slot_within_horizon", "not_applicable", "not_generated"]),
  data_mode: z.string().optional(),
  reason: z.string().optional(),
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
  /** Full coordinator overlay; absent in snapshots taken before the actions endpoint existed. */
  api_overlay: ApiOverlay.optional(),
  review_ledger: ReviewLedger.optional(),
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

const CRC_DECISIONS = ["same", "different", "adequate", "inadequate", "confirm_contact"] as const;
const LUNG_OUTCOMES = ["stable", "resolved", "growing", "suspicious_solid", "not_assessable"] as const;
/** Index-CT labels such as `solid/6-8/single` or `ground_glass/<6/multiple`. */
const LUNG_INDEX_LABEL = /^[a-z_]+\/[^/\s]+\/[a-z]+$/;

/** DR screening grades as the review endpoint accepts them: `R{0|1|2|3A|3S}M{0|1}`, optional `/urgent`, or `U`. */
const DR_GRADE = /^(R(0|1|2|3A|3S)M[01](\/urgent)?|U)$/;
const BREAST_LABELS = ["B0", "B1", "B2", "B3", "B4", "B5", "TR", ...BREAST_OUTCOMES] as const;

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

export const ReviewRequest = z.object({
  item: z.string(),
  decision: ReviewDecision,
  note: z.string().default(""),
});

export const ReviewResponse = withMeta({ review: ApiReview, note: z.string().optional() });

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
export const CompensationResponse = withMeta({
  event: CompensationEvent,
  canonical_reopened: z.literal(false),
  auto_closure: z.literal(false),
  canonical_state: CaseState,
  requires_resolution: z.literal(true).optional(),
  projection: UndoProjectionData,
  note: z.string(),
});

/** POST /api/cases/{id}/actions body (docs/API.md, "POST actions"). `expected_head` is added by the caller. */
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
]);
export const ActionResponse = withMeta({
  /** The appended event. `actor` is the one the API bound to the coordinator token, not one the frontend asserted. */
  event: z.looseObject({ hash: z.string(), at: z.string(), actor: z.string() }),
  api_overlay: ApiOverlay,
});
export const ActionError = withMeta({ error: z.string(), conflict: z.string().optional(), current_head: z.string().optional() });
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
export type ReviewRequest = z.input<typeof ReviewRequest>;
export type ReviewResponse = z.infer<typeof ReviewResponse>;
export type UndoRequest = z.input<typeof UndoRequest>;
export type ContestRequest = z.input<typeof ContestRequest>;
export type ContestResolutionRequest = z.input<typeof ContestResolutionRequest>;
export type CompensationResponse = z.infer<typeof CompensationResponse>;
export type UndoProjection = z.infer<typeof UndoProjection>;
export type ApiReview = z.infer<typeof ApiReview>;
export type AuditReview = z.infer<typeof AuditReview>;
export type LedgerEvent = z.infer<typeof LedgerEvent>;
export type AuditLedgerEvent = z.infer<typeof AuditLedgerEvent>;
export type ApiOverlay = z.infer<typeof ApiOverlay>;
export type CaseAction = z.infer<typeof CaseAction>;
export type ActionResponse = z.infer<typeof ActionResponse>;
