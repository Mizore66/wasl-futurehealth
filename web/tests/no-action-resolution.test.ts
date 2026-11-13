import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ProgrammeFindingCard, ReasonCard } from "@/components/case/sections";
import { caseDeadlineDays, nextScreenDays } from "@/components/worklist/days";
import { closedHeadline, dueColumnWords, recordNodes } from "@/components/worklist/model";
import { CaseDetail, CaseList, type CaseSummary } from "@/lib/api/schema";
import { loadSample } from "./samples";

const PAGES = {
  lung_nodule: "cases_lung_nodule_page1.json",
  diabetic_retinopathy: "cases_diabetic_retinopathy_page1.json",
  breast: "cases_breast_page1.json",
  lung_ldct: "cases_lung_ldct_page1.json",
  ifhas: "cases_ifhas_page1.json",
} as const;
const NO_ACTION = {
  lung_nodule: "no_followup_needed",
  diabetic_retinopathy: "no_referral_needed",
  breast: "no_recall_needed",
  lung_ldct: "no_recall_needed",
  ifhas: "no_followup_needed",
} as const;

const summaryFor = (p: keyof typeof PAGES): CaseSummary => {
  const found = CaseList.parse(loadSample(PAGES[p])).cases.find((c) => c.closed && c.state === NO_ACTION[p]);
  if (!found) throw new Error(`no no-action sample for ${p}`);
  return found;
};
const detailFor = (summary: CaseSummary): CaseDetail =>
  CaseDetail.parse({
    ...(loadSample("case_dr_exam_closure.json") as object),
    case: summary,
    current: { id: `dec-${summary.id}`, loop: summary.id, state: summary.state, chain: [] },
  });

const PROCEDURE_CLAIMS = /colonoscopy|follow-up CT|eye-clinic|assessment|follow-up LDCT|follow-up visit|with evidence, dated to/i;
const text = (el: React.JSX.Element) => renderToStaticMarkup(el).replace(/<[^>]+>/g, " ");

describe("no-action resolutions", () => {
  for (const p of Object.keys(PAGES) as (keyof typeof PAGES)[]) {
    it(`${p} ${NO_ACTION[p]}: the final node and detail claim no completed procedure`, () => {
      const d = detailFor(summaryFor(p));
      const last = recordNodes(d, []).at(-1)!;
      expect(last.what).not.toMatch(PROCEDURE_CLAIMS);
      expect(last.what).toMatch(/no .*needed/i);
      expect(text(ReasonCard({ detail: d }))).not.toMatch(/standard|deadline|inside|past/i);
      expect(closedHeadline(d.case)).not.toMatch(/inside|past|standard|deadline/i);
    });
  }

  it("RAD-sr-10 reads as no routine follow-up with no 31-day clock anywhere", () => {
    const s = summaryFor("lung_nodule");
    expect(s.id).toBe("rad-imaging/ServiceRequest/RAD-sr-10");
    expect(recordNodes(detailFor(s), []).at(-1)!.what).toMatch(/no routine follow-up/i);
    expect(nextScreenDays(s)).toBeNull();
    expect(closedHeadline(s)).not.toContain("31");
    expect(dueColumnWords(s)).not.toMatch(/\d/);
  });

  it("keeps LDCT annual and IFHAS routine intervals as next-screen information", () => {
    const ldct = summaryFor("lung_ldct");
    expect(nextScreenDays(ldct)).toBe(365);
    expect(closedHeadline(ldct)).toContain("next screen in 365 days");
    expect(dueColumnWords(ldct)).toMatch(/^Next screen /);
    const ifhas = summaryFor("ifhas");
    expect(nextScreenDays(ifhas)).toBe(1095);
    const card = text(ProgrammeFindingCard({ detail: detailFor(ifhas) }));
    expect(card).toContain("Next screen");
    expect(card).not.toMatch(/Deadline/);
  });

  it("a closed procedure case keeps its procedure copy, hash, and deadline comparison", () => {
    const d = CaseDetail.parse(loadSample("case_evidence_closure.json"));
    const last = recordNodes(d, [{ type: "closure_decision", hash: "abc" }]).at(-1)!;
    expect(last.what).toContain("with evidence, dated to the colonoscopy");
    expect(last.hash).toBe("abc");
    expect(closedHeadline(d.case)).toMatch(/Closed in 27 days, 4 inside the 31-day standard/);
    expect(nextScreenDays(d.case)).toBeNull();
    expect(caseDeadlineDays(d.case)).toBe(31);
  });
});
