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

  it("rejects an unknown programme and an unknown DR outcome", () => {
    const fx = loadSample("fixtures.json") as { fixtures: { dr_exam_closure: { programme: string; dr: { outcome: string } } } };
    expect(Fixtures.safeParse(fx).success).toBe(true);
    fx.fixtures.dr_exam_closure.dr.outcome = "seen";
    expect(Fixtures.safeParse(fx).success).toBe(false);
    fx.fixtures.dr_exam_closure.dr.outcome = "examined_followup";
    fx.fixtures.dr_exam_closure.programme = "prostate";
    expect(Fixtures.safeParse(fx).success).toBe(false);
  });

  it("accepts small-cell counts as \"<5\"", () => {
    const d = Dashboard.parse(loadSample("dashboard.json"));
    const values = d.origins.flatMap((o) => Object.values((o.dropoff ?? {}) as Record<string, unknown>));
    expect(values).toContain("<5");
  });

  it.each(["lung_ldct", "ifhas"] as const)("keeps the complete %s finding on case and current", (programme) => {
    const page = CaseList.parse(loadSample(`cases_${programme}_page1.json`));
    const key = programme === "lung_ldct" ? "ldct" : "ifhas";
    const summary = page.cases.find((c) => c[key]?.outcome);
    expect(summary).toBeDefined();
    const finding = { ...summary![key] };
    if (programme === "lung_ldct") {
      Object.assign(finding, { followup_report: "LCS-ct-77", followup_label: "LR3", followup_next_step: "LDCT in 6 months" });
    }

    const detail = loadSample("case_dr_exam_closure.json") as Record<string, unknown>;
    const current = { ...(detail.current as Record<string, unknown>), [key]: finding };
    delete current.dr;
    const parsed = CaseDetail.parse({ ...detail, case: { ...summary, [key]: finding }, current });
    expect(parsed.case[key]).toEqual(finding);
    expect(parsed.current[key]).toEqual(finding);
  });

  it.each(["incomplete_scan_not_completed", "followup_ldct_not_done", "no_workup_outcome", "no_followup_documented"] as const)(
    "accepts the documented drop-off %s",
    (dropoff) => {
      const page = loadSample("cases_lung_ldct_page1.json") as { cases: Record<string, unknown>[] };
      page.cases = page.cases.slice(0, 1).map((c) => ({ ...c, state: "open", closed: false, dropoff }));
      expect(CaseList.safeParse(page).success).toBe(true);
    },
  );
});

describe("ReviewRequest", () => {
  it.each(["same", "different", "adequate", "inadequate", "confirm_contact", "stable", "solid/6-8/single"])(
    "accepts %s",
    (decision) => expect(ReviewRequest.safeParse({ item: "rv-1", decision }).success).toBe(true),
  );

  it.each(["R0M0", "R2M1", "R3SM0", "R3AM1", "R1M0/urgent", "U", "examined_treated", "not_assessable", "B0", "B5", "TR", "early_recall"])(
    "accepts the DR or breast label %s",
    (decision) => expect(ReviewRequest.safeParse({ item: "rv-1", decision }).success).toBe(true),
  );

  it.each([
    "LR0",
    "LR1",
    "LR2",
    "LR3",
    "LR4A",
    "LR4B",
    "LR4X",
    "wk_treatment",
    "wk_benign",
    "wk_surveillance",
    "wk_incomplete",
    "wk_dna",
  ])("accepts the LDCT review label %s", (decision) => {
    expect(ReviewRequest.safeParse({ item: "rv-1", decision }).success).toBe(true);
  });

  it.each([
    "bp=H;glyc=D;lipid=N;weight=O",
    "bp=N;glyc=P;lipid=N;weight=N",
    "bp=managed",
    "glyc=pending;bp=not_confirmed",
    "lipid=managed;weight=pending;bp=not_confirmed;glyc=managed",
    "dna",
  ])("accepts the IFHAS review label %s", (decision) => {
    expect(ReviewRequest.safeParse({ item: "rv-1", decision }).success).toBe(true);
  });

  it.each(["close it", "R9M0", "R2M2", "B6", "examined", "LR5", "wk_unknown", "bp=X;glyc=N;lipid=N;weight=N", "bp=unknown", "bp=managed;bp=pending", "indeterminate"])(
    "rejects %s",
    (decision) => {
      expect(ReviewRequest.safeParse({ item: "rv-1", decision }).success).toBe(false);
    },
  );
});

describe("independent review contracts", async () => {
  const { ReviewState, ReviewRead, ReviewResponse } = await import("@/lib/api/schema");
  const state = { item: "i", question: "link", state: "awaiting_r2", head: "h", next_read: "r2", may_submit: false, reads: [{ read: "r1", submitted: true }, { read: "r2", submitted: false }], disagreement: false, canonical_unchanged: true };

  it("parses a concealed projection and keeps unknown fields", () => {
    expect(ReviewRead.parse({ read: "r1", submitted: true })).toEqual({ read: "r1", submitted: true });
    expect(ReviewState.parse({ ...state, extra: 1 })).toMatchObject({ extra: 1, next_read: "r2" });
    expect(ReviewState.parse({ ...state, state: "resolved_agreed", next_read: null }).next_read).toBeNull();
  });

  it("rejects unknown slots, states and a false canonical_unchanged", () => {
    expect(ReviewState.safeParse({ ...state, state: "done" }).success).toBe(false);
    expect(ReviewState.safeParse({ ...state, canonical_unchanged: false }).success).toBe(false);
    expect(ReviewRead.safeParse({ read: "r4", submitted: true }).success).toBe(false);
  });

  it("keeps the legacy request valid and accepts additive protocol fields", () => {
    expect(ReviewRequest.parse({ item: "i", decision: "same" })).toEqual({ item: "i", decision: "same", note: "" });
    expect(ReviewRequest.parse({ item: "i", decision: "same", read: "r1", expected_head: "" })).toMatchObject({ read: "r1", expected_head: "" });
    expect(ReviewResponse).toBeDefined();
  });
});
