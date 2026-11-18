import type { CaseDetail, OutcomeHandoff } from "@/lib/api/schema";
import { OUTCOME_NEXT_STEPS } from "@/lib/api/patient-schema";
import { resolveReviewer } from "@/lib/api/reviewer-session";
import { apiMode } from "@/lib/api/source";
import { reviewAuthMode, type ReviewAuthMode } from "@/lib/auth/oidc-config";
import { GovernanceIdentity, GovernanceSignIn } from "./governance-controls";
import { OutcomeHandoffCard, type OutcomeHandoffActions } from "./outcome-handoff-card";
import { OutcomeHandoffApproveForm, OutcomeHandoffReleaseButton } from "./outcome-handoff-controls";

/** The viewer's own reviewer session, resolved on the server. Never the shared web credential. */
export type HandoffSession = { live: boolean; signedIn: boolean; mode?: ReviewAuthMode; expiresAt?: string };

/**
 * Staff step slots for `OutcomeHandoffCard`. Approval needs the viewer's own signed-in session (the API allows only a
 * clinician); without one the card asks for a personal sign-in. Release uses the server's coordinator credential and
 * appears once an approved version exists. Nothing is offered for a case that is not eligible.
 */
export function outcomeHandoffActions(caseId: string, handoff: OutcomeHandoff, session: HandoffSession): OutcomeHandoffActions {
  if (!session.live) return { live: false };
  if (handoff.status === "not_eligible" || !handoff.draft) return { live: true };
  const nextSteps = (handoff.next_steps ?? [...OUTCOME_NEXT_STEPS]).filter((s): s is (typeof OUTCOME_NEXT_STEPS)[number] =>
    (OUTCOME_NEXT_STEPS as readonly string[]).includes(s),
  );
  const version = handoff.version ?? "";
  return {
    live: true,
    identity: session.signedIn ? (
      <GovernanceIdentity caseId={caseId} mode={session.mode} expiresAt={session.expiresAt} acts="Approval acts" />
    ) : (
      <GovernanceSignIn caseId={caseId} mode={session.mode} purpose="approve the outcome hand-off as a clinician" acts="Approval acts" />
    ),
    approve: session.signedIn ? (
      <OutcomeHandoffApproveForm caseId={caseId} decision={handoff.draft.decision} version={version} nextSteps={nextSteps} current={handoff.content?.next_step} />
    ) : undefined,
    release: version ? <OutcomeHandoffReleaseButton caseId={caseId} version={version} again={!!handoff.release} /> : undefined,
  };
}

/**
 * The case page's outcome hand-off card with its staff steps. Synchronous unless a reviewer session must be read (a live,
 * eligible case), so pages and tests that render without a session never suspend here.
 */
export function OutcomeHandoffSection({ detail }: { detail: CaseDetail }) {
  const handoff = detail.outcome_handoff;
  if (!handoff) return null;
  const live = apiMode() === "live";
  if (!live || handoff.status === "not_eligible") {
    return <OutcomeHandoffCard handoff={handoff} actions={outcomeHandoffActions(detail.case.id, handoff, { live, signedIn: false })} />;
  }
  return <LiveOutcomeHandoff caseId={detail.case.id} handoff={handoff} />;
}

async function LiveOutcomeHandoff({ caseId, handoff }: { caseId: string; handoff: OutcomeHandoff }) {
  const reviewer = await resolveReviewer();
  const session: HandoffSession = reviewer.ok
    ? { live: true, signedIn: true, mode: reviewer.method, expiresAt: reviewer.expiresAt }
    : { live: true, signedIn: false, mode: reviewAuthMode() };
  return <OutcomeHandoffCard handoff={handoff} actions={outcomeHandoffActions(caseId, handoff, session)} />;
}
