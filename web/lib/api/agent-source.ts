import { readFile } from "node:fs/promises";
import path from "node:path";
import type { z } from "zod";
import { AgentAudit, AgentCaseDetail, AgentFixtures, AgentHealthz } from "./agent-schema";
import type { AgentAudit as AuditT, AgentCaseDetail as DetailT, AgentFixtures as FixturesT } from "./agent-schema";

/*
 * The read-only accessor for the separate r1agent10 lane (CRC only, seed 987).
 * It reads web/data/api-agent only (copied from docs/api-samples/agent by scripts/sync-samples.mjs) and never calls the main API:
 * docs/API.md describes no live agent endpoint, so the agent lane is a published snapshot in every mode.
 */

const DIR = path.join(process.cwd(), "data", "api-agent");

async function read<S extends z.ZodType>(schema: S, file: string): Promise<z.infer<S>> {
  return schema.parse(JSON.parse(await readFile(path.join(DIR, file), "utf8")));
}

/** The agent lane is a snapshot in every mode; this is the sentence the page shows to say so. */
export const AGENT_LANE_SOURCE =
  "The agent lane is a published snapshot only. Wasl has no live agent endpoint, so this page reads a saved run and never calls the live API.";

export type AgentDisclosure = { source: string; line: number; text: string };

/** The disclosure sentence, copied from docs/DEMO-RECORDING-GUIDE.md line 43 by the sync script. */
export async function getAgentDisclosure(): Promise<AgentDisclosure> {
  return JSON.parse(await readFile(path.join(DIR, "disclosure.json"), "utf8")) as AgentDisclosure;
}

/** The lane's run metadata and case count (from healthz.json). */
export async function getAgentLane(): Promise<{ meta: z.infer<typeof AgentHealthz>["meta"]; cases: number }> {
  const h = await read(AgentHealthz, "healthz.json");
  return { meta: h.meta, cases: h.cases };
}

export async function getAgentFixtures(): Promise<FixturesT> {
  return read(AgentFixtures, "fixtures.json");
}

/** The one case in this lane that carries an agent investigation. */
export async function getAgentReviewCase(): Promise<{ detail: DetailT; audit: AuditT }> {
  const [detail, audit] = await Promise.all([
    read(AgentCaseDetail, "case_agent_review.json"),
    read(AgentAudit, "case_agent_review_audit.json"),
  ]);
  return { detail, audit };
}

