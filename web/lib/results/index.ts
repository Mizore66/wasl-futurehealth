import data from "@/data/results.json";
import type { ResultEntry, Results } from "./types";

export const results = data as unknown as Results;

/** Every number a screen shows comes through here; an unknown id is a build-time bug, not a blank. */
export function result(id: string): ResultEntry {
  const entry = results.entries[id];
  if (!entry) throw new Error(`results.json has no entry "${id}"`);
  return entry;
}
