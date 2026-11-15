// A server-projected denial of authority, never a client proof of authority.
// Compensation/pair attribution remains in the existing dedicated contracts.
const NONBINDING = "legacy_unbound_nonbinding";
const MODERN_PROVENANCE = ["type", "protocol", "nonce", "read", "expected_head", "claim_head", "resolution_reads", "claimed_at", "heartbeat_at", "expires_at", "expected_claim_head"] as const;

type HistoryInput = { ledger?: unknown; source_binding?: unknown; action?: unknown; decision?: unknown; question?: unknown; [field: string]: unknown };
const modern = (event: HistoryInput) => MODERN_PROVENANCE.some((field) => event[field] !== undefined);

/** Deny-only filter at existing legacy-answer inputs; final protocol states are unchanged. */
export function legacyReviewMayAnswer(event: object): boolean {
  const row = event as HistoryInput;
  return row.ledger === "review" && !modern(row) && row.source_binding !== NONBINDING;
}

/** Label only the server-recognized old CRC evidence shape, not a modern read or claim. */
export function isNonbindingLegacyHistory(event: object): boolean {
  const row = event as HistoryInput;
  return row.ledger === "review" && row.source_binding === NONBINDING && !modern(row) && row.question === undefined && row.action === "signoff_evidence" && ["adequate", "inadequate", "histology", "no_histology"].includes(String(row.decision));
}
