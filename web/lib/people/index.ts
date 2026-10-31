import data from "@/data/people.json";
import type { People, Person } from "./types";

export const people = data as People;

export function personFor(registration: string): Person | undefined {
  return people.people[registration];
}

export function personForLoop(loop: string): Person | undefined {
  return Object.values(people.people).find((p) => p.loop === loop);
}
