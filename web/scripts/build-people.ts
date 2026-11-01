// Builds data/people.json: display names for the registrations of the three CRC demo fixtures (plan gap G1).
// The cast has no one for dr_exam_closure, so that fixture keeps its registration ids.
// The API returns registrations, not names. persons3.json is used if it maps registrations to people;
// otherwise names come from the design's canonical cast and are marked source "canonical-cast".
import { readFileSync, readdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { CaseDetail, CRC_FIXTURES } from "../lib/api/schema";
import type { People, Person } from "../lib/people/types";

const web = path.join(import.meta.dirname, "..");
const samples = path.join(web, "data", "api");
const PERSONS = "data/persons3.json";

/** Canonical cast (design-brief-v2 "Shared synthetic cast" and xcritique-consistency "Canonical facts"). */
const CAST: Record<(typeof CRC_FIXTURES)[number], { name: string; nameAr: string; age: number; recordedDownstream: string }> = {
  evidence_closure: { name: "Fatima Al Zaabi", nameAr: "فاطمة الزعابي", age: 54, recordedDownstream: "Fatima Alzaabi" },
  review_hold: { name: "Mariam Al Shamsi", nameAr: "مريم الشامسي", age: 61, recordedDownstream: "Mariam Al Shamsi" },
  false_closure_trap: { name: "Aisha Al Mansoori", nameAr: "عائشة المنصوري", age: 58, recordedDownstream: "Aisha Al Mansoori" },
};

const persons: Record<string, unknown>[] = JSON.parse(readFileSync(path.join(web, "..", PERSONS), "utf8"));
const mappingKeys = new Set(persons.flatMap((p) => Object.keys(p)).filter((k) => /mrn|registration|loop/i.test(k)));
if (mappingKeys.size > 0) {
  throw new Error(`${PERSONS} now carries ${[...mappingKeys].join(", ")}: map registrations from it instead of the canonical cast`);
}

const people: Record<string, Person> = {};
for (const file of readdirSync(samples).filter((f) => /^case_[a-z_]+\.json$/.test(f) && !f.endsWith("_audit.json"))) {
  const detail = CaseDetail.parse(JSON.parse(readFileSync(path.join(samples, file), "utf8")));
  const fixture = detail.case.fixture;
  if (!fixture || fixture === "dr_exam_closure") continue;
  const cast = CAST[fixture];
  for (const identity of detail.identities) {
    const referral = identity.role === "referral";
    people[identity.registration] = {
      registration: identity.registration,
      provider: identity.provider,
      loop: detail.case.id,
      fixture,
      display_name: cast.name,
      display_name_ar: cast.nameAr,
      age: cast.age,
      recorded_name: referral ? cast.nameAr : cast.recordedDownstream,
      recorded_script: referral ? "arabic" : "latin",
      source: "canonical-cast",
    };
  }
}

const fixtures = new Set(Object.values(people).map((p) => p.fixture));
if (fixtures.size !== 3) throw new Error(`expected 3 fixtures in data/api, found ${[...fixtures].join(", ")}`);

const out: People = {
  label: "simulated",
  note: `The API returns registrations, not names, and ${PERSONS} has no registration mapping. Names are the design's synthetic cast.`,
  people,
};
writeFileSync(path.join(web, "data", "people.json"), JSON.stringify(out, null, 1) + "\n");
console.log(`build-people: ${Object.keys(people).length} registrations across ${fixtures.size} fixtures (canonical-cast)`);
