import { HonestyLabel } from "@/components/wasl/honesty-label";
import { deskConfigured, deskCredentialName, getDoubleReview } from "@/lib/api/double-review";
import type { CaseDetail, Role } from "@/lib/api/schema";
import { apiMode } from "@/lib/api/source";
import { ownView } from "./model";
import { DoubleReviewPanel, QUESTION_ROLE, type DeskView, type DoubleItem } from "./panel";

/** The case's single-question link and evidence items, once each, in queue order: what the API double-reviews. */
export function doubleItems(detail: Pick<CaseDetail, "review_items">): DoubleItem[] {
  const out: DoubleItem[] = [];
  for (const r of detail.review_items) {
    const q = r.kinds.length === 1 ? r.kinds[0] : "";
    if ((q === "link" || q === "evidence") && !out.some((x) => x.item === r.item)) out.push({ item: r.item, question: q });
  }
  return out;
}

/**
 * Loads the case's double review with this deployment's one credential for each question role, then renders the panel.
 * There is no seat to choose: the page never reads or answers with any other credential. Fixtures mode has no review
 * log to read or append to, so it says so instead.
 */
export async function DoubleReviewSection({ detail }: { detail: CaseDetail }) {
  const items = doubleItems(detail);
  if (items.length === 0) return null;
  if (apiMode() !== "live") {
    return (
      <section id="double-review" aria-labelledby="double-review-h" className="min-w-0 rounded-card border border-line bg-surface p-4 md:p-pad">
        <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
          <h2 id="double-review-h" className="m-0 text-section font-medium">
            Independent double review
          </h2>
          <HonestyLabel kind="built">Built in the live API only</HonestyLabel>
        </div>
        <p className="m-0 mt-1 text-small text-muted">
          Blinded reads by two different people and a senior read after a disagreement need the live API. This snapshot has no review log to
          read or append to.
        </p>
      </section>
    );
  }
  const roles = [...new Set(items.map((i) => QUESTION_ROLE[i.question]))] as Role[];
  const views: DeskView[] = await Promise.all(
    roles.map(async (role): Promise<DeskView> => {
      const credential = deskCredentialName(role);
      if (!deskConfigured(role)) return { role, credential, status: "missing" };
      try {
        const state = await getDoubleReview(detail.case.id, role);
        // Only this deployment's own blinded answer survives ownView; nothing else reaches the render tree.
        return state ? { role, credential, status: "ok", state: ownView(state) } : { role, credential, status: "unavailable", error: "not found" };
      } catch (e) {
        return { role, credential, status: "unavailable", error: e instanceof Error ? e.message : "request failed" };
      }
    }),
  );
  return <DoubleReviewPanel caseId={detail.case.id} programme={detail.case.programme} items={items} views={views} />;
}
