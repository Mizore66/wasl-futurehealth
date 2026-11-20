"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { cancelRecovery, confirmRecovery, type RecoveryResult } from "@/lib/api/recovery-source";
import { RecoveryCancelRequest, RecoveryConfirmRequest } from "@/lib/api/recovery-schema";

const SlotID = z.string().regex(/^rel_[a-f0-9]{24}$/);
const Args = z.object({ slotId: SlotID, input: RecoveryConfirmRequest });
const CancelArgs = z.object({ slotId: SlotID, input: RecoveryCancelRequest });

function revalidateRecovery() {
  revalidatePath("/app/recovery");
  revalidatePath("/app/today");
  revalidatePath("/app/patients/[id]", "page");
}

export async function confirmBackfill(slotId: string, input: unknown): Promise<RecoveryResult> {
  const args = Args.safeParse({ slotId, input });
  if (!args.success) return { ok: false, status: 400, error: "This confirmation could not be read. Try again" };
  const result = await confirmRecovery(args.data.slotId, args.data.input);
  if (result.ok) revalidateRecovery();
  return result;
}

export async function cancelBackfill(slotId: string, input: unknown): Promise<RecoveryResult> {
  const args = CancelArgs.safeParse({ slotId, input });
  if (!args.success) return { ok: false, status: 400, error: "A cancellation reason (up to 200 characters) is required" };
  const result = await cancelRecovery(args.data.slotId, args.data.input);
  if (result.ok) revalidateRecovery();
  return result;
}
