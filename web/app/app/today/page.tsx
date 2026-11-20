import { PageHeader } from "@/components/shell/page-header";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { parseTodayQueue, parseTodaySelection } from "@/components/worklist/filters";
import { TodayBoard } from "@/components/worklist/today-board";
import { scopeWords, TodayScope } from "@/components/worklist/today-scope";
import { apiMode } from "@/lib/api/source";
import { simulatedRunDate } from "@/lib/domain/staff-time";
import { todayItems } from "@/components/worklist/today-items";

export const metadata = { title: "Today" };

export default async function Page({ searchParams }: PageProps<"/app/today">) {
  const sp = await searchParams;
  const selection = parseTodaySelection(sp);
  const queue = parseTodayQueue(sp);
  const { items, now, programme, provider } = await todayItems(selection);
  const live = apiMode() === "live";
  const honesty = live ? (
    <HonestyLabel kind="built">Call log: saved to the case ledger</HonestyLabel>
  ) : (
    <HonestyLabel kind="illustrative">Call log: kept in this browser (no live API)</HonestyLabel>
  );
  const scope = scopeWords(selection);
  return (
    <>
      <PageHeader
        title="Today"
        sub={`${scope} · ${simulatedRunDate(now)}`}
        // The scope menu opens below the band, so the band must not clip it (and stacks above the board).
        className="relative z-10 overflow-visible!"
        aside={<TodayScope selection={selection} options={{ programme, provider }} count={items.length} queue={queue} />}
      />
      {/* Keyed by scope only: switching queue keeps this browser's logged actions. */}
      <TodayBoard key={scope} items={items} now={now} aside={honesty} live={live} scope={scope} queue={queue} />
    </>
  );
}
