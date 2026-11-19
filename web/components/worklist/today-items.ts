import { cache } from "react";
import { facets } from "@/components/patients/facets";
import { TODAY_DEFAULT, type TodaySelection } from "@/components/worklist/filters";
import { workItem } from "@/components/worklist/model";
import { isActionable, openObligation } from "@/components/worklist/obligation";
import { hasRecord, runClock } from "@/components/worklist/run-clock";
import { getAllCases } from "@/lib/api/all-cases";
import { matches } from "@/lib/api/source";

/**
 * Today's work items, shared by the Today page and the sidebar's queue counts. Closed episodes stay closed; one joins
 * the queue only while it still owes a diabetic eye recall that is due or overdue, so closed cases are read only for
 * that programme. Open cases are read across programmes for the facets.
 */
export const todayItems = cache(async (selection: TodaySelection) => {
  const recalls = selection.programme === undefined || selection.programme === "diabetic_retinopathy";
  const [{ cases: open }, closed, now] = await Promise.all([
    getAllCases({ closed: false }),
    recalls ? getAllCases({ closed: true, programme: "diabetic_retinopathy" }) : null,
    runClock(),
  ]);
  const cases = [...open, ...(closed?.cases.filter((c) => isActionable(openObligation(c, now))) ?? [])];
  const chosen = cases.filter((c) => matches(c, selection));
  const items = chosen.map((c) => workItem(c, now, hasRecord(c)));
  return { cases, items, now, ...facets(cases, selection) };
});

/** The server's count per Today queue (before any call logged in this browser). */
export async function todayQueueCounts(selection: TodaySelection = TODAY_DEFAULT) {
  const { items } = await todayItems(selection);
  const counts = { call: 0, waiting: 0, none: 0 };
  for (const item of items) counts[item.view] += 1;
  return counts;
}
