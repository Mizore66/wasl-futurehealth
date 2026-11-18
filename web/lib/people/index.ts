import data from "@/data/people.json";
import type { People, Person } from "./types";

export const people = data as People;

export function personFor(registration: string): Person | undefined {
  return people.people[registration];
}

export function personForLoop(loop: string): Person | undefined {
  return Object.values(people.people).find((p) => p.loop === loop);
}

/**
 * Whether a case's staff screens show a simulated name. Only fixture loops have one; a live case is identified by its
 * case ID alone, so it must not carry a "Simulated names" label (the person journey applies the same rule).
 */
export function showsSimulatedName(loop: string): boolean {
  return personForLoop(loop) !== undefined;
}
