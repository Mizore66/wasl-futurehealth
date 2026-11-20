import { getCases, type CaseQuery } from "@/lib/api/source";
import type { CaseList } from "@/lib/api/schema";

const PAGE = 500;

/**
 * Every case matching the filters, in the API's stable ID order. The first page gives the total; the remaining pages
 * are then read in parallel instead of one after another. If the total moved underneath (a page came back short or
 * the list grew), it falls back to reading on sequentially from where the parallel pages ended.
 */
export async function getAllCases(query: Omit<CaseQuery, "limit" | "offset"> = {}): Promise<CaseList> {
  const first = await getCases({ ...query, limit: PAGE, offset: 0 });
  const cases: CaseList["cases"] = [...first.cases];
  if (first.cases.length > 0 && cases.length < first.total) {
    const offsets: number[] = [];
    for (let offset = PAGE; offset < first.total; offset += PAGE) offsets.push(offset);
    const pages = await Promise.all(offsets.map((offset) => getCases({ ...query, limit: PAGE, offset })));
    for (const page of pages) cases.push(...page.cases);
  }
  for (let offset = cases.length; cases.length < first.total; offset = cases.length) {
    const page = await getCases({ ...query, limit: PAGE, offset });
    if (page.cases.length === 0) break;
    cases.push(...page.cases);
  }
  return { ...first, cases, total: cases.length, limit: cases.length, offset: 0 };
}
