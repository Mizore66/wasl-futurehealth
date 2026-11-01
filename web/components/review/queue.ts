import { getCase, getCases, getFixtures } from "@/lib/api/source";
import type { CaseDetail } from "@/lib/api/schema";

export type ReviewQueue = {
  cases: CaseDetail[];
  /** `open`: the API's open review items. `rebuilt`: none are open, so the review_hold case stands in. */
  source: "open" | "rebuilt";
};

/** Cases waiting for a second signature, oldest referral first. */
export async function reviewQueue(): Promise<ReviewQueue> {
  const open = await getCases({ review: "open", limit: 50 });
  if (open.cases.length > 0) {
    const details = await Promise.all(open.cases.map((c) => getCase(c.id)));
    const cases = details.filter((d): d is CaseDetail => d !== null && d.review_items.length > 0);
    if (cases.length > 0) return { cases: cases.sort((a, b) => a.case.referral_day - b.case.referral_day), source: "open" };
  }
  const hold = await getCase((await getFixtures()).fixtures.review_hold.id);
  return { cases: hold ? [hold] : [], source: "rebuilt" };
}
