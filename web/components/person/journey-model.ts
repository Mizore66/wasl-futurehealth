import type { NextAction, PersonJourney, PersonObligation } from "@/lib/api/person-schema";
import { personForLoop } from "@/lib/people";

export const programmeLabels: Record<PersonJourney["person"]["programmes"][number], string> = {
  crc: "Colorectal screening",
  cervical: "Cervical screening",
  lung_nodule: "Lung nodule",
  diabetic_retinopathy: "Diabetic retinopathy",
  breast: "Breast screening",
  lung_ldct: "Lung cancer LDCT",
  ifhas: "IFHAS",
};

export const reasonLabels: Record<string, string> = {
  duplicate_outreach_requires_review: "Duplicate or overlapping outreach needs coordinator review.",
  case_priority_urgent: "The canonical case priority is urgent.",
  case_overdue: "A canonical follow-up clock is overdue.",
  consent_not_granted: "Canonical contact permission is not granted; review permission before patient contact.",
  dropoff_no_show: "The canonical worklist records a no-show.",
  dr_recall_overdue: "The canonical diabetic-eye recall is overdue.",
  dr_next_recall_overdue: "The canonical diabetic-eye recall is overdue.",
  programme_follow_up_required: "The canonical worklist needs programme follow-up.",
  no_open_coordinator_action: "No open coordinator action was found.",
};

export function reasonLabel(code: string): string {
  if (reasonLabels[code]) return reasonLabels[code];
  if (code.startsWith("dropoff_")) return `Canonical worklist reason: ${code.slice("dropoff_".length).replaceAll("_", " ")}.`;
  return code.replaceAll("_", " ");
}

export const actionLabels: Record<NextAction["kind"], string> = {
  review_duplicate_outreach: "Review duplicate outreach",
  review_contact_permission: "Review contact permission",
  programme_follow_up: "Programme follow-up",
  none: "No open coordinator action",
};

export function personDisplay(journey: PersonJourney) {
  // /2 IDs are resolved by canonical provenance, never by a fixture's case-name cast.
  const fixture = journey.schema === "wasl-person-journey/1" ? personForLoop(journey.person.anchor_case_id) : undefined;
  return {
    name: fixture?.display_name ?? `Person ${journey.person.id.slice(7, 15)}`,
    arabic: fixture?.display_name_ar,
    synthetic: Boolean(fixture),
  };
}

export function obligationsFor(journey: PersonJourney, caseID: string): PersonObligation[] {
  return journey.obligations.filter((obligation) => obligation.case_id === caseID);
}

export function lifecycleLabel(lifecycle: "released" | "held" | "withheld" | "recorded"): string {
  switch (lifecycle) {
    case "released":
      return "Release recorded — not evidence of dispatch or delivery";
    case "held":
      return "Held for a canonical guard";
    case "withheld":
      return "Withheld; not actionable";
    case "recorded":
      return "Intent recorded";
  }
}

export function contactLabel(contact: string): string {
  return contact.replaceAll("_", " ");
}
