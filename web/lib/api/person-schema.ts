import { z } from "zod";
import { ActiveCaseSummary, ActiveDecision, RetainedDecision, Consent, Hold, Identity, Link, Meta, Programme, TimelineEvent } from "./schema";

const hex64 = /^[0-9a-f]{64}$/;

export const PersonOutreach = z.looseObject({
  kind: z.string(),
  ev: z.string(),
  basis: z.string(),
  p_cal: z.number(),
  released: z.boolean(),
  suppress: z.string().default(""),
  guard: z.string().default(""),
  pair: z.array(z.string()).optional(),
  channel: z.string().optional(),
  sender: z.string().optional(),
  tpl: z.string().optional(),
});

export const PersonDecision = z.union([
  ActiveDecision.extend({ outreach: z.array(PersonOutreach).default([]) }),
  RetainedDecision.extend({ outreach: z.array(PersonOutreach).max(0).default([]) }),
]);

export const PersonObligation = z.looseObject({
  case_id: z.string(),
  programme: Programme,
  kind: z.string(),
  due_day: z.number(),
  window_start_day: z.number().optional(),
  status: z.enum(["open", "scheduled", "overdue", "informational"]),
  source_field: z.string(),
  basis: z.string().optional(),
});

export const OutreachIntent = z.looseObject({
  case_id: z.string(),
  programme: Programme,
  first_decision_id: z.string(),
  latest_decision_id: z.string(),
  latest_decision_sig: z.string(),
  lifecycle: z.enum(["released", "held", "withheld", "recorded"]),
  participation: z.enum(["current", "historical"]).optional(),
  outreach: PersonOutreach,
});

export const OutreachFlag = z.looseObject({
  id: z.string().regex(/^outreach_[0-9a-f]{64}$/),
  match: z.enum(["duplicate", "overlap"]),
  actionable: z.boolean(),
  intents: z.array(OutreachIntent),
});

export const NextAction = z.looseObject({
  kind: z.enum(["review_duplicate_outreach", "review_contact_permission", "programme_follow_up", "none"]),
  case_id: z.string().optional(),
  outreach_flag_id: z.string().optional(),
  priority: z.enum(["urgent", "routine"]),
  deadline_day: z.number().optional(),
  overdue: z.boolean(),
  reason_codes: z.array(z.string()),
  rule: z.literal("wasl-person-next-action/1"),
});

export const PersonCase = z.looseObject({
  case_id: z.string(),
  parent_case_id: z.string().optional(),
  referral_registration: z.string().regex(/^reg_[0-9a-f]{64}$/),
  lifecycle: z.literal("active").optional(),
  history_availability: z.literal("reference_only").optional(),
  detail: z.looseObject({
    case: ActiveCaseSummary,
    identities: z.array(Identity),
    timeline: z.array(TimelineEvent),
    links: z.array(Link),
    holds: z.array(Hold),
    outreach: z.array(PersonOutreach),
    decisions: z.array(PersonDecision),
  }),
  contact: z.looseObject({
    canonical: Consent,
    overlay_status: z.enum(["none", "granted", "denied", "opted_out"]),
    contact_blocked: z.boolean(),
    canonical_unchanged: z.literal(true),
  }),
});

const LegacyPersonJourney = z.looseObject({
  schema: z.literal("wasl-person-journey/1"),
  meta: Meta,
  person: z.looseObject({
    id: z.string().regex(/^person_[0-9a-f]{64}$/),
    anchor_case_id: z.string(),
    as_of_day: z.number(),
    case_ids: z.array(z.string()).min(1),
    programmes: z.array(Programme),
    registrations: z.array(z.looseObject({ registration: z.string().regex(/^reg_[0-9a-f]{64}$/), providers: z.array(z.string()) })),
    identity_provenance: z.looseObject({
      source: z.literal("canonical_linker_clusters"),
      artifact_schema: z.literal("wasl-identity-clusters/1"),
      artifact_sha256: z.string().regex(hex64),
      policy: z.string(),
    }),
  }),
  cases: z.array(PersonCase).min(1),
  obligations: z.array(PersonObligation),
  outreach_flags: z.array(OutreachFlag),
  next_action: NextAction,
});

export const RetainedPersonReference = z.looseObject({
  case_id: z.string(), parent_case_id: z.string().optional(),
  lifecycle: z.enum(["active", "retained"]), availability: z.literal("reference_only"),
  history_access: z.literal("authorized_case_audit"), actionable: z.literal(false),
  programmes: z.array(Programme),
  decisions: z.array(z.looseObject({ decision_id: z.string(), sig: z.string().regex(hex64), in_seq: z.number().int().positive(), facts_seq: z.number().int().positive(), inactive: z.boolean(), programme: Programme.optional() })),
  outreach_intents: z.array(OutreachIntent.safeExtend({ participation: z.literal("historical") })),
});

const RetainedPersonJourney = LegacyPersonJourney.extend({
  schema: z.literal("wasl-person-journey/2"),
  person: LegacyPersonJourney.shape.person.extend({ identity_provenance: LegacyPersonJourney.shape.person.shape.identity_provenance.extend({ artifact_schema: z.literal("wasl-identity-clusters/2") }) }),
  cases: z.array(PersonCase.safeExtend({ lifecycle: z.literal("active"), history_availability: z.literal("reference_only") })),
  retained_references: z.array(RetainedPersonReference),
});
export const PersonJourney = z.union([LegacyPersonJourney, RetainedPersonJourney]);

export const PersonAssociationUnavailable = z.object({
  schema: z.literal("wasl-person-association-unavailable/2"), meta: Meta,
  case_id: z.string(), lifecycle: z.enum(["active", "retained"]), actionable: z.literal(false),
  history_access: z.literal("authorized_case_audit"),
  association: z.object({ status: z.literal("unavailable"), reason: z.enum(["conflicting_request", "unbound_request", "unknown_programme", "unproven_binding", "multiple_historical_subjects"]) }),
});
export type PersonAssociationUnavailable = z.infer<typeof PersonAssociationUnavailable>;
export type PersonJourney = z.infer<typeof PersonJourney>;
export type PersonCase = z.infer<typeof PersonCase>;
export type NextAction = z.infer<typeof NextAction>;
export type OutreachFlag = z.infer<typeof OutreachFlag>;
export type PersonObligation = z.infer<typeof PersonObligation>;
