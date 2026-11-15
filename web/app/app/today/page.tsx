import { facets } from "@/components/patients/facets";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { parseTodaySelection } from "@/components/worklist/filters";
import { workItem } from "@/components/worklist/model";
import { isActionable, openObligation } from "@/components/worklist/obligation";
import { hasRecord, runClock } from "@/components/worklist/run-clock";
import { TodayBoard } from "@/components/worklist/today-board";
import { scopeWords, TodayScope } from "@/components/worklist/today-scope";
import { getAllCases } from "@/lib/api/all-cases";
import { apiMode, matches } from "@/lib/api/source";

export const metadata = { title: "Today" };

export default async function Page({ searchParams }: PageProps<"/app/today">) {
  const selection = parseTodaySelection(await searchParams);
  // Closed episodes stay closed; one joins the queue only while it still owes a recall that is due or overdue.
  const [{ cases: all }, now] = await Promise.all([getAllCases(), runClock()]);
  const cases = all.filter((c) => !c.closed || isActionable(openObligation(c, now)));
  const chosen = cases.filter((c) => matches(c, selection));
  const items = chosen.map((c) => workItem(c, now, hasRecord(c)));
  const { programme, provider } = facets(cases, selection);
  const live = apiMode() === "live";
  const aside = live ? (
    <HonestyLabel kind="built">Call log: saved to the case ledger</HonestyLabel>
  ) : (
    <HonestyLabel kind="illustrative">Call log: kept in this browser (no live API)</HonestyLabel>
  );
  return (
    <>
      <TodayScope selection={selection} options={{ programme, provider }} count={items.length} />
      <TodayBoard key={scopeWords(selection)} items={items} now={now} aside={aside} live={live} scope={scopeWords(selection)} />
    </>
  );
}
