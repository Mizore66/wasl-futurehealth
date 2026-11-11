import { HonestyLabel } from "@/components/wasl/honesty-label";
import { workItem } from "@/components/worklist/model";
import { hasRecord, runClock } from "@/components/worklist/run-clock";
import { TodayBoard } from "@/components/worklist/today-board";
import { apiMode, getCases } from "@/lib/api/source";

export const metadata = { title: "Today" };

export default async function Page() {
  const [{ cases }, now] = await Promise.all([getCases({ provider: "ahs-mushrif", programme: "crc", limit: 500 }), runClock()]);
  const items = cases.filter((c) => !c.closed).map((c) => workItem(c, now, hasRecord(c)));
  const live = apiMode() === "live";
  const aside = live ? (
    <HonestyLabel kind="built">Call log: saved to the case ledger</HonestyLabel>
  ) : (
    <HonestyLabel kind="illustrative">Call log: kept in this browser (no live API)</HonestyLabel>
  );
  return <TodayBoard items={items} now={now} aside={aside} live={live} />;
}
