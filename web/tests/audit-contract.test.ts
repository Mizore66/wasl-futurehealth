import { readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { ApiReview, Audit } from "@/lib/api/schema";

/*
 * fixtures/audit_signed_reviews.json is GET /api/cases/{id}/audit as the Go API serves it to the auditor after two
 * reviews were signed through the authenticated handler. pipeline/api_audit_contract_test.go writes it
 * (WASL_WRITE_AUDIT_FIXTURE=1) and fails when the served shape drifts from it.
 */
const served = JSON.parse(readFileSync(path.join(__dirname, "fixtures", "audit_signed_reviews.json"), "utf8")) as {
  history: Record<string, unknown>[];
};

describe("redacted audit contract", () => {
  it("the served audit with signed reviews parses", () => {
    const result = Audit.safeParse(served);
    expect(result.error?.issues ?? []).toEqual([]);
    expect(result.data?.history).toHaveLength(2);
  });

  it("pins the redaction: history carries no actor or note", () => {
    expect(served.history.length).toBeGreaterThan(0);
    for (const event of served.history) {
      expect(event).not.toHaveProperty("actor");
      expect(event).not.toHaveProperty("note");
      expect(event).toHaveProperty("decision");
      expect(event).toHaveProperty("hash");
    }
  });

  it("case detail reviews still require actor and note", () => {
    // Case detail serves the unredacted event; the audit-only schema must not loosen it.
    expect(ApiReview.safeParse(served.history[0]).success).toBe(false);
  });
});
