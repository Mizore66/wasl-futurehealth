import { getCase, getCases, getFixtures } from "@/lib/api/source";
import { isGuided, type CaseDetail } from "@/lib/api/schema";
import { beforeReview } from "@/lib/people/identity";

export type ReviewQueue = {
  cases: CaseDetail[];
  /** `open`: the API's open review items. `rebuilt`: none are open, so the review_hold case stands in, its signatures removed. */
  source: "open" | "rebuilt";
};

/** Cases waiting for a person's answer in every programme the desk can sign, oldest referral first. Cervical has no answer panel. */
export async function reviewQueue(): Promise<ReviewQueue> {
  const open = await getCases({ review: "open", limit: 50 });
  const guided = open.cases.filter((c) => isGuided(c.programme));
  if (guided.length > 0) {
    const details = await Promise.all(guided.map((c) => getCase(c.id)));
    const cases = details.filter((d): d is CaseDetail => d !== null && d.review_items.length > 0);
    if (cases.length > 0) return { cases: cases.sort((a, b) => a.case.referral_day - b.case.referral_day), source: "open" };
  }
  const hold = await getCase((await getFixtures()).fixtures.review_hold.id);
  return { cases: hold ? [beforeReview(hold)] : [], source: "rebuilt" };
}
