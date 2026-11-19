import { slotsByDay } from "@/components/recovery/candidates";
import { RecoveryBoard } from "@/components/recovery/recovery-board";
import { getRecoverySlot, getRecoverySlots } from "@/lib/api/recovery-source";
import { apiMode } from "@/lib/api/source";

export const metadata = { title: "Recovery" };

export default async function Page({ searchParams }: PageProps<"/app/recovery">) {
  const query = await searchParams;
  const requested = typeof query.slot === "string" ? query.slot : undefined;
  const list = await getRecoverySlots();
  // The first open slot; once none is open, the earliest slot, so an assigned backfill stays in view.
  const selected = requested ?? (list.slots.find((slot) => slot.status === "open") ?? slotsByDay(list.slots)[0])?.slot_id;
  const detail = selected ? await getRecoverySlot(selected) : null;
  return <RecoveryBoard key={detail?.slot?.slot_id ?? "none"} list={list} detail={detail} readOnly={apiMode() === "fixtures"} />;
}
