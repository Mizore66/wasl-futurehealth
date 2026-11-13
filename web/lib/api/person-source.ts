import { readFile } from "node:fs/promises";
import path from "node:path";
import { apiMode, ApiStatusError, authenticated, liveBase } from "./source";
import { PersonJourney } from "./person-schema";

const FIXTURE_FILE = path.join(process.cwd(), "data", "api", "person_journey.json");

async function fixturePerson(): Promise<PersonJourney> {
  return PersonJourney.parse(JSON.parse(await readFile(FIXTURE_FILE, "utf8")));
}

async function livePerson(route: string): Promise<PersonJourney | null> {
  const response = await fetch(liveBase() + route, authenticated(undefined, "web_reviewer"));
  if (response.status === 404) return null;
  if (!response.ok) throw new ApiStatusError(route, response.status);
  return PersonJourney.parse(await response.json());
}

/** Resolve membership on the server; a case ID is never used as a person identifier. */
export async function getPersonByCase(caseID: string): Promise<PersonJourney | null> {
  if (apiMode() === "live") return livePerson(`/api/people?case_id=${encodeURIComponent(caseID)}`);
  const journey = await fixturePerson();
  return journey.person.case_ids.includes(caseID) ? journey : null;
}

export async function getPerson(personID: string): Promise<PersonJourney | null> {
  if (apiMode() === "live") return livePerson(`/api/people/${encodeURIComponent(personID)}`);
  const journey = await fixturePerson();
  return journey.person.id === personID ? journey : null;
}
