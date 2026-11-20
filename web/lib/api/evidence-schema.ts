import { z } from "zod";

/*
 * The contract of web/data/evidence.json, written by scripts/evidence.py from the frozen evaluation
 * (pipeline/results/final/summary.json) and the benchmark summary. Strict on shape, so a generator change that
 * the page cannot show fails the build instead of rendering blanks.
 */

const num = z.number().finite();

export const EvidencePer1000Row = z.object({
  row: z.string().min(1),
  eid_only: num.nullable(),
  wasl: num.nullable(),
  truth: num.nullable(),
  unit: z.string(),
});

export const EvidenceCalibration = z.object({
  label: z.string().min(1),
  /** A percentage (30.5) or a share (0.305); `asPerThousand` reads both. */
  value: num,
  source: z.string().min(1),
  url: z.string(),
  caveat: z.string(),
});

export const EvidenceScenario = z.object({
  name: z.enum(["worst", "base", "best"]),
  uplift_pp: num,
  /** Percent (2.5) or share (0.025). */
  ppv: num,
  extra_colonoscopies_per_1000: num,
  cancers_found_per_1000: num,
});

export const EvidenceTornadoRow = z.object({
  input: z.string().min(1),
  low: z.string(),
  high: z.string(),
  out_low: num,
  out_high: num,
});

export const EvidenceParameter = z.object({
  name: z.string().min(1),
  value: z.string(),
  scope: z.enum(["UAE", "INTL", "ASSUMED"]),
  source: z.string(),
  url: z.string(),
});

export const EvidenceLinkage = z.object({
  noise: z.enum(["v1", "v2"]),
  metric: z.string().min(1),
  value: num,
  low: num,
  high: num,
});

export const EvidenceSafety = z.object({
  metric: z.string().min(1),
  events: z.number().int().nonnegative(),
  denominator: z.number().int().nonnegative().nullable(),
  upper_95: num.nullable(),
});

export const EvidenceBenchmark = z.object({
  name: z.string().min(1),
  dataset: z.string(),
  url: z.string(),
  metric: z.string(),
  wasl: num,
  baseline: num.nullable(),
  baseline_source: z.string().nullable(),
  note: z.string(),
});

export const EvidenceClaim = z.object({
  id: z.string().min(1),
  claim: z.string().min(1),
  status: z.enum(["built", "simulated", "designed"]),
  evidence: z.string(),
  reproduce: z.string(),
});

export const Evidence = z.object({
  generated: z.object({
    revision: z.string().min(1),
    frozen_at: z.string().min(1),
    source: z.string(),
    generated_by: z.string(),
  }),
  impact: z.object({
    per_1000: z.array(EvidencePer1000Row),
    calibration: EvidenceCalibration,
    modelled: z.object({
      label: z.string().min(1),
      scenarios: z.array(EvidenceScenario),
      tornado: z.array(EvidenceTornadoRow),
      parameters: z.array(EvidenceParameter),
    }),
  }),
  accuracy: z.object({
    definitions: z.array(z.object({ term: z.string().min(1), meaning: z.string() })),
    linkage: z.array(EvidenceLinkage),
    safety: z.array(EvidenceSafety),
    worst: z.array(z.object({ label: z.string().min(1), value: z.string() })),
  }),
  benchmarks: z.array(EvidenceBenchmark),
  claims: z.array(EvidenceClaim),
});

export type Evidence = z.infer<typeof Evidence>;
export type EvidencePer1000Row = z.infer<typeof EvidencePer1000Row>;
export type EvidenceScenario = z.infer<typeof EvidenceScenario>;
export type EvidenceTornadoRow = z.infer<typeof EvidenceTornadoRow>;
export type EvidenceLinkage = z.infer<typeof EvidenceLinkage>;
export type EvidenceSafety = z.infer<typeof EvidenceSafety>;
export type EvidenceBenchmark = z.infer<typeof EvidenceBenchmark>;
export type EvidenceClaim = z.infer<typeof EvidenceClaim>;
