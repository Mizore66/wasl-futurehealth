import Link from "next/link";
import { Card } from "@/components/case/sections";
import { caseHref } from "@/lib/domain/ids";
import { CLINICIAN_HOLDS, FIT_REOFFERS, REFUSAL_REASONS, recordedWords, reviewDateWords, type DispositionContext } from "./model";

const short = (h: unknown) => (typeof h === "string" && h ? h.slice(0, 12) : "not recorded");
const roleWords = (r: unknown) => (r === "senior_clinician" ? "senior clinician" : r === "clinician" ? "clinician" : "clinician role not recorded");

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-wrap gap-x-2">
      <dt className="text-muted">{label}</dt>
      <dd className="m-0">{children}</dd>
    </div>
  );
}

/**
 * What a clinician has decided for this case and what still waits on one. Renders nothing when the case has no
 * clinician-decision hold, no decline and no recorded disposition. `link` adds the way into the workflow.
 */
export function DispositionCard({ ctx, link = true }: { ctx: DispositionContext; link?: boolean }) {
  const { hold, holdPlan, decline, declinePlan, open } = ctx;
  if (!hold && !decline && !declinePlan) return null;
  return (
    <Card
      id="disposition"
      title="Clinician disposition"
      note="A recorded plan or informed refusal. It is not completed care: no appointment, test result or procedure is recorded, and the screening record stays as the pipeline left it."
    >
      <div className="mt-3 flex flex-col gap-4" data-testid="disposition-card">
        {hold && (
          <div data-testid="disposition-hold">
            <p className="m-0 font-medium">{CLINICIAN_HOLDS[hold].label}</p>
            {holdPlan ? (
              <dl className="m-0 mt-2 flex flex-col gap-1">
                <Row label="Status">{recordedWords("clinician_disposition", holdPlan)}</Row>
                <Row label="Clinician">{String(holdPlan.actor ?? "recorded")}</Row>
                {holdPlan.reason !== undefined && <Row label="Rationale">{String(holdPlan.reason)}</Row>}
                {holdPlan.details !== undefined && <Row label="Plan details">{String(holdPlan.details)}</Row>}
                <Row label="Owner">{String(holdPlan.owner)}</Row>
                <Row label="Review date">{reviewDateWords(holdPlan.review_date)}</Row>
              </dl>
            ) : (
              <p className="m-0 mt-1">Waiting on a clinician: no plan recorded yet. The case stays open.</p>
            )}
          </div>
        )}
        {(decline || declinePlan) && (
          <div data-testid="disposition-decline">
            {decline && !decline.answered && (
              <p className="m-0 font-medium">
                Patient declined{decline.at ? ` (${decline.at.slice(0, 10)})` : ""}, reported by the coordinator · GP to sign the informed refusal
              </p>
            )}
            {declinePlan && (
              <dl className="m-0 mt-2 flex flex-col gap-1">
                <Row label="Status">{recordedWords("decline_disposition", declinePlan)}</Row>
                <Row label="Signed by">
                  {String(declinePlan.actor ?? "recorded")} ({roleWords(declinePlan.role)})
                </Row>
                <Row label="Answers decline">event {short(declinePlan.decline_event)}</Row>
                <Row label="Reason">{REFUSAL_REASONS[declinePlan.reason as keyof typeof REFUSAL_REASONS] ?? String(declinePlan.reason)}</Row>
                {declinePlan.counselling !== undefined && <Row label="Counselling">{String(declinePlan.counselling)}</Row>}
                {declinePlan.safety_net !== undefined && <Row label="Safety net">{String(declinePlan.safety_net)}</Row>}
                {declinePlan.fit_reoffer !== undefined && <Row label="FIT re-offer">{FIT_REOFFERS[declinePlan.fit_reoffer as keyof typeof FIT_REOFFERS] ?? String(declinePlan.fit_reoffer)}</Row>}
              </dl>
            )}
          </div>
        )}
        {link && open.length > 0 && (
          <p className="m-0">
            <Link
              href={`${caseHref("/app/patients", ctx.caseId)}/disposition`}
              className="inline-flex min-h-11 items-center rounded-control bg-cta px-4 font-medium text-white no-underline hover:bg-ink hover:text-white"
            >
              Record clinician disposition
            </Link>
          </p>
        )}
      </div>
    </Card>
  );
}
