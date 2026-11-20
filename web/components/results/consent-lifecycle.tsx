import { CountBars } from "@/components/results/charts";

const fmt = (n: number | undefined) => (n === undefined ? "not reported" : n.toLocaleString("en-US"));

/** Issue #9. Each entry: the dashboard field, the plain label, and what the backend counts (pipeline/report7.go, `consent_timing`). */
export const LIFECYCLE = [
  {
    key: "intents_ever_held_for_consent",
    label: "Held",
    what: "Waited because there was no permission to contact yet. Includes ones sent later.",
  },
  {
    key: "released_total",
    label: "Released",
    what: "Cleared to send. Includes ones that never waited.",
  },
  {
    key: "intents_cancelled_by_consent",
    label: "Cancelled",
    what: "Stopped because consent was refused or withdrawn.",
  },
] as const;

export const GUARDS = [
  {
    key: "released_without_granted_consent",
    label: "No consent",
    what: "Cleared to send without permission to contact on file.",
  },
  {
    key: "released_after_opt_out_on_file",
    label: "After opt-out",
    what: "Cleared to send after the patient had already opted out. This is the violation.",
  },
  {
    key: "released_before_opt_out_recorded",
    label: "Before opt-out",
    what: "Cleared to send before the patient opted out later.",
  },
] as const;

/**
 * The consent hold, release and cancellation counts for the active run, read from `dashboard.metrics.consent_timing`.
 * Nothing is hardcoded: every figure is a field of the dashboard payload. Counts are message intents, not patients.
 * Labels stay one or two words; what each count means is in the tooltip and the table view.
 */
export function ConsentLifecycle({ timing }: { timing: Record<string, number> }) {
  const released = timing.released_total ?? 0;
  return (
    <section aria-labelledby="consent-h" data-testid="consent-lifecycle" className="bg-surface px-pad py-5 lg:col-span-2">
      <h2 id="consent-h" className="m-0 text-section font-medium">
        Consent: held, released, cancelled
      </h2>
      <p className="m-0 mt-1 text-small text-muted">Simulated planned messages, not patients. No message was sent.</p>

      <div className="mt-4 grid grid-cols-1 gap-x-10 gap-y-5 lg:grid-cols-2">
        <div className="min-w-0">
          <CountBars
            caption="Planned messages by consent outcome. A message held and then released counts in both."
            tableHead={["Outcome", "Planned messages"]}
            rows={LIFECYCLE.map((s) => ({ label: s.label, what: s.what, count: timing[s.key] ?? 0, testId: `consent-${s.key}` }))}
          />
        </div>
        <div className="min-w-0">
          <h3 className="m-0 text-body font-medium">Consent rules broken</h3>
          <div className="mt-2">
            <CountBars
              caption="Released messages that broke a consent rule, out of all released"
              tableHead={["Rule", "Released messages"]}
              rows={GUARDS.map((g) => ({ label: g.label, what: g.what, count: timing[g.key] ?? 0, of: released, testId: `consent-${g.key}` }))}
            />
          </div>
          <p data-testid="consent-opted_out_loops" className="m-0 mt-2 text-small text-muted">
            {fmt(timing.opted_out_loops)} cases had an opt-out on file (this counts cases, not messages).
          </p>
        </div>
      </div>
    </section>
  );
}
