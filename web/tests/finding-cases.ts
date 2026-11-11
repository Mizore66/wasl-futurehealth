import { CaseDetail, CaseList, type CaseSummary } from "@/lib/api/schema";
import { loadSample } from "./samples";

export type FindingProgramme = "lung_nodule" | "diabetic_retinopathy" | "breast";

const PAGE: Record<FindingProgramme, string> = {
  lung_nodule: "cases_lung_nodule_page1.json",
  diabetic_retinopathy: "cases_diabetic_retinopathy_page1.json",
  breast: "cases_breast_page1.json",
};
const KEY = { lung_nodule: "lung", diabetic_retinopathy: "dr", breast: "breast" } as const;

/** One served case summary, by id, from the programme's committed API page (docs/api-samples). */
export function servedSummary(programme: FindingProgramme, id: string): CaseSummary {
  const summary = CaseList.parse(loadSample(PAGE[programme])).cases.find((c) => c.id === id);
  if (!summary) throw new Error(`${id} is not on the served ${programme} page`);
  return summary;
}

/**
 * A case detail whose `case` and `current` finding are exactly a served summary's, on the served DR fixture's detail
 * shape. Only the finding card reads them; the snapshot has no lung-nodule or breast case detail.
 */
export function findingDetail(programme: FindingProgramme, id: string): CaseDetail {
  const summary = servedSummary(programme, id);
  const base = loadSample("case_dr_exam_closure.json") as Record<string, unknown>;
  const key = KEY[programme];
  return CaseDetail.parse({
    ...base,
    case: summary,
    current: { id: `dec-${summary.id}`, loop: summary.id, state: summary.state, chain: [], [key]: summary[key] },
  });
}
