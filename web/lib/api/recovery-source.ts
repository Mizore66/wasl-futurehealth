import { readFile } from "node:fs/promises";
import path from "node:path";
import { apiMode, authenticated, liveBase } from "./source";
import {
  RecoveryConfirmRequest,
  RecoveryConfirmResponse,
  RecoveryDetail,
  RecoveryError,
  RecoveryList,
  type RecoveryConfirmRequest as ConfirmInput,
} from "./recovery-schema";

const fixture = async <T>(name: string, schema: { parse: (value: unknown) => T }): Promise<T> =>
  schema.parse(JSON.parse(await readFile(path.join(process.cwd(), "data", "api", name), "utf8")));

export async function getRecoverySlots(programme?: "crc" | "cervical"): Promise<RecoveryList> {
  if (apiMode() === "fixtures") return fixture("recovery_slots.json", RecoveryList);
  const query = programme ? `?programme=${programme}` : "";
  const response = await fetch(`${liveBase()}/api/recovery/slots${query}`, authenticated(undefined, "web_reviewer"));
  if (!response.ok) throw new Error(`GET recovery slots: HTTP ${response.status}`);
  return RecoveryList.parse(await response.json());
}

export async function getRecoverySlot(id: string): Promise<RecoveryDetail | null> {
  if (apiMode() === "fixtures") {
    const detail = await fixture("recovery_slot.json", RecoveryDetail);
    return detail.slot.slot_id === id ? detail : null;
  }
  const response = await fetch(`${liveBase()}/api/recovery/slots/${encodeURIComponent(id)}`, authenticated(undefined, "web_reviewer"));
  if (response.status === 404) return null;
  if (!response.ok) throw new Error(`GET recovery slot: HTTP ${response.status}`);
  return RecoveryDetail.parse(await response.json());
}

export type RecoveryResult =
  | { ok: true; status: 200 | 201; response: RecoveryConfirmResponse }
  | { ok: false; status: number; error: string; conflict?: string };

/** The API derives coordinator identity from its token; no actor or role is accepted from this client. */
export async function confirmRecovery(slotID: string, input: ConfirmInput): Promise<RecoveryResult> {
  const request = RecoveryConfirmRequest.safeParse(input);
  if (!request.success) return { ok: false, status: 400, error: "Malformed recovery confirmation" };
  if (apiMode() !== "live") return { ok: false, status: 503, error: "Fixture mode is read-only; no backfill was saved" };
  const response = await fetch(
    `${liveBase()}/api/recovery/slots/${encodeURIComponent(slotID)}/confirm`,
    authenticated(
      { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(request.data) },
      "coordinator",
    ),
  );
  const json = await response.json().catch(() => undefined);
  if (response.status === 200 || response.status === 201) {
    return { ok: true, status: response.status, response: RecoveryConfirmResponse.parse(json) };
  }
  if ([400, 401, 403, 404, 409, 500, 503].includes(response.status)) {
    const error = RecoveryError.safeParse(json);
    return { ok: false, status: response.status, error: error.success ? error.data.error : `HTTP ${response.status}`, conflict: error.success ? error.data.conflict : undefined };
  }
  throw new Error(`POST recovery confirmation: HTTP ${response.status}`);
}
