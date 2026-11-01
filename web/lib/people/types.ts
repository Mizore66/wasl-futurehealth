import type { FixtureName } from "@/lib/api/schema";

export type Sex = "female" | "male";

/** The person the system believes in, as an avatar draws them. `key` is a person key, never a record id. */
export interface PersonSeed {
  key: string;
  sex?: Sex;
}

export type Person = {
  registration: string;
  provider: string;
  loop: string;
  fixture: FixtureName;
  display_name: string;
  display_name_ar: string;
  age: number;
  /** Present only when the source data records it; avatars never guess it from a name. */
  sex?: Sex;
  /** The name as this provider's record writes it (Arabic at the referrer, Latin downstream). */
  recorded_name: string;
  recorded_script: "arabic" | "latin";
  source: "persons3" | "canonical-cast";
};

export type People = { label: "simulated"; note: string; people: Record<string, Person> };
