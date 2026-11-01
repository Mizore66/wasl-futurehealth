import { cache } from "react";
import { apiMode, getCase, getFixtures } from "@/lib/api/source";
import type { CaseSummary } from "@/lib/api/schema";

/**
 * The API carries no wall clock. Each case's `current.now` is the run day of its latest decision;
 * the run's clock is the latest of those across the fixtures.
 */
export const runClock = cache(async (): Promise<number> => {
  const fx = await getFixtures();
  const details = await Promise.all(Object.values(fx.fixtures).map((c) => getCase(c.id)));
  const nows = details.map((d) => d?.current.now).filter((n): n is number => typeof n === "number");
  if (nows.length === 0) throw new Error("run clock: no fixture carries current.now");
  return Math.max(...nows);
});

/** Fixtures mode only has detail records for the three fixtures; the live API has one for every case. */
export function hasRecord(c: CaseSummary): boolean {
  return apiMode() === "live" || c.fixture !== "";
}
