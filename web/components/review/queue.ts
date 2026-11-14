import { getCase, getCases, getCaseWithToken, getFixtures } from "@/lib/api/source";
import { isGuided, type CaseDetail } from "@/lib/api/schema";
import { beforeReview } from "@/lib/people/identity";

export type DetailFailure = { id: string; status: 401 | 403 | 404 };

/**
 * `open`: the API's open review items. `rebuilt`: none are open, so the review_hold case stands in, its signatures removed.
 * `incomplete`: a viewer-token detail read of an open case failed, so `cases` holds only the cases that could be read.
 */
export type ReviewQueue =
  | { cases: CaseDetail[]; source: "open" | "rebuilt" }
  | { cases: CaseDetail[]; source: "incomplete"; failures: DetailFailure[] };

/**
 * Whether the independent protocol has finished with a case. Completion needs every eligible question resolved
 * (agreed or by a senior); one read, even a second one, leaves the case in the queue. Legacy cases carry no states.
 */
export function protocolFinished(detail: Pick<CaseDetail, "api_review_states">): boolean {
  const states = detail.api_review_states ?? [];
  return states.length > 0 && states.every((s) => s.state === "resolved_agreed" || s.state === "resolved_senior");
}

/** Cases waiting for a person's answer in every programme the desk can sign, oldest referral first. Cervical has no answer panel. */
export async function reviewQueue(token?: string): Promise<ReviewQueue> {
  /* The list itself is a shared read; only its per-case projections carry claim actions, and those use the viewer token. */
  const detail = async (id: string): Promise<CaseDetail | DetailFailure | null> => {
    if (!token) return getCase(id);
    const result = await getCaseWithToken(id, token);
    return result.ok ? result.detail : { id, status: result.status };
  };
  const open = await getCases({ review: "open", limit: 50 });
  const guided = open.cases.filter((c) => isGuided(c.programme));
  if (guided.length > 0) {
    const details = await Promise.all(guided.map((c) => detail(c.id)));
    const failures = details.filter((d): d is DetailFailure => d !== null && "status" in d);
    const cases = details
      .filter((d): d is CaseDetail => d !== null && !("status" in d) && d.review_items.length > 0 && !protocolFinished(d))
      .sort((a, b) => a.case.referral_day - b.case.referral_day);
    if (failures.length > 0) return { cases, source: "incomplete", failures };
    if (cases.length > 0) return { cases, source: "open" };
  }
  const hold = await detail((await getFixtures()).fixtures.review_hold.id);
  return { cases: hold && !("status" in hold) ? [beforeReview(hold)] : [], source: "rebuilt" };
}
