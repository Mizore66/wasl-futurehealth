import { HonestyLabel } from "@/components/wasl/honesty-label";
import { parseTodaySelection } from "@/components/worklist/filters";
import { TodayBoard } from "@/components/worklist/today-board";
import { scopeWords, TodayScope } from "@/components/worklist/today-scope";
import { apiMode } from "@/lib/api/source";
import { todayItems } from "@/components/worklist/today-items";

export const metadata = { title: "Today" };

export default async function Page({ searchParams }: PageProps<"/app/today">) {
  const selection = parseTodaySelection(await searchParams);
  const { items, now, programme, provider } = await todayItems(selection);
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
