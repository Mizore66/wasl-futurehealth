import { readFileSync, readdirSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import type { z } from "zod";
import { AgentAudit, AgentCaseDetail, AgentFixtures, AgentHealthz } from "@/lib/api/agent-schema";
import { Meta } from "@/lib/api/schema";
import { AgentCaseList, AgentDashboard } from "@/lib/api/agent-schema";

const DIR = path.join(__dirname, "..", "..", "docs", "api-samples", "agent");
const files = readdirSync(DIR).filter((f) => f.endsWith(".json")).sort();
const load = (f: string): unknown => JSON.parse(readFileSync(path.join(DIR, f), "utf8"));

function schemaFor(file: string): z.ZodType {
  if (file === "healthz.json") return AgentHealthz;
  if (file === "fixtures.json") return AgentFixtures;
  if (file === "dashboard.json") return AgentDashboard;
  if (file.startsWith("cases_")) return AgentCaseList;
  if (file.endsWith("_audit.json")) return AgentAudit;
  if (file.startsWith("case_")) return AgentCaseDetail;
  throw new Error(`no schema for agent/${file}`);
}

describe("docs/api-samples/agent contract", () => {
  it("has the ten JSON files of the r1agent10 lane", () => expect(files).toHaveLength(10));

  it.each(files)("%s parses", (file) => {
    expect(schemaFor(file).safeParse(load(file)).error?.issues ?? []).toEqual([]);
  });

  it.each(files)("%s carries run metadata marked simulated", (file) => {
    const meta = Meta.parse((load(file) as { meta: unknown }).meta);
    expect(meta.data_mode).toBe("simulated");
    expect(meta.policy).toContain("+agent");
  });

  it("agent_review carries a cited investigation and provenance", () => {
    const d = AgentCaseDetail.parse(load("case_agent_review.json"));
    expect(d.agent_investigations?.[0].label).toBe("same");
    expect(d.agent_investigations?.[0].human.reads).toEqual(["r1", "agent"]);
    expect(d.agent_investigations?.[0].citations.length).toBeGreaterThan(0);
    expect(d.agent_provenance).toMatchObject({ cached: true, experimental: true });
  });

  it("the synced disclosure is exactly the quoted sentence on line 43 of the recording guide", () => {
    const guide = readFileSync(path.join(__dirname, "..", "..", "docs", "DEMO-RECORDING-GUIDE.md"), "utf8").split("\n");
    const quoted = /“([^”]+)”/.exec(guide[42])![1];
    const synced = JSON.parse(readFileSync(path.join(__dirname, "..", "data", "api-agent", "disclosure.json"), "utf8")) as { text: string };
    expect(synced.text).toBe(quoted);
    expect(quoted).toBe(
      "Development passed; stress recall equals guarded human and uses 10.8% less reviewer time, but has five fewer closures than unguarded. Validation did not run.",
    );
  });

  it("rejects provenance that is missing its flags", () => {
    const d = load("case_agent_review.json") as { agent_provenance: Record<string, unknown> };
    delete d.agent_provenance.experimental;
    expect(AgentCaseDetail.safeParse(d).success).toBe(false);
  });
});
