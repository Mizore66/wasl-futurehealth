import { HonestyLabel } from "@/components/wasl/honesty-label";
import { workItem } from "@/components/worklist/model";
import { hasRecord, runClock } from "@/components/worklist/run-clock";
import { TodayBoard } from "@/components/worklist/today-board";
import { getCases } from "@/lib/api/source";

export const metadata = { title: "Today" };

export default async function Page() {
  const [{ cases }, now] = await Promise.all([getCases({ provider: "ahs-mushrif", programme: "crc", limit: 500 }), runClock()]);
  const items = cases.filter((c) => !c.closed).map((c) => workItem(c, now, hasRecord(c)));
  return <TodayBoard items={items} now={now} aside={<HonestyLabel kind="designed">Call log: designed, not built</HonestyLabel>} />;
}
