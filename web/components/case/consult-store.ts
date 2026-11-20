"use client";

import { useSyncExternalStore } from "react";
import { isSpecialty, type Consult } from "./consult-model";

/*
 * Consults asked from the case page, kept in this browser's sessionStorage (the same pattern as
 * worklist/session-returns.ts) so the case page and Today show the same consult. The API has no consult action yet:
 * nothing here reaches the API, the specialist or another browser, and closing the tab drops it.
 */

const KEY = "wasl.case.consults.v1";
const EMPTY: readonly Consult[] = Object.freeze([]);
const listeners = new Set<() => void>();
let cache: { raw: string | null; value: readonly Consult[] } = {
  raw: null,
  value: EMPTY,
};

function storage(): Storage | null {
  try {
    return globalThis.sessionStorage ?? null;
  } catch {
    return null;
  }
}

const isConsult = (v: unknown): v is Consult => {
  const c = v as Consult;
  return (
    !!c &&
    typeof c === "object" &&
    typeof c.id === "string" &&
    typeof c.caseId === "string" &&
    isSpecialty(c.specialty) &&
    typeof c.question === "string" &&
    typeof c.askedDay === "number" &&
    typeof c.replyBy === "number"
  );
};

export function readConsults(): readonly Consult[] {
  const raw = storage()?.getItem(KEY) ?? null;
  if (raw === cache.raw) return cache.value;
  let value: readonly Consult[] = EMPTY;
  try {
    const parsed: unknown = raw ? JSON.parse(raw) : null;
    if (Array.isArray(parsed)) value = parsed.filter(isConsult);
  } catch {
    value = EMPTY; // a corrupt entry is dropped, never shown
  }
  cache = { raw, value };
  return value;
}

function write(next: readonly Consult[]) {
  const s = storage();
  if (!s) return;
  try {
    s.setItem(KEY, JSON.stringify(next));
  } catch {
    return;
  }
  listeners.forEach((l) => l());
}

export function saveConsult(c: Consult) {
  write([...readConsults().filter((x) => x.id !== c.id), c]);
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

/** Consults kept in this browser; empty on the server and during hydration. */
export function useConsults(): readonly Consult[] {
  return useSyncExternalStore(subscribe, readConsults, () => EMPTY);
}
