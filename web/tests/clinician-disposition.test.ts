import { describe, expect, it } from "vitest";
import { CaseList } from "@/lib/api/schema";
import { statusWords, workItem } from "@/components/worklist/model";
import { ledgerWords } from "@/components/case/words";
import { loadSample } from "./samples";

const open = CaseList.parse(loadSample("cases_crc_page1.json")).cases.find((c) => !c.closed && c.dropoff !== "")!;
const held = (dropoff: "not_assessable" | "out_of_scope", overlay?: Record<string, unknown>) =>
  CaseList.parse({
    ...(loadSample("cases_crc_page1.json") as object),
    cases: [{ ...open, dropoff, state: dropoff, owner: "coordinator-1", api_overlay: { ...open.api_overlay, head: "h", ...overlay } }],
  }).cases[0];

const plan = {
  clinician_disposition: {
    at: "2026-11-01T00:00:00Z", event: "e".repeat(64), hold: "clinician_decision_outside_fleischner",
    plan: "specialist_pathway", owner: "Dr Synthetic", review_date: "2026-12-01",
  },
};

describe("clinician disposition on the worklist", () => {
  it("shows the clinician prompt until a plan is recorded", () => {
    expect(statusWords(held("out_of_scope")).next).toBe("Clinician to decide");
    expect(statusWords(held("not_assessable")).next).toBe("Clinician to decide on a repeat");
  });

  it("replaces the prompt with the recorded plan, owner and review date, and keeps the case open in Waiting", () => {
    const c = held("out_of_scope", plan);
    expect(statusWords(c)).toEqual({ status: "Clinician plan recorded", next: "Plan: specialist pathway · Dr Synthetic to review by 2026-12-01" });
    expect(c.closed).toBe(false);
    expect(workItem(c, 100, true).view).toBe("waiting");
  });

  it("refuses a plan type the API does not support", () => {
    expect(() => held("out_of_scope", { clinician_disposition: { ...plan.clinician_disposition, plan: "procedure_done" } })).toThrow();
  });
});

describe("clinician disposition in case activity", () => {
  it("names the plan, owner and date and says it is not completed care", () => {
    const words = ledgerWords({
      ledger: "coordinator", type: "coordinator_action", kind: "clinician_disposition", case: "c", role: "clinician", actor: "clinician-1",
      action: "record_clinician_disposition", at: "2026-11-01T00:00:00Z", mode: "demo", prev: "p", hash: "h", head: "h0", seq: 1,
      data: { plan: "repeat_diagnostic", owner: "Dr Synthetic", review_date: "2026-12-01", reason: "synthetic", details: "synthetic", hold: "clinician_decision_no_fundal_view" },
    } as Parameters<typeof ledgerWords>[0]);
    expect(words).toContain("not completed care");
    expect(words).toContain("Dr Synthetic to review by 2026-12-01");
  });
});
