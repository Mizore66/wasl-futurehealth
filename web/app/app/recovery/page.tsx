import { RecoveryBoard } from "@/components/recovery/recovery-board";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { getRecoverySlot, getRecoverySlots } from "@/lib/api/recovery-source";
import { apiMode } from "@/lib/api/source";

export const metadata = { title: "Recovery" };

export default async function Page({ searchParams }: PageProps<"/app/recovery">) {
  const query = await searchParams;
  const requested = typeof query.slot === "string" ? query.slot : undefined;
  const list = await getRecoverySlots();
  const selected = requested ?? list.slots.find((slot) => slot.status === "open")?.slot_id;
  const detail = selected ? await getRecoverySlot(selected) : null;
  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="m-0 text-title font-semibold">No-show recovery</h1>
          <p className="mb-0 mt-2 max-w-2xl text-muted">Rank the existing waitlist for projected released capacity. A coordinator must confirm every simulated backfill and outreach intent.</p>
        </div>
        <HonestyLabel kind="simulated">Simulated capacity · decision support only</HonestyLabel>
      </header>
      <RecoveryBoard list={list} detail={detail} readOnly={apiMode() === "fixtures"} />
    </>
  );
}
