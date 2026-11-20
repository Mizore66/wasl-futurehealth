"use client";

import { useId, useState } from "react";
import { Button } from "@/components/ui/button";
import { COMMUNICATION_CHANNELS, IDENTITY_CHECKS, type CaseAction } from "@/lib/api/schema";

type Channel = (typeof COMMUNICATION_CHANNELS)[number];
type Check = (typeof IDENTITY_CHECKS)[number];
export type CommunicationAction = Extract<CaseAction, { action: "initial_notification" | "receipt_acknowledgement" }>;

const CHANNEL_LABEL: Record<Channel, string> = { phone: "Phone", video: "Video call", in_person: "In person" };
const CHECK_LABEL: Record<Check, string> = {
  name_and_dob: "Name and date of birth",
  name_and_emirates_id: "Name and Emirates ID",
  name_and_mrn: "Name and medical record number",
};

/**
 * Explicit staff-assisted communication recording (issue #197). Notification and receipt are separate actions with
 * separate labels; neither records that the patient understood the next step (teach-back does that). Nothing is sent:
 * the coordinator says what happened on a call they made, after confirming two identifiers.
 */
export function CommunicationForm({
  step,
  template,
  disabled,
  onRecord,
}: {
  step: "notify" | "receipt";
  template?: string;
  disabled: boolean;
  onRecord: (action: CommunicationAction) => void;
}) {
  const id = useId();
  const [channel, setChannel] = useState<Channel>("phone");
  const [check, setCheck] = useState<Check | "">("");
  const notify = step === "notify";
  const title = notify ? "Result notification" : "Receipt of the result";
  const label = notify ? "Record result notification" : "Record receipt acknowledged";
  if (notify && !template) {
    return (
      <section aria-label={title} className="mt-4" data-testid="communication-form">
        <h3 className="m-0 text-section font-medium">{title}</h3>
        <p className="m-0 mt-1 text-muted">The approved wording for telling the patient is not available, so the notification cannot be recorded here.</p>
      </section>
    );
  }
  return (
    <section aria-label={title} className="mt-4" data-testid="communication-form">
      <h3 className="m-0 text-section font-medium">{title}</h3>
      <p className="m-0 mt-1 text-muted">
        {notify
          ? `After telling the patient, using only the approved wording ${template}. A call outcome such as Reached does not record this.`
          : "After the patient confirms they received the result. This is not a check that they understood the next step."}
      </p>
      <form
        className="mt-2 flex flex-wrap items-end gap-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (!check) return;
          const occurred_at = new Date().toISOString().replace(/\.\d{3}Z$/, "Z");
          onRecord(
            notify
              ? { action: "initial_notification", occurred_at, channel, template: template!, identity_check: check }
              : { action: "receipt_acknowledgement", occurred_at, channel, identity_check: check },
          );
        }}
      >
        <label htmlFor={`${id}-channel`} className="flex min-w-0 flex-col gap-1 text-small text-muted">
          How
          <select id={`${id}-channel`} value={channel} onChange={(e) => setChannel(e.target.value as Channel)} className="h-11 rounded-control border border-input bg-surface px-2 text-body text-ink">
            {COMMUNICATION_CHANNELS.map((c) => (
              <option key={c} value={c}>
                {CHANNEL_LABEL[c]}
              </option>
            ))}
          </select>
        </label>
        <label htmlFor={`${id}-check`} className="flex min-w-0 flex-col gap-1 text-small text-muted">
          Identity confirmed with
          <select id={`${id}-check`} required value={check} onChange={(e) => setCheck(e.target.value as Check)} className="h-11 rounded-control border border-input bg-surface px-2 text-body text-ink">
            <option value="">Choose two identifiers</option>
            {IDENTITY_CHECKS.map((c) => (
              <option key={c} value={c}>
                {CHECK_LABEL[c]}
              </option>
            ))}
          </select>
        </label>
        <Button type="submit" variant="chip" size="touch" disabled={disabled || !check} className="border-control">
          {label}
        </Button>
      </form>
    </section>
  );
}
