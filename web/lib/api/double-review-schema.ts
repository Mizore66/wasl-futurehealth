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
 * One read event as case detail `api_reviews` and the auditor's `history` serve it. Fields the API may omit are
 * optional or defaulted: a blinded read has no `decision`, `note` or `nonce`, and the auditor's form never has `actor`,
 * `note` or `nonce`. A field of the wrong type fails this shape; schema.ts then makes the entry `unknown`, never a
 * single-signer review.
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

/**
 * The fields of a double-review event that may be shown while its item is open (lacks an r1 or r2 read): who read,
 * when, and where it sits in the ledger, never what was answered. An allow-list, so `decision`, `note`, `nonce`,
 * `final` and any field a future or misbehaving API adds are all withheld. `phase_after` is handled separately: it is
 * kept only as `awaiting_reads`, the one transition a read on an open item can have made.
 */
const OPEN_READ_FIELDS = new Set([
  "ledger",
  "type",
  "kind",
  "case",
  "item",
  "question",
  "read",
  "role",
  "actor",
  "action",
  "at",
  "mode",
  "prev",
  "hash",
  "head_signed",
  "round_head",
  "round_seq",
]);

/**
 * Case detail `api_reviews` and audit `history` blind an initial read for EVERYONE, its author included, until both
 * initial reads of its item exist (docs/API-DOUBLE-REVIEW.md). This repeats that rule on the parsed list: every
 * double-review event of an item without both an r1 and an r2 read in the list keeps only OPEN_READ_FIELDS and is
 * marked blinded, whatever the response carried. A misbehaving API therefore cannot get another person's blinded
 * answer, or a settled answer for a round that is not settled, onto a case or audit page through these lists. A
 * `type: "double_review"` entry that does not parse as one never gets here as one: schema.ts makes it an `unknown`
 * ledger entry, which carries no answer or note.
 */
export function blindOpenReads<E extends { ledger: string }>(events: E[]): E[] {
  const reads = (e: E) => (e.ledger === "double_review" ? (e as unknown as { item: string; read: string; kind: string; phase_after: string }) : null);
  const initial = new Map<string, Set<string>>();
  for (const e of events) {
    const r = reads(e);
    if (r && r.kind === "read") initial.set(r.item, (initial.get(r.item) ?? new Set()).add(r.read));
  }
  return events.map((e) => {
    const r = reads(e);
    if (!r) return e;
    const seen = initial.get(r.item);
    if (seen?.has("r1") && seen.has("r2")) return e;
    const kept = Object.entries(e).filter(([k]) => OPEN_READ_FIELDS.has(k));
    return Object.fromEntries([...kept, ["phase_after", r.phase_after === "awaiting_reads" ? r.phase_after : ""], ["blinded", true]]) as E;
  });
}

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
