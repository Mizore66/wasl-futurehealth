"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { confirmRecovery, type RecoveryResult } from "@/lib/api/recovery-source";
import { RecoveryConfirmRequest } from "@/lib/api/recovery-schema";

const Args = z.object({ slotId: z.string().regex(/^rel_[a-f0-9]{24}$/), input: RecoveryConfirmRequest });

export async function confirmBackfill(slotId: string, input: unknown): Promise<RecoveryResult> {
  const args = Args.safeParse({ slotId, input });
  if (!args.success) return { ok: false, status: 400, error: "Malformed recovery confirmation" };
  const result = await confirmRecovery(args.data.slotId, args.data.input);
  if (result.ok) {
    revalidatePath("/app/recovery");
    revalidatePath("/app/today");
    revalidatePath("/app/patients/[id]", "page");
  }
  return result;
}
