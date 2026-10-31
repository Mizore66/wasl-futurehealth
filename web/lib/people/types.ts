import type { FixtureName } from "@/lib/api/schema";

export type Person = {
  registration: string;
  provider: string;
  loop: string;
  fixture: FixtureName;
  display_name: string;
  display_name_ar: string;
  age: number;
  /** The name as this provider's record writes it (Arabic at the referrer, Latin downstream). */
  recorded_name: string;
  recorded_script: "arabic" | "latin";
  source: "persons3" | "canonical-cast";
};

export type People = { label: "simulated"; note: string; people: Record<string, Person> };
