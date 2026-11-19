"use client";

import { useSyncExternalStore } from "react";
import type { ReferralReturnReason } from "@/lib/api/schema";

/*
 * Fixtures mode only (no live API): the receiving unit's returns and the GP's acknowledgements, kept in this browser's
 * sessionStorage so the inbox and the GP's returned-referral page can show the same hand-off. Nothing here reaches the
 * API, another browser or the GP; closing the tab drops it. Live mode writes the case ledger instead and never reads this.
 */

/** A return made in this browser session, with what the GP view needs to show it. */
export type SessionReturn = {
  id: string;
  name: string;
  short: string;
  clinic: string;
  unit: string;
  reason: ReferralReturnReason;
  /** The unit's note to the GP, trimmed; absent when none was written. */
  note?: string;
  urgent: boolean;
  /** The run day the GP must act by. */
  actBy: number;
  /** When the GP acknowledged it in this browser (ISO time); absent until then. */
  acknowledgedAt?: string;
};

export type SessionReturns = Readonly<Record<string, SessionReturn>>;

const KEY = "wasl.referrals.session-returns.v1";
const EMPTY: SessionReturns = Object.freeze({});
const listeners = new Set<() => void>();
let cache: { raw: string | null; value: SessionReturns } = { raw: null, value: EMPTY };

function storage(): Storage | null {
  try {
    return globalThis.sessionStorage ?? null;
  } catch {
    return null; // storage disabled: behave as an empty session
  }
}

const isReturn = (v: unknown): v is SessionReturn =>
  !!v && typeof v === "object" && typeof (v as SessionReturn).id === "string" && typeof (v as SessionReturn).reason === "string" && typeof (v as SessionReturn).actBy === "number";

/** The returns kept in this browser; the same object until the stored value changes (a stable snapshot). */
export function readSessionReturns(): SessionReturns {
  const raw = storage()?.getItem(KEY) ?? null;
  if (raw === cache.raw) return cache.value;
  let value: SessionReturns = EMPTY;
  try {
    const parsed: unknown = raw ? JSON.parse(raw) : null;
    if (parsed && typeof parsed === "object") value = Object.fromEntries(Object.entries(parsed).filter(([k, v]) => isReturn(v) && v.id === k));
  } catch {
    value = EMPTY; // a corrupt entry is dropped, never shown
  }
  cache = { raw, value };
  return value;
}

function write(next: SessionReturns) {
  const s = storage();
  if (!s) return;
  try {
    s.setItem(KEY, JSON.stringify(next));
  } catch {
    return; // quota or privacy mode: nothing is kept, so nothing is claimed
  }
  listeners.forEach((l) => l());
}

export function saveSessionReturn(r: SessionReturn) {
  write({ ...readSessionReturns(), [r.id]: r });
}

export function removeSessionReturn(id: string) {
  const { [id]: _gone, ...rest } = readSessionReturns();
  void _gone;
  write(rest);
}

/** The GP acknowledges a return kept in this browser. A second acknowledgement keeps the first time. */
export function acknowledgeSessionReturn(id: string, at = new Date().toISOString()) {
  const all = readSessionReturns();
  const r = all[id];
  if (!r || r.acknowledgedAt) return;
  write({ ...all, [id]: { ...r, acknowledgedAt: at } });
}

function subscribe(onChange: () => void) {
  listeners.add(onChange);
  const onStorage = (e: StorageEvent) => {
    if (e.key === null || e.key === KEY) onChange();
  };
  globalThis.addEventListener?.("storage", onStorage);
  return () => {
    listeners.delete(onChange);
    globalThis.removeEventListener?.("storage", onStorage);
  };
}

/** The returns kept in this browser; empty on the server and during hydration. */
export function useSessionReturns(): SessionReturns {
  return useSyncExternalStore(subscribe, readSessionReturns, () => EMPTY);
}
