import { z } from "zod";
import { ActiveCaseSummary, RetainedCaseSummary, Audit, CaseDetail, CaseList, Dashboard, Healthz, Meta } from "./schema";

/*
 * Schemas for the separate r1agent10 lane (docs/api-samples/agent/).
 * They extend the loose CaseDetail and Audit; schema.ts is untouched.
 */

/** The main schema's fixture enum has no agent_review; this lane adds it. */
const AgentFixture = { fixture: z.union([z.enum(["agent_review", "evidence_closure", "false_closure_trap"]), z.literal("")]).default("") };
export const AgentCaseSummary = z.union([ActiveCaseSummary.extend(AgentFixture), RetainedCaseSummary]);

export const AgentCaseList = CaseList.extend({ cases: z.array(AgentCaseSummary) });

/** One cited fact. `arg` is a record reference, `value` the field value the tool returned. The raw prompt is never part of the sample. */
export const AgentCitation = z.looseObject({
  tool: z.string(),
  arg: z.string(),
  field: z.string(),
  value: z.string(),
});

export const AgentInvestigation = z.looseObject({
  item: z.string(),
  label: z.string(),
  pair: z.array(z.string()),
  citations: z.array(AgentCitation),
  human: z.looseObject({
    link: z.string(),
    reads: z.array(z.string()),
    role: z.string(),
  }),
});

export const AgentProvenance = z.looseObject({
  cached: z.boolean(),
  experimental: z.boolean(),
  model: z.string(),
  prompt_ver: z.string(),
  schema_ver: z.string(),
});

export const AgentCaseDetail = CaseDetail.extend({
  case: AgentCaseSummary,
  agent_investigations: z.array(AgentInvestigation).optional(),
  agent_provenance: AgentProvenance.optional(),
});

/** The agent lane's audit returns `anchors: null` where the main lane returns an object. */
export const AgentAudit = Audit.extend({
  anchors: z.record(z.string(), z.string()).nullable(),
});

/** This lane has no review_hold and no dr_exam_closure fixture. */
export const AgentFixtureMap = z.looseObject({
  agent_review: AgentCaseSummary,
  evidence_closure: AgentCaseSummary,
  false_closure_trap: AgentCaseSummary,
});

export const AgentFixtures = z.looseObject({ meta: Meta, fixtures: AgentFixtureMap });

export const AgentHealthz = Healthz;

/** The dashboard names the lane's own fixtures rather than the main four. */
export const AgentDashboard = Dashboard.extend({
  // A programme with no loops (cervical here) reports null rates, so the closure arms are loose.
  metrics: Dashboard.shape.metrics.extend({ closure: z.record(z.string(), z.looseObject({ loops: z.number() })) }),
  fixtures: z.looseObject({ agent_review: z.string(), evidence_closure: z.string(), false_closure_trap: z.string() }),
});

export type AgentInvestigation = z.infer<typeof AgentInvestigation>;
export type AgentProvenance = z.infer<typeof AgentProvenance>;
export type AgentCaseDetail = z.infer<typeof AgentCaseDetail>;
export type AgentAudit = z.infer<typeof AgentAudit>;
export type AgentFixtures = z.infer<typeof AgentFixtures>;
