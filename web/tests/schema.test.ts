import { describe, expect, it } from "vitest";
import type { z } from "zod";
import { Audit, CaseDetail, CaseList, Dashboard, Fixtures, Healthz, ReviewRequest } from "@/lib/api/schema";
import { loadSample, sampleFiles } from "./samples";

function schemaFor(file: string): z.ZodType {
  if (file === "dashboard.json") return Dashboard;
  if (file === "fixtures.json") return Fixtures;
  if (file === "healthz.json") return Healthz;
  if (file.startsWith("cases_")) return CaseList;
  if (file.endsWith("_audit.json")) return Audit;
  if (file.startsWith("case_")) return CaseDetail;
  throw new Error(`no schema for docs/api-samples/${file}; add one to lib/api/schema.ts`);
}

/** Rewrites the run as the τ 0.99 policy would serve it: policy strings and link τ. */
function asTau099(value: unknown): unknown {
  if (typeof value === "string") return value.replace("tau=0.98", "tau=0.99");
  if (Array.isArray(value)) return value.map(asTau099);
  if (value && typeof value === "object") {
    return Object.fromEntries(Object.entries(value).map(([k, v]) => [k, k === "tau" ? 0.99 : asTau099(v)]));
  }
  return value;
}

describe("docs/api-samples contract", () => {
  const files = sampleFiles();

  it("has a sample for every endpoint", () => {
    expect(files.length).toBeGreaterThanOrEqual(10);
  });

  it.each(files)("%s parses", (file) => {
    const result = schemaFor(file).safeParse(loadSample(file));
    expect(result.error?.issues ?? []).toEqual([]);
  });

  it.each(files)("%s parses as a τ 0.99 run", (file) => {
    const sample = asTau099(loadSample(file));
    expect(JSON.stringify(sample)).not.toContain("tau=0.98");
    expect(schemaFor(file).safeParse(sample).success).toBe(true);
  });

  it("keeps unknown extra fields", () => {
    const sample = loadSample("fixtures.json") as { fixtures: { review_hold: Record<string, unknown> } };
    sample.fixtures.review_hold.display_name = "Mariam Al Shamsi";
    const parsed = Fixtures.parse(sample);
    expect((parsed.fixtures.review_hold as Record<string, unknown>).display_name).toBe("Mariam Al Shamsi");
  });

  it("rejects an unknown case state", () => {
    const sample = loadSample("fixtures.json") as { fixtures: { review_hold: { state: string } } };
    sample.fixtures.review_hold.state = "teleported";
    expect(Fixtures.safeParse(sample).success).toBe(false);
  });

  it("accepts small-cell counts as \"<5\"", () => {
    const d = Dashboard.parse(loadSample("dashboard.json"));
    const values = d.origins.flatMap((o) => Object.values((o.dropoff ?? {}) as Record<string, unknown>));
    expect(values).toContain("<5");
  });
});

describe("ReviewRequest", () => {
  it.each(["same", "different", "adequate", "inadequate", "confirm_contact", "stable", "solid/6-8/single"])(
    "accepts %s",
    (decision) => expect(ReviewRequest.safeParse({ item: "rv-1", decision }).success).toBe(true),
  );

  it("rejects an unknown decision", () => {
    expect(ReviewRequest.safeParse({ item: "rv-1", decision: "close it" }).success).toBe(false);
  });
});
