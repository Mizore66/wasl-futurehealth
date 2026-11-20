import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ReasonCard } from "@/components/case/sections";
import { closureReason } from "@/components/case/words";
import { CaseDetail } from "@/lib/api/schema";
import { caseStage } from "@/lib/domain/stage";
import { loadSample } from "./samples";

type Hold = CaseDetail["holds"][number];

const detail = (file: string) => CaseDetail.parse(loadSample(file));
const text = (d: CaseDetail) => renderToStaticMarkup(ReasonCard({ detail: d })).replace(/<[^>]+>/g, " ").replace(/\s+/g, " ");
const currentList = (d: CaseDetail) => /aria-label="Holds">(.*?)<\/ul>/.exec(renderToStaticMarkup(ReasonCard({ detail: d })))?.[1] ?? "";
const historyList = (d: CaseDetail) => /aria-label="Earlier holds">(.*?)<\/ul>/.exec(renderToStaticMarkup(ReasonCard({ detail: d })))?.[1] ?? "";
const holdEntry = (body: Hold, i: number) => ({ type: "hold_decision", body, prev: `p${i}`, hash: `h${i}` });

const trap = detail("case_false_closure_trap.json");
const trapHold = trap.holds[0];
const holdA: Hold = { ...trapHold, reason: "awaiting_eye_clinic_exam", state: "review_hold", rule: "rule_a" };
const holdB: Hold = { ...trapHold, reason: "scan_report_not_received", state: "review_hold", rule: "rule_b" };

const withHolds = (all: Hold[], current: Hold[]): CaseDetail => ({
  ...trap,
  holds: all,
  current: {
    ...trap.current,
    chain: [...trap.current.chain.filter((e) => e.type !== "hold_decision"), ...current.map(holdEntry)],
  },
});

describe("case holds come from the latest canonical decision", () => {
  it("a closed case shows no current waiting instruction, and keeps its resolved hold as history", () => {
    const d = detail("case_dr_exam_closure.json");
    expect(d.case.closed).toBe(true);
    expect(d.holds.map((h) => h.reason)).toEqual(["awaiting_eye_clinic_exam"]);
    expect(text(d)).toContain("Closed with evidence");
    expect(text(d)).not.toMatch(/Waiting for the eye-clinic exam/);
    expect(currentList(d)).toBe("");
    expect(historyList(d)).toContain(d.holds[0].rule);
    expect(caseStage(d).key).toBe("recorded");
  });

  it("an open case whose old hold left the latest decision is no longer held by it", () => {
    const d = withHolds([trapHold], []);
    expect(closureReason(d).title).not.toBe("Held, not closed");
    expect(caseStage(d).key).not.toBe("gate_held");
    expect(currentList(d)).toBe("");
    expect(historyList(d)).toContain(trapHold.rule);
  });

  it("hold B replacing hold A drives the reason, the hold list and the stage", () => {
    const d = withHolds([holdA, holdB], [holdB]);
    expect(closureReason(d)).toEqual({ title: "Held, not closed", body: "Scan report not received. Rule: rule_b." });
    expect(caseStage(d).technical).toBe("review_hold · rule_b");
    expect(currentList(d)).toContain("rule_b");
    expect(currentList(d)).not.toContain("rule_a");
    expect(historyList(d)).toContain("rule_a");
    expect(historyList(d)).not.toContain("rule_b");
  });

  it("a hold still in the latest decision stays current", () => {
    expect(trap.current.chain.some((e) => e.type === "hold_decision")).toBe(true);
    expect(closureReason(trap).title).toBe("Held, not closed");
    expect(caseStage(trap).key).toBe("gate_held");
    expect(currentList(trap)).toContain(trapHold.rule);
    expect(historyList(trap)).toBe("");
  });
});
