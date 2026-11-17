import type { ReviewAuthMode } from "@/lib/auth/oidc-config";
import { GovernanceIdentity, GovernanceSignIn } from "./governance-controls";
import { PatientContestControls } from "./patient-contest-controls";
import type { PatientContestReview } from "./patient-journey-card";

/** The viewer's own reviewer session, as resolved on the server. Never the shared web credential. */
export type ContestSession = { live: boolean; signedIn: boolean; mode?: ReviewAuthMode; expiresAt?: string };

/**
 * Live slots for `PatientRequestsCard` (#167). Kept apart from the card so that rendering the card, for example in a
 * snapshot, never loads the server actions. Controls exist only for a signed-in viewer; otherwise the card asks for a
 * personal sign-in.
 */
export function patientContestReview(caseId: string, session: ContestSession): PatientContestReview {
  if (!session.live) return { live: false };
  if (!session.signedIn) return { live: true, identity: <GovernanceSignIn caseId={caseId} mode={session.mode} /> };
  return {
    live: true,
    identity: <GovernanceIdentity caseId={caseId} mode={session.mode} expiresAt={session.expiresAt} />,
    controls: (receipt, excluded) => <PatientContestControls caseId={caseId} contest={receipt} excluded={excluded} />,
  };
}
