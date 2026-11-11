import { HonestyLabel } from "@/components/wasl/honesty-label";
import { getDoubleReview, seatConfigured, seatCredentialName } from "@/lib/api/double-review";
import type { CaseDetail, ReadName, Role } from "@/lib/api/schema";
import { apiMode } from "@/lib/api/source";
import { DoubleReviewPanel, QUESTION_ROLE, type DoubleItem, type SeatView } from "./panel";

/** The case's single-question link and evidence items, once each, in queue order: what the API double-reviews. */
export function doubleItems(detail: Pick<CaseDetail, "review_items">): DoubleItem[] {
  const out: DoubleItem[] = [];
  for (const r of detail.review_items) {
    const q = r.kinds.length === 1 ? r.kinds[0] : "";
    if ((q === "link" || q === "evidence") && !out.some((x) => x.item === r.item)) out.push({ item: r.item, question: q });
  }
  return out;
}

export function parseSeat(raw: string | string[] | undefined): ReadName {
  return raw === "r2" || raw === "r3" ? raw : "r1";
}

/**
 * Loads the case's double review with the selected seat's own credential for each question role, then renders the
 * panel. Fixtures mode has no review log to read or append to, so it says so instead.
 */
export async function DoubleReviewSection({ detail, seat, hrefFor }: { detail: CaseDetail; seat: ReadName; hrefFor: Record<ReadName, string> }) {
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
  const views: SeatView[] = await Promise.all(
    roles.map(async (role): Promise<SeatView> => {
      const credential = seatCredentialName(seat, role);
      if (!seatConfigured(seat, role)) return { role, credential, status: "missing" };
      try {
        const state = await getDoubleReview(detail.case.id, seat, role);
        return state ? { role, credential, status: "ok", state } : { role, credential, status: "unavailable", error: "not found" };
      } catch (e) {
        return { role, credential, status: "unavailable", error: e instanceof Error ? e.message : "request failed" };
      }
    }),
  );
  return <DoubleReviewPanel caseId={detail.case.id} programme={detail.case.programme} seat={seat} hrefFor={hrefFor} items={items} views={views} />;
}
