import { describe, expect, it } from "vitest";
import type { z } from "zod";
import { Audit, CaseDetail, CaseList, Dashboard, Fixtures, Healthz, Operations, ReviewRequest } from "@/lib/api/schema";
import { RecoveryDetail, RecoveryList } from "@/lib/api/recovery-schema";
import { PersonJourney } from "@/lib/api/person-schema";
import { loadSample, sampleFiles } from "./samples";

function schemaFor(file: string): z.ZodType {
  if (file === "dashboard.json") return Dashboard;
  if (file === "fixtures.json") return Fixtures;
  if (file === "healthz.json") return Healthz;
  if (file === "operations.json") return Operations;
  if (file === "recovery_slots.json") return RecoveryList;
  if (file === "recovery_slot.json") return RecoveryDetail;
  if (file === "person_journey.json") return PersonJourney;
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

  it("keeps historical evidence labelled and separate from the current checklist", () => {
    const sample = CaseDetail.parse(loadSample("case_dr_exam_closure.json"));
    const report = sample.evidence[0];
    const past = { ...report, history: "rejected_registration", block_hash: "block-1", decision_id: "decision-1", decision_sig: "original-signature" };
    const parsed = CaseDetail.parse({ ...sample, evidence: [], evidence_history: [past] });
    expect(parsed.evidence).toEqual([]);
    expect(parsed.evidence_history?.[0]).toMatchObject(past);
    expect(CaseDetail.safeParse({ ...sample, evidence_history: [{ ...past, history: "current" }] }).success).toBe(false);
    expect(CaseDetail.safeParse({ ...sample, evidence_history: [{ ...past, history: "superseded" }] }).success).toBe(true);
  });

  it("accepts the person_journey anchor tag wherever the API serves it", () => {
    const fx = loadSample("fixtures.json") as { fixtures: Record<string, { id: string; fixture: string }> };
    fx.fixtures.person_journey = { ...fx.fixtures.evidence_closure, fixture: "person_journey" };
    expect(Fixtures.parse(fx).fixtures.person_journey?.fixture).toBe("person_journey");
    const dashboard = loadSample("dashboard.json") as { fixtures: Record<string, string> };
    dashboard.fixtures.person_journey = fx.fixtures.person_journey.id;
    expect(Dashboard.parse(dashboard).fixtures.person_journey).toBe(fx.fixtures.person_journey.id);
    const journey = loadSample("person_journey.json") as { cases: { detail: { case: { fixture: string } } }[] };
    expect(journey.cases.length).toBeGreaterThan(0);
    for (const c of journey.cases) c.detail.case.fixture = "person_journey";
    expect(PersonJourney.safeParse(journey).error?.issues ?? []).toEqual([]);
    fx.fixtures.person_journey.fixture = "person_showcase";
    expect(Fixtures.safeParse(fx).success).toBe(false);
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

const operationsHealthy: z.input<typeof Operations> = {
  schema: "wasl-operations/1",
  meta: { api: "wasl-api/1", data_mode: "simulated", run: "r", policy: "p", reader_model: "m", loaded_at: "2026-11-13T11:00:00Z", note: "synthetic" },
  observed_at: "2026-11-13T12:00:00Z",
  api: { status: "ok", source: "/healthz", dependencies: { materialised_run: "loaded", nats: "up", reader: "cached", ingest_adapter: "fhir", review_ledger: "ok" } },
  nats: { status: "reachable", source: "nats_jetstream_consumer_info" },
  queue: {
    status: "available", source: "nats_jetstream_consumer_info", coverage: "known_durable_consumers",
    consumers: [
      { stream: "FHIRIN", consumer: "notifications", status: "available", num_pending: 2, num_ack_pending: 1 },
      { stream: "FHIR", consumer: "prep", status: "available", num_pending: 3, num_ack_pending: 0 },
      { stream: "OUT", consumer: "reviewer", status: "available", num_pending: 4, num_ack_pending: 1 },
    ],
  },
  run: {
    status: "available", scope: "completed_synthetic_run", source: "report7.json",
    latency_ms: { n: 4, p50: 12.5, p90: 30.25, p99: 48.75, max: 50 },
    recovery: { generated_resources: 100, fhir_msgs: 120, expected_outputs: 80, live_outputs: 79, ingest_lost: 1, ingest_duplicates: 2, lost_outputs: 3, duplicate_outputs: 4 },
  },
  api_measurements: {
    latency: { status: "unmeasured", reason: "api_request_latency_not_recorded" },
    error_rate: { status: "unmeasured", reason: "api_request_outcomes_not_recorded" },
  },
  backup: { status: "available", source: "recovery_bundle_manifest", bundle_schema: "wasl-file-backup-v2-aesgcm", created_at: "2026-11-13T11:59:57Z", age_seconds: 3, verification_status: "not_verified_by_api" },
};

describe("operations contract", () => {
  it("parses healthy values without inventing current API metrics", () => {
    const parsed = Operations.parse(operationsHealthy);
    expect(parsed.queue.consumers[1]).toMatchObject({ stream: "FHIR", consumer: "prep", num_pending: 3, num_ack_pending: 0 });
    expect(parsed.queue.consumers[2]).toMatchObject({ stream: "OUT", consumer: "reviewer", num_pending: 4, num_ack_pending: 1 });
    expect(parsed.run).toMatchObject({ status: "available", latency_ms: { p50: 12.5 }, recovery: { ingest_lost: 1 } });
    expect(parsed.api_measurements.latency).toEqual({ status: "unmeasured", reason: "api_request_latency_not_recorded" });
  });

  it("parses partial queue data without zero-filling the missing consumer", () => {
    const value = structuredClone(operationsHealthy);
    value.queue = {
      status: "partial", source: "nats_jetstream_consumer_info", coverage: "known_durable_consumers",
      reason: "consumer_info_unavailable", consumers: value.queue.consumers,
    };
    value.queue.consumers[1] = { stream: "FHIR", consumer: "prep", status: "unavailable", reason: "consumer_not_found" };
    const parsed = Operations.parse(value);
    expect(parsed.queue.consumers[0]).toMatchObject({ num_pending: 2 });
    expect(parsed.queue.consumers[1]).not.toHaveProperty("num_pending");
  });

  it("parses unreachable NATS with an explicitly empty queue", () => {
    const value = structuredClone(operationsHealthy);
    value.nats = { status: "unreachable", source: "nats_jetstream_consumer_info", reason: "jetstream_probe_failed" };
    value.queue = { status: "unavailable", source: "nats_jetstream_consumer_info", coverage: "known_durable_consumers", reason: "nats_unreachable", consumers: [] };
    expect(Operations.parse(value).queue.consumers).toEqual([]);
  });

  it("parses unmeasured run and backup states", () => {
    const value = structuredClone(operationsHealthy);
    value.run = { status: "unmeasured", scope: "completed_synthetic_run", source: "report7.json", reason: "run_metrics_not_recorded" };
    value.backup = { status: "unmeasured", source: "recovery_bundle_manifest", reason: "backup_manifest_not_configured" };
    const parsed = Operations.parse(value);
    expect(parsed.run).not.toHaveProperty("latency_ms");
    expect(parsed.backup).not.toHaveProperty("age_seconds");
  });

  const measuredAPI: Required<z.input<typeof Operations>["api_measurements"]> & { window: { samples: number } } = {
    window: {
      scope: "current_api_process", source: "in_memory_request_observer", process_started_at: "2026-11-13T11:58:00Z",
      start: "2026-11-13T11:58:00Z", end: "2026-11-13T12:00:00.5Z", start_bound: "process_start", length_seconds: 300, capacity: 4096,
      samples: 11, latest_sample_at: "2026-11-13T11:59:59.25Z",
    },
    latency: { status: "measured", unit: "ms", samples: 11, p50: 50, p90: 90, p99: 100, max: 100 },
    error_rate: { status: "measured", definition: "http_5xx_responses_per_request", samples: 11, server_errors: 2, client_errors: 2, rate: 0.181818 },
    routes: [
      { route: "/api/cases/:case", requests: 10, server_errors: 2, client_errors: 1 },
      { route: "/api/operations", requests: 1, server_errors: 0, client_errors: 1 },
    ],
  };

  it("parses a measured current API window with consistent sample counts", () => {
    const value = structuredClone(operationsHealthy);
    value.api_measurements = structuredClone(measuredAPI);
    const parsed = Operations.parse(value);
    expect(parsed.api_measurements.latency).toMatchObject({ status: "measured", p50: 50, p99: 100 });
    expect(parsed.api_measurements.error_rate).toMatchObject({ status: "measured", rate: 0.181818 });
  });

  it("keeps an empty observed window unmeasured and rejects zero-filled metrics", () => {
    const empty = structuredClone(operationsHealthy);
    empty.api_measurements = {
      window: { ...measuredAPI.window, samples: 0, latest_sample_at: undefined },
      latency: { status: "unmeasured", reason: "no_requests_in_window" },
      error_rate: { status: "unmeasured", reason: "no_requests_in_window" },
      routes: [],
    };
    expect(Operations.safeParse(empty).success).toBe(true);
    const zeroFilled = structuredClone(empty);
    zeroFilled.api_measurements.latency = { status: "measured", unit: "ms", samples: 1, p50: 0, p90: 0, p99: 0, max: 0 };
    expect(Operations.safeParse(zeroFilled).success).toBe(false);
  });

  it("rejects measured values without a window, mismatched counts and out-of-range rates", () => {
    const noWindow = structuredClone(operationsHealthy);
    noWindow.api_measurements = { latency: measuredAPI.latency, error_rate: measuredAPI.error_rate };
    expect(Operations.safeParse(noWindow).success).toBe(false);
    const mismatch = structuredClone(operationsHealthy);
    mismatch.api_measurements = { ...structuredClone(measuredAPI), routes: [{ route: "/api/cases/:case", requests: 3, server_errors: 0, client_errors: 0 }] };
    expect(Operations.safeParse(mismatch).success).toBe(false);
    const rate = structuredClone(operationsHealthy);
    rate.api_measurements = structuredClone(measuredAPI);
    rate.api_measurements.error_rate = { status: "measured", definition: "http_5xx_responses_per_request", samples: 11, server_errors: 2, client_errors: 2, rate: 1.5 };
    expect(Operations.safeParse(rate).success).toBe(false);
  });

  it("parses unavailable backup metadata without an age", () => {
    const value = structuredClone(operationsHealthy);
    value.backup = { status: "unavailable", source: "recovery_bundle_manifest", reason: "backup_manifest_unreadable" };
    expect(Operations.parse(value).backup).not.toHaveProperty("age_seconds");
  });

  it.each([
    ["nats", "unreachable"],
    ["queue", "unavailable"],
    ["run", "unmeasured"],
    ["backup", "unavailable"],
  ])("rejects %s %s without its reason", (section, status) => {
    const value = structuredClone(operationsHealthy) as unknown as Record<string, Record<string, unknown>>;
    value[section] = { ...value[section], status };
    delete value[section].reason;
    expect(Operations.safeParse(value).success).toBe(false);
  });

  it("rejects negative queue counts, zero-fill on unavailable consumers and unknown fields", () => {
    const negative = structuredClone(operationsHealthy);
    const measured = negative.queue.consumers[0];
    if (measured.status !== "available") throw new Error("expected measured consumer");
    measured.num_pending = -1;
    expect(Operations.safeParse(negative).success).toBe(false);
    const zero = structuredClone(operationsHealthy);
    zero.queue.consumers[0] = { ...zero.queue.consumers[0], status: "unavailable", reason: "consumer_info_failed" } as (typeof zero.queue.consumers)[number];
    expect(Operations.safeParse(zero).success).toBe(false);
    expect(Operations.safeParse({ ...operationsHealthy, internal_error: "secret" }).success).toBe(false);
  });
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
  const { ReviewState, ReviewRead, ReviewResponse, ReviewClaimRequest } = await import("@/lib/api/schema");
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
    expect(ReviewRequest.parse({ item: "i", decision: "same", read: "r1", expected_head: "", claim_head: "c".repeat(64) })).toMatchObject({ claim_head: "c".repeat(64) });
    expect(ReviewResponse).toBeDefined();
  });

  it("parses active, expired and null claims and rejects unknown lease values", () => {
    const claim = { owner: "Rev-A", role: "reviewer", read: "r2", claimed_at: "2026-11-13T10:00:00Z", heartbeat_at: "2026-11-13T10:01:00Z", expires_at: "2026-11-13T10:06:00Z", status: "active" };
    expect(ReviewState.parse({ ...state, claim, claim_head: "c".repeat(64), claim_actions: { claim: false, renew: true, release: true, takeover: false } }).claim?.status).toBe("active");
    expect(ReviewState.parse({ ...state, claim: { ...claim, status: "expired" } }).claim?.status).toBe("expired");
    expect(ReviewState.parse({ ...state, claim: null }).claim).toBeNull();
    expect(ReviewState.safeParse({ ...state, claim: { ...claim, status: "released" } }).success).toBe(false);
    expect(ReviewClaimRequest.safeParse({ item: "i", question: "link", operation: "steal", expected_claim_head: "" }).success).toBe(false);
  });
});
