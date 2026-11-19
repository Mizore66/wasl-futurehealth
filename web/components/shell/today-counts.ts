"use client";

import { useSyncExternalStore } from "react";

/** The three Today queues the sidebar nests under Today. */
export type TodayQueue = "call" | "waiting" | "none";
export type TodayCounts = Record<TodayQueue, number>;

export const TODAY_QUEUES: [TodayQueue, string][] = [
  ["call", "To call"],
  ["waiting", "Waiting"],
  ["none", "No owner"],
];

export function isTodayQueue(v: unknown): v is TodayQueue {
  return v === "call" || v === "waiting" || v === "none";
}

/**
 * Live counts published by the mounted Today board (after this browser's logged actions); the sidebar prefers them
 * over the server's counts, which cannot see browser-kept call logs in fixtures mode.
 */
let live: TodayCounts | null = null;
const listeners = new Set<() => void>();

export function publishTodayCounts(next: TodayCounts | null) {
  if (next && live && next.call === live.call && next.waiting === live.waiting && next.none === live.none) return;
  live = next;
  listeners.forEach((l) => l());
}

export function useLiveTodayCounts(): TodayCounts | null {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => live,
    () => null,
  );
}
