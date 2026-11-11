import { z } from "zod";

/*
 * Independent double review (issue #41): GET/POST /api/cases/{id}/double-review and the `type: "double_review"` read
 * events it appends to the shared review ledger. Contract: docs/API-DOUBLE-REVIEW.md, pipeline/api_double_review.go.
 * Pinned by the real handler output in tests/fixtures/double_review_*.json (pipeline/api_double_review_test.go).
 *
 * This module imports nothing from ./schema, so schema.ts can place these shapes in its ledger unions without a cycle.
 */

export const READS = ["r1", "r2", "r3"] as const;
export type ReadName = (typeof READS)[number];
export const PHASES = ["awaiting_reads", "agreed", "disagreement", "resolved"] as const;
export type Phase = (typeof PHASES)[number];

const text = z.string().default("");

/**
 * One read event as case detail `api_reviews` and the auditor's `history` serve it. Every field but `type` is lenient,
 * so any `double_review` event lands here and is never mistaken for a single-signer review: a blinded read has no
 * `decision`, `note` or `nonce`, and the auditor's form never has `actor`, `note` or `nonce`.
 */
export const DoubleReviewEvent = z.looseObject({
  type: z.literal("double_review"),
  kind: text,
  case: text,
  item: text,
  question: text,
  read: text,
  decision: z.string().optional(),
  note: z.string().optional(),
  role: text,
  actor: z.string().optional(),
  action: text,
  at: text,
  mode: text,
  prev: text,
  hash: text,
  round_head: text,
  round_seq: z.number().optional(),
  phase_after: text,
  final: z.string().optional(),
  blinded: z.boolean().default(false),
});

/** One read as GET /double-review shows it to the calling credential. A blinded read has no decision or note. */
export const DoubleReadView = z.looseObject({
  read: z.string(),
  actor: z.string(),
  role: z.string(),
  at: z.string(),
  hash: z.string(),
  blinded: z.boolean(),
  decision: z.string().optional(),
  note: z.string().optional(),
});

export const DoubleReviewRound = z.looseObject({
  item: z.string(),
  question: z.string(),
  action: z.string(),
  phase: z.string(),
  head: z.string(),
  seq: z.number(),
  r3_open: z.boolean(),
  /** The reads this credential could submit now, by the API's own checks. */
  viewer_may: z.array(z.string()),
  final: z.string().optional(),
  reads: z.looseObject({ r1: DoubleReadView.nullable(), r2: DoubleReadView.nullable(), r3: DoubleReadView.nullable() }),
});

export const DoubleReviewState = z.looseObject({
  protocol: z.string(),
  case: z.string(),
  /** The principal the API bound to the credential that asked: who a read sent with it would be recorded as. */
  viewer: z.looseObject({ role: z.string(), actor: z.string() }),
  rounds: z.array(DoubleReviewRound),
  canonical_state: z.string().optional(),
  canonical_review: z.string().optional(),
  note: z.string().optional(),
});

export const DoubleReadRequest = z.object({
  item: z.string().min(1),
  read: z.enum(READS),
  decision: z.string().min(1).max(80),
  note: z.string().max(1000).default(""),
  expected_head: z.string().min(1).max(128),
});

export const DoubleReadResponse = z.looseObject({
  event: DoubleReviewEvent,
  round: DoubleReviewRound,
  canonical_unchanged: z.boolean().optional(),
  note: z.string().optional(),
});

/** A refusal: `reason` is `rbac` or `same_actor` (403); `conflict` is `head`, `read` or `phase` (409). */
export const DoubleReviewError = z.looseObject({
  error: z.string(),
  reason: z.string().optional(),
  conflict: z.string().optional(),
  current_head: z.string().optional(),
  phase: z.string().optional(),
});

export type DoubleReviewEvent = z.infer<typeof DoubleReviewEvent>;
export type DoubleReadView = z.infer<typeof DoubleReadView>;
export type DoubleReviewRound = z.infer<typeof DoubleReviewRound>;
export type DoubleReviewState = z.infer<typeof DoubleReviewState>;
export type DoubleReadRequest = z.input<typeof DoubleReadRequest>;
export type DoubleReadResponse = z.infer<typeof DoubleReadResponse>;
