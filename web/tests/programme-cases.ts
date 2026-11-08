import { CaseDetail, CaseList } from "@/lib/api/schema";
import { loadSample } from "./samples";

export type NewProgramme = "lung_ldct" | "ifhas";

/** Complete case details built from the served summaries and the backend's documented evidence shapes. */
export function programmeDetail(programme: NewProgramme): CaseDetail {
  const page = CaseList.parse(loadSample(`cases_${programme}_page1.json`));
  const summary = page.cases.find((c) => (programme === "lung_ldct" ? c.ldct?.outcome : c.ifhas?.outcome));
  if (!summary) throw new Error(`No closed ${programme} sample`);
  const key = programme === "lung_ldct" ? "ldct" : "ifhas";
  const finding = summary[key]!;
  const report = "scan_report" in finding ? finding.scan_report : finding.screen_report;
  const label = "scan_label" in finding ? finding.scan_label : finding.screen_label;
  if (!report || !label) throw new Error(`Incomplete ${programme} sample`);
  const qualifiedReport = `${summary.provider}/DiagnosticReport/${report}`;
  const evidence =
    programme === "lung_ldct"
      ? {
          report: qualifiedReport,
          agree: true,
          label,
          reader_a_regex: label,
          reader_b: label,
          reader_a_fields: { lungrads_overall: "4X", lungrads_worst: "4X", label, quote: "Lung-RADS 4X; suspicious pulmonary nodule." },
          reader_b_fields: { lungrads_overall: "4X", lungrads_worst: "4X", label, quote: "Lung-RADS 4X; suspicious pulmonary nodule." },
        }
      : {
          report: qualifiedReport,
          agree: true,
          label,
          reader_a: label,
          reader_b: label,
          reader_a_fields: "doh2025ifhas printed panel",
          reader_b_fields: "coded LOINC/UCUM results",
        };
  const base = loadSample("case_dr_exam_closure.json") as Record<string, unknown>;
  const current = {
    id: `dec-${summary.id}`,
    loop: summary.id,
    state: summary.state,
    chain: [],
    now: 120,
    prog: programme,
    dropoff: summary.dropoff,
    [key]: finding,
  };
  return CaseDetail.parse({
    ...base,
    case: summary,
    identities: [
      { provider: summary.provider, registration: `${summary.provider}|SCREEN-MRN`, role: "referral" },
      { provider: summary.provider, registration: `${summary.provider}|FOLLOWUP-MRN`, role: "downstream" },
    ],
    timeline: [{ id: qualifiedReport, type: "DiagnosticReport", provider: summary.provider, day: summary.referral_day, category: `${programme}-screen` }],
    links: [],
    evidence: [evidence],
    holds: [],
    outreach: [],
    decisions: [],
    review_items: [],
    signoffs: [],
    api_reviews: [],
    current,
    booking: { status: "not_applicable", data_mode: "simulated" },
  });
}
