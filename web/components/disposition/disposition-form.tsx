"use client";

import { useId, useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { recordDisposition } from "@/app/app/patients/[id]/disposition/actions";
import { FIT_REOFFERS, HOLD_PLANS, REFUSAL_REASONS, type DispositionKind } from "./model";

const FIELD = "mt-1 min-h-11 w-full rounded-control border border-control bg-surface px-3 text-ink";
const AREA = "mt-1 min-h-24 w-full rounded-control border border-control bg-surface px-3 py-2 text-ink";
const SUBMIT = "min-h-11 self-start rounded-control bg-cta px-4 font-medium text-white hover:bg-ink aria-disabled:opacity-60";

function Field({ id, label, hint, children }: { id: string; label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div>
      <label htmlFor={id} className="block font-medium">
        {label}
      </label>
      {hint && <p className="m-0 text-small text-muted">{hint}</p>}
      {children}
    </div>
  );
}

/**
 * One form for both disposition kinds. The kind decides the fields; the case, its head and the signed-in credential are
 * bound on the server. The form never offers a "done" or "closed" choice: a hold plan is a plan, and a refusal end state
 * says no procedure happened.
 */
export function DispositionForm({ caseId, expectedHead, kind, crc }: { caseId: string; expectedHead: string; kind: DispositionKind; crc: boolean }) {
  const id = useId();
  const router = useRouter();
  const [pending, start] = useTransition();
  const [error, setError] = useState<string>();
  const [saved, setSaved] = useState<string>();
  const [declinePlan, setDeclinePlan] = useState<"follow_up" | "closed_declined">("follow_up");

  function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const value = (k: string) => String(form.get(k) ?? "").trim();
    const disposition =
      kind === "clinician_disposition"
        ? { action: kind, reason: value("reason"), plan: value("plan"), details: value("details"), owner: value("owner"), review_date: value("review_date") }
        : {
            action: kind,
            reason: value("reason"),
            counselling: value("counselling"),
            safety_net: value("safety_net"),
            plan: declinePlan,
            ...(declinePlan === "follow_up" ? { follow_up_owner: value("follow_up_owner"), next_review: value("next_review") } : {}),
            ...(crc ? { fit_reoffer: value("fit_reoffer") } : {}),
          };
    start(async () => {
      const res = await recordDisposition({ caseId, expectedHead, disposition });
      if (!res.ok) {
        setError(res.error);
        setSaved(undefined);
        return;
      }
      setError(undefined);
      setSaved(res.message);
      router.refresh();
    });
  }

  const f = (name: string) => `${id}-${name}`;
  return (
    <form onSubmit={submit} className="mt-3 flex flex-col gap-3" data-testid={`form-${kind}`} aria-label={kind === "clinician_disposition" ? "Plan for the clinician hold" : "Informed-refusal disposition"}>
      {kind === "clinician_disposition" ? (
        <>
          <Field id={f("reason")} label="Reason for the decision (required)">
            <textarea id={f("reason")} name="reason" required maxLength={1000} className={AREA} />
          </Field>
          <Field id={f("plan")} label="Plan (required)" hint="A plan for what happens next. It does not record a test or exam as done.">
            <select id={f("plan")} name="plan" required defaultValue="" className={FIELD}>
              <option value="" disabled>
                Choose a supported plan
              </option>
              {Object.entries(HOLD_PLANS).map(([v, l]) => (
                <option key={v} value={v}>
                  {l}
                </option>
              ))}
            </select>
          </Field>
          <Field id={f("details")} label="Plan details (required)">
            <textarea id={f("details")} name="details" required maxLength={1000} className={AREA} />
          </Field>
          <Field id={f("owner")} label="Responsible owner (required)">
            <input id={f("owner")} name="owner" required maxLength={64} className={FIELD} />
          </Field>
          <Field id={f("review_date")} label="Review date (required)">
            <input id={f("review_date")} name="review_date" type="date" required className={FIELD} />
          </Field>
        </>
      ) : (
        <>
          <Field id={f("reason")} label="Refusal reason (required)">
            <select id={f("reason")} name="reason" required defaultValue="" className={FIELD}>
              <option value="" disabled>
                Choose a reason
              </option>
              {Object.entries(REFUSAL_REASONS).map(([v, l]) => (
                <option key={v} value={v}>
                  {l}
                </option>
              ))}
            </select>
          </Field>
          <Field id={f("counselling")} label="Counselling given (required)" hint="What the risks of not having the procedure are, as explained to the patient.">
            <textarea id={f("counselling")} name="counselling" required maxLength={1000} className={AREA} />
          </Field>
          <Field id={f("safety_net")} label="Safety-net advice (required)" hint="Symptoms that should bring the patient back.">
            <textarea id={f("safety_net")} name="safety_net" required maxLength={1000} className={AREA} />
          </Field>
          {crc && (
            <Field id={f("fit_reoffer")} label="FIT re-offer (required for bowel screening)">
              <select id={f("fit_reoffer")} name="fit_reoffer" required defaultValue="" className={FIELD}>
                <option value="" disabled>
                  Record the FIT re-offer
                </option>
                {Object.entries(FIT_REOFFERS).map(([v, l]) => (
                  <option key={v} value={v}>
                    {l}
                  </option>
                ))}
              </select>
            </Field>
          )}
          <fieldset className="m-0 border-0 p-0">
            <legend className="font-medium">Next step (required)</legend>
            <label className="mt-1 flex min-h-11 items-center gap-2">
              <input type="radio" name="decline_plan" value="follow_up" checked={declinePlan === "follow_up"} onChange={() => setDeclinePlan("follow_up")} />
              Owned follow-up review
            </label>
            <label className="flex min-h-11 items-center gap-2">
              <input type="radio" name="decline_plan" value="closed_declined" checked={declinePlan === "closed_declined"} onChange={() => setDeclinePlan("closed_declined")} />
              Non-procedure end state: informed refusal, no procedure
            </label>
          </fieldset>
          {declinePlan === "follow_up" && (
            <>
              <Field id={f("follow_up_owner")} label="Follow-up owner (required)">
                <input id={f("follow_up_owner")} name="follow_up_owner" required maxLength={64} className={FIELD} />
              </Field>
              <Field id={f("next_review")} label="Next review date (required)">
                <input id={f("next_review")} name="next_review" type="date" required className={FIELD} />
              </Field>
            </>
          )}
        </>
      )}
      {error && (
        <p role="alert" className="m-0 rounded-control bg-pill-negative-bg px-3 py-2 text-pill-negative-ink" data-testid="disposition-error">
          {error}
        </p>
      )}
      {saved && (
        <p role="status" className="m-0 rounded-control bg-pill-positive-bg px-3 py-2 text-pill-positive-ink" data-testid="disposition-saved">
          {saved}
        </p>
      )}
      <button type="submit" aria-disabled={pending} disabled={pending} className={SUBMIT}>
        {kind === "clinician_disposition" ? "Save clinician plan" : "Save informed-refusal disposition"}
      </button>
    </form>
  );
}
