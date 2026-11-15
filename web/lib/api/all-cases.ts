import { getCases, type CaseQuery } from "@/lib/api/source";
import type { CaseList } from "@/lib/api/schema";

const PAGE = 500;

/** Every case matching the filters, fetched page by page in the API's stable ID order. */
export async function getAllCases(query: Omit<CaseQuery, "limit" | "offset"> = {}): Promise<CaseList> {
  let first: CaseList | undefined;
  const cases: CaseList["cases"] = [];
  for (let offset = 0; ; offset = cases.length) {
    const page = await getCases({ ...query, limit: PAGE, offset });
    first ??= page;
    cases.push(...page.cases);
    if (page.cases.length === 0 || cases.length >= page.total) return { ...first, cases, total: cases.length, limit: cases.length, offset: 0 };
  }
}
