import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ProgrammeFindingCard } from "@/components/case/sections";
import { CaseDetail } from "@/lib/api/schema";
import { formatDayYear } from "@/lib/domain/day";
import { shortId } from "@/lib/domain/ids";
import { findingDetail, servedSummary } from "./finding-cases";
import { loadSample } from "./samples";

/*
 * Issue #44: the lung-nodule, DR and breast branches of the clinical-findings card, checked against the served API JSON
 * (docs/api-samples). Each expectation is read from the same summary the card renders, so the card is compared with
 * the API, not with itself. Unknown and pending states must say so, never show an invented value.
 */
const render = (detail: CaseDetail) => renderToStaticMarkup(ProgrammeFindingCard({ detail }));
/** The text of the <dd> after the <dt> named `label`, or null when the row is absent. */
function row(html: string, label: string): string | null {
  const m = new RegExp(`<dt[^>]*>${label}</dt><dd[^>]*>(.*?)</dd>`).exec(html);
  return m ? m[1].replace(/<!-- -->/g, "").replaceAll("&#x27;", "'") : null;
}

describe("lung nodule (Fleischner)", () => {
  it("an open case shows the index finding, Fleischner cell, risk and window, with the follow-up pending", () => {
    const id = "rad-imaging/ServiceRequest/RAD-sr-1";
    const lung = servedSummary("lung_nodule", id).lung!;
    const html = render(findingDetail("lung_nodule", id));
    expect(html).toContain("Incidental lung nodule (Fleischner)");
    expect(row(html, "Index CT report")).toBe(shortId(lung.finding_report!));
    expect(row(html, "Nodule")).toBe(lung.finding_label);
    expect(row(html, "Fleischner category")).toBe(lung.rec!.cell);
    expect(row(html, "Patient risk")).toBe("Low");
    expect(row(html, "Recommendation")).toBe("Follow-up CT");
    expect(row(html, "Risk window")).toBe(
      `${lung.rec!.win_lo_days} to ${lung.rec!.win_hi_days} days, ${formatDayYear(lung.window_start_day!)} to ${formatDayYear(lung.window_end_day!)}`,
    );
    expect(row(html, "Follow-up outcome")).toBe("Pending: no follow-up CT read yet");
    expect(html).toContain(`Next step: ${lung.rec!.next_step}.`);
  });

  it("a closed case shows the follow-up outcome the API read", () => {
    const html = render(findingDetail("lung_nodule", "rad-imaging/ServiceRequest/RAD-sr-101"));
    expect(row(html, "Follow-up outcome")).toBe("Stable");
    expect(row(html, "Patient risk")).toBe("High");
  });

  it("a Fleischner cell with no routine follow-up has no window and needs no outcome", () => {
    const html = render(findingDetail("lung_nodule", "rad-imaging/ServiceRequest/RAD-sr-10"));
    expect(row(html, "Recommendation")).toBe("No routine follow-up");
    expect(row(html, "Risk window")).toBe("No follow-up window");
    expect(row(html, "Follow-up outcome")).toBe("None needed");
  });

  it("marks Fleischner's optional CT as optional", () => {
    const html = render(findingDetail("lung_nodule", "rad-imaging/ServiceRequest/RAD-sr-104"));
    expect(row(html, "Recommendation")).toBe("Follow-up CT (optional)");
  });

  it("a surveillance child with no index finding of its own says so instead of borrowing one", () => {
    const html = render(findingDetail("lung_nodule", "rad-imaging/ServiceRequest/RAD-sr-101#surv"));
    expect(row(html, "Index CT report")).toBe("Not on this case");
    expect(row(html, "Nodule")).toBe("Not read yet");
    expect(row(html, "Fleischner category")).toBe("solid/multiple/high/6-8");
  });
});

describe("diabetic retinopathy", () => {
  it("the served dr_exam_closure detail shows grade, urgency, outcome and next due from the API", () => {
    const detail = CaseDetail.parse(loadSample("case_dr_exam_closure.json"));
    const dr = detail.case.dr!;
    const html = render(detail);
    expect(html).toContain("Diabetic retinopathy screening");
    expect(row(html, "Screening report")).toBe(shortId(dr.screen_report!));
    expect(row(html, "Grade")).toBe(dr.screen_label);
    expect(row(html, "Recommendation")).toBe("Refer to the eye clinic");
    expect(row(html, "Urgency")).toBe("Routine");
    expect(row(html, "Deadline")).toBe(`${dr.rec!.deadline_days} days, due ${formatDayYear(dr.deadline_day!)}`);
    expect(row(html, "Eye-clinic note")).toBe(shortId(dr.clinic_note!));
    expect(row(html, "Outcome")).toBe("Examined, follow-up");
    expect(row(html, "Retina examined")).toBe("Yes");
    expect(row(html, "Within deadline")).toBe("Yes");
    expect(row(html, "Next due")).toBe(`Clinic review ${formatDayYear(dr.next_due!.due_day)} (note review interval)`);
  });

  it("an urgent referral still waiting shows the clinic note and outcome as pending, and no next due yet", () => {
    const html = render(findingDetail("diabetic_retinopathy", "phc-retina/ServiceRequest/PHC-sr-1"));
    expect(row(html, "Grade")).toBe("R2M0/urgent (coded result matched the text)");
    expect(row(html, "Urgency")).toBe("Urgent");
    expect(row(html, "Eye-clinic note")).toBe("Pending: no eye-clinic note read yet");
    expect(row(html, "Outcome")).toBe("Pending");
    expect(row(html, "Retina examined")).toBeNull();
    expect(row(html, "Next due")).toBe("Set once the loop resolves");
  });

  it("a rescreen grade is not a referral and carries the annual rescreen date", () => {
    const id = "phc-retina/ServiceRequest/PHC-sr-10";
    const dr = servedSummary("diabetic_retinopathy", id).dr!;
    const html = render(findingDetail("diabetic_retinopathy", id));
    expect(row(html, "Recommendation")).toBe("Rescreen");
    expect(row(html, "Urgency")).toBe("Not a referral");
    expect(row(html, "Eye-clinic note")).toBeNull();
    expect(row(html, "Outcome")).toBeNull();
    expect(row(html, "Next due")).toBe(`Rescreen ${formatDayYear(dr.next_due!.due_day)} (doh2024dr annual rescreen)`);
  });

  it("a did-not-attend outcome reads as such", () => {
    const html = render(findingDetail("diabetic_retinopathy", "phc-retina/ServiceRequest/PHC-sr-1017"));
    expect(row(html, "Outcome")).toBe("Did not attend");
    expect(row(html, "Urgency")).toBe("Urgent");
  });

  it("a resolved loop without next_due says it is not recorded rather than guessing a date", () => {
    const detail = CaseDetail.parse(loadSample("case_dr_exam_closure.json"));
    const { next_due: _drop, ...dr } = detail.case.dr!;
    void _drop;
    const html = render({ ...detail, case: { ...detail.case, dr }, current: { ...detail.current, dr } });
    expect(row(html, "Next due")).toBe("Not recorded in the API data");
  });
});

describe("breast", () => {
  it("a routine BI-RADS 2 screen needs no recall and no assessment", () => {
    const id = "bsu-screen/ServiceRequest/BSU-sr-1";
    const breast = servedSummary("breast", id).breast!;
    const html = render(findingDetail("breast", id));
    expect(html).toContain("Breast screening (BI-RADS)");
    expect(row(html, "BI-RADS category")).toBe("B2 (coded assessment matched the text)");
    expect(row(html, "Recall recommendation")).toBe("Routine screening, no recall");
    expect(row(html, "Deadline")).toBe(`${breast.rec!.deadline_days} days, due ${formatDayYear(breast.deadline_day!)}`);
    expect(row(html, "Assessment document")).toBeNull();
    expect(row(html, "Outcome")).toBe("None: no recall");
  });

  it("an open recall shows the assessment and its outcome as pending", () => {
    const html = render(findingDetail("breast", "bsu-screen/ServiceRequest/BSU-sr-15"));
    expect(row(html, "BI-RADS category")).toBe("B0 (coded assessment matched the text)");
    expect(row(html, "Recall recommendation")).toBe("Recall for assessment, routine");
    expect(row(html, "Assessment document")).toBe("Pending: none read yet");
    expect(row(html, "Outcome")).toBe("Pending");
    expect(row(html, "Within deadline")).toBeNull();
  });

  it("an assessment outcome the API read is shown with its document and deadline result", () => {
    const detail = findingDetail("breast", "bsu-screen/ServiceRequest/BSU-sr-15");
    const breast = { ...detail.case.breast!, assessment_document: "BAU-br-8", outcome: "return_routine" as const, within_deadline: false };
    const html = render({ ...detail, case: { ...detail.case, breast } });
    expect(row(html, "Assessment document")).toBe("BAU-br-8");
    expect(row(html, "Outcome")).toBe("Return to routine screening");
    expect(row(html, "Within deadline")).toBe("No");
  });

  it("a screen with no category or rule yet says not decided", () => {
    const detail = findingDetail("breast", "bsu-screen/ServiceRequest/BSU-sr-15");
    const breast = { screen_day: detail.case.breast!.screen_day };
    const html = render({ ...detail, case: { ...detail.case, breast } });
    expect(row(html, "Screening report")).toBe("Not received yet");
    expect(row(html, "BI-RADS category")).toBe("Not read yet");
    expect(row(html, "Recall recommendation")).toBe("Not decided yet");
    expect(row(html, "Deadline")).toBe("Not recorded in the API data");
    expect(row(html, "Outcome")).toBe("Not decided yet");
  });
});

describe("no finding object in the API data", () => {
  it.each([
    ["lung_nodule", "rad-imaging/ServiceRequest/RAD-sr-1", "lung", "The API sent no lung-nodule finding for this case yet."],
    ["diabetic_retinopathy", "phc-retina/ServiceRequest/PHC-sr-1", "dr", "The API sent no retinal screening finding for this case yet."],
    ["breast", "bsu-screen/ServiceRequest/BSU-sr-1", "breast", "The API sent no breast screening finding for this case yet."],
  ] as const)("%s renders an honest empty state", (programme, id, key, words) => {
    const detail = findingDetail(programme, id);
    const html = render({ ...detail, case: { ...detail.case, [key]: undefined }, current: { ...detail.current, [key]: undefined } });
    expect(html).toContain(words);
    expect(html).not.toContain("<dl");
  });

  it("other programmes are unchanged: CRC still renders no finding card", () => {
    const detail = CaseDetail.parse(loadSample("case_evidence_closure.json"));
    expect(render(detail)).toBe("");
  });
});
