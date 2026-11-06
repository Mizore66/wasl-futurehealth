import fixtures from "../data/api/fixtures.json";

const id = (name: keyof typeof fixtures.fixtures) => encodeURIComponent(fixtures.fixtures[name].id);

const FIXTURE_NAMES = ["evidence_closure", "false_closure_trap", "review_hold", "dr_exam_closure"] as const;

/** Every route in docs/FRONTEND-PLAN.md "Routes", with a fixture id for the dynamic ones. */
export const ROUTES = [
  "/",
  "/how-it-works",
  "/experts",
  "/app",
  "/app/today",
  "/app/review",
  `/app/review/${id("review_hold")}`,
  "/app/patients",
  `/app/patients/${id("evidence_closure")}`,
  `/app/patients/${id("dr_exam_closure")}`,
  `/app/patients/${id("false_closure_trap")}`,
  `/app/patients/${id("review_hold")}`,
  ...FIXTURE_NAMES.map((n) => `/app/patients/${id(n)}/audit`),
  "/app/referrals",
  "/app/pipeline",
  "/app/pipeline/matching",
  "/app/pipeline/reading",
  "/app/results",
  "/patient/notice",
  "/patient/check",
  "/patient/book",
] as const;

export function screenName(route: string): string {
  return route === "/" ? "home" : route.slice(1).replace(/%2F/gi, "~").replaceAll("/", "_");
}
